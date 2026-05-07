# 分析类型参考手册

## 概述

perf-analysis 支持多种性能分析类型，覆盖 Java、Go、C/C++ 等语言的 CPU、内存、并发等维度。每种分析类型由 **profiler**（采集工具）和 **event**（事件类型）两个维度正交组合，自动派生出 **mode**（分析模式）：

```
mode = "{profiler}-{event}"
```

---

## 支持的分析类型一览

### Java 分析

| Mode | Profiler | Event | 资源类别 | 说明 | 输入格式 |
|------|----------|-------|---------|------|---------|
| `async-profiler-cpu` | `async-profiler` | `cpu` | CPU | Java CPU 热点分析 | Collapsed stack (.collapsed, .data, .txt) |
| `async-profiler-alloc` | `async-profiler` | `alloc` | Memory | Java 内存分配分析 | Collapsed stack (.collapsed, .data, .txt) |
| `async-profiler-wall` | `async-profiler` | `wall` | App | Java Wall-clock 耗时分析 | Collapsed stack (.collapsed, .data, .txt) |
| `async-profiler-lock` | `async-profiler` | `lock` | Concurrency | Java 锁争用分析 | Collapsed stack (.collapsed, .data, .txt) |
| `heapdump-heap` | `heapdump` | `heap` | Memory | Java 堆内存快照分析 | HPROF binary (.hprof) |

### Go 分析

| Mode | Profiler | Event | 资源类别 | 说明 | 输入格式 |
|------|----------|-------|---------|------|---------|
| `pprof-cpu` | `pprof` | `cpu` | CPU | Go CPU profile 分析 | Go pprof (.pprof, .pb.gz) |
| `pprof-heap` | `pprof` | `heap` | Memory | Go Heap profile 分析 | Go pprof (.pprof, .pb.gz) |
| `pprof-goroutine` | `pprof` | `goroutine` | Goroutine | Go Goroutine profile 分析 | Go pprof (.pprof, .pb.gz) |
| `pprof-block` | `pprof` | `block` | Concurrency | Go Block profile 分析 | Go pprof (.pprof, .pb.gz) |
| `pprof-mutex` | `pprof` | `mutex` | Concurrency | Go Mutex profile 分析 | Go pprof (.pprof, .pb.gz) |
| `pprof-all` | `pprof` | `cpu` | CPU | Go 批量 pprof 分析（目录） | 包含 cpu/heap/goroutine 等子目录的目录 |

### 通用分析

| Mode | Profiler | Event | 资源类别 | 说明 | 输入格式 |
|------|----------|-------|---------|------|---------|
| `perf-cpu` | `perf` | `cpu` | CPU | Linux perf 通用 CPU 分析 | Collapsed stack (.collapsed, .data, .txt) |
| `jeprof-heap` | `jeprof` | `heap` | Memory | jemalloc 堆内存分析 | Jeprof heap format |

---

## 有效的 Profiler 值

| Profiler | 说明 |
|----------|------|
| `async-profiler` | Java async-profiler 采集的数据 |
| `heapdump` | Java 堆内存转储 |
| `pprof` | Go pprof 采集的数据 |
| `perf` | Linux perf 采集的数据 |
| `jeprof` | jemalloc jeprof 采集的数据 |

## 有效的 Event 值

| Event | 说明 |
|-------|------|
| `cpu` | CPU 采样 |
| `alloc` | 内存分配 |
| `heap` | 堆内存快照 |
| `wall` | Wall-clock 挂钟时间 |
| `lock` | 锁争用 |
| `goroutine` | Go goroutine |
| `block` | Go 阻塞 |
| `mutex` | Go 互斥锁 |
| `io` | IO 追踪（预留） |

---

## 分析产出

每种分析类型会产出以下文件（上传至存储）：

| 文件 | 说明 | 适用类型 |
|------|------|---------|
| `collapsed_data.json.gz` | 火焰图数据 | CPU/Wall/Lock/Perf |
| `callgraph_data.json.gz` | 调用图数据 | CPU/Wall/Lock/Perf |
| `alloc_data.json.gz` | 内存分配火焰图 | Alloc |
| `alloc_callgraph_data.json.gz` | 内存分配调用图 | Alloc |
| `summary.json` | 概览数据（WebUI Overview） | 所有类型 |
| `retainer_analysis.json` | 保留对象分析 | HeapDump |

---

## HTTP API 提交分析任务

### 接口信息

- **URL**: `POST http://{host}:8081/tasks`
- **Content-Type**: `application/json`
- **最大请求体**: 1MB

### 请求参数

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `tid` | string | ✅ | 任务唯一标识（UUID） |
| `profiler` | string | ✅ | 采集工具，见"有效的 Profiler 值" |
| `event` | string | ✅ | 事件类型，见"有效的 Event 值" |
| `result_file` | string | — | 待分析文件的存储路径（COS key 或本地路径） |
| `user_name` | string | — | 提交用户名 |
| `mastertask_tid` | string | — | 父任务 UUID（用于批量分析场景） |
| `cos_bucket` | string | — | 指定 COS bucket（覆盖默认） |
| `callback_url` | string | — | 任务级回调 URL，分析完成/失败时通知 |
| `request_params` | object | — | 额外请求参数 |
| `metadata` | object | — | 自定义元数据，透传至回调通知 |

### 响应格式

```json
{
  "success": true,
  "task_id": "task-uuid-001",
  "message": "task created and queued for analysis"
}
```

### 错误响应

```json
{
  "success": false,
  "message": "invalid profiler: unknown-profiler"
}
```

---

## 请求示例

### 1. Java CPU 热点分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-java-cpu-001",
    "profiler": "async-profiler",
    "event": "cpu",
    "result_file": "uploads/java-cpu-data.collapsed",
    "user_name": "zhangsan",
    "callback_url": "https://api.example.com/callback"
  }'
```

### 2. Java 内存分配分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-java-alloc-001",
    "profiler": "async-profiler",
    "event": "alloc",
    "result_file": "uploads/java-alloc-data.collapsed",
    "user_name": "lisi"
  }'
```

### 3. Java Wall-clock 分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-java-wall-001",
    "profiler": "async-profiler",
    "event": "wall",
    "result_file": "uploads/java-wall-data.collapsed"
  }'
```

### 4. Java 锁争用分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-java-lock-001",
    "profiler": "async-profiler",
    "event": "lock",
    "result_file": "uploads/java-lock-data.collapsed"
  }'
```

### 5. Java 堆内存分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-java-heap-001",
    "profiler": "heapdump",
    "event": "heap",
    "result_file": "uploads/heapdump.hprof",
    "user_name": "wangwu"
  }'
```

### 6. Go CPU Profile 分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-go-cpu-001",
    "profiler": "pprof",
    "event": "cpu",
    "result_file": "uploads/cpu.pprof"
  }'
```

### 7. Go Heap Profile 分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-go-heap-001",
    "profiler": "pprof",
    "event": "heap",
    "result_file": "uploads/heap.pprof"
  }'
```

### 8. Go Goroutine Profile 分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-go-goroutine-001",
    "profiler": "pprof",
    "event": "goroutine",
    "result_file": "uploads/goroutine.pprof"
  }'
```

### 9. Go Block Profile 分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-go-block-001",
    "profiler": "pprof",
    "event": "block",
    "result_file": "uploads/block.pprof"
  }'
```

### 10. Go Mutex Profile 分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-go-mutex-001",
    "profiler": "pprof",
    "event": "mutex",
    "result_file": "uploads/mutex.pprof"
  }'
```

### 11. Go 批量 pprof 分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-go-all-001",
    "profiler": "pprof",
    "event": "cpu",
    "result_file": "uploads/pprof-bundle/"
  }'
```

### 12. Linux Perf 通用 CPU 分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-perf-cpu-001",
    "profiler": "perf",
    "event": "cpu",
    "result_file": "uploads/perf-data.collapsed"
  }'
```

### 13. Jemalloc 堆分析

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-jeprof-001",
    "profiler": "jeprof",
    "event": "heap",
    "result_file": "uploads/jeprof.heap"
  }'
```

### 14. 带完整参数的请求示例

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "tid": "task-full-example-001",
    "profiler": "async-profiler",
    "event": "cpu",
    "result_file": "uploads/app-cpu-profile.collapsed",
    "user_name": "zhangsan",
    "mastertask_tid": "master-task-001",
    "cos_bucket": "my-custom-bucket",
    "callback_url": "https://api.example.com/perf/callback",
    "request_params": {
      "duration": 60,
      "container_name": "my-app-container"
    },
    "metadata": {
      "env": "production",
      "cluster": "sh-01",
      "app_name": "order-service",
      "version": "v2.1.0"
    }
  }'
```

---

## CLI 使用方式

除了 HTTP API，也可通过 CLI 直接分析本地文件：

```bash
# Java CPU 分析
perf-cli analyze -i ./data.collapsed -m async-profiler-cpu

# Java 内存分配分析
perf-cli analyze -i ./alloc.data -m async-profiler-alloc

# Java 堆内存分析
perf-cli analyze -i ./heap.hprof -m heapdump-heap

# Go CPU Profile 分析
perf-cli analyze -i ./cpu.pprof -m pprof-cpu

# Go Heap Profile 分析
perf-cli analyze -i ./heap.pprof -m pprof-heap

# 使用 detailed profile 深度分析
perf-cli analyze -i ./data.collapsed -m async-profiler-cpu --profile detailed

# 分析后启动 Web 查看
perf-cli analyze -i ./data.collapsed -m async-profiler-cpu --serve --port 8080
```

CLI 的 `-m` / `--mode` 参数直接使用上方表格中的 **Mode** 值。

---

## 回调通知

分析完成或失败后，系统会向 callback URL 发送通知。回调 URL 优先级：

1. **任务级** `callback_url`（请求中指定）
2. **Source 级** `callback_url`（配置文件中 source 的 options）
3. **全局默认** `callback.default_url`（配置文件中全局配置）

详细的回调协议说明请参考 [callback-api-protocol.md](./callback-api-protocol.md)。
