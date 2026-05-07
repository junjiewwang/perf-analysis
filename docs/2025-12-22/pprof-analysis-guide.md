# Go pprof 性能分析完整指南

## 目录

1. [概述](#概述)
2. [Profile 类型详解](#profile-类型详解)
3. [分析工具使用](#分析工具使用)
4. [CPU 性能分析](#cpu-性能分析)
5. [内存分析](#内存分析)
6. [Goroutine 分析](#goroutine-分析)
7. [阻塞与锁分析](#阻塞与锁分析)
8. [性能优化建议](#性能优化建议)
9. [实战案例](#实战案例)

---

## 概述

### 什么是 pprof？

pprof 是 Go 语言内置的性能分析工具，通过采样的方式收集程序运行时的性能数据。它可以帮助你：

- 找出 CPU 热点函数
- 发现内存泄漏
- 定位 Goroutine 泄漏
- 分析锁竞争问题

### 数据采集原理

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           采样原理                                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  CPU Profile:                                                           │
│  ┌─────┬─────┬─────┬─────┬─────┬─────┬─────┬─────┐                     │
│  │ 采样 │ 采样 │ 采样 │ 采样 │ 采样 │ 采样 │ 采样 │ 采样 │  (默认100Hz)    │
│  └──┬──┴──┬──┴──┬──┴──┬──┴──┬──┴──┬──┴──┬──┴──┬──┘                     │
│     │     │     │     │     │     │     │     │                         │
│     ▼     ▼     ▼     ▼     ▼     ▼     ▼     ▼                         │
│  记录当前正在执行的函数调用栈                                              │
│                                                                         │
│  Heap Profile:                                                          │
│  每次内存分配时记录调用栈和分配大小                                         │
│                                                                         │
│  Goroutine Profile:                                                     │
│  快照所有 goroutine 的当前调用栈                                          │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Profile 类型详解

| Profile 类型 | 文件后缀 | 采集方式 | 用途 |
|-------------|---------|---------|------|
| **CPU** | cpu_*.pprof | 持续采样 | 找出 CPU 消耗最多的函数 |
| **Heap** | heap_*.pprof | 瞬时快照 | 分析内存使用和泄漏 |
| **Goroutine** | goroutine_*.pprof | 瞬时快照 | 分析 goroutine 分布和泄漏 |
| **Block** | block_*.pprof | 累积统计 | 分析阻塞操作 (channel, select) |
| **Mutex** | mutex_*.pprof | 累积统计 | 分析锁竞争 |
| **Allocs** | allocs_*.pprof | 累积统计 | 分析内存分配频率 |

### 关键指标解释

#### flat vs cum

```
      flat  flat%   sum%        cum   cum%
     1.20s 24.00% 24.00%      2.50s 50.00%  main.processData
     0.80s 16.00% 40.00%      0.80s 16.00%  runtime.memmove
     ^^^^                     ^^^^
     flat                     cum

flat (自身时间): 函数自身代码消耗的时间，不包括调用其他函数
cum (累积时间): 函数自身 + 调用的所有子函数消耗的总时间
```

**分析技巧**：
- `flat` 高：函数自身有性能问题，需要优化函数内部代码
- `cum` 高但 `flat` 低：问题在被调用的子函数中

#### 内存指标

| 指标 | 含义 | 用途 |
|-----|------|------|
| `inuse_space` | 当前使用中的内存大小 | 找内存占用大户 |
| `inuse_objects` | 当前存活的对象数量 | 找对象数量多的地方 |
| `alloc_space` | 累计分配的内存大小 | 找频繁分配的热点 |
| `alloc_objects` | 累计分配的对象数量 | 找频繁创建对象的地方 |

---

## 分析工具使用

### 1. 自动化分析脚本

```bash
# 运行自动分析脚本
./scripts/analyze-pprof.sh ./pprof ./pprof-report

# 查看生成的报告
cat ./pprof-report/SUMMARY.md
```

### 2. Web UI (推荐)

```bash
# 启动交互式 Web 界面
go tool pprof -http=:8080 ./pprof/cpu/cpu_20250122_150400.pprof
```

Web UI 功能：
- **Top**: 热点函数列表
- **Graph**: 调用关系图
- **Flame Graph**: 火焰图
- **Peek**: 查看函数的调用者和被调用者
- **Source**: 源码级别的性能数据

### 3. 命令行交互模式

```bash
go tool pprof ./pprof/cpu/cpu_20250122_150400.pprof

# 常用命令
(pprof) top           # 显示热点函数
(pprof) top20         # 显示前20个
(pprof) top -cum      # 按累积时间排序
(pprof) list funcName # 查看函数源码
(pprof) web           # 浏览器打开调用图
(pprof) tree          # 树形展示
(pprof) traces        # 显示所有调用栈
(pprof) help          # 查看所有命令
```

### 4. 直接生成报告

```bash
# 生成文本报告
go tool pprof -top ./pprof/cpu/cpu_*.pprof > cpu_top.txt

# 生成 SVG 调用图
go tool pprof -svg ./pprof/cpu/cpu_*.pprof > cpu.svg

# 生成 PNG 图片
go tool pprof -png ./pprof/heap/heap_*.pprof > heap.png
```

---

## CPU 性能分析

### 分析流程

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        CPU 分析流程                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. 查看 Top 函数                                                        │
│     ↓                                                                   │
│  2. 识别热点 (flat% > 5% 的函数)                                         │
│     ↓                                                                   │
│  3. 分析调用链 (cum 高的入口函数)                                         │
│     ↓                                                                   │
│  4. 查看源码 (list 命令)                                                 │
│     ↓                                                                   │
│  5. 定位具体代码行                                                        │
│     ↓                                                                   │
│  6. 制定优化方案                                                          │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 实操步骤

```bash
# Step 1: 启动分析
go tool pprof -http=:8080 ./pprof/cpu/cpu_*.pprof

# Step 2: 在 Web UI 中
# - 点击 "Top" 查看热点函数
# - 点击 "Flame Graph" 查看火焰图
# - 点击函数名进入 "Source" 查看源码

# Step 3: 命令行深入分析
go tool pprof ./pprof/cpu/cpu_*.pprof

(pprof) top10 -cum
# 找出累积时间最高的函数

(pprof) list processData
# 查看 processData 函数的逐行消耗

(pprof) peek processData
# 查看谁调用了 processData，processData 调用了谁
```

### 常见 CPU 问题模式

| 模式 | 特征 | 优化方向 |
|-----|------|---------|
| 计算密集 | 单个函数 flat 很高 | 算法优化、并行化 |
| 序列化开销 | json/xml 相关函数占比高 | 换用更快的序列化库 |
| 内存分配 | runtime.mallocgc 占比高 | 减少分配、使用对象池 |
| GC 压力 | runtime.gcBg* 占比高 | 减少对象分配 |
| 锁竞争 | sync.* 函数占比高 | 减少锁粒度、无锁设计 |

---

## 内存分析

### 分析流程

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        内存分析流程                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. 查看当前内存使用 (inuse_space)                                       │
│     ↓                                                                   │
│  2. 识别内存大户                                                         │
│     ↓                                                                   │
│  3. 对比多个时间点 (发现泄漏)                                             │
│     ↓                                                                   │
│  4. 分析分配热点 (alloc_space)                                           │
│     ↓                                                                   │
│  5. 查看分配调用栈                                                        │
│     ↓                                                                   │
│  6. 定位泄漏点或优化分配                                                   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 实操步骤

```bash
# Step 1: 查看当前内存使用
go tool pprof -http=:8080 ./pprof/heap/heap_*.pprof

# Step 2: 命令行分析
go tool pprof ./pprof/heap/heap_*.pprof

(pprof) top -inuse_space
# 当前占用内存最多的函数

(pprof) top -alloc_space
# 累计分配内存最多的函数

# Step 3: 内存泄漏检测 - 对比两个时间点
go tool pprof -diff_base=heap_early.pprof heap_late.pprof

(pprof) top -inuse_space
# 正值 = 内存增长，可能是泄漏
```

### 内存泄漏判断标准

```
内存泄漏特征:
┌─────────────────────────────────────────────────────────────────────────┐
│                                                                         │
│  时间 ─────────────────────────────────────────────────────────►        │
│                                                                         │
│  内存                                                                    │
│   ▲                                                    ┌───┐            │
│   │                                              ┌─────┘   │            │
│   │                                        ┌─────┘         │  泄漏      │
│   │                                  ┌─────┘               │  (持续增长) │
│   │                            ┌─────┘                     │            │
│   │       ┌───┐          ┌─────┘                           │            │
│   │  ┌────┘   └────┐ ────┘                                 │            │
│   │──┘             └─────────────────────────────────────  │  正常      │
│   │                                                        │  (有涨有跌) │
│   └────────────────────────────────────────────────────────┴───────────►│
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 常见内存问题

| 问题 | 特征 | 解决方案 |
|-----|------|---------|
| 大对象 | 单个分配 > 1MB | 分块处理、流式处理 |
| 频繁分配 | alloc_objects 很高 | sync.Pool、预分配 |
| 字符串拼接 | strings 相关函数 | strings.Builder |
| 切片扩容 | append 相关 | 预分配容量 |
| 泄漏 | inuse 持续增长 | 检查引用、及时释放 |

---

## Goroutine 分析

### 分析流程

```bash
# Step 1: 查看 goroutine 分布
go tool pprof ./pprof/goroutine/goroutine_*.pprof

(pprof) top
# 显示 goroutine 堆积在哪些函数

(pprof) traces
# 显示所有 goroutine 的完整堆栈

# Step 2: 泄漏检测 - 对比两个时间点
go tool pprof -diff_base=goroutine_early.pprof goroutine_late.pprof

(pprof) top
# 正值 = goroutine 数量增加
```

### Goroutine 泄漏模式

| 模式 | 特征 | 原因 |
|-----|------|------|
| Channel 阻塞 | 堆积在 chan receive/send | 没有发送者/接收者 |
| Select 阻塞 | 堆积在 select | 所有 case 都阻塞 |
| 锁等待 | 堆积在 Lock | 死锁或长时间持锁 |
| IO 等待 | 堆积在 Read/Write | 连接未关闭 |
| Sleep | 堆积在 time.Sleep | 无限循环中的 sleep |

### 排查示例

```
(pprof) top
Showing nodes accounting for 1000, 100% of 1000 total
      flat  flat%   sum%        cum   cum%
      800    80%    80%        800    80%  runtime.gopark
      150    15%    95%        150    15%  runtime.chanrecv1
       50     5%   100%         50     5%  sync.(*Mutex).Lock

分析:
- 800 个 goroutine 在 gopark (等待状态)
- 150 个在等待 channel 接收
- 50 个在等待获取锁

下一步: 用 traces 查看具体堆栈
```

---

## 阻塞与锁分析

### Block Profile 分析

```bash
go tool pprof ./pprof/block/block_*.pprof

(pprof) top
# 显示阻塞时间最长的操作

# 常见阻塞点:
# - channel 操作
# - select 语句
# - sync.Cond.Wait
```

### Mutex Profile 分析

```bash
go tool pprof ./pprof/mutex/mutex_*.pprof

(pprof) top
# 显示锁等待时间最长的地方

# 优化方向:
# - 减小锁粒度
# - 使用读写锁
# - 无锁数据结构
```

---

## 性能优化建议

### CPU 优化清单

- [ ] 使用更高效的算法
- [ ] 减少不必要的计算
- [ ] 缓存计算结果
- [ ] 并行化处理
- [ ] 使用更快的序列化库 (如 sonic 替代 encoding/json)
- [ ] 减少反射使用

### 内存优化清单

- [ ] 使用 sync.Pool 复用对象
- [ ] 预分配切片容量
- [ ] 使用 strings.Builder 拼接字符串
- [ ] 避免在循环中创建对象
- [ ] 及时释放不再使用的引用
- [ ] 使用指针避免大结构体复制

### Goroutine 优化清单

- [ ] 使用 worker pool 限制并发数
- [ ] 确保 channel 有发送者和接收者
- [ ] 使用 context 控制 goroutine 生命周期
- [ ] 避免 goroutine 泄漏
- [ ] 合理设置超时

### 锁优化清单

- [ ] 减小临界区范围
- [ ] 使用 RWMutex 替代 Mutex
- [ ] 考虑无锁数据结构
- [ ] 避免锁嵌套
- [ ] 使用 atomic 操作替代锁

---

## 实战案例

### 案例1: CPU 热点优化

**问题**: JSON 序列化占用 30% CPU

```
(pprof) top
     30%  encoding/json.Marshal
```

**解决方案**:
```go
// Before
data, _ := json.Marshal(obj)

// After - 使用 sonic (字节跳动开源)
import "github.com/bytedance/sonic"
data, _ := sonic.Marshal(obj)
```

### 案例2: 内存泄漏

**问题**: 内存持续增长

```
(pprof) top -diff_base=heap_t1.pprof heap_t2.pprof
    +50MB  myapp.(*Cache).Set
```

**解决方案**:
```go
// Before - 无限增长的 cache
func (c *Cache) Set(key string, value interface{}) {
    c.data[key] = value
}

// After - 带 TTL 的 cache
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
    c.data[key] = &entry{value: value, expireAt: time.Now().Add(ttl)}
    c.scheduleCleanup()
}
```

### 案例3: Goroutine 泄漏

**问题**: Goroutine 数量持续增加

```
(pprof) top -diff_base=goroutine_t1.pprof goroutine_t2.pprof
    +1000  myapp.processRequest
```

**解决方案**:
```go
// Before - 没有超时控制
go func() {
    result := <-ch  // 可能永远阻塞
    process(result)
}()

// After - 带超时控制
go func() {
    select {
    case result := <-ch:
        process(result)
    case <-time.After(30 * time.Second):
        log.Warn("timeout waiting for result")
    case <-ctx.Done():
        return
    }
}()
```

---

## 快速参考卡

### 常用命令

```bash
# 启动 Web UI
go tool pprof -http=:8080 profile.pprof

# 查看热点
go tool pprof -top profile.pprof

# 对比分析
go tool pprof -diff_base=old.pprof new.pprof

# 生成 SVG
go tool pprof -svg profile.pprof > graph.svg
```

### pprof 交互命令

| 命令 | 说明 |
|-----|------|
| `top [n]` | 显示前 n 个热点 |
| `top -cum` | 按累积时间排序 |
| `list func` | 显示函数源码 |
| `peek func` | 显示调用者和被调用者 |
| `web` | 浏览器打开调用图 |
| `tree` | 树形显示 |
| `traces` | 显示所有调用栈 |

### 内存分析选项

| 选项 | 说明 |
|-----|------|
| `-inuse_space` | 当前使用的内存 |
| `-inuse_objects` | 当前对象数 |
| `-alloc_space` | 累计分配内存 |
| `-alloc_objects` | 累计分配对象数 |
