# 魔法值统一管理重构

## 需求背景

项目中存在大量散落在各处的硬编码文件名（魔法值），如 `"goroutine_analysis.json"`、`"collapsed_data.json.gz"` 等。
这些文件名在分析器（writer）和 WebUI（reader）两端重复出现，存在以下问题：
1. 修改一处容易遗漏另一处
2. 无法通过 IDE "Find Usages" 追踪所有引用
3. 新开发者不知道文件用途

## 设计方案

### 核心原则
- **DRY**：每个文件名只定义一次
- **Single Source of Truth**：所有 writer/reader 引用同一个常量
- **Package Cohesion**：常量定义在 `perflib/output`，与 Writer/ReadJSON 同属一个包

### 文件位置
```
perflib/output/convention.go
```

### 常量分组
1. **Profile Subdirectory Names**: `DirCPU`, `DirHeap`, `DirGoroutine`, `DirBlock`, `DirMutex`
2. **Common Output Files**: `FileSummary`, `FileBatchAnalysis`
3. **CPU Profile Files**: `FileCPUFlameGraph`, `FileCPUCallGraph`, `FileCPUCallGraphLegacy`, `FileCPUFlameGraphAlt`
4. **Goroutine Files**: `FileGoroutineAnalysis`, `FileGoroutineFlameGraph`
5. **Heap Files**: `FileHeapIndex`, `FileClassHistogram`, `FileInuseSpaceFlameGraph`, etc.
6. **Contention Files**: `FileBlockFlameGraph`, `FileMutexFlameGraph`

### 辅助函数
- `FlameGraphFileName(profileType string) string` — 动态生成 `<type>_flamegraph.json.gz`

## 实施记录

### 已完成 ✅

| 阶段 | 内容 | 影响文件数 |
|------|------|-----------|
| Phase 1 | 创建 `perflib/output/convention.go` | 1 新增 |
| Phase 2 | 替换 `perflib/analyzer/` 中所有魔法值 | 7 文件 |
| Phase 3 | 替换 `internal/webui/` 中所有魔法值 | 4 文件 |
| Phase 4 | 替换 `internal/publisher/` 和 `internal/analyzer/` | 3 文件 |
| Phase 5 | 编译验证 + 全量测试通过 | — |
| Phase 6 | 修复验证发现的 7 处残留硬编码 | 7 文件 |

### 变更文件清单

**新增文件:**
- `perflib/output/convention.go` — 文件名和目录约定常量定义

**修改文件 (perflib/analyzer/):**
- `pprof_cpu_analyzer.go` — 替换 `collapsed_data.json.gz`, `callgraph_data.json.gz`
- `pprof_goroutine_analyzer.go` — 替换 `goroutine_flamegraph.json.gz`, `goroutine_analysis.json`
- `pprof_contention_analyzer.go` — 替换 `%s_flamegraph.json.gz` → `output.FlameGraphFileName()`
- `pprof_heap_analyzer.go` — 替换 `%s_flamegraph.json.gz` → `output.FlameGraphFileName()`
- `pprof_batch_analyzer.go` — 替换 `batch_analysis.json`, `collapsed_data.json.gz`, 目录名常量
- `java_cpu_analyzer.go` — 替换 `collapsed_data.json.gz`, `callgraph_data.json.gz`
- `java_mem_analyzer.go` — 替换 `alloc_data.json.gz`, `alloc_callgraph_data.json.gz`
- `java_heap_analyzer.go` — 替换 `class_histogram.json`, `heap_index.bin`

**修改文件 (internal/webui/):**
- `flamegraph_service.go` — 替换所有 Loader 中的文件名和目录名
- `server.go` — 替换 `summary.json`, `batch_analysis.json`, call graph 文件名
- `refgraph_service.go` — 替换 `heap_index.bin`
- `pprof_api_handlers.go` — 替换 `goroutine_analysis.json`

**修改文件 (internal/):**
- `internal/publisher/default_publisher.go` — 替换 `summary.json`（使用 `perflibOutput` 别名避免命名冲突）
- `internal/analyzer/java_cpu_analyzer.go` — 替换 OutputFiles 中的文件名
- `internal/analyzer/java_heap_analyzer.go` — 替换 OutputFiles 中的文件名
- `internal/analyzer/java_mem_analyzer.go` — 替换 OutputFiles 中的 `alloc_data.json.gz`, `alloc_callgraph_data.json.gz`

### Phase 6 修复的残留项

验证发现以下 7 处生产代码中仍有硬编码，已全部修复：

| 文件 | 原始值 | 替换为 |
|------|--------|--------|
| `internal/webui/server.go:228` | `"summary.json"` | `output.FileSummary` |
| `internal/webui/server.go:388-399` | `"heap"`, `"cpu"`, `"alloc_callgraph_data.json.gz"`, `"callgraph_data.json.gz"`, `"callgraph.json"` | `output.DirHeap`, `output.DirCPU`, `output.FileAllocCallGraph`, `output.FileCPUCallGraph`, `output.FileCPUCallGraphLegacy` |
| `perflib/analyzer/pprof_goroutine_analyzer.go:98,103` | `"goroutine_flamegraph.json.gz"` | `output.FileGoroutineFlameGraph` |
| `internal/webui/refgraph_service.go:251` | `"heap_index.bin"` | `output.FileHeapIndex` |
| `perflib/analyzer/java_heap_analyzer.go:294,324,333` | `"heap_index.bin"`, `"class_histogram.json"` | `output.FileHeapIndex`, `output.FileClassHistogram` |
| `perflib/analyzer/java_mem_analyzer.go:146,152` | `"alloc_data.json.gz"`, `"alloc_callgraph_data.json.gz"` | `output.FileAllocData`, `output.FileAllocCallGraph` |
| `internal/analyzer/java_mem_analyzer.go:63-70` | `"alloc_data.json.gz"`, `"alloc_callgraph_data.json.gz"` | `output.FileAllocData`, `output.FileAllocCallGraph` |

### 验证结果
- ✅ `go build ./...` 编译通过
- ✅ `go test ./perflib/...` 全部通过
- ✅ `go test ./internal/...` 全部通过

## 遗留事项

1. **测试文件中的硬编码**：`internal/analyzer/java_cpu_analyzer_test.go`、`internal/analyzer/integration_test.go`、`internal/publisher/publisher_test.go` 等测试文件中仍有硬编码文件名。这些属于低优先级，可在后续迭代中替换。
2. **`heap_analysis.json`**：出现在 `internal/analyzer/java_heap_analyzer.go` 中但仅为单处使用，暂未纳入常量（不属于通用约定，仅为业务层的中间产物）。
3. **Legacy 文件名**：如 `"memory_flamegraph.json.gz"`, `"alloc_flamegraph.json.gz"` 等仅在读取端作为兼容候选，未定义为常量（它们只用于向后兼容读取，不再作为新产出文件名）。
