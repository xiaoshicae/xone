# Go 编码规范

## 代码风格

- 遵循 `gofmt` 默认规则
- 所有代码必须通过 `go vet ./...`
- import 分三组：标准库、第三方库、本地包

## 命名规范

| 类型 | 规范 | 示例 |
|------|------|------|
| 包名 | 小写单词，不用下划线 | `xconfig`, `xhttp` |
| 文件名 | 小写，下划线分隔 | `xconfig_init.go` |
| 接口 | 动词/er 结尾 | `Reader`, `Closer` |
| 结构体 | 驼峰命名 | `HttpClient`, `Config` |
| 常量 | 驼峰 | `MaxRetry`, `DefaultTimeout` |
| 私有变量 | 小写开头驼峰 | `defaultClient` |
| 公开变量 | 大写开头驼峰 | `DefaultTimeout` |

## 并发安全

- 全局变量必须使用 `sync.RWMutex` 保护读写
- 一次性初始化使用 `sync.Once`
- 关闭/停止操作使用 `sync.Once` 防止重复调用
- map 类型全局变量必须加锁保护

## 配置处理

- 时间配置统一使用 `xutil.ToDuration()`，支持 "d"（天）格式
- 配置默认值使用 `xutil.GetOrDefault()` 函数
- 配置合并使用 `configMergeDefault()` 模式
- 禁止使用 `cast.ToDuration()`

## 日志规范

- 框架内部日志使用 `xutil.InfoIfEnableDebug()` / `xutil.WarnIfEnableDebug()` / `xutil.ErrorIfEnableDebug()`
- 业务日志使用 `xlog.Info(ctx, ...)` 等，必须传递 context
- 日志格式：`"描述信息, key1=[%v], key2=[%v]"`

## Context 传递

- 对外 API 优先提供 `XxxWithCtx(ctx context.Context, ...)` 版本
- Context 用于传递链路追踪信息、超时控制等
- 不要在 context 中存储业务数据

## 文件内代码排列

1. package 声明
2. import（标准库、第三方库、本地包分组）
3. 常量
4. 变量
5. init 函数
6. 公开函数/方法
7. 私有函数/方法

## 注释规范

- 公开 API 必须有注释（Go doc 格式）
- 包级注释用 `// Package xxx ...`
- 注释使用中文

## 错误处理

- 优先使用 `xerror.XOneError` 统一错误类型，禁止直接使用 `fmt.Errorf`
- 创建错误：`xerror.New(module, op, err)` 或 `xerror.Newf(module, op, format, args...)`
- 判断模块错误：`xerror.Is(err, "xconfig")`
- 提取模块名：`xerror.Module(err)`

## 函数签名

- 公开函数除 `ctx context.Context` 外，参数数量不超过 3 个
- 参数过多时使用结构体封装（如 Option / Event / Request）

## 编码要点

- 使用有意义的变量名，避免无意义缩写
- 错误必须包装上下文信息：`xerror.Newf("xgorm", "query", "query user failed, userID=[%d], err=[%v]", userID, err)`
- 使用 defer 确保资源释放
- 小接口，单一职责
- 常量优于魔法值
- 预分配切片容量：`make([]T, 0, len(source))`
- 单文件不超过 500 行（测试文件 `*_test.go` 不受此限制）