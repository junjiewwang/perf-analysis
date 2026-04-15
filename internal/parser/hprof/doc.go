// Package hprof provides parsing and analysis functionality for Java HPROF heap dump files.
//
// This package is a thin wrapper around github.com/junjiewwang/perf-analysis/perflib/parser/hprof,
// providing backward compatibility for the internal API.
//
// # Key Types
//
//   - Parser: Main HPROF parser
//   - ReferenceGraph: Object reference graph with GC root tracking
//   - BiggestObjectsBuilder: Builds biggest objects list with lazy field loading
//   - HeapAnalysisResult: Complete analysis result
//
// # Usage Example
//
//	parser := hprof.NewParser(hprof.DefaultParserOptions())
//	result, err := parser.Parse(ctx, reader)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Access analysis results
//	for _, cls := range result.TopClasses {
//	    fmt.Printf("%s: %d instances, %d bytes\n",
//	        cls.ClassName, cls.InstanceCount, cls.TotalSize)
//	}
package hprof
