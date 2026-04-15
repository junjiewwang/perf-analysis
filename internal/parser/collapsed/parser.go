// Package collapsed implements parsing of collapsed stack format data.
// This is a thin wrapper around github.com/junjiewwang/perf-analysis/perflib/parser/collapsed.
package collapsed

import (
	"fmt"

	libcollapsed "github.com/junjiewwang/perf-analysis/perflib/parser/collapsed"
)

// Constants - re-export from perflib.
const (
	// DefaultTopN is the default number of top functions to return.
	DefaultTopN = libcollapsed.DefaultTopN

	// DefaultMinSamplePercent is the minimum percentage for samples to be included.
	DefaultMinSamplePercent = libcollapsed.DefaultMinSamplePercent
)

// Type aliases - delegate all types to perflib/parser/collapsed.
type (
	// ParserOptions holds configuration options for the collapsed parser.
	ParserOptions = libcollapsed.ParserOptions

	// Parser implements the collapsed format parser.
	Parser = libcollapsed.Parser
)

// DefaultParserOptions returns default parser options.
var DefaultParserOptions = libcollapsed.DefaultParserOptions

// NewParser creates a new collapsed format parser.
var NewParser = libcollapsed.NewParser

// IsCollapsedFormat checks if the content appears to be in collapsed format.
var IsCollapsedFormat = libcollapsed.IsCollapsedFormat

// ErrInvalidFormat is returned when the collapsed format is invalid.
var ErrInvalidFormat = fmt.Errorf("invalid collapsed format")
