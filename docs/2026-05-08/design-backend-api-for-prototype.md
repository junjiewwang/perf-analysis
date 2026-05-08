# PerfScope 原型 — 后端 API 支持分析与实施方案

> 创建日期: 2026-05-08  
> 状态: Sprint 1 ✅ 已完成  
> 关联文档: [React 组件库设计](../2026-05-07/design-react-component-library.md) | [原型文件](../../prototype/)

## 1. 设计原则

**Backend-First Architecture（后端优先架构）**

```
┌─────────────────────────────────────────────────────────┐
│  Frontend (React)                                       │
│  ├─ 纯展示层：接收 JSON → 渲染可视化组件               │
│  ├─ 交互层：用户操作 → 调用 API → 更新视图             │
│  └─ 零计算：不做聚合、排序、过滤、统计等数据处理       │
├─────────────────────────────────────────────────────────┤
│  Backend (Go)                                           │
│  ├─ 分析层：解析 profile → 生成分析数据                 │
│  ├─ 查询层：接收参数 → 排序/过滤/聚合 → 返回结构化 JSON│
│  └─ 计算层：所有 CPU 密集型操作在后端完成               │
└─────────────────────────────────────────────────────────┘
```

**核心原则：**
- 前端只做渲染和交互，不做数据处理
- 所有排序、过滤、聚合、统计在后端完成
- API 返回"即插即用"的数据，前端直接绑定到组件
- 唯一例外：Callers/Callees 可从火焰图树结构在前端推算（避免新增 API）

---

## 2. 原型面板需求 vs 后端 API 覆盖总览

| 面板 | 原型 UI 功能点 | 已有 API | 需新建 API | 覆盖率 |
|------|---------------|----------|-----------|--------|
| **CPU Profile** | 火焰图 + Hot Functions + Callers/Callees + 线程分析 | 6 | 2 | **~75%** |
| **Heap Dump** | Class Histogram + Leak Suspects + Object Explorer + GC Roots | 12 | 2 | **~85%** |
| **Goroutine Dump** | Stack 分组列表 + 并发问题 + 泄漏检测 | 3 | 3 | **~40%** |

---

## 3. CPU Profile 面板详细分析

### 3.1 已有 API 覆盖

| 原型功能 | 对应 API | 返回数据 | 状态 |
|---------|---------|---------|------|
| **火焰图渲染** | `GET /api/flamegraph?task=<id>&type=cpu` | `FlameGraph{Root, TotalSamples, MaxDepth, ThreadAnalysis}` | ✅ 完全覆盖 |
| **Hot Functions 表格** | 同上（`ThreadAnalysis.TopFunctions`） | `[]TopFunction{Name, Module, Samples, Percentage, ThreadCount, Threads}` | ✅ 完全覆盖 |
| **线程列表** | 同上（`ThreadAnalysis.Threads`） | `[]ThreadInfo{TID, Name, Group, Samples, Percentage, TopFunctions}` | ✅ 完全覆盖 |
| **线程分组** | 同上（`ThreadAnalysis.ThreadGroups`） | `[]ThreadGroupInfo{Name, ThreadCount, TotalSamples, Percentage}` | ✅ 完全覆盖 |
| **摘要统计** | 同上（`ThreadAnalysis` 顶层字段） | `TotalThreads, ActiveThreads, UniqueFunctions` | ✅ 完全覆盖 |
| **调用图** | `GET /api/callgraph?task=<id>&type=cpu` | 调用图 JSON | ✅ 已有 |

### 3.2 需新建 API

| 原型功能 | 建议 API | 优先级 | 说明 |
|---------|---------|--------|------|
| **按线程过滤火焰图** | `GET /api/flamegraph?task=<id>&type=cpu&tid=<TID>` | P1 | 后端已有 `ThreadInfo.FlameRoot` 数据结构，只需在 handler 中添加 tid 参数过滤逻辑 |
| **全局搜索** | `GET /api/search?task=<id>&q=<query>&type=<function\|thread>&limit=<N>` | P2 | 后端 `CPUAnalysisResult.Search()` 方法已实现，只需暴露为 HTTP API |

### 3.3 前端可计算（无需新 API）

| 功能 | 计算方式 | 复杂度 |
|------|---------|--------|
| **Callers/Callees** | 遍历 `FlameGraph.Root` 树结构，查找目标函数的父节点（callers）和子节点（callees） | 低，纯树遍历 |
| **火焰图交互**（zoom、highlight、tooltip） | 前端组件内部状态管理 | 中，属于渲染层 |

### 3.4 数据结构映射

```typescript
// 前端 TypeScript 类型直接映射后端 JSON
interface FlameGraphResponse {
  root: FlameNode;
  total_samples: number;
  max_depth: number;
  thread_analysis?: ThreadAnalysisData;
}

interface ThreadAnalysisData {
  total_threads: number;
  active_threads: number;
  unique_functions: number;
  threads: ThreadInfo[];
  top_functions: TopFunction[];
  thread_groups: ThreadGroupInfo[];
}

interface TopFunction {
  name: string;
  module: string;
  samples: number;
  percentage: number;
  thread_count: number;
  threads?: ThreadFunctionInfo[];
}
```

---

## 4. Heap Dump 面板详细分析

### 4.1 已有 API 覆盖

| 原型功能 | 对应 API | 返回数据 | 状态 |
|---------|---------|---------|------|
| **Class Histogram（按类排序的最大对象）** | `GET /api/biggest-objects?task=<id>&sort=retained&top=50` | `[]{object_id, class_name, shallow_size, retained_size}` | ⚠️ 部分覆盖（按对象粒度，非按类聚合） |
| **对象字段探索** | `GET /api/refgraph/fields?task=<id>&id=<objectID>` | `[]{name, type, value, ref_id, ref_class, shallow_size, retained_size, has_children}` | ✅ 完全覆盖 |
| **对象基本信息** | `GET /api/refgraph/info?task=<id>&id=<objectID>` | `{object_id, class_name, shallow_size, retained_size}` | ✅ 完全覆盖 |
| **GC Root 路径** | `GET /api/refgraph/gc-roots?task=<id>&id=<objectID>` | GC Root 路径列表 | ✅ 完全覆盖 |
| **GC Root 概览** | `GET /api/refgraph/gc-roots-summary?task=<id>` | 按类分组的 GC Root 摘要 | ✅ 完全覆盖 |
| **GC Root 列表** | `GET /api/refgraph/gc-roots-list?task=<id>` | 完整 GC Root 列表 | ✅ 完全覆盖 |
| **Retainer 分析** | `GET /api/refgraph/retainers?task=<id>&id=<objectID>` | 引用关系列表 | ✅ 完全覆盖 |
| **Dominator Tree** | `GET /api/refgraph/dominator-tree?task=<id>&id=<objectID>` | 被支配的子对象 | ✅ 完全覆盖 |
| **Dominator 路径** | `GET /api/refgraph/dominator-path?task=<id>&id=<objectID>` | 支配路径 | ✅ 完全覆盖 |
| **Treemap 可视化** | `GET /api/refgraph/treemap?task=<id>&root=<objectID>&maxNodes=<N>` | Treemap 节点数据 | ✅ 完全覆盖 |
| **泄漏检测** | `GET /api/pprof/leak-report?task=<id>&type=heap` | 泄漏检测报告 | ✅ 完全覆盖 |
| **按类查最大对象** | `GET /api/refgraph/biggest-by-class?task=<id>&class=<name>` | 指定类的最大对象列表 | ✅ 完全覆盖 |

### 4.2 需新建 API

| 原型功能 | 建议 API | 优先级 | 说明 |
|---------|---------|--------|------|
| **Class Histogram（按类聚合统计）** | `GET /api/refgraph/class-histogram?task=<id>&sort=<retained\|shallow\|count>&top=<N>` | **P0** | 按类名聚合对象数量、shallow size 总和、retained size 总和。后端 HeapQueryEngine 已有数据基础，需新增聚合逻辑 |
| **Heap 概览统计** | `GET /api/refgraph/heap-stats?task=<id>` | **P0** | 返回堆总大小、对象总数、类总数、GC Root 总数等概览信息 |

### 4.3 Class Histogram API 设计

```go
// Request: GET /api/refgraph/class-histogram?task=<id>&sort=retained&top=50&filter=<className>
// Response:
type ClassHistogramResponse struct {
    TotalClasses  int                    `json:"total_classes"`
    TotalObjects  int64                  `json:"total_objects"`
    TotalSize     int64                  `json:"total_size"`
    Classes       []ClassHistogramEntry  `json:"classes"`
}

type ClassHistogramEntry struct {
    ClassName    string  `json:"class_name"`
    ObjectCount  int64   `json:"object_count"`
    ShallowSize  int64   `json:"shallow_size"`
    RetainedSize int64   `json:"retained_size"`
    Percentage   float64 `json:"percentage"` // 占总 retained size 百分比
}
```

### 4.4 Heap Stats API 设计

```go
// Request: GET /api/refgraph/heap-stats?task=<id>
// Response:
type HeapStatsResponse struct {
    TotalHeapSize    int64  `json:"total_heap_size"`
    TotalObjects     int64  `json:"total_objects"`
    TotalClasses     int    `json:"total_classes"`
    TotalGCRoots     int    `json:"total_gc_roots"`
    MaxObjectSize    int64  `json:"max_object_size"`
    MaxRetainedSize  int64  `json:"max_retained_size"`
    TopClassName     string `json:"top_class_name"`
}
```

---

## 5. Goroutine Dump 面板详细分析

### 5.1 已有 API 覆盖

| 原型功能 | 对应 API | 返回数据 | 状态 |
|---------|---------|---------|------|
| **Goroutine 火焰图** | `GET /api/flamegraph?task=<id>&type=goroutine` | `FlameGraph{Root, TotalSamples}` | ✅ 完全覆盖 |
| **Goroutine 泄漏检测** | `GET /api/pprof/leak-report?task=<id>&type=goroutine` | `PProfLeakReport` | ✅ 完全覆盖 |
| **批量分析概览** | `GET /api/pprof/batch-analysis?task=<id>` | 包含 goroutine 的批量数据 | ✅ 间接覆盖 |

### 5.2 需新建 API

| 原型功能 | 建议 API | 优先级 | 说明 |
|---------|---------|--------|------|
| **Goroutine 分组列表** | `GET /api/goroutine/groups?task=<id>&sort=<count\|percentage>&top=<N>` | **P0** | 按调用栈分组的 goroutine 分布。后端 `PProfGoroutineData.Distribution` 已有完整数据，只需暴露为 API |
| **Goroutine 统计概览** | `GET /api/goroutine/stats?task=<id>` | P1 | 总数、状态分布、异常检测结论。数据来源于 `PProfGoroutineData.TotalCount` + `Distribution` |
| **并发问题检测** | `GET /api/goroutine/issues?task=<id>` | P2 | 基于 goroutine 分布和泄漏报告，识别潜在并发问题（死锁风险、goroutine 泄漏、过度并发等） |

### 5.3 Goroutine Groups API 设计

```go
// Request: GET /api/goroutine/groups?task=<id>&sort=count&top=20
// Response:
type GoroutineGroupsResponse struct {
    TotalCount  int64            `json:"total_count"`
    GroupCount  int              `json:"group_count"`
    Groups      []GoroutineGroup `json:"groups"`  // 复用已有的 model.GoroutineGroup
    TopFuncs    []PProfTopFunc   `json:"top_funcs"`
}

// model.GoroutineGroup 已有完美的数据结构：
// type GoroutineGroup struct {
//     Count      int64    `json:"count"`
//     Percentage float64  `json:"percentage"`
//     State      string   `json:"state,omitempty"`
//     TopFunc    string   `json:"top_func"`
//     Stack      []string `json:"stack,omitempty"`
// }
```

### 5.4 实现难度评估

| API | 难度 | 工作量 | 原因 |
|-----|------|--------|------|
| `/api/goroutine/groups` | **极低** | 0.5 天 | 数据已在分析阶段生成并保存为 JSON，只需读取并返回 |
| `/api/goroutine/stats` | **低** | 0.5 天 | 从 groups 数据聚合统计 |
| `/api/goroutine/issues` | **中** | 1-2 天 | 需设计并发问题检测规则（可复用泄漏报告逻辑） |

---

## 6. 新建 API 完整清单（按优先级）

### P0 — 原型核心功能必需

| # | API | 面板 | 实现难度 | 说明 |
|---|-----|------|---------|------|
| 1 | `GET /api/goroutine/groups` | Goroutine | 极低 | 数据已存在，仅需 handler |
| 2 | `GET /api/refgraph/class-histogram` | Heap | 低 | HeapQueryEngine 需新增聚合查询 |
| 3 | `GET /api/refgraph/heap-stats` | Heap | 极低 | 索引文件中已有元数据 |

### P1 — 体验增强

| # | API | 面板 | 实现难度 | 说明 |
|---|-----|------|---------|------|
| 4 | `GET /api/flamegraph` (tid 参数) | CPU | 极低 | handler 添加过滤逻辑 |
| 5 | `GET /api/goroutine/stats` | Goroutine | 低 | 从 groups 聚合 |
| 6 | `GET /api/search` | 全局 | 低 | `CPUAnalysisResult.Search()` 已实现 |

### P2 — 高级分析

| # | API | 面板 | 实现难度 | 说明 |
|---|-----|------|---------|------|
| 7 | `GET /api/goroutine/issues` | Goroutine | 中 | 需设计检测规则 |

---

## 7. 前端实现复杂度评估

### 7.1 组件复杂度分级

| 复杂度 | 组件 | 说明 |
|--------|------|------|
| **高** | FlameGraph（火焰图渲染器） | Canvas/SVG 高性能渲染、缩放、交互、搜索高亮。建议使用 d3-flame-graph 或自研 Canvas 方案 |
| **高** | Treemap（树图） | 嵌套矩形布局算法、缩放交互。建议使用 d3-treemap |
| **中** | ObjectExplorer（对象探索树） | 树形懒加载、展开/折叠、异步加载子节点 |
| **中** | CallGraph（调用图） | 有向图布局渲染。建议使用 dagre + d3 |
| **低** | ClassHistogram（类直方图表） | 纯表格 + 排序 + 过滤 + 进度条 |
| **低** | HotFunctions（热点函数表） | 纯表格 + 排序 + 跳转 |
| **低** | GoroutineList（Goroutine 分组列表） | 可折叠列表 + 状态 badge |
| **低** | LeakSuspects（泄漏嫌疑列表） | 卡片列表 + 严重度标记 |
| **极低** | StatsOverview（概览统计卡片） | 纯数字展示 |
| **极低** | SessionSidebar（会话侧边栏） | 静态列表 + 点击切换 |

### 7.2 "后端优先"下的前端职责

```
前端做什么：
  ✅ 调用 API 获取数据
  ✅ 将 JSON 绑定到组件 props
  ✅ 渲染可视化（火焰图、Treemap、表格）
  ✅ 处理用户交互（点击、缩放、搜索输入）
  ✅ 管理视图状态（当前选中面板、展开节点等）
  ✅ 从火焰图树推算 Callers/Callees（唯一的前端计算）

前端不做什么：
  ❌ 数据排序（后端 sort 参数处理）
  ❌ 数据过滤（后端 filter/query 参数处理）
  ❌ 数据聚合（后端返回聚合后的结果）
  ❌ 统计计算（后端返回百分比/总和等）
  ❌ 搜索匹配（后端全文搜索 API）
```

### 7.3 前端工作量估算

| 模块 | 工作量 | 依赖 |
|------|--------|------|
| API Client 层（TypeScript 类型 + fetch 封装） | 2 天 | - |
| FlameGraph 组件（含交互） | 5 天 | d3-flame-graph 或自研 |
| Treemap 组件 | 3 天 | d3-treemap |
| ObjectExplorer 树组件 | 3 天 | - |
| 表格组件（Hot Functions / Class Histogram / Goroutine Groups） | 3 天 | 可复用同一基础表格 |
| 布局框架（三栏 + 面板切换 + 主题） | 2 天 | - |
| 其他小组件（Stats、Sidebar、Badges 等） | 2 天 | - |
| **总计** | **~20 天** | |

---

## 8. 实施路线图

### Sprint 1：后端 API 补全（3-5 天）

```
1. [P0] 新增 /api/goroutine/groups handler（0.5 天）
   - 读取分析输出中的 goroutine 数据 JSON
   - 返回 GoroutineGroupsResponse
   
2. [P0] 新增 /api/refgraph/class-histogram（1-2 天）
   - HeapQueryEngine 新增 ClassHistogram 查询方法
   - 遍历 heap index 按类名聚合 count/size
   - 支持 sort、top、filter 参数
   
3. [P0] 新增 /api/refgraph/heap-stats（0.5 天）
   - 从 heap index 元数据提取统计信息
   
4. [P1] flamegraph handler 支持 tid 过滤（0.5 天）
   - 已有 ThreadInfo.FlameRoot，按 tid 参数返回对应子树
   
5. [P1] 新增 /api/search（0.5 天）
   - 封装已有的 CPUAnalysisResult.Search() 为 HTTP handler
```

### Sprint 2：React 基础架构（5 天）

```
1. 搭建 Monorepo（pnpm workspace + Turborepo）
2. @perf-analysis/api-client 包开发（TypeScript 类型 + fetch）
3. 基础布局组件（AppShell、Sidebar、Panel）
4. 主题系统（PerfScope 深色主题）
```

### Sprint 3：核心可视化组件（8 天）

```
1. FlameGraph 组件（高优）
2. 表格基础组件 + HotFunctions / ClassHistogram 实例
3. GoroutineList 组件
4. StatsOverview 组件
```

### Sprint 4：高级功能（7 天）

```
1. Treemap 组件
2. ObjectExplorer 树组件
3. CallGraph 组件
4. 搜索 + 联动交互
```

---

## 9. 验收标准

| 阶段 | 验收条件 |
|------|---------|
| Sprint 1 | 所有 P0/P1 API 通过单元测试，Postman/curl 可正常调用 |
| Sprint 2 | `pnpm dev` 启动后看到原型布局，面板切换正常 |
| Sprint 3 | 火焰图能渲染真实数据，表格支持排序，goroutine 列表可折叠 |
| Sprint 4 | 所有面板功能完整，交互流畅，与原型设计一致 |

---

## 10. 遗留问题

1. **Go pprof Heap 的 Class Histogram**：Go 的 pprof heap 没有 Java hprof 那样的类概念，需要按分配点（函数）聚合而非类名。这部分 API 设计需要区分 Java Heap Dump 和 Go pprof Heap 两种场景。

2. **Goroutine Issues 检测规则**：需要定义什么场景算"并发问题"（如：同一锁上阻塞的 goroutine 超过阈值、某个 goroutine 组数量异常增长等）。建议先从泄漏检测复用，后续迭代增加规则。

3. **实时 vs 静态分析**：当前架构是"分析时计算，展示时读取"，新 API 也应遵循此原则，优先读取预计算数据而非实时计算。

4. **火焰图组件选型**：d3-flame-graph 是成熟方案但可定制性有限，自研 Canvas 方案性能更好但开发成本高。建议 Sprint 3 先用 d3-flame-graph 快速验证，后续根据性能需求决定是否自研。

---

## 11. Sprint 1 实施记录

### 已完成（2026-05-08）

#### 新增 perflib 公共库（可复用）

| 包 | 文件 | 说明 |
|---|------|------|
| `perflib/output` | `writer.go` | 通用分析输出写入工具（JSON/Gzip-JSON 读写、文件查找） |
| `perflib/query` | `goroutine_query.go` | Goroutine 查询引擎（Groups/Stats/Issues） |
| `perflib/query` | `heap_query.go` | Heap 查询工具（ClassHistogram/HeapStats） |
| `perflib/query` | `search.go` | 跨数据源搜索引擎 |

#### 测试覆盖

| 文件 | 测试数 | 覆盖 |
|------|--------|------|
| `perflib/output/writer_test.go` | 6 | WriteJSON/Gzip/FindFile/IsGzipped |
| `perflib/query/goroutine_query_test.go` | 9 | QueryGroups/Stats/Issues/NilData |
| `perflib/query/search_test.go` | 9 | SearchFunctions/Threads/Goroutine/Sort/Limit |

#### 修改的现有文件

| 文件 | 变更 |
|------|------|
| `perflib/analyzer/pprof_goroutine_analyzer.go` | 分析完成后持久化 `goroutine_analysis.json` |
| `internal/webui/server.go` | 注册 7 个新 API 路由 + flamegraph tid 过滤 |
| `internal/webui/refgraph_service.go` | 新增 `GetHeapQueryHelper()` 方法 |
| `internal/webui/heap_query_engine.go` | 新增 `GetGraph()` 方法暴露 HeapGraph |

#### 新增的 API Handler 文件

| 文件 | 包含 Handlers |
|------|-------------|
| `internal/webui/pprof_api_handlers.go` | 所有 6 个新 API handler |

#### API 清单

| API | 方法 | 参数 | 状态 |
|-----|------|------|------|
| `/api/goroutine/groups` | GET | `task`, `sort`, `top` | ✅ |
| `/api/goroutine/stats` | GET | `task` | ✅ |
| `/api/goroutine/issues` | GET | `task` | ✅ |
| `/api/refgraph/class-histogram` | GET | `task`, `sort`, `top`, `filter` | ✅ |
| `/api/refgraph/heap-stats` | GET | `task` | ✅ |
| `/api/search` | GET | `task`, `q`, `type`, `limit` | ✅ |
| `/api/flamegraph?tid=<TID>` | GET | 原有参数 + `tid` | ✅ |
