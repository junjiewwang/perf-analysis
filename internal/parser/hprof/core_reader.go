package hprof

import (
	"io"

	libhprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// Reader provides buffered reading of HPROF binary data.
type Reader = libhprof.Reader

// NewReader creates a new HPROF reader.
func NewReader(r io.Reader) *Reader {
	return libhprof.NewReader(r)
}
