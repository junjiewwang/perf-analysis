# perflib

[![Go Reference](https://pkg.go.dev/badge/github.com/junjiewwang/perf-analysis/perflib.svg)](https://pkg.go.dev/github.com/junjiewwang/perf-analysis/perflib)

**perflib** 是一个可复用的 Go 性能分析引擎库，支持 Java/Go/C++ 等多语言的 profiling 数据解析与分析。

## 特性

- **13 种分析模式** — 覆盖 CPU、内存、锁竞争、Goroutine、堆转储等场景
- **多格式解析** — 支持 collapsed stack、Go pprof、Java HPROF 二进制格式
- **火焰图生成** — 全局 + 线程级火焰图，支持热点函数分析
- **调用图生成** — 函数调用关系图，支持热路径检测和模块聚合
- **统计分析** — Top 热点函数排名、线程分布统计
- **泄漏检测** — Go heap/goroutine 泄漏检测（多快照对比）
- **零业务耦合** — 纯分析引擎，无数据库/网络/业务依赖
- **可扩展** — 通过 Factory 注册自定义分析器，通过 Logger/Timer 接口集成外部基础设施

## 安装

```bash
go get github.com/junjiewwang/perf-analysis/perflib
```

要求 Go 1.24 或更高版本。

## 快速开始

### 示例 1: 分析 Java CPU 热点

使用 Factory 模式分析 async-profiler 采集的 collapsed stack 数据：

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/junjiewwang/perf-analysis/perflib/analyzer"
	"github.com/junjiewwang/perf-analysis/perflib/model"
)

func main() {
	// 创建工厂（使用默认配置）
	factory := analyzer.NewFactory(nil)

	// 根据模式创建分析器
	a, err := factory.CreateAnalyzerForMode(analyzer.ModeJavaCPU)
	if err != nil {
		log.Fatal(err)
	}

	// 构建分析请求
	req := &model.AnalysisRequest{
		Mode:      string(analyzer.ModeJavaCPU),
		InputFile: "/path/to/cpu.collapsed",
		OutputDir: "/tmp/analysis-output",
	}

	// 执行分析
	resp, err := a.Analyze(context.Background(), req)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("分析完成: %d 条记录, %d 个输出文件\n",
		resp.TotalRecords, len(resp.OutputFiles))
	for _, f := range resp.OutputFiles {
		fmt.Printf("  → %s (%s)\n", f.RelativePath, f.Description)
	}
}
```

### 示例 2: 分析 Go pprof CPU Profile

```go
config := analyzer.DefaultBaseAnalyzerConfig()
config.OutputDir = "/tmp/pprof-output"
config.TopFuncsN = 100
config.AnalysisProfile = analyzer.ProfileDetailed

factory := analyzer.NewFactory(config)
a, _ := factory.CreateAnalyzerForMode(analyzer.ModePProfCPU)

resp, err := a.Analyze(context.Background(), &model.AnalysisRequest{
	Mode:      string(analyzer.ModePProfCPU),
	InputFile: "/path/to/cpu.pprof",
})
```

### 示例 3: 使用 io.Reader 分析

所有分析器支持通过 `AnalyzeFromReader` 接受 `io.Reader` 输入，适用于流式数据或内存中的数据：

```go
file, _ := os.Open("/path/to/profile.collapsed")
defer file.Close()

resp, err := a.AnalyzeFromReader(context.Background(), req, file)
```

## 架构

```
perflib/                     # 根包：Logger, Timer 接口
├── model/                   # 数据模型（Request/Response/Sample/AnalysisData）
├── analyzer/                # 分析引擎（Factory + 13 种分析器）
│   ├── factory.go           #   Factory 模式：CreateAnalyzerForMode
│   ├── base_analyzer.go     #   BaseAnalyzer：解析/火焰图/调用图/统计
│   ├── java_cpu_analyzer    #   Java CPU/Wall/Lock/Perf 分析
│   ├── java_mem_analyzer    #   Java 内存分配分析
│   ├── java_heap_analyzer   #   Java HPROF 堆转储分析
│   ├── pprof_*_analyzer     #   Go pprof CPU/Heap/Goroutine/Block/Mutex
│   └── pprof_batch_analyzer #   pprof 批量分析 + 泄漏检测
├── parser/                  # 解析器接口 + 实现
│   ├── collapsed/           #   Collapsed stack 格式解析器
│   ├── pprof/               #   Go pprof 格式解析器 + 泄漏检测
│   └── hprof/               #   Java HPROF 二进制格式解析器
├── flamegraph/              # 火焰图生成引擎
├── callgraph/               # 调用图生成引擎
├── statistics/              # 统计分析（Top 函数 + 线程统计）
├── profiling/               # 通用工具（线程名解析等）
└── writer/                  # 通用 JSON/Gzip 写入器
```

**数据流**:

```
输入文件 → Parser → model.ParseResult/Sample
                          ↓
                     Analyzer
                    ↙    ↓    ↘
           FlameGraph  CallGraph  Statistics
                    ↘    ↓    ↙
              model.AnalysisResponse
              (OutputFiles + Data + Suggestions)
```

## 分析模式

| 模式 | 常量 | 描述 | 输入格式 |
|------|------|------|----------|
| `async-profiler-cpu` | `ModeJavaCPU` | Java CPU 热点分析 | Collapsed stack |
| `async-profiler-alloc` | `ModeJavaAlloc` | Java 内存分配分析 | Collapsed stack |
| `async-profiler-wall` | `ModeJavaWall` | Java Wall-clock 分析 | Collapsed stack |
| `async-profiler-lock` | `ModeJavaLock` | Java 锁竞争分析 | Collapsed stack |
| `heapdump-heap` | `ModeJavaHeap` | Java 堆转储分析 | HPROF 二进制 |
| `perf-cpu` | `ModeCPU` | 通用 CPU 分析 | Collapsed stack |
| `pprof-cpu` | `ModePProfCPU` | Go pprof CPU 分析 | pprof (.pb.gz) |
| `pprof-heap` | `ModePProfHeap` | Go pprof Heap 分析 | pprof (.pb.gz) |
| `pprof-goroutine` | `ModePProfGoroutine` | Go Goroutine 分析 | pprof (.pb.gz) |
| `pprof-block` | `ModePProfBlock` | Go Block 分析 | pprof (.pb.gz) |
| `pprof-mutex` | `ModePProfMutex` | Go Mutex 分析 | pprof (.pb.gz) |
| `pprof-all` | `ModePProfAll` | pprof 批量分析 | 含子目录的目录 |
| `jeprof-heap` | `ModeJeprof` | Jemalloc 堆分析 | Jeprof 格式 |

使用 `analyzer.ParseMode(s)` 从字符串解析模式，`analyzer.AllModes()` 获取所有模式元信息。

## 配置

### BaseAnalyzerConfig

通过 `analyzer.DefaultBaseAnalyzerConfig()` 获取默认配置：

```go
config := analyzer.DefaultBaseAnalyzerConfig()
config.OutputDir = "/tmp/output"       // 输出目录
config.TopFuncsN = 100                 // Top N 热点函数数量（默认 50）
config.IncludeSwapper = true           // 包含 swapper/idle 线程
config.Verbose = true                  // 详细日志
config.Logger = myLogger               // 注入自定义 Logger
config.AnalysisProfile = analyzer.ProfileDetailed // 分析深度
```

### AnalysisProfile（分析档位）

三种预设档位控制火焰图/调用图的分析深度：

| 参数 | Quick | Standard（默认） | Detailed |
|------|-------|---------|----------|
| 火焰图 MinPercent | 0.5% | 0.1% | 0.05% |
| 火焰图 TopNGlobal | 20 | 50 | 100 |
| 火焰图 线程分析 | ❌ | ✅ | ✅ |
| 火焰图 每线程火焰图 | ❌ | ✅ | ✅ |
| 调用图 MinNodePct | 1.0% | 0.5% | 0.1% |
| 调用图 热路径分析 | ❌ | ✅ | ✅ |
| 调用图 模块聚合 | ❌ | ✅ | ✅ |

### FlameGraphOptions / CallGraphOptions

如需精细控制，可直接设置 `config.FlameGraphOptions` 和 `config.CallGraphOptions`：

```go
config.FlameGraphOptions = &flamegraph.GeneratorOptions{
	MinPercent:                0.05,
	EnableThreadAnalysis:      true,
	BuildPerThreadFlameGraphs: true,
	TopNPerThread:             30,
	TopNGlobal:                100,
	MaxCallStacksPerThread:    500,
	MaxCallStacksPerFunc:      20,
	IncludeModule:             true,
}
```

## Logger 集成

perflib 通过 `perflib.Logger` 接口接受外部日志器，最小化耦合：

```go
// perflib.Logger 接口（4 个方法）
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}
```

**适配 log/slog**:

```go
type SlogAdapter struct{ logger *slog.Logger }

func (a *SlogAdapter) Debug(msg string, args ...interface{}) { a.logger.Debug(fmt.Sprintf(msg, args...)) }
func (a *SlogAdapter) Info(msg string, args ...interface{})  { a.logger.Info(fmt.Sprintf(msg, args...)) }
func (a *SlogAdapter) Warn(msg string, args ...interface{})  { a.logger.Warn(fmt.Sprintf(msg, args...)) }
func (a *SlogAdapter) Error(msg string, args ...interface{}) { a.logger.Error(fmt.Sprintf(msg, args...)) }

config.Logger = &SlogAdapter{logger: slog.Default()}
```

**适配 zap**:

```go
type ZapAdapter struct{ logger *zap.SugaredLogger }

func (a *ZapAdapter) Debug(msg string, args ...interface{}) { a.logger.Debugf(msg, args...) }
func (a *ZapAdapter) Info(msg string, args ...interface{})  { a.logger.Infof(msg, args...) }
func (a *ZapAdapter) Warn(msg string, args ...interface{})  { a.logger.Warnf(msg, args...) }
func (a *ZapAdapter) Error(msg string, args ...interface{}) { a.logger.Errorf(msg, args...) }
```

不设置 Logger 时，所有日志静默丢弃。

## Timer 集成

`perflib.Timer` 接口用于分析过程的阶段计时：

```go
type Timer interface {
	Start(phaseName string) PhaseTimer
	TimeFunc(phaseName string, fn func()) time.Duration
	TimeFuncWithError(phaseName string, fn func() error) (time.Duration, error)
	PrintSummary()
}
```

不需要计时时使用 `perflib.NullTimer`（零开销空实现）。

## 解析器

### Collapsed Stack 解析器

解析 async-profiler、perf 等工具输出的折叠调用栈格式：

```go
import "github.com/junjiewwang/perf-analysis/perflib/parser/collapsed"

p := collapsed.NewParser(collapsed.DefaultParserOptions())
result, err := p.Parse(ctx, reader)
// result.Samples — []*model.Sample
// result.TotalSamples — 总采样数
```

### Go pprof 解析器

解析 Go pprof 格式的 profile 数据（.pprof, .pb.gz）：

```go
import "github.com/junjiewwang/perf-analysis/perflib/parser/pprof"

p := pprof.NewParser()
err := p.Parse(reader)

// 获取 collapsed 格式输出
collapsed := p.ToCollapsed(pprof.SampleTypeCPU)

// 获取 model.Sample 列表
samples := p.ToSamples(pprof.SampleTypeCPU)
```

### pprof 泄漏检测

通过多快照对比检测内存/goroutine 泄漏：

```go
detector := pprof.NewLeakDetector()
report, err := detector.DetectHeapLeak(profiles) // []*profile.Profile
// report.Items — 增长项列表
// report.Summary — 泄漏摘要
```

### Java HPROF 解析器

解析 Java 堆转储 HPROF 二进制格式，详见 `parser/hprof` 包文档（`doc.go`）。

## 扩展

### 注册自定义分析器

通过 `Factory.RegisterConstructor` 注册自定义分析模式：

```go
factory := analyzer.NewFactory(config)

// 注册自定义模式
factory.RegisterConstructor("custom-mode", func(cfg *analyzer.BaseAnalyzerConfig) analyzer.Analyzer {
	return NewMyCustomAnalyzer(cfg)
})

// 使用自定义模式
a, err := factory.CreateAnalyzerForMode("custom-mode")
```

### 自定义分析器需实现 Analyzer 接口

```go
type Analyzer interface {
	Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error)
	AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, reader io.Reader) (*model.AnalysisResponse, error)
	Name() string
}
```

## 错误处理

`analyzer` 包定义了 5 个 sentinel 错误，可用 `errors.Is` 匹配：

```go
import "github.com/junjiewwang/perf-analysis/perflib/analyzer"

resp, err := a.Analyze(ctx, req)
if errors.Is(err, analyzer.ErrEmptyData) {
	// 输入数据为空
} else if errors.Is(err, analyzer.ErrParseError) {
	// 解析失败
} else if errors.Is(err, analyzer.ErrUnsupportedMode) {
	// 不支持的分析模式
}
```

完整列表：`ErrUnsupportedMode`、`ErrParseError`、`ErrEmptyData`、`ErrAnalysisFailed`、`ErrContextCanceled`。

## 包参考

| 包 | 描述 |
|------|------|
| `perflib` | 根包：`Logger`、`Timer`、`PhaseTimer` 接口，`NullTimer` |
| `model` | 数据模型：`AnalysisRequest`/`Response`、`Sample`、`AnalysisData` 接口及 11 种实现 |
| `analyzer` | 分析引擎：`Analyzer` 接口、`Factory`、`BaseAnalyzer`、13 种具体分析器 |
| `parser` | 解析器接口：`Parser`、`Registry`、`ParseOptions` |
| `parser/collapsed` | Collapsed stack 格式解析器 |
| `parser/pprof` | Go pprof 格式解析器 + `LeakDetector` |
| `parser/hprof` | Java HPROF 二进制格式解析器（dominator tree、retained size、GC roots） |
| `flamegraph` | 火焰图生成引擎：`Generator`、`GeneratorOptions`、JSON/Gzip/Folded 写入器 |
| `callgraph` | 调用图生成引擎：`Generator`、`GeneratorOptions`、JSON/Gzip/XDot/DOT 写入器 |
| `statistics` | 统计分析：`TopFuncsCalculator`、`ThreadStatsCalculator` |
| `profiling` | 工具函数：`ExtractThreadGroup`、`IsSwapperThread`、`SplitFuncAndModule` |
| `writer` | 通用泛型写入器：`JSONWriter[T]`、`GzipWriter[T]` |

## 依赖

| 依赖 | 用途 |
|------|------|
| `github.com/google/pprof` | Go pprof 格式解析 |
| `github.com/klauspost/compress` | 高性能 gzip 压缩 |
| `google.golang.org/protobuf` | HPROF protobuf 序列化 |
| `github.com/stretchr/testify` | 测试断言（仅测试） |

## License

请参考项目根目录的 LICENSE 文件。
