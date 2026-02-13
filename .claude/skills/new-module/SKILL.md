# 新增三方库模块

快速接入新的三方库到 XOne 框架。

使用方法: /new-module <模块名> <三方库名>

示例:
- /new-module xredis go-redis
- /new-module xmongo mongo-driver

$ARGUMENTS

---

请按照 XOne 框架规范创建新模块，包含以下文件：

## 1. 目录结构

```
x{模块名}/
├── config.go        # 配置结构和默认值
├── client.go        # 客户端封装和 API
├── x{模块名}_init.go # 初始化逻辑
├── x{模块名}_test.go # 单元测试
└── README.md        # 模块文档
```

## 2. 编码规范

### config.go
```go
package x{模块名}

const X{模块名}ConfigKey = "X{模块名}"

type Config struct {
    // 配置字段...
}

func configMergeDefault(c *Config) *Config {
    if c == nil {
        c = &Config{}
    }
    // 设置默认值，使用 xutil.GetOrDefault()
    return c
}
```

### client.go
```go
package x{模块名}

import "sync"

var (
    defaultClient *Client
    clientMu      sync.RWMutex
)

// C 获取客户端
func C() *Client {
    clientMu.RLock()
    defer clientMu.RUnlock()
    return defaultClient
}

// 注意：CWithCtx 仅在三方库 API 要求通过 context 传递 client 时才需要提供
// 例如 xhttp 的 resty 需要 R().SetContext(ctx) 来传递 trace 信息
// 如果三方库的每个操作方法本身已接受 ctx 参数（如 go-redis），则无需 CWithCtx
```

### x{模块名}_init.go
```go
package x{模块名}

import (
    "github.com/xiaoshicae/xone/v2/xconfig"
    "github.com/xiaoshicae/xone/v2/xerror"
    "github.com/xiaoshicae/xone/v2/xhook"
    "github.com/xiaoshicae/xone/v2/xutil"
)

func init() {
    xhook.BeforeStart(initX{模块名})
    xhook.BeforeStop(closeX{模块名})
}

func initX{模块名}() error {
    if !xconfig.ContainKey(X{模块名}ConfigKey) {
        return nil
    }
    // 初始化逻辑...
    // 错误使用 xerror: xerror.Newf("x{模块名}", "init", "xxx failed, err=[%v]", err)
    // 内部日志: xutil.InfoIfEnableDebug("XOne initX{模块名} ...")
    return nil
}

func closeX{模块名}() error {
    // 清理逻辑...
    return nil
}
```

## 3. 注意事项

- 时间配置使用 `xutil.ToDuration()`，支持 "d" 格式
- 全局变量使用 `sync.RWMutex` 保护
- 错误优先使用 `xerror.XOneError`，禁止直接使用 `fmt.Errorf`
  - 创建错误：`xerror.New(module, op, err)` 或 `xerror.Newf(module, op, format, args...)`
- 内部 debug 日志使用 `xutil.InfoIfEnableDebug()` / `xutil.WarnIfEnableDebug()` / `xutil.ErrorIfEnableDebug()`
- 正式日志记录使用 `xlog.Info(ctx, ...)` / `xlog.Warn(ctx, ...)` / `xlog.Error(ctx, ...)`，必须传递 context
- 如需链路追踪，检查 `xtrace.EnableTrace()`
- import 路径使用 v2：`github.com/xiaoshicae/xone/v2/x{模块名}`
- 测试使用 mockey + goconvey，需要 `-gcflags="all=-N -l"`
- 配置默认值需要考虑生产级别的最佳实践：
  - 如果三方库对零值有合理的默认处理（如 go-redis 的 PoolSize=0 → 10*GOMAXPROCS），应优先让三方库自身处理，不要在 `configMergeDefault` 中硬编码一个可能偏小的值
  - 超时类默认值应兼顾安全性和性能（如建连超时不宜过长，读写超时需考虑复杂操作场景）
  - 连接池相关配置（MinIdleConns、PoolTimeout 等）应有合理预设，避免冷启动延迟和资源耗尽
  - 重试相关配置应保留三方库的禁用语义（如 -1 表示禁用），不要强制覆盖为默认值

## 4. README.md 模板

```markdown
## X{模块名}模块

### 1. 模块简介
- 基于 [{三方库}](链接) 封装

### 2. 配置参数
\`\`\`yaml
X{模块名}:
  # 配置项...
\`\`\`

### 3. API 接口
\`\`\`go
// API 说明...
\`\`\`

### 4. 使用示例
\`\`\`go
// 示例代码...
\`\`\`
```

请根据以上模板创建模块代码。