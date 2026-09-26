# 更新日志

这里记录每个版本里**使用者看得见的变化**：新功能、行为变化、不兼容变更、修复、性能。
仓库内部的重构和工具改动不写，除非它改变了使用者要做的事。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循
[语义化版本](https://semver.org/lang/zh-CN/)。所有模块共用同一个版本号（见 `scripts/release.sh`）。

## [未发布]

## [v0.1.0] - 2026-09-26

首个版本。所有模块同时发布、共用这个版本号。

- **核心** `github.com/xiaoshicae/xone`：
  - `xone.Run` / `MustRun`：按档位启动、逆序关闭，整个退出流程共用一份停止预算；`xone.Func` / `xone.UntilSignal`。
  - `xconfig`（懒加载、严格解码、`${VAR}` 占位符、`XApp.Profiles` 多环境、`XApp.Import` 拆文件）、`xhook`、`xerror`、
    `xlog`（`AddKV` / `CtxWithKV` 请求级字段）、`xapp`、`xflow`、`xtls`、`xutil`、`xonetest`。
  - `XONE_DEBUG=1`：启动时打出用了哪个配置文件、激活了哪些 profile、按优先级读了哪些文件、合并之后的完整配置
    （凭证遮成 `***`）和启动钩子的顺序。
  - 启动 banner：只在 stderr 是终端时打，带版本号。
- **集成**：`xgin`（默认只信私有网段的代理）、`xginswagger`、`xgorm`（MySQL / PostgreSQL）、`xgorm/clickhouse`、
  `xredis`、`xcache`（`Get[V]` 按类型取值）、`xhttp`、`xtrace`、`xmetric`。
