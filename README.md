# Perf Analysis Service

Go 语言编写的性能分析平台，支持 Java 和 Go 生态的多种 Profiling 数据分析，提供火焰图、调用图、统计分析、优化建议等能力，并通过交互式 WebUI 进行可视化展示。

## Features

### 分析能力

- **Java CPU Profiling**：分析 async-profiler 采集的 CPU 热点数据（`async-profiler-cpu`）
- **Java Wall-clock**：分析 async-profiler Wall-clock 时间数据（`async-profiler-wall`）
- **Java Lock Contention**：分析 async-profiler 锁争用数据（`async-profiler-lock`）
- **Java Memory Allocation**：分析 async-profiler 内存分配数据（`async-profiler-alloc`）
- **Java Heap Dump**：解析 hprof 格式堆转储，支持支配树、Retained Size、大对象、GC Roots、Retainer 分析（`heapdump-heap`）
- **Linux perf CPU**：分析 Linux perf 采集的 CPU 数据（`perf-cpu`）
- **Go pprof CPU**：分析 Go pprof CPU profile（`pprof-cpu`）
- **Go pprof Heap**：分析 Go pprof 堆内存 profile（`pprof-heap`）
- **Go pprof Goroutine**：分析 Goroutine 栈（`pprof-goroutine`）
- **Go pprof Block**：分析 Block 竞争（`pprof-block`）
- **Go pprof Mutex**：分析 Mutex 竞争（`pprof-mutex`）
- **Go pprof Batch**：批量分析多种 pprof profile（`pprof-all`）
- **Jemalloc Heap**：Jemalloc 堆内存分析（`jeprof-heap`）

### 可视化输出

- **交互式火焰图**：支持 CPU、内存、Tracing、pprof 等多种类型
- **调用图**：函数调用关系可视化
- **Top Functions**：热点函数排名统计
- **线程分析**：线程活动与统计
- **Java Heap 视图**：大对象、GC Roots、直方图、合并路径、Treemap
- **优化建议**：基于规则引擎自动生成性能优化建议

### 平台能力

- **双模式运行**：CLI 本地分析 + 后台服务模式
- **任务调度**：多 Worker 并发处理，优先级队列
- **多数据源**：Database 轮询、Kafka 消费（策略模式，可扩展）
- **HTTP Ingress**：HTTP 接口接收外部任务，持久化到数据库
- **嵌入式 WebUI**：内置 Web 服务器查看分析结果
- **对象存储**：支持 COS 远程存储和本地文件系统
- **回调通知**：三级 Callback URL 回退（task > source > global）
- **结果自动清理**：基于保留策略自动清理过期分析结果
- **HMAC 签名 URL**：支持 URL 鉴权，安全共享分析结果
- **OpenTelemetry**：分布式链路追踪
- **灵活部署**：同一二进制支持全功能、仅 Ingress、仅 Analyzer 等多种角色

## Supported Analysis Types

分析模式采用 `{profiler}-{event}` 命名规则。详细的请求示例和参数说明请参考 [Analysis Modes Reference](docs/analysis-modes.md)。

| Mode | Profiler | Event | 说明 | 输入格式 | 资源分类 |
|------|----------|-------|------|----------|----------|
| `async-profiler-cpu` | async-profiler | cpu | Java CPU 热点分析 | collapsed stack | CPU |
| `async-profiler-wall` | async-profiler | wall | Java Wall-clock 时间分析 | collapsed stack | CPU |
| `async-profiler-lock` | async-profiler | lock | Java 锁争用分析 | collapsed stack | Concurrency |
| `async-profiler-alloc` | async-profiler | alloc | Java 内存分配分析 | collapsed stack | Memory |
| `heapdump-heap` | heapdump | heap | Java 堆内存快照分析 | hprof | Memory |
| `perf-cpu` | perf | cpu | Linux perf CPU 分析 | collapsed stack | CPU |
| `pprof-cpu` | pprof | cpu | Go pprof CPU 分析 | pprof binary | CPU |
| `pprof-heap` | pprof | heap | Go pprof Heap 分析 | pprof binary | Memory |
| `pprof-goroutine` | pprof | goroutine | Go pprof Goroutine 分析 | pprof binary | Goroutine |
| `pprof-block` | pprof | block | Go pprof Block 分析 | pprof binary | Concurrency |
| `pprof-mutex` | pprof | mutex | Go pprof Mutex 分析 | pprof binary | Concurrency |
| `pprof-all` | pprof | all | Go 批量 pprof 分析 | pprof binary | Multiple |
| `jeprof-heap` | jeprof | heap | Jemalloc 堆内存分析 | jeprof | Memory |

## Project Structure

```
perf-analysis/
├── cmd/
│   ├── analyzer/              # 后台服务入口 (perf-analyzer)
│   └── cli/                   # CLI 工具入口 (perf-analysis)
│       └── cmd/               # 子命令 (analyze, serve, version)
├── internal/
│   ├── analyzer/              # 核心分析引擎 (模式定义/注册表工厂/各类分析器)
│   ├── parser/                # 数据解析器
│   │   ├── collapsed/         #   折叠栈格式 (perf/async-profiler)
│   │   ├── pprof/             #   Go pprof 格式
│   │   └── hprof/             #   Java 堆 dump (支配树/引用图/大对象/Retained Size)
│   ├── flamegraph/            # 火焰图数据生成
│   ├── callgraph/             # 调用图数据生成
│   ├── statistics/            # 统计分析 (Top 函数/线程统计)
│   ├── advisor/               # 性能优化建议
│   ├── formatter/             # 结果格式化器 (CPU/内存/堆/Tracing)
│   ├── webui/                 # 嵌入式 WebUI 服务器 + 前端资源
│   ├── scheduler/             # 任务调度器
│   │   └── source/            #   数据源策略 (Database/Kafka)
│   ├── ingress/               # 任务接入层 (HTTP Ingress)
│   ├── publisher/             # 分析结果发布 (Summary/元数据)
│   ├── storage/               # 存储抽象层 (COS/本地)
│   ├── repository/            # 数据库 ORM 层 (GORM AutoMigrate)
│   ├── service/               # 顶层服务编排
│   ├── integration/           # 集成测试
│   ├── mock/                  # 测试 Mock
│   └── testutil/              # 测试工具
├── pkg/
│   ├── config/                # 配置管理
│   ├── model/                 # 核心数据模型
│   ├── telemetry/             # OpenTelemetry 链路追踪
│   ├── pprof/                 # 服务自身性能采集
│   ├── utils/                 # 工具集 (Logger/Clock/Timer)
│   ├── collections/           # 数据结构 (BitSet/Pool)
│   ├── compression/           # 压缩工具
│   ├── errors/                # 自定义错误类型
│   ├── filter/                # 类过滤器
│   ├── parallel/              # 并行工作池
│   ├── profiling/             # 线程模型
│   └── writer/                # JSON 输出写入器
├── configs/
│   └── template/              # 配置模板
├── docs/                      # 参考文档
├── test/                      # 测试数据
├── scripts/                   # 脚本工具
├── Makefile
├── go.mod
└── README.md
```

## Requirements

- Go 1.24 or later
- PostgreSQL or MySQL database（服务模式必选）
- COS 对象存储（可选，支持本地存储替代）
- Kafka（可选，作为任务数据源）

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/perf-analysis/perf-analysis.git
cd perf-analysis
```

### 2. Install dependencies

```bash
make deps
```

### 3. Build

```bash
# 构建所有二进制（perf-analysis + perf-analyzer）
make build

# 仅构建 CLI
make build-cli

# 仅构建后台服务
make build-analyzer
```

### 4. CLI 本地分析

无需数据库和配置文件，直接分析本地 profiling 文件：

```bash
# 分析 Java CPU profiling 数据 (async-profiler)
./bin/perf-analysis analyze -i data.collapsed -m async-profiler-cpu

# 分析 Java Wall-clock 数据
./bin/perf-analysis analyze -i data.collapsed -m async-profiler-wall

# 分析 Go pprof CPU profile
./bin/perf-analysis analyze -i cpu.prof -m pprof-cpu

# 分析 Java Heap Dump
./bin/perf-analysis analyze -i heap.hprof -m heapdump-heap

# 分析 Linux perf 数据
./bin/perf-analysis analyze -i perf.collapsed -m perf-cpu

# 分析后自动启动 Web 服务查看结果
./bin/perf-analysis analyze -i data.collapsed -m async-profiler-cpu --serve

# 指定分析深度
./bin/perf-analysis analyze -i data.collapsed -m async-profiler-cpu --profile detailed
```

分析深度：`quick`（快速）、`standard`（标准，默认）、`detailed`（详细）

### 5. CLI 启动 Web 服务

查看已有的分析结果：

```bash
# 本地模式：指定数据目录
./bin/perf-analysis serve -d ./data -p 8080

# 远程存储模式：通过配置文件连接 COS
./bin/perf-analysis serve -c configs/config.yaml -p 8080
```

### 6. 后台服务模式

```bash
# 准备配置文件
cp configs/template/config.yaml configs/config.yaml
# 编辑 configs/config.yaml，填入数据库、存储等配置

# 启动后台服务
./bin/perf-analyzer -c configs/config.yaml
```

## Architecture

### 整体架构

```
                    ┌─────────────────────────────────┐
                    │         External Services        │
                    └──────┬──────────────┬────────────┘
                           │              │
                    HTTP POST        Callback URL
                           │              ▲
                           ▼              │
                  ┌────────────────┐      │
                  │  HTTP Ingress  │      │
                  │  (接收任务,     │      │
                  │   写入数据库)   │      │
                  └───────┬────────┘      │
                          │               │
                          ▼               │
                  ┌───────────────┐       │
                  │   Database    │       │
                  └───────┬───────┘       │
                          │               │
              ┌───────────┼───────────┐   │
              ▼           ▼           ▼   │
        ┌──────────┐ ┌─────────┐ ┌──────┐│
        │ DB Source│ │  Kafka  │ │ ...  ││
        │ (轮询)   │ │ Source  │ │      ││
        └────┬─────┘ └────┬────┘ └──┬───┘│
             └─────────┬───┘────────┘    │
                       ▼                 │
              ┌─────────────────┐        │
              │   Aggregator    │        │
              └────────┬────────┘        │
                       ▼                 │
              ┌─────────────────┐        │
              │   Scheduler     │        │
              │  (Worker Pool)  │        │
              └────────┬────────┘        │
                       ▼                 │
              ┌─────────────────┐  ┌─────┴──────┐
              │   Processor     │──│  Callback   │
              │  (分析 + 存储)   │  │  Notifier   │
              └────────┬────────┘  └────────────┘
                       ▼
              ┌─────────────────┐
              │  WebUI Server   │
              │  (结果可视化)    │
              └─────────────────┘
```

### 设计原则

- **高内聚低耦合**：每个模块职责单一，通过接口交互
- **接口驱动设计**：所有核心组件通过接口定义，支持 Mock 测试
- **依赖注入**：通过构造函数注入依赖
- **策略模式**：任务数据源（Database/Kafka）通过注册表工厂动态创建
- **注册表模式**：分析器工厂通过 `map[AnalysisMode]AnalyzerConstructor` 注册表管理，支持运行时扩展
- **分析时计算，展示时读取**：分析阶段生成完整预计算数据，WebUI 仅读取展示

### 部署模式

同一个 `perf-analyzer` 二进制通过配置项控制运行角色：

| 部署角色 | `scheduler.enabled` | `ingress.http.enabled` | `webui.enabled` | 说明 |
|---------|---------------------|------------------------|-----------------|------|
| 全功能节点 | `true` | `true` | `true` | 接收任务 + 分析 + 查看结果 |
| 仅 Analyzer | `true` | `false` | `false` | 只从数据库拉取任务并分析 |
| 仅 Ingress | `false` | `true` | `false` | 只接收 HTTP 任务写入数据库 |
| Analyzer + WebUI | `true` | `false` | `true` | 分析 + 查看，合并部署 |

### Callback URL 三级回退

任务完成后发送回调通知，URL 按以下优先级回退：

1. **Task 级别**：任务自身携带的 `callback_url`
2. **Source 级别**：数据源配置的 `callback_url`
3. **Global 级别**：全局配置的 `callback.default_url`

HTTP Ingress 支持 **降级保存**：当任务未携带 `callback_url` 时，自动将 Ingress 配置的 `callback_url` 写入任务记录。

## Documentation

| 文档 | 说明 |
|------|------|
| [Analysis Modes Reference](docs/analysis-modes.md) | 所有分析模式详解、HTTP API 接口定义、请求示例 |
| [Callback API Protocol](docs/callback-api-protocol.md) | 入站接口、出站回调协议、重试机制、接收方实现指南 |

## Configuration

详细配置参见 [configs/template/config.yaml](configs/template/config.yaml)。

### 核心配置项

| Section | Option | Description | Default |
|---------|--------|-------------|---------|
| `analysis` | `version` | 分析版本号 | `1.0.0` |
| `analysis` | `data_dir` | 数据目录 | `./data` |
| `analysis` | `max_worker` | 最大分析 Worker 数 | `5` |
| `database` | `type` | 数据库类型 (`postgres` / `mysql`) | `postgres` |
| `storage` | `type` | 存储类型 (`cos` / `local`) | `local` |
| `scheduler` | `enabled` | 是否启用任务调度器 | `true` |
| `scheduler` | `poll_interval` | 任务轮询间隔（秒） | `2` |
| `scheduler` | `worker_count` | 并发 Worker 数 | `5` |
| `ingress.http` | `enabled` | 是否启用 HTTP Ingress | `false` |
| `ingress.http` | `listen_addr` | Ingress 监听地址 | `:8081` |
| `ingress.http` | `callback_url` | Ingress 级别 Callback URL | `""` |
| `webui` | `enabled` | 是否启用嵌入式 WebUI | `false` |
| `webui` | `port` | WebUI 端口 | `8080` |
| `view_url` | `base_url` | 查看 URL 基础地址 | `""` (自动推导) |
| `view_url.auth` | `enabled` | 是否启用 HMAC URL 鉴权 | `false` |
| `callback` | `default_url` | 全局默认 Callback URL | `""` |
| `callback` | `max_retries` | 回调最大重试次数 | `3` |
| `retention` | `default` | 默认结果保留时长 | `168h` (7天) |

## Development

### Running Tests

```bash
# 运行所有测试
make test

# 带覆盖率
make test-coverage

# 带 Race Detector
make test-race

# 运行 Benchmark
make bench
```

### Code Quality

```bash
# 格式化代码
make fmt

# 运行 Linter
make lint

# 运行 vet
make vet
```

### Cross-platform Release

```bash
# 交叉编译 Linux/macOS/Windows (amd64/arm64)
make release
```

## Key Dependencies

| Dependency | Usage |
|------------|-------|
| `github.com/spf13/cobra` | CLI 框架 |
| `github.com/spf13/viper` | 配置管理 |
| `github.com/google/pprof` | pprof 数据解析 |
| `gorm.io/gorm` | ORM（支持 PostgreSQL / MySQL / SQLite / ClickHouse） |
| `github.com/tencentyun/cos-go-sdk-v5` | 腾讯云 COS 对象存储 |
| `go.opentelemetry.io/otel` | 分布式链路追踪 |
| `google.golang.org/grpc` | gRPC / Protobuf |
| `github.com/klauspost/compress` | 压缩工具 |

## License

MIT License

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request
