# 统一泄漏嫌疑检测架构 (Unified Leak Suspects)

> 创建日期: 2026-05-19  
> 状态: ✅ 已完成  
> 关联文档: [后端 API 设计](../2026-05-08/design-backend-api-for-prototype.md)

## 1. 背景与问题

### 原有架构问题

旧的 `/api/pprof/leak-report` API 存在以下设计缺陷：

1. **违反 OCP（开闭原则）**：handler 直接读取 `batch_analysis.json`，仅支持时序对比泄漏检测
2. **违反 SRP（单一职责）**：没有区分"时序泄漏"和"快照嫌疑"两种不同语义
3. **违反 DIP（依赖反转）**：高层 API 直接依赖低层文件格式
4. **数据模型碎片化**：Go pprof 泄漏、Java heap 建议、Java 内存泄漏用三套不同模型

### 核心问题

单次 Java hprof 分析后，`/api/pprof/leak-report` 始终返回空 `{"leak_reports":{}}`，因为没有 `batch_analysis.json`。但原型的 "Leak Suspects" 面板需要在单次分析时就有数据。

## 2. 解决方案

### 策略模式 + Provider Chain + 统一输出

```
┌──────────────────────────────────────────────────────┐
│  LeakSuspectProvider (Interface)                      │
│  CanDetect(taskDir) bool                             │
│  Detect(taskDir) ([]LeakSuspect, error)              │
└──────────────────────────────────────────────────────┘
        ▲                    ▲
        │                    │
┌───────┴──────────┐  ┌─────┴──────────────┐
│ TimeSeriesLeak   │  │ HprofSnapshotLeak   │
│ Provider         │  │ Provider            │
│ (batch_analysis) │  │ (class_stats.json / │
│                  │  │  heap_stats.json)   │
└──────────────────┘  └────────────────────┘
```

### 设计原则映射

| 原则 | 如何满足 |
|------|---------|
| SRP | 每个 Provider 只负责一种检测策略 |
| OCP | 新增检测策略只需实现接口，无需改 handler |
| DIP | Handler 依赖 `LeakSuspectProvider` 接口 |
| LSP | 所有 Provider 输出统一 `[]LeakSuspect` |
| 高内聚 | 检测逻辑封装在各 Provider 内部 |
| 低耦合 | Provider 之间互不感知 |

## 3. 数据模型

```go
type LeakSuspect struct {
    Type        string        `json:"type"`        // "heap" | "goroutine" | "class_accumulation" | "classloader_leak"
    Source      LeakSource    `json:"source"`      // "time_series" | "snapshot_heuristic"
    Severity    LeakSeverity  `json:"severity"`    // "info" | "warning" | "critical"
    Title       string        `json:"title"`       // 一句话标题
    Description string        `json:"description"` // 详细描述
    Evidence    []LeakEvidence `json:"evidence"`   // 支撑数据
    Metrics     *LeakMetrics  `json:"metrics"`     // 增长量化（时序型）
    Suggestions []string      `json:"suggestions"` // 修复建议
}
```

## 4. API 设计

### 新增统一 API

```
GET /api/leak-suspects?task=<id>&type=<heap|goroutine|all>&severity=<info|warning|critical>
```

### 响应示例

```json
{
  "total_count": 2,
  "suspects": [
    {
      "type": "heap",
      "source": "snapshot_heuristic",
      "severity": "critical",
      "title": "Class com.app.model.Order dominates heap at 40.0%",
      "description": "com.app.model.Order retains 40.0% of total heap (200000 instances, 100.0 MB retained).",
      "evidence": [
        {"name": "com.app.model.Order", "value": 104857600, "unit": "bytes", "detail": "40.0% of heap"},
        {"name": "com.app.model.Order", "value": 200000, "unit": "count", "detail": "instances"}
      ],
      "suggestions": [
        "Inspect com.app.model.Order for unbounded growth",
        "Check if references to this class are released after use"
      ]
    },
    {
      "type": "class_accumulation",
      "source": "snapshot_heuristic",
      "severity": "warning",
      "title": "Collection java.util.HashMap$Node has 150000 instances",
      "description": "...",
      "evidence": [...],
      "suggestions": [...]
    }
  ]
}
```

## 5. Provider 实现

### TimeSeriesLeakProvider

- **数据源**：`batch_analysis.json`
- **检测逻辑**：读取已有的时序对比结果（需要 ≥2 个同类 profile）
- **适用场景**：Go pprof 批量分析

### HprofSnapshotLeakProvider

- **数据源**：`class_stats.json` + `heap_stats.json`
- **检测逻辑**：基于单次快照的启发式规则
- **适用场景**：Java hprof 单次分析、Go pprof heap 单次分析

#### 启发式规则

| 规则 | 阈值 | 严重度 |
|------|------|--------|
| **DominantClassRule** | 单类 retained ≥ 25% → warning, ≥ 40% → critical | 跳过原始数组（byte[] 等） |
| **CollectionAccumulationRule** | 集合类实例 ≥ 50K → info, ≥ 100K → warning, ≥ 500K → critical | HashMap、ArrayList、HashSet 等 |
| **ClassLoaderLeakRule** | 类数量 ≥ 30K → info, ≥ 50K → warning, ≥ 80K → critical | 暗示 classloader 泄漏 |

## 6. 实施记录

### 新增文件

| 文件 | 说明 |
|------|------|
| `perflib/query/leak_suspect.go` | 统一模型 + Provider 接口 + Engine |
| `perflib/query/leak_suspect_timeseries.go` | TimeSeriesLeakProvider 实现 |
| `perflib/query/leak_suspect_hprof.go` | HprofSnapshotLeakProvider 实现 |
| `perflib/query/leak_suspect_test.go` | 17 个单元测试 |

### 修改文件

| 文件 | 变更 |
|------|------|
| `perflib/output/convention.go` | 新增 `FileLeakSuspects` 常量 |
| `perflib/analyzer/java_heap_analyzer.go` | Step 9c: 分析时预计算 `leak_suspects.json` |
| `perflib/analyzer/pprof_batch_analyzer.go` | 分析后预计算 `leak_suspects.json` |
| `internal/webui/server.go` | 注册 `/api/leak-suspects` 路由 |
| `internal/webui/pprof_api_handlers.go` | 新增 `handleLeakSuspects` handler |

### 数据链路

```
分析时（Write Path）:
  JavaHeapAnalyzer → class_stats.json + heap_stats.json
                   → HprofSnapshotLeakProvider.Detect()
                   → leak_suspects.json

  PProfBatchAnalyzer → batch_analysis.json
                     → LeakSuspectEngine.Detect() (两个 Provider)
                     → leak_suspects.json

API 请求时（Read Path）:
  GET /api/leak-suspects
    → 快路径: 直接读取 leak_suspects.json（O(1)）
    → 慢路径: 运行时执行 Provider Chain（fallback）
```

### 兼容性

- `/api/pprof/leak-report` **保留不动**，旧客户端继续可用
- `/api/leak-suspects` 是新的统一 API，推荐新前端使用

## 7. 测试覆盖

| 测试 | 验证点 |
|------|--------|
| TestLeakSuspectEngine_NoProviders | 空 Provider 返回空列表 |
| TestLeakSuspectEngine_ProviderNotApplicable | Provider 不适用时优雅跳过 |
| TestLeakSuspectEngine_SortsBySeverity | 结果按严重度降序排列 |
| TestLeakSuspectsResult_FilterByType | 类型过滤正确 |
| TestLeakSuspectsResult_FilterBySeverity | 严重度过滤正确 |
| TestTimeSeriesLeakProvider_CanDetect | 文件存在性检测 |
| TestTimeSeriesLeakProvider_Detect_WithLeakReports | 正确适配 batch_analysis.json |
| TestTimeSeriesLeakProvider_Detect_EmptyLeakReports | 空报告返回空列表 |
| TestHprofSnapshotLeakProvider_CanDetect | 文件存在性检测 |
| TestHprofSnapshotLeakProvider_DominantClassRule | 高占比类触发 critical |
| TestHprofSnapshotLeakProvider_DominantClassRule_SkipsPrimitiveArrays | byte[] 不触发 |
| TestHprofSnapshotLeakProvider_CollectionAccumulationRule | 集合类高实例数触发 |
| TestHprofSnapshotLeakProvider_ClassLoaderLeakRule | 高类数量触发 |
| TestHprofSnapshotLeakProvider_ClassLoaderLeakRule_Normal | 正常类数不触发 |
| TestHprofSnapshotLeakProvider_NoSuspectsForNormalHeap | 正常堆无误报 |
| TestLeakSuspectEngine_Integration | 多 Provider 聚合验证 |

## 8. 后续扩展方向

1. **SuggestionsLeakProvider**：从 `summary.json` 的 suggestions 字段提取泄漏相关建议
2. **GoroutineSnapshotLeakProvider**：基于单次 goroutine dump 的启发式检测
3. **前端适配**：原型 "Leak Suspects" 面板改为调用 `/api/leak-suspects`
4. **废弃旧 API**：逐步迁移后移除 `/api/pprof/leak-report`
