# xserver

服务运行与生命周期管理，负责按顺序拉起各模块、阻塞等待退出信号、优雅关闭。

## Server 接口

```go
type Server interface {
    Run() error  // 建议以阻塞方式运行
    Stop() error // 资源清理逻辑放这里
}
```

## 三种启动方式

```go
xserver.Run(server)   // 启动 Server，阻塞等待退出信号
xserver.RunBlocking() // 无 Server 的常驻服务（consumer / job），阻塞等待退出信号
xserver.Init()           // 只执行 BeforeStart hook，调试用
```

`Init()` 不执行 BeforeStop hook —— 各模块初始化后保持可用，调用方可以继续用
`xgorm.C()`、`xhttp.C()`。代价是日志写入器不会 flush，要确保日志落盘请用 `Run`。

## 生命周期

```
InvokeBeforeStartHook()          xconfig → xlog → xtrace → xhttp → xgorm
        │
        ├─ 失败 ─→ InvokeBeforeStopHook()  回滚已初始化的模块，返回 error
        │
        ↓ 成功
Server.Run()  （在独立 goroutine 中阻塞运行）
        │
        ├─ 收到 SIGHUP/SIGINT/SIGTERM ─→ Server.Stop() ─→ 等待 Run 退出
        └─ Run 自行返回（正常退出 / 监听失败 / panic）─→ Server.Stop()
        │
        ↓
InvokeBeforeStopHook()           xgorm → xhttp → xtrace → xlog → xconfig
```

三点需要留意：

**启动失败会回滚。** hook 正序执行，某个模块初始化失败时，它之前的模块都已就绪。
不回滚意味着日志写入器不 flush、trace provider 不 shutdown、连接池不关闭 ——
而「启动为什么失败」这条日志恰恰最需要落盘。

**Run 自行返回时同样调用 Stop。** 端口被占用、Run 主动退出、Run panic ——
这几条路径上 `Server.Stop()` 里的清理逻辑照样执行。`Stop` 保证只执行一次，
退出信号与 Run 返回同时发生时不会重复调用。

**Stop 之后会等 Run goroutine 退出**，默认 30s，避免 goroutine 泄漏。
超时只打一条 warn，不阻止进程退出：

```go
xserver.SetWaitRunExitTimeout(10 * time.Second) // 线程安全，<= 0 时忽略
```

## 退出信号

`SIGHUP`、`SIGINT`、`SIGTERM`。`SIGKILL` 无法捕获，进程直接终止，
不会执行任何 Stop 与 BeforeStop hook。

## 与 xgin 配合

`*xgin.XGin` 实现了 `Server` 接口：

```go
gx := xgin.New().WithRouteRegister(register).Build()
xserver.Run(gx)   // 等价于 gx.Start()
```

优雅退出时间由 `XGin.GracefulStopTimeout` 控制（默认 25s），
它应当小于部署环境的进程终止宽限期（如 K8s `terminationGracePeriodSeconds`，默认 30s），
否则 Shutdown 还没走完 pod 就被 SIGKILL，等于没有优雅退出。
