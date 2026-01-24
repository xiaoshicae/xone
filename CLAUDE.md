# XOne 项目说明

## 项目概述

XOne 是一个 Go 语言微服务框架，提供配置管理、日志、HTTP 客户端、数据库、链路追踪等开箱即用的功能模块。

## 项目结构

```
xone/
├── xconfig/     # 配置管理模块 (基于 Viper)
├── xlog/        # 日志模块 (基于 Logrus)
├── xhttp/       # HTTP 客户端模块 (基于 Resty)
├── xgorm/       # 数据库模块 (基于 GORM，支持 MySQL/PostgreSQL)
├── xtrace/      # 链路追踪模块 (基于 OpenTelemetry)
├── xhook/       # 生命周期钩子模块
├── xutil/       # 工具函数模块
└── test/        # 集成测试
```

---

## 项目约束 (Rules)

### 1. 模块规范

- 每个模块必须包含: `config.go`, `client.go`, `x{模块名}_init.go`, `x{模块名}_test.go`, `README.md`
- 模块配置 key 统一为 `X{模块名}ConfigKey = "X{模块名}"`
- 模块初始化通过 `xhook.BeforeStart()` 注册，清理通过 `xhook.BeforeStop()` 注册
- 导出的 API 使用大写开头，内部函数使用小写开头

### 2. 并发安全

- 全局变量必须使用 `sync.RWMutex` 保护读写
- 一次性初始化使用 `sync.Once`
- 关闭/停止操作使用 `sync.Once` 防止重复调用
- map 类型全局变量必须加锁保护

### 3. 错误处理

- 错误信息格式: `fmt.Errorf("XOne xxx failed, err=[%v]", err)`
- 不要忽略错误，必须处理或向上传递
- 使用 `errors.Is()` 和 `errors.As()` 进行错误判断
- 关键操作失败时记录日志: `xutil.ErrorIfEnableDebug()`

### 4. 配置处理

- 时间配置统一使用 `xutil.ToDuration()`，支持 "d"（天）格式
- 配置默认值使用 `xutil.GetOrDefault()` 函数
- 配置合并使用 `configMergeDefault()` 模式
- 不要使用 `cast.ToDuration()`，统一用 `xutil.ToDuration()`

### 5. 日志规范

- 框架内部日志使用 `xutil.InfoIfEnableDebug()` / `xutil.WarnIfEnableDebug()` / `xutil.ErrorIfEnableDebug()`
- 业务日志使用 `xlog.Info(ctx, ...)` 等，必须传递 context
- 日志格式: `"描述信息, key1=[%v], key2=[%v]"`

### 6. Context 传递

- 对外 API 优先提供 `XxxWithCtx(ctx context.Context, ...)` 版本
- Context 用于传递链路追踪信息、超时控制等
- 不要在 context 中存储业务数据

---

## Go 最佳实践

### 1. 代码风格

```go
// Good: 使用有意义的变量名
func getUserByID(userID int64) (*User, error)

// Bad: 使用无意义的缩写
func getU(id int64) (*User, error)
```

### 2. 错误处理

```go
// Good: 包装错误信息
if err != nil {
    return fmt.Errorf("query user failed, userID=[%d], err=[%v]", userID, err)
}

// Bad: 直接返回错误
if err != nil {
    return err
}
```

### 3. 并发安全

```go
// Good: 使用 RWMutex 保护
var (
    cache   = make(map[string]string)
    cacheMu sync.RWMutex
)

func Get(key string) string {
    cacheMu.RLock()
    defer cacheMu.RUnlock()
    return cache[key]
}

func Set(key, value string) {
    cacheMu.Lock()
    defer cacheMu.Unlock()
    cache[key] = value
}
```

### 4. 资源管理

```go
// Good: 使用 defer 确保资源释放
func readFile(path string) ([]byte, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    return io.ReadAll(f)
}
```

### 5. 接口设计

```go
// Good: 小接口，单一职责
type Reader interface {
    Read(p []byte) (n int, err error)
}

// Bad: 大接口，职责不清
type FileHandler interface {
    Read() []byte
    Write(data []byte)
    Delete()
    Copy()
    Move()
}
```

### 6. 常量优于魔法值

```go
// Good: 使用常量
const (
    defaultTimeout = 30 * time.Second
    maxRetryCount  = 3
)

// Bad: 魔法数字
time.Sleep(30 * time.Second)
```

### 7. 预分配切片容量

```go
// Good: 预分配容量
users := make([]User, 0, len(ids))
for _, id := range ids {
    users = append(users, getUser(id))
}

// Bad: 不预分配
var users []User
for _, id := range ids {
    users = append(users, getUser(id))
}
```

### 8. 避免不必要的内存分配

```go
// Good: 复用 buffer
var buf bytes.Buffer
buf.WriteString("hello")
buf.WriteString(" ")
buf.WriteString("world")
result := buf.String()

// Bad: 字符串拼接
result := "hello" + " " + "world"
```

---

## 代码通用规范

### 1. 命名规范

| 类型 | 规范 | 示例 |
|------|------|------|
| 包名 | 小写单词，不用下划线 | `xconfig`, `xhttp` |
| 文件名 | 小写，下划线分隔 | `xconfig_init.go` |
| 接口 | 动词/er 结尾 | `Reader`, `Closer` |
| 结构体 | 驼峰命名 | `HttpClient`, `Config` |
| 常量 | 驼峰或全大写 | `MaxRetry`, `DEFAULT_TIMEOUT` |
| 私有变量 | 小写开头驼峰 | `defaultClient` |
| 公开变量 | 大写开头驼峰 | `DefaultTimeout` |

### 2. 注释规范

```go
// Package xhttp 提供 HTTP 客户端封装
// 基于 go-resty，支持链路追踪和重试机制
package xhttp

// Config HTTP 客户端配置
type Config struct {
    // Timeout 请求超时时间，支持 "d" 格式，如 "1d12h"
    Timeout string `yaml:"Timeout"`
}

// C 获取 HTTP 客户端
// 推荐使用 RWithCtx() 以确保链路追踪信息传递
func C() *resty.Client {
    // ...
}
```

### 3. 文件组织

```go
// 1. package 声明
package xhttp

// 2. import（标准库、第三方库、本地包分组）
import (
    "context"
    "net/http"

    "github.com/go-resty/resty/v2"

    "github.com/xiaoshicae/xone/xutil"
)

// 3. 常量
const (
    DefaultTimeout = "60s"
)

// 4. 变量
var (
    defaultClient *resty.Client
    clientMu      sync.RWMutex
)

// 5. init 函数
func init() {
    // ...
}

// 6. 公开函数/方法
func C() *resty.Client {
    // ...
}

// 7. 私有函数/方法
func setDefaultClient(client *resty.Client) {
    // ...
}
```

### 4. 测试规范

```go
// 测试函数命名: Test{函数名}_{场景}
func TestGetUser_NotFound(t *testing.T) {
    // Arrange: 准备测试数据
    // Act: 执行被测函数
    // Assert: 验证结果
}

// 使用 mockey + goconvey
func TestXHttpConfig(t *testing.T) {
    mockey.PatchConvey("TestXHttpConfig-Default", t, func() {
        config := configMergeDefault(nil)
        c.So(config.Timeout, c.ShouldEqual, "60s")
    })
}
```

### 5. Git 提交规范

```
<type>: <简短描述>

<详细说明（可选）>
```

类型:
- `feat`: 新功能
- `fix`: Bug 修复
- `docs`: 文档更新
- `refactor`: 代码重构（不改变功能）
- `perf`: 性能优化
- `test`: 测试相关
- `chore`: 构建/工具/依赖更新

---

## 常用命令

```bash
# 运行所有测试（需要 -gcflags 禁用内联以支持 Mockey）
go test -gcflags="all=-N -l" ./...

# 运行单个模块测试
go test -gcflags="all=-N -l" ./xhttp/... -v

# 构建
go build ./...

# 格式化代码
gofmt -w .

# 检查代码问题
go vet ./...
```

## 测试框架

- 使用 [bytedance/mockey](https://github.com/bytedance/mockey) 进行 mock
- 使用 [smartystreets/goconvey](https://github.com/smartystreets/goconvey) 进行断言
- 测试时必须添加 `-gcflags="all=-N -l"` 参数

## 模块配置示例

```yaml
# application.yml
XLog:
  Level: "info"
  Console: true

XHttp:
  Timeout: "60s"
  RetryCount: 3

XGorm:
  Driver: "postgres"
  DSN: "host=localhost user=test dbname=test"

XTrace:
  Enable: true
  Console: false
```

## 注意事项

- 默认数据库驱动是 PostgreSQL
- 修改代码后确保运行测试验证
- 新增 API 需要更新对应模块的 README.md
- 导出的常量/函数需要添加注释说明
