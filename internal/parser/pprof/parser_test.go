package pprof

import (
	"bytes"
	"testing"

	"github.com/google/pprof/profile"
)

// createTestProfile creates a test pprof profile for testing.
func createTestProfile(sampleTypes []string, samples [][]int64, locations []*profile.Location) *profile.Profile {
	prof := &profile.Profile{
		SampleType: make([]*profile.ValueType, len(sampleTypes)),
		Sample:     make([]*profile.Sample, len(samples)),
		Location:   locations,
	}

	for i, st := range sampleTypes {
		prof.SampleType[i] = &profile.ValueType{
			Type: st,
			Unit: "count",
		}
	}

	for i, values := range samples {
		prof.Sample[i] = &profile.Sample{
			Value:    values,
			Location: locations,
		}
	}

	return prof
}

func TestParser_GetSampleTypes(t *testing.T) {
	p := &Parser{
		profile: createTestProfile([]string{"cpu", "samples"}, nil, nil),
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
	p := &Parser{}
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
			p := &Parser{
				profile: createTestProfile(tt.sampleTypes, nil, nil),
			}
			got := p.DetectProfileType()
			if got != tt.want {
				t.Errorf("DetectProfileType() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParser_DetectProfileType_Nil(t *testing.T) {
	p := &Parser{}
	got := p.DetectProfileType()
	if got != "unknown" {
		t.Errorf("DetectProfileType() with nil profile = %s, want unknown", got)
	}
}

func TestParser_GetDuration(t *testing.T) {
	p := &Parser{
		profile: &profile.Profile{
			DurationNanos: 1000000000, // 1 second
		},
	}

	duration := p.GetDuration()
	if duration != 1000000000 {
		t.Errorf("GetDuration() = %d, want 1000000000", duration)
	}
}

func TestParser_GetDuration_Nil(t *testing.T) {
	p := &Parser{}
	duration := p.GetDuration()
	if duration != 0 {
		t.Errorf("GetDuration() with nil profile = %d, want 0", duration)
	}
}

func TestParser_findSampleTypeIndex(t *testing.T) {
	p := &Parser{
		profile: createTestProfile([]string{"cpu", "samples", "inuse_space"}, nil, nil),
	}

	tests := []struct {
		typeName string
		want     int
	}{
		{"cpu", 0},
		{"samples", 1},
		{"inuse_space", 2},
		{"nonexistent", -1},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			got := p.findSampleTypeIndex(tt.typeName)
			if got != tt.want {
				t.Errorf("findSampleTypeIndex(%s) = %d, want %d", tt.typeName, got, tt.want)
			}
		})
	}
}

func TestParser_findAlternativeSampleTypeIndex(t *testing.T) {
	p := &Parser{
		profile: createTestProfile([]string{"nanoseconds", "count"}, nil, nil),
	}

	tests := []struct {
		sampleType SampleType
		want       int
	}{
		{SampleTypeCPU, 0},         // nanoseconds is alternative for cpu
		{SampleTypeSamples, 1},     // count is alternative for samples
		{SampleTypeInuseSpace, -1}, // not found
	}

	for _, tt := range tests {
		t.Run(string(tt.sampleType), func(t *testing.T) {
			got := p.findAlternativeSampleTypeIndex(tt.sampleType)
			if got != tt.want {
				t.Errorf("findAlternativeSampleTypeIndex(%s) = %d, want %d", tt.sampleType, got, tt.want)
			}
		})
	}
}

func TestGetModuleName(t *testing.T) {
	tests := []struct {
		funcName string
		want     string
	}{
		{"github.com/pkg/errors.Wrap", "github.com/pkg/errors"},
		{"runtime.main", "runtime"},
		{"main.main", "main"},
		{"net/http.(*Server).Serve", "net/http"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.funcName, func(t *testing.T) {
			got := getModuleName(tt.funcName)
			if got != tt.want {
				t.Errorf("getModuleName(%s) = %s, want %s", tt.funcName, got, tt.want)
			}
		})
	}
}

func TestParser_GetUnit(t *testing.T) {
	prof := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "cpu", Unit: "nanoseconds"},
			{Type: "samples", Unit: "count"},
		},
	}
	p := &Parser{profile: prof}

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

func TestParser_Parse_InvalidData(t *testing.T) {
	p := NewParser()
	err := p.Parse(bytes.NewReader([]byte("invalid pprof data")))
	if err == nil {
		t.Error("Parse() with invalid data should return error")
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

func TestParser_containsSampleType(t *testing.T) {
	p := &Parser{
		profile: createTestProfile([]string{"cpu", "samples"}, nil, nil),
	}

	if !p.containsSampleType("cpu") {
		t.Error("containsSampleType(cpu) should return true")
	}
	if !p.containsSampleType("samples") {
		t.Error("containsSampleType(samples) should return true")
	}
	if p.containsSampleType("nonexistent") {
		t.Error("containsSampleType(nonexistent) should return false")
	}
}
