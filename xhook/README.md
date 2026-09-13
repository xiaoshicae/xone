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

### 注册顺序由 Go 的 init 顺序决定，而不是 import 的书写顺序

各模块在 `init()` 中注册 Hook，所以注册顺序就是包的初始化顺序。Go 的规则是
**拓扑排序（被依赖的包先初始化）+ 就绪集合内按 import path 字典序**，
与 import 在源码中的书写顺序**无关** —— `gofmt` 本来也会把同一组内的 import 重排。

```
写 pc, pb, pa（互不依赖）                → 实际 pa, pb, pc          字典序
写 zebra, middle, alpha（alpha 依赖 zebra）→ 实际 middle, zebra, alpha 依赖优先
目录 aaa/包名 zpkg、目录 zzz/包名 apkg     → 实际 aaa 先             比的是路径不是包名
```

因此**不要试图靠调整 import 顺序来控制生命周期顺序**。xone 各模块之间顺序正确，
靠的是真实的依赖边：每个模块都 import xconfig，xgorm / xhttp / xtrace 都 import xlog，
于是 xconfig、xlog 必然先初始化。

### BeforeStart 正序、BeforeStop 反序

BeforeStart 按 Order 升序执行，同 Order 内按注册顺序；BeforeStop 是它的整体镜像，
确保**后初始化的模块先关闭**，符合资源管理的 LIFO 原则——先申请的资源最后释放。

```
启动：xconfig → xlog → xtrace → xhttp → xgorm
关闭：xgorm → xhttp → xtrace → xlog → xconfig
```

xgorm 依赖 xlog 记录日志、依赖 xtrace 上报链路，因此关闭时先关 xgorm，
最后关 xconfig（配置在整个生命周期中都需要可用）。

### 需要确定性保证时用 Order，而不是 import 顺序

用户自己的包若在 `init()` 里注册 Hook，它相对 xone 各模块的位置取决于**模块路径的字典序**
——模块名叫 `acme/...` 就排在 `github.com/xiaoshicae/xone/...` 之前，叫 `myapp/...` 就排在之后，
用户无法通过调整 import 来改变。框架因此对两个必须保证位置的模块显式声明 Order：

| 模块 | Order | 含义 |
|------|-------|------|
| xconfig | 1 | 最先启动（配置必须先于一切就绪）；它没有关闭钩子 |
| xlog | 10 | 次先启动、最后关闭，使任何模块在关闭阶段打的日志仍能落盘 |
| 其余模块 / 用户资源 | 100（默认） | 相互之间按 init 顺序，关闭时逆序 |

用户资源保持默认 Order 即可：它一定在 xlog 之前关闭，所以关闭逻辑里可以放心打日志。

## 配置选项

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `Order(n)` | 100 | 资源层级，值越小越底层：启动越早、关闭越晚。**不推荐使用**，详见下文 |
| `MustInvokeSuccess(b)` | true | 失败时是否中断流程（仅 BeforeStart 有效） |
| `Timeout(d)` | 10s | 单个 Hook 超时时间 |

### 关于 Order

`Order` 表示 Hook 所处的**资源层级**，不是阶段内的执行优先级：
**值越小越底层 —— BeforeStart 越先执行，BeforeStop 越后执行**。
相同 Order 的 Hook：BeforeStart 按注册顺序正序，BeforeStop 按注册顺序逆序。

```
注册：数据库(Order=100)  缓存(Order=100)  日志(Order=10)
启动：日志 → 数据库 → 缓存
关闭：缓存 → 数据库 → 日志      # 整体镜像：低 Order 最后关闭
```

这样一个资源只需声明**一个** Order 就能做到"先启动、后关闭"。
若两个阶段都按 Order 升序执行，同一个资源就得在启停两处各填一个方向相反的值，
启停对称性也就丢了。

**普通模块与业务资源应保持默认值（100）**，只有"必须在某一侧到底"的基础设施才声明 Order。原因：

1. Order 值分散在各模块中，声明得越多越难全局把控
2. 多模块各自声明容易冲突
3. 全部默认时行为就是"启动按 init 顺序、关闭按其逆序"，绝大多数资源要的正是这个

框架内部只有 `xconfig`（1）和 `xlog`（10）声明了 Order，且两者都不是可选的：
Go 的 init 顺序按 import path 字典序排，用户模块叫 `acme/...` 还是 `myapp/...`
就决定了它排在 xone 之前还是之后——这不是使用者能通过 import 纪律控制的。
没有 Order(1)，路径靠前的用户包会在配置加载完成前就执行 BeforeStart；
没有 Order(10)，这样的用户包会在日志写入器关闭之后才关闭，其关闭日志直接丢失。

### 关于重复注册

**不做去重**。Hook 多为闭包，而同一函数字面量产生的所有闭包共享同一代码指针，
按指针去重会把循环或工厂中注册的不同闭包误判为重复而丢弃：

```go
for _, name := range dbNames {
    xhook.BeforeStop(func() error { return closeDB(name) })  // 三个不同闭包，需全部生效
}
```

同一函数重复注册会执行多次，由调用方自行保证。

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

- **Panic 捕获**：所有 Hook 执行均有 recover 保护，panic 转为错误返回
- **并发安全**：全局状态受 `sync.RWMutex` 保护
- **数量限制**：单类型最多 1000 个 Hook
