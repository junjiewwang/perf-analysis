# perflib 库抽取实施文档

## 概述

将 perf-analysis 项目中的分析引擎模块抽取为独立的可复用库 `perflib/`，使其可以被其他项目通过 Go Module 引用。

## 架构设计

```
perflib/                          # 独立子模块 (github.com/perf-analysis/perflib)
├── go.mod                        # 外部依赖: google/pprof, stretchr/testify
├── model/                        # 分析数据模型（纯净版，无业务耦合）
│   ├── analysis.go               # 核心接口: AnalysisData, OutputFile, TopItem, Marshal/Unmarshal
│   ├── cpu.go                    # CPUProfilingData, AllocationData, MemoryLeakData, TracingData
│   ├── heap.go                   # HeapAnalysisData + 14个堆分析相关结构体
│   ├── pprof.go                  # PProfCPUData, PProfHeapData, PProfGoroutineData, PProfBlockData, PProfBatchData
│   ├── sample.go                 # Sample, ParseResult, ThreadInfo, TopFuncsMap, SuggestionItem
│   ├── request.go                # 纯净 AnalysisRequest（无 TaskID/COSBucket 等业务字段）
│   └── enum.go                   # Profiler, EventType, ResourceType 枚举
├── profiling/                    # 纯字符串工具函数
│   └── thread.go                 # ExtractThreadGroup, IsSwapperThread, SplitFuncAndModule
├── writer/                       # 通用 JSON/Gzip 写入器
│   └── json.go                   # JSONWriter[T], GzipWriter[T], WriteResult
├── flamegraph/                   # 火焰图引擎 (Sprint 2 Phase 1)
│   ├── model.go                  # Node, FlameGraph, ThreadAnalysisData, NodeBuilder 等
│   ├── generator.go              # Generator, GenerateFromParseResult, GeneratorOptions
│   └── writer.go                 # JSONWriter, GzipWriter, FoldedWriter
├── callgraph/                    # 调用图引擎 (Sprint 2 Phase 1)
│   ├── model.go                  # CallGraph, Node, Edge, ThreadCallGraph, FunctionAnalysis 等
│   ├── generator.go              # Generator, GenerateFromParseResult, GeneratorOptions
│   └── writer.go                 # JSONWriter, GzipWriter, XDotWriter, DOTWriter
├── statistics/                   # 分析统计 (Sprint 2 Phase 1)
│   ├── top_funcs.go              # TopFuncsCalculator, TopFuncsResult
│   └── thread_stats.go           # ThreadStatsCalculator, ThreadStatsResult
├── parser/                       # Parser 接口 + 实现 (Sprint 2 Phase 2-3a)
│   ├── parser.go                 # Parser 接口, Registry, ParseOptions, ParserOption
│   ├── errors.go                 # 6个 sentinel errors
│   ├── collapsed/                # Collapsed 格式解析器
│   │   ├── parser.go             # Parser, ParserOptions, NewParser, Parse, IsCollapsedFormat
│   │   ├── stack_frame.go        # StackFrame, ThreadInfo, SplitFuncAndModule, ExtractThreadInfo
│   │   ├── factory.go            # Factory, RegisterWithRegistry, With*Option
│   │   └── collapsed_test.go     # 完整测试 (15+ test cases)
│   └── pprof/                    # Go pprof 解析器 (需 google/pprof)
│       ├── parser.go             # Parser, SampleType, TopFunction, ToCollapsed, ToSamples
│       ├── leak_detector.go      # LeakDetector, LeakReport, GrowthItem, DetectHeapLeak
│       ├── parser_test.go        # parser 测试
│       └── leak_detector_test.go # leak detector 测试
```

## 实施进展

### Sprint 1: 模型分离 + perflib 骨架 ✅

**状态**: 已完成  
**完成日期**: 2026-04-14

#### 已完成的工作

1. **创建 perflib/ 子模块骨架**
   - `perflib/go.mod` - Go 1.24.9，依赖: google/pprof + klauspost/compress
   - 目录结构: `model/`, `profiling/`, `writer/`

2. **模型拆分** - 将 `pkg/model/output.go`（825行单体）拆分为6个文件
   - `perflib/model/analysis.go` - 核心接口和 Marshal/Unmarshal
   - `perflib/model/cpu.go` - CPU/内存分配/内存泄漏/追踪数据
   - `perflib/model/heap.go` - Java堆分析数据（14个相关结构体）
   - `perflib/model/pprof.go` - Go pprof 所有分析数据类型
   - `perflib/model/sample.go` - 采样/解析结果/线程信息
   - `perflib/model/enum.go` - Profiler/EventType/ResourceType 枚举
   - `perflib/model/request.go` - 纯净 AnalysisRequest/AnalysisResponse

3. **纯工具包迁移**
   - `perflib/profiling/thread.go` - 线程名工具函数
   - `perflib/writer/json.go` - 通用 JSON/Gzip 写入器

4. **向后兼容机制**
   - `pkg/model/aliases.go` - 所有迁移类型的类型别名
   - `go.work` - Go 工作区文件
   - `go.mod` - 添加 perflib 依赖 + replace 指令

5. **编译和测试验证**
   - perflib 子模块编译通过 ✅
   - 主模块编译通过 ✅
   - 所有现有测试通过 ✅

#### 关键设计决策

1. **OutputFile.COSKey 保留**: 原设计移除 COSKey 替换为 RelativePath，但为保证向后兼容，Sprint 1 保留了 COSKey 字段（标记 Deprecated），同时添加 RelativePath。完全移除 COSKey 推迟到 Sprint 2。

2. **ParseResult.Suggestions 类型变更**: 从 `[]Suggestion`（带 DB tags）改为 `[]SuggestionItem`（纯净版）。Parser 实际未填充此字段，consumer 端字段访问兼容。

3. **类型别名策略**: 使用 `type X = libmodel.X`（类型别名而非类型定义），确保完全类型兼容。

4. **AnalysisMode vs AnalysisModeString**: perflib 中函数命名为 `AnalysisModeString` 避免与 `AnalysisMode` 类型命名冲突。主模块保留原始 `AnalysisMode` 函数。

### Sprint 2: 引擎迁移（进行中）

**状态**: Phase 1-3b 已完成 ✅  
**完成日期**: 2026-04-15

目标: 将分析引擎核心逻辑迁移到 perflib，`internal/` 包变为 thin wrapper

#### Phase 1: 无外部依赖包迁移 ✅

迁移 flamegraph、callgraph、statistics 三个包（仅依赖 perflib/model + perflib/profiling + perflib/writer）。

- [x] 创建 `perflib/flamegraph/` (model.go + generator.go + writer.go)
- [x] 创建 `perflib/callgraph/` (model.go + generator.go + writer.go)
- [x] 创建 `perflib/statistics/` (top_funcs.go + thread_stats.go)
- [x] 验证 perflib 子模块编译通过
- [x] 转换 `internal/flamegraph/` 为 thin wrapper（类型别名 + 转发函数）
- [x] 转换 `internal/callgraph/` 为 thin wrapper
- [x] 转换 `internal/statistics/` 为 thin wrapper
- [x] 修复测试文件（移除对 unexported 字段的直接访问）
- [x] 在 perflib 中补充白盒测试（覆盖 unexported 逻辑）
- [x] 主模块编译通过
- [x] 全部测试通过（含 perflib 测试）

**关键设计决策**:
1. 使用 Go 类型别名 `type X = libpkg.X`，确保完全向后兼容
2. 构造函数和工厂函数通过转发函数委托
3. 测试中对 unexported 字段的白盒测试移至 perflib 内部测试
4. `internal/` 的 generator.go 清空为仅包含 package 声明

#### Phase 2: Parser 接口 + Collapsed 解析器迁移 ✅

迁移 parser 接口定义和 collapsed 格式解析器到 perflib。

- [x] 创建 `perflib/parser/parser.go` (Parser 接口, Registry, ParseOptions, ParserOption)
- [x] 创建 `perflib/parser/errors.go` (6个 sentinel errors)
- [x] 创建 `perflib/parser/collapsed/parser.go` (Parser, ParserOptions, Parse, IsCollapsedFormat)
- [x] 创建 `perflib/parser/collapsed/stack_frame.go` (StackFrame, ThreadInfo, SplitFuncAndModule等)
- [x] 创建 `perflib/parser/collapsed/factory.go` (Factory, RegisterWithRegistry, With*Option)
- [x] 创建 `perflib/parser/collapsed/collapsed_test.go` (15+ test cases)
- [x] 转换 `internal/parser/parser.go` 为 thin wrapper
- [x] 转换 `internal/parser/errors.go` 为 thin wrapper
- [x] 转换 `internal/parser/collapsed/` 为 thin wrapper (3 files)
- [x] perflib + 主模块编译通过
- [x] 全部测试通过

**关键设计决策**:
1. `SplitFuncAndModule` 委托到 `perflib/profiling.SplitFuncAndModule`，消除重复代码 (DRY)
2. collapsed parser 的 `ErrInvalidFormat` 在 internal 包保留为本地 `fmt.Errorf` 以避免与 perflib 包级错误冲突

#### Phase 3a: pprof 解析器迁移 ✅

迁移 pprof 解析器到 perflib，添加 google/pprof 外部依赖。

- [x] 创建 `perflib/parser/pprof/parser.go` (Parser, SampleType, TopFunction, ToCollapsed, ToSamples)
- [x] 创建 `perflib/parser/pprof/leak_detector.go` (LeakDetector, LeakReport, GrowthItem)
- [x] 添加 `github.com/google/pprof` 依赖到 perflib/go.mod
- [x] 创建 `perflib/parser/pprof/parser_test.go` + `leak_detector_test.go` (白盒测试)
- [x] 转换 `internal/parser/pprof/parser.go` 为 thin wrapper
- [x] 转换 `internal/parser/pprof/leak_detector.go` 为 thin wrapper
- [x] 修复 internal pprof 测试文件（移除 unexported 字段/方法访问）
- [x] perflib + 主模块编译通过
- [x] 全部测试通过（含 5 个 analyzer 引用 pprof 的测试）

**关键设计决策**:
1. pprof `Parser` 有独立 API（`Parse(io.Reader) error`），不实现 `parser.Parser` 接口
2. internal pprof 测试改为通过 `Parse()` + exported API 进行功能测试，白盒测试在 perflib 覆盖
3. perflib 现有外部依赖: `google/pprof`, `stretchr/testify`

#### Phase 3b: hprof 解析器迁移 ✅

hprof 解析器（Java 堆分析）是最大的迁移包（23个非测试 .go 文件），分3个批次转换为 thin wrapper。

**Sub-phase 3b-1**: 前置准备 — perflib/parser/hprof/ 已包含完整实现代码（由之前的 Sprint 迁移完成）

**Sub-phase 3b-2**: Batch A（3文件）+ Batch B（8文件）✅
- [x] `util_bitset.go` — 类型别名 + 函数转发 (Batch A)
- [x] `util_compression.go` — 类型别名 + 函数转发 (Batch A)
- [x] `util_worker_pool.go` — 类型别名 + 函数转发 (Batch A)
- [x] `types.go` — 22 类型别名 + ~30 常量别名 + 1 函数转发 (Batch B)
- [x] `core_reader.go` — Reader 类型别名 + NewReader 函数转发 (Batch B)
- [x] `doc.go` — 更新包文档 (Batch B)
- [x] `graph_gc_root.go` — 6 类型别名 + 9 常量别名 (Batch B)
- [x] `graph_indexed.go` — 4 类型别名 + 4 函数转发 (Batch B)
- [x] `dom_parallel.go` — 4 类型别名 + 6 函数转发 (Batch B)
- [x] `serial_async.go` — 2 类型别名 + 5 函数转发 (Batch B)
- [x] `util_mmap_store.go` — 5 类型别名（含泛型 MmapArray[T]）+ 5 函数转发 (Batch B)

**Sub-phase 3b-3**: Batch C（12文件）— 原子转换 ✅
由于类型别名后不能再定义方法，12个文件必须同时转换，否则编译失败。
- [x] `graph_reference.go` — 4 类型别名 + 2 函数转发（ReferenceGraph 核心类型）
- [x] `core_result_builder.go` — 1 类型别名（ResultBuilder）
- [x] `graph_buffer_pool.go` — 7 类型别名 + 8 var 别名 + 22 函数转发（最大的 wrapper）
- [x] `serial_serializer.go` — 2 类型别名 + 2 常量 + 5 函数转发
- [x] `dom_dominator.go` — 仅 package 声明（所有方法通过别名继承）
- [x] `dom_hierarchical.go` — 5 类型别名 + 3 常量 + 7 函数转发
- [x] `parallel_analyzer.go` — 9 类型别名 + 3 函数转发
- [x] `parser.go` — 4 类型别名 + 3 常量 + 2 函数转发
- [x] `analysis_biggest_objects.go` — 1 类型别名 + 1 函数转发
- [x] `analysis_retainer.go` — 7 类型别名 + 1 常量 + 1 函数转发
- [x] `analysis_retained_calc.go` — 8 类型别名 + 2 常量 + 1 var 别名 + 3 函数转发
- [x] `analysis_retained_debug.go` — 16 类型别名 + 3 常量 + 4 函数转发

**Sub-phase 3b-4**: 编译验证 ✅
- [x] `go build ./internal/parser/hprof/...` — 通过
- [x] `go build ./...` — 整个项目编译通过

**测试文件处理**: ✅ 已完成
- `parser_test.go` — 已移除（引用未导出字段，perflib 中已有相同白盒测试）
- `serializer_test.go` — 已移除（引用未导出字段，perflib 中已有相同白盒测试）
- `worker_pool_test.go` — 保留（仅使用导出 API，编译运行通过）✅

**关键设计决策**:
1. **原子转换**: Batch C 的12个文件必须同时转换，因为类型别名后不能在不同包中定义新方法
2. **空文件策略**: `dom_dominator.go` 中所有导出标识符都是 `*ReferenceGraph` 上的方法，通过别名自动继承，wrapper 仅需 package 声明
3. **构造函数处理**: `NewResultBuilder` 接受未导出的 `*parserState` 参数，转换后调用发生在 perflib 内部，wrapper 不需要转发此函数
4. **变量别名**: `graph_buffer_pool.go` 中的8个 sync.Pool 变量使用 `var X = libhprof.X` 进行别名

#### Phase 4: Analyzer 迁移（待实施）

- [ ] 迁移 `internal/analyzer/` → `perflib/analyzer/`（需重新设计接口）
- [ ] 替换 `utils.Logger` 为 `log/slog`
- [ ] 完全移除 `OutputFile.COSKey`（用 RelativePath 替代）
- [ ] 在 `internal/` 建立引擎适配层，桥接 perflib 纯净接口和业务层

### Sprint 3: 文档 + 发布（待实施）

**状态**: 待实施

- [ ] perflib README.md
- [ ] API 使用文档
- [ ] 从 Java hprof parser 的迁移策略
- [ ] v0.1.0 release tag

## 文件变更清单

### 新增文件
| 文件 | 说明 |
|------|------|
| `perflib/go.mod` | perflib 子模块定义 |
| `perflib/model/analysis.go` | 核心接口 + Marshal/Unmarshal |
| `perflib/model/cpu.go` | CPU/分配/泄漏/追踪数据类型 |
| `perflib/model/heap.go` | 堆分析数据类型 |
| `perflib/model/pprof.go` | pprof 分析数据类型 |
| `perflib/model/sample.go` | 采样/解析数据类型 |
| `perflib/model/request.go` | 纯净 Request/Response |
| `perflib/model/enum.go` | Profiler/EventType/ResourceType |
| `perflib/profiling/thread.go` | 线程名工具函数 |
| `perflib/writer/json.go` | JSON/Gzip 写入器 |
| `perflib/flamegraph/model.go` | 火焰图数据结构和节点构建 (Sprint 2 Phase 1) |
| `perflib/flamegraph/generator.go` | 火焰图生成器 (Sprint 2 Phase 1) |
| `perflib/flamegraph/writer.go` | 火焰图 JSON/Gzip/Folded 写入器 (Sprint 2 Phase 1) |
| `perflib/flamegraph/model_internal_test.go` | 火焰图白盒测试 (Sprint 2 Phase 1) |
| `perflib/callgraph/model.go` | 调用图数据结构 (Sprint 2 Phase 1) |
| `perflib/callgraph/generator.go` | 调用图生成器 (Sprint 2 Phase 1) |
| `perflib/callgraph/writer.go` | 调用图 JSON/XDot/DOT 写入器 (Sprint 2 Phase 1) |
| `perflib/callgraph/model_internal_test.go` | 调用图白盒测试 (Sprint 2 Phase 1) |
| `perflib/statistics/top_funcs.go` | Top 函数统计 (Sprint 2 Phase 1) |
| `perflib/statistics/thread_stats.go` | 线程统计 (Sprint 2 Phase 1) |
| `perflib/parser/parser.go` | Parser 接口 + Registry + ParseOptions (Sprint 2 Phase 2) |
| `perflib/parser/errors.go` | 6个 sentinel errors (Sprint 2 Phase 2) |
| `perflib/parser/collapsed/parser.go` | Collapsed 格式解析器 (Sprint 2 Phase 2) |
| `perflib/parser/collapsed/stack_frame.go` | 栈帧解析工具 (Sprint 2 Phase 2) |
| `perflib/parser/collapsed/factory.go` | 工厂模式 + 选项函数 (Sprint 2 Phase 2) |
| `perflib/parser/collapsed/collapsed_test.go` | Collapsed 解析器测试 (Sprint 2 Phase 2) |
| `perflib/parser/pprof/parser.go` | Go pprof 解析器 (Sprint 2 Phase 3a) |
| `perflib/parser/pprof/leak_detector.go` | 泄漏检测器 (Sprint 2 Phase 3a) |
| `perflib/parser/pprof/parser_test.go` | pprof 解析器测试 (Sprint 2 Phase 3a) |
| `perflib/parser/pprof/leak_detector_test.go` | 泄漏检测器测试 (Sprint 2 Phase 3a) |
| `pkg/model/aliases.go` | 向后兼容类型别名 |
| `go.work` | Go 工作区文件 |

### 修改文件
| 文件 | 说明 |
|------|------|
| `go.mod` | 添加 perflib 依赖 + replace |
| `pkg/model/output.go` | 移除已迁移类型，仅保留 AnalysisMode 函数 |
| `pkg/model/result.go` | 移除已迁移类型，保留业务类型 |
| `pkg/model/task.go` | 移除已迁移枚举，保留 Task/RequestParams/TaskStatus |
| `internal/flamegraph/model.go` | 改为 thin wrapper (Sprint 2 Phase 1) |
| `internal/flamegraph/generator.go` | 清空为 package 声明 (Sprint 2 Phase 1) |
| `internal/flamegraph/writer.go` | 改为 thin wrapper (Sprint 2 Phase 1) |
| `internal/flamegraph/model_test.go` | 移除 unexported 字段访问 (Sprint 2 Phase 1) |
| `internal/callgraph/model.go` | 改为 thin wrapper (Sprint 2 Phase 1) |
| `internal/callgraph/generator.go` | 清空为 package 声明 (Sprint 2 Phase 1) |
| `internal/callgraph/writer.go` | 改为 thin wrapper (Sprint 2 Phase 1) |
| `internal/callgraph/model_test.go` | 移除 unexported 字段访问 (Sprint 2 Phase 1) |
| `internal/statistics/top_funcs.go` | 改为 thin wrapper (Sprint 2 Phase 1) |
| `internal/statistics/thread_stats.go` | 改为 thin wrapper (Sprint 2 Phase 1) |
| `internal/parser/parser.go` | 改为 thin wrapper (Sprint 2 Phase 2) |
| `internal/parser/errors.go` | 改为 thin wrapper (Sprint 2 Phase 2) |
| `internal/parser/collapsed/parser.go` | 改为 thin wrapper (Sprint 2 Phase 2) |
| `internal/parser/collapsed/stack_frame.go` | 改为 thin wrapper (Sprint 2 Phase 2) |
| `internal/parser/collapsed/factory.go` | 改为 thin wrapper (Sprint 2 Phase 2) |
| `internal/parser/pprof/parser.go` | 改为 thin wrapper (Sprint 2 Phase 3a) |
| `internal/parser/pprof/leak_detector.go` | 改为 thin wrapper (Sprint 2 Phase 3a) |
| `internal/parser/pprof/parser_test.go` | 移除 unexported 访问，改用 exported API (Sprint 2 Phase 3a) |
| `internal/parser/pprof/leak_detector_test.go` | 移除 unexported 访问 (Sprint 2 Phase 3a) |
| `internal/parser/hprof/types.go` | 改为 thin wrapper: 22 类型别名 + ~30 常量别名 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/core_reader.go` | 改为 thin wrapper: Reader 类型别名 + NewReader (Sprint 2 Phase 3b) |
| `internal/parser/hprof/doc.go` | 更新包文档 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/graph_gc_root.go` | 改为 thin wrapper: 6 类型别名 + 9 常量别名 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/graph_indexed.go` | 改为 thin wrapper: 4 类型别名 + 4 函数转发 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/dom_parallel.go` | 改为 thin wrapper: 4 类型别名 + 6 函数转发 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/serial_async.go` | 改为 thin wrapper: 2 类型别名 + 5 函数转发 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/util_mmap_store.go` | 改为 thin wrapper: 5 类型别名 + 5 函数转发 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/util_bitset.go` | 改为 thin wrapper (Sprint 2 Phase 3b) |
| `internal/parser/hprof/util_compression.go` | 改为 thin wrapper (Sprint 2 Phase 3b) |
| `internal/parser/hprof/util_worker_pool.go` | 改为 thin wrapper (Sprint 2 Phase 3b) |
| `internal/parser/hprof/graph_reference.go` | 改为 thin wrapper: 4 类型别名 + 2 函数转发 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/core_result_builder.go` | 改为 thin wrapper: 1 类型别名 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/graph_buffer_pool.go` | 改为 thin wrapper: 7 类型 + 8 var + 22 函数 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/serial_serializer.go` | 改为 thin wrapper: 2 类型 + 2 常量 + 5 函数 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/dom_dominator.go` | 改为 thin wrapper: 仅 package 声明 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/dom_hierarchical.go` | 改为 thin wrapper: 5 类型 + 3 常量 + 7 函数 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/parallel_analyzer.go` | 改为 thin wrapper: 9 类型 + 3 函数 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/parser.go` | 改为 thin wrapper: 4 类型 + 3 常量 + 2 函数 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/analysis_biggest_objects.go` | 改为 thin wrapper: 1 类型 + 1 函数 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/analysis_retainer.go` | 改为 thin wrapper: 7 类型 + 1 常量 + 1 函数 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/analysis_retained_calc.go` | 改为 thin wrapper: 8 类型 + 2 常量 + 1 var + 3 函数 (Sprint 2 Phase 3b) |
| `internal/parser/hprof/analysis_retained_debug.go` | 改为 thin wrapper: 16 类型 + 3 常量 + 4 函数 (Sprint 2 Phase 3b) |

## 遗留问题

1. **OutputFile.COSKey 未移除**: 为向后兼容暂时保留，Phase 4 处理
2. **Suggestion 业务类型**: `pkg/model/suggestion.go` 中的 `Suggestion` 仍含 DB tags，perflib 使用 `SuggestionItem` 替代
3. ~~**perflib 尚无测试**~~: Sprint 2 已补充 flamegraph + callgraph 白盒测试 ✅
4. **go.work.sum**: 会自动生成，已加入 `.gitignore` 考虑
5. ~~**perflib 零外部依赖**~~: Phase 3a 已添加 google/pprof ✅
6. **utils.Logger 替换**: Phase 4 迁移 analyzer 时需将 utils.Logger 替换为 log/slog
7. ~~**hprof 测试文件处理**~~: 已移除 `parser_test.go` 和 `serializer_test.go`，保留 `worker_pool_test.go` ✅

## 验证命令

```bash
# 编译 perflib
cd perflib && go build ./...

# 编译主模块
cd .. && go build ./...

# 运行所有测试
go test ./...
```
