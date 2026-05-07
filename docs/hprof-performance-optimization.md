# HPROF 解析性能优化方案

> 创建时间: 2026-05-06  
> 状态: 方案设计阶段  
> 目标: 将 2.6GB hprof 文件分析时间从 ~3 分钟缩短至 ~30-45 秒（4-5x 加速）

---

## 一、背景与问题

### 1.1 当前性能表现

| 测试文件 | 大小 | 估算对象数 | 估算引用数 | 分析耗时 |
|----------|------|-----------|-----------|---------|
| `test/heap-1.hprof` | 138 MB | ~120 万 | ~350 万 | ~16s |
| `test/heap.hprof` | 2.6 GB | ~500 万+ | ~1500 万+ | ~3min+ |

### 1.2 Phase 耗时分布（基于 138MB 文件的比例推算）

| 阶段 | 138MB 实测 | 2.6GB 估算 | 占比 |
|------|-----------|-----------|------|
| Phase 1 - Parse HPROF records | 1.3s | ~25s | 15% |
| Phase 2 - Process deferred instances | <1ms | <10ms | 0% |
| Phase 3 - Build result (构建引用图+dominator) | 14.5s | **~90s** | **50%** |
| Phase 4 - Dominator tree computation | 2.7s | ~30s | 17% |
| Phase 5 - Class statistics collection | 1.2s | ~10s | 6% |
| Phase 6 - Parallel analysis (retainer/graph) | 7.9s | ~25s | 14% |
| Phase 7-8 - Biggest objects + GC roots | 2.7s | ~10s | 6% |

### 1.3 内存使用分析

当前核心数据结构 `ReferenceGraph` 使用 `map[uint64][]ObjectReference`：

```go
type ObjectReference struct {
    FromID    uint64  // 8 bytes
    ToID      uint64  // 8 bytes  
    FieldName string  // 16 bytes (string header) + N bytes (string data)
    ClassID   uint64  // 8 bytes
}
// 每条引用 ≈ 56 bytes × 2 份 (incomingRefs + outgoingRefs)
```

| 对象规模 | map key + slice 开销 | ObjectReference 数据 | 总内存 |
|---------|---------------------|---------------------|--------|
| 5M 对象, 15M 引用 | ~2 GB | ~1.7 GB × 2 | **~7-8 GB** |

---

## 二、业界参考策略

### 2.1 Eclipse MAT 架构

```
┌─────────────────────────────────────────────────┐
│  Two-Pass Parsing                                │
│  ┌─────────────┐    ┌──────────────────────┐    │
│  │ Pass 1      │    │ Pass 2               │    │
│  │ 扫描元数据   │───▶│ 提取对象引用          │    │
│  │ (~20% 时间)  │    │ (~80% 时间)          │    │
│  └─────────────┘    └──────────────────────┘    │
│                              │                   │
│                              ▼                   │
│  ┌──────────────────────────────────────────┐   │
│  │ Index Files (.index)                      │   │
│  │ - objectID → file offset                  │   │
│  │ - 仅存活对象 (GC 过滤)                     │   │
│  │ - 下次打开免解析                            │   │
│  └──────────────────────────────────────────┘   │
└─────────────────────────────────────────────────┘
```

**核心策略**: 
1. Pass 1 只收集统计信息（对象数、引用度数、类信息）
2. Pass 2 基于 Pass 1 的统计预分配数组，一次性填充
3. 生成索引文件缓存解析结果，重复打开秒开
4. 使用 `int[]` 紧凑数组存储，非 HashMap

### 2.2 Android Studio (perflib) 架构

```
┌─────────────────────────────────────────────────┐
│  Single-Pass + Lazy Loading                      │
│  ┌──────────────────────────────────────────┐   │
│  │ Parse Phase:                              │   │
│  │   - 只记录对象在文件中的 offset            │   │
│  │   - 不读取字段值 (skip field data)        │   │
│  │   - 使用 mmap，skip = 移动指针            │   │
│  └──────────────────────────────────────────┘   │
│                              │                   │
│                              ▼                   │
│  ┌──────────────────────────────────────────┐   │
│  │ On-Demand Read:                           │   │
│  │   - 需要分析某对象时才从 mmap 读取字段     │   │
│  │   - 天然支持大于物理内存的文件             │   │
│  └──────────────────────────────────────────┘   │
│                                                  │
│  数据结构: Trove 原始类型 Map (避免装箱)         │
└─────────────────────────────────────────────────┘
```

**核心策略**:
1. 惰性读取：解析时只存 offset，不读字段数据
2. mmap：文件映射为内存，skip 操作零 I/O
3. 原始类型 Map：避免 Java 自动装箱的内存开销

### 2.3 我们的优化方向

综合两者策略，结合 Go 语言特性和现有代码基础：

| 策略 | 来源 | Go 实现 | 预期收益 |
|------|------|---------|---------|
| Two-Pass CSR 构建 | MAT | Pass 1 计数 → Pass 2 填充预分配数组 | 消除 map 中间层，内存降 10x |
| mmap 文件读取 | perflib | `syscall.Mmap` / `golang.org/x/exp/mmap` | I/O 提速 2-3x |
| int32 索引替代 uint64 | MAT | 已有 `IndexedObjectStore` | 指针宽度减半 |
| CSR 格式边列表 | MAT | 已有 `CompactEdgeList` | 缓存友好，GC 压力为零 |
| 索引文件缓存 | MAT | protobuf 序列化 dominator tree + CSR | 二次打开秒开 |
| FastMode 自动启用 | 自有 | 大文件自动跳过深度分析 | 减少 70-90% 分析时间 |

---

## 三、Roadmap 总览

```
Phase 1: 核心引擎重构 (Two-Pass CSR)        预期: 3x 加速, 内存降 10x
  │
  ├── Sprint 1.1: Two-Pass 解析框架
  ├── Sprint 1.2: CSR 图构建 + IndexedObjectStore 集成
  └── Sprint 1.3: Dominator / Retainer 适配 CSR 格式
  
Phase 2: I/O 优化 (mmap)                    预期: 额外 30-50% 加速
  │
  ├── Sprint 2.1: mmap Reader 实现
  └── Sprint 2.2: 大文件自动 mmap + 阈值策略
  
Phase 3: 缓存与体验 (Index Files)           预期: 二次打开秒开
  │
  ├── Sprint 3.1: 索引文件序列化/反序列化
  └── Sprint 3.2: WebUI 渐进式加载
```

---

## 四、Phase 1: 核心引擎重构 (Two-Pass CSR)

### 4.1 目标

- 消除 `map[uint64][]ObjectReference` 中间数据结构
- 解析阶段直接产出 CSR 格式紧凑图
- 内存使用从 ~8 GB 降至 ~800 MB
- 总分析时间减少 60-70%

### 4.2 Sprint 1.1: Two-Pass 解析框架

**改动范围**: `perflib/parser/hprof/parser.go`

**设计**:

```go
// Parse 方法重构为 Two-Pass
func (p *Parser) Parse(ctx context.Context, r io.ReadSeeker) (*HeapAnalysisResult, error) {
    // Pass 1: 快速扫描 - 只收集元数据和统计
    scanResult, err := p.scanPass(ctx, r)
    // scanResult 包含:
    //   - objectCount (总对象数)
    //   - edgeCount (总引用数)  
    //   - objectIndex: objectID → int32 compactIndex
    //   - degreeCounts: []int32 (每个对象的出引用数量)
    //   - classInfo, strings 等元数据

    // Pass 2: 引用填充 - 预分配 CSR 数组并填充
    r.Seek(0, io.SeekStart)  // 回到文件开头
    graph, err := p.buildGraphPass(ctx, r, scanResult)
    // graph 是 CompactEdgeList (CSR 格式)
    
    // Phase 3+: Dominator + Analysis (直接使用 CSR 图)
    ...
}
```

**Pass 1 输出**:

```go
type ScanResult struct {
    Header        *Header
    ObjectCount   int32
    EdgeCount     int64
    ObjectIndex   *IndexedObjectStore  // objectID → compact int32 index
    DegreeCounts  []int32              // 每个对象的出引用数
    ClassInfo     map[uint64]*ClassInfo
    ClassFields   map[uint64][]FieldDescriptor
    Strings       map[uint64]string
    ClassNames    map[uint64]uint64
    GCRoots       []GCRootEntry
}
```

**接口变更**:
- `Parser.Parse()` 的参数从 `io.Reader` 变为 `io.ReadSeeker`（支持 Seek 回到 Pass 1 起点）
- 保留 `io.Reader` 的兼容路径：如果不支持 Seek，fallback 到旧的单 Pass 路径

**预期结果**:
- Pass 1 耗时约为当前 Phase 1 的 70%（不构建引用数据，只计数）
- 内存使用在 Pass 1 阶段仅需 ~200 MB（对象索引 + 度数数组）

### 4.3 Sprint 1.2: CSR 图构建 + IndexedObjectStore 集成

**改动范围**: 
- `perflib/parser/hprof/graph_reference.go` (重构)
- `perflib/parser/hprof/graph_indexed.go` (集成到主流程)
- 新增 `perflib/parser/hprof/graph_csr_builder.go`

**设计**:

```go
// CSRGraphBuilder 基于 Pass 1 的统计预分配并填充
type CSRGraphBuilder struct {
    objectStore  *IndexedObjectStore
    outEdges     *CompactEdgeList  // 出引用 CSR
    inEdges      *CompactEdgeList  // 入引用 CSR (用于 retainer 分析)
}

// 预分配策略 (基于 Pass 1 的 DegreeCounts)
func NewCSRGraphBuilder(scan *ScanResult) *CSRGraphBuilder {
    outEdges := NewCompactEdgeList(int(scan.ObjectCount), int(scan.EdgeCount))
    // 使用 DegreeCounts 计算 offsets 前缀和
    // offsets[i+1] = offsets[i] + degreeCounts[i]
    ...
}

// Pass 2 中逐条填充
func (b *CSRGraphBuilder) AddEdge(fromIdx, toIdx int32, fieldName string, classID uint64) {
    // 直接写入预分配位置，零 append
}
```

**核心优势**:
- 零 slice 扩容：CSR offsets + targets 在 Pass 2 开始前一次性分配
- 零 map：所有访问通过 int32 索引，O(1) 数组寻址
- 缓存友好：连续内存访问模式

**内存对比**:

| 数据结构 | 5M 对象, 15M 引用 | 当前 |
|----------|-------------------|------|
| `offsets []int32` (出) | 20 MB | - |
| `targets []int32` (出) | 60 MB | - |
| `fieldIDs []int32` (出) | 60 MB | - |
| `offsets []int32` (入) | 20 MB | - |
| `targets []int32` (入) | 60 MB | - |
| `IndexedObjectStore` | 200 MB | - |
| **总计** | **~420 MB** | **~8 GB** |

**预期结果**:
- 内存使用降至 ~500 MB（含元数据）
- Pass 2 填充速度比当前 Phase 1 快 2x（预分配 + 无 map 查找）

### 4.4 Sprint 1.3: Dominator / Retainer 适配 CSR 格式

**改动范围**:
- `perflib/parser/hprof/dom_hierarchical.go` (适配 CSR 输入)
- `perflib/parser/hprof/dom_dominator.go` (适配 CSR 输入)
- `perflib/parser/hprof/analysis_retainer.go` (使用 CSR 反向图)

**设计**:

当前 dominator 算法已有 CSR 内部表示（`LevelDominatorState`），但需要先从 `ReferenceGraph` 的 map 转换为 CSR。重构后直接传入 `CompactEdgeList`：

```go
// 当前: map → CSR 转换 (多一次 O(V+E) 遍历)
func (d *LevelDominatorComputer) buildCSRFromRefGraph(rg *ReferenceGraph) { ... }

// 重构后: 直接使用 CSR
func (d *LevelDominatorComputer) ComputeFromCSR(
    outEdges *CompactEdgeList, 
    store *IndexedObjectStore,
    gcRoots []int32,
) error { ... }
```

**预期结果**:
- 消除 map → CSR 转换步骤（节省 ~15-20s）
- Dominator 计算速度不变（算法本身已优化）
- Retainer 分析使用 CSR 入引用，比 map 随机访问快 3-5x

### 4.5 Phase 1 总结

| 指标 | 优化前 | 优化后 | 变化 |
|------|--------|--------|------|
| 总分析时间 (2.6GB) | ~180s | ~55s | **3.3x 加速** |
| 峰值内存 (2.6GB) | ~8 GB | ~800 MB | **10x 降低** |
| Phase 1 解析 | ~25s | ~35s (两趟) | +40% (多一趟) |
| Build result (map→CSR) | ~90s | 0s (消除) | ∞ |
| Dominator | ~30s | ~15s | 2x |
| Retainer | ~25s | ~8s | 3x |

---

## 五、Phase 2: I/O 优化 (mmap)

### 5.1 目标

- 替换 buffered reader 为 mmap 文件映射
- Two-Pass 中第二次读取几乎零 I/O 开销（OS 页缓存命中）
- 支持大于物理内存的堆文件分析

### 5.2 Sprint 2.1: mmap Reader 实现

**改动范围**: 
- 新增 `perflib/parser/hprof/reader_mmap.go`
- 修改 `perflib/parser/hprof/core_reader.go` (抽象 Reader 接口)

**设计**:

```go
// ReaderSeeker 抽象接口 (兼容 io.ReadSeeker 和 mmap)
type ReaderSeeker interface {
    io.Reader
    io.Seeker
    ReadByte() (byte, error)
    ReadUint32() (uint32, error)
    ReadUint64() (uint64, error)
    ReadBytes(n int) ([]byte, error)
    Skip(n int64) error
    Position() int64
}

// MmapReader mmap 实现
type MmapReader struct {
    data []byte  // mmap 映射的内存区域
    pos  int64   // 当前读取位置
    size int64   // 文件大小
}

func NewMmapReader(filePath string) (*MmapReader, error) { ... }
func (r *MmapReader) Skip(n int64) error {
    r.pos += n  // 零 I/O，纯指针移动
    return nil
}
```

**兼容性**:
- 保留 `BufferedReader` 作为 fallback（stdin 管道等不支持 mmap 的场景）
- 通过 `Parse(ctx, io.ReadSeeker)` 自动检测：文件则 mmap，管道则 buffered

### 5.3 Sprint 2.2: 大文件自动策略

**阈值策略**:

```go
const (
    MmapThreshold     = 100 * 1024 * 1024  // >100MB 自动使用 mmap
    FastModeThreshold = 1024 * 1024 * 1024  // >1GB 自动启用 FastMode
)
```

**预期结果**:
- Two-Pass 的 Pass 2 几乎免费（OS 页缓存已预热）
- 随机访问模式（retainer BFS）性能提升 3-5x
- 支持分析 10GB+ 堆文件（依赖 OS 分页，非全量加载）

### 5.4 Phase 2 总结

| 指标 | Phase 1 后 | Phase 2 后 | 变化 |
|------|-----------|-----------|------|
| 总分析时间 (2.6GB) | ~55s | ~35-40s | 30-40% 加速 |
| Pass 1 解析 | ~20s | ~12s | mmap 连续读取 |
| Pass 2 填充 | ~15s | ~8s | 页缓存命中 |
| I/O 等待 | 显著 | 接近零 | OS 预读 |

---

## 六、Phase 3: 缓存与体验 (Index Files)

### 6.1 目标

- 首次解析后生成索引文件，再次打开同一文件秒开
- WebUI 支持渐进式加载，先展示 Overview 再后台计算
- 适合 `--serve` 模式反复查看分析结果

### 6.2 Sprint 3.1: 索引文件序列化/反序列化

**改动范围**: 
- 新增 `perflib/parser/hprof/index_cache.go`
- 修改 `perflib/parser/hprof/parser.go` (添加缓存检查)

**设计**:

```
文件结构:
  heap.hprof              # 原始堆文件
  heap.hprof.idx          # 索引文件 (与原文件同目录)

索引文件内容 (protobuf):
  - file_hash: SHA256(前 64KB + 文件大小)  # 快速校验一致性
  - object_store: IndexedObjectStore 序列化
  - out_edges: CompactEdgeList 序列化 (offsets + targets)
  - in_edges: CompactEdgeList 序列化
  - dominator_tree: []int32 (parent indices)
  - retained_sizes: []int64
  - class_stats: ClassStatistics
  - gc_roots: []GCRootEntry
```

**加载流程**:

```
Parse(file):
  1. 检查 file.idx 是否存在且 hash 匹配
  2. 如果匹配 → 反序列化索引，跳过全部解析，直接构建结果
  3. 如果不匹配 → 正常 Two-Pass 解析 → 写入新的 .idx 文件
```

**预期结果**:
- 首次分析: 正常时间 (~35-40s) + 索引写入 (~2-3s)
- 再次打开: **~1-2s**（只需反序列化索引文件）

### 6.3 Sprint 3.2: WebUI 渐进式加载

**设计思路**:

将分析拆分为多个独立阶段，每阶段完成后立即写入输出文件：

```
Stage 1 (即时): Class Histogram → class_histogram.json  → Overview 可展示
Stage 2 (5s):   Dominator Tree → biggest_objects.json   → Biggest Objects 可展示
Stage 3 (10s):  GC Roots       → gc_roots.json          → GC Roots 可展示
Stage 4 (20s):  Retainer       → retainer_analysis.json → Merged Paths 可展示
```

WebUI 通过轮询或 SSE 检测新文件出现，逐步解锁 tab。

**预期结果**:
- 用户在 ~3s 内看到 Overview 和 Class Histogram
- 后续 tab 逐步解锁，无需等待全部完成

---

## 七、风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| Two-Pass 需要 Seekable 输入 | 管道/stdin 场景不可用 | 保留单 Pass fallback 路径 |
| mmap 在 32 位系统受限 | 大文件无法映射 | 仅 64 位系统启用，32 位回退 buffered |
| int32 索引上限 21 亿对象 | 超大堆溢出 | 预留 int64 升级路径，实际 JVM 很少超 20 亿对象 |
| CSR 格式不支持动态插入 | 需要全量预计算 | Two-Pass 保证在 Build 前已知全部边 |
| 索引文件一致性 | 堆文件修改后索引过期 | SHA256 校验 + 文件大小 + mtime 三重验证 |
| 向后兼容 | 旧接口调用方需适配 | 保留 `Parse(ctx, io.Reader)` 签名，内部检测类型 |

---

## 八、验收标准

### Phase 1 验收

- [ ] `test/heap.hprof` (2.6GB) 分析时间 < 60s
- [ ] 峰值内存使用 < 1.5 GB（当前 ~8 GB）
- [ ] 所有现有测试通过（`go test ./perflib/parser/hprof/...`）
- [ ] 分析结果与优化前一致（class histogram、dominator tree、retained sizes）
- [ ] Benchmark 对比报告

### Phase 2 验收

- [ ] `test/heap.hprof` 分析时间 < 40s
- [ ] 管道输入 fallback 路径正常工作
- [ ] 无 mmap 泄漏（测试中验证 unmap）

### Phase 3 验收

- [ ] 二次打开同一文件 < 3s
- [ ] 索引文件大小 < 原文件的 5%
- [ ] WebUI 在 3s 内展示 Overview
- [ ] 索引文件损坏时优雅降级（重新解析）

---

## 九、实施计划

| Sprint | 预计工时 | 依赖 | 关键交付 |
|--------|---------|------|---------|
| 1.1 Two-Pass 框架 | 3-4 天 | 无 | ScanResult + 新 Parse 入口 |
| 1.2 CSR 构建 | 3-4 天 | 1.1 | CompactEdgeList 集成主流程 |
| 1.3 Dominator/Retainer 适配 | 2-3 天 | 1.2 | 全链路打通 + 性能验证 |
| 2.1 mmap Reader | 2 天 | 1.1 | MmapReader + 兼容路径 |
| 2.2 自动策略 | 1 天 | 2.1 | 阈值配置 + FastMode 联动 |
| 3.1 索引文件 | 3-4 天 | 1.3 | protobuf schema + 序列化/反序列化 |
| 3.2 渐进式加载 | 2-3 天 | 3.1 | WebUI SSE + 分阶段输出 |

**总计**: ~16-21 天（不含测试和文档）

---

## 十、现有代码资产

以下代码已存在于 `perflib/parser/hprof/` 中，可直接复用：

| 文件 | 可复用组件 | 当前状态 |
|------|-----------|---------|
| `graph_indexed.go` | `IndexedObjectStore` | ✅ 完整实现，未接入主流程 |
| `graph_indexed.go` | `CompactEdgeList` + `CompactEdgeListBuilder` | ✅ 完整实现，未接入主流程 |
| `dom_hierarchical.go` | 层次并行 dominator（CSR 内部格式） | ✅ 已在大图使用 |
| `dom_parallel.go` | 并行 predecessors 构建 | ✅ 已使用 |
| `util_mmap_store.go` | mmap 基础设施 | ✅ 已实现，用于可选存储 |
| `graph_buffer_pool.go` | sync.Pool 复用 BFS 队列 | ✅ 已使用 |
| `util_bitset.go` | Versioned bitset | ✅ 已使用 |
| `serial_serializer.go` | Protobuf 序列化 | ✅ 已用于 refgraph.bin |

---

## 十一、关联文档

- **架构优化方案**: [docs/architecture-optimization-plan.md](./architecture-optimization-plan.md)
  - 基于方案 B（轻量预计算 + 运行时按需分析）的详细实施路径
  - 包含 HeapQueryEngine 设计、heap_index.bin 格式、WebUI 集成策略
  - 与本文档 Phase 1/Phase 3 有共享部分，建议合并执行

---

## 十二、遗留问题

1. ~~**`io.Reader` vs `io.ReadSeeker`**: 是否强制要求 Seekable 输入？还是保留 Reader-only 的降级路径？~~ → 已解决：强制要求 Seekable，不支持 non-seekable reader。
2. ~~**fieldName 存储**: CSR 格式中 fieldName 用 intern ID 替代字符串，retainer 分析需要还原时如何高效查找？~~ → 已解决：`CompactEdgeList.GetFieldName(id)` + 序列化到 heap_index.bin。
3. ~~**并行 Pass 2**: Pass 2 能否用多个 goroutine 并行填充 CSR？~~ → ✅ P4 已实施：14 workers 并行解析，Build reference edges 1.72x 加速。
4. ~~**增量分析**: 是否考虑支持"快速模式"先完成 class histogram + overview，后台异步计算 dominator？~~ → 已解决：HeapQueryEngine 按需计算。
5. ~~**heap_index.bin 文件大小优化**~~ → P3 待实施：采用 mmap 局部加载方案。

---

## 十三、后续优化路线图

| 顺序 | 编号 | 任务 | 目标 | 依赖 | 影响范围 | 状态 |
|------|------|------|------|------|---------|------|
| 1 | **P2** | `internal/parser/hprof` 瘦身 | 清理 legacy 类型别名和委托函数 | 无 | internal/parser/hprof | ✅ 已完成 |
| 2 | **P4** | Build Pass 并行化 | Build Pass 5.8s → 3.37s | 无 | perflib/parser/hprof | ✅ 已完成 |
| 3 | **P3** | heap_index.bin mmap 局部加载 | 内存 ~1.2GB → ~50-100MB | P4 后，格式可能微调 | index_format + reader/writer 重新设计 | ✅ 已完成 |
| 4 | **P5** | 文档 + ARCHITECTURE.md | 记录 Two-Pass CSR + HeapQueryEngine | P2-P4 完成后 | docs/ | ✅ 已完成 |
| 5 | **P6** | 单测补全 | 覆盖 dom_indexed / build_pass / heap_query_engine | P3 之后 | perflib/parser/hprof/*_test.go | ✅ 已完成 |
| 6 | **P7** | WebUI 展示优化 | Dominator tree 视图、retained size treemap | P3 之后 | internal/webui/ | ✅ 已完成 |

---

### P3 设计方案：heap_index.bin mmap 局部加载

#### 问题背景

当前 `ReadHeapIndex` 一次性将整个 heap_index.bin 加载到内存：
- 16M 对象的文件约 ~450MB（磁盘）→ 加载后内存占 ~1.2GB（含 map 重建等开销）
- 大堆文件分析后仅查询少量对象，全量加载浪费
- WebUI `--serve` 模式下可能同时缓存多个分析结果

#### 设计目标

1. **内存**：从 ~1.2GB 降至 ~50-100MB（常驻 header + string pool + objToIdx map）
2. **延迟**：冷启动 <500ms（之前 ~2s），热路径 <0.1ms
3. **兼容**：格式版本号 v1 → v2，ReadHeapIndex 自动识别并降级兼容 v1

#### 格式设计：heap_index.bin v2

```
┌──────────────────────────────────────────────────────────────────────┐
│                      heap_index.bin v2 Layout                         │
├──────────────┬──────────────┬──────────────┬─────────────────────────┤
│   File Header (40B)         │   Section Table (固定)                  │
│   Magic + Version + Counts  │   每 section 的 (offset, length)       │
├──────────────┴──────────────┴──────────────┴─────────────────────────┤
│                                                                      │
│   Section 1: ObjectStore (mmap 按需)                                 │
│   ├── objIDs:       uint64[N]     → 128MB (16M 对象)                 │
│   ├── classIDs:     uint64[N]     → 128MB                            │
│   ├── shallowSizes: int64[N]      → 128MB                            │
│   └── retainedSizes: int64[N]     → 128MB                            │
│                                                                      │
│   Section 2: CSR OutEdges (mmap 按需)                                │
│   ├── offsets: int32[N+1]         → 64MB                             │
│   ├── targets: int32[E]           → 77MB (19M edges)                 │
│   ├── fieldIDs: int32[E]          → 77MB                             │
│   └── classIDs: uint64[E]         → 153MB                            │
│                                                                      │
│   Section 3: CSR InEdges (mmap 按需)                                 │
│   └── (同 OutEdges 布局)                                             │
│                                                                      │
│   Section 4: DominatorTree (mmap 按需)                               │
│   └── dominators: int32[N]        → 64MB                             │
│                                                                      │
│   Section 5: Bitsets (常驻，<5MB)                                     │
│   └── gcRoot + classObject + reachable                               │
│                                                                      │
│   Section 6: Metadata (常驻，zstd 压缩 → <20MB)                      │
│   └── classNames + fieldNames + GC roots + objToIdx map              │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

#### 核心变更

| 变更 | v1 (当前) | v2 (目标) |
|------|-----------|-----------|
| Section 发现 | 顺序读取，固定顺序 | Section Table 记录 (offset, length)，支持随机定位 |
| 大数组读取 | `io.ReadFull` 全量分配 | `mmap` 映射，OS page cache 管理 |
| objToIdx map | 反序列化时 O(N) 遍历重建 | 序列化到 Metadata section（或 perfect hash） |
| 文件访问 | `bufio.Reader` 顺序流 | `os.File` + `syscall.Mmap` 随机访问 |
| 版本兼容 | 仅 v1 | 检测 Version 字段，v1 走旧路径，v2 走 mmap 路径 |

#### 实现方案

```go
// MmapHeapIndex 是 v2 格式的 mmap 读取器
type MmapHeapIndex struct {
    file     *os.File
    data     []byte          // mmap 映射的完整文件
    header   IndexFileHeader
    sections []SectionEntry  // Section Table: type → (offset, length)
    
    // 常驻内存 (加载时完全读取)
    classNames  map[uint64]string
    fieldNames  []string
    gcRoots     []GCRoot
    gcRootBits  *Bitset
    classBits   *Bitset
    reachBits   *Bitset
    objToIdx    map[uint64]int32 // 或 perfect hash function
    
    // mmap 按需访问 (零拷贝视图)
    objIDs       []uint64  // unsafe 指向 mmap 区域
    classIDs     []uint64
    shallowSizes []int64
    retainedSizes []int64
    outOffsets   []int32
    outTargets   []int32
    outFieldIDs  []int32
    outClassIDs  []uint64
    inOffsets    []int32
    inTargets    []int32
    dominators   []int32
}

// SectionEntry records position of a section in the file
type SectionEntry struct {
    Type   SectionType
    Offset int64  // byte offset from file start
    Length int64  // section data length
}

// Open 加载 v2 索引文件：mmap 整个文件，只解析 header + section table + metadata
func OpenMmapHeapIndex(filePath string) (*MmapHeapIndex, error) {
    // 1. Open file + mmap
    // 2. Parse header (40B) + section table
    // 3. 从 mmap 区域创建各数组的 unsafe.Slice 视图 (零拷贝)
    // 4. 解压 Metadata section (常驻内存: classNames, fieldNames, gcRoots, objToIdx)
    // 5. 解析 Bitsets section (常驻内存: <5MB)
    // 返回: ~50-100MB 常驻 (objToIdx map 是主要开销)
}

// Close 释放 mmap 和文件句柄
func (m *MmapHeapIndex) Close() error {
    syscall.Munmap(m.data)
    return m.file.Close()
}
```

#### 内存分析

| 组件 | v1 全量加载 | v2 mmap (常驻) | v2 mmap (热路径) |
|------|------------|---------------|-----------------|
| objToIdx map | ~300MB | ~300MB (常驻) | ~300MB |
| classNames + fieldNames | ~20MB | ~20MB (常驻) | ~20MB |
| Bitsets | ~6MB | ~6MB (常驻) | ~6MB |
| objIDs/classIDs/sizes | 512MB | 0 (按需 page fault) | 热数据 ~10-50MB |
| CSR edges | 370MB | 0 (按需 page fault) | 热数据 ~10-50MB |
| dominators | 64MB | 0 (按需 page fault) | 热数据 ~5MB |
| **总计** | **~1.2GB** | **~330MB** | **~400MB (峰值)** |

> **注**: objToIdx map 是最大常驻开销。如果需要进一步降低到 <100MB，可考虑：
> - 使用 perfect hash function（如 CHD/MPHF）替代 Go map → ~60MB
> - 或使用 sorted array + binary search → ~130MB (16M × 8B)

#### API 兼容性

`MmapHeapIndex` 需要实现 `HeapGraph` 接口使 `HeapQueryEngine` 无需修改即可切换：

> ✅ **已实现**：`HeapGraph` 接口已定义在 `perflib/parser/hprof/heap_graph.go`，
> `IndexedReferenceGraph` 已实现该接口，`HeapQueryEngine` 已改为依赖接口。
> P3 实现 `MmapHeapIndex` 时只需满足 `HeapGraph` 接口即可无缝接入。

```go
// HeapGraph 是 HeapQueryEngine 需要的只读图接口 (perflib/parser/hprof/heap_graph.go)
type HeapGraph interface {
    ObjectCount() int32
    GetObjectID(idx int32) uint64
    GetObjectIndex(objectID uint64) int32
    GetClassID(idx int32) uint64
    GetShallowSize(idx int32) int64
    GetRetainedSize(idx int32) int64
    GetDominator(idx int32) int32
    GetClassName(classID uint64) string
    IsGCRoot(idx int32) bool
    IsReachable(idx int32) bool
    IsClassObject(idx int32) bool
    
    GetOutgoingEdges(idx int32) (targets []int32, fieldIDs []int32, classIDs []uint64)
    GetIncomingEdges(idx int32) (sources []int32, fieldIDs []int32, classIDs []uint64)
    GetFieldName(fieldID int32) string
    GetObjectsByClass(classID uint64) []int32
    GetGCRoots() []GCRoot
}
```

#### 辅助工具

> ✅ **已实现**：`ObjectInfoAssembler` 定义在 `perflib/parser/hprof/object_info_assembler.go`

```go
// ObjectInfoAssembler 消除重复的对象信息组装代码
type ObjectInfoAssembler struct { graph HeapGraph }

// AssembleByIndex: idx → HeapObjectInfo{ObjectID(hex), ClassName, ShallowSize, RetainedSize}
// AssembleByObjectID: objectID → HeapObjectInfo
// GetClassNameByIndex: idx → className
// ResolveFieldName: fieldIDs[i] → fieldName string
```

#### 实施步骤

1. ~~**Sprint 3.1**: 格式 v2 定义 + Section Table + Writer 适配~~ ✅ **已完成**
   - `index_format_v2.go`: Header(48B) + SectionTableEntry(16B) + PageAlignment 常量
   - `index_writer_v2.go`: `WriteHeapIndexV2` 两阶段写入（计算偏移 → 顺序写入）
2. ~~**Sprint 3.2**: MmapHeapIndex Reader 实现 + unsafe.Slice 视图~~ ✅ **已完成**
   - `index_reader_v2.go`: `OpenMmapHeapIndex` + `Close` + 完整 HeapGraph 实现
   - 零拷贝 unsafe.Slice 视图 + zstd 解压 metadata + bitset 加载
3. ~~**Sprint 3.3**: HeapGraph 接口抽象 + HeapQueryEngine 适配~~ ✅ **已提前完成**
4. ~~**Sprint 3.4**: 版本兼容 (ReadHeapIndex 自动识别 v1/v2) + 测试验证~~ ✅ **已完成**
   - `index_reader.go`: `ReadHeapIndex` peek version → v1(bufio) / v2(mmap) 自动路由
   - `index_test.go`: `TestIndexV2Roundtrip` + `verifyHeapGraphEquality` 接口级验证
   - nil vs empty slice 行为一致性修复

#### 风险与缓解

| 风险 | 缓解 |
|------|------|
| 32 位系统无法 mmap 大文件 | 检测运行环境，32 位走 v1 全量读取路径 |
| mmap 后进程 SIGBUS (文件被截断) | 安装 signal handler，graceful 降级 |
| objToIdx map 仍占 300MB | 后续可用 perfect hash 替代，初期保持 map |
| unsafe.Slice 安全性 | 严格对齐检查 + Close() 后置空所有 slice |

---

### P5 设计方案：文档 + ARCHITECTURE.md

#### 产出物

1. **`docs/ARCHITECTURE.md`** — 系统架构文档
   - Two-Pass CSR 解析引擎（Scan Pass → Build Pass → Dominator → heap_index.bin）
   - HeapQueryEngine 按需计算模型
   - WebUI HeapDataProvider 策略模式
   - 数据流图：hprof → perflib → heap_index.bin → WebUI API → 前端

2. **`perflib/parser/hprof/README.md`** — 包文档
   - 公共 API 使用指南
   - Two-Pass 解析流程详解
   - CSR 数据结构说明
   - 性能调优建议

3. **更新 `README.md`** — 项目顶层
   - 添加架构概览图
   - 更新性能数据
   - 添加 CLI 使用示例

#### 依赖

- P2 (瘦身) 和 P4 (并行化) 完成后，架构稳定
- 在 P3 开始前完成，为 P3 的 API 变更提供参考

---

### P6 设计方案：单测补全

#### 覆盖目标

| 模块 | 当前覆盖 | 目标 | 关键测试场景 | 实施结果 |
|------|---------|------|-------------|---------|
| `dom_indexed.go` | 0% | 100% | 断连子图、环路、深度链 | ✅ 100% (15 用例) |
| `build_pass.go` | ~80% | 边界 | 空文件、畸形数据、超大 dataSize | 已有覆盖 |
| `build_pass_parallel.go` | 基础正确性 | 边界 + 竞态 | segment 边界切割、context cancel、race detector | 已有覆盖 |
| `heap_query_engine.go` | 6 函数覆盖 | 边界补充 | TopN 边界、BFS depth 限制、empty filter | ✅ +7 组边界 |
| `index_writer.go` + `index_reader.go` | roundtrip | 边界 | 空图、仅 1 对象、超大 fieldName | ✅ +6 个边界测试 |
| `scan_pass.go` | 基础 | 边界 | 未知 sub-tag、Android 特有 tag、deferred instance | 未覆盖（低优先级） |

#### 修复的 Bug

1. **vertex 数组越界** (`dom_hierarchical.go`): `NewLevelDominatorState` 中 `vertex` 使用 1-based 索引但只分配了 `nodeCount` 长度，当所有节点可达时 `dfnNum` 达到 `nodeCount` 导致越界。修复：分配 `nodeCount+1`。
2. **ancestor sentinel 值冲突** (`dom_hierarchical.go`): Lengauer-Tarjan 算法用 `ancestor[v]==0` 判断"未链接"，但 super root 的索引正好是 0。当节点被链接到 super root 时，算法错误判定其为未链接，导致多 GC root 场景下 idom 计算错误。修复：sentinel 改为 `-1`。

#### 实施原则

- 表驱动测试 + `t.Parallel()` 并行执行
- 竞态检测：`go test -race`
- 覆盖率目标：核心路径 ≥ 80%
- 每个新增测试附带 issue/scenario 注释

#### 依赖

- P3 (mmap) 完成后统一补测，避免 API 变更导致测试失效

---

### P7 设计方案：WebUI 展示优化

#### 目标功能

1. **Dominator Tree 视图** — 可展开树，类似 Eclipse MAT
   - 按 dominator 关系逐层展开
   - 每个节点显示 className + shallowSize + retainedSize
   - 支持按 retained size 排序子节点
   - Lazy loading（每次只加载直接子节点）

2. **Retained Size Treemap** — 可视化面积图
   - 面积 = retained size
   - 颜色 = class 或 retained/shallow 比率
   - 可钻取（点击进入子 dominator 树）

3. **对象详情面板增强**
   - 显示 dominator path（从 GC Root 到对象的支配链）
   - 显示 dominated objects 列表

#### 新增 API

```
GET /api/refgraph/dominator-tree?idx={idx}&depth=1
  → 返回指定节点的直接 dominated children

GET /api/refgraph/dominator-path?idx={idx}
  → 返回从 virtual root 到指定节点的支配链

GET /api/refgraph/treemap?root={idx}&maxNodes=500
  → 返回 treemap 数据（retained size 驱动的嵌套矩形）
```

#### 前端技术选型

- Treemap: ECharts v5 `treemap` 系列（已集成）
- Dominator Tree: 懒加载树组件（原生 DOM + fetch on expand）
- 复用现有 Alpine.js + Tailwind CSS 架构

#### 依赖

- P3 (mmap) 提供高效的 dominator 随机访问能力
- 需要后端 HeapQueryEngine 新增 `QueryDominatorChildren(idx, topN, sortBy)` 方法

#### 实施结果 (2026-05-07)

| 组件 | 文件 | 说明 |
|------|------|------|
| HeapQueryEngine 扩展 | `internal/webui/heap_query_engine.go` | +3 方法：QueryDominatorChildren, QueryDominatorPath, QueryRetainedSizeTreemap |
| API Handler | `internal/webui/server.go` | +3 路由：/api/refgraph/dominator-tree, /dominator-path, /treemap |
| HeapDataProvider 扩展 | `internal/webui/refgraph_service.go` | +3 接口方法 + indexedProvider 实现 |
| Dominator Tree 前端 | `internal/webui/static/js/heap-dominator-tree.js` | 可展开树视图，Path 弹窗 |
| Retained Treemap 前端 | `internal/webui/static/js/heap-retained-treemap.js` | ECharts treemap + 钻取导航 |
| HTML 模板 | `internal/webui/templates/index_modular.html` | +2 Tab + 2 Panel |
| API 模块 | `internal/webui/static/js/api.js` | +3 方法：getDominatorChildren, getDominatorPath, getRetainedSizeTreemap |

#### 性能优化

- `hasDominatedChildren` 使用 `sync.Once` 懒计算全局 map，避免每次 O(N) 遍历
- `QueryDominatorChildren` 使用 min-heap top-N 选择算法
- `QueryRetainedSizeTreemap` 按类分组后截断，限制 maxNodes 避免前端渲染压力

### P4 实施结果：Build Pass 并行化

**已完成** (2026-05-07)

实现方案：
- Scan Pass 中对大型 HEAP_DUMP_SEGMENT 按 ~64MB 边界记录 "虚拟 chunk" 的文件偏移
- Build Pass 检测 `io.ReaderAt` + segment 数 ≥ 4 时自动走并行路径
- Worker-per-chunk 模型：N 个 goroutine 各自通过 `io.SectionReader` 独立解析指定区间
- 每个 worker 产出本地 `[]edgeRecord`，最后统一 merge 到 `CompactEdgeListBuilder`

核心文件：
- `perflib/parser/hprof/build_pass_parallel.go` (并行 Build Pass 主逻辑)
- `perflib/parser/hprof/scan_pass.go` (增加 SegmentInfo 记录 + 虚拟 chunking)
- `perflib/parser/hprof/core_reader.go` (增加 Position() 方法用于 offset 跟踪)
- `perflib/parser/hprof/build_pass.go` (ParseTwoPass 自动检测并行条件)

性能数据（2.45GB, 16M 对象, 34M 边, 14 workers on Apple M3 Pro）：

| 阶段 | 顺序 | 并行 | 加速 |
|------|------|------|------|
| Build reference edges | 5.78s | 3.37s | **1.72x** |
| Merge worker edges | - | 886ms | - |
| Assemble graph | 1.99s | 2.19s | 0.91x |
| Compute reachability | 123ms | 134ms | - |
| **总 Build Pass** | **8.12s** | **6.63s** | **1.23x** |

瓶颈分析：
- Build reference edges 从 5.78s → 3.37s，达到 1.72x 加速，接近预期
- Merge 阶段 886ms 为预计算容量的一次性合并（38M 条 edge 的 field name intern）
- Assemble graph 2.19s 的主要开销是 `sort.Slice`（19M edges），与顺序版一致
- 进一步优化方向：radix sort 替代 `sort.Slice`（O(n) vs O(n log n)），可在 P7 实施

---

## 更新记录

| 日期 | 内容 |
|------|------|
| 2026-05-06 | 初始方案设计完成，归档 |
| 2026-05-06 | Phase 1 完成：Two-Pass CSR + HeapQueryEngine + CLI 集成 |
| 2026-05-06 | Phase 2 完成：heap_index.bin 序列化格式实现（index_format/writer/reader + roundtrip 测试） |
| 2026-05-06 | Sprint 3 WebUI 集成：重写 refgraph_service.go（HeapDataProvider 接口 + indexedProvider/legacyProvider 双策略）+ 适配 server.go handlers |
| 2026-05-06 | 端到端验证通过：132MB hprof → 1.2s 分析 → 114MB heap_index.bin → WebUI 所有 API 正常 |
| 2026-05-06 | P0 Dominator Tree 集成完成：113ms 完成 120 万对象的 dominator 计算 |
| 2026-05-06 | Dominator 验证通过：retained_size ≠ shallow_size |
| 2026-05-07 | **P0 大文件验证通过**：2.4GB hprof (16M 对象, 34M 边) → **16s 完成分析**（目标 <45s ✅） |
| 2026-05-07 | **P1 Legacy 代码清理**：删除 refgraph.bin 序列化路径 (~4,100 行)，全部测试通过 |
| 2026-05-07 | 整理后续路线图（P2→P4→P3→P5→P6→P7） |
| 2026-05-07 | **P4 Build Pass 并行化完成**：Build reference edges 5.78s→3.37s (1.72x)，总 Build Pass 8.12s→6.63s (1.23x) |
| 2026-05-07 | **P3-P7 设计方案**：更新 heap_index.bin v2 mmap 详细设计、ARCHITECTURE.md 规划、单测策略、WebUI Dominator Tree 视图设计 |
| 2026-05-07 | **perflib 查询接口重构**：HeapGraph 接口 + ObjectInfoAssembler + HeapQueryEngine 解耦（渐进式重构 Phase 1-4） |
| 2026-05-07 | **P3 heap_index.bin v2 mmap 实施完成**：format v2(Section Table + page-aligned) + WriteHeapIndexV2 + MmapHeapIndex(unsafe.Slice zero-copy) + ReadHeapIndex auto-detect v1/v2 + roundtrip 测试通过 |
| 2026-05-07 | **P5 文档完成**：docs/ARCHITECTURE.md (系统架构 + 数据流 + 设计决策) + perflib/parser/hprof/README.md (包文档 + API 指南 + 性能数据) + README.md 更新 |
| 2026-05-07 | **P6 单测补全完成**：dom_indexed.go 0%→100% (15 个测试用例)，修复 2 个 bug (vertex 数组越界 + ancestor sentinel 值冲突)；index_test.go 新增 6 个边界测试（v2 空图、单对象、大 fieldName、Unicode 类名、极值 objectID）；heap_query_engine_test.go 新增 7 组边界测试 |
