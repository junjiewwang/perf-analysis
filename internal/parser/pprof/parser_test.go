package pprof

import (
	"bytes"
	"testing"

	"github.com/google/pprof/profile"
)

// createTestPprofData creates a valid pprof binary data for testing via Parse().
func createTestPprofData(sampleTypes []string) []byte {
	prof := &profile.Profile{
		SampleType: make([]*profile.ValueType, len(sampleTypes)),
	}
	for i, st := range sampleTypes {
		prof.SampleType[i] = &profile.ValueType{
			Type: st,
			Unit: "count",
		}
	}

	var buf bytes.Buffer
	_ = prof.Write(&buf)
	return buf.Bytes()
}

func TestParser_Parse_InvalidData(t *testing.T) {
	p := NewParser()
	err := p.Parse(bytes.NewReader([]byte("invalid pprof data")))
	if err == nil {
		t.Error("Parse() with invalid data should return error")
	}
}

func TestParser_Parse_ValidData(t *testing.T) {
	data := createTestPprofData([]string{"cpu", "samples"})
	p := NewParser()
	err := p.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Parse() with valid data should not return error: %v", err)
	}

	types := p.GetSampleTypes()
	if len(types) != 2 {
		t.Errorf("GetSampleTypes() returned %d types, want 2", len(types))
	}
	if types[0] != "cpu" {
		t.Errorf("GetSampleTypes()[0] = %s, want cpu", types[0])
	}
	if types[1] != "samples" {
		t.Errorf("GetSampleTypes()[1] = %s, want samples", types[1])
	}
}

func TestParser_GetSampleTypes_Nil(t *testing.T) {
	p := NewParser()
	types := p.GetSampleTypes()
	if types != nil {
		t.Errorf("GetSampleTypes() with nil profile should return nil")
	}
}

func TestParser_DetectProfileType(t *testing.T) {
	tests := []struct {
		name        string
		sampleTypes []string
		want        string
	}{
		{"cpu profile", []string{"cpu", "samples"}, "cpu"},
		{"heap profile", []string{"inuse_space", "inuse_objects"}, "heap"},
		{"goroutine profile", []string{"goroutine", "count"}, "goroutine"},
		{"block profile", []string{"contentions", "delay"}, "block"},
		{"unknown profile", []string{"unknown"}, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := createTestPprofData(tt.sampleTypes)
			p := NewParser()
			if err := p.Parse(bytes.NewReader(data)); err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			got := p.DetectProfileType()
			if got != tt.want {
				t.Errorf("DetectProfileType() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParser_DetectProfileType_Nil(t *testing.T) {
	p := NewParser()
	got := p.DetectProfileType()
	if got != "unknown" {
		t.Errorf("DetectProfileType() with nil profile = %s, want unknown", got)
	}
}

func TestParser_GetDuration(t *testing.T) {
	// Create profile with duration
	prof := &profile.Profile{
		SampleType:    []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}},
		DurationNanos: 1000000000, // 1 second
	}
	var buf bytes.Buffer
	_ = prof.Write(&buf)

	p := NewParser()
	if err := p.Parse(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	duration := p.GetDuration()
	if duration != 1000000000 {
		t.Errorf("GetDuration() = %d, want 1000000000", duration)
	}
}

func TestParser_GetDuration_Nil(t *testing.T) {
	p := NewParser()
	duration := p.GetDuration()
	if duration != 0 {
		t.Errorf("GetDuration() with nil profile = %d, want 0", duration)
	}
}

func TestParser_GetUnit(t *testing.T) {
	prof := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "cpu", Unit: "nanoseconds"},
			{Type: "samples", Unit: "count"},
		},
	}
	var buf bytes.Buffer
	_ = prof.Write(&buf)

	p := NewParser()
	if err := p.Parse(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	tests := []struct {
		sampleType SampleType
		want       string
	}{
		{SampleTypeCPU, "nanoseconds"},
		{SampleTypeSamples, "count"},
	}

	for _, tt := range tests {
		t.Run(string(tt.sampleType), func(t *testing.T) {
			got := p.GetUnit(tt.sampleType)
			if got != tt.want {
				t.Errorf("GetUnit(%s) = %s, want %s", tt.sampleType, got, tt.want)
			}
		})
	}
}

func TestParser_ToCollapsed_NilProfile(t *testing.T) {
	p := NewParser()
	_, err := p.ToCollapsed(SampleTypeCPU)
	if err == nil {
		t.Error("ToCollapsed() with nil profile should return error")
	}
}

func TestParser_ToSamples_NilProfile(t *testing.T) {
	p := NewParser()
	_, err := p.ToSamples(SampleTypeCPU)
	if err == nil {
		t.Error("ToSamples() with nil profile should return error")
	}
}

func TestParser_GetTotalSamples_NilProfile(t *testing.T) {
	p := NewParser()
	total := p.GetTotalSamples(SampleTypeCPU)
	if total != 0 {
		t.Errorf("GetTotalSamples() with nil profile = %d, want 0", total)
	}
}

func TestParser_GetTopFunctions_NilProfile(t *testing.T) {
	p := NewParser()
	funcs := p.GetTopFunctions(10, SampleTypeCPU, false)
	if funcs != nil {
		t.Error("GetTopFunctions() with nil profile should return nil")
	}
}

func TestParser_Profile_NilBeforeParse(t *testing.T) {
	p := NewParser()
	if p.Profile() != nil {
		t.Error("Profile() before Parse() should return nil")
	}
}

func TestParser_Profile_AfterParse(t *testing.T) {
	data := createTestPprofData([]string{"cpu", "samples"})
	p := NewParser()
	if err := p.Parse(bytes.NewReader(data)); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if p.Profile() == nil {
		t.Error("Profile() after Parse() should not return nil")
	}
}
