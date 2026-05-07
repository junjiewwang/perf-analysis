# perflib/parser/hprof

Java HPROF 堆转储解析库，实现了高性能的 Two-Pass CSR 解析引擎和持久化索引文件格式。

## 概述

本包提供：
- **HPROF 文件解析**：支持标准 Java HPROF 二进制格式（JDK 生成的 `.hprof` 文件）
- **Two-Pass CSR 引用图构建**：内存高效的图数据结构
- **Dominator Tree 计算**：Lengauer-Tarjan 算法 + retained size
- **索引文件序列化**：`heap_index.bin` 格式（v1 全量加载 / v2 mmap 零拷贝）
- **查询接口**：`HeapGraph` 抽象接口，供上层按需查询

## 快速开始

### 基本用法：解析 HPROF 文件

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

func main() {
    f, _ := os.Open("heap.hprof")
    defer f.Close()

    parser := hprof.NewParser()
    ctx := context.Background()

    // Two-Pass 解析：产出完整的引用图 + dominator tree
    graph, err := parser.ParseTwoPass(ctx, f)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Objects: %d\n", graph.ObjectCount())

    // 序列化到索引文件（后续可快速加载）
    _ = hprof.WriteHeapIndex("heap_index.bin", graph)
}
```

### 加载已有索引文件

```go
// 自动检测 v1/v2 格式
graph, err := hprof.ReadHeapIndex("heap_index.bin")
if err != nil {
    panic(err)
}

// graph 实现了 HeapGraph 接口
fmt.Printf("Object count: %d\n", graph.ObjectCount())

// v2 格式返回 *MmapHeapIndex，需要 Close 释放 mmap
if mmapGraph, ok := graph.(*hprof.MmapHeapIndex); ok {
    defer mmapGraph.Close()
}
```

### 通过 HeapGraph 接口查询

```go
func analyzeGraph(g hprof.HeapGraph) {
    n := g.ObjectCount()

    for i := int32(0); i < n; i++ {
        if g.GetRetainedSize(i) > 10*1024*1024 { // >10MB
            classID := g.GetClassID(i)
            fmt.Printf("Large object: %s (retained: %d bytes)\n",
                g.GetClassName(classID), g.GetRetainedSize(i))
        }
    }

    // 查询对象的引用关系
    targets, fieldIDs, _ := g.GetOutgoingEdges(0)
    for j, target := range targets {
        fmt.Printf("  → %s.%s\n",
            g.GetClassName(g.GetClassID(target)),
            g.GetFieldName(fieldIDs[j]))
    }
}
```

## 架构

### Two-Pass 解析流程

```
┌─────────────────┐     ┌─────────────────┐     ┌──────────────────┐
│  Pass 1: Scan   │────▶│  Pass 2: Build  │────▶│  Dominator Tree  │
│  - 统计计数      │     │  - 预分配 CSR    │     │  - Lengauer-Tarjan│
│  - 收集元数据    │     │  - 填充引用目标  │     │  - retained sizes │
│  - Segment 偏移  │     │  - 并行 (可选)   │     │                  │
└─────────────────┘     └─────────────────┘     └──────────────────┘
         │                        │                        │
     ScanResult          IndexedReferenceGraph      (dominator 填充)
```

### 文件组织

| 文件 | 职责 |
|------|------|
| **解析核心** | |
| `scan_pass.go` | Pass 1：扫描 HPROF records，收集元数据和计数 |
| `build_pass.go` | Pass 2（顺序版）：构建 CSR 图 |
| `build_pass_parallel.go` | Pass 2（并行版）：多 Worker 分 Segment 解析 |
| `parser.go` | Parser 入口 + ParseTwoPass 编排 |
| `core_reader.go` | 底层 HPROF 二进制读取器 |
| **数据结构** | |
| `graph_indexed.go` | `IndexedReferenceGraph` + `IndexedObjectStore` + `CompactEdgeList` |
| `heap_graph.go` | `HeapGraph` 接口定义 |
| `object_info_assembler.go` | 查询辅助工具（消除重复组装逻辑） |
| `types.go` | 基础类型定义 (GCRoot, ClassInfo 等) |
| **Dominator** | |
| `dom_indexed.go` | Lengauer-Tarjan dominator 算法 (CSR 版) |
| `dom_hierarchical.go` | 层次并行优化 |
| `dom_parallel.go` | 并行 predecessors 构建 |
| **索引文件** | |
| `index_format.go` | v1 格式定义（Header, SectionType, Flags） |
| `index_format_v2.go` | v2 格式定义（Section Table, PageAlignment） |
| `index_writer.go` | v1 Writer (bufio 顺序写) |
| `index_writer_v2.go` | v2 Writer (page-aligned, mmap friendly) |
| `index_reader.go` | Reader 入口（自动检测版本）+ v1 反序列化 |
| `index_reader_v2.go` | v2 Reader (`MmapHeapIndex`，unsafe.Slice 零拷贝) |

### 核心类型

#### HeapGraph 接口

```go
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

实现者：
- `*IndexedReferenceGraph` — 内存全量加载（v1 格式反序列化产物）
- `*MmapHeapIndex` — mmap 零拷贝映射（v2 格式）

#### CompactEdgeList (CSR)

```go
type CompactEdgeList struct {
    offsets    []int32    // len = N+1, 节点 i 的边范围: [offsets[i], offsets[i+1])
    targets    []int32    // len = E, 边目标节点索引
    fieldIDs   []int32    // len = E, 字段名 ID
    classIDs   []uint64   // len = E, 引用的 classID
    fieldNames []string   // 字段名字符串池 (intern)
}
```

#### ObjectInfoAssembler

消除重复的 `idx → HeapObjectInfo` 组装代码：

```go
assembler := hprof.NewObjectInfoAssembler(graph)
info := assembler.AssembleByIndex(idx)
// info.ObjectID, info.ClassName, info.ShallowSize, info.RetainedSize
```

## 性能基准

### 测试环境

- Apple M3 Pro, 18GB RAM
- 测试文件：2.45 GB hprof (16M 对象, 34M 边, 19M 出边)

### 解析性能

| 阶段 | 耗时 | 说明 |
|------|------|------|
| Scan Pass | 5.5s | 顺序扫描 |
| Build Pass (并行, 14 workers) | 6.6s | 包含 merge |
| Dominator Tree | 1.5s | Lengauer-Tarjan |
| Total | **~16s** | |

### 内存使用

| 方案 | 峰值内存 |
|------|---------|
| 旧方案 (map-based) | ~7-8 GB |
| Two-Pass CSR | ~1.2 GB |
| v2 mmap (常驻) | ~330 MB |

### 索引文件

| 文件 | 大小 |
|------|------|
| 原始 .hprof | 2.45 GB |
| heap_index.bin (v1) | ~450 MB |
| heap_index.bin (v2) | ~450 MB (page-aligned padding 增加少量) |

## 性能调优建议

### 解析阶段

1. **确保文件可 Seek**：传入 `*os.File`（支持 `io.ReadSeeker` + `io.ReaderAt`），自动启用并行 Build Pass
2. **大文件自动并行**：Segment 数 ≥ 4 时自动使用多 Worker，无需手动配置
3. **内存控制**：CSR 预分配基于精确计数，无需额外 buffer 预留

### 查询阶段

1. **优先使用 v2 格式**：`WriteHeapIndexV2` 产出 mmap 友好文件，加载速度 <500ms
2. **关闭 MmapHeapIndex**：使用完毕后调用 `Close()` 释放 mmap 映射
3. **复用 ObjectInfoAssembler**：避免在循环中重复创建

### 内存优化

1. **v2 mmap 按需加载**：只有实际访问的 page 才会加载到物理内存
2. **classToObjects 懒加载**：`GetObjectsByClass` 首次调用时构建索引
3. **objToIdx map 是主要常驻开销**：16M 对象 ≈ 300MB Go map

## 索引文件格式

### 版本检测

`ReadHeapIndex(filePath)` 自动检测格式版本：
1. 读取前 8 字节：Magic("HPIX") + Version
2. Version == 1 → `readHeapIndexV1` (bufio 全量加载)
3. Version == 2 → `OpenMmapHeapIndex` (mmap 零拷贝)

### v2 格式优势

| 特性 | v1 | v2 |
|------|----|----|
| 读取方式 | 顺序 bufio 流式 | mmap 随机访问 |
| 加载时间 | ~2s | <500ms |
| 常驻内存 | ~1.2GB | ~330MB |
| Section 发现 | 顺序解析 | Section Table 直接定位 |
| 数据对齐 | 无 | 4096 字节 page 对齐 |

## 测试

```bash
# 运行所有单元测试
go test ./perflib/parser/hprof/ -v

# 运行索引 roundtrip 测试
go test ./perflib/parser/hprof/ -run "TestIndex" -v

# 运行大文件集成测试（需要测试数据）
go test ./perflib/parser/hprof/ -run "TestBuildPass" -v

# 带竞态检测
go test ./perflib/parser/hprof/ -race

# 运行 benchmark
go test ./perflib/parser/hprof/ -bench .
```
