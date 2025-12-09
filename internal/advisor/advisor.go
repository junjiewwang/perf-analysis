// Package advisor provides analysis suggestions based on profiling results.
package advisor

import (
	"strconv"
	"strings"

	"github.com/perf-analysis/internal/statistics"
	"github.com/perf-analysis/pkg/model"
)

// Advisor generates analysis suggestions based on profiling data.
type Advisor struct {
	rules []Rule
}

// Rule represents a suggestion rule.
type Rule struct {
	Type        string
	Name        string
	Description string
	Threshold   float64
	Check       RuleCheckFunc
}

// RuleCheckFunc is a function that checks if a rule applies.
type RuleCheckFunc func(ctx *RuleContext) []model.Suggestion

// RuleContext provides context for rule checking.
type RuleContext struct {
	TaskType       model.TaskType
	ProfilerType   model.ProfilerType
	ParseResult    *model.ParseResult
	TopFuncsResult *statistics.TopFuncsResult
	RequestParams  *model.RequestParams
}

// NewAdvisor creates a new Advisor with default rules.
func NewAdvisor() *Advisor {
	return &Advisor{
		rules: defaultRules(),
	}
}

// NewAdvisorWithRules creates a new Advisor with custom rules.
func NewAdvisorWithRules(rules []Rule) *Advisor {
	return &Advisor{
		rules: rules,
	}
}

// Advise generates suggestions based on the analysis context.
func (a *Advisor) Advise(ctx *RuleContext) []model.Suggestion {
	suggestions := make([]model.Suggestion, 0)

	for _, rule := range a.rules {
		if rule.Check != nil {
			ruleSuggestions := rule.Check(ctx)
			suggestions = append(suggestions, ruleSuggestions...)
		}
	}

	return suggestions
}

// defaultRules returns the default set of analysis rules.
func defaultRules() []Rule {
	return []Rule{
		{
			Type:        "cpu",
			Name:        "high_cpu_function",
			Description: "Check for functions with high CPU usage",
			Threshold:   10.0,
			Check:       checkHighCPUFunction,
		},
		{
			Type:        "cpu",
			Name:        "gc_overhead",
			Description: "Check for high GC overhead",
			Threshold:   5.0,
			Check:       checkGCOverhead,
		},
		{
			Type:        "cpu",
			Name:        "lock_contention",
			Description: "Check for lock contention",
			Threshold:   3.0,
			Check:       checkLockContention,
		},
		{
			Type:        "memory",
			Name:        "frequent_allocation",
			Description: "Check for frequent memory allocation",
			Threshold:   10.0,
			Check:       checkFrequentAllocation,
		},
		{
			Type:        "common",
			Name:        "reflection_usage",
			Description: "Check for reflection usage",
			Threshold:   2.0,
			Check:       checkReflectionUsage,
		},
	}
}

// checkHighCPUFunction checks for functions with high CPU usage.
func checkHighCPUFunction(ctx *RuleContext) []model.Suggestion {
	suggestions := make([]model.Suggestion, 0)

	if ctx.TopFuncsResult == nil {
		return suggestions
	}

	for _, tf := range ctx.TopFuncsResult.TopFuncs {
		if tf.SelfPercent > 15.0 {
			suggestions = append(suggestions, model.Suggestion{
				Type:       "cpu_hotspot",
				Severity:   "warning",
				Suggestion: "函数 " + tf.Name + " CPU占用率较高(" + formatPercent(tf.SelfPercent) + "%)，建议进行性能优化",
				FuncName:   tf.Name,
			})
		}
	}

	return suggestions
}

// checkGCOverhead checks for high GC overhead.
func checkGCOverhead(ctx *RuleContext) []model.Suggestion {
	suggestions := make([]model.Suggestion, 0)

	if ctx.TopFuncsResult == nil {
		return suggestions
	}

	gcKeywords := []string{
		"GC", "gc", "G1", "CMS", "ParNew", "ParallelGC",
		"SafepointSynchronize", "safepoint",
	}

	var totalGCPercent float64
	for _, tf := range ctx.TopFuncsResult.TopFuncs {
		for _, keyword := range gcKeywords {
			if strings.Contains(tf.Name, keyword) {
				totalGCPercent += tf.SelfPercent
				break
			}
		}
	}

	if totalGCPercent > 5.0 {
		suggestions = append(suggestions, model.Suggestion{
			Type:       "gc_overhead",
			Severity:   "warning",
			Suggestion: "GC相关函数CPU占用率较高(" + formatPercent(totalGCPercent) + "%)，建议检查GC配置和内存使用情况",
			FuncName:   "",
		})
	}

	return suggestions
}

// checkLockContention checks for lock contention issues.
func checkLockContention(ctx *RuleContext) []model.Suggestion {
	suggestions := make([]model.Suggestion, 0)

	if ctx.TopFuncsResult == nil {
		return suggestions
	}

	lockKeywords := []string{
		"Monitor", "monitor", "Lock", "lock", "synchronized",
		"pthread_mutex", "futex", "spin", "Spin",
	}

	var totalLockPercent float64
	lockFunctions := make([]string, 0)

	for _, tf := range ctx.TopFuncsResult.TopFuncs {
		for _, keyword := range lockKeywords {
			if strings.Contains(tf.Name, keyword) && tf.SelfPercent > 1.0 {
				totalLockPercent += tf.SelfPercent
				lockFunctions = append(lockFunctions, tf.Name)
				break
			}
		}
	}

	if totalLockPercent > 3.0 {
		suggestions = append(suggestions, model.Suggestion{
			Type:       "lock_contention",
			Severity:   "warning",
			Suggestion: "检测到锁竞争相关函数CPU占用率较高(" + formatPercent(totalLockPercent) + "%)，建议检查锁使用情况",
			FuncName:   strings.Join(lockFunctions, ", "),
		})
	}

	return suggestions
}

// checkFrequentAllocation checks for frequent memory allocation.
func checkFrequentAllocation(ctx *RuleContext) []model.Suggestion {
	suggestions := make([]model.Suggestion, 0)

	if ctx.TopFuncsResult == nil || ctx.ProfilerType != model.ProfilerTypeAsyncAlloc {
		return suggestions
	}

	allocKeywords := []string{
		"alloc", "Alloc", "new", "New", "malloc", "calloc",
		"StringBuilder", "concat", "toString",
	}

	for _, tf := range ctx.TopFuncsResult.TopFuncs {
		if tf.SelfPercent > 5.0 {
			for _, keyword := range allocKeywords {
				if strings.Contains(tf.Name, keyword) {
					suggestions = append(suggestions, model.Suggestion{
						Type:       "frequent_allocation",
						Severity:   "info",
						Suggestion: "函数 " + tf.Name + " 频繁分配内存(" + formatPercent(tf.SelfPercent) + "%)，考虑使用对象池或减少临时对象创建",
						FuncName:   tf.Name,
					})
					break
				}
			}
		}
	}

	return suggestions
}

// checkReflectionUsage checks for reflection usage.
func checkReflectionUsage(ctx *RuleContext) []model.Suggestion {
	suggestions := make([]model.Suggestion, 0)

	if ctx.TopFuncsResult == nil {
		return suggestions
	}

	reflectionKeywords := []string{
		"reflect", "Reflect", "invoke", "Invoke",
		"Method.invoke", "Field.get", "Field.set",
		"Class.forName", "newInstance",
	}

	for _, tf := range ctx.TopFuncsResult.TopFuncs {
		if tf.SelfPercent > 2.0 {
			for _, keyword := range reflectionKeywords {
				if strings.Contains(tf.Name, keyword) {
					suggestions = append(suggestions, model.Suggestion{
						Type:       "reflection_usage",
						Severity:   "info",
						Suggestion: "检测到反射相关函数 " + tf.Name + " 占用较高(" + formatPercent(tf.SelfPercent) + "%)，反射操作性能较低，建议优化",
						FuncName:   tf.Name,
					})
					break
				}
			}
		}
	}

	return suggestions
}

// formatPercent formats a percentage value.
func formatPercent(pct float64) string {
	// Format with up to 2 decimal places, removing trailing zeros
	s := strconv.FormatFloat(pct, 'f', 2, 64)
	// Remove trailing zeros
	s = strings.TrimRight(s, "0")
	// Remove trailing decimal point
	s = strings.TrimRight(s, ".")
	return s
}
