// Package parser defines the interfaces for parsing profiling data.
package parser

import (
	"context"
	"io"

	"github.com/perf-analysis/pkg/model"
)

// Parser is the interface for parsing profiling data.
type Parser interface {
	// Parse parses profiling data from the reader.
	Parse(ctx context.Context, reader io.Reader) (*model.ParseResult, error)

	// SupportedFormats returns the formats supported by this parser.
	SupportedFormats() []string

	// Name returns the name of this parser.
	Name() string
}

// ParserFactory is a function that creates a new Parser instance.
type ParserFactory func(opts ...ParserOption) (Parser, error)

// ParserOption is a function that configures a Parser.
type ParserOption func(interface{})

// Registry holds registered parsers.
type Registry struct {
	parsers map[string]Parser
}

// NewRegistry creates a new parser Registry.
func NewRegistry() *Registry {
	return &Registry{
		parsers: make(map[string]Parser),
	}
}

// Register registers a parser with the given format name.
func (r *Registry) Register(format string, parser Parser) {
	r.parsers[format] = parser
}

// Get returns a parser for the given format.
func (r *Registry) Get(format string) (Parser, bool) {
	parser, ok := r.parsers[format]
	return parser, ok
}

// ParseOptions holds common parsing options.
type ParseOptions struct {
	// StrictMode enables strict parsing that fails on any error.
	StrictMode bool

	// MaxSamples limits the maximum number of samples to parse.
	MaxSamples int64

	// FilterThreads filters samples by thread name pattern.
	FilterThreads string

	// SkipKernel skips kernel stack frames.
	SkipKernel bool

	// NormalizeFrames normalizes frame names (e.g., remove addresses).
	NormalizeFrames bool
}

// DefaultParseOptions returns default parsing options.
func DefaultParseOptions() *ParseOptions {
	return &ParseOptions{
		StrictMode:      false,
		MaxSamples:      0, // no limit
		NormalizeFrames: true,
	}
}
