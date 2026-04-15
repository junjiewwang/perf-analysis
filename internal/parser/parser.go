// Package parser defines the interfaces for parsing profiling data.
// This is a thin wrapper around github.com/junjiewwang/perf-analysis/perflib/parser.
package parser

import (
	libparser "github.com/junjiewwang/perf-analysis/perflib/parser"
)

// Type aliases - delegate all types to perflib/parser.
type (
	// Parser is the interface for parsing profiling data.
	Parser = libparser.Parser

	// ParserFactory is a function that creates a new Parser instance.
	ParserFactory = libparser.ParserFactory

	// ParserOption is a function that configures a Parser.
	ParserOption = libparser.ParserOption

	// Registry holds registered parsers.
	Registry = libparser.Registry

	// ParseOptions holds common parsing options.
	ParseOptions = libparser.ParseOptions
)

// NewRegistry creates a new parser Registry.
var NewRegistry = libparser.NewRegistry

// DefaultParseOptions returns default parsing options.
var DefaultParseOptions = libparser.DefaultParseOptions
