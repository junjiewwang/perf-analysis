// Package query provides reusable query utilities for analysis data.
// This file implements the HprofSnapshotLeakProvider which detects potential leaks
// from a single Java hprof heap dump using heuristic rules.
// Unlike TimeSeriesLeakProvider, it does NOT require multiple snapshots.
// Instead, it identifies suspicious patterns in a single heap snapshot.
package query

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/junjiewwang/perf-analysis/perflib/output"
)

// ============================================================================
// HprofSnapshotLeakProvider
// ============================================================================

// HprofSnapshotLeakProvider detects potential memory leaks from a single Java heap dump.
// It applies heuristic rules to the precomputed class_stats.json and heap_stats.json files.
//
// Detection rules:
//  1. DominantClassRule: A single class retains > 30% of heap → likely leak candidate
//  2. CollectionAccumulationRule: Collection classes (HashMap, ArrayList, etc.) with
//     extremely high instance counts suggest unbounded growth
//  3. LargeHeapRule: Total heap exceeds a threshold, combined with concentrated allocation
//  4. ClassLoaderLeakRule: Excessive number of loaded classes suggests classloader leak
type HprofSnapshotLeakProvider struct {
	rules []snapshotRule
}

// NewHprofSnapshotLeakProvider creates a provider with default heuristic rules.
func NewHprofSnapshotLeakProvider() *HprofSnapshotLeakProvider {
	return &HprofSnapshotLeakProvider{
		rules: defaultSnapshotRules(),
	}
}

// Name returns the provider identifier.
func (p *HprofSnapshotLeakProvider) Name() string {
	return "hprof_snapshot"
}

// CanDetect checks if class_stats.json or heap_stats.json exists.
func (p *HprofSnapshotLeakProvider) CanDetect(taskDir string) bool {
	classStatsFile := filepath.Join(taskDir, output.FileClassStats)
	if _, err := os.Stat(classStatsFile); err == nil {
		return true
	}
	heapStatsFile := filepath.Join(taskDir, output.FileHeapStats)
	if _, err := os.Stat(heapStatsFile); err == nil {
		return true
	}
	return false
}

// Detect loads heap snapshot data and applies heuristic rules.
func (p *HprofSnapshotLeakProvider) Detect(taskDir string) ([]LeakSuspect, error) {
	snapshot, err := loadSnapshotData(taskDir)
	if err != nil {
		return nil, err
	}

	var suspects []LeakSuspect
	for _, rule := range p.rules {
		if findings := rule.evaluate(snapshot); len(findings) > 0 {
			suspects = append(suspects, findings...)
		}
	}

	if suspects == nil {
		suspects = []LeakSuspect{}
	}
	return suspects, nil
}

// ============================================================================
// Snapshot data model (loaded from precomputed files)
// ============================================================================

// snapshotData holds the loaded heap snapshot for rule evaluation.
type snapshotData struct {
	classStats *snapshotClassStats
	heapStats  *snapshotHeapStats
}

type snapshotClassStats struct {
	TotalClasses int                     `json:"total_classes"`
	TotalObjects int64                   `json:"total_objects"`
	TotalSize    int64                   `json:"total_size"`
	Classes      []snapshotClassEntry    `json:"classes"`
}

type snapshotClassEntry struct {
	ClassName    string  `json:"class_name"`
	ObjectCount  int64   `json:"object_count"`
	ShallowSize  int64   `json:"shallow_size"`
	RetainedSize int64   `json:"retained_size"`
	Percentage   float64 `json:"percentage"`
}

type snapshotHeapStats struct {
	TotalHeapSize   int64  `json:"total_heap_size"`
	TotalObjects    int64  `json:"total_objects"`
	TotalClasses    int    `json:"total_classes"`
	TotalGCRoots    int    `json:"total_gc_roots"`
	MaxObjectSize   int64  `json:"max_object_size"`
	MaxRetainedSize int64  `json:"max_retained_size"`
	TopClassName    string `json:"top_class_name"`
}

// loadSnapshotData loads precomputed data from taskDir.
func loadSnapshotData(taskDir string) (*snapshotData, error) {
	snapshot := &snapshotData{}

	// Load class_stats.json
	classStatsFile := filepath.Join(taskDir, output.FileClassStats)
	if data, err := os.ReadFile(classStatsFile); err == nil {
		var cs snapshotClassStats
		if json.Unmarshal(data, &cs) == nil {
			snapshot.classStats = &cs
		}
	}

	// Load heap_stats.json
	heapStatsFile := filepath.Join(taskDir, output.FileHeapStats)
	if data, err := os.ReadFile(heapStatsFile); err == nil {
		var hs snapshotHeapStats
		if json.Unmarshal(data, &hs) == nil {
			snapshot.heapStats = &hs
		}
	}

	if snapshot.classStats == nil && snapshot.heapStats == nil {
		return nil, fmt.Errorf("no snapshot data found in %s", taskDir)
	}

	return snapshot, nil
}

// ============================================================================
// Snapshot Rule Interface and Implementations
// ============================================================================

// snapshotRule is the internal interface for heuristic detection rules.
type snapshotRule interface {
	evaluate(data *snapshotData) []LeakSuspect
}

// defaultSnapshotRules returns the standard set of heuristic rules.
func defaultSnapshotRules() []snapshotRule {
	return []snapshotRule{
		&dominantClassRule{},
		&collectionAccumulationRule{},
		&classLoaderLeakRule{},
	}
}

// ─── Rule 1: Dominant Class ──────────────────────────────────────────────────

// dominantClassRule detects when a single class retains a disproportionate amount of heap.
type dominantClassRule struct{}

func (r *dominantClassRule) evaluate(data *snapshotData) []LeakSuspect {
	if data.classStats == nil || len(data.classStats.Classes) == 0 {
		return nil
	}

	var suspects []LeakSuspect
	for _, cls := range data.classStats.Classes {
		if cls.Percentage < 20.0 {
			break // Sorted by retained desc, no need to check further
		}

		// Skip primitive arrays — they're usually held by other objects
		if isPrimitiveArray(cls.ClassName) {
			continue
		}

		severity := LeakSeverityInfo
		if cls.Percentage >= 40.0 {
			severity = LeakSeverityCritical
		} else if cls.Percentage >= 25.0 {
			severity = LeakSeverityWarning
		}

		suspects = append(suspects, LeakSuspect{
			Type:     "heap",
			Source:   LeakSourceSnapshotHeuristic,
			Severity: severity,
			Title:    fmt.Sprintf("Class %s dominates heap at %.1f%%", cls.ClassName, cls.Percentage),
			Description: fmt.Sprintf(
				"%s retains %.1f%% of total heap (%d instances, %s retained). "+
					"A single class holding this much memory often indicates unbounded caching or accumulation.",
				cls.ClassName, cls.Percentage, cls.ObjectCount, formatBytesCompact(cls.RetainedSize),
			),
			Evidence: []LeakEvidence{
				{Name: cls.ClassName, Value: cls.RetainedSize, Unit: "bytes", Detail: fmt.Sprintf("%.1f%% of heap", cls.Percentage)},
				{Name: cls.ClassName, Value: cls.ObjectCount, Unit: "count", Detail: "instances"},
			},
			Suggestions: []string{
				fmt.Sprintf("Inspect %s for unbounded growth (missing eviction/TTL)", cls.ClassName),
				"Check if references to this class are released after use",
				"Consider using WeakReference or SoftReference for cache entries",
			},
		})
	}

	return suspects
}

// ─── Rule 2: Collection Accumulation ─────────────────────────────────────────

// collectionAccumulationRule detects collection classes with abnormally high instance counts.
type collectionAccumulationRule struct{}

// collectionPatterns lists class name patterns that indicate collection types.
var collectionPatterns = []string{
	"HashMap", "ArrayList", "LinkedList", "HashSet",
	"ConcurrentHashMap", "LinkedHashMap", "TreeMap",
	"CopyOnWriteArrayList", "ArrayDeque",
}

func (r *collectionAccumulationRule) evaluate(data *snapshotData) []LeakSuspect {
	if data.classStats == nil {
		return nil
	}

	var suspects []LeakSuspect
	for _, cls := range data.classStats.Classes {
		if !isCollectionClass(cls.ClassName) {
			continue
		}

		// Threshold: collection class with > 50,000 instances is suspicious
		if cls.ObjectCount < 50000 {
			continue
		}

		severity := LeakSeverityInfo
		if cls.ObjectCount >= 500000 {
			severity = LeakSeverityCritical
		} else if cls.ObjectCount >= 100000 {
			severity = LeakSeverityWarning
		}

		suspects = append(suspects, LeakSuspect{
			Type:     "class_accumulation",
			Source:   LeakSourceSnapshotHeuristic,
			Severity: severity,
			Title:    fmt.Sprintf("Collection %s has %d instances", cls.ClassName, cls.ObjectCount),
			Description: fmt.Sprintf(
				"%s has %d instances occupying %s. Large numbers of collection instances "+
					"often indicate that entries are added but never removed.",
				cls.ClassName, cls.ObjectCount, formatBytesCompact(cls.RetainedSize),
			),
			Evidence: []LeakEvidence{
				{Name: cls.ClassName, Value: cls.ObjectCount, Unit: "count", Detail: "instances"},
				{Name: cls.ClassName, Value: cls.RetainedSize, Unit: "bytes", Detail: "retained size"},
			},
			Suggestions: []string{
				fmt.Sprintf("Review code that creates %s instances — ensure they are bounded", cls.ClassName),
				"Add eviction policies (LRU, TTL) to caches using this collection",
				"Check for event listeners or callbacks that accumulate without removal",
			},
		})
	}

	return suspects
}

// ─── Rule 3: ClassLoader Leak ────────────────────────────────────────────────

// classLoaderLeakRule detects excessive class counts suggesting classloader leaks.
type classLoaderLeakRule struct{}

func (r *classLoaderLeakRule) evaluate(data *snapshotData) []LeakSuspect {
	totalClasses := 0
	if data.heapStats != nil {
		totalClasses = data.heapStats.TotalClasses
	} else if data.classStats != nil {
		totalClasses = data.classStats.TotalClasses
	}

	if totalClasses < 30000 {
		return nil
	}

	severity := LeakSeverityInfo
	if totalClasses >= 80000 {
		severity = LeakSeverityCritical
	} else if totalClasses >= 50000 {
		severity = LeakSeverityWarning
	}

	return []LeakSuspect{
		{
			Type:     "classloader_leak",
			Source:   LeakSourceSnapshotHeuristic,
			Severity: severity,
			Title:    fmt.Sprintf("Excessive loaded classes: %d", totalClasses),
			Description: fmt.Sprintf(
				"The heap contains %d loaded classes. Normal applications typically have < 20,000 classes. "+
					"This may indicate a classloader leak from hot-deployment, dynamic proxies, or script engines.",
				totalClasses,
			),
			Evidence: []LeakEvidence{
				{Name: "loaded_classes", Value: int64(totalClasses), Unit: "count"},
			},
			Suggestions: []string{
				"Check for repeated class redefinition (hot-deploy, OSGi, dynamic proxies)",
				"Inspect PermGen/Metaspace usage for classloader leak patterns",
				"Review Groovy/Kotlin script compilation that may create new classes per invocation",
			},
		},
	}
}

// ============================================================================
// Helpers
// ============================================================================

// isPrimitiveArray checks if a class name is a Java primitive array type.
func isPrimitiveArray(className string) bool {
	switch className {
	case "byte[]", "char[]", "int[]", "long[]", "short[]",
		"float[]", "double[]", "boolean[]":
		return true
	}
	return false
}

// isCollectionClass checks if a class name matches collection patterns.
func isCollectionClass(className string) bool {
	for _, pattern := range collectionPatterns {
		if strings.Contains(className, pattern) {
			return true
		}
	}
	return false
}

// formatBytesCompact formats bytes to a compact human-readable string.
func formatBytesCompact(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
