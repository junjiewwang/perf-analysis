package profiling

import (
	"reflect"
	"testing"
)

func TestExtractThreadGroup(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple with number", "worker-1", "worker"},
		{"simple with underscore", "thread_1", "thread"},
		{"simple with hash", "pool#1", "pool"},
		{"multiple numbers", "grpc-nio-worker-123", "grpc-nio-worker"},
		{"no trailing number", "main", "main"},
		{"only numbers", "123", "123"},
		{"empty string", "", ""},
		{"complex name", "C2 CompilerThre-26", "C2 CompilerThre"},
		{"with tid format", "XWorker#0 tid=12", "XWorker#0 tid="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractThreadGroup(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractThreadGroup(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsSwapperThread(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"exact swapper", "swapper", true},
		{"swapper with cpu", "swapper/0", true},
		{"bracketed swapper", "[swapper]", true},
		{"bracketed swapper with cpu", "[swapper/0]", true},
		{"not swapper", "worker-1", false},
		{"contains swapper", "myswapper", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSwapperThread(tt.input)
			if result != tt.expected {
				t.Errorf("IsSwapperThread(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSplitFuncAndModule(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedFunc   string
		expectedModule string
	}{
		{"with module", "foo(bar)", "foo", "bar"},
		{"no module", "foo", "foo", ""},
		{"nested parens", "foo(bar(baz))", "foo(bar", "baz)"},
		{"empty module", "foo()", "foo", ""},
		{"complex function", "java/lang/Thread.run(libjvm.so)", "java/lang/Thread.run", "libjvm.so"},
		{"no closing paren", "foo(bar", "foo(bar", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, mod := SplitFuncAndModule(tt.input)
			if fn != tt.expectedFunc || mod != tt.expectedModule {
				t.Errorf("SplitFuncAndModule(%q) = (%q, %q), want (%q, %q)",
					tt.input, fn, mod, tt.expectedFunc, tt.expectedModule)
			}
		})
	}
}

func TestStackToString(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"normal stack", []string{"a", "b", "c"}, "a;b;c"},
		{"single frame", []string{"a"}, "a"},
		{"empty stack", []string{}, ""},
		{"nil stack", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StackToString(tt.input)
			if result != tt.expected {
				t.Errorf("StackToString(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStringToStack(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"normal string", "a;b;c", []string{"a", "b", "c"}},
		{"single frame", "a", []string{"a"}},
		{"empty string", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringToStack(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("StringToStack(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
