# xhook - 生命周期钩子

xhook 提供 `BeforeStart` 和 `BeforeStop` 两类生命周期钩子，用于管理模块的初始化和清理。

## 快速开始

```go
import "github.com/xiaoshicae/xone/v2/xhook"

func init() {
    xhook.BeforeStart(initMyModule)
    xhook.BeforeStop(closeMyModule)
}
```

## 执行顺序

### BeforeStart：按注册顺序正序执行

各模块通过 `init()` 注册 Hook，Go 的 import 机制保证 `init()` 按包导入顺序依次执行。对于相同 Order 值的 Hook，使用稳定排序保持注册时的相对顺序。

```
注册顺序：xconfig → xtrace → xlog → xhttp → xgorm
执行顺序：xconfig → xtrace → xlog → xhttp → xgorm（正序）
```

### BeforeStop：按注册顺序反序执行

BeforeStop 在执行时自动反转顺序，确保**后初始化的模块先关闭**，与 BeforeStart 形成对称。这符合资源管理的 LIFO（后进先出）原则——先申请的资源最后释放。

```
注册顺序：xconfig → xtrace → xlog → xhttp → xgorm
执行顺序：xgorm → xhttp → xlog → xtrace → xconfig（反序）
```

以上述顺序为例，xgorm 依赖 xlog 记录日志、依赖 xtrace 上报链路，因此关闭时应先关 xgorm，最后关 xconfig（配置在整个生命周期中都需要可用）。

### 用户侧控制顺序

在 `main.go` 中通过 import 顺序控制各模块的生命周期顺序：

```go
import (
    _ "github.com/xiaoshicae/xone/v2/xconfig" // 最先启动，最后关闭
    _ "github.com/xiaoshicae/xone/v2/xtrace"
    _ "github.com/xiaoshicae/xone/v2/xlog"
    _ "github.com/xiaoshicae/xone/v2/xhttp"
    _ "github.com/xiaoshicae/xone/v2/xgorm"   // 最后启动，最先关闭
)
```

## 配置选项

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `Order(n)` | 100 | 执行优先级，值越小越先执行。**不推荐使用**，详见下文 |
| `MustInvokeSuccess(b)` | true | 失败时是否中断流程（仅 BeforeStart 有效） |
| `Timeout(d)` | 10s | 单个 Hook 超时时间 |

### 关于 Order

**不推荐普通模块使用 Order**，应保持默认值（100），依靠 import 顺序控制执行顺序。原因：

1. import 顺序更直观，Order 值分散在各模块中难以全局把控
2. 多模块各自声明 Order 容易冲突
3. 框架内部已有保障（xconfig 使用 `Order(1)` 确保最先初始化）

## BeforeStart 错误处理

- `MustInvokeSuccess=true`（默认）：Hook 失败时立即返回错误，中断启动流程
- `MustInvokeSuccess=false`：Hook 失败时记录警告，继续执行后续 Hook

## BeforeStop 错误处理

- 单个 Hook 失败不中断关闭流程，继续执行其余 Hook
- 所有错误被收集合并返回
- 支持全局超时（默认 60s）和个体超时（默认 10s），取两者较小值

```go
// 自定义全局关闭超时
xhook.SetStopTimeout(30 * time.Second)
```

## 安全特性

- **去重检测**：同一函数重复注册会被自动跳过
- **Panic 捕获**：所有 Hook 执行均有 recover 保护，panic 转为错误返回
- **并发安全**：全局状态受 `sync.RWMutex` 保护
- **数量限制**：单类型最多 1000 个 Hook
