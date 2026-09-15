## XRedis 模块

### 1. 模块简介

基于 [go-redis](https://github.com/redis/go-redis) 封装的 Redis 客户端模块，提供：

- 开箱即用的连接池管理
- 单实例 / 多实例支持
- OpenTelemetry 链路追踪集成
- 连接验证（Ping 重试）
- 密码脱敏日志

### 2. 配置参数

#### 单实例模式

```yaml
XRedis:
  Addr: "localhost:6379"      # Redis 地址（required，default "localhost:6379"）
  Password: ""                # 认证密码（optional）
  DB: 0                       # 数据库编号（optional，default 0）
  Username: ""                # Redis 6.0+ ACL 用户名（optional）
  DialTimeout: "500ms"        # 建连超时（optional，default "500ms"）
  ReadTimeout: "500ms"        # 读超时（optional，default "500ms"）
  WriteTimeout: "500ms"       # 写超时（optional，default "500ms"）
  PoolSize: 0                 # 连接池大小（optional，default 0 = 10 * runtime.GOMAXPROCS）
  MinIdleConns: 5             # 最小空闲连接数（optional，default 5）
  MaxIdleConns: 0             # 最大空闲连接数（optional，default 0 = 无限制）
  MaxActiveConns: 0           # 最大活跃连接数（optional，default 0 = 无限制）
  PoolTimeout: "1s"           # 连接池获取超时（optional，default "1s"）
  ConnMaxIdleTime: "5m"       # 空闲连接最大存活时间（optional，default "5m"）
  ConnMaxLifetime: "5m"       # 连接最大存活时间（optional，default "5m"）
  MaxRetries: 0               # 最大重试次数（optional，default 0 = go-redis 默认 3 次，-1 禁用）
  MinRetryBackoff: ""         # 最小重试退避时间（optional，go-redis 默认 8ms，"-1" 禁用）
  MaxRetryBackoff: ""         # 最大重试退避时间（optional，go-redis 默认 512ms，"-1" 禁用）
```

#### 多实例模式

```yaml
XRedis:
  - Name: "cache"
    Addr: "redis-cache:6379"
    DB: 0
  - Name: "session"
    Addr: "redis-session:6379"
    DB: 1
    Password: "secret"
```

### 3. API 接口

```go
// C 获取 redis client（默认获取第一个 client）
func C(name ...string) *redis.Client
```

### 4. 使用示例

```go
package main

import (
    "context"

    "github.com/xiaoshicae/xone/v3/xredis"
    "github.com/xiaoshicae/xone/v3/xserver"
)

func main() {
    // 启动 XOne（自动初始化 Redis）
    xserver.RunBlocking()
}

func example() {
    ctx := context.Background()

    // 单实例
    val, err := xredis.C().Get(ctx, "key").Result()

    // 多实例 - 指定名称
    val, err = xredis.C("cache").Get(ctx, "key").Result()

    // 写入
    err = xredis.C().Set(ctx, "key", "value", 0).Err()

    _ = val
    _ = err
}
```

## 连接池指标

默认启用（需配合 xmetric 模块），在 scrape 时实时读取 `redis.PoolStats()`，不额外占用协程：

| 指标 | 类型 | 说明 |
|------|------|------|
| `redis_pool_connections_total_current{name}` | Gauge | 连接池中的连接数（使用中 + 空闲） |
| `redis_pool_connections_idle{name}` | Gauge | 空闲的连接数 |
| `redis_pool_connections_stale_total{name}` | Counter | 因超时被移除的连接累计数 |
| `redis_pool_hits_total{name}` | Counter | 累计命中空闲连接的次数 |
| `redis_pool_misses_total{name}` | Counter | 累计未命中空闲连接的次数 |
| `redis_pool_timeouts_total{name}` | Counter | 累计等待连接超时的次数 |

`timeouts` 持续增长说明 `PoolSize` 不够或下游变慢，是最值得告警的一个。

关闭方式：

```yaml
XRedis:
  EnableMetric: false
```

## 注意事项

`MinIdleConns` 默认 **5**，这是框架加的默认值（go-redis 本身默认 0）。
多实例配置下每个实例都会常驻这么多连接 —— 配了 10 个 Redis 就是 50 条常驻连接，
实例多时按需调小。

### `C()` 找不到 client 时 panic

`C()` 取不到 client 就直接 panic，信息里带上你要的名字和实际配了哪些：

```
XOne xredis: no client found for name=[typo], configured=[primary replica]
```

之前是返回 `nil` 加一条 Error 日志。但 go-redis 的方法都是指针接收者，对 `nil` 调用必然
空指针解引用 —— 返回 `nil` 并不会让程序走得更远，只是把同一个 panic 推迟到调用方第一次
用它的时候，而那里的栈里只剩 `invalid memory address`，看不出根因是配置没配。
这是启动期的配置问题，不是运行期需要处理的错误。

可选依赖（配了就用、没配就跳过）用 `Has()` 先判断：

```go
if xredis.Has("cache") {
    xredis.C("cache").Set(ctx, k, v, 0)
}
```

注意 `C()` 仍然不要在启动完成前调用 —— 那时 client 尚未注册，会 panic。
