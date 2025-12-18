package filter

import (
	"sync"
	"testing"
)

func TestClassFilter_Classify(t *testing.T) {
	f := NewClassFilter()

	tests := []struct {
		className string
		expected  ClassCategory
	}{
		// Primitive arrays
		{"byte[]", CategoryPrimitive},
		{"char[]", CategoryPrimitive},
		{"int[]", CategoryPrimitive},
		{"long[]", CategoryPrimitive},

		// JDK classes
		{"java.lang.String", CategoryJDK},
		{"java.util.HashMap", CategoryJDK},
		{"java.io.File", CategoryJDK},
		{"java.nio.ByteBuffer", CategoryJDK},
		{"javax.servlet.Servlet", CategoryJDK},
		{"sun.misc.Unsafe", CategoryJDK},
		{"com.sun.proxy.$Proxy0", CategoryJDK},
		{"jdk.internal.misc.Unsafe", CategoryJDK},

		// Array types (JDK)
		{"java.lang.String[]", CategoryJDK},
		{"java.lang.Object[]", CategoryJDK},

		// Framework internals
		{"org.springframework.aop.framework.ProxyFactory", CategoryFramework},
		{"io.netty.buffer.PoolArena", CategoryFramework},
		{"io.netty.util.internal.PlatformDependent", CategoryFramework},
		{"com.google.common.collect.ImmutableList", CategoryFramework},
		{"ch.qos.logback.core.Appender", CategoryFramework},
		{"net.bytebuddy.description.type.TypeDescription", CategoryFramework},

		// Application-level classes (framework beans, consumers, etc.)
		{"org.springframework.web.servlet.DispatcherServlet", CategoryApplication},
		{"org.apache.kafka.clients.consumer.KafkaConsumer", CategoryApplication},
		{"io.netty.channel.ChannelHandler", CategoryApplication},
		{"com.fasterxml.jackson.databind.ObjectMapper", CategoryApplication},

		// Business classes
		{"com.example.MyService", CategoryApplication},
		{"com.mycompany.app.UserController", CategoryApplication},
		{"org.myorg.service.OrderService", CategoryApplication},
	}

	for _, tt := range tests {
		t.Run(tt.className, func(t *testing.T) {
			got := f.Classify(tt.className)
			if got != tt.expected {
				t.Errorf("Classify(%q) = %v, want %v", tt.className, got, tt.expected)
			}
		})
	}
}

func TestClassFilter_IsBusiness(t *testing.T) {
	f := NewClassFilter()

	tests := []struct {
		className string
		expected  bool
	}{
		// Not business
		{"byte[]", false},
		{"java.lang.String", false},
		{"java.util.HashMap", false},
		{"io.netty.util.internal.PlatformDependent", false},
		{"org.springframework.aop.framework.ProxyFactory", false},

		// Business (application-level)
		{"com.example.MyService", true},
		{"org.springframework.web.servlet.DispatcherServlet", true},
		{"org.apache.kafka.clients.consumer.KafkaConsumer", true},
	}

	for _, tt := range tests {
		t.Run(tt.className, func(t *testing.T) {
			got := f.IsBusiness(tt.className)
			if got != tt.expected {
				t.Errorf("IsBusiness(%q) = %v, want %v", tt.className, got, tt.expected)
			}
		})
	}
}

func TestClassFilter_ShouldFilterTopLevel(t *testing.T) {
	f := NewClassFilter()

	tests := []struct {
		className string
		expected  bool
	}{
		// Should filter
		{"byte[]", true},
		{"char[]", true},
		{"java.lang.Object[]", true},
		{"java.lang.Class", true},
		{"java.util.HashMap$Node", true},
		{"java.util.HashMap", true},
		{"java.util.ArrayList", true},
		{"jdk.proxy.$Proxy0", true},
		{"com.sun.proxy.$Proxy1", true},
		{"io.netty.buffer.PooledByteBufAllocator", true},
		{"com.example.MyClass$$Lambda$123", true},

		// Should not filter
		{"com.example.MyService", false},
		{"org.springframework.web.servlet.DispatcherServlet", false},
		{"java.lang.String", false},
	}

	for _, tt := range tests {
		t.Run(tt.className, func(t *testing.T) {
			got := f.ShouldFilterTopLevel(tt.className)
			if got != tt.expected {
				t.Errorf("ShouldFilterTopLevel(%q) = %v, want %v", tt.className, got, tt.expected)
			}
		})
	}
}

func TestClassFilter_AddBusinessPrefix(t *testing.T) {
	f := NewClassFilter()

	// Before adding prefix
	if f.Classify("com.mycompany.MyClass") != CategoryApplication {
		t.Error("Expected CategoryApplication before adding prefix")
	}

	// Add business prefix
	f.AddBusinessPrefix("com.mycompany.")

	// After adding prefix
	if f.Classify("com.mycompany.MyClass") != CategoryBusiness {
		t.Error("Expected CategoryBusiness after adding prefix")
	}

	// Other classes should not be affected
	if f.Classify("com.example.MyClass") != CategoryApplication {
		t.Error("Expected CategoryApplication for non-matching prefix")
	}
}

func TestClassFilter_ConcurrentAccess(t *testing.T) {
	f := NewClassFilter()

	var wg sync.WaitGroup
	numGoroutines := 100
	numIterations := 1000

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				// Mix of reads and writes
				if j%10 == 0 {
					f.AddBusinessPrefix("com.test" + string(rune('0'+id%10)) + ".")
				}
				f.Classify("java.lang.String")
				f.Classify("com.example.MyClass")
				f.IsBusiness("com.test.Service")
				f.ShouldFilterTopLevel("byte[]")
			}
		}(i)
	}

	wg.Wait()
}

func TestClassFilter_Cache(t *testing.T) {
	f := NewClassFilter()

	// First call - not cached
	cat1 := f.Classify("com.example.MyService")

	// Second call - should be cached
	cat2 := f.Classify("com.example.MyService")

	if cat1 != cat2 {
		t.Errorf("Cached result differs: %v vs %v", cat1, cat2)
	}

	// Check cache stats
	size, maxSize := f.CacheStats()
	if size != 1 {
		t.Errorf("Expected cache size 1, got %d", size)
	}
	if maxSize != 10000 {
		t.Errorf("Expected max cache size 10000, got %d", maxSize)
	}

	// Clear cache
	f.ClearCache()
	size, _ = f.CacheStats()
	if size != 0 {
		t.Errorf("Expected cache size 0 after clear, got %d", size)
	}
}

func TestDefaultFilter(t *testing.T) {
	// Test global functions
	if !IsJDK("java.lang.String") {
		t.Error("Expected IsJDK to return true for java.lang.String")
	}

	if !IsPrimitive("byte[]") {
		t.Error("Expected IsPrimitive to return true for byte[]")
	}

	if !IsBusiness("com.example.MyService") {
		t.Error("Expected IsBusiness to return true for com.example.MyService")
	}

	if !ShouldFilterTopLevel("java.util.HashMap") {
		t.Error("Expected ShouldFilterTopLevel to return true for java.util.HashMap")
	}
}

func TestClassCategory_String(t *testing.T) {
	tests := []struct {
		cat      ClassCategory
		expected string
	}{
		{CategoryUnknown, "unknown"},
		{CategoryPrimitive, "primitive"},
		{CategoryJDK, "jdk"},
		{CategoryFramework, "framework"},
		{CategoryApplication, "application"},
		{CategoryBusiness, "business"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.cat.String(); got != tt.expected {
				t.Errorf("ClassCategory.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func BenchmarkClassFilter_Classify(b *testing.B) {
	f := NewClassFilter()
	classNames := []string{
		"java.lang.String",
		"com.example.MyService",
		"byte[]",
		"org.springframework.web.servlet.DispatcherServlet",
		"io.netty.buffer.PoolArena",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, cn := range classNames {
			f.Classify(cn)
		}
	}
}

func BenchmarkClassFilter_Classify_Cached(b *testing.B) {
	f := NewClassFilter()
	className := "com.example.MyService"

	// Pre-populate cache
	f.Classify(className)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Classify(className)
	}
}

func BenchmarkClassFilter_ShouldFilterTopLevel(b *testing.B) {
	f := NewClassFilter()
	classNames := []string{
		"byte[]",
		"java.util.HashMap",
		"com.example.MyService",
		"jdk.proxy.$Proxy0",
		"com.example.MyClass$$Lambda$123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, cn := range classNames {
			f.ShouldFilterTopLevel(cn)
		}
	}
}
