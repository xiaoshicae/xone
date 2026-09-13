# xutil

工具函数包，提供通用工具函数、异步任务（Future）和任务池（Pool）。

> **零第三方依赖**：本包只用标准库。它几乎会被所有模块编进去，任何第三方依赖都会
> 转嫁给全部使用者，因此需要上层能力时一律走扩展点注入，见下方「链路标识」。

## Future - 异步任务

`Future` 提供类似 Java Future 的异步编程能力，支持泛型。

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
// 提交任务（fire-and-forget）
xutil.Submit(func() {
    sendEmail(user)
})
```

### 创建自定义任务池

```go
pool := xutil.NewPool(10) // 10 个 worker
defer pool.Shutdown()      // 优雅关闭，等待所有任务完成

// 提交任务
pool.Submit(func() {
    processItem(item)
})

// 提交并获取 Future
f := xutil.Go(pool, func() (Result, error) {
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
    futures[i] = xutil.Go(pool, func() (string, error) {
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
| `Async(fn) *Future[T]` | 启动异步任务 |
| `Get() (T, error)` | 阻塞等待结果 |
| `GetWithTimeout(d) (T, error)` | 超时等待，超时返回 `context.DeadlineExceeded` |
| `IsDone() bool` | 非阻塞检查是否完成 |

### Pool

| 方法 | 说明 |
|------|------|
| `Submit(fn)` | 向全局任务池提交任务 |
| `NewPool(n) *Pool` | 创建 n 个 worker 的自定义任务池 |
| `pool.Submit(fn)` | 向自定义任务池提交任务 |
| `Go[T](pool, fn) *Future[T]` | 提交任务，返回 Future |
| `pool.Shutdown()` | 优雅关闭，等待所有任务完成 |

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
