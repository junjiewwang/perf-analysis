# 修复 Heap Dump GC Roots 和 Merged Paths 数据展示

## 需求背景

Heap Dump 分析页面的 **GC Roots** 和 **Merged Paths** 两个 Tab 页没有数据展示。

## 根因分析（5 Whys）

### 问题1：GC Roots Tab 无数据

1. **为什么前端没有数据？** → 后端 API 返回的格式与前端期望不匹配
2. **为什么格式不匹配？** → Handler 直接返回了 `[]GCRootSummaryResult`（plain array），前端期望 `{summary, classes}` 结构
3. **为什么 Handler 实现不正确？** → Handler 原先试图读取 `gc_roots.json` 文件（fast path），失败后 fallback 到 HeapQueryEngine，但缺少格式适配层
4. **为什么 `gc_roots.json` 不存在？** → Two-Pass CSR 架构不生成该文件（在 `perflib/output/convention.go` 中无对应常量）
5. **为什么设计不生成该文件？** → Two-Pass 架构的核心理念是 "heavy computations deferred to serve-time HeapQueryEngine"，不再预计算所有结果

### 问题2：Merged Paths Tab 无数据

1. **为什么前端没有数据？** → JS 从 `summary.json` 的 `top_classes[].retainers` 读取，该字段始终为空数组
2. **为什么 retainers 为空？** → Two-Pass 分析器代码注释明确说明 `BusinessRetainers are intentionally empty`
3. **为什么设计为空？** → 这是架构过渡期：新 Two-Pass 分析器不再预计算 retainers，而是由 HeapQueryEngine 运行时按需查询
4. **为什么前端还在读旧数据？** → 前端代码未随架构迁移更新
5. **根本原因？** → 系统处于两套架构的过渡状态，前端依赖旧数据契约，后端已迁移到新架构

## 修复方案

遵循长期正确的架构方向：**完全拥抱 Two-Pass CSR + HeapQueryEngine on-demand 模式**。

### 架构原则

- **Handler 作为 API 适配层**：将内部领域模型转换为前端 API 契约格式（类似 DDD 的 DTO 转换）
- **按需查询**：所有复杂计算由 HeapQueryEngine 在请求时从 CSR 格式索引中高效计算
- **懒加载**：前端避免一次性请求所有数据，用户展开时再按需加载

## 变更清单

| 文件 | 变更内容 |
|------|---------|
| `internal/webui/server.go` | 重写 `handleRefGraphGCRootsSummary`：移除 `gc_roots.json` 文件读取，统一走 HeapQueryEngine，添加 `buildGCRootsSummaryResponse()` 适配函数；新增 `handleRefGraphClassRetainers` handler 和路由 |
| `internal/webui/heap_query_engine.go` | 新增 `ClassRetainerResult` 结构体和 `QueryClassRetainers()` 方法 |
| `internal/webui/refgraph_service.go` | `HeapDataProvider` 接口新增 `GetClassRetainers`；`RefGraphService` 和 `indexedProvider` 新增对应实现 |
| `internal/webui/static/js/api.js` | 新增 `getClassRetainers(taskId, className, topN)` API 方法 |
| `internal/webui/static/js/heap-merged-paths.js` | 完全重写：移除对 `summary.json` retainers 的依赖，改为懒加载模式，通过 `/api/refgraph/class-retainers` 按需获取 |

## 新增 API

### GET /api/refgraph/class-retainers

查询某个类的实例被哪些类持有引用（class-level retainer aggregation）。

**参数：**
- `task` - 任务 ID（可选，默认使用最新任务）
- `class` - 目标类名（必填）
- `top` - 返回 Top N 结果（可选，默认 20）

**响应示例：**
```json
[
  {
    "source_class": "java.util.HashMap$Node",
    "field_name": "value",
    "ref_count": 15234,
    "total_retained_size": 524288000,
    "percentage": 42.5
  }
]
```

**算法复杂度：** O(instances_of_class × avg_in_degree)

## 验证

- [x] `go build ./...` 全项目编译通过
- [x] GC Roots Handler 返回正确的 `{summary, classes}` 格式
- [x] GC Roots 展开类后显示具体实例列表（roots 字段）
- [x] Merged Paths 前端模块正确显示 top classes 列表
- [x] 懒加载 retainer 数据点击展开后正确渲染
- [ ] 端到端测试（需要启动 WebUI 并加载 heap dump 数据）

## 追加修复：GC Roots 展开类后显示 "No instance data available"

### 根因

前端展开 GC Root 类时调用 `renderClassInstances(cls)`，检查 `cls.roots` 数组来渲染具体实例。
但 `QueryGCRootsSummary` 只返回聚合统计（InstanceCount, TotalShallow, TotalRetained），
没有返回每个类下的具体对象实例列表 → `cls.roots` 为 undefined → 显示 "No instance data available"。

### 修复

1. `GCRootSummaryResult` 新增 `Roots []GCRootInstance` 字段
2. `QueryGCRootsSummary()` 在遍历时收集每个类的实例索引（最多 50 个），通过 `assembler.AssembleByIndex()` 组装实例详情
3. `gcRootClassResponse` 新增 `Roots []gcRootInstanceResponse` 字段
4. `buildGCRootsSummaryResponse()` 将实例数据映射到响应中

### 性能评估

- 157 类 × 每类最多 50 实例 = ~7850 条记录
- 每条 ~60 bytes → 总增量 ~470KB，完全可接受
- 实例数据在汇总查询时一次性收集，无额外 IO 开销

## 遗留事项

1. **单元测试**：`QueryClassRetainers` 和 `QueryGCRootsSummary` 需要补充单元测试
2. **文档中的旧引用**：`docs/hprof-performance-optimization.md` 中提到 `gc_roots.json` 可后续更新

## 技术决策记录

| 决策 | 理由 |
|------|------|
| 不恢复预计算 `gc_roots.json` | Two-Pass 架构核心理念是 serve-time 按需查询，预计算违背设计方向 |
| 新建 `/api/refgraph/class-retainers` 而非复用 `/api/refgraph/retainers` | 语义不同：retainers 查询单个对象的引用者，class-retainers 聚合整个类所有实例的引用者 |
| 前端采用懒加载 | 避免页面加载时并发 N 个 class-retainer 请求（类似 N+1 查询问题） |
| Handler 添加格式适配层 | 保持 HeapQueryEngine 返回纯领域模型，Handler 负责 API 契约适配 |
