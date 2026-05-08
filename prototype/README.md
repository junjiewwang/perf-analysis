# PerfScope — Profiler 原型设计说明

## 背景

为 perf-analysis 项目设计一套专业级 Profiler 性能分析工具的 UI 原型，对标 JetBrains Profiler、Chrome DevTools Performance、Grafana Pyroscope 等商业产品标准。

## 设计目标

1. **操作简单**：核心操作路径 ≤ 3 步到达任何分析结果
2. **清晰简洁**：信息层级分明，数据密度高但不拥挤
3. **专业可靠**：深色系工业风格，减少长时间分析的视觉疲劳

## 方案设计

### 设计理念

**"Industrial Precision + Data Warmth"** — 工业精密 + 数据温暖

- **深色基底**：`#0f1117` 极深蓝黑，长时间盯屏不疲劳
- **热力数据色**：橙→红渐变表达 CPU 热点，自然直觉
- **信息层级**：背景 → 卡片 → 高亮，三层递进
- **字体配对**：Inter（UI 文案） + JetBrains Mono（代码/数据）

### 三栏式布局架构

```
┌──────────────────────────────────────────────────────────────────────┐
│ Top Bar: Logo + Breadcrumb + Quick Search (⌘K) + Profile Switch      │
├────────┬─────────────────────────────────────────────────┬───────────┤
│        │ View Tabs: Flame Graph | Top Down | Call Tree | │           │
│  Left  │ Treemap | Timeline                             │   Right   │
│ Sidebar│─────────────────────────────────────────────────│  Context  │
│        │ Summary Metrics Strip                          │   Panel   │
│Sessions│─────────────────────────────────────────────────│           │
│  List  │                                                 │ ● Details │
│        │          Main Visualization                     │ ● Callers │
│  240px │           (Flame Graph)                         │ ● Callees │
│        │                                                 │ ● Source  │
│        │─────────────────────────────────────────────────│ ● AI Tips │
│        │ Hot Functions Table                             │   300px   │
├────────┴─────────────────────────────────────────────────┴───────────┤
│ Status Bar: Connection · Runtime · Profile Config · Stats            │
└──────────────────────────────────────────────────────────────────────┘
```

### 核心交互设计

| 操作 | 方式 | 说明 |
|------|------|------|
| 全局搜索 | `⌘K` / 点击搜索栏 | Spotlight 风格，搜索函数/类/线程 |
| 切换分析类型 | 顶栏 Profile 切换器 | CPU / Heap / Goroutine 一键切换 |
| 切换视图 | Tab 栏 | Flame Graph / Top Down / Call Tree / Treemap / Timeline |
| 查看详情 | 点击火焰帧 | 右侧面板实时展示 Callers/Callees/Source |
| 筛选线程 | 下拉选择器 | 快速聚焦特定线程 |
| 高亮搜索 | 可视化内搜索框 | 输入函数名高亮匹配帧 |

### 配色系统

| 用途 | 颜色 | 值 |
|------|------|------|
| 应用代码帧 | 🟠 橙色 | `#ff6b35` |
| 热点帧 | 🔴 红色 | `#ff3d00` / `#ff1744` |
| Runtime 帧 | 🟢 绿色 | `#10b981` |
| GC 帧 | 🟡 琥珀色 | `#f59e0b` |
| Kernel 帧 | 🟣 靛蓝 | `#6366f1` |
| 品牌强调色 | 🔵 蓝色 | `#4f6af5` |

### 差异化亮点

1. **AI Insight 面板**：选中热点函数后，右侧自动展示优化建议
2. **Source Preview**：直接在上下文面板中预览热点源码行
3. **Command Palette**：⌘K 快速搜索，无需在界面中翻找
4. **热力色阶**：火焰帧颜色饱和度随占比自动升高，视觉直觉
5. **Status Bar**：底部始终展示连接状态/运行时信息/渲染性能

## 文件结构

```
prototype/
├── index.html          ← 完整 HTML 结构
├── style.css           ← 设计系统 + 全部样式
├── interactions.js     ← 轻量交互（Tab 切换、Command Palette、Tooltip）
└── README.md           ← 本文件
```

## 使用方式

```bash
cd prototype/
python3 -m http.server 8901
# 浏览器访问 http://localhost:8901
```

## 实施进展

- [x] 三栏布局 + 顶栏 + 状态栏
- [x] 火焰图可视化（静态模拟）
- [x] Hot Functions 表格
- [x] 右侧上下文面板（Details / Callers / Callees / Source / AI Insight）
- [x] Command Palette (⌘K)
- [x] Profile 类型切换
- [x] Session 列表
- [x] 交互动效（Hover、Tab 切换、淡入动画）
- [ ] Heap 分析模式页面
- [ ] Goroutine 分析模式页面
- [ ] Timeline 视图
- [ ] 对比模式（Differential Flame Graph）
- [ ] 响应式移动端适配

## 遗留问题

1. 原型为静态 HTML，实际实现需接入真实数据和 D3/ECharts 渲染
2. AI Insight 功能需要后端 LLM 集成支持
3. Source Preview 需要服务端提供源码定位 API
