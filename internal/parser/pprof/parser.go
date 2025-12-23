// Package pprof provides parsing functionality for Go pprof profile data.
package pprof

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/google/pprof/profile"

	"github.com/perf-analysis/pkg/model"
)

// SampleType represents the type of sample in a pprof profile.
type SampleType string

const (
	// CPU sample types
	SampleTypeCPU     SampleType = "cpu"
	SampleTypeSamples SampleType = "samples"

	// Heap sample types
	SampleTypeInuseSpace   SampleType = "inuse_space"
	SampleTypeInuseObjects SampleType = "inuse_objects"
	SampleTypeAllocSpace   SampleType = "alloc_space"
	SampleTypeAllocObjects SampleType = "alloc_objects"

	// Goroutine sample types
	SampleTypeGoroutine SampleType = "goroutine"

	// Block/Mutex sample types
	SampleTypeContentions SampleType = "contentions"
	SampleTypeDelay       SampleType = "delay"
)

// Parser parses Go pprof profile data.
type Parser struct {
	profile *profile.Profile
}

// NewParser creates a new pprof parser.
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses pprof data from a reader.
func (p *Parser) Parse(r io.Reader) error {
	prof, err := profile.Parse(r)
	if err != nil {
		return fmt.Errorf("failed to parse pprof: %w", err)
	}
	p.profile = prof
	return nil
}

// Profile returns the underlying pprof profile.
func (p *Parser) Profile() *profile.Profile {
	return p.profile
}

// GetSampleTypes returns available sample types in the profile.
func (p *Parser) GetSampleTypes() []string {
	if p.profile == nil {
		return nil
	}
	types := make([]string, 0, len(p.profile.SampleType))
	for _, st := range p.profile.SampleType {
		types = append(types, st.Type)
	}
	return types
}

// GetDuration returns the profile duration in nanoseconds.
func (p *Parser) GetDuration() int64 {
	if p.profile == nil {
		return 0
	}
	return p.profile.DurationNanos
}

// GetTotalSamples returns total samples for a given sample type.
func (p *Parser) GetTotalSamples(sampleType SampleType) int64 {
	if p.profile == nil {
		return 0
	}
	idx := p.findSampleTypeIndex(string(sampleType))
	if idx < 0 {
		return 0
	}
	var total int64
	for _, sample := range p.profile.Sample {
		total += sample.Value[idx]
	}
	return total
}

// TopFunction represents a top function in the profile.
type TopFunction struct {
	Name       string
	Flat       int64
	FlatPct    float64
	Cum        int64
	CumPct     float64
	Module     string
	SourceFile string
	SourceLine int
}

// GetTopFunctions returns top N functions sorted by flat value.
func (p *Parser) GetTopFunctions(n int, sampleType SampleType, sortByCum bool) []TopFunction {
	if p.profile == nil {
		return nil
	}

	idx := p.findSampleTypeIndex(string(sampleType))
	if idx < 0 {
		// Try alternative names
		idx = p.findAlternativeSampleTypeIndex(sampleType)
		if idx < 0 {
			return nil
		}
	}

	// Aggregate by function
	funcStats := make(map[string]*TopFunction)
	var totalValue int64

	for _, sample := range p.profile.Sample {
		value := sample.Value[idx]
		if value == 0 {
			continue
		}
		totalValue += value

		// Process locations in the stack
		for i, loc := range sample.Location {
			for _, line := range loc.Line {
				if line.Function == nil {
					continue
				}
				funcName := line.Function.Name
				if funcName == "" {
					funcName = fmt.Sprintf("0x%x", loc.Address)
				}

				stat, ok := funcStats[funcName]
				if !ok {
					stat = &TopFunction{
						Name:       funcName,
						Module:     getModuleName(funcName),
						SourceFile: line.Function.Filename,
						SourceLine: int(line.Line),
					}
					funcStats[funcName] = stat
				}

				// Flat: only count the leaf function (first in stack)
				if i == 0 {
					stat.Flat += value
				}
				// Cumulative: count all functions in the stack
				stat.Cum += value
			}
		}
	}

	// Convert to slice and calculate percentages
	result := make([]TopFunction, 0, len(funcStats))
	for _, stat := range funcStats {
		if totalValue > 0 {
			stat.FlatPct = float64(stat.Flat) * 100.0 / float64(totalValue)
			stat.CumPct = float64(stat.Cum) * 100.0 / float64(totalValue)
		}
		result = append(result, *stat)
	}

	// Sort
	if sortByCum {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Cum > result[j].Cum
		})
	} else {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Flat > result[j].Flat
		})
	}

	// Limit to top N
	if n > 0 && len(result) > n {
		result = result[:n]
	}

	return result
}

// ToCollapsed converts the pprof profile to collapsed stack format.
// Returns a map of stack string to sample count.
func (p *Parser) ToCollapsed(sampleType SampleType) (map[string]int64, error) {
	if p.profile == nil {
		return nil, fmt.Errorf("profile not loaded")
	}

	idx := p.findSampleTypeIndex(string(sampleType))
	if idx < 0 {
		idx = p.findAlternativeSampleTypeIndex(sampleType)
		if idx < 0 {
			return nil, fmt.Errorf("sample type %q not found in profile", sampleType)
		}
	}

	result := make(map[string]int64)

	for _, sample := range p.profile.Sample {
		value := sample.Value[idx]
		if value == 0 {
			continue
		}

		// Build stack string (reverse order: root to leaf)
		stack := p.buildStackString(sample.Location)
		if stack == "" {
			continue
		}

		result[stack] += value
	}

	return result, nil
}

// ToSamples converts the pprof profile to model.Sample slice.
func (p *Parser) ToSamples(sampleType SampleType) ([]*model.Sample, error) {
	collapsed, err := p.ToCollapsed(sampleType)
	if err != nil {
		return nil, err
	}

	samples := make([]*model.Sample, 0, len(collapsed))
	for stack, count := range collapsed {
		frames := strings.Split(stack, ";")
		samples = append(samples, &model.Sample{
			CallStack: frames,
			Value:     count,
		})
	}

	return samples, nil
}

// buildStackString builds a collapsed stack string from locations.
func (p *Parser) buildStackString(locations []*profile.Location) string {
	if len(locations) == 0 {
		return ""
	}

	// Build frames in reverse order (root to leaf)
	frames := make([]string, 0, len(locations))
	for i := len(locations) - 1; i >= 0; i-- {
		loc := locations[i]
		for j := len(loc.Line) - 1; j >= 0; j-- {
			line := loc.Line[j]
			var funcName string
			if line.Function != nil {
				funcName = line.Function.Name
			}
			if funcName == "" {
				funcName = fmt.Sprintf("0x%x", loc.Address)
			}
			frames = append(frames, funcName)
		}
	}

	return strings.Join(frames, ";")
}

// findSampleTypeIndex finds the index of a sample type by name.
func (p *Parser) findSampleTypeIndex(typeName string) int {
	for i, st := range p.profile.SampleType {
		if st.Type == typeName {
			return i
		}
	}
	return -1
}

// findAlternativeSampleTypeIndex tries to find alternative sample type names.
func (p *Parser) findAlternativeSampleTypeIndex(sampleType SampleType) int {
	alternatives := map[SampleType][]string{
		SampleTypeCPU:          {"cpu", "nanoseconds", "samples"},
		SampleTypeSamples:      {"samples", "count"},
		SampleTypeInuseSpace:   {"inuse_space", "inuse_bytes"},
		SampleTypeInuseObjects: {"inuse_objects", "inuse_count"},
		SampleTypeAllocSpace:   {"alloc_space", "alloc_bytes"},
		SampleTypeAllocObjects: {"alloc_objects", "alloc_count"},
		SampleTypeGoroutine:    {"goroutine", "count"},
		SampleTypeContentions:  {"contentions", "count"},
		SampleTypeDelay:        {"delay", "nanoseconds"},
	}

	alts, ok := alternatives[sampleType]
	if !ok {
		return -1
	}

	for _, alt := range alts {
		if idx := p.findSampleTypeIndex(alt); idx >= 0 {
			return idx
		}
	}
	return -1
}

// getModuleName extracts module name from function name.
func getModuleName(funcName string) string {
	// Go function names are like: github.com/pkg/errors.Wrap
	lastSlash := strings.LastIndex(funcName, "/")
	if lastSlash < 0 {
		// No slash, might be runtime or main
		dot := strings.Index(funcName, ".")
		if dot > 0 {
			return funcName[:dot]
		}
		return funcName
	}

	// Find the package name after the last slash
	remaining := funcName[lastSlash+1:]
	dot := strings.Index(remaining, ".")
	if dot > 0 {
		return funcName[:lastSlash+1+dot]
	}
	return funcName[:lastSlash+1] + remaining
}

// DetectProfileType detects the type of pprof profile.
func (p *Parser) DetectProfileType() string {
	if p.profile == nil {
		return "unknown"
	}

	sampleTypes := p.GetSampleTypes()
	if len(sampleTypes) == 0 {
		return "unknown"
	}

	// Check for specific sample types
	for _, st := range sampleTypes {
		switch st {
		case "cpu", "nanoseconds":
			if p.containsSampleType("samples") || p.containsSampleType("count") {
				return "cpu"
			}
		case "inuse_space", "inuse_objects", "alloc_space", "alloc_objects":
			return "heap"
		case "goroutine":
			return "goroutine"
		case "contentions", "delay":
			return "block"
		}
	}

	// Default based on first sample type
	return sampleTypes[0]
}

// containsSampleType checks if profile contains a specific sample type.
func (p *Parser) containsSampleType(typeName string) bool {
	return p.findSampleTypeIndex(typeName) >= 0
}

// GetUnit returns the unit for a sample type.
func (p *Parser) GetUnit(sampleType SampleType) string {
	if p.profile == nil {
		return ""
	}
	idx := p.findSampleTypeIndex(string(sampleType))
	if idx < 0 {
		idx = p.findAlternativeSampleTypeIndex(sampleType)
	}
	if idx < 0 || idx >= len(p.profile.SampleType) {
		return ""
	}
	return p.profile.SampleType[idx].Unit
}
