# xmetric

Prometheus 指标采集模块，提供快捷打点函数、`/metrics` 端点和 `xlog.Error` 自动上报。

## 配置

```yaml
XMetric:
  Namespace: "myapp"              # 指标命名空间前缀（可选）
  ConstLabels:                    # 全局常量标签，自动附加到所有指标上（可选）
    env: "prod"
    cluster: "cn-east"
  HttpDurationBuckets: [...]      # HTTP 入站/出站请求耗时桶边界（ms），默认 [1,5,10,25,50,100,250,500,1000,2500,5000,10000]
  HistogramObserveBuckets: [...]  # HistogramObserve() API 业务指标桶边界（秒），默认 prometheus.DefBuckets
  EnableGoMetrics: true           # Go runtime 指标，默认 true
  EnableProcessMetrics: true      # 进程指标，默认 true
  EnableLogErrorMetric: true      # xlog.Error 自动上报，默认 true
```

## 指标类型选择指南

### Counter — 累计计数，只增不减

> 核心问题：**"一共发生了多少次/多少量？"**

适合统计**累计值**，配合 `rate()` 算速率。重启后归零，Prometheus 自动处理。

| 场景 | 示例 |
|------|------|
| 业务事件计次 | 下单次数、登录次数、支付次数 |
| 错误计次 | 接口报错次数、超时次数、熔断触发次数 |
| 数据量累计 | 累计金额、传输字节数、消费消息数 |

```go
// CounterInc: +1 计次
xmetric.CounterInc("order_created_total", xmetric.T("channel", "wechat"))
xmetric.CounterInc("login_total", xmetric.T("method", "sms"))
xmetric.CounterInc("payment_failed_total", xmetric.T("reason", "timeout"))

// CounterAdd: +N 计量（如金额、字节数）
xmetric.CounterAdd("payment_amount_total", 99.9, xmetric.T("channel", "alipay"))
xmetric.CounterAdd("mq_consumed_total", 1, xmetric.T("topic", "order"))
```

常用 PromQL：
```promql
rate(myapp_order_created_total[5m])                     # 每秒下单数
sum by(reason) (rate(myapp_payment_failed_total[5m]))   # 按原因分组的失败速率
```

---

### Gauge — 实时快照，可增可减

> 核心问题：**"当前值是多少？"**

适合反映**某一刻的状态**，不需要 `rate()`，直接看值。

| 场景 | 示例 |
|------|------|
| 连接/会话数 | WebSocket 连接数、在线用户数 |
| 队列/缓存状态 | 消息队列积压量、缓存命中率 |
| 资源水位 | 线程池活跃线程数、连接池可用连接数 |

```go
// GaugeSet: 直接设置当前值
xmetric.GaugeSet("ws_connections", 42, xmetric.T("app", "chat"))
xmetric.GaugeSet("mq_pending_messages", 1500, xmetric.T("queue", "order"))
xmetric.GaugeSet("thread_pool_active", 8, xmetric.T("pool", "worker"))

// GaugeInc / GaugeDec: 在当前值上 +1 / -1
xmetric.GaugeInc("ws_connections", xmetric.T("app", "chat"))   // 新连接
xmetric.GaugeDec("ws_connections", xmetric.T("app", "chat"))   // 断开连接
```

---

### Histogram — 分布统计，分桶计数

> 核心问题：**"这个值的分布情况如何？P99 是多少？"**

适合统计**耗时、大小等需要看分布和分位数**的值。自动生成 `_bucket`、`_count`、`_sum` 三组数据。

| 场景 | 示例 |
|------|------|
| 接口耗时 | HTTP 请求耗时、RPC 调用耗时 |
| 外部调用耗时 | 数据库查询耗时、Redis 调用耗时、三方 API 耗时 |
| 数据大小 | 请求体大小、响应体大小 |

```go
// 接口耗时（毫秒）
xmetric.HistogramObserve("db_query_duration_ms", 12.5, xmetric.T("table", "orders"))
xmetric.HistogramObserve("redis_call_duration_ms", 0.8, xmetric.T("cmd", "GET"))
xmetric.HistogramObserve("third_api_duration_ms", 230, xmetric.T("api", "sms"))

// 数据大小（字节）
xmetric.HistogramObserve("response_size_bytes", 4096, xmetric.T("endpoint", "/api/users"))
```

常用 PromQL：
```promql
histogram_quantile(0.99, rate(myapp_db_query_duration_ms_bucket[5m]))   # P99 延迟
histogram_quantile(0.50, rate(myapp_redis_call_duration_ms_bucket[5m])) # P50 中位数
```

---

### 一句话速查

| 你想知道... | 用 | 函数 |
|------------|-----|------|
| 一共发生了多少次 | **Counter** | `CounterInc` |
| 一共累计了多少量 | **Counter** | `CounterAdd` |
| 当前值是多少 | **Gauge** | `GaugeSet` / `GaugeInc` / `GaugeDec` |
| 耗时/大小分布、P99 | **Histogram** | `HistogramObserve` |

## 标签使用

使用 `xmetric.T(name, value)` 创建强类型标签，编译期检查键值配对：

```go
xmetric.CounterInc("order_total",
    xmetric.T("channel", "wechat"),
    xmetric.T("status", "success"),
)
```

- 标签顺序无关（内部自动排序）
- Namespace 从配置自动读取，无需手动指定
- 指标懒注册，首次调用自动创建并注册到 Registry

## /metrics 端点

xgin 启用 MetricMiddleware（默认开启）时自动注册 `/metrics` 端点，无需手动配置。

自定义路径：

```go
gx := xgin.New(options.MetricsPath("/custom/metrics")).Build()
```

## xlog.Error 自动上报

启用后（默认开启），调用 `xlog.Error()` 时自动递增 `log_errors_total` 计数器：

```
myapp_log_errors_total{level="error"} 42
```

trace_id 通过 Prometheus Exemplar 附加（不作为 label，避免高基数），Grafana 可从 exemplar 跳转到链路追踪系统。

## Gin HTTP 中间件

默认开启，通过 option 关闭：

```go
// 默认开启，无需额外配置
gx := xgin.New().Build()

// 显式关闭
gx := xgin.New(options.EnableMetricMiddleware(false)).Build()
```

采集指标：
- `http_requests_total{method, path, status}` — 请求数量
- `http_request_duration_ms{method, path, status}` — 请求耗时（毫秒）

## HTTP 出站请求指标

xhttp 模块默认启用出站请求 Prometheus 指标采集，自动记录所有 HTTP 客户端请求的调用量和耗时。

采集指标：
- `http_client_requests_total{method, host, status}` — 出站请求总数
- `http_client_request_duration_ms{method, host, status}` — 出站请求耗时（毫秒）

标签说明：
- `method` — HTTP 方法（GET、POST 等）
- `host` — 目标主机（如 `api.example.com`）
- `status` — HTTP 状态码（`200`、`500` 等），网络错误时为 `0`

通过 xhttp 配置关闭：

```yaml
XHttp:
  EnableMetric: false
```

常用 PromQL：

```promql
rate(myapp_http_client_requests_total[5m])                              # 出站请求速率
rate(myapp_http_client_requests_total{status=~"5.."}[5m])               # 5xx 错误速率
histogram_quantile(0.99, rate(myapp_http_client_request_duration_ms_bucket[5m]))  # P99 耗时
```

## 自定义指标注册

需要完全控制时，直接使用 prometheus API：

```go
counter := prometheus.NewCounterVec(...)
xmetric.MustRegister(counter)
```
