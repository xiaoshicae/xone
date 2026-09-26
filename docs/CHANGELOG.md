# 更新日志

这里记录每个版本里**使用者看得见的变化**：新功能、行为变化、不兼容变更、修复、性能。
仓库内部的重构和工具改动不写，除非它改变了使用者要做的事。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循
[语义化版本](https://semver.org/lang/zh-CN/)。所有模块共用同一个版本号（见 `scripts/release.sh`）。

## [未发布]

### 修复

- 只用核心（`xone.Func`、`xone.UntilSignal`，一个集成都没 import）的程序，写了 `XLog` 块会启动失败
  （`config keys [XLog] are not read by anyone`），不写则日志停在标准库的默认输出。现在 `xlog` 跟着框架一起来，
  和 `xapp` 一样不用另外 import；原来为此写的 `_ "github.com/xiaoshicae/xone/xlog"` 可以删掉，留着也无妨。

## [v1.11.0] - 2026-09-26

### 不兼容变更

- xgin 访问日志默认不再记请求头（字段 `request_headers`），和查询串、body、响应头一样由开关控制。
  迁移：要保留原来的输出，配 `XGin.LogRequestHeaders: true`；直接用 `middleware.Log` 的，加 `middleware.WithHeaders(true, false)`。

### 新增

- xgin 访问日志可以记查询串和响应头：`XGin.LogQuery: true` 加字段 `query`（逐字段脱敏），
  `XGin.LogResponseHeaders: true` 加字段 `response_headers`（`Set-Cookie` 等凭证类脱敏）。默认都关。
- xgin 访问日志默认多记五个字段：`host`、`proto`、`user_agent`、`bytes_in`（请求的 `Content-Length`，分块上传是 `-1`）、
  `bytes_out`（响应体字节数）。实测每个请求多约 1µs，不多分配。
  词表里没有的敏感参数名（比如 OAuth 的 `code`）用 `middleware.AddSensitiveFields` 补上。

### 修复

- 日志里的耗时带上单位：xgin 访问日志、xgorm 的 SQL 日志、xflow 的流程 / 步骤日志的 `elapsed` 改名为 `elapsed_ms`，
  值是毫秒（保留到微秒，如 `0.051`）；xgorm 慢查询的 `threshold` 改名为 `threshold_ms`。原来 JSON 里是没有单位的纳秒整数。
  迁移：日志平台里按 `elapsed` / `threshold` 查询、告警、做看板的，换成新字段名，阈值按毫秒写。
- xgin 访问日志里没匹配上路由的请求，`route` 记成 `unmatched`，和指标、Span 一致；原来填的是请求路径，分不出是路由还是 404。

## [v1.10.0] - 2026-09-26

首个版本。所有模块同时发布、共用这个版本号。
版本号从 v1.10.0 起步：这个模块路径上更早的 v0.x / v1.0.0～v1.3.1 是另一份代码，已在 go.mod 里撤回（`retract`）。

- **核心** `github.com/xiaoshicae/xone`：
  - `xone.Run` / `MustRun`：按档位启动、逆序关闭，整个退出流程共用一份停止预算；`xone.Func` / `xone.UntilSignal`。
  - `xconfig`（懒加载、严格解码、`${VAR}` 占位符、`XApp.Profiles` 多环境、`XApp.Import` 拆文件）、`xhook`、`xerror`、
    `xlog`（`AddKV` / `CtxWithKV` 请求级字段）、`xapp`、`xflow`、`xtls`、`xutil`、`xonetest`。
  - `XONE_DEBUG=1`：启动时打出用了哪个配置文件、激活了哪些 profile、按优先级读了哪些文件、合并之后的完整配置
    （凭证遮成 `***`）和启动钩子的顺序。
  - 启动 banner：只在 stderr 是终端时打，带版本号。
- **集成**：`xgin`（默认只信私有网段的代理）、`xginswagger`、`xgorm`（MySQL / PostgreSQL）、`xgorm/clickhouse`、
  `xredis`、`xcache`（`Get[V]` 按类型取值）、`xhttp`、`xtrace`、`xmetric`。
