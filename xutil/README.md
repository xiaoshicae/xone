# xutil

工具函数包，提供通用工具函数、异步任务（Future）和任务池（Pool）。

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
