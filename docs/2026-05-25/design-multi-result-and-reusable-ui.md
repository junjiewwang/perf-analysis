# PerfScope — 多结果展示与可复用 UI 设计方案

> 创建日期: 2026-05-25  
> 状态: 原型导航已实施，React 架构待启动  
> 关联文档: [后端 API 设计](../2026-05-08/design-backend-api-for-prototype.md) | [原型文件](../../prototype/)

## 1. 背景与目标

### 1.1 需求

1. **多结果展示** — 如何在 UI 中展示多个分析任务的结果，支持切换和对比
2. **可复用 UI** — 如何对外提供独立的、可嵌入的分析视图页面，支持按需展示
3. **React 演进** — 前端后续从原生 HTML/JS 演进到 React 组件架构

### 1.2 现状

| 层面 | 现有能力 |
|------|----------|
| 后端 | `GET /api/tasks` 返回所有 Task 列表；所有 API 接受 `?task=<id>` 参数 |
| 前端 WebUI | `api.js` 的 `getTasks()` 动态加载任务列表 |
| 原型 | 左侧 Session 列表（静态硬编码），Profile 类型切换器（CPU/Heap/Goroutine） |

**核心观察**：后端已天然支持多任务路由，瓶颈在前端设计层。

---

## 2. 多结果展示 — Task-Based Three-Level Navigation

### 2.1 当前原型的问题

Session List 是扁平列表 + 直接绑定 panel 切换，无法区分：
- "同一次分析的多种数据类型"（一个 task 可能同时包含 CPU + Heap + Goroutine）
- "不同时间的多次分析"（不同 task）

### 2.2 推荐方案：三级路由导航

```
┌──────────────────────────────────────────────────────────────────┐
│ Top Bar: Logo + Task Selector (Dropdown) + Quick Search (⌘K)     │
├─────────┬────────────────────────────────────────────────────────┤
│         │ Profile Type Tabs: [CPU] [Heap] [Goroutine] [Block]... │
│  Task   ├────────────────────────────────────────────────────────┤
│  List   │ View Sub-Tabs: Flame Graph | Top Down | Treemap | ...  │
│ (Left)  │────────────────────────────────────────────────────────│
│         │                                                         │
│ ● Task1 │              Main Visualization                         │
│   Task2 │                                                         │
│   Task3 │                                                         │
│         │                                                         │
├─────────┴────────────────────────────────────────────────────────┤
│ Status Bar                                                        │
└──────────────────────────────────────────────────────────────────┘
```

| 导航层级 | 载体 | 说明 |
|---------|------|------|
| 第一层 | 左侧 Task 列表（按时间/服务分组） | 对应一次完整分析 |
| 第二层 | Profile Type Tabs（顶栏） | 同一 Task 下的不同数据类型，动态显示可用类型 |
| 第三层 | View Sub-Tabs（面板内） | 同一数据类型的不同可视化视角 |

### 2.3 URL 路由设计

```
/#/tasks/<taskId>/<profileType>/<view>

示例：
/#/tasks/abc123/cpu/flamegraph
/#/tasks/abc123/heap/histogram
/#/tasks/abc123/goroutine/groups
```

### 2.4 后端适配：增强 `/api/tasks` 响应

```typescript
interface TaskInfo {
  id: string;
  created_at: string;
  service_name?: string;       // 服务名
  available_types: string[];   // 该 task 有哪些数据类型 ["cpu", "heap", "goroutine"]
  summary?: {                  // 快速预览摘要
    cpu_hot_path?: string;
    heap_size?: string;
    goroutine_count?: number;
  };
}
```

前端根据 `available_types` 动态显示/隐藏 Profile Type Tabs。

---

## 3. 可复用 UI — 对外嵌入架构

### 3.1 方案对比

| 方案 | 嵌入方式 | 隔离性 | 复用粒度 | 交互通信 | 适用场景 |
|------|---------|--------|---------|---------|---------|
| **A. iframe embed** | `<iframe src="/embed/flamegraph?task=xx">` | ★★★★★ | 页面级 | postMessage | 非 React 系统、快速集成 |
| **B. React 组件** | `import { FlameGraph } from '@perf-scope/ui'` | ★★★★ | 组件级 | Props + Events | React 系统深度集成 |
| **C. 完整应用** | 独立部署 `@perf-scope/app` | ★★★★★ | 应用级 | URL | 独立运维 |

**推荐**：B（主力）+ A（兜底），覆盖所有消费场景。

### 3.2 Embed Route 设计（iframe 方式）

```
/embed/<view>?task=<id>&type=<type>&theme=<dark|light>&compact=<true|false>

示例：
/embed/flamegraph?task=abc123&type=cpu       → 单独的火焰图
/embed/histogram?task=abc123                 → 单独的 Class Histogram
/embed/gc-roots?task=abc123                  → 单独的 GC Roots 树
/embed/goroutine-groups?task=abc123          → 单独的 Goroutine 分组
/embed/leak-suspects?task=abc123             → 单独的泄漏检测
/embed/summary?task=abc123                   → 概览仪表盘
```

**路由参数：**

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `task` | 分析任务 ID（必须） | - |
| `type` | 数据类型（部分 view 需要） | 自动检测 |
| `theme` | 主题 | `dark` |
| `compact` | 紧凑模式（隐藏 Toolbar） | `false` |
| `interactive` | 是否允许交互 | `true` |

**外部消费示例：**

```html
<!-- 监控平台告警详情页中嵌入火焰图 -->
<iframe 
  src="https://perfscope.internal/embed/flamegraph?task=abc&type=cpu&compact=true"
  width="100%" height="500" frameborder="0">
</iframe>
```

### 3.3 React 组件方式（主力）

```tsx
import { FlameGraph, GCRoots, ApiProvider } from '@perf-scope/ui';

function MyPage() {
  return (
    <ApiProvider baseURL="https://perfscope.internal">
      <FlameGraph task="abc123" type="cpu" onFrameClick={handleFrame} />
      <GCRoots task="abc123" />
    </ApiProvider>
  );
}
```

---

## 4. React 架构设计

### 4.1 Monorepo 包结构

```
packages/
├── @perf-scope/core           ← 核心类型定义 + API Client
│   ├── src/
│   │   ├── types.ts           ← 所有后端 API 响应的 TypeScript 类型
│   │   ├── api-client.ts      ← fetch 封装（支持自定义 baseURL）
│   │   └── index.ts
│   └── package.json
│
├── @perf-scope/ui             ← 可复用的分析视图组件库
│   ├── src/
│   │   ├── flamegraph/        ← <FlameGraph task={id} type="cpu" />
│   │   ├── histogram/         ← <ClassHistogram task={id} />
│   │   ├── gc-roots/          ← <GCRoots task={id} />
│   │   ├── merged-paths/      ← <MergedPaths task={id} />
│   │   ├── goroutine-groups/  ← <GoroutineGroups task={id} />
│   │   ├── treemap/           ← <Treemap task={id} />
│   │   ├── dominator-tree/    ← <DominatorTree task={id} />
│   │   ├── leak-suspects/     ← <LeakSuspects task={id} />
│   │   └── index.ts           ← 按需 export
│   └── package.json
│
├── @perf-scope/app            ← 完整的 PerfScope 应用
│   ├── src/
│   │   ├── App.tsx
│   │   ├── router.tsx         ← /tasks/:taskId/:type/:view
│   │   ├── layouts/           ← AppShell, Sidebar, ContextPanel
│   │   ├── pages/             ← CPU/Heap/Goroutine 面板页
│   │   └── providers/         ← ApiProvider, ThemeProvider, TaskProvider
│   └── package.json
│
└── @perf-scope/embed          ← 轻量 embed 入口（给 iframe 用）
    ├── src/
    │   ├── entry-flamegraph.tsx
    │   ├── entry-histogram.tsx
    │   └── ...                ← 每个 view 一个入口
    └── package.json
```

### 4.2 组件接口契约

所有可视化组件遵循统一的双模式 Props 设计：

```tsx
interface ViewComponentProps<T> {
  // === 数据源（二选一） ===
  task?: string;              // 模式A：传 taskId，组件内部调 API
  data?: T;                   // 模式B：传数据，纯展示（受控模式）

  // === 外观 ===
  theme?: 'dark' | 'light';
  compact?: boolean;
  className?: string;
  style?: React.CSSProperties;

  // === 交互回调 ===
  onSelect?: (item: any) => void;
  onDrillDown?: (item: any) => void;

  // === API 配置（模式A 时生效） ===
  apiBase?: string;
}
```

### 4.3 React Router 路由

```tsx
<Routes>
  {/* 任务列表首页 */}
  <Route path="/" element={<TaskListPage />} />
  
  {/* 单个任务：自动跳转到第一个可用类型 */}
  <Route path="/tasks/:taskId" element={<TaskRedirect />} />
  
  {/* 具体分析视图 */}
  <Route path="/tasks/:taskId/:profileType" element={<AnalysisLayout />}>
    <Route index element={<DefaultView />} />
    <Route path="flamegraph" element={<LazyFlameGraph />} />
    <Route path="histogram" element={<LazyHistogram />} />
    <Route path="gc-roots" element={<LazyGCRoots />} />
    <Route path="merged-paths" element={<LazyMergedPaths />} />
    <Route path="treemap" element={<LazyTreemap />} />
    <Route path="dominator-tree" element={<LazyDominatorTree />} />
    <Route path="goroutine-groups" element={<LazyGoroutineGroups />} />
  </Route>
  
  {/* Embed 单视图（无 Shell，给 iframe 用） */}
  <Route path="/embed/:view" element={<EmbedWrapper />} />
</Routes>
```

### 4.4 按需加载

```tsx
const LazyFlameGraph = React.lazy(() => import('@perf-scope/ui/flamegraph'));
const LazyHistogram = React.lazy(() => import('@perf-scope/ui/histogram'));
const LazyGCRoots = React.lazy(() => import('@perf-scope/ui/gc-roots'));

// 使用
<Suspense fallback={<ViewSkeleton />}>
  <Outlet />
</Suspense>
```

---

## 5. 按需加载架构

```
                       ┌────────────────────────┐
                       │   Entry Point (HTML)   │
                       │   只加载 Shell + Router │
                       └───────────┬────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              ▼                    ▼                    ▼
   ┌──────────────────┐  ┌─────────────────┐  ┌──────────────────┐
   │  cpu-panel.js    │  │ heap-panel.js   │  │ goroutine-panel.js│
   │  (React.lazy)    │  │ (React.lazy)    │  │ (React.lazy)      │
   └──────┬───────────┘  └──────┬──────────┘  └──────┬───────────┘
          │                      │                     │
   ┌──────┴──────────┐   ┌──────┴────────┐    ┌──────┴──────────┐
   │ flamegraph.js   │   │ histogram.js  │    │ group-list.js   │
   │ top-down.js     │   │ gc-roots.js   │    │ block-profile.js│
   │ call-tree.js    │   │ merged-paths.js│    │ timeline.js     │
   │ treemap.js      │   │ treemap.js    │    └─────────────────┘
   │ timeline.js     │   │ dominator.js  │
   └─────────────────┘   └───────────────┘
```

- **Shell 只加载路由和 API 模块**（< 10KB gzip）
- **面板级 React.lazy**：切换到 Heap 面板时才加载
- **视图级 React.lazy**：切 Tab 时才加载具体视图代码
- **共享 chunks**：火焰图在 CPU 和 Goroutine 面板都用 → Vite 自动抽为共享 chunk

---

## 6. 技术选型

| 维度 | 选型 | 理由 |
|------|------|------|
| 构建工具 | **Vite** | 快、React 生态一等公民、按需 build |
| 路由 | **React Router v6** | 嵌套路由天然契合三级导航 |
| 状态管理 | **Zustand** | 轻量、避免 Context 性能问题 |
| 样式 | **Tailwind CSS** | 组件库 + 主题系统、与深色设计契合 |
| 火焰图渲染 | **Canvas 自研** | 性能关键路径，需 60fps 缩放 |
| 图表 | **ECharts（Treemap）** | 按需引入 |
| 组件文档 | **Storybook** | 可视化开发 + 对外文档 |
| Monorepo | **pnpm workspace + Turborepo** | 多包管理 |

---

## 7. 过渡策略（现有 → React）

### Phase 0（当前已完成）
- 原生 HTML + JS 模块化 (`internal/webui/static/js/*.js`)
- 后端 API 完善

### Phase 1：搭建 React 工程 + 核心组件
- 新建 `web/` 目录（独立 React 项目）
- 实现 `@perf-scope/core`（API Client + Types）
- 实现核心组件（FlameGraph、ClassHistogram）
- 构建产物 embed 到 Go binary 或独立部署

**文件结构：**

```
perf-analysis/
├── web/                        ← 新增 React 前端
│   ├── package.json
│   ├── tsconfig.json
│   ├── vite.config.ts          ← 输出到 internal/webui/dist/
│   ├── src/
│   │   ├── main.tsx
│   │   ├── App.tsx
│   │   ├── api/               ← @perf-scope/core 原型
│   │   ├── components/        ← @perf-scope/ui 原型
│   │   └── pages/
│   └── index.html
├── internal/webui/
│   ├── server.go              ← 保持不变，增加 serve React dist
│   └── static/                ← 旧前端（过渡期保留）
└── ...
```

Go 后端增加逻辑：优先 serve `web/dist/`，不存在则 fallback 到旧 `static/`。

### Phase 2：完整迁移 + embed 路由
- 逐步迁移所有视图到 React 组件
- 实现 `/embed/*` 路由
- Go 后端只保留 API 层

### Phase 3：组件库发布
- `@perf-scope/ui` 发布 npm
- Storybook 文档站
- 外部消费者接入

---

## 8. 后端适配改动（最小化）

| 改动 | 工作量 | 说明 |
|------|--------|------|
| `/api/tasks` 增加 `available_types` 字段 | 0.5d | 扫描 task 目录检查存在哪些分析输出文件 |
| 新增 `/embed/<view>` HTML handler | 1d | 渲染最小化 HTML Shell + 加载对应 JS 模块 |
| CORS 支持增强 | 0.5d | iframe 嵌入需要正确的 CORS 和 CSP 头 |
| Serve React dist 逻辑 | 0.5d | 优先 `web/dist/`，fallback 旧 `static/` |

核心思想：**后端 API 不变，只增加入口层适配**。

---

## 9. 决策记录

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 多结果导航模型 | Task → Type → View 三级路由 | 符合数据模型，避免扁平列表混乱 |
| 对外嵌入优先方案 | React 组件 import（主力）+ iframe（兜底） | React 系统最大灵活度，非 React 有 iframe 兜底 |
| 按需加载策略 | React.lazy + Suspense 两层 | 首屏只加载当前面板代码 |
| 组件双模式 | task prop（自动拉数据）+ data prop（受控纯展示） | 同时满足独立使用和集成使用 |
| 过渡策略 | 新旧并行，Go 后端 fallback | 零中断迁移 |

---

## 10. 原型导航重设计 — Session-Driven Navigation (2026-05-25 已实施)

### 10.1 问题发现

原始原型存在**双重导航入口**冗余：
- 左侧 Session 列表（按类型点击切换面板）
- 右上角 Profile Selector 按钮（CPU / Heap / Goroutine）

两者功能完全重叠，违反"Single Source of Truth"设计原则。

### 10.2 专家分析

| 视角 | 判断 |
|------|------|
| 性能剖析专家 | 用户心智模型是"先选时间点/事件，再选分析维度"，Session 列表已天然包含类型信息 |
| 前端设计专家 | 参考 Chrome DevTools、IntelliJ Profiler、Grafana Pyroscope、Eclipse MAT，均使用单入口导航 |

**核心结论**：保留左侧 Session 列表作为唯一导航源，移除 Profile Selector。

### 10.3 实施方案

| 变更 | 描述 |
|------|------|
| 移除 Profile Selector | top-bar-right 区域不再显示 CPU/Heap/Goroutine 按钮 |
| 新增 Compare + Share | 用更有价值的操作替代原有位置 |
| Session 时间分组 | 左侧 Session 按时间分组（Today 10:30 / Today 09:15 / Yesterday） |
| Session 作为唯一入口 | `app.js` 中只保留 session click 的面板切换逻辑 |
| ViewRouter 增强 | `switchPanel()` 同时切换 analysis panel + context panel |

### 10.4 导航模型

```
Session (左侧, 按时间分组)  →  View (面板内 Tab 栏)
     "今天 10:30 CPU"           Flame Graph | Top Down | Treemap | Timeline
     "今天 10:30 Heap"          Histogram | Treemap | Dominator | GC Roots | Merged Paths
     "今天 09:15 CPU"           ...
```

**两级导航**：Session 选择 → View Tab 切换。清晰、无歧义。

### 10.5 文件变更清单

| 文件 | 变更 |
|------|------|
| `prototype/index.html` | 移除 `.profile-selector`，新增 Compare/Share 按钮，Session 列表增加时间分组 |
| `prototype/style.css` | 新增 `.action-btn-top`、`.top-bar-sep`、`.session-group-header`、`.session-badge.ok`；移除旧 profile 样式 |
| `prototype/js/app.js` | 重写为 Session-only 导航逻辑，移除 profileBtns 绑定 |
| `prototype/js/router.js` | `switchPanel()` 新增 context-body 联动切换 |
| `prototype/interactions.js` | **已删除** — 功能被 router.js + app.js 完全取代 |

---

## 11. 遗留问题

1. **FlameGraph Canvas 渲染方案** — 是基于 d3-flame-graph 二次开发还是纯自研 Canvas，需 Phase 1 POC 验证性能后决策
2. **组件间联动** — 如 FlameGraph 点击 frame 后 ContextPanel 显示详情，跨组件通信用 Zustand store 还是 React Context
3. **构建产物集成** — Vite build 后是打包到 Go binary（embed）还是独立 CDN 部署，影响发布流程
4. **API Client 鉴权** — 对外 embed 场景下的 Token/Cookie 鉴权方案
5. **Monorepo 时机** — Phase 1 先在 `web/src/` 内扁平开发，Phase 3 再拆包；还是从一开始就 monorepo

---

## 12. 对外消费者集成方式总结

| 方式 | 适用场景 | 集成成本 | 灵活度 |
|------|---------|---------|--------|
| iframe `/embed/*` | 非 React 系统、内部平台快速接入 | ⭐ | ⭐⭐ |
| npm 组件 `@perf-scope/ui` | React 系统、深度集成 | ⭐⭐ | ⭐⭐⭐⭐ |
| 完整应用 `@perf-scope/app` | 独立部署 | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
