// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// ============================================================================
// Memory-Mapped File Store for Large Heap Processing
// ============================================================================
//
// For heaps larger than available RAM, we use memory-mapped files to store
// intermediate data structures. This allows the OS to manage paging and
// reduces Go's GC pressure.
//
// Key data structures stored in mmap:
// - Object metadata (classID, size): 16 bytes per object
// - Dominator indices: 4 bytes per object
// - Retained sizes: 8 bytes per object
// - Edge data (CSR format): variable
//
// For 100M objects:
// - In-memory maps: ~10GB+ (with map overhead)
// - Mmap arrays: ~2.8GB (16+4+8 bytes per object)

// MmapConfig configures memory-mapped storage behavior.
type MmapConfig struct {
	// TempDir is the directory for temporary mmap files.
	// Default: os.TempDir()
	TempDir string

	// Threshold is the minimum number of objects before using mmap.
	// Default: 10_000_000 (10M objects)
	Threshold int

	// PageSize is the allocation unit for mmap files.
	// Default: 64MB
	PageSize int64

	// PreFault pre-faults pages to avoid page faults during processing.
	// Default: false (let OS handle paging)
	PreFault bool
}

// DefaultMmapConfig returns default mmap configuration.
func DefaultMmapConfig() MmapConfig {
	return MmapConfig{
		TempDir:   os.TempDir(),
		Threshold: 10_000_000,
		PageSize:  64 * 1024 * 1024, // 64MB
		PreFault:  false,
	}
}

// ============================================================================
// MmapArray - Generic memory-mapped array
// ============================================================================

// MmapArray provides a memory-mapped array of fixed-size elements.
// It automatically grows by allocating new pages.
type MmapArray[T any] struct {
	file     *os.File
	data     []byte
	elemSize int
	capacity int64
	length   atomic.Int64
	mu       sync.RWMutex
	closed   bool
}

// NewMmapArray creates a new memory-mapped array.
func NewMmapArray[T any](filename string, initialCapacity int64) (*MmapArray[T], error) {
	var zero T
	elemSize := int(unsafe.Sizeof(zero))

	file, err := os.CreateTemp("", filename+"_*.mmap")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	// Calculate initial file size (round up to page size)
	pageSize := int64(os.Getpagesize())
	fileSize := initialCapacity * int64(elemSize)
	fileSize = ((fileSize + pageSize - 1) / pageSize) * pageSize
	if fileSize < pageSize {
		fileSize = pageSize
	}

	// Truncate file to initial size
	if err := file.Truncate(fileSize); err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, fmt.Errorf("failed to truncate file: %w", err)
	}

	// Memory map the file
	data, err := syscall.Mmap(int(file.Fd()), 0, int(fileSize),
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, fmt.Errorf("failed to mmap: %w", err)
	}

	return &MmapArray[T]{
		file:     file,
		data:     data,
		elemSize: elemSize,
		capacity: fileSize / int64(elemSize),
	}, nil
}

// grow increases the capacity of the array.
func (a *MmapArray[T]) grow(newCapacity int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if newCapacity <= a.capacity {
		return nil
	}

	// Calculate new file size
	pageSize := int64(os.Getpagesize())
	newFileSize := newCapacity * int64(a.elemSize)
	newFileSize = ((newFileSize + pageSize - 1) / pageSize) * pageSize

	// Unmap current data
	if err := syscall.Munmap(a.data); err != nil {
		return fmt.Errorf("failed to munmap: %w", err)
	}

	// Extend file
	if err := a.file.Truncate(newFileSize); err != nil {
		return fmt.Errorf("failed to extend file: %w", err)
	}

	// Remap
	data, err := syscall.Mmap(int(a.file.Fd()), 0, int(newFileSize),
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to remap: %w", err)
	}

	a.data = data
	a.capacity = newFileSize / int64(a.elemSize)
	return nil
}

// Get returns the element at index i.
func (a *MmapArray[T]) Get(i int64) T {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if i < 0 || i >= a.length.Load() {
		var zero T
		return zero
	}

	offset := i * int64(a.elemSize)
	return *(*T)(unsafe.Pointer(&a.data[offset]))
}

// Set sets the element at index i.
func (a *MmapArray[T]) Set(i int64, value T) error {
	if i >= a.capacity {
		if err := a.grow(i * 2); err != nil {
			return err
		}
	}

	a.mu.RLock()
	offset := i * int64(a.elemSize)
	*(*T)(unsafe.Pointer(&a.data[offset])) = value
	a.mu.RUnlock()

	// Update length if necessary
	for {
		current := a.length.Load()
		if i < current {
			break
		}
		if a.length.CompareAndSwap(current, i+1) {
			break
		}
	}
	return nil
}

// Len returns the current length of the array.
func (a *MmapArray[T]) Len() int64 {
	return a.length.Load()
}

// Cap returns the current capacity of the array.
func (a *MmapArray[T]) Cap() int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.capacity
}

// Close unmaps and closes the array, deleting the backing file.
func (a *MmapArray[T]) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true

	var errs []error
	if err := syscall.Munmap(a.data); err != nil {
		errs = append(errs, fmt.Errorf("munmap: %w", err))
	}

	filename := a.file.Name()
	if err := a.file.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close: %w", err))
	}

	if err := os.Remove(filename); err != nil {
		errs = append(errs, fmt.Errorf("remove: %w", err))
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Sync flushes changes to disk.
// Note: On some platforms (e.g., macOS), Msync may not be available in syscall package.
// We use a file sync as fallback.
func (a *MmapArray[T]) Sync() error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	// Sync the underlying file to ensure data is written to disk
	return a.file.Sync()
}

// ============================================================================
// MmapObjectStore - Memory-mapped object storage
// ============================================================================

// ObjectRecord represents object metadata in mmap storage.
// Packed to 24 bytes for efficient storage.
type ObjectRecord struct {
	ObjectID uint64 // 8 bytes
	ClassID  uint64 // 8 bytes
	Size     int64  // 8 bytes
}

// MmapObjectStore stores object data in memory-mapped files.
type MmapObjectStore struct {
	// Object metadata
	objects *MmapArray[ObjectRecord]

	// Object ID to index mapping (still in memory for fast lookup)
	objToIdx map[uint64]int32

	// Dominator indices (4 bytes each)
	dominators *MmapArray[int32]

	// Retained sizes (8 bytes each)
	retainedSizes *MmapArray[int64]

	// Configuration
	config MmapConfig

	// Thread safety
	mu sync.RWMutex

	// Count
	count atomic.Int64
}

// NewMmapObjectStore creates a new memory-mapped object store.
func NewMmapObjectStore(estimatedObjects int, config MmapConfig) (*MmapObjectStore, error) {
	objects, err := NewMmapArray[ObjectRecord]("objects", int64(estimatedObjects))
	if err != nil {
		return nil, fmt.Errorf("failed to create objects mmap: %w", err)
	}

	dominators, err := NewMmapArray[int32]("dominators", int64(estimatedObjects))
	if err != nil {
		objects.Close()
		return nil, fmt.Errorf("failed to create dominators mmap: %w", err)
	}

	retainedSizes, err := NewMmapArray[int64]("retained", int64(estimatedObjects))
	if err != nil {
		objects.Close()
		dominators.Close()
		return nil, fmt.Errorf("failed to create retained sizes mmap: %w", err)
	}

	return &MmapObjectStore{
		objects:       objects,
		objToIdx:      make(map[uint64]int32, estimatedObjects),
		dominators:    dominators,
		retainedSizes: retainedSizes,
		config:        config,
	}, nil
}

// AddObject adds an object to the store.
func (s *MmapObjectStore) AddObject(objectID, classID uint64, size int64) (int32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already exists
	if idx, ok := s.objToIdx[objectID]; ok {
		return idx, nil
	}

	idx := int32(s.count.Load())
	record := ObjectRecord{
		ObjectID: objectID,
		ClassID:  classID,
		Size:     size,
	}

	if err := s.objects.Set(int64(idx), record); err != nil {
		return -1, err
	}

	// Initialize retained size to shallow size
	if err := s.retainedSizes.Set(int64(idx), size); err != nil {
		return -1, err
	}

	// Initialize dominator to -1 (no dominator)
	if err := s.dominators.Set(int64(idx), -1); err != nil {
		return -1, err
	}

	s.objToIdx[objectID] = idx
	s.count.Add(1)
	return idx, nil
}

// GetIndex returns the index for an object ID.
func (s *MmapObjectStore) GetIndex(objectID uint64) int32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if idx, ok := s.objToIdx[objectID]; ok {
		return idx
	}
	return -1
}

// GetObjectID returns the object ID for an index.
func (s *MmapObjectStore) GetObjectID(idx int32) uint64 {
	return s.objects.Get(int64(idx)).ObjectID
}

// GetClassID returns the class ID for an index.
func (s *MmapObjectStore) GetClassID(idx int32) uint64 {
	return s.objects.Get(int64(idx)).ClassID
}

// GetSize returns the shallow size for an index.
func (s *MmapObjectStore) GetSize(idx int32) int64 {
	return s.objects.Get(int64(idx)).Size
}

// GetRetainedSize returns the retained size for an index.
func (s *MmapObjectStore) GetRetainedSize(idx int32) int64 {
	return s.retainedSizes.Get(int64(idx))
}

// SetRetainedSize sets the retained size for an index.
func (s *MmapObjectStore) SetRetainedSize(idx int32, size int64) error {
	return s.retainedSizes.Set(int64(idx), size)
}

// GetDominator returns the dominator index for an index.
func (s *MmapObjectStore) GetDominator(idx int32) int32 {
	return s.dominators.Get(int64(idx))
}

// SetDominator sets the dominator index for an index.
func (s *MmapObjectStore) SetDominator(idx int32, domIdx int32) error {
	return s.dominators.Set(int64(idx), domIdx)
}

// Count returns the number of objects.
func (s *MmapObjectStore) Count() int32 {
	return int32(s.count.Load())
}

// Close closes the store and removes backing files.
func (s *MmapObjectStore) Close() error {
	var errs []error
	if err := s.objects.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := s.dominators.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := s.retainedSizes.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// ============================================================================
// MmapEdgeStore - Memory-mapped edge storage (CSR format)
// ============================================================================

// MmapEdgeStore stores edges in CSR format using memory-mapped files.
type MmapEdgeStore struct {
	// offsets[i] = start index in targets for node i's edges
	offsets *MmapArray[int64]

	// targets[offsets[i]:offsets[i+1]] = target indices for node i
	targets *MmapArray[int32]

	// fieldIDs for each edge
	fieldIDs *MmapArray[int32]

	// classIDs for each edge
	classIDs *MmapArray[uint64]

	// Field name interning (in memory)
	fieldNames []string
	fieldToID  map[string]int32

	// Node count
	nodeCount int32

	// Edge count
	edgeCount atomic.Int64

	// Thread safety
	mu sync.RWMutex

	// Building phase
	building bool
	edges    []struct {
		from    int32
		to      int32
		fieldID int32
		classID uint64
	}
}

// NewMmapEdgeStore creates a new memory-mapped edge store.
func NewMmapEdgeStore(nodeCount int, estimatedEdges int) (*MmapEdgeStore, error) {
	return &MmapEdgeStore{
		nodeCount:  int32(nodeCount),
		fieldNames: make([]string, 0, 1000),
		fieldToID:  make(map[string]int32, 1000),
		building:   true,
		edges: make([]struct {
			from    int32
			to      int32
			fieldID int32
			classID uint64
		}, 0, estimatedEdges),
	}, nil
}

// internFieldName returns the ID for a field name.
func (s *MmapEdgeStore) internFieldName(name string) int32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, ok := s.fieldToID[name]; ok {
		return id
	}
	id := int32(len(s.fieldNames))
	s.fieldNames = append(s.fieldNames, name)
	s.fieldToID[name] = id
	return id
}

// AddEdge adds an edge during the building phase.
func (s *MmapEdgeStore) AddEdge(from, to int32, fieldName string, classID uint64) {
	if !s.building {
		return
	}

	fieldID := s.internFieldName(fieldName)
	s.mu.Lock()
	s.edges = append(s.edges, struct {
		from    int32
		to      int32
		fieldID int32
		classID uint64
	}{from, to, fieldID, classID})
	s.mu.Unlock()
}

// Build finalizes the edge store and creates mmap files.
func (s *MmapEdgeStore) Build() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.building {
		return nil
	}
	s.building = false

	edgeCount := len(s.edges)

	// Create mmap arrays
	var err error
	s.offsets, err = NewMmapArray[int64]("offsets", int64(s.nodeCount+1))
	if err != nil {
		return err
	}

	s.targets, err = NewMmapArray[int32]("targets", int64(edgeCount))
	if err != nil {
		s.offsets.Close()
		return err
	}

	s.fieldIDs, err = NewMmapArray[int32]("fieldids", int64(edgeCount))
	if err != nil {
		s.offsets.Close()
		s.targets.Close()
		return err
	}

	s.classIDs, err = NewMmapArray[uint64]("classids", int64(edgeCount))
	if err != nil {
		s.offsets.Close()
		s.targets.Close()
		s.fieldIDs.Close()
		return err
	}

	// Count edges per node
	counts := make([]int64, s.nodeCount+1)
	for _, edge := range s.edges {
		counts[edge.from+1]++
	}

	// Convert to offsets (prefix sum)
	for i := int32(1); i <= s.nodeCount; i++ {
		counts[i] += counts[i-1]
		s.offsets.Set(int64(i), counts[i])
	}

	// Fill edge data
	currentIdx := make([]int64, s.nodeCount)
	copy(currentIdx, counts[:s.nodeCount])

	for _, edge := range s.edges {
		idx := currentIdx[edge.from]
		currentIdx[edge.from]++

		s.targets.Set(idx, edge.to)
		s.fieldIDs.Set(idx, edge.fieldID)
		s.classIDs.Set(idx, edge.classID)
	}

	s.edgeCount.Store(int64(edgeCount))

	// Clear building data
	s.edges = nil

	return nil
}

// GetEdges returns edges for a node.
func (s *MmapEdgeStore) GetEdges(nodeIdx int32) (targets []int32, fieldIDs []int32, classIDs []uint64) {
	if s.offsets == nil || nodeIdx < 0 || nodeIdx >= s.nodeCount {
		return nil, nil, nil
	}

	start := s.offsets.Get(int64(nodeIdx))
	end := s.offsets.Get(int64(nodeIdx + 1))
	count := end - start

	if count == 0 {
		return nil, nil, nil
	}

	targets = make([]int32, count)
	fieldIDs = make([]int32, count)
	classIDs = make([]uint64, count)

	for i := int64(0); i < count; i++ {
		targets[i] = s.targets.Get(start + i)
		fieldIDs[i] = s.fieldIDs.Get(start + i)
		classIDs[i] = s.classIDs.Get(start + i)
	}

	return targets, fieldIDs, classIDs
}

// GetFieldName returns the field name for a field ID.
func (s *MmapEdgeStore) GetFieldName(fieldID int32) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if fieldID < 0 || int(fieldID) >= len(s.fieldNames) {
		return ""
	}
	return s.fieldNames[fieldID]
}

// Close closes the store.
func (s *MmapEdgeStore) Close() error {
	var errs []error
	if s.offsets != nil {
		if err := s.offsets.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.targets != nil {
		if err := s.targets.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.fieldIDs != nil {
		if err := s.fieldIDs.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.classIDs != nil {
		if err := s.classIDs.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// ============================================================================
// Utility Functions
// ============================================================================

// writeInt32 writes an int32 to a byte slice at the given offset.
func writeInt32(data []byte, offset int64, value int32) {
	binary.LittleEndian.PutUint32(data[offset:], uint32(value))
}

// readInt32 reads an int32 from a byte slice at the given offset.
func readInt32(data []byte, offset int64) int32 {
	return int32(binary.LittleEndian.Uint32(data[offset:]))
}

// writeInt64 writes an int64 to a byte slice at the given offset.
func writeInt64(data []byte, offset int64, value int64) {
	binary.LittleEndian.PutUint64(data[offset:], uint64(value))
}

// readInt64 reads an int64 from a byte slice at the given offset.
func readInt64(data []byte, offset int64) int64 {
	return int64(binary.LittleEndian.Uint64(data[offset:]))
}

// ShouldUseMmap determines if mmap should be used based on object count.
func ShouldUseMmap(objectCount int, config MmapConfig) bool {
	return objectCount >= config.Threshold
}
