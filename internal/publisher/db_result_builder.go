package publisher

import (
	"encoding/json"

	"github.com/perf-analysis/pkg/model"
)

// DBResultFields holds the fields extracted from analysis data
// that are needed for database persistence.
type DBResultFields struct {
	TopFuncs          string
	ActiveThreadsJSON string
	FlameGraphKey     string
	CallGraphKey      string
}

// ExtractDBFields extracts database-specific fields from analysis data
// and the uploaded file key map. This consolidates the type-switch logic
// that was previously inlined in processor.saveResults().
func ExtractDBFields(data model.AnalysisData, uploadedFiles map[string]string) *DBResultFields {
	fields := &DBResultFields{}
	if data == nil {
		return fields
	}

	switch d := data.(type) {
	case *model.CPUProfilingData:
		fields.TopFuncs = marshalJSON(d.TopFuncs)
		fields.ActiveThreadsJSON = marshalJSON(d.ThreadStats)
		fields.FlameGraphKey = uploadedFiles["Flame Graph"]
		fields.CallGraphKey = uploadedFiles["Call Graph"]

	case *model.AllocationData:
		fields.TopFuncs = marshalJSON(d.TopAllocators)
		fields.ActiveThreadsJSON = marshalJSON(d.ThreadStats)
		fields.FlameGraphKey = uploadedFiles["Allocation Flame Graph"]
		fields.CallGraphKey = uploadedFiles["Allocation Call Graph"]

	case *model.HeapAnalysisData:
		fields.ActiveThreadsJSON = marshalJSON(d.Summary())
		fields.TopFuncs = marshalJSON(d.TopClasses)
		fields.FlameGraphKey = uploadedFiles["Heap Report"]
		fields.CallGraphKey = uploadedFiles["Class Histogram"]

	case *model.TracingData:
		fields.TopFuncs = marshalJSON(d.TopFuncs)
		fields.ActiveThreadsJSON = marshalJSON(d.ThreadStats)
		fields.FlameGraphKey = uploadedFiles["Flame Graph"]
		fields.CallGraphKey = uploadedFiles["Call Graph"]
	}

	return fields
}

// marshalJSON marshals v to a JSON string, returning empty string on error.
func marshalJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}
