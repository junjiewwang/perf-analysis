// Package model defines output data abstractions for different analysis types.
package model

// HeapClassStats holds statistics for a single class in heap analysis.
type HeapClassStats struct {
	ClassName     string         `json:"class_name"`
	InstanceCount int64          `json:"instance_count"`
	TotalSize     int64          `json:"total_size"`
	Percentage    float64        `json:"percentage"`
	RetainedSize  int64          `json:"retained_size,omitempty"`
	Retainers     []HeapRetainer `json:"retainers,omitempty"`
	GCRootPaths   []*GCRootPath  `json:"gc_root_paths,omitempty"`
}

// HeapRetainer describes what retains instances of a class.
type HeapRetainer struct {
	RetainerClass string  `json:"retainer_class"`
	FieldName     string  `json:"field_name,omitempty"`
	RetainedSize  int64   `json:"retained_size"`
	RetainedCount int64   `json:"retained_count"`
	Percentage    float64 `json:"percentage"`
	Depth         int     `json:"depth,omitempty"`
}

// GCRootPathNode represents a node in a GC root path.
type GCRootPathNode struct {
	ClassName string `json:"class_name"`
	FieldName string `json:"field_name,omitempty"`
	Size      int64  `json:"size"`
}

// GCRootPath represents a path from GC Root to an object.
type GCRootPath struct {
	RootType string            `json:"root_type"`
	Path     []*GCRootPathNode `json:"path"`
	Depth    int               `json:"depth"`
}

// HeapReferenceNode represents a node in the reference graph visualization.
type HeapReferenceNode struct {
	ID           string `json:"id"`
	ClassName    string `json:"class_name"`
	Size         int64  `json:"size"`
	RetainedSize int64  `json:"retained_size"`
	IsGCRoot     bool   `json:"is_gc_root"`
	GCRootType   string `json:"gc_root_type,omitempty"`
}

// HeapReferenceEdge represents an edge in the reference graph visualization.
type HeapReferenceEdge struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	FieldName string `json:"field_name,omitempty"`
}

// HeapReferenceGraph represents the reference graph for a class.
type HeapReferenceGraph struct {
	ClassName string              `json:"class_name"`
	Nodes     []HeapReferenceNode `json:"nodes"`
	Edges     []HeapReferenceEdge `json:"edges"`
}

// HeapBusinessRetainer represents a business-level class that retains memory.
type HeapBusinessRetainer struct {
	ClassName     string   `json:"class_name"`
	FieldPath     []string `json:"field_path"`
	RetainedSize  int64    `json:"retained_size"`
	RetainedCount int64    `json:"retained_count"`
	Percentage    float64  `json:"percentage"`
	Depth         int      `json:"depth"`
	IsGCRoot      bool     `json:"is_gc_root"`
	GCRootType    string   `json:"gc_root_type,omitempty"`
}

// HeapBiggestObject represents a large object with its details.
type HeapBiggestObject struct {
	ObjectID     string            `json:"object_id"`
	ClassName    string            `json:"class_name"`
	ShallowSize  int64             `json:"shallow_size"`
	RetainedSize int64             `json:"retained_size"`
	Fields       []HeapObjectField `json:"fields,omitempty"`
	GCRootPath   *HeapGCRootPath   `json:"gc_root_path,omitempty"`
}

// HeapObjectField represents a field value in an object.
type HeapObjectField struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Value        interface{} `json:"value,omitempty"`
	RefID        string      `json:"ref_id,omitempty"`
	RefClass     string      `json:"ref_class,omitempty"`
	ShallowSize  int64       `json:"shallow_size,omitempty"`
	RetainedSize int64       `json:"retained_size,omitempty"`
	HasChildren  bool        `json:"has_children,omitempty"`
	IsStatic     bool        `json:"is_static,omitempty"`
}

// HeapGCRootPath represents a path from GC Root to an object.
type HeapGCRootPath struct {
	RootType string               `json:"root_type"`
	Path     []HeapGCRootPathNode `json:"path"`
	Depth    int                  `json:"depth"`
}

// HeapGCRootPathNode represents a node in a GC root path.
type HeapGCRootPathNode struct {
	ClassName string `json:"class_name"`
	FieldName string `json:"field_name,omitempty"`
	Size      int64  `json:"size"`
}

// HeapGCRootsData holds GC roots analysis data for persistence.
// This is written to gc_roots.json during analysis for fast loading in serve mode.
type HeapGCRootsData struct {
	Summary HeapGCRootsSummary `json:"summary"`
	Classes []HeapGCRootClass  `json:"classes"`
}

// HeapGCRootsSummary holds summary statistics for GC roots.
type HeapGCRootsSummary struct {
	TotalRoots    int   `json:"total_roots"`
	TotalClasses  int   `json:"total_classes"`
	TotalRetained int64 `json:"total_retained"`
	TotalShallow  int64 `json:"total_shallow"`
}

// HeapGCRootClass represents GC roots grouped by class name (like IDEA).
type HeapGCRootClass struct {
	ClassName     string               `json:"class_name"`
	RootType      string               `json:"root_type,omitempty"`
	TotalShallow  int64                `json:"total_shallow"`
	TotalRetained int64                `json:"total_retained"`
	InstanceCount int                  `json:"instance_count"`
	Roots         []HeapGCRootInstance `json:"roots,omitempty"`
}

// HeapGCRootInstance represents a single GC root instance.
type HeapGCRootInstance struct {
	ObjectID     string `json:"object_id"`
	RootType     string `json:"root_type"`
	ShallowSize  int64  `json:"shallow_size"`
	RetainedSize int64  `json:"retained_size"`
	ThreadID     string `json:"thread_id,omitempty"`
	FrameIndex   int    `json:"frame_index,omitempty"`
}

// HeapAnalysisData holds Java heap dump analysis data.
type HeapAnalysisData struct {
	HeapReportFile    string                           `json:"heap_report_file"`
	HistogramFile     string                           `json:"histogram_file"`
	Format            string                           `json:"format,omitempty"`
	IDSize            int                              `json:"id_size,omitempty"`
	Timestamp         int64                            `json:"timestamp,omitempty"`
	TotalClasses      int                              `json:"total_classes"`
	TotalInstances    int64                            `json:"total_instances"`
	TotalHeapSize     int64                            `json:"total_heap_size"`
	HeapSizeHuman     string                           `json:"heap_size_human"`
	LiveBytes         int64                            `json:"live_bytes,omitempty"`
	LiveObjects       int64                            `json:"live_objects,omitempty"`
	TopClasses        []HeapClassStats                 `json:"top_classes"`
	BiggestObjects    []HeapBiggestObject              `json:"biggest_objects,omitempty"`
	ReferenceGraphs   map[string]*HeapReferenceGraph   `json:"reference_graphs,omitempty"`
	BusinessRetainers map[string][]HeapBusinessRetainer `json:"business_retainers,omitempty"`
}

// Type returns the analysis data type.
func (d *HeapAnalysisData) Type() AnalysisDataType {
	return DataTypeHeapDump
}

// Summary returns a summary of the heap analysis.
func (d *HeapAnalysisData) Summary() map[string]interface{} {
	return map[string]interface{}{
		"format":           d.Format,
		"id_size":          d.IDSize,
		"timestamp":        d.Timestamp,
		"total_classes":    d.TotalClasses,
		"total_instances":  d.TotalInstances,
		"total_heap_size":  d.TotalHeapSize,
		"heap_size_human":  d.HeapSizeHuman,
		"live_bytes":       d.LiveBytes,
		"live_objects":     d.LiveObjects,
		"heap_report_file": d.HeapReportFile,
		"histogram_file":   d.HistogramFile,
	}
}

// TopItems returns the top classes from heap analysis.
func (d *HeapAnalysisData) TopItems() []TopItem {
	items := make([]TopItem, 0, len(d.TopClasses))
	for _, cls := range d.TopClasses {
		items = append(items, TopItem{
			Name:       cls.ClassName,
			Value:      cls.TotalSize,
			Percentage: cls.Percentage,
			Extra: map[string]interface{}{
				"instance_count": cls.InstanceCount,
			},
		})
	}
	return items
}
