# 遗留问题实施方案（后端）

> 创建日期: 2026-05-08  
> 状态: ✅ 实施完成  
> 前置文档: [后端 API 设计](./design-backend-api-for-prototype.md)  
> 范围: 仅后端（问题 1/2/3），前端选型（问题 4）待前端重构时再设计

---

## 问题 1: Go pprof Heap 的 Allocation Histogram

### 问题分析

当前 `/api/refgraph/class-histogram` 只支持 Java hprof 场景（基于 `HeapGraph` 接口按 classID 聚合）。
Go pprof heap 没有 "类" 概念，其分配数据按 **分配点函数**（allocation site）组织，底层数据为：

```go
type PProfHeapData struct {
    InuseSpace      *PProfMemoryStats  // 当前持有的空间
    InuseObjects    *PProfMemoryStats  // 当前持有的对象数
    AllocSpace      *PProfMemoryStats  // 累计分配空间
    AllocObjects    *PProfMemoryStats  // 累计分配对象数
    FlameGraphFiles map[string]string
    HeapSummary     *PProfHeapSummary
}

type PProfMemoryStats struct {
    Total     int64
    Unit      string
    TopFuncs  []PProfTopFunc  // ← 这就是 Go heap 的 "histogram by function"
    TopNCount int
}
```

**核心洞察**：Go pprof heap 的 `TopFuncs` 天然就是按分配函数聚合的 histogram，
只是字段语义与 Java Class Histogram 不同。

### 设计方案

**方案：统一 API 入口 + 双模型适配（Strategy Pattern）**

```
┌──────────────────────────────────────────────────────────────────┐
│  GET /api/heap/histogram?task=<id>&metric=<inuse_space|...>&...  │
└──────────┬───────────────────────────────────────────────────────┘
           │
    ┌──────▼──────┐
    │ Detect Type │  根据 task 目录下存在哪种数据文件自动识别
    └──────┬──────┘
           │
    ┌──────┴───────────────────────────────┐
    ▼                                      ▼
┌─────────────────────┐    ┌─────────────────────────────────┐
│ Java hprof path     │    │ Go pprof path                   │
│ heap_index.bin 存在  │    │ 火焰图/分析 JSON 存在，无 index  │
│                     │    │                                 │
│ → HeapQueryHelper   │    │ → PProfHeapQueryHelper          │
│   .QueryClassHisto  │    │   .QueryAllocHistogram          │
└─────────────────────┘    └─────────────────────────────────┘
```

### 新增 API

```
GET /api/heap/histogram?task=<id>&metric=<inuse_space|inuse_objects|alloc_space|alloc_objects>&sort=<flat|cum|flat_pct>&top=<N>&filter=<funcName>
```

**响应结构（统一）**：

```go
// AllocHistogramResponse 统一 API 响应（Java 和 Go 共用）
type AllocHistogramResponse struct {
    Source     string               `json:"source"`      // "java_hprof" | "go_pprof"
    Metric     string               `json:"metric"`      // "inuse_space" | "retained_size" | ...
    Total      int64                `json:"total"`
    Unit       string               `json:"unit"`        // "bytes" | "objects"
    EntryCount int                  `json:"entry_count"` // 总条目数
    Entries    []AllocHistogramEntry `json:"entries"`
}

type AllocHistogramEntry struct {
    Name       string  `json:"name"`        // Java: className, Go: funcName
    Flat       int64   `json:"flat"`        // Java: shallowSize, Go: flat
    FlatPct    float64 `json:"flat_pct"`
    Cum        int64   `json:"cum"`         // Java: retainedSize, Go: cum
    CumPct     float64 `json:"cum_pct"`
    Count      int64   `json:"count"`       // Java: objectCount, Go: 不适用(0)
    Module     string  `json:"module,omitempty"` // Go: package path
}
```

### 实现步骤

| # | 任务 | 文件 | 工作量 |
|---|------|------|--------|
| 1 | 新增 `PProfHeapQueryHelper` | `perflib/query/pprof_heap_query.go` | 0.5 天 |
| 2 | 分析时预计算并持久化 `heap_analysis.json`（含 TopFuncs 完整数据） | `perflib/analyzer/pprof_heap_analyzer.go` | 0.5 天 |
| 3 | 新增统一 handler `handleHeapHistogram` | `internal/webui/pprof_api_handlers.go` | 0.5 天 |
| 4 | 保留原 `/api/refgraph/class-histogram` 兼容性 | `internal/webui/pprof_api_handlers.go` | 极低 |

### 预计算 vs 运行时

遵循"分析时计算，展示时读取"原则：
- **分析阶段**：`PProfHeapAnalyzer` 输出 `heap_analysis.json`，包含完整的 4 维 TopFuncs
- **API 阶段**：`PProfHeapQueryHelper` 从 JSON 加载 + 排序/过滤/限制

---

## 问题 2: Goroutine Issues 检测规则增强

### 当前状态

已实现 4 条规则：
1. ✅ Excessive count（> 10k/100k）
2. ✅ Dominant group（> 50% + > 100）
3. ✅ Fragmentation（70%+ 单一 goroutine groups）
4. ✅ Blocking patterns（TopFunc 匹配 Lock/Wait/chan 且 > 20%）

### 新增规则设计

| # | 规则名 | 严重度 | 检测逻辑 | 优先级 |
|---|--------|--------|---------|--------|
| 5 | **Mutex Contention** | warning | 多个 group 的栈顶含 `sync.Mutex.Lock` 且总数 > 50 | P1 |
| 6 | **Channel Deadlock Risk** | warning | 同时存在大量 `chan send` 和 `chan receive` 阻塞的 group | P2 |
| 7 | **IO Wait Saturation** | info | `net/http.(*conn).serve` 或 `io.Read` 类 group 占比 > 80% | P1 |
| 8 | **Timer/Ticker Leak** | warning | `time.Sleep` 或 `time.After` group 数量 > 100 | P2 |
| 9 | **Context Cancellation Pile-up** | info | `context.(*cancelCtx).Done` group 数量异常高 | P2 |

### 架构设计：规则引擎模式

```go
// Rule 是 goroutine 问题检测规则的接口
type Rule interface {
    Name() string
    Evaluate(data *model.PProfGoroutineData) []GoroutineIssue
}

// RuleEngine 管理所有规则
type RuleEngine struct {
    rules []Rule
}

func NewDefaultRuleEngine() *RuleEngine {
    return &RuleEngine{
        rules: []Rule{
            &ExcessiveCountRule{},
            &DominantGroupRule{},
            &FragmentationRule{},
            &BlockingPatternRule{},
            // 新增规则
            &MutexContentionRule{},
            &ChannelDeadlockRule{},
            &IOWaitSaturationRule{},
            &TimerLeakRule{},
            &ContextPileupRule{},
        },
    }
}

func (e *RuleEngine) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
    var issues []GoroutineIssue
    for _, rule := range e.rules {
        issues = append(issues, rule.Evaluate(data)...)
    }
    // 按严重度排序
    sort.Slice(issues, func(i, j int) bool {
        return severityOrder(issues[i].Severity) < severityOrder(issues[j].Severity)
    })
    return issues
}
```

### 实现步骤

| # | 任务 | 文件 | 工作量 |
|---|------|------|--------|
| 1 | 将现有 4 条规则重构为 `Rule` 接口实现 | `perflib/query/goroutine_rules.go` | 0.5 天 |
| 2 | 实现 Rule 5-9 | `perflib/query/goroutine_rules.go` | 1 天 |
| 3 | `QueryIssues()` 改为委托给 `RuleEngine` | `perflib/query/goroutine_query.go` | 0.5 天 |
| 4 | 新增 `GoroutineIssue.Suggestion` 字段，附带修复建议 | model 调整 | 0.5 天 |
| 5 | 单元测试（每条规则至少 2 个用例） | `perflib/query/goroutine_rules_test.go` | 1 天 |

### GoroutineIssue 增强字段

```go
type GoroutineIssue struct {
    Severity    string   `json:"severity"`
    Type        string   `json:"type"`
    Title       string   `json:"title"`
    Description string   `json:"description"`
    Suggestion  string   `json:"suggestion,omitempty"`  // 新增：修复建议
    GroupIndex  int      `json:"group_index,omitempty"`
    RelatedFuncs []string `json:"related_funcs,omitempty"` // 新增：关联函数
}
```

---

## 问题 3: 实时 vs 静态分析（Heap 预计算优化）

### 问题分析

当前违背"分析时计算，展示时读取"原则的 API：

| API | 当前行为 | 问题 |
|-----|---------|------|
| `/api/refgraph/class-histogram` | 每次请求遍历全量 HeapGraph (可能百万对象) | O(N) 延迟 |
| `/api/refgraph/heap-stats` | 每次请求遍历全量 HeapGraph | O(N) 延迟 |

**实际影响评估**：
- HeapGraph 已常驻内存（RefGraphService 缓存），遍历为 CPU-bound
- 10M 对象 × 简单计算 ≈ 100-200ms，可接受但不优雅
- 多并发请求时会竞争 CPU

### 设计方案：Lazy Precompute + Cache

**原则**：分析时预计算写入文件；API 优先读取预计算文件，fallback 到运行时计算。

```
┌─────────────────────────────────────────────────────────────┐
│  分析阶段（Analyzer）                                        │
│  ├─ java_heap_analyzer: 写入 heap_index.bin + class_stats.json │
│  └─ pprof_heap_analyzer: 写入 heap_analysis.json              │
├─────────────────────────────────────────────────────────────┤
│  API 阶段（Handler）                                         │
│  ├─ 1. 尝试读取 class_stats.json（预计算文件）               │
│  ├─ 2. 命中 → 直接返回（O(1)）                              │
│  └─ 3. 未命中 → fallback 到 HeapGraph 运行时计算（兼容旧数据）│
└─────────────────────────────────────────────────────────────┘
```

### 预计算文件设计

**新增常量**：
```go
// perflib/output/convention.go
const (
    FileClassStats = "class_stats.json"     // Java heap class histogram 预计算
    FileHeapStats  = "heap_stats.json"      // Heap 概览统计预计算
    FilePProfHeapAnalysis = "heap_analysis.json" // Go pprof heap 分析数据
)
```

**`class_stats.json` 结构**：
```json
{
  "total_classes": 1234,
  "total_objects": 567890,
  "total_size": 1234567890,
  "classes": [
    {
      "class_name": "java.lang.String",
      "object_count": 12345,
      "shallow_size": 296280,
      "retained_size": 1048576,
      "percentage": 12.5
    }
  ]
}
```

### 实现步骤

| # | 任务 | 文件 | 工作量 |
|---|------|------|--------|
| 1 | Java heap 分析器分析完成后预计算并输出 `class_stats.json` | `perflib/analyzer/java_heap_analyzer.go` | 1 天 |
| 2 | Java heap 分析器输出 `heap_stats.json` | 同上 | 0.5 天 |
| 3 | Handler 增加"先读文件，再 fallback"逻辑 | `internal/webui/pprof_api_handlers.go` | 0.5 天 |
| 4 | Go pprof heap 分析器输出 `heap_analysis.json` | `perflib/analyzer/pprof_heap_analyzer.go` | 0.5 天 |
| 5 | 新增 `output.FileClassStats` / `output.FileHeapStats` 常量 | `perflib/output/convention.go` | 极低 |

### 兼容性策略

- **新数据**：分析时产出预计算文件，API 直接读取，毫秒级响应
- **旧数据**：无预计算文件，fallback 到 HeapGraph 运行时计算，数百毫秒响应
- **渐进迁移**：用户重新分析旧 profile 即可自动生成预计算文件

---

## 实施优先级与排期

### 总览

| 问题 | 优先级 | 预估工作量 | 建议排期 |
|------|--------|-----------|---------|
| 问题 3（预计算优化） | **P0** | 3 天 | Sprint 2 Week 1 |
| 问题 1（Go heap histogram） | **P0** | 2 天 | Sprint 2 Week 1 |
| 问题 2（Rules 增强） | P1 | 3 天 | Sprint 2 Week 2 |

### 依赖关系

```
问题 3（预计算优化）
    └──→ 问题 1（Go heap histogram）← 依赖问题 3 的预计算基础设施
    
问题 2（Rules 增强）← 独立，无依赖
```

### Sprint 2 实施顺序

```
Day 1-2: 问题 3 — 预计算基础设施
  ├─ 新增常量 (FileClassStats, FileHeapStats, FilePProfHeapAnalysis)
  ├─ Java heap analyzer 预计算输出
  ├─ Go pprof heap analyzer 预计算输出
  └─ Handler "先读文件再 fallback" 逻辑

Day 3: 问题 1 — Go heap histogram
  ├─ PProfHeapQueryHelper 实现
  ├─ 统一 API /api/heap/histogram handler
  └─ 单元测试

Day 4-5: 问题 2 — Rules 增强
  ├─ Rule 接口抽象 + 现有规则重构
  ├─ 实现 5 条新规则
  └─ 单元测试（每规则 2+ 用例）

Day 6: 集成测试 + 文档更新
```

---

## 验收标准

| 问题 | 验收条件 |
|------|---------|
| 问题 1 | Go pprof heap 数据可通过 `/api/heap/histogram?metric=inuse_space` 获取分配函数 histogram；Java hprof 数据同一 API 返回 class histogram |
| 问题 2 | `QueryIssues()` 返回结构化 issues，每条含 severity/type/title/description/suggestion；新规则有单测覆盖 |
| 问题 3 | 新分析的 Java heap 任务目录下产出 `class_stats.json` + `heap_stats.json`；API 响应时间 < 10ms（预计算命中时） |

---

## 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| 预计算文件增加磁盘占用 | 低（每文件 < 1MB） | gzip 压缩；只对 top-N 预计算 |
| Rule 误报率高 | 中（用户不信任） | 阈值可配置；标注 confidence level |
| Go pprof 与 Java 统一 API 语义差异 | 中（前端混淆） | 响应中明确 `source` 字段，前端按 source 渲染不同列标题 |

---

## 实施记录

### 2026-05-08 实施完成

#### 问题 3: 预计算优化 ✅

**变更文件**：
- `perflib/output/convention.go` — 新增 `FileClassStats`, `FileHeapStats`, `FilePProfHeapAnalysis` 常量
- `perflib/analyzer/java_heap_analyzer.go` — Step 9b 新增预计算逻辑：
  - `buildPrecomputedClassStats()`: 遍历 IndexedReferenceGraph，按类聚合 retained/shallow/count，输出 top 500 类
  - `buildPrecomputedHeapStats()`: 输出 heap 概览统计
  - 写入 `class_stats.json` + `heap_stats.json`（非致命，失败只 warn）
- `perflib/analyzer/pprof_heap_analyzer.go` — Step 5 写入 `heap_analysis.json`
- `internal/webui/pprof_api_handlers.go` — `handleClassHistogram` 和 `handleHeapStats` 重写为"先读预计算文件，再 fallback"模式

#### 问题 1: Go pprof Heap Histogram ✅

**变更文件**：
- `perflib/query/pprof_heap_query.go` — 新建 `PProfHeapQueryHelper`，支持 4 维度指标查询 + 排序/过滤/限制
- `internal/webui/pprof_api_handlers.go` — 新增 `handleHeapHistogram` 统一 API handler
- `internal/webui/server.go` — 注册 `/api/heap/histogram` 路由

**统一 API 设计**：
- 路径: `GET /api/heap/histogram?task=<id>&metric=<...>&sort=<...>&top=<N>&filter=<name>`
- 自动检测数据类型：`heap_index.bin` 存在 → Java hprof 路径，否则 → Go pprof 路径
- 统一响应结构 `AllocHistogramResponse` 含 `source` 字段标识数据来源

#### 问题 2: Goroutine Rules 增强 ✅

**变更文件**：
- `perflib/query/goroutine_rules.go` — 新建规则引擎：
  - `GoroutineRule` 接口 + `GoroutineRuleEngine` 执行引擎
  - 现有 4 条规则重构为独立结构体（`ExcessiveCountRule`, `DominantGroupRule`, `BlockingFunctionRule`, `FragmentationRule`）
  - 5 条新规则：`IOWaitRule`, `MutexContentionRule`, `SyscallBlockingRule`, `ChannelLeakRule`, `SleepAccumulationRule`
- `perflib/query/goroutine_query.go` — 
  - `GoroutineIssue` 新增 `Suggestion` 和 `RelatedFuncs` 字段
  - `QueryIssues()` 委托给 `GoroutineRuleEngine`

**规则清单（9 条）**：

| # | 规则 | 类型 | 严重度 |
|---|------|------|--------|
| 1 | ExcessiveCountRule | excessive | critical/warning |
| 2 | DominantGroupRule | blocking | warning |
| 3 | BlockingFunctionRule | blocking | warning |
| 4 | IOWaitRule | io_wait | info/warning |
| 5 | MutexContentionRule | mutex_contention | info/warning/critical |
| 6 | FragmentationRule | fragmentation | info |
| 7 | SyscallBlockingRule | syscall_blocking | warning |
| 8 | ChannelLeakRule | channel_leak | warning/critical |
| 9 | SleepAccumulationRule | sleep_accumulation | info |

### 编译验证

- `go build ./...` ✅ 通过
- `go vet ./...` ✅ 通过
- `go test ./perflib/query/...` ✅ 全部通过（覆盖率 68.8%）

### 单元测试清单

- `goroutine_rules_test.go`：
  - RuleEngine 基础测试（nil data、no issues、自定义规则注入）
  - IOWaitRule（4 个用例：triggered/info severity/low percentage/no IO functions）
  - MutexContentionRule（4 个用例：warning/critical/low contention/no mutex）
  - SyscallBlockingRule（4 个用例：by state/by stack pattern/low percentage/no syscalls）
  - ChannelLeakRule（5 个用例：warning/critical/low percentage/low total count/selectgo）
  - SleepAccumulationRule（4 个用例：triggered/timer pattern/low count/no sleep）
  - Helper 函数测试（containsInStack、appendUnique）
  - 集成测试（多规则同时触发）

- `pprof_heap_query_test.go`：
  - NilData 处理
  - 4 种指标（inuse_space/inuse_objects/alloc_space/alloc_objects）
  - 默认指标回退
  - 3 种排序方式（flat/cum/flat_pct）
  - TopN 限制（含默认值）
  - 过滤（精确/部分匹配/大小写不敏感/无匹配）
  - HeapStats（正常/无 InuseSpace/无 Summary）

### 遗留任务

- [ ] 前端适配 `/api/heap/histogram` 统一 API（待前端重构时进行）
