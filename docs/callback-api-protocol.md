# Perf-Analysis 回调接口协议文档

> 版本：v1.0  
> 最后更新：2026-02-12

本文档描述 perf-analysis 服务对外提供的两类 HTTP 接口协议：

1. **入站接口（Inbound）** — 外部服务调用 perf-analysis 提交分析任务
2. **出站回调（Outbound Webhook）** — 分析完成/失败后 perf-analysis 主动通知外部服务

---

## 1. 入站接口 — 提交分析任务

### 1.1 接口概要

| 项目 | 说明 |
|------|------|
| URL | `POST {ingress_addr}/tasks` |
| Content-Type | `application/json` |
| 请求体上限 | 默认 1MB（可配置 `ingress.http.max_body_size`）|
| 认证 | 当前版本无内置认证，建议在网关层配置 |

### 1.2 请求体（Request Body）

```json
{
  "tid": "unique-task-uuid-001",
  "profiler": "async-profiler",
  "event": "cpu",
  "result_file": "cos://bucket/path/to/result.jfr",
  "user_name": "alice",
  "cos_bucket": "my-bucket",
  "callback_url": "https://your-service.example.com/perf/callback",
  "mastertask_tid": "master-uuid-001",
  "request_params": {
    "duration": 60,
    "perf_duration": 30,
    "container_type": 1,
    "container_name": "my-container",
    "annotate_enable": false
  },
  "metadata": {
    "env": "production",
    "cluster": "us-east-1",
    "trace_id": "abc123",
    "triggered_by": "auto-diagnosis"
  }
}
```

### 1.3 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `tid` | string | **是** | 任务唯一标识，由调用方生成（建议 UUID） |
| `profiler` | string | **是** | 采集工具类型，见下方枚举值 |
| `event` | string | **是** | 事件类型，见下方枚举值 |
| `result_file` | string | 否 | 待分析文件的存储路径 |
| `user_name` | string | 否 | 提交人 |
| `cos_bucket` | string | 否 | 指定 COS 存储桶（覆盖默认配置）|
| `callback_url` | string | 否 | 任务级回调 URL（最高优先级），分析完成后通知此地址 |
| `mastertask_tid` | string | 否 | 关联的主任务 ID（批量分析场景）|
| `request_params` | object | 否 | 采集参数（duration、container 等）|
| `metadata` | object | 否 | 调用方自定义键值对，**原样透传到回调通知中** |

#### `profiler` 枚举值

| 值 | 说明 |
|----|------|
| `perf` | Linux perf 采集 |
| `async-profiler` | Java async-profiler 采集 |
| `pprof` | Go pprof 采集 |
| `heapdump` | Java heap dump |
| `jeprof` | jemalloc jeprof 采集 |

#### `event` 枚举值

| 值 | 说明 |
|----|------|
| `cpu` | CPU 采样 |
| `alloc` | 内存分配采样 |
| `heap` | 堆快照 |
| `wall` | Wall-clock 采样 |
| `lock` | 锁竞争采样 |
| `goroutine` | Go goroutine 分析 |
| `block` | Go block 分析 |
| `mutex` | Go mutex 分析 |
| `io` | IO 追踪 |

#### `profiler` + `event` 组合（mode）

系统内部自动计算 `mode = "{profiler}-{event}"`，常见有效组合：

| mode | 说明 |
|------|------|
| `perf-cpu` | Linux perf CPU 分析 |
| `async-profiler-cpu` | Java CPU 火焰图分析 |
| `async-profiler-alloc` | Java 内存分配分析 |
| `async-profiler-wall` | Java Wall-clock 分析 |
| `async-profiler-lock` | Java 锁竞争分析 |
| `heapdump-heap` | Java 堆内存分析 |
| `jeprof-heap` | jemalloc 内存分析 |
| `pprof-cpu` | Go CPU 分析 |
| `pprof-heap` | Go 堆内存分析 |
| `pprof-goroutine` | Go Goroutine 分析 |
| `pprof-block` | Go Block 分析 |
| `pprof-mutex` | Go Mutex 分析 |
| `pprof-io` | IO 追踪分析 |

### 1.4 成功响应

**HTTP 201 Created**

```json
{
  "success": true,
  "task_id": "unique-task-uuid-001",
  "message": "task created and queued for analysis"
}
```

### 1.5 错误响应

**HTTP 4xx / 5xx**

```json
{
  "success": false,
  "message": "error description"
}
```

常见错误码：

| HTTP 状态码 | 含义 |
|------------|------|
| 400 | 请求参数错误（缺少必填字段、无效的 profiler/event 等）|
| 405 | 仅允许 POST 方法 |
| 500 | 服务内部错误（数据库写入失败等）|

### 1.6 Callback URL 三级回退

当任务未指定 `callback_url` 时，系统按以下优先级选择回调地址：

```
任务级 callback_url  >  数据源级 callback_url  >  全局 callback.default_url
```

| 优先级 | 来源 | 配置方式 |
|--------|------|----------|
| 1（最高）| 任务自带 | 请求体中的 `callback_url` 字段 |
| 2 | 数据源配置 | `config.yaml` 中 `sources[].options.callback_url` |
| 3（最低）| 全局默认 | `config.yaml` 中 `callback.default_url` |

---

## 2. 出站回调（Webhook） — 分析结果通知

分析完成（成功或失败）后，perf-analysis 会向回调 URL 发送 HTTP POST 通知。

### 2.1 接口概要

| 项目 | 说明 |
|------|------|
| 方法 | `POST {callback_url}` |
| Content-Type | `application/json` |
| 超时 | 默认 10s（可配置 `callback.timeout`）|
| 重试 | 指数退避重试，默认最多 3 次（可配置 `callback.max_retries`）|

### 2.2 回调请求体（Callback Payload）

#### 分析成功

```json
{
  "task_id": "unique-task-uuid-001",
  "mode": "async-profiler-cpu",
  "status": "completed",
  "view_url": "https://perf.example.com/view?tid=unique-task-uuid-001&token=xxx",
  "summary": {
    "total_records": 15320,
    "suggestions": 3
  },
  "metadata": {
    "env": "production",
    "cluster": "us-east-1",
    "trace_id": "abc123",
    "triggered_by": "auto-diagnosis"
  }
}
```

#### 分析失败

```json
{
  "task_id": "unique-task-uuid-001",
  "mode": "async-profiler-cpu",
  "status": "failed",
  "error": "analysis failed: empty input file",
  "metadata": {
    "env": "production",
    "cluster": "us-east-1"
  }
}
```

### 2.3 字段说明

| 字段 | 类型 | 出现条件 | 说明 |
|------|------|----------|------|
| `task_id` | string | 始终 | 任务唯一标识（与提交时的 `tid` 一致）|
| `mode` | string | 始终 | 分析模式（`{profiler}-{event}`）|
| `status` | string | 始终 | `"completed"` 或 `"failed"` |
| `view_url` | string | 成功时 | 分析结果查看页面 URL（可能含签名 token）|
| `error` | string | 失败时 | 失败原因描述 |
| `summary` | object | 成功时 | 分析结果摘要 |
| `summary.total_records` | int | 成功时 | 总采样/记录数 |
| `summary.suggestions` | int | 成功时 | 生成的优化建议数量 |
| `metadata` | object | 提交时有则有 | **原样透传**提交时的 metadata 键值对 |

### 2.4 外部服务接收方实现要求

外部服务需实现一个 HTTP 端点来接收回调通知，要求如下：

#### 成功确认

- 返回 HTTP **2xx** 状态码表示接收成功
- 返回其他状态码视为失败，perf-analysis 将进行重试

#### 幂等性

- 回调可能因重试而被**重复投递**
- 接收方应根据 `task_id` 做幂等处理，避免重复消费

#### 超时

- 接收方应在 **10 秒内**返回响应
- 超时将触发重试

#### 最小实现示例（Go）

```go
http.HandleFunc("/perf/callback", func(w http.ResponseWriter, r *http.Request) {
    var payload struct {
        TaskID   string            `json:"task_id"`
        Mode     string            `json:"mode"`
        Status   string            `json:"status"`
        ViewURL  string            `json:"view_url"`
        Error    string            `json:"error"`
        Summary  *struct {
            TotalRecords int `json:"total_records"`
            Suggestions  int `json:"suggestions"`
        } `json:"summary"`
        Metadata map[string]string `json:"metadata"`
    }

    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }

    switch payload.Status {
    case "completed":
        log.Printf("Task %s completed, view: %s, records: %d",
            payload.TaskID, payload.ViewURL, payload.Summary.TotalRecords)
    case "failed":
        log.Printf("Task %s failed: %s", payload.TaskID, payload.Error)
    }

    // 用 task_id 做幂等检查...

    w.WriteHeader(http.StatusOK)
})
```

#### 最小实现示例（Python / Flask）

```python
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route("/perf/callback", methods=["POST"])
def perf_callback():
    payload = request.get_json()

    task_id = payload["task_id"]
    status = payload["status"]
    mode = payload.get("mode", "")
    metadata = payload.get("metadata", {})

    if status == "completed":
        view_url = payload.get("view_url", "")
        summary = payload.get("summary", {})
        print(f"Task {task_id} ({mode}) completed, "
              f"records={summary.get('total_records', 0)}, "
              f"view={view_url}")
    elif status == "failed":
        error = payload.get("error", "unknown")
        print(f"Task {task_id} ({mode}) failed: {error}")

    # 用 task_id 做幂等检查...

    return jsonify({"received": True}), 200
```

### 2.5 重试机制

| 参数 | 默认值 | 配置项 |
|------|--------|--------|
| 最大重试次数 | 3 | `callback.max_retries` |
| 退避策略 | 指数退避（1s, 2s, 4s, ...）| — |
| 单次超时 | 10s | `callback.timeout` |

重试触发条件：
- HTTP 连接失败（网络错误、DNS 解析失败等）
- 返回非 2xx 状态码

重试**不会**触发的条件：
- 上下文被取消（Context cancelled）

---

## 3. 健康检查接口

| 项目 | 说明 |
|------|------|
| URL | `GET {ingress_addr}/health` |
| 用途 | 负载均衡器 / 监控探活 |

```json
{
  "status": "healthy",
  "service": "ingress"
}
```

---

## 4. 配置参考

```yaml
# 入站 HTTP 服务配置
ingress:
  http:
    enabled: true
    listen_addr: ":8081"
    path: /tasks
    read_timeout: 30s
    write_timeout: 30s
    max_body_size: 1048576     # 1MB
    callback_url: ""           # 入站级默认回调 URL（降级写入任务）

# 出站回调配置
callback:
  default_url: ""              # 全局默认回调 URL（三级回退最低优先级）
  timeout: "10s"               # 单次回调 HTTP 超时
  max_retries: 3               # 最大重试次数

# 数据源级回调配置
sources:
  - type: database
    name: primary-db
    enabled: true
    options:
      callback_url: ""         # 数据源级回调 URL（三级回退中间优先级）
```

---

## 5. 时序图

```
外部服务                          perf-analysis                     回调接收方
  │                                    │                                │
  │  POST /tasks                       │                                │
  │  {tid, profiler, event, ...}       │                                │
  │───────────────────────────────────>│                                │
  │                                    │                                │
  │  201 Created                       │                                │
  │  {success: true, task_id: "xxx"}   │                                │
  │<───────────────────────────────────│                                │
  │                                    │                                │
  │                                    │  (异步分析处理...)              │
  │                                    │                                │
  │                                    │  POST callback_url             │
  │                                    │  {task_id, status, view_url,   │
  │                                    │   mode, summary, metadata}     │
  │                                    │───────────────────────────────>│
  │                                    │                                │
  │                                    │  200 OK                        │
  │                                    │<───────────────────────────────│
```
