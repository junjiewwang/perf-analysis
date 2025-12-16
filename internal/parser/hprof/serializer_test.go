package hprof

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSerializeDeserialize(t *testing.T) {
	// Create a test ReferenceGraph
	g := NewReferenceGraphWithCapacity(1000)
	
	// Add class names (use object IDs as class IDs for simplicity)
	g.SetClassName(1000, "java.lang.String")
	g.SetClassName(2000, "java.util.ArrayList")
	g.SetClassName(3000, "com.example.MyClass")
	g.SetClassName(4000, "byte[]")
	
	// Add objects (objectID, classID, size)
	g.SetObjectInfo(100, 1000, 48)   // String object
	g.SetObjectInfo(101, 1000, 56)   // String object
	g.SetObjectInfo(200, 2000, 24)   // ArrayList object
	g.SetObjectInfo(300, 3000, 128)  // MyClass object
	g.SetObjectInfo(400, 4000, 1024) // byte[] object
	
	// Add references
	g.AddReference(ObjectReference{FromObjectID: 300, ToObjectID: 200, FromClassID: 3000, FieldName: "list"})
	g.AddReference(ObjectReference{FromObjectID: 200, ToObjectID: 100, FromClassID: 2000, FieldName: "elementData"})
	g.AddReference(ObjectReference{FromObjectID: 200, ToObjectID: 101, FromClassID: 2000, FieldName: "elementData"})
	g.AddReference(ObjectReference{FromObjectID: 300, ToObjectID: 400, FromClassID: 3000, FieldName: "data"})
	
	// Add GC roots - use object IDs that exist
	g.AddGCRoot(&GCRoot{ObjectID: 300, Type: GCRootJavaFrame, ThreadID: 1, FrameIndex: 0})
	
	// Note: Don't compute dominator tree for this simple test to avoid complexity
	// The serializer should work with or without dominator data
	
	// Serialize
	opts := DefaultSerializeOptions()
	opts.SourceFile = "test.hprof"
	data, stats, err := g.Serialize(opts)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	
	t.Logf("Serialization stats:")
	t.Logf("  Objects: %d", stats.Objects)
	t.Logf("  References: %d", stats.References)
	t.Logf("  GC Roots: %d", stats.GCRoots)
	t.Logf("  Classes: %d", stats.Classes)
	t.Logf("  Unique field names: %d", stats.UniqueFieldNames)
	t.Logf("  Raw size: %d bytes", stats.RawSize)
	t.Logf("  Compressed size: %d bytes", stats.CompressedSize)
	t.Logf("  Compression ratio: %.2fx", stats.CompressionRatio)
	t.Logf("  Duration: %v", stats.Duration)
	
	// Deserialize
	g2, err := DeserializeReferenceGraph(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}
	
	// Verify objects
	if len(g2.objectClass) != len(g.objectClass) {
		t.Errorf("Object count mismatch: got %d, want %d", len(g2.objectClass), len(g.objectClass))
	}
	
	// Verify class names
	if len(g2.classNames) != len(g.classNames) {
		t.Errorf("Class name count mismatch: got %d, want %d", len(g2.classNames), len(g.classNames))
	}
	for classID, name := range g.classNames {
		if g2.classNames[classID] != name {
			t.Errorf("Class name mismatch for %d: got %q, want %q", classID, g2.classNames[classID], name)
		}
	}
	
	// Verify references
	origRefs := 0
	for _, refs := range g.outgoingRefs {
		origRefs += len(refs)
	}
	deserRefs := 0
	for _, refs := range g2.outgoingRefs {
		deserRefs += len(refs)
	}
	if deserRefs != origRefs {
		t.Errorf("Reference count mismatch: got %d, want %d", deserRefs, origRefs)
	}
	
	// Verify GC roots
	if len(g2.gcRoots) != len(g.gcRoots) {
		t.Errorf("GC root count mismatch: got %d, want %d", len(g2.gcRoots), len(g.gcRoots))
	}
	
	// Dominator data not computed in this test, so skip verification
}

func TestSerializeToFile(t *testing.T) {
	// Create a test ReferenceGraph
	g := NewReferenceGraphWithCapacity(100)
	g.SetClassName(1000, "java.lang.Object")
	g.SetObjectInfo(100, 1000, 16)
	g.AddGCRoot(&GCRoot{ObjectID: 100, Type: GCRootStickyClass})
	
	// Create temp file
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.refgraph")
	
	// Serialize to file
	opts := DefaultSerializeOptions()
	stats, err := g.SerializeToFile(filename, opts)
	if err != nil {
		t.Fatalf("SerializeToFile failed: %v", err)
	}
	
	t.Logf("Wrote %d bytes to %s", stats.CompressedSize, filename)
	
	// Verify file exists
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("File not created: %v", err)
	}
	if info.Size() != stats.CompressedSize {
		t.Errorf("File size mismatch: got %d, want %d", info.Size(), stats.CompressedSize)
	}
	
	// Deserialize from file
	g2, err := DeserializeReferenceGraphFromFile(filename)
	if err != nil {
		t.Fatalf("DeserializeFromFile failed: %v", err)
	}
	
	// Verify
	if len(g2.objectClass) != 1 {
		t.Errorf("Object count mismatch: got %d, want 1", len(g2.objectClass))
	}
	if g2.classNames[1000] != "java.lang.Object" {
		t.Errorf("Class name mismatch: got %q, want %q", g2.classNames[1000], "java.lang.Object")
	}
}

func TestSerializeWithoutDominatorData(t *testing.T) {
	g := NewReferenceGraphWithCapacity(100)
	g.SetClassName(1000, "java.lang.Object")
	g.SetObjectInfo(100, 1000, 16)
	g.AddGCRoot(&GCRoot{ObjectID: 100, Type: GCRootStickyClass})
	// Don't compute dominator tree - just test serialization options
	
	// Serialize without dominator data
	opts := DefaultSerializeOptions()
	opts.IncludeDominatorData = false
	
	data, stats, err := g.Serialize(opts)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	
	t.Logf("Size without dominator data: %d bytes", stats.CompressedSize)
	
	// Serialize with dominator data
	opts.IncludeDominatorData = true
	dataWithDom, statsWithDom, err := g.Serialize(opts)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	
	t.Logf("Size with dominator data: %d bytes", statsWithDom.CompressedSize)
	t.Logf("Difference: %d bytes", statsWithDom.CompressedSize-stats.CompressedSize)
	
	// Deserialize without dominator data
	g2, err := DeserializeReferenceGraph(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Since we didn't compute dominator tree, it should not be set
	if g2.dominatorComputed {
		t.Error("Dominator should not be computed")
	}

	// Deserialize with dominator data option (but still no dominator computed)
	g3, err := DeserializeReferenceGraph(dataWithDom)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Still no dominator since we didn't compute it
	if g3.dominatorComputed {
		t.Error("Dominator should not be computed (was never computed)")
	}
}

func TestInvalidData(t *testing.T) {
	// Test empty data
	_, err := DeserializeReferenceGraph([]byte{})
	if err == nil {
		t.Error("Expected error for empty data")
	}
	
	// Test invalid magic
	_, err = DeserializeReferenceGraph([]byte("XXXX01234567890"))
	if err == nil {
		t.Error("Expected error for invalid magic")
	}
	
	// Test invalid version
	data := []byte(MagicBytes)
	data = append(data, 99) // Invalid version
	data = append(data, 0, 0, 0, 0) // String table length
	_, err = DeserializeReferenceGraph(data)
	if err == nil {
		t.Error("Expected error for invalid version")
	}
}

// BenchmarkSerialize benchmarks serialization performance.
func BenchmarkSerialize(b *testing.B) {
	// Create a larger test graph
	g := NewReferenceGraphWithCapacity(10000)
	
	// Use class IDs starting from 1000 to avoid conflicts
	for i := uint64(0); i < 100; i++ {
		g.SetClassName(1000+i, "com.example.Class"+string(rune('A'+i%26)))
	}
	
	for i := uint64(1); i <= 10000; i++ {
		classID := 1000 + (i % 100)
		g.SetObjectInfo(i, classID, int64(i*10))
		if i > 1 {
			g.AddReference(ObjectReference{
				FromObjectID: i - 1,
				ToObjectID:   i,
				FromClassID:  1000 + ((i - 1) % 100),
				FieldName:    "field",
			})
		}
	}
	
	g.AddGCRoot(&GCRoot{ObjectID: 1, Type: GCRootJavaFrame})
	// Don't compute dominator tree - just benchmark serialization
	
	opts := DefaultSerializeOptions()
	opts.IncludeDominatorData = false
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := g.Serialize(opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDeserialize benchmarks deserialization performance.
func BenchmarkDeserialize(b *testing.B) {
	// Create and serialize a test graph
	g := NewReferenceGraphWithCapacity(10000)
	
	for i := uint64(0); i < 100; i++ {
		g.SetClassName(1000+i, "com.example.Class"+string(rune('A'+i%26)))
	}
	
	for i := uint64(1); i <= 10000; i++ {
		classID := 1000 + (i % 100)
		g.SetObjectInfo(i, classID, int64(i*10))
		if i > 1 {
			g.AddReference(ObjectReference{
				FromObjectID: i - 1,
				ToObjectID:   i,
				FromClassID:  1000 + ((i - 1) % 100),
				FieldName:    "field",
			})
		}
	}
	
	g.AddGCRoot(&GCRoot{ObjectID: 1, Type: GCRootJavaFrame})
	
	opts := DefaultSerializeOptions()
	opts.IncludeDominatorData = false
	data, _, err := g.Serialize(opts)
	if err != nil {
		b.Fatal(err)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := DeserializeReferenceGraph(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestCompressionRatio tests and reports the compression ratio for different graph sizes.
func TestCompressionRatio(t *testing.T) {
	sizes := []int{1000, 10000, 100000}
	
	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size_%d", size), func(t *testing.T) {
			g := NewReferenceGraphWithCapacity(size)
			
			// Add realistic class names
			classNames := []string{
				"java.lang.String",
				"java.util.ArrayList",
				"java.util.HashMap$Node",
				"java.util.concurrent.ConcurrentHashMap$Node",
				"byte[]",
				"char[]",
				"int[]",
				"com.example.service.UserService",
				"com.example.model.User",
				"com.example.cache.CacheEntry",
			}
			
			for i, name := range classNames {
				g.SetClassName(uint64(1000+i), name)
			}
			
			// Add objects with varying sizes
			for i := uint64(1); i <= uint64(size); i++ {
				classID := uint64(1000 + int(i)%len(classNames))
				objSize := int64(16 + (i%10)*8) // 16-88 bytes
				g.SetObjectInfo(i, classID, objSize)
				
				// Add references (average 2 refs per object)
				if i > 1 {
					g.AddReference(ObjectReference{
						FromObjectID: i - 1,
						ToObjectID:   i,
						FromClassID:  uint64(1000 + int(i-1)%len(classNames)),
						FieldName:    "next",
					})
				}
				if i > 10 && i%3 == 0 {
					g.AddReference(ObjectReference{
						FromObjectID: i,
						ToObjectID:   i - 10,
						FromClassID:  uint64(1000 + int(i)%len(classNames)),
						FieldName:    "ref",
					})
				}
			}
			
			g.AddGCRoot(&GCRoot{ObjectID: 1, Type: GCRootJavaFrame})
			
			// Serialize
			opts := DefaultSerializeOptions()
			opts.IncludeDominatorData = false
			data, stats, err := g.Serialize(opts)
			if err != nil {
				t.Fatalf("Serialize failed: %v", err)
			}
			
			t.Logf("Graph with %d objects:", size)
			t.Logf("  Objects: %d", stats.Objects)
			t.Logf("  References: %d", stats.References)
			t.Logf("  Classes: %d", stats.Classes)
			t.Logf("  Unique field names: %d", stats.UniqueFieldNames)
			t.Logf("  Raw protobuf size: %d bytes (%.2f KB)", stats.RawSize, float64(stats.RawSize)/1024)
			t.Logf("  Compressed size: %d bytes (%.2f KB)", stats.CompressedSize, float64(stats.CompressedSize)/1024)
			t.Logf("  Compression ratio: %.2fx", stats.CompressionRatio)
			t.Logf("  Bytes per object (compressed): %.2f", float64(stats.CompressedSize)/float64(stats.Objects))
			t.Logf("  Duration: %v", stats.Duration)
			
			// Verify round-trip
			g2, err := DeserializeReferenceGraph(data)
			if err != nil {
				t.Fatalf("Deserialize failed: %v", err)
			}
			
			if len(g2.objectClass) != len(g.objectClass) {
				t.Errorf("Object count mismatch after round-trip: got %d, want %d", len(g2.objectClass), len(g.objectClass))
			}
		})
	}
}
