// Package filter provides unified class name filtering logic for heap analysis.
// This package consolidates filtering rules for JDK, framework, and business classes.
package filter

import (
	"strings"
	"sync"
)

// ClassCategory represents the category of a class.
type ClassCategory int

const (
	// CategoryUnknown indicates the class category is unknown.
	CategoryUnknown ClassCategory = iota
	// CategoryPrimitive indicates primitive types and their arrays.
	CategoryPrimitive
	// CategoryJDK indicates JDK internal classes.
	CategoryJDK
	// CategoryFramework indicates framework internal classes.
	CategoryFramework
	// CategoryApplication indicates application-level classes (including framework beans).
	CategoryApplication
	// CategoryBusiness indicates business/user code classes.
	CategoryBusiness
)

// String returns the string representation of the category.
func (c ClassCategory) String() string {
	switch c {
	case CategoryPrimitive:
		return "primitive"
	case CategoryJDK:
		return "jdk"
	case CategoryFramework:
		return "framework"
	case CategoryApplication:
		return "application"
	case CategoryBusiness:
		return "business"
	default:
		return "unknown"
	}
}

// ClassFilter provides unified class name filtering logic.
// It is safe for concurrent use.
type ClassFilter struct {
	mu sync.RWMutex

	// Primitive arrays
	primitiveArrays map[string]bool

	// JDK class prefixes
	jdkPrefixes []string

	// Framework internal class prefixes (deep internals, not beans)
	frameworkInternalPrefixes []string

	// Top-level filtered classes (for Biggest Objects view)
	topLevelFilteredClasses  map[string]bool
	topLevelFilteredPrefixes []string
	topLevelFilteredSuffixes []string
	topLevelFilteredContains []string

	// Custom business package prefixes
	businessPrefixes []string

	// Cache for frequently queried classes
	categoryCache     map[string]ClassCategory
	categoryCacheSize int
}

// NewClassFilter creates a new ClassFilter with default rules.
func NewClassFilter() *ClassFilter {
	f := &ClassFilter{
		primitiveArrays:   make(map[string]bool),
		categoryCache:     make(map[string]ClassCategory),
		categoryCacheSize: 10000, // Cache up to 10k classes
	}
	f.initDefaults()
	return f
}

// initDefaults initializes default filtering rules.
func (f *ClassFilter) initDefaults() {
	// Primitive arrays
	f.primitiveArrays = map[string]bool{
		"byte[]":    true,
		"char[]":    true,
		"int[]":     true,
		"long[]":    true,
		"short[]":   true,
		"boolean[]": true,
		"float[]":   true,
		"double[]":  true,
	}

	// JDK class prefixes
	f.jdkPrefixes = []string{
		"java.lang.",
		"java.util.",
		"java.io.",
		"java.nio.",
		"java.net.",
		"java.security.",
		"java.math.",
		"java.text.",
		"java.time.",
		"java.sql.",
		"java.reflect.",
		"javax.",
		"sun.",
		"com.sun.",
		"jdk.",
	}

	// Framework internal class prefixes (deep internals only)
	// These are implementation details that are almost never the root cause
	f.frameworkInternalPrefixes = []string{
		// Spring internals (not beans or components)
		"org.springframework.aop.framework.",
		"org.springframework.beans.factory.support.",
		"org.springframework.context.annotation.ConfigurationClassParser",
		"org.springframework.core.annotation.AnnotationUtils",
		"org.springframework.util.ConcurrentReferenceHashMap",
		// Netty buffer pool internals
		"io.netty.buffer.PoolArena",
		"io.netty.buffer.PoolChunk",
		"io.netty.buffer.PoolSubpage",
		"io.netty.buffer.PoolThreadCache",
		"io.netty.util.internal.",
		"io.netty.util.Recycler",
		// Guava internals
		"com.google.common.collect.",
		"com.google.common.cache.",
		// Logging internals
		"org.slf4j.impl.",
		"ch.qos.logback.core.",
		"ch.qos.logback.classic.spi.",
		// Jackson internals
		"com.fasterxml.jackson.core.json.",
		"com.fasterxml.jackson.databind.cfg.",
		"com.fasterxml.jackson.databind.introspect.",
		// ByteBuddy internals
		"net.bytebuddy.description.",
		"net.bytebuddy.pool.",
		"net.bytebuddy.dynamic.",
		// OpenTelemetry agent internals
		"io.opentelemetry.javaagent.tooling.",
		"io.opentelemetry.javaagent.shaded.",
		"io.opentelemetry.javaagent.bootstrap.",
		// Arthas internals
		"com.alibaba.arthas.deps.",
	}

	// Top-level filtered classes (for Biggest Objects view)
	f.topLevelFilteredClasses = map[string]bool{
		// Primitive arrays
		"byte[]":    true,
		"char[]":    true,
		"int[]":     true,
		"long[]":    true,
		"short[]":   true,
		"boolean[]": true,
		"float[]":   true,
		"double[]":  true,
		// Basic wrapper arrays
		"java.lang.Object[]": true,
		"java.lang.String[]": true,
		// JVM internal classes
		"java.lang.Class": true,
		// HashMap/HashSet internal nodes
		"java.util.HashMap$Node":     true,
		"java.util.HashMap$Node[]":   true,
		"java.util.HashMap$TreeNode": true,
		"java.util.HashSet$Node":     true,
		// ConcurrentHashMap internal nodes
		"java.util.concurrent.ConcurrentHashMap$Node":   true,
		"java.util.concurrent.ConcurrentHashMap$Node[]": true,
		// Collection classes (containers, not root causes)
		"java.util.ArrayList":                       true,
		"java.util.LinkedList":                      true,
		"java.util.HashMap":                         true,
		"java.util.LinkedHashMap":                   true,
		"java.util.TreeMap":                         true,
		"java.util.HashSet":                         true,
		"java.util.LinkedHashSet":                   true,
		"java.util.TreeSet":                         true,
		"java.util.concurrent.ConcurrentHashMap":    true,
		"java.util.concurrent.CopyOnWriteArrayList": true,
	}

	f.topLevelFilteredPrefixes = []string{
		"jdk.proxy",
		"com.sun.proxy",
	}

	f.topLevelFilteredSuffixes = []string{
		"Allocator",
		"ByteBufAllocator",
	}

	f.topLevelFilteredContains = []string{
		"$$Lambda",
	}
}

// Classify returns the category of a class.
func (f *ClassFilter) Classify(className string) ClassCategory {
	if className == "" {
		return CategoryUnknown
	}

	// Check cache first
	f.mu.RLock()
	if cat, ok := f.categoryCache[className]; ok {
		f.mu.RUnlock()
		return cat
	}
	f.mu.RUnlock()

	// Compute category
	cat := f.classifyUncached(className)

	// Update cache (with size limit)
	f.mu.Lock()
	if len(f.categoryCache) < f.categoryCacheSize {
		f.categoryCache[className] = cat
	}
	f.mu.Unlock()

	return cat
}

// classifyUncached computes the category without using cache.
func (f *ClassFilter) classifyUncached(className string) ClassCategory {
	// Check primitive arrays
	if f.primitiveArrays[className] {
		return CategoryPrimitive
	}

	// Array types are JDK internal
	if strings.HasSuffix(className, "[]") {
		return CategoryJDK
	}

	// Check JDK prefixes
	for _, prefix := range f.jdkPrefixes {
		if strings.HasPrefix(className, prefix) {
			return CategoryJDK
		}
	}

	// Check framework internal prefixes
	for _, prefix := range f.frameworkInternalPrefixes {
		if strings.HasPrefix(className, prefix) {
			return CategoryFramework
		}
	}

	// Check custom business prefixes
	f.mu.RLock()
	businessPrefixes := f.businessPrefixes
	f.mu.RUnlock()

	for _, prefix := range businessPrefixes {
		if strings.HasPrefix(className, prefix) {
			return CategoryBusiness
		}
	}

	// Default to application level (includes framework beans, consumers, etc.)
	return CategoryApplication
}

// IsPrimitive returns true if the class is a primitive type or primitive array.
func (f *ClassFilter) IsPrimitive(className string) bool {
	return f.Classify(className) == CategoryPrimitive
}

// IsJDK returns true if the class is a JDK internal class.
func (f *ClassFilter) IsJDK(className string) bool {
	return f.Classify(className) == CategoryJDK
}

// IsJDKInternal is an alias for IsJDK for backward compatibility.
func (f *ClassFilter) IsJDKInternal(className string) bool {
	return f.IsJDK(className)
}

// IsFramework returns true if the class is a framework internal class.
func (f *ClassFilter) IsFramework(className string) bool {
	return f.Classify(className) == CategoryFramework
}

// IsFrameworkInternal is an alias for IsFramework for backward compatibility.
func (f *ClassFilter) IsFrameworkInternal(className string) bool {
	return f.IsFramework(className)
}

// IsApplication returns true if the class is an application-level class.
// This includes framework beans, consumers, etc.
func (f *ClassFilter) IsApplication(className string) bool {
	cat := f.Classify(className)
	return cat == CategoryApplication || cat == CategoryBusiness
}

// IsBusiness returns true if the class is likely a business/user code class.
// This is true for both CategoryApplication and CategoryBusiness.
func (f *ClassFilter) IsBusiness(className string) bool {
	cat := f.Classify(className)
	return cat == CategoryApplication || cat == CategoryBusiness
}

// IsApplicationLevel returns true if the class is application-level (not JDK or framework internal).
// This is useful for root cause analysis.
func (f *ClassFilter) IsApplicationLevel(className string) bool {
	cat := f.Classify(className)
	return cat != CategoryJDK && cat != CategoryFramework && cat != CategoryPrimitive
}

// ShouldFilterTopLevel returns true if the class should be filtered from top-level views.
// This is used for Biggest Objects view to filter out container classes, proxies, etc.
func (f *ClassFilter) ShouldFilterTopLevel(className string) bool {
	// Direct match
	if f.topLevelFilteredClasses[className] {
		return true
	}

	// Check prefixes
	for _, prefix := range f.topLevelFilteredPrefixes {
		if strings.HasPrefix(className, prefix) {
			return true
		}
	}

	// Check suffixes
	for _, suffix := range f.topLevelFilteredSuffixes {
		if strings.HasSuffix(className, suffix) {
			return true
		}
	}

	// Check contains
	for _, substr := range f.topLevelFilteredContains {
		if strings.Contains(className, substr) {
			return true
		}
	}

	return false
}

// AddBusinessPrefix adds a custom business package prefix.
// Classes with this prefix will be classified as CategoryBusiness.
func (f *ClassFilter) AddBusinessPrefix(prefix string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Check for duplicates
	for _, p := range f.businessPrefixes {
		if p == prefix {
			return
		}
	}

	f.businessPrefixes = append(f.businessPrefixes, prefix)

	// Clear cache since classification may change
	f.categoryCache = make(map[string]ClassCategory)
}

// AddBusinessPrefixes adds multiple custom business package prefixes.
func (f *ClassFilter) AddBusinessPrefixes(prefixes []string) {
	for _, prefix := range prefixes {
		f.AddBusinessPrefix(prefix)
	}
}

// GetBusinessPrefixes returns the current list of business prefixes.
func (f *ClassFilter) GetBusinessPrefixes() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make([]string, len(f.businessPrefixes))
	copy(result, f.businessPrefixes)
	return result
}

// AddJDKPrefix adds a custom JDK prefix.
func (f *ClassFilter) AddJDKPrefix(prefix string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.jdkPrefixes = append(f.jdkPrefixes, prefix)
	f.categoryCache = make(map[string]ClassCategory)
}

// AddFrameworkInternalPrefix adds a custom framework internal prefix.
func (f *ClassFilter) AddFrameworkInternalPrefix(prefix string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.frameworkInternalPrefixes = append(f.frameworkInternalPrefixes, prefix)
	f.categoryCache = make(map[string]ClassCategory)
}

// AddTopLevelFilteredClass adds a class to the top-level filter list.
func (f *ClassFilter) AddTopLevelFilteredClass(className string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.topLevelFilteredClasses[className] = true
}

// AddTopLevelFilteredPrefix adds a prefix to the top-level filter list.
func (f *ClassFilter) AddTopLevelFilteredPrefix(prefix string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.topLevelFilteredPrefixes = append(f.topLevelFilteredPrefixes, prefix)
}

// ClearCache clears the classification cache.
func (f *ClassFilter) ClearCache() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.categoryCache = make(map[string]ClassCategory)
}

// CacheStats returns cache statistics.
func (f *ClassFilter) CacheStats() (size int, maxSize int) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return len(f.categoryCache), f.categoryCacheSize
}

// SetCacheSize sets the maximum cache size.
func (f *ClassFilter) SetCacheSize(size int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.categoryCacheSize = size
	if len(f.categoryCache) > size {
		f.categoryCache = make(map[string]ClassCategory)
	}
}

// DefaultFilter is the default global filter instance.
var DefaultFilter = NewClassFilter()

// Classify classifies a class using the default filter.
func Classify(className string) ClassCategory {
	return DefaultFilter.Classify(className)
}

// IsPrimitive checks if a class is primitive using the default filter.
func IsPrimitive(className string) bool {
	return DefaultFilter.IsPrimitive(className)
}

// IsJDK checks if a class is JDK internal using the default filter.
func IsJDK(className string) bool {
	return DefaultFilter.IsJDK(className)
}

// IsJDKInternal is an alias for IsJDK.
func IsJDKInternal(className string) bool {
	return DefaultFilter.IsJDKInternal(className)
}

// IsFramework checks if a class is framework internal using the default filter.
func IsFramework(className string) bool {
	return DefaultFilter.IsFramework(className)
}

// IsFrameworkInternal is an alias for IsFramework.
func IsFrameworkInternal(className string) bool {
	return DefaultFilter.IsFrameworkInternal(className)
}

// IsApplication checks if a class is application-level using the default filter.
func IsApplication(className string) bool {
	return DefaultFilter.IsApplication(className)
}

// IsBusiness checks if a class is business code using the default filter.
func IsBusiness(className string) bool {
	return DefaultFilter.IsBusiness(className)
}

// IsApplicationLevel checks if a class is application-level using the default filter.
func IsApplicationLevel(className string) bool {
	return DefaultFilter.IsApplicationLevel(className)
}

// ShouldFilterTopLevel checks if a class should be filtered from top-level views.
func ShouldFilterTopLevel(className string) bool {
	return DefaultFilter.ShouldFilterTopLevel(className)
}

// AddBusinessPrefix adds a business prefix to the default filter.
func AddBusinessPrefix(prefix string) {
	DefaultFilter.AddBusinessPrefix(prefix)
}
