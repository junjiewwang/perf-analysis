package collapsed

import (
	"github.com/perf-analysis/internal/parser"
)

// Factory creates new Collapsed format parsers.
type Factory struct{}

// NewFactory creates a new CollapsedParserFactory.
func NewFactory() *Factory {
	return &Factory{}
}

// Create creates a new Collapsed format parser with the given options.
func (f *Factory) Create(opts ...parser.ParserOption) (parser.Parser, error) {
	parserOpts := DefaultParserOptions()

	// Apply options
	for _, opt := range opts {
		opt(parserOpts)
	}

	return NewParser(parserOpts), nil
}

// RegisterWithRegistry registers the collapsed parser with the given registry.
func RegisterWithRegistry(registry *parser.Registry) {
	factory := NewFactory()
	p, _ := factory.Create()
	registry.Register("collapsed", p)
	registry.Register("folded", p)
}

// ParserOption implementations for Collapsed parser

// WithTopNOption returns a parser option that sets the TopN value.
func WithTopNOption(n int) parser.ParserOption {
	return func(opts interface{}) {
		if o, ok := opts.(*ParserOptions); ok {
			o.TopN = n
		}
	}
}

// WithStrictModeOption returns a parser option that enables strict mode.
func WithStrictModeOption(strict bool) parser.ParserOption {
	return func(opts interface{}) {
		if o, ok := opts.(*ParserOptions); ok {
			o.StrictMode = strict
		}
	}
}

// WithIncludeSwapperOption returns a parser option that includes swapper.
func WithIncludeSwapperOption(include bool) parser.ParserOption {
	return func(opts interface{}) {
		if o, ok := opts.(*ParserOptions); ok {
			o.IncludeSwapper = include
		}
	}
}
