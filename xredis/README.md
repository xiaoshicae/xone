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

    "github.com/xiaoshicae/xone/v2/xredis"
    "github.com/xiaoshicae/xone/v2/xserver"
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
