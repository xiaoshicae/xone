# 性能瓶颈分析

对指定模块进行性能瓶颈扫描，定位热点代码并给出修复建议。

使用方法: /perf <模块名>

示例:
- /perf xhttp
- /perf xgorm
- /perf xlog

$ARGUMENTS

---

请对 `$ARGUMENTS` 模块进行性能瓶颈分析：

## 1. 运行 Benchmark（如果存在）

```bash
# 查找并运行已有的 benchmark
go test -gcflags="all=-N -l" -bench=. -benchmem ./$ARGUMENTS/... 2>&1 || true
```

如果存在 benchmark 结果，分析 `ns/op`、`B/op`、`allocs/op` 指标。

## 2. 逐文件扫描性能瓶颈

读取 `$ARGUMENTS/` 目录下所有 `.go` 文件（排除 `_test.go`），逐一排查以下问题：

### 内存分配（高频问题）

- 循环内重复创建切片/map，未预分配容量（应 `make([]T, 0, n)`）
- 字符串拼接使用 `+` 或 `fmt.Sprintf`，应改用 `strings.Builder`
- 频繁创建临时对象，适合使用 `sync.Pool`
- 返回值为大结构体而非指针，导致不必要的拷贝
- `[]byte` 与 `string` 之间反复转换

### 锁与并发

- 使用 `sync.Mutex` 保护读多写少的场景，应改用 `sync.RWMutex`
- 锁粒度过大（整个函数加锁），可缩小临界区
- 热路径上存在不必要的 `defer mu.Unlock()`（defer 有额外开销）
- 全局锁竞争（高并发下成为瓶颈）

### I/O 与网络

- HTTP 客户端未复用（每次请求创建新 `http.Client`）
- 未设置连接池或连接池参数不合理
- 响应 Body 未及时关闭（`defer resp.Body.Close()`）
- 未使用缓冲 I/O（`bufio.Reader` / `bufio.Writer`）
- 同步 I/O 阻塞 goroutine，可改异步或批量处理

### 序列化与反射

- 热路径使用 `encoding/json`，可改用 `json-iterator` 或 `sonic`
- 使用 `reflect` 做类型判断，可改为类型断言或泛型
- 正则表达式未预编译（`regexp.MustCompile` 应放到包级变量）
- `fmt.Sprintf` 用于简单拼接，`+` 或 `strings.Builder` 更高效

### 算法与数据结构

- O(n²) 可优化为 O(n log n) 或 O(n)
- 线性查找可改用 map 查找
- 重复计算可缓存结果
- 排序后二分查找 vs 每次全量遍历

### 数据库（如果涉及）

- N+1 查询（循环内逐条查询）
- 缺少索引提示
- 未使用批量插入/更新
- 查询返回过多不需要的字段（`SELECT *`）

## 3. 生成性能分析报告

以表格形式输出，按影响程度排列：

| 影响 | 文件:行号 | 瓶颈类型 | 问题描述 | 修复建议 | 预估收益 |
|------|-----------|----------|----------|----------|----------|
| 高 | xhttp/client.go:58 | 内存分配 | 循环内未预分配切片 | `make([]T, 0, len(src))` | 减少 GC 压力 |

影响分级：
- **高**：热路径上的问题，直接影响吞吐量或延迟
- **中**：非热路径但会在高负载下暴露
- **低**：微优化，仅在极端场景有收益

## 4. 给出优化优先级建议

根据发现的问题，给出优化的优先级建议：
1. 先修复哪些问题收益最大
2. 哪些可以快速修复（一行改动）
3. 哪些需要重构（需要评估风险）