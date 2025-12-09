package collapsed

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/perf-analysis/pkg/model"
)

const (
	// DefaultTopN is the default number of top functions to return.
	DefaultTopN = 15

	// DefaultMinSamplePercent is the minimum percentage for samples to be included.
	DefaultMinSamplePercent = 0.01
)

// ParserOptions holds configuration options for the collapsed parser.
type ParserOptions struct {
	// TopN specifies how many top functions to return.
	TopN int

	// IncludeSwapper includes swapper (idle) thread in statistics.
	IncludeSwapper bool

	// MinSamplePercent is the minimum percentage for samples to be retained.
	MinSamplePercent float64

	// StrictMode enables strict parsing that fails on any error.
	StrictMode bool
}

// DefaultParserOptions returns default parser options.
func DefaultParserOptions() *ParserOptions {
	return &ParserOptions{
		TopN:             DefaultTopN,
		IncludeSwapper:   false,
		MinSamplePercent: DefaultMinSamplePercent,
		StrictMode:       false,
	}
}

// Parser implements the collapsed format parser.
type Parser struct {
	opts *ParserOptions
}

// NewParser creates a new collapsed format parser.
func NewParser(opts *ParserOptions) *Parser {
	if opts == nil {
		opts = DefaultParserOptions()
	}
	return &Parser{opts: opts}
}

// Parse parses collapsed format data from the reader.
func (p *Parser) Parse(ctx context.Context, reader io.Reader) (*model.ParseResult, error) {
	result := &model.ParseResult{
		Samples:            make([]*model.Sample, 0),
		ThreadStats:        make(map[string]*model.ThreadInfo),
		TopFuncs:           make(model.TopFuncsMap),
		TopFuncsCallstacks: make(map[string]*model.CallStackInfo),
	}

	// Statistics collectors
	funcSamples := make(map[string]int64)               // function -> sample count
	funcSamplesWithSwapper := make(map[string]int64)    // includes swapper
	threadSamples := make(map[int]*threadStats)         // tid -> stats
	funcCallstacks := make(map[string]map[string]int64) // func -> callstack -> count

	var totalSamples int64
	var totalSamplesWithSwapper int64

	scanner := bufio.NewScanner(reader)
	lineNum := 0

	for scanner.Scan() {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse the line
		sample, err := p.parseLine(line)
		if err != nil {
			if p.opts.StrictMode {
				return nil, fmt.Errorf("line %d: %w", lineNum, err)
			}
			continue
		}

		if sample == nil {
			continue // Skip invalid data
		}

		// Check if this is a swapper thread
		isSwapper := IsSwapperThread(sample.ThreadName)

		// Update total counts
		totalSamplesWithSwapper += sample.Value
		if !isSwapper {
			totalSamples += sample.Value
		}

		// Get top function (leaf of call stack)
		topFunc := ""
		if len(sample.CallStack) > 0 {
			topFunc = sample.CallStack[len(sample.CallStack)-1]
		}

		// Update function samples
		if topFunc != "" {
			funcSamplesWithSwapper[topFunc] += sample.Value
			if !isSwapper {
				funcSamples[topFunc] += sample.Value
			}

			// Track call stacks for this function
			callStackStr := strings.Join(sample.CallStack, ";")
			if _, ok := funcCallstacks[topFunc]; !ok {
				funcCallstacks[topFunc] = make(map[string]int64)
			}
			funcCallstacks[topFunc][callStackStr] += sample.Value
		}

		// Update thread statistics
		tidKey := sample.TID
		if _, ok := threadSamples[tidKey]; !ok {
			threadSamples[tidKey] = &threadStats{
				TID:        sample.TID,
				ThreadName: sample.ThreadName,
				Samples:    0,
			}
		}
		threadSamples[tidKey].Samples += sample.Value

		// Store sample
		result.Samples = append(result.Samples, sample)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	result.TotalSamples = totalSamples

	// Build top functions map
	result.TopFuncs = p.buildTopFuncs(funcSamples, totalSamples)

	// Build thread stats
	result.ThreadStats = p.buildThreadStats(threadSamples, totalSamplesWithSwapper)

	// Build top functions callstacks
	result.TopFuncsCallstacks = p.buildTopFuncsCallstacks(funcCallstacks, result.TopFuncs)

	return result, nil
}

// SupportedFormats returns the formats supported by this parser.
func (p *Parser) SupportedFormats() []string {
	return []string{"collapsed", "folded"}
}

// Name returns the name of this parser.
func (p *Parser) Name() string {
	return "collapsed"
}

// parseLine parses a single line of collapsed format data.
// Format: stack count
// Example: process-pid/tid;func1;func2;func3 123
func (p *Parser) parseLine(line string) (*model.Sample, error) {
	// Split by last space to get stack and count
	lastSpace := strings.LastIndex(line, " ")
	if lastSpace == -1 {
		return nil, ErrInvalidFormat
	}

	stack := line[:lastSpace]
	countStr := strings.TrimSpace(line[lastSpace+1:])

	count, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid count value: %w", err)
	}

	// Parse the stack
	parts := strings.Split(stack, ";")
	if len(parts) == 0 {
		return nil, ErrInvalidFormat
	}

	// First part is thread info
	threadFrame := parts[0]

	// Check for invalid data pattern
	if IsInvalidData(threadFrame) {
		return nil, nil // Skip invalid data silently
	}

	// Extract thread info
	threadInfo := ExtractThreadInfo(threadFrame)

	// Build call stack (skip thread frame)
	callStack := make([]string, 0, len(parts)-1)
	startIdx := 1

	// Skip APM format prefix if present
	if startIdx < len(parts) && apmFormatRegex.MatchString(parts[startIdx]) {
		startIdx++
	}

	for i := startIdx; i < len(parts); i++ {
		frame := parts[i]
		if frame == "" || frame == "[]" {
			continue
		}
		// Extract function name (without module)
		funcName, _ := SplitFuncAndModule(frame)
		callStack = append(callStack, funcName)
	}

	return &model.Sample{
		ThreadName: threadInfo.ThreadName,
		TID:        threadInfo.TID,
		CallStack:  callStack,
		Value:      count,
	}, nil
}

// threadStats holds intermediate thread statistics.
type threadStats struct {
	TID        int
	ThreadName string
	Samples    int64
}

// buildTopFuncs builds the top functions map from sample data.
func (p *Parser) buildTopFuncs(funcSamples map[string]int64, totalSamples int64) model.TopFuncsMap {
	result := make(model.TopFuncsMap)

	if totalSamples == 0 {
		return result
	}

	// Sort by sample count and take top N
	entries := make([]funcEntry, 0, len(funcSamples))
	for name, samples := range funcSamples {
		entries = append(entries, funcEntry{name: name, samples: samples})
	}

	// Sort descending by samples
	sortFuncEntries(entries)

	// Take top N
	topN := p.opts.TopN
	if topN > len(entries) {
		topN = len(entries)
	}

	for i := 0; i < topN; i++ {
		entry := entries[i]
		pct := float64(entry.samples) / float64(totalSamples) * 100
		result[entry.name] = model.TopFuncValue{
			Self: pct,
		}
	}

	return result
}

// buildThreadStats builds thread statistics from sample data.
func (p *Parser) buildThreadStats(threadSamples map[int]*threadStats, totalSamples int64) map[string]*model.ThreadInfo {
	result := make(map[string]*model.ThreadInfo)

	if totalSamples == 0 {
		return result
	}

	for _, ts := range threadSamples {
		key := fmt.Sprintf("%d", ts.TID)
		result[key] = &model.ThreadInfo{
			TID:        ts.TID,
			ThreadName: ts.ThreadName,
			Samples:    ts.Samples,
			Percentage: float64(ts.Samples) / float64(totalSamples) * 100,
		}
	}

	return result
}

// buildTopFuncsCallstacks builds call stack information for top functions.
func (p *Parser) buildTopFuncsCallstacks(funcCallstacks map[string]map[string]int64, topFuncs model.TopFuncsMap) map[string]*model.CallStackInfo {
	result := make(map[string]*model.CallStackInfo)

	for funcName := range topFuncs {
		callstacks, ok := funcCallstacks[funcName]
		if !ok {
			continue
		}

		// Sort callstacks by count and take top 5
		type stackEntry struct {
			stack string
			count int64
		}
		entries := make([]stackEntry, 0, len(callstacks))
		for stack, count := range callstacks {
			entries = append(entries, stackEntry{stack: stack, count: count})
		}

		// Sort descending
		for i := 0; i < len(entries)-1; i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[j].count > entries[i].count {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}

		// Take top 5
		topStacks := make([]string, 0, 5)
		for i := 0; i < len(entries) && i < 5; i++ {
			topStacks = append(topStacks, entries[i].stack)
		}

		result[funcName] = &model.CallStackInfo{
			FunctionName: funcName,
			CallStacks:   topStacks,
			Count:        len(callstacks),
		}
	}

	return result
}

// sortFuncEntries sorts function entries by sample count in descending order.
func sortFuncEntries(entries []funcEntry) {
	// Simple bubble sort for small arrays, could be optimized
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].samples > entries[i].samples {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

// funcEntry type used by sortFuncEntries
type funcEntry struct {
	name    string
	samples int64
}

// IsCollapsedFormat checks if the content appears to be in collapsed format.
// Collapsed format: stack_trace count
// Example: process-pid/tid;func1;func2 123
func IsCollapsedFormat(line string) bool {
	pattern := regexp.MustCompile(`^[^;]+(;[^;]+)*\s\d+$`)
	return pattern.MatchString(strings.TrimSpace(line))
}

// Error definitions for the parser.
var (
	ErrInvalidFormat = fmt.Errorf("invalid collapsed format")
)
