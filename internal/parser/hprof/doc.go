// Package hprof provides parsing and analysis functionality for Java HPROF heap dump files.
//
// # Package Organization
//
// The package is organized into logical groups using file name prefixes:
//
// ## Core Parsing (core_*.go, parser.go, types.go)
//   - types.go: Core type definitions (RecordTag, HeapDumpTag, ClassInfo, etc.)
//   - parser.go: Main HPROF parser implementation
//   - core_reader.go: Binary data reader for HPROF format
//   - core_result_builder.go: Analysis result builder
//
// ## Reference Graph (graph_*.go)
//   - graph_reference.go: Core ReferenceGraph data structure
//   - graph_gc_root.go: GC root types and path finding
//   - graph_indexed.go: High-performance indexed graph (CSR format)
//   - graph_buffer_pool.go: Memory pools for BFS/DFS traversal
//
// ## Dominator Tree (dom_*.go)
//   - dom_dominator.go: Standard Lengauer-Tarjan dominator algorithm
//   - dom_hierarchical.go: Hierarchical parallel dominator algorithm
//   - dom_parallel.go: Parallel computation helpers
//
// ## Analysis (analysis_*.go)
//   - analysis_biggest_objects.go: Biggest objects analysis (like IDEA's view)
//   - analysis_retainer.go: Retainer analysis (who holds references)
//   - analysis_retained_calc.go: Retained size calculation strategies
//   - analysis_retained_debug.go: Retained size debugging/comparison
//
// ## Serialization (serial_*.go)
//   - serial_serializer.go: Protobuf serialization/deserialization
//   - serial_async.go: Async serialization support
//
// ## Parallel Processing (parallel_*.go)
//   - parallel_analyzer.go: Parallel analysis coordinator
//
// ## Utilities (util_*.go)
//   - util_bitset.go: Bitset type aliases (-> pkg/collections)
//   - util_worker_pool.go: Worker pool aliases (-> pkg/parallel)
//   - util_compression.go: Compression aliases (-> pkg/compression)
//   - util_mmap_store.go: Memory-mapped file storage for large heaps
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
//
// # Key Types
//
//   - Parser: Main HPROF parser
//   - ReferenceGraph: Object reference graph with GC root tracking
//   - BiggestObjectsBuilder: Builds biggest objects list with lazy field loading
//   - HeapAnalysisResult: Complete analysis result
//
// # Performance Notes
//
// For large heaps (>1M objects), the package automatically selects optimized algorithms:
//   - Hierarchical parallel dominator algorithm
//   - CSR-format indexed graph storage
//   - Memory-mapped file storage (optional)
package hprof
