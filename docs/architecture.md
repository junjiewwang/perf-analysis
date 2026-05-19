# Architecture Guide

> 本文档描述 perf-analysis 项目中 **Java Heap Dump 分析子系统** 的架构设计。  
> 涵盖 Two-Pass CSR 解析引擎、HeapQueryEngine 按需查询模型、WebUI 数据流以及索引文件格式。

---

## 一、系统总览

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                            perf-analysis                                      │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────┐    ┌──────────────┐    ┌────────────┐    ┌──────────────────┐ │
│  │  CLI /   │───▶│  Analyzer    │───▶│  perflib   │───▶│  heap_index.bin  │ │
│  │  Service │    │ (调度+分析)   │    │  (解析库)  │    │  (持久化索引)     │ │
│  └──────────┘    └──────────────┘    └────────────┘    └────────┬─────────┘ │
│                                                                  │           │
│                                                                  ▼           │
│                  ┌──────────────────────────────────────────────────────┐    │
│                  │                  WebUI Layer                          │    │
│                  │  ┌─────────────┐  ┌──────────────┐  ┌────────────┐  │    │
│                  │  │RefGraphSvc  │──│HeapQueryEngine│──│HeapGraph   │  │    │
│                  │  │(HTTP API)   │  │(按需计算)     │  │(接口)      │  │    │
│                  │  └─────────────┘  └──────────────┘  └────────────┘  │    │
│                  └──────────────────────────────────────────────────────┘    │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 核心设计原则

| 原则 | 体现 |
|------|------|
| **分析时计算，展示时读取** | 解析阶段产出完整分析数据写入 heap_index.bin，WebUI 只读取展示 |
| **接口驱动设计** | `HeapGraph` 接口解耦查询层与存储层，支持 v1(全量加载) / v2(mmap) 实现切换 |
| **Two-Pass 避免中间层** | Pass 1 计数 → Pass 2 预分配填充，消除 `map[uint64][]ObjectReference` 中间结构 |
| **CSR 格式最小化内存** | Compressed Sparse Row 紧凑存储，缓存友好，GC 压力为零 |
| **策略模式** | `HeapDataProvider` 接口抽象数据来源，支持 indexed/legacy 双策略 |

---

## 二、数据流：从 HPROF 到 WebUI

```
                    ┌─────────────────────────┐
                    │   .hprof Binary File     │  (2-10 GB)
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  Pass 1: ScanPass        │  perflib/parser/hprof/scan_pass.go
                    │  - 顺序扫描 HPROF records │
                    │  - 统计对象/边/GC Root    │
                    │  - 记录 Segment offsets   │
                    │  - 提取 class/field 信息  │
                    │  Output: ScanResult       │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  Pass 2: BuildPass       │  build_pass.go / build_pass_parallel.go
                    │  - 基于 ScanResult 预分配 │
                    │  - 提取引用目标，填充 CSR  │
                    │  - 并行模式: 多 Worker     │
                    │    分别解析不同 Segment    │
                    │  Output:                  │
                    │    IndexedReferenceGraph   │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  Dominator Computation    │  dom_indexed.go / dom_hierarchical.go
                    │  - Lengauer-Tarjan 算法   │
                    │  - 层次并行优化            │
                    │  - retained size 计算     │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  Serialize to File        │  index_writer.go / index_writer_v2.go
                    │  - heap_index.bin v1/v2   │
                    │  - CSR + metadata + zstd  │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  Deserialize             │  index_reader.go / index_reader_v2.go
                    │  - 自动检测 v1/v2         │
                    │  - v1: bufio 全量加载     │
                    │  - v2: mmap 零拷贝映射    │
                    │  Output: HeapGraph 接口   │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  HeapQueryEngine          │  internal/webui/heap_query_engine.go
                    │  - 最大对象查询           │
                    │  - GC Root 路径 (BFS)     │
                    │  - Retainer 分析          │
                    │  - Class 直方图           │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  RefGraphService          │  internal/webui/refgraph_service.go
                    │  - HTTP API 路由          │
                    │  - LRU 缓存管理           │
                    │  - JSON 序列化响应        │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  Frontend (HTML + JS)     │  internal/webui/static/
                    │  - 火焰图/直方图/引用图   │
                    │  - Alpine.js + Tailwind   │
                    └─────────────────────────┘
```

---

## 三、核心组件详解

### 3.1 Two-Pass CSR 解析引擎

#### 为什么用 Two-Pass？

传统方案使用 `map[uint64][]ObjectReference` 存储引用关系：
- 16M 对象 × 19M 边 → **内存 ~7-8 GB**（map overhead + slice header + 装箱）
- 大量小 slice 分配 → GC 压力巨大
- 随机访问 map → 缓存不友好

Two-Pass + CSR 方案：
- Pass 1 只计数：获得精确的 `objectCount` 和每个节点的 `degreeCount`
- Pass 2 一次性预分配平坦数组，顺序填充
- **内存降至 ~500MB**（无 map、无 slice header、无 GC 扫描）

#### Pass 1: ScanPass

```go
// perflib/parser/hprof/scan_pass.go
func (p *Parser) ScanPass(ctx context.Context, r io.ReadSeeker) (*ScanResult, error)
```

职责：
1. 解析 HPROF file header（获取 ID size）
2. 顺序遍历所有 records（HEAP_DUMP_SEGMENT 中的 sub-tags）
3. 收集：对象 ID/classID/size、class 定义、字符串池、GC Root 列表
4. 统计每个对象的出/入度（`DegreeCounts`），用于 Pass 2 预分配
5. 记录 `SegmentInfo`（大 Segment 的文件偏移），用于并行 Build Pass

#### Pass 2: BuildPass

```go
// 顺序版
func (p *Parser) BuildPass(ctx context.Context, r io.ReadSeeker, scan *ScanResult) (*IndexedReferenceGraph, error)

// 并行版（自动检测：Segment 数 ≥ 4 && io.ReaderAt 可用）
func (p *Parser) BuildPassParallel(ctx context.Context, ra io.ReaderAt, scan *ScanResult) (*IndexedReferenceGraph, error)
```

并行策略：
- 将 HEAP_DUMP_SEGMENT 按 ~64MB 划分为虚拟 chunk
- N 个 Worker goroutine 各自通过 `io.SectionReader` 独立解析指定区间
- 每个 Worker 产出本地 `[]edgeRecord`
- 主线程合并到全局 `CompactEdgeListBuilder`

#### ParseTwoPass 入口

```go
// perflib/parser/hprof/build_pass.go
func (p *Parser) ParseTwoPass(ctx context.Context, r io.ReadSeeker) (*IndexedReferenceGraph, error)
```

自动选择路径：
1. `ScanPass(r)` → `ScanResult`
2. 检测 `r` 是否实现 `io.ReaderAt` 且 Segment 数 ≥ 4
3. 是 → `BuildPassParallel(r, scan)` + merge
4. 否 → `BuildPass(r, scan)`
5. `ComputeDominatorForIndexedGraph(graph)`
6. 返回完整的 `IndexedReferenceGraph`

---

### 3.2 CSR 数据结构

**Compressed Sparse Row (CSR)** 是图计算领域标准的稀疏图存储格式：

```
offsets:  [0, 2, 3, 5, 5, ...]    // len = N+1，节点 i 的边范围: [offsets[i], offsets[i+1])
targets:  [3, 7, 1, 4, 9, ...]    // len = E，第 j 条边的目标节点索引
fieldIDs: [0, 1, 2, 0, 3, ...]    // len = E，第 j 条边的字段名 ID
classIDs: [100, 101, ...]          // len = E，第 j 条边的源/目标 classID
```

**优势**：
| 特性 | 说明 |
|------|------|
| O(1) 访问 | 节点 i 的所有边 = `targets[offsets[i]:offsets[i+1]]` |
| 内存连续 | 一个平坦 `[]int32`，CPU 缓存友好 |
| 零 GC 压力 | 无指针、无 slice-of-slice、无 map |
| 大小可控 | 16M 对象 × 19M 边 ≈ 500MB（vs 7-8GB） |

**Go 实现**（`CompactEdgeList`）：

```go
type CompactEdgeList struct {
    offsets    []int32           // 节点偏移表 (N+1)
    targets    []int32           // 边目标索引 (E)
    fieldIDs   []int32           // 字段名 ID (E)
    classIDs   []uint64          // 引用的 classID (E)
    fieldNames []string          // 字段名字符串池
    fieldToID  map[string]int32  // 字段名去重 intern
    nodeCount  int32
    edgeCount  int32
}
```

---

### 3.3 IndexedReferenceGraph

整个 heap dump 分析结果的核心数据结构：

```go
type IndexedReferenceGraph struct {
    objects            *IndexedObjectStore   // 对象属性（平行数组）
    outgoing           *CompactEdgeList      // CSR 出边
    incoming           *CompactEdgeList      // CSR 入边
    classNames         map[uint64]string     // classID → 类名
    gcRoots            []GCRoot              // GC Root 列表
    gcRootBits         *Bitset               // 哪些对象是 GC Root
    classObjectBits    *Bitset               // 哪些对象是 java.lang.Class 实例
    reachableBits      *Bitset               // 哪些对象从 GC Root 可达
    dominatorComputed  bool
}
```

**IndexedObjectStore** 内部使用 **int32 索引** 替代 uint64 objectID：
- `objToIdx map[uint64]int32` — objectID → 连续索引
- `idxToObj []uint64` — 索引 → objectID
- `classIDs []uint64` — 每个对象的 classID
- `shallowSizes []int64` — shallow size
- `retainedSizes []int64` — retained size（dominator 计算后填充）
- `dominators []int32` — dominator 树父节点

---

### 3.4 HeapGraph 接口

```go
// perflib/parser/hprof/heap_graph.go
type HeapGraph interface {
    ObjectCount() int32
    GetObjectIndex(objectID uint64) int32
    GetObjectID(idx int32) uint64
    GetClassID(idx int32) uint64
    GetClassName(classID uint64) string
    GetShallowSize(idx int32) int64
    GetRetainedSize(idx int32) int64
    GetDominator(idx int32) int32
    IsGCRoot(idx int32) bool
    IsReachable(idx int32) bool
    IsClassObject(idx int32) bool
    GetOutgoingEdges(idx int32) (targets []int32, fieldIDs []int32, classIDs []uint64)
    GetIncomingEdges(idx int32) (sources []int32, fieldIDs []int32, classIDs []uint64)
    GetObjectsByClass(classID uint64) []int32
    GetFieldName(fieldID int32) string
    GetGCRoots() []GCRoot
}
```

**实现者**：
| 实现 | 文件 | 特点 |
|------|------|------|
| `*IndexedReferenceGraph` | `graph_indexed.go` | 全量加载到内存，v1 格式 |
| `*MmapHeapIndex` | `index_reader_v2.go` | mmap 零拷贝，v2 格式，按需 page fault |

**消费者**：
- `HeapQueryEngine` — 仅依赖 `HeapGraph`，实现可测试、可替换

---

### 3.5 heap_index.bin 文件格式

#### v1 格式（顺序流式）

```
[Header: 40B]
  Magic("HPIX") + Version(1) + ObjectCount + EdgeCount + Flags + ClassCount + GCRootCount

[SectionHeader + ObjectStore]
  objIDs[N] + classIDs[N] + shallowSizes[N] + retainedSizes[N]

[SectionHeader + OutEdges]
  nodeCount + edgeCount + offsets[N+1] + targets[E] + fieldIDs[E] + classIDs[E]

[SectionHeader + InEdges]  (optional, FlagHasInEdges)

[SectionHeader + DominatorTree]  (optional, FlagHasDominator)
  dominators[N]

[SectionHeader + Bitsets]  (optional, FlagHasBitsets)
  numBitsets + [size + wordCount + words[]] × 3

[SectionHeader + Metadata]  (zstd compressed)
  classNames + fieldNames + gcRoots
```

#### v2 格式（mmap 友好）

```
[Header: 48B]
  Magic("HPIX") + Version(2) + ObjectCount + EdgeCount + InEdgeCount + Flags
  + NumSections + ClassCount + GCRootCount

[Section Table: 16B × 6]
  每个 entry: (Type: uint32, Reserved: uint32, Offset: int64)

[Padding to 4096-byte alignment]

[Section 1: ObjectStore]  — page-aligned, mmap 直接映射
[Section 2: OutEdges]     — CSR 格式
[Section 3: InEdges]      — CSR 格式 (optional)
[Section 4: DominatorTree] — dominators[N] (optional)
[Section 5: Bitsets]       — 常驻内存 (<5MB)
[Section 6: Metadata]      — zstd 压缩 (classNames + fieldNames + gcRoots)
```

**v2 的 mmap 优势**：
- Section Table 支持随机定位任意 section
- 数据数组通过 `unsafe.Slice` 创建零拷贝视图
- OS page cache 管理物理内存，热数据自动驻留
- 常驻内存从 ~1.2GB 降至 ~330MB

---

### 3.6 HeapQueryEngine

```go
// internal/webui/heap_query_engine.go
type HeapQueryEngine struct {
    graph     hprof.HeapGraph
    assembler *hprof.ObjectInfoAssembler
    // 懒加载索引
    classNameToID     map[string]uint64
    classNameToIDOnce sync.Once
}
```

提供的查询 API：

| 方法 | 功能 |
|------|------|
| `QueryBiggestObjects(topN, sortBy, classFilter)` | Top-N 大对象（按 retained/shallow size 排序） |
| `QueryGCRootPath(objectID, maxPaths, maxDepth)` | BFS 查找从 GC Root 到目标的引用路径 |
| `QueryRetainers(objectID, maxRetainers)` | 谁持有该对象（incoming edges 递归） |
| `QueryObjectFields(objectID)` | 对象的字段引用（outgoing edges） |
| `QueryGCRootsSummary()` | GC Root 按类型/类名分组统计 |
| `QueryObjectInfo(objectID)` | 单个对象的完整信息 |
| `QueryClassInstances(className, topN, sortBy)` | 按类名查询实例列表 |
| `QueryDominatorChildren(objectID, topN, sortBy)` | 支配树子节点查询 |
| `QueryDominatorPath(objectID)` | 从对象到 GC Root 的支配树路径 |
| `QueryRetainedSizeTreemap(objectID, maxNodes)` | Retained Size Treemap 数据 |
| `GetGraph()` | 获取底层 HeapGraph 接口（供外部查询工具使用） |

---

### 3.7 WebUI 服务层

```go
// internal/webui/refgraph_service.go
type RefGraphService struct {
    dataDir      string
    cache        map[string]*heapCacheEntry  // taskID → provider (LRU)
    maxCacheSize int
}
```

**数据提供策略**：

```
RefGraphService
    │
    ├── indexedProvider (优先)
    │   └── HeapQueryEngine + HeapGraph (heap_index.bin)
    │
    └── legacyProvider (降级)
        └── 直接读取旧格式 JSON 文件
```

**HTTP API 路由**：

| 分组 | Endpoint | 描述 |
|------|----------|------|
| 通用 | `GET /api/summary` | 分析结果摘要 |
| 通用 | `GET /api/flamegraph` | 火焰图数据 |
| 通用 | `GET /api/callgraph` | 调用图数据 |
| 通用 | `GET /api/tasks` | 任务列表 |
| 通用 | `GET /api/search` | 全文搜索 |
| Heap (顶层) | `GET /api/biggest-objects` | 最大对象列表 |
| Heap (顶层) | `GET /api/retainers` | 对象持有者分析 |
| Heap (顶层) | `GET /api/object-fields` | 对象字段详情 |
| Heap (RefGraph) | `GET /api/refgraph/fields` | 引用图字段 |
| Heap (RefGraph) | `GET /api/refgraph/info` | 对象元信息 |
| Heap (RefGraph) | `GET /api/refgraph/gc-roots` | GC Root 引用路径 |
| Heap (RefGraph) | `GET /api/refgraph/gc-roots-summary` | GC Root 统计 |
| Heap (RefGraph) | `GET /api/refgraph/gc-roots-list` | GC Root 列表 |
| Heap (RefGraph) | `GET /api/refgraph/gc-root-retained` | GC Root Retained Size |
| Heap (RefGraph) | `GET /api/refgraph/retainers` | 引用图 Retainer 查询 |
| Heap (RefGraph) | `GET /api/refgraph/biggest-by-class` | 按类名查最大实例 |
| Heap (RefGraph) | `GET /api/refgraph/dominator-tree` | 支配树子节点 |
| Heap (RefGraph) | `GET /api/refgraph/dominator-path` | 支配树路径 |
| Heap (RefGraph) | `GET /api/refgraph/treemap` | Retained Size Treemap |
| Heap (RefGraph) | `GET /api/refgraph/class-histogram` | 类直方图 |
| Heap (RefGraph) | `GET /api/refgraph/heap-stats` | 堆统计数据 |
| Heap | `GET /api/heap/histogram` | 堆直方图（统一入口） |
| pprof | `GET /api/pprof/leak-report` | 泄漏报告（旧，需 batch） |
| pprof | `GET /api/pprof/batch-analysis` | 批量分析数据 |
| Leak | `GET /api/leak-suspects` | 统一泄漏检测（新） |
| Goroutine | `GET /api/goroutine/groups` | Goroutine 分组 |
| Goroutine | `GET /api/goroutine/stats` | Goroutine 统计 |
| Goroutine | `GET /api/goroutine/issues` | Goroutine 问题检测 |

---

### 3.8 LeakSuspect Provider 架构

统一泄漏检测系统，基于 **策略模式 + Provider Chain**，支持多种检测策略产出一致的结果模型。

```
perflib/query/
├── leak_suspect.go              ← 统一模型 + Engine
├── leak_suspect_timeseries.go   ← TimeSeriesLeakProvider
└── leak_suspect_hprof.go        ← HprofSnapshotLeakProvider
```

#### 统一数据模型

```go
// perflib/query/leak_suspect.go
type LeakSuspect struct {
    Type        string        // 泄漏类别 (heap/goroutine/class_accumulation...)
    Source      LeakSource    // 检测来源 (time_series/snapshot_heuristic/static_analysis)
    Severity    LeakSeverity  // 严重程度 (info/warning/critical)
    Title       string        // 一行摘要
    Description string        // 详细说明
    Evidence    []LeakEvidence
    Metrics     *LeakMetrics
    Suggestions []string
}
```

#### Provider 接口

```go
type LeakSuspectProvider interface {
    Name() string
    CanDetect(outputDir string) bool
    Detect(outputDir string) ([]LeakSuspect, error)
}
```

#### Engine 编排

```go
type LeakSuspectEngine struct {
    providers []LeakSuspectProvider
}

func (e *LeakSuspectEngine) Detect(outputDir string) *LeakSuspectsResult {
    // 遍历所有 Provider → CanDetect() → Detect() → 聚合 + 排序
}
```

#### 当前 Provider 实现

| Provider | 数据来源 | 检测策略 |
|----------|---------|---------|
| `TimeSeriesLeakProvider` | `batch_analysis.json` | 比较多次 profile 的增长趋势 |
| `HprofSnapshotLeakProvider` | `heap_stats.json` | 单快照启发式（3 条规则） |

**HprofSnapshotLeakProvider 启发式规则**：

| 规则 | 检测目标 | 阈值 |
|------|---------|------|
| DominantClassRule | 单类占堆比例过高 | >25% warning, >40% critical |
| CollectionAccumulationRule | 集合类实例数异常 | >50K info, >100K warning, >500K critical |
| ClassLoaderLeakRule | ClassLoader 泄漏 | >30K info, >50K warning, >80K critical |

#### 数据流

```
分析时（Analyzer）：
  java_heap_analyzer / pprof_batch_analyzer
      → LeakSuspectEngine.Detect(outputDir)
      → 写入 leak_suspects.json（预计算）

服务时（WebUI）：
  /api/leak-suspects
      → 读取 leak_suspects.json（快路径）
      → 若不存在，运行 Provider Chain（降级路径）
```

---

## 四、性能数据

### 解析性能（Apple M3 Pro, 2.45 GB hprof, 16M 对象, 34M 边）

| 阶段 | 耗时 | 说明 |
|------|------|------|
| Scan Pass | 5.5s | 顺序扫描，统计元数据 |
| Build Pass (并行, 14 workers) | 6.6s | 提取引用 + 构建 CSR |
| Dominator Tree | 1.5s | Lengauer-Tarjan + retained size |
| WriteHeapIndex | 1.0s | 序列化到磁盘 |
| **总计** | **~16s** | 目标 <45s ✅ |

### 内存使用

| 方案 | 峰值内存 |
|------|---------|
| 旧方案（map） | ~7-8 GB |
| Two-Pass CSR (v1) | ~1.2 GB |
| v2 mmap | ~330 MB (常驻) |

### 查询性能（heap_index.bin 加载后）

| 操作 | 延迟 |
|------|------|
| 加载 heap_index.bin (v1) | ~2s |
| 加载 heap_index.bin (v2, mmap) | <500ms |
| BiggestObjects Top-100 | <50ms |
| GC Root Path (BFS) | <100ms |
| Retainer 查询 | <30ms |
| 单对象信息 | <1ms |

---

## 五、扩展点

### 新增 Heap 分析能力

1. 实现新的分析算法 → 通过 `HeapGraph` 接口读取数据
2. 在 `HeapQueryEngine` 中添加新方法
3. 在 `RefGraphService` 中注册新的 HTTP 路由

### 新增存储后端

1. 实现 `HeapGraph` 接口
2. 在 `ReadHeapIndex` 中添加版本分发
3. 对应的 Writer（如有序列化需求）

### 新增数据展示

1. 在 `HeapDataProvider` 接口中添加方法
2. `indexedProvider` 和 `legacyProvider` 分别实现
3. 前端新增对应的视图组件

### 新增泄漏检测策略

1. 实现 `LeakSuspectProvider` 接口（`Name()`、`CanDetect()`、`Detect()`）
2. 在 `LeakSuspectEngine` 的 providers 列表中注册
3. 无需修改 API 层 — 所有 Provider 的输出自动聚合到 `/api/leak-suspects`

示例：新增 Goroutine 泄漏检测

```go
// perflib/query/leak_suspect_goroutine.go
type GoroutineLeakProvider struct{}

func (p *GoroutineLeakProvider) Name() string { return "goroutine_snapshot" }

func (p *GoroutineLeakProvider) CanDetect(outputDir string) bool {
    // 检查 goroutine_analysis.json 是否存在
}

func (p *GoroutineLeakProvider) Detect(outputDir string) ([]LeakSuspect, error) {
    // 分析 goroutine 数据，产出 []LeakSuspect
}
```

---

## 六、关键设计决策记录

| 决策 | 选项 | 选择 | 原因 |
|------|------|------|------|
| 图存储格式 | CSR vs Adjacency List vs Map | CSR | 内存最小、缓存友好、O(1) 访问 |
| 对象索引 | uint64 objectID vs int32 index | int32 index | 指针宽度减半，适合平坦数组 |
| 解析策略 | Single-Pass vs Two-Pass | Two-Pass | 消除中间 map，支持预分配 |
| 文件格式 v2 | Section Table vs 固定布局 | Section Table | 支持 mmap 随机定位 |
| 查询接口 | 具体类型 vs interface | HeapGraph interface | 解耦、可测试、支持多实现 |
| Dominator 算法 | Simple vs Lengauer-Tarjan | Lengauer-Tarjan | O(E·α(N))，大图性能稳定 |
| 并行 Build | Worker-per-chunk vs Pipeline | Worker-per-chunk | 实现简单，利用 SectionReader 独立性 |
| metadata 压缩 | gzip vs zstd | zstd | 更快的解压速度（2-3x） |
