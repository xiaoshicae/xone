# XGorm

XGorm 是 XOne 框架的数据库模块，基于 [GORM](https://gorm.io/) 封装，支持 MySQL 和 PostgreSQL，提供连接池管理、链路追踪、慢查询日志等功能。

## 功能特性

- 支持 MySQL 和 PostgreSQL 数据库
- 支持单数据库和多数据库配置
- 自动连接池管理
- 集成 OpenTelemetry 链路追踪
- 慢查询日志记录
- 自动重试连接

## 配置说明

### 单数据库配置

```yaml
XGorm:
  Driver: "mysql"                    # 数据库驱动: mysql, postgres (默认: postgres)
  DSN: "user:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True"
  DialTimeout: "500ms"               # 建连超时 (默认: 500ms)
  ReadTimeout: "3s"                  # 读超时，仅 MySQL 有效 (默认: 3s)
  WriteTimeout: "5s"                 # 写超时，仅 MySQL 有效 (默认: 5s)
  MaxOpenConns: 50                   # 最大连接数 (默认: 50)
  MaxIdleConns: 50                   # 最大空闲连接数 (默认: 等于 MaxOpenConns)
  MaxLifetime: "5m"                  # 连接最长存活时间 (默认: 5m)
  MaxIdleTime: "5m"                  # 空闲连接最长存活时间 (默认: 等于 MaxLifetime)
  EnableLog: true                    # 是否开启日志 (默认: false)
  SlowThreshold: "3s"                # 慢查询阈值 (默认: 3s)
  IgnoreRecordNotFoundErrorLog: true # 是否忽略记录未找到的错误日志 (默认: false)
```

### 多数据库配置

```yaml
XGorm:
  - Name: "master"                   # 必填，用于区分不同数据库
    Driver: "mysql"
    DSN: "user:pass@tcp(127.0.0.1:3306)/master_db"
    MaxOpenConns: 100
  - Name: "slave"
    Driver: "mysql"
    DSN: "user:pass@tcp(127.0.0.1:3307)/slave_db"
    MaxOpenConns: 50
```

### DSN 格式

**MySQL:**
```
user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
```

**PostgreSQL:**
```
host=localhost user=postgres password=pass dbname=mydb port=5432 sslmode=disable
```

## 使用方式

### 获取默认客户端

```go
import "github.com/xiaoshicae/xone/xgorm"

// 获取默认客户端（单数据库模式或多数据库模式的第一个）
db := xgorm.C()

// 查询示例
var user User
db.First(&user, 1)
```

### 获取指定客户端（多数据库模式）

```go
// 通过名称获取指定数据库客户端
masterDB := xgorm.C("master")
slaveDB := xgorm.C("slave")
```

### 带 Context 的客户端（推荐）

使用 `CWithCtx` 可以确保 context 中的链路追踪信息传递到数据库操作中：

```go
func GetUser(ctx context.Context, id uint) (*User, error) {
    var user User
    err := xgorm.CWithCtx(ctx).First(&user, id).Error
    return &user, err
}
```

## 配置项说明

| 配置项 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| Driver | string | 否 | postgres | 数据库驱动类型: mysql, postgres |
| DSN | string | 是 | - | 数据库连接字符串 |
| Name | string | 多数据库时必填 | - | 数据库标识名称 |
| DialTimeout | string | 否 | 500ms | 建立连接超时时间 |
| ReadTimeout | string | 否 | 3s | 读取超时时间（仅 MySQL） |
| WriteTimeout | string | 否 | 5s | 写入超时时间（仅 MySQL） |
| MaxOpenConns | int | 否 | 50 | 最大打开连接数 |
| MaxIdleConns | int | 否 | MaxOpenConns | 最大空闲连接数 |
| MaxLifetime | string | 否 | 5m | 连接最大存活时间 |
| MaxIdleTime | string | 否 | MaxLifetime | 空闲连接最大存活时间 |
| EnableLog | bool | 否 | false | 是否启用 SQL 日志 |
| SlowThreshold | string | 否 | 3s | 慢查询日志阈值 |
| IgnoreRecordNotFoundErrorLog | bool | 否 | false | 是否忽略记录未找到错误的日志 |

## 链路追踪

当 `XTrace.Enable` 为 `true` 时，xgorm 会自动集成 OpenTelemetry 链路追踪，所有数据库操作都会被记录到追踪链路中。

确保使用 `CWithCtx(ctx)` 以正确传递追踪上下文。
