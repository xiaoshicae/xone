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

// CWithCtx 获取带 context 的客户端（推荐）
func CWithCtx(ctx context.Context) *Client {
    // ...
}
```

### x{模块名}_init.go
```go
package x{模块名}

import (
    "github.com/xiaoshicae/xone/xconfig"
    "github.com/xiaoshicae/xone/xhook"
    "github.com/xiaoshicae/xone/xutil"
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
- 错误格式: `fmt.Errorf("XOne xxx failed, err=[%v]", err)`
- 日志使用 `xutil.InfoIfEnableDebug()` 等函数
- 如需链路追踪，检查 `xtrace.EnableTrace()`
- 测试使用 mockey + goconvey，需要 `-gcflags="all=-N -l"`

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