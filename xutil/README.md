# xutil

工具函数包，提供通用工具函数、异步任务（Future）、任务池（Pool）和重试（Retry）。

> **零第三方依赖**：本包只用标准库。它几乎会被所有模块编进去，任何第三方依赖都会
> 转嫁给全部使用者，因此需要上层能力时一律走扩展点注入，见下方「链路标识」。

## Future - 异步任务

`Future` 提供类似 Java Future 的异步编程能力，支持泛型。

两种启动方式，区别只在**执行载体**：

| | 跑在哪 | 并发上限 | 队列满时 |
|---|-------|---------|---------|
| `Async(fn)` | 每次新起一个 goroutine | **无** | 不存在这回事 |
| `AsyncWithPool(pool, fn)` | 复用池中的 worker | 池的 worker 数 | 阻塞调用方 |
| `TryAsyncWithPool(pool, fn)` | 同上 | 同上 | 立即以 `ErrPoolFull` 完成 |

少量、一次性的并行（比如并发调三个下游）用 `Async`；需要限流（处理一万个 item
但只要十个并发）用 `AsyncWithPool` —— 循环里 `Async` 一万次就是一万个 goroutine，
这正是后者存在的理由。

### 基本用法

```go
// 创建异步任务
f := xutil.Async(func() (string, error) {
    resp, err := http.Get("https://example.com")
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)
    return string(body), nil
})

// 阻塞等待结果
result, err := f.Get()
```

### 超时等待

```go
f := xutil.Async(func() (string, error) {
    return fetchData(), nil
})

result, err := f.GetWithTimeout(5 * time.Second)
if errors.Is(err, context.DeadlineExceeded) {
    // 超时处理
}
```

### 检查状态

```go
f := xutil.Async(func() (int, error) {
    return compute(), nil
})

if f.IsDone() {
    result, err := f.Get() // 不会阻塞
}
```

## Pool - 任务池

`Pool` 是固定 worker 数量的并发任务池，支持提交任务、同步等待和 Future 集成。

### 使用全局任务池

内置 100 worker 的全局任务池，直接调用包级函数：

```go
// 提交任务（fire-and-forget），队列满时返回 false，不会阻塞
if !xutil.TrySubmit(func() { sendEmail(user) }) {
    // 队列满或池已关闭，按需降级：同步执行、丢弃、或记一次指标
}
```

**全局池上没有阻塞背压，这是有意的。** 它由进程内所有调用方共用 ——
一个业务的慢任务把队列填满后，阻塞会传播给毫不相干的另一个业务，
而后者既不知道前者的存在，也没有「慢下来」的余地，它只是想扔个埋点。

需要背压请用 `NewPool` 自建池并调用 `pool.Submit`：那是你独占的池子，
知道容量，也控制得了生产速度。

### 创建自定义任务池

```go
pool := xutil.NewPool(10) // 10 个 worker
defer pool.Shutdown()      // 优雅关闭，等待所有任务完成

// 提交任务，队列满时阻塞到有空位（背压）
pool.Submit(func() {
    processItem(item)
})

// 不想等就用 TrySubmit，队列满直接返回 false
pool.TrySubmit(func() {
    processItem(item)
})

// 提交并获取 Future
f := xutil.AsyncWithPool(pool, func() (Result, error) {
    return fetchResult(), nil
})
result, err := f.Get()
```

### 批量并发

```go
pool := xutil.NewPool(8)
defer pool.Shutdown()

urls := []string{"url1", "url2", "url3"}
futures := make([]*xutil.Future[string], len(urls))

for i, url := range urls {
    u := url
    futures[i] = xutil.AsyncWithPool(pool, func() (string, error) {
        return fetch(u)
    })
}

// 收集结果
for _, f := range futures {
    result, err := f.GetWithTimeout(10 * time.Second)
    if err != nil {
        // 处理超时或错误
        continue
    }
    process(result)
}
```

## API 参考

### Future

| 方法 | 说明 |
|------|------|
| `Async(fn) *Future[T]` | 启动异步任务；任务中的 panic 会转成 error 由 `Get` 返回 |
| `Get() (T, error)` | 阻塞等待结果 |
| `GetWithContext(ctx) (T, error)` | 等待结果，ctx 结束时返回 `ctx.Err()` |
| `GetWithTimeout(d) (T, error)` | 超时等待，超时返回 `context.DeadlineExceeded` |
| `IsDone() bool` | 非阻塞检查是否完成 |

### Pool

| 方法 | 说明 |
|------|------|
| `TrySubmit(fn) bool` | 向全局任务池提交任务，队列满返回 false，不阻塞 |
| `NewPool(n) *Pool` | 创建 n 个 worker 的自定义任务池 |
| `pool.Submit(fn) bool` | 提交任务，队列满时阻塞；nil / 池已关闭返回 false |
| `pool.TrySubmit(fn) bool` | 提交任务，队列满 / nil / 池已关闭均返回 false，不阻塞 |
| `AsyncWithPool[T](pool, fn) *Future[T]` | 在池中执行任务，返回 Future；队列满时**阻塞**，池已关闭时立即以 `ErrPoolClosed` 完成 |
| `TryAsyncWithPool[T](pool, fn) *Future[T]` | 同上但不阻塞，队列满时立即以 `ErrPoolFull` 完成 |
| `pool.Shutdown()` | 优雅关闭，等待所有任务完成，多次调用安全 |

关于任务池的三条保证：

- **任务里的 panic 被隔离**：转成日志记录，既不崩进程，也不会杀死 worker
- **并发 Submit 与 Shutdown 是安全的**：发送与关闭互斥，不会出现 send on closed channel。
  阻塞在队列满上的 `Submit` 会被 `Shutdown` 唤醒并返回 false —— 否则它会一直持着读锁，
  `Shutdown` 永远取不到写锁，而 Go 的 `RWMutex` 写者优先，后续所有 `Submit` 会跟着挂起
- **`Submit` 返回 true 则任务一定会被执行**：发送在读锁内完成，此时队列不可能已关闭
- **全局池是惰性创建的**：只 import xutil 而不调用 `Submit` 时不会启动任何 worker goroutine

### Retry

| 方法 | 说明 |
|------|------|
| `Retry(fn, attempts, sleep)` | 固定间隔重试 |
| `RetryWithContext(ctx, fn, attempts, sleep)` | 可取消版本，ctx 结束时立即停止 |
| `RetryWithBackoff(fn, attempts, initial, max)` | 指数退避重试 |
| `RetryWithBackoffContext(ctx, fn, attempts, initial, max)` | 可取消的指数退避 |

重试常出现在初始化路径上，`attempts × sleep` 可能长达数十秒。服务关闭时不该被它硬拖住，
因此涉及外部依赖的重试建议用带 ctx 的版本。

## 链路标识

`GetTraceIDFromCtx` / `GetSpanIDFromCtx` / `GetTraceAndSpanIDFromCtx` 从 ctx 中读取链路标识，
供 xlog 关联日志、xmetric 生成 Exemplar 使用。

本包**不依赖 OpenTelemetry** —— 提取逻辑由 xtrace 在 `init` 阶段注入：

```go
// xtrace 中自动完成，业务无需关心
xutil.SetTraceContextExtractor(func(ctx context.Context) (traceID, spanID string) { ... })
```

因此：

- 服务 import 了 xtrace（通常经由 xone 的任一上层模块）→ 日志自动带上 TraceID / SpanID
- 完全不用 xtrace → 三个函数返回空串，而不必为此编进整棵 otel trace/attribute 树
- 直接使用 OpenTelemetry 而不经由 xtrace 时，可自行调用 `SetTraceContextExtractor` 注入

热点路径（如每条日志）建议用 `GetTraceAndSpanIDFromCtx` 一次取两个值。
