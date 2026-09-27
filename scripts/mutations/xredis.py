# xredis 的变异：module xredis 里的承诺。写法见 scripts/mutations/__init__.py
from . import cut, mutate, section, swap

section("配置")
mutate("xredis 多实例铺的是自己的默认值", "xredis/config.go", "./xredis", "TestConfig_SingleInstanceForm",
       swap('xconfig.UnmarshalClients(ConfigKey, DefaultClientConfig)', 'xconfig.UnmarshalClients(ConfigKey, func() ClientConfig { return ClientConfig{} })'))

section("启动与退出")
mutate("建实例 panic 不漏掉已建好的", "internal/xclient/xclient.go", "./xredis", "TestInitAll",
       swap('safeNew(ctx, r.module, name, cfgs[name], new)', 'new(ctx, name, cfgs[name])'))
mutate("一个实例建不起来就把已建好的全关掉", "internal/xclient/xclient.go", "./xredis", "TestInitAll",
       swap('\t\t\tcloseAll(r.module, closers)\n\t\t\treturn err\n','\t\t\treturn err\n'))

section("客户端")
mutate("xredis 没配也让注册表知道启动过了", "xredis/xredis.go", "./xredis", "TestInitXRedis_CSaysUnconfiguredNotTooEarly",
       swap('\t\treturn xclient.Build(ctx, reg, nil, build)\n', '\t\treturn nil\n'))
mutate("Redis 命令遵守请求 deadline", "xredis/xredis.go", "./xredis", "TestNew",
       swap('\t\tContextTimeoutEnabled: true,\n',''))
# go-redis 默认每次建连内部重拨 5 次、间隔 100ms：主机宕机时一条命令实测 11.7s，
# 远超文档那条式子推出来的 7s
mutate("Redis 一次建连只拨一次号", "xredis/xredis.go", "./xredis", "TestNew_RedisDownDialsOncePerConnect",
       swap('\t\tDialerRetries: 1,\n', ''))
# 打在调用点上：绕开 probe 直接 Ping，启动期间的退出信号要等到 ReadTimeout 才生效
mutate("Redis 建连探测收到退出信号当场放弃", "xredis/xredis.go", "./xredis", "TestNew_ExitSignalAtStartupSkipsReadTimeout",
       swap('xclient.Probe(ctx, probePolicy(cfg), probe(client))',
            'xclient.Probe(ctx, probePolicy(cfg), func(ctx context.Context) error { return client.Ping(ctx).Err() })'))
mutate("Redis 建连探测取消时不等这次读", "xredis/xredis.go", "./xredis", "TestNew_ExitSignalAtStartupSkipsReadTimeout",
       swap('\t\t\tif errors.Is(ctx.Err(), context.Canceled) {\n', '\t\t\tif false && errors.Is(ctx.Err(), context.Canceled) {\n'))
mutate("Redis 认证失败不重试", "xredis/xredis.go", "./xredis", "TestNew_AuthFailureReportedClearlyWithoutRetry",
       swap('\t\tAuthFailed: redis.IsAuthError,\n', ''))
mutate("Redis 认证失败不说成连不上", "xredis/xredis.go", "./xredis", "TestNew_AuthFailureReportedClearlyWithoutRetry",
       swap('\t\tif redis.IsAuthError(err) {\n\t\t\treturn nil, nil, xerror.Newf', '\t\tif false {\n\t\t\treturn nil, nil, xerror.Newf'))
# 不接的话 go-redis 往 stderr 写纯文本，不是 JSON、不带 trace_id
mutate("go-redis 自己的日志进 slog", "xredis/xredis.go", "./xredis", "TestGoRedisOwnLogsGoToSlogWithCallerCtx",
       swap('\tredis.SetLogger(slogLogger{})\n', ''))
mutate("go-redis 的日志带着调用方的 ctx", "xredis/xredis.go", "./xredis", "TestGoRedisOwnLogsGoToSlogWithCallerCtx",
       swap('slog.WarnContext(ctx, "xredis go-redis log"', 'slog.WarnContext(context.Background(), "xredis go-redis log"'))
mutate("Redis 命令参数不进 Span", "xredis/xredis.go", "./xredis", "TestTrace",
       swap('redisotel.InstrumentTracing(client, redisotel.WithDBStatement(false))', 'redisotel.InstrumentTracing(client)'))
mutate("Metric 关掉的 Redis 实例不导出", "xredis/xredis.go", "./xredis", "TestInstall", swap('\t\tif inst.metric {\n', '\t\tif true {\n'))
mutate("xmetric 重装后 Redis 连接池指标照样导出", "xredis/xredis.go", "./xredis", "TestInstall",
       swap('func installPoolMetrics() {\n\tif _, err := xmetric.RegisterAs(',
     'var poolInstalled bool\n\nfunc installPoolMetrics() {\n\tif poolInstalled {\n\t\treturn\n\t}\n\tpoolInstalled = true\n\tif _, err := xmetric.RegisterAs('))
mutate("XRedis 块在读配置时就校验", "xredis/config.go", "./xredis", "TestConfig_InvalidValuesFailAtConfigRead",
       swap('clients, err := xconfig.UnmarshalClients(ConfigKey, DefaultClientConfig)\n\treturn Config{Clients: clients}, err',
            'type raw ClientConfig\n\tm, err := xconfig.UnmarshalClients(ConfigKey, func() raw { return raw(DefaultClientConfig()) })\n\tclients := map[string]ClientConfig{}\n\tfor k, v := range m {\n\t\tclients[k] = ClientConfig(v)\n\t}\n\treturn Config{Clients: clients}, err'))
mutate("直接调 xredis.New 也校验", "xredis/xredis.go", "./xredis", "TestNew_NoConnectOnInvalidConfig|TestNew_ValidatesBeforeConnecting|TestNew_TLSUnreadableFileIsConfigError",
       swap('\tif err := cfg.Validate(); err != nil {', '\tif err := cfg.Validate(); false && err != nil {'))
# go-redis 把 ReadTimeout -1 当成「不限时」：一个减号就静默关掉超时保护
mutate("Redis 负的时长要被拦住", "xredis/config.go", "./xredis", "TestValidate|TestConfig_InvalidValuesFailAtConfigRead",
       swap('\t\tif d.val < 0 {\n\t\t\treturn fmt.Errorf("%s must not be negative', '\t\tif false {\n\t\t\treturn fmt.Errorf("%s must not be negative'))
mutate("Redis 负的连接数要被拦住", "xredis/config.go", "./xredis", "TestValidate",
       swap('\t\tif n.val < 0 {', '\t\tif false {'))
mutate("Redis MaxRetries 只收 -1 这一个负数", "xredis/config.go", "./xredis", "TestValidate",
       swap('if c.MaxRetries < -1 {', 'if false {'))
mutate("Redis 退避只收 -1ns 这一个负数", "xredis/config.go", "./xredis", "TestValidate",
       swap('if d.val < 0 && d.val != -1 {', 'if false {'))
mutate("Redis 校验 TLS 块", "xredis/config.go", "./xredis", "TestValidate",
       swap('\treturn c.TLS.Validate()\n}', '\treturn nil\n}'))
mutate("Redis 的 TLS 配置传给 go-redis", "xredis/xredis.go", "./xredis", "TestNew_TLS",
       swap('tlsCfg, err := cfg.TLS.Build()', '_, err := cfg.TLS.Build()'), swap('TLSConfig: tlsCfg,', 'TLSConfig: nil,'))
# Redis 7.2 之前每条新连接一个报错的 CLIENT SETINFO Span
mutate("Redis 建连不发 CLIENT SETINFO", "xredis/xredis.go", "./xredis", "TestNew_NoClientSetinfoNorMaintNotifications",
       swap('DisableIdentity: true,', 'DisableIdentity: false,'))
mutate("Redis 不开维护通知", "xredis/xredis.go", "./xredis", "TestNew_NoClientSetinfoNorMaintNotifications",
       swap('Mode: maintnotifications.ModeDisabled}', 'Mode: maintnotifications.ModeAuto}'))
# 钩子挂在建连验证之前：启动时每次 Ping 尝试都是一个没有父 Span 的 ping
mutate("Redis 链路钩子在建连验证成功之后才挂", "xredis/xredis.go", "./xredis", "TestTrace_StartupConnectCheckOpensNoSpan",
       swap('\tif err := xclient.Probe(ctx, probePolicy(cfg), probe(client)); err != nil {',
            '\tif cfg.Trace {\n\t\t_ = redisotel.InstrumentTracing(client, redisotel.WithDBStatement(false))\n\t}\n\tif err := xclient.Probe(ctx, probePolicy(cfg), probe(client)); err != nil {'))
mutate("xredis connected 日志带着实例名", "xredis/xredis.go", "./xredis", "TestInstall_LogsNameAndAddrButNotPassword",
       swap('"xredis connected", "name", name,', '"xredis connected", "name", "",'))

section("链路")
# 用了 xredis 就有链路，使用者不用记得另外 import xtrace。摘掉这一行，
# 全局的 TracerProvider 就一直是 noop：Span 什么都不记、日志没有 trace_id，而且没有任何报错
mutate("用了 xredis 不另外 import xtrace 也有链路", "xredis/xredis.go", "./xredis", "TestTracingWorksWithoutImportingXtrace",
       swap('\t_ "github.com/xiaoshicae/xone/xtrace"\n', ''))
