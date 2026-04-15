package collapsed

import (
	"github.com/perf-analysis/internal/parser"

	libcollapsed "github.com/perf-analysis/perflib/parser/collapsed"
)

// Type aliases - delegate to perflib/parser/collapsed.
type (
	// Factory creates new Collapsed format parsers.
	Factory = libcollapsed.Factory
)

// NewFactory creates a new CollapsedParserFactory.
var NewFactory = libcollapsed.NewFactory

// RegisterWithRegistry registers the collapsed parser with the given registry.
func RegisterWithRegistry(registry *parser.Registry) {
	libcollapsed.RegisterWithRegistry(registry)
}

// WithTopNOption returns a parser option that sets the TopN value.
var WithTopNOption = libcollapsed.WithTopNOption

// WithStrictModeOption returns a parser option that enables strict mode.
var WithStrictModeOption = libcollapsed.WithStrictModeOption

// WithIncludeSwapperOption returns a parser option that includes swapper.
var WithIncludeSwapperOption = libcollapsed.WithIncludeSwapperOption
