package writer

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type testData struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestJSONWriter_Write(t *testing.T) {
	data := testData{Name: "test", Value: 42}

	t.Run("compact output", func(t *testing.T) {
		w := NewJSONWriter[testData]()
		var buf bytes.Buffer
		err := w.Write(data, &buf)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		expected := `{"name":"test","value":42}` + "\n"
		if buf.String() != expected {
			t.Errorf("got %q, want %q", buf.String(), expected)
		}
	})

	t.Run("pretty output", func(t *testing.T) {
		w := NewPrettyJSONWriter[testData]()
		var buf bytes.Buffer
		err := w.Write(data, &buf)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Verify it's valid JSON and indented
		var decoded testData
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Fatalf("Failed to decode output: %v", err)
		}
		if decoded != data {
			t.Errorf("decoded data mismatch: got %+v, want %+v", decoded, data)
		}
	})
}

func TestJSONWriter_WriteToFile(t *testing.T) {
	data := testData{Name: "test", Value: 42}
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.json")

	w := NewJSONWriter[testData]()
	err := w.WriteToFile(data, filePath)
	if err != nil {
		t.Fatalf("WriteToFile failed: %v", err)
	}

	// Read and verify
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	var decoded testData
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("Failed to decode file: %v", err)
	}
	if decoded != data {
		t.Errorf("decoded data mismatch: got %+v, want %+v", decoded, data)
	}
}

func TestGzipWriter_Write(t *testing.T) {
	data := testData{Name: "test", Value: 42}

	w := NewGzipWriter[testData]()
	var buf bytes.Buffer
	err := w.Write(data, &buf)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Decompress and verify
	gzReader, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzReader.Close()

	decompressed, err := io.ReadAll(gzReader)
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	var decoded testData
	if err := json.Unmarshal(decompressed, &decoded); err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}
	if decoded != data {
		t.Errorf("decoded data mismatch: got %+v, want %+v", decoded, data)
	}
}

func TestGzipWriter_WriteToFile(t *testing.T) {
	data := testData{Name: "test", Value: 42}
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.json.gz")

	w := NewGzipWriter[testData]()
	err := w.WriteToFile(data, filePath)
	if err != nil {
		t.Fatalf("WriteToFile failed: %v", err)
	}

	// Read and decompress
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzReader.Close()

	decompressed, err := io.ReadAll(gzReader)
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	var decoded testData
	if err := json.Unmarshal(decompressed, &decoded); err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}
	if decoded != data {
		t.Errorf("decoded data mismatch: got %+v, want %+v", decoded, data)
	}
}

func TestGzipWriter_WriteToFileWithStats(t *testing.T) {
	data := testData{Name: "test", Value: 42}
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.json.gz")

	w := NewGzipWriter[testData]()
	result, err := w.WriteToFileWithStats(data, filePath)
	if err != nil {
		t.Fatalf("WriteToFileWithStats failed: %v", err)
	}

	if result.JSONSize <= 0 {
		t.Errorf("JSONSize should be positive, got %d", result.JSONSize)
	}
	if result.CompressedSize <= 0 {
		t.Errorf("CompressedSize should be positive, got %d", result.CompressedSize)
	}
	if result.CompressionPct <= 0 {
		t.Errorf("CompressionPct should be positive, got %f", result.CompressionPct)
	}
}

func TestGzipWriterWithLevel(t *testing.T) {
	w := NewGzipWriterWithLevel[testData](gzip.BestSpeed)
	if w.CompressionLevel != gzip.BestSpeed {
		t.Errorf("CompressionLevel = %d, want %d", w.CompressionLevel, gzip.BestSpeed)
	}
}
