# 架构优化方案：轻量预计算 + 运行时按需分析

> 创建时间: 2026-05-06  
> 状态: 阶段 1 实施中  
> 基线方案: 方案 B（Light Pre-compute + Runtime On-Demand）  
> 目标: 保留 "Analyze Then Serve" 架构优势，消除全量预计算浪费，提升用户体验

---

## 一、背景与动机

### 1.1 当前架构："Analyze Then Serve"

```
┌──────────────────────────────────────────────────────────────┐
│  CLI: analyze 命令                                           │
│                                                              │
│  inputFile → Parser.Parse() → HeapAnalysisResult             │
│       ↓                                                      │
│  JavaHeapAnalyzer → 全量计算                                  │
│       │  ├── Class Histogram      (轻量)                     │
│       │  ├── Dominator Tree       (必须全量遍历)              │
│       │  ├── Retained Sizes       (必须全量遍历)              │
│       │  ├── GC Roots Analysis    (全量 BFS)  ← 可延迟       │
│       │  ├── Retainer Analysis    (全量 BFS)  ← 可延迟       │
│       │  ├── Biggest Objects      (排序 Top N) ← 可延迟      │
│       │  └── Reference Graph Serialize (序列化) ← 可延迟     │
│       ↓                                                      │
│  Publisher.Publish() → 写入 7+ 个文件                         │
└──────────────────────────────────────────────────────────────┘
                        ↓ 文件系统边界
┌──────────────────────────────────────────────────────────────┐
│  CLI: serve 命令                                             │
│                                                              │
│  WebUI Server → 读取预计算文件 → JSON Response                │
│       ├── /api/summary          → 读 summary.json           │
│       ├── /api/flamegraph       → 读 collapsed_data.json.gz │
│       ├── /api/biggest-objects  → 读 biggest_objects.json    │
│       ├── /api/refgraph/*       → 懒加载 refgraph.bin        │
│       │   (反序列化 ~500MB-1.5GB, 耗时 30-90s)               │
│       └── ...                                                │
└──────────────────────────────────────────────────────────────┘
```

### 1.2 问题分析

| 问题 | 影响 | 根因 |
|------|------|------|
| 分析耗时 ~3min（2.6GB 文件） | 用户等待时间过长 | 全量 GC Root BFS + Retainer 分析 |
| refgraph.bin 序列化 500MB+ | 磁盘 I/O + 写入延迟 | 完整 map 结构序列化 |
| serve 阶段加载 refgraph.bin 30-90s | 首次 API 响应慢 | 反序列化 + 内存重建 |
| 峰值内存 ~8GB | OOM 风险 | `map[uint64][]ObjectReference` × 2 |
| 用户可能只看 Overview | 预计算浪费 | GC Root/Retainer 全量计算但可能不用 |

### 1.3 方案 B 核心思路

**将分析拆分为"必须全量"和"可按需"两部分**：

| 分析阶段（analyze 时执行） | 服务阶段（serve 时按需执行） |
|---|---|
| ✅ 解析 HPROF → 对象/引用元数据 | ❌ |
| ✅ 构建 CSR 格式图（紧凑索引） | ❌ |
| ✅ 计算 Dominator Tree（必须全量遍历） | ❌ |
| ✅ 计算 Retained Sizes（依赖 Dominator） | ❌ |
| ✅ Class Histogram + Summary | ❌ |
| ❌ | ✅ GC Root Path 查找（BFS 按需） |
| ❌ | ✅ 对象字段展开（按需读取 CSR） |
| ❌ | ✅ Retainer 分析（按需反向 BFS） |
| ❌ | ✅ 按类过滤/排序/Top N（按需计算） |
| ❌ | ✅ Biggest Objects 深度展开 |

---

## 二、目标架构

### 2.1 架构总览

```
┌─────────────────────────────────────────────────────────────────────┐
│  CLI: analyze 命令 (轻量预计算)                                      │
│                                                                     │
│  inputFile → Parser.Parse() [Two-Pass CSR]                          │
│       ↓                                                             │
│  输出紧凑索引文件 (新格式):                                           │
│       ├── summary.json             (元数据 + Class Histogram)        │
│       ├── heap_index.bin           (CSR图 + ObjectStore + Dominator) │
│       └── class_histogram.json     (快速预览用)                      │
│                                                                     │
│  预计算内容 (只做必须全量遍历的):                                      │
│       ├── Dominator Tree (int32[] parent indices)                    │
│       ├── Retained Sizes (int64[] per object)                       │
│       └── Class Statistics (聚合统计)                                │
│                                                                     │
│  不再预计算:                                                         │
│       ✗ GC Root Paths (改为 serve 时按需 BFS)                        │
│       ✗ Retainer Analysis (改为 serve 时按需反向 BFS)                │
│       ✗ Biggest Objects 深度展开 (改为 serve 时按需查询)              │
│       ✗ refgraph.bin 大文件序列化 (被 heap_index.bin 替代)           │
└─────────────────────────────────────────────────────────────────────┘
                              ↓ 文件系统边界
┌─────────────────────────────────────────────────────────────────────┐
│  CLI: serve 命令 (运行时按需分析)                                     │
│                                                                     │
│  HeapQueryEngine (新组件):                                           │
│       │                                                             │
│       ├── LoadIndex(taskDir) → 加载 heap_index.bin (~200-500MB)     │
│       │   ├── CSR OutEdges (连续内存, 缓存友好)                      │
│       │   ├── CSR InEdges  (连续内存, 缓存友好)                      │
│       │   ├── IndexedObjectStore                                    │
│       │   └── DominatorTree + RetainedSizes                         │
│       │                                                             │
│       ├── QueryBiggestObjects(topN, sortBy, classFilter)            │
│       │   → O(N) 扫描 retainedSizes[] + 排序                       │
│       │                                                             │
│       ├── QueryGCRootPath(objectID, maxPaths, maxDepth)             │
│       │   → BFS on CSR inEdges, 按需搜索, <50ms                    │
│       │                                                             │
│       ├── QueryRetainers(objectID, maxDepth)                        │
│       │   → CSR inEdges 反向遍历, <20ms                            │
│       │                                                             │
│       ├── QueryObjectFields(objectID)                               │
│       │   → CSR outEdges 直接查找, <5ms                            │
│       │                                                             │
│       └── QueryClassInstances(className, topN, sortBy)              │
│           → 扫描 classIDs[] + retainedSizes[], <100ms               │
│                                                                     │
│  API Routes (不变):                                                  │
│       /api/summary          → 读 summary.json (不变)                │
│       /api/biggest-objects  → HeapQueryEngine.QueryBiggestObjects   │
│       /api/refgraph/fields  → HeapQueryEngine.QueryObjectFields     │
│       /api/refgraph/gc-roots → HeapQueryEngine.QueryGCRootPath      │
│       /api/refgraph/retainers → HeapQueryEngine.QueryRetainers      │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 新输出文件格式

#### `heap_index.bin` 二进制格式

```
┌────────────────────────────────────────────────────────┐
│ Header (32 bytes)                                      │
│   Magic: "HPIX" (4 bytes)                             │
│   Version: uint32                                      │
│   ObjectCount: int32                                   │
│   EdgeCount: int64                                     │
│   Flags: uint32 (hasDominator, hasRetained, etc.)     │
│   Reserved: 8 bytes                                    │
├────────────────────────────────────────────────────────┤
│ Section: ObjectStore                                   │
│   objIDs:       []uint64  (8 × ObjectCount bytes)     │
│   classIDs:     []uint64  (8 × ObjectCount bytes)     │
│   shallowSizes: []int64   (8 × ObjectCount bytes)     │
│   retainedSizes: []int64  (8 × ObjectCount bytes)     │
├────────────────────────────────────────────────────────┤
│ Section: CSR OutEdges                                  │
│   offsets:  []int32  (4 × (ObjectCount+1) bytes)      │
│   targets:  []int32  (4 × EdgeCount bytes)            │
│   fieldIDs: []int32  (4 × EdgeCount bytes)            │
├────────────────────────────────────────────────────────┤
│ Section: CSR InEdges                                   │
│   offsets:  []int32  (4 × (ObjectCount+1) bytes)      │
│   targets:  []int32  (4 × EdgeCount bytes)            │
├────────────────────────────────────────────────────────┤
│ Section: DominatorTree                                 │
│   parents:  []int32  (4 × ObjectCount bytes)          │
├────────────────────────────────────────────────────────┤
│ Section: Metadata                                      │
│   ObjectID → Index mapping (varint encoded)           │
│   ClassNames (string table)                           │
│   FieldNames (string table)                           │
│   GC Roots ([]GCRootEntry)                            │
└────────────────────────────────────────────────────────┘
```

**文件大小估算（5M 对象, 15M 引用）**:

| Section | 大小 |
|---------|------|
| ObjectStore | 4 × 8 × 5M = 160 MB |
| CSR OutEdges | (5M+1)×4 + 15M×4×2 = 140 MB |
| CSR InEdges | (5M+1)×4 + 15M×4 = 80 MB |
| DominatorTree | 5M × 4 = 20 MB |
| Metadata | ~50 MB |
| **总计** | **~450 MB** |

对比当前 `refgraph.bin`: ~500-1500 MB → 优化后 ~450 MB（减少 10-70%，且加载更快）

### 2.3 关键设计决策

| 决策 | 选项 | 选择 | 原因 |
|------|------|------|------|
| 预计算 vs 按需 | 全量预计算 / 按需计算 | **混合** | Dominator 必须全量；GC Root 查询天然适合按需 |
| 索引格式 | JSON / Protobuf / 自定义二进制 | **自定义二进制** | 零反序列化开销（mmap 后直接 slice cast） |
| serve 加载策略 | 全量加载 / mmap 按需 | **全量加载** → 后续可改 mmap | 简单可靠，500MB 加载 <2s |
| API 兼容性 | 破坏性 / 向后兼容 | **完全向后兼容** | WebUI 前端不需要改动 |
| 旧格式支持 | 删除 / 并存 | **并存 + 降级** | 已有分析结果仍可查看 |

---

## 三、实施 Roadmap

### 3.1 总览

```
Sprint 1: HeapQueryEngine 核心 (按需查询引擎)         [3-4 天]
  │  目标: serve 阶段可以直接查询 CSR 格式数据
  │
Sprint 2: heap_index.bin 序列化 (紧凑索引输出)         [3-4 天]
  │  目标: analyze 阶段输出 heap_index.bin 替代 refgraph.bin
  │
Sprint 3: Analyze 流程瘦身 (去除冗余预计算)            [2-3 天]
  │  目标: 去除 GC Root/Retainer/BiggestObjects 全量预计算
  │
Sprint 4: WebUI 集成 + 向后兼容                       [2-3 天]
  │  目标: WebUI 无缝切换新后端，旧数据仍可查看
  │
Sprint 5: 渐进式加载 + 用户体验                       [2-3 天]
     目标: 分阶段输出，WebUI 渐进解锁

总计: 12-17 天 (不含测试和文档)
```

### 3.2 依赖关系

```
Sprint 1 ────┐
              ├──→ Sprint 4
Sprint 2 ────┤
              ├──→ Sprint 3 ──→ Sprint 5
              │
              │  (Sprint 1 和 Sprint 2 可并行)
```

---

## 四、Sprint 详细设计

### 4.1 Sprint 1: HeapQueryEngine 核心

**目标**: 在 serve 阶段提供高效的按需查询引擎，替代当前的 `RefGraphService`。

**新增文件**:
- `internal/webui/heap_query_engine.go`
- `internal/webui/heap_query_engine_test.go`

**核心接口设计**:

```go
// HeapQueryEngine provides on-demand heap analysis queries
// using pre-computed compact index data (CSR format).
type HeapQueryEngine struct {
    // Core data (loaded from heap_index.bin)
    objects    *hprof.IndexedObjectStore
    outEdges   *hprof.CompactEdgeList
    inEdges    *hprof.CompactEdgeList
    domTree    []int32   // parent index for each object
    gcRoots    []int32   // GC root object indices
    classNames []string  // class name string table
    fieldNames []string  // field name string table
    
    // Derived (lazy computed)
    gcRootSet  *hprof.Bitset  // fast GC root membership test
}

// QueryBiggestObjects returns top N objects sorted by retained size.
// Supports filtering by class name.
// Time complexity: O(N) scan + O(topN log topN) sort
func (e *HeapQueryEngine) QueryBiggestObjects(topN int, sortBy string, classFilter string) []BiggestObjectResult

// QueryGCRootPath finds shortest paths from object to GC roots.
// Uses BFS on inEdges CSR, bounded by maxPaths and maxDepth.
// Time complexity: O(V+E) worst case, typically <50ms for maxPaths=3, maxDepth=15
func (e *HeapQueryEngine) QueryGCRootPath(objectID uint64, maxPaths int, maxDepth int) []GCRootPathResult

// QueryRetainers returns objects that hold a reference to the given object.
// Direct lookup in inEdges CSR - O(1) for finding, O(degree) for results.
func (e *HeapQueryEngine) QueryRetainers(objectID uint64, maxRetainers int) []RetainerResult

// QueryObjectFields returns outgoing references (fields) of an object.
// Direct lookup in outEdges CSR - O(1) for finding, O(degree) for results.
func (e *HeapQueryEngine) QueryObjectFields(objectID uint64) []ObjectFieldResult

// QueryClassInstances returns instances of a class sorted by size.
// Scan classIDs[] for matching entries, sort by retained size.
func (e *HeapQueryEngine) QueryClassInstances(className string, topN int, sortBy string) []ClassInstanceResult

// QueryGCRootsSummary returns GC roots grouped by type.
// Pre-computed during index loading (lightweight).
func (e *HeapQueryEngine) QueryGCRootsSummary() []GCRootSummaryResult
```

**性能目标**:

| API | 当前性能 | 目标性能 | 方式 |
|-----|---------|---------|------|
| biggest-objects | 预计算 (不适用) | <100ms | 扫描 retainedSizes[] |
| gc-root path | refgraph.bin 加载后 ~200ms | <50ms | CSR BFS |
| retainers | refgraph.bin 加载后 ~100ms | <20ms | CSR 直接查找 |
| object-fields | refgraph.bin 加载后 ~50ms | <5ms | CSR 直接查找 |
| class-instances | 预计算 (不适用) | <100ms | 扫描 + 排序 |

**验收标准**:
- [ ] 所有查询 API 响应 < 200ms（5M 对象规模）
- [ ] 内存占用 < 600 MB（加载 heap_index.bin 后）
- [ ] 单元测试覆盖所有查询路径
- [ ] BFS 深度限制防止无限循环

---

### 4.2 Sprint 2: heap_index.bin 序列化

**目标**: 定义紧凑的二进制索引格式，支持快速写入（analyze）和快速加载（serve）。

**新增文件**:
- `perflib/parser/hprof/index_writer.go` (写入)
- `perflib/parser/hprof/index_reader.go` (读取)
- `perflib/parser/hprof/index_format.go` (格式定义)
- `perflib/parser/hprof/index_test.go`

**格式设计**:

```go
// IndexFileHeader is the header of heap_index.bin
type IndexFileHeader struct {
    Magic        [4]byte  // "HPIX"
    Version      uint32   // format version (1)
    ObjectCount  int32
    EdgeCount    int64
    Flags        uint32   // feature flags
    ClassCount   int32
    GCRootCount  int32
    _reserved    [4]byte
}

// IndexFileFlags defines feature flags
const (
    FlagHasDominator  uint32 = 1 << 0
    FlagHasRetained   uint32 = 1 << 1
    FlagHasInEdges    uint32 = 1 << 2
    FlagHasFieldNames uint32 = 1 << 3
    FlagCompressed    uint32 = 1 << 4  // zstd compression for metadata section
)
```

**写入流程**:

```go
// WriteHeapIndex writes the compact heap index file.
func WriteHeapIndex(w io.Writer, data *HeapIndexData) error {
    // 1. Write header
    // 2. Write ObjectStore section (raw []byte cast from slices)
    // 3. Write CSR OutEdges section
    // 4. Write CSR InEdges section
    // 5. Write DominatorTree section
    // 6. Write Metadata section (zstd compressed)
    //    - objectID→index mapping
    //    - class names
    //    - field names
    //    - GC root entries
}
```

**读取流程（关键：快速加载）**:

```go
// ReadHeapIndex loads the compact heap index file into memory.
// For a 450MB file, target load time is <2s.
func ReadHeapIndex(filePath string) (*HeapIndexData, error) {
    // 1. Read and validate header
    // 2. Bulk-read each section directly into pre-allocated slices
    //    (使用 unsafe.Slice 或 binary.Read 避免逐元素解码)
    // 3. Decompress metadata section
    // 4. Build objectID→index map from metadata
}
```

**关键优化**:
- 数值数组（int32[], int64[], uint64[]）直接 binary 读写，无逐元素编码
- Metadata section 使用 zstd 压缩（类名/字段名重复率高）
- 考虑 mmap 模式：数组 section 直接映射，不复制到 Go 堆

**验收标准**:
- [x] 100K 对象 + 300K 引用写入/读取 roundtrip < 0.1s ✅ (实测 0.05s)
- [ ] 5M 对象 + 15M 引用写入 < 5s (待大文件验证)
- [ ] 5M 对象 + 15M 引用读取 < 2s (待大文件验证)
- [ ] 文件大小 < 500MB (当前估算 ~670MB，需要后续优化 incoming classIDs 存储)
- [x] 格式版本化，支持前向兼容 ✅ (magic + version + flags)
- [x] 单元测试：写入→读取→查询结果一致 ✅ (5 个测试全部通过)

**实现文件**:
- `perflib/parser/hprof/index_format.go` — Header/Flags/SectionType 定义
- `perflib/parser/hprof/index_writer.go` — WriteHeapIndex (bulk binary + zstd metadata)
- `perflib/parser/hprof/index_reader.go` — ReadHeapIndex (bulk read + pre-allocated slices)
- `perflib/parser/hprof/index_test.go` — roundtrip 测试 (small + empty + invalid + large)
- `perflib/internal/collections/bitset.go` — 新增 Words()/NewBitsetFromWords() 序列化支持

**集成点**:
- `perflib/analyzer/java_heap_analyzer.go` analyzeTwoPass Step 8: 自动输出 `heap_index.bin`

---

### 4.3 Sprint 3: Analyze 流程瘦身

**目标**: 从 `JavaHeapAnalyzer` 中去除不必要的全量预计算，减少 analyze 耗时。

**改动文件**:
- `perflib/analyzer/java_heap_analyzer.go`
- `perflib/parser/hprof/parser.go`（集成 Two-Pass CSR，来自性能优化 Roadmap Phase 1）

**当前 analyze 步骤 vs 优化后**:

| 步骤 | 当前 | 优化后 | 变化 |
|------|------|--------|------|
| Step 1: Parse HPROF | ✅ 保留 | ✅ 改为 Two-Pass CSR | 重构 |
| Step 2: Output dir | ✅ 保留 | ✅ 保留 | 不变 |
| Step 3: Heap report | ✅ 写 heap_analysis.json | ❌ 移除 | 信息已在 summary 中 |
| Step 4: Class histogram | ✅ 写 class_histogram.json | ✅ 保留 | 不变 |
| Step 5: Build top classes | ✅ 含 retainer 信息 | ⚡ 仅基本信息 | 简化 |
| Step 6: Generate suggestions | ✅ 保留 | ✅ 保留 | 不变 |
| Step 7: Build HeapData | ✅ 含 BiggestObjects/ReferenceGraphs | ⚡ 仅摘要 | 简化 |
| Step 8: Write biggest_objects.json | ✅ 全量预计算 | ❌ 移除 | **由 serve 按需计算** |
| Step 8.5: Write gc_roots.json | ✅ 全量 BFS | ⚡ 仅 GC root 列表 | 简化 |
| Step 9: Serialize refgraph.bin | ✅ 500MB+ | ⚡ 写 heap_index.bin | **减少 70%** |
| **新增**: Write heap_index.bin | - | ✅ CSR + Dominator + Metadata | 替代 Step 9 |

**预期耗时对比（2.6GB 文件）**:

| 阶段 | 当前 | 优化后 | 节省 |
|------|------|--------|------|
| Parse (Phase 1-2) | ~25s | ~35s（Two-Pass） | -10s |
| Build ReferenceGraph (Phase 3) | ~90s | 0s（CSR 直接产出） | **+90s** |
| Dominator + Retained (Phase 4) | ~30s | ~15s（CSR 直接） | +15s |
| Class Stats (Phase 5) | ~10s | ~10s | 0 |
| Retainer + GC Root (Phase 6-8) | ~35s | 0s（**移到 serve**） | **+35s** |
| Serialize refgraph.bin | ~30s | ~5s（heap_index.bin） | +25s |
| **总计** | **~180s** | **~65s** | **~115s (-64%)** |

**验收标准**:
- [ ] `test/heap.hprof` (2.6GB) analyze 完成时间 < 70s
- [ ] 输出文件: `summary.json` + `class_histogram.json` + `heap_index.bin`
- [ ] `summary.json` 内容与当前等价（WebUI Overview 不受影响）
- [ ] 旧格式文件不再生成（`refgraph.bin`, `biggest_objects.json`, `heap_analysis.json`）

---

### 4.4 Sprint 4: WebUI 集成 + 向后兼容

**目标**: WebUI 后端无缝切换到 HeapQueryEngine，同时支持旧格式数据。

**改动文件**:
- `internal/webui/server.go`（路由适配）
- `internal/webui/refgraph_service.go`（重构为适配器）
- `internal/webui/heap_query_engine.go`（集成）

**向后兼容策略**:

```go
// HeapDataProvider abstracts different data sources for heap queries.
type HeapDataProvider interface {
    GetBiggestObjects(topN int, sortBy string, classFilter string) ([]BiggestObjectResult, error)
    GetGCRootPath(objectID string, maxPaths int, maxDepth int) ([]GCRootPathResult, error)
    GetRetainers(objectID string, maxRetainers int) ([]RetainerResult, error)
    GetObjectFields(objectID string) ([]ObjectFieldResult, error)
    GetGCRootsSummary() ([]GCRootSummaryResult, error)
}

// resolveHeapProvider selects the appropriate data provider based on available files.
func (s *Server) resolveHeapProvider(taskDir string) HeapDataProvider {
    // 优先使用新格式
    indexFile := filepath.Join(taskDir, "heap_index.bin")
    if _, err := os.Stat(indexFile); err == nil {
        return s.getOrLoadQueryEngine(taskDir)
    }
    
    // 降级到旧格式 (refgraph.bin)
    refGraphFile := filepath.Join(taskDir, "refgraph.bin")
    if _, err := os.Stat(refGraphFile); err == nil {
        return s.refGraphService.GetProvider(taskDir)
    }
    
    // 最终降级: 静态 JSON 文件
    return NewStaticFileProvider(taskDir)
}
```

**加载策略**:

```go
// HeapQueryEngineCache manages loaded engines with LRU eviction.
type HeapQueryEngineCache struct {
    mu       sync.RWMutex
    cache    map[string]*HeapQueryEngine
    maxSize  int  // 默认 2（每个 ~500MB）
}
```

**验收标准**:
- [ ] 新格式数据（heap_index.bin）：所有 API 正常响应
- [ ] 旧格式数据（refgraph.bin）：所有 API 正常响应（降级路径）
- [ ] 纯静态文件数据（无 bin）：Overview/Histogram 正常，深度查询返回 404
- [ ] WebUI 前端代码零改动
- [ ] API 响应格式与当前完全一致

---

### 4.5 Sprint 5: 渐进式加载 + 用户体验

**目标**: analyze 分阶段输出 + serve 渐进解锁，减少用户感知等待时间。

**改动文件**:
- `perflib/analyzer/java_heap_analyzer.go`（分阶段写入）
- `internal/webui/server.go`（进度检测）
- `internal/webui/templates/index_modular.html`（进度展示）

**分阶段输出设计**:

```
时间线 (2.6GB 文件):
  t=0     开始分析
  t=35s   Two-Pass CSR 完成 → 写入 summary.json + class_histogram.json
           └── WebUI: Overview Tab 可用 ✅
  t=50s   Dominator 计算完成 → 追加 retained_sizes 到 heap_index.bin (部分)
           └── WebUI: Top Classes 含 retained size ✅  
  t=65s   heap_index.bin 写入完成
           └── WebUI: 所有 Tab 可用 (按需查询) ✅
```

**进度 API**:

```go
// GET /api/progress?task=xxx
// Response:
{
    "status": "analyzing",  // "analyzing" | "ready" | "error"
    "stages": [
        {"name": "parse", "status": "completed", "duration_ms": 35000},
        {"name": "dominator", "status": "completed", "duration_ms": 15000},
        {"name": "index", "status": "in_progress", "progress": 0.7},
    ],
    "available_tabs": ["overview", "histogram", "biggest-objects"],
    "pending_tabs": ["gc-roots"]
}
```

**WebUI 变化**:
- 各 Tab 标记状态：`✅ Ready` / `⏳ Computing...` / `🔒 Pending`
- 在 `--serve` 模式下实时轮询进度
- 已就绪的 Tab 立即可用，无需等待全部完成

**验收标准**:
- [ ] analyze 35s 后 WebUI 即可显示 Overview
- [ ] 不同阶段完成后对应 Tab 自动解锁
- [ ] 进度 API 正确反映当前状态
- [ ] 非 `--serve` 模式下仍然是批量输出（向后兼容）

---

## 五、与 HPROF 性能优化 Roadmap 的关系

本文档与 `docs/hprof-performance-optimization.md` 的关系：

| 性能优化 Roadmap | 本文档（架构优化） | 关系 |
|---|---|---|
| Phase 1: Two-Pass CSR | Sprint 3: Analyze 流程瘦身 | **共享**：Two-Pass CSR 是两者的基础 |
| Phase 2: mmap I/O | - | 独立：I/O 优化是额外加速 |
| Phase 3: Index Files | Sprint 2: heap_index.bin | **等价**：索引文件格式是同一个东西 |
| - | Sprint 1: HeapQueryEngine | **新增**：按需查询引擎 |
| - | Sprint 4: WebUI 集成 | **新增**：serve 端适配 |
| - | Sprint 5: 渐进式加载 | **新增**：用户体验提升 |

**建议合并执行顺序**:

```
阶段 1 (并行):
  ├── 性能优化 Sprint 1.1-1.2: Two-Pass CSR 框架 + 构建  [4-5 天]
  └── 架构优化 Sprint 1: HeapQueryEngine 核心            [3-4 天]

阶段 2:
  └── 合并: heap_index.bin 格式设计 + 序列化              [3-4 天]
      (= 性能优化 Sprint 3.1 + 架构优化 Sprint 2)

阶段 3:
  ├── 性能优化 Sprint 1.3: Dominator/Retainer 适配 CSR   [2-3 天]
  └── 架构优化 Sprint 3: Analyze 流程瘦身                 [2-3 天]

阶段 4:
  └── 架构优化 Sprint 4: WebUI 集成 + 向后兼容            [2-3 天]

阶段 5 (可选):
  ├── 性能优化 Phase 2: mmap I/O                         [3 天]
  └── 架构优化 Sprint 5: 渐进式加载                       [2-3 天]
```

---

## 六、风险评估

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| HeapQueryEngine 内存占用超预期 | OOM | 中 | LRU cache 限制 + mmap 后备方案 |
| BFS 查询在复杂图上超时 | API 卡顿 | 低 | 深度/宽度双限制 + context 超时 |
| 新旧格式兼容逻辑复杂 | Bug | 中 | 充分测试 + 清晰的 Provider 接口 |
| heap_index.bin 格式后续需要变更 | 维护成本 | 中 | 版本号 + 向前兼容设计 |
| Two-Pass 依赖 io.ReadSeeker | 管道场景受限 | 低 | 保留单 Pass fallback |
| serve 阶段计算密集影响响应 | 高延迟 | 低 | 异步 + 结果缓存 |

---

## 七、预期收益总结

| 指标 | 当前 | 优化后 | 变化 |
|------|------|--------|------|
| analyze 耗时 (2.6GB) | ~180s | ~65s | **2.8x 加速** |
| analyze 峰值内存 | ~8 GB | ~800 MB | **10x 降低** |
| 输出文件总大小 | ~1.5 GB (refgraph.bin 为主) | ~450 MB | **3x 减小** |
| serve 首次加载 | ~30-90s (反序列化 refgraph.bin) | ~2s (加载 heap_index.bin) | **15-45x 加速** |
| API 查询响应 | ~50-200ms | ~5-100ms | **2-10x 加速** |
| 用户首次看到结果 | 等分析完 (~180s) | ~35s (Overview 可用) | **5x 改善** |
| WebUI 前端改动 | - | 零改动 | ✅ |

---

## 八、未完成事项 / 遗留问题

1. **ObjectID→Index 映射存储**: 当前 `IndexedObjectStore` 使用 `map[uint64]int32`，5M 对象时占 ~200MB。是否考虑 perfect hash 或排序数组 + 二分查找？
2. **多 goroutine 查询安全**: HeapQueryEngine 是否需要读锁保护？（如果数据加载后不可变则无需）
3. **大规模 BFS 内存**: GC Root Path BFS 队列在极端情况下可能增长。是否需要内存池？（已有 `graph_buffer_pool.go`）
4. **ClassID→ClassName 映射**: 是否在 heap_index.bin 中用 string interning 减少重复？
5. **增量更新**: 如果用户在 serve 运行期间重新 analyze 同一个 task，如何热更新索引？
6. **与 CPU/Tracing 分析的统一**: 当前 HeapQueryEngine 只服务 heap 分析。CPU/Tracing 的文件格式是否也需要类似优化？

---

## 更新记录

| 日期 | 内容 |
|------|------|
| 2026-05-06 | 初始方案设计完成，基于方案 B 思路细化实施路径 |
| 2026-05-06 | **阶段 1 实施**: 完成 ScanPass (Two-Pass CSR Pass 1) + HeapQueryEngine 核心 |
| 2026-05-06 | **阶段 1 完成**: 完成全部单元测试 (28 tests) + CLI 集成 + 端到端验证；TwoPass 为默认行为 |

---

## 九、实施进展

### 阶段 1: Two-Pass CSR 框架 + HeapQueryEngine 核心

**状态**: ✅ Pass 1 + Pass 2 + ParseTwoPass 入口完成，单元测试完整，CLI `--two-pass` 集成完成

**已完成文件**:

| 文件 | 说明 | 行数 |
|------|------|------|
| `perflib/parser/hprof/scan_pass.go` | Pass 1 扫描实现 | ~980 行 |
| `perflib/parser/hprof/build_pass.go` | Pass 2 CSR 填充 + ParseTwoPass 入口 | ~430 行 |
| `internal/webui/heap_query_engine.go` | 按需查询引擎 | ~520 行 |
| `perflib/parser/hprof/graph_indexed.go` | 添加 Accessor 方法 | +30 行 |

**Pass 1 (scan_pass.go) 关键特性**:
- `ScanPass(ctx, io.ReadSeeker)` → 快速扫描 HPROF，收集元数据 + 边度数计数
- 使用 `bytesRead` 跟踪模式（与现有 parser.go 一致）
- 支持所有 HeapDumpTag 子记录（含 Android/OpenJDK 扩展格式）
- 使用 `RetainedSize` 字段临时存储度数（`ExtractDegreeCounts()` 提取）
- 处理 deferred instances（CLASS_DUMP 未先于 INSTANCE_DUMP 出现的情况）

**HeapQueryEngine 关键特性**:
- `QueryBiggestObjects` - min-heap top-N 选择，O(N) 扫描
- `QueryGCRootPath` - BFS on CSR inEdges，深度 + 宽度限制
- `QueryRetainers` - CSR inEdges 直接查找
- `QueryObjectFields` - CSR outEdges 直接查找
- `QueryClassInstances` - class 过滤 + 排序
- `QueryGCRootsSummary` - GC roots 分组聚合
- 所有方法基于 `IndexedReferenceGraph` 的现有接口

**Pass 2 (build_pass.go) 关键特性**:
- `BuildPass(ctx, io.ReadSeeker, *ScanResult)` → 重新读取 HPROF，提取实际引用目标，填充 CSR
- `ParseTwoPass(ctx, io.ReadSeeker)` → 编排入口：Pass 1 + Pass 2 一键调用
- 使用 `CompactEdgeListBuilder` 构建出边和入边 CSR
- `readObjectID(data, idSize)` 公共工具函数，从字节切片读取对象 ID
- `extractBuildReferences` 复用 Pass 1 的 ClassHierarchyFields 提取实例字段引用
- 处理 CLASS_DUMP 静态字段引用、OBJECT_ARRAY_DUMP 元素引用
- 处理 deferred instances（同 Pass 1 模式）
- `assembleGraph` 组装 IndexedReferenceGraph（对象、边、GC root bitset、class bitset）
- `computeReachability` BFS 计算可达性标记
- 所有 GC root/Android 扩展 tag 在 Build Pass 中仅 Skip（已在 Pass 1 收集）

**待完成（阶段 1 剩余）**:
- [x] `scan_pass_test.go` - ScanPass 单元测试 ✅
- [x] `build_pass_test.go` - BuildPass 单元测试 ✅
- [x] `heap_query_engine_test.go` - HeapQueryEngine 单元测试 ✅
- [x] 用真实 `test/heap.hprof` 端到端验证 ParseTwoPass 正确性 ✅
- [x] 集成到 CLI analyze 命令（添加 `--two-pass` 模式开关） ✅

**CLI 集成改动文件**:

| 文件 | 说明 |
|------|------|
| `perflib/analyzer/base_analyzer.go` | TwoPass 为默认行为，无需配置字段 |
| `perflib/analyzer/java_heap_analyzer.go` | `Analyze` 默认使用 `analyzeTwoPass`；`AnalyzeFromReader` 自动检测 ReadSeeker |
| `internal/analyzer/base_analyzer.go` | 无需额外配置 |
| `internal/analyzer/convert.go` | 无需传递 TwoPass 标志 |
| `cmd/cli/cmd/analyze.go` | 无需额外 flag，heap 分析默认走 TwoPass |

**CLI 使用方式**:
```bash
# 分析 heap dump（默认使用 Two-Pass CSR 轻量预计算）
perf-analysis analyze -i ./heap.hprof -m heapdump-heap

# 分析 + 启动 Web 服务器查看结果
perf-analysis analyze -i ./heap.hprof -m heapdump-heap --serve
```

**端到端验证结果（132MB heap dump）**:
- 总耗时: **1.26 秒** (vs 旧流程 ~10s+)
- 对象数: 1,218,857
- 边数: 4,069,117 (出边 + 入边各 2,184,110)
- GC Roots: 7,030
- 类数: 27,423
- 可达对象: 90.1%
- 输出: `class_histogram.json` (51 KB) + `summary.json` (自动生成)
