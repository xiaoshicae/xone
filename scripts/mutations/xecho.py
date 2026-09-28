# xecho 的变异：module xecho 里的承诺。写法见 scripts/mutations/__init__.py
#
# 服务启停、TLS、代理网段、访问日志字段的实现在 internal/web，它们本身的变异在 xgin.py 和 core.py；
# 这里打的是 xecho 的调用点（配置有没有交过去、中间件有没有接上），以及 echo 独有的那几件事
# （错误在中间件返回之后才渲染、405 给了模板、c.RealIP() 默认全信、gommon 写 stdout）。
from . import cut, mutate, section, swap

section("HTTP 服务")
# 调用点：XEcho 的 Start / Stop 只是转给 web.Server。Stop 不转的话，退出信号到了服务照跑
mutate("XEcho.Stop 转给 web.Server", "xecho/xecho.go", "./xecho", "TestStop",
       swap('{ return x.server.Stop(ctx) }', '{ return nil }'))
# 监听地址、TLS 和 echo 上的中间件、信任的代理必须出自同一份配置
mutate("XEcho.Start 用的是装配时的那份配置", "xecho/xecho.go", "./xecho", "TestStart_UsesConfigFromBuildTime",
       swap('\treturn x.server.Start(x.conf.server(), x.engine)\n', '\tc, _ := x.cfg()\n\treturn x.server.Start(c.server(), x.engine)\n'))
# echo 的 e.Server 四个超时全是 0：超时得从配置抄给 web.Server 建的那个 http.Server
mutate("ReadHeaderTimeout 交给了 web.Server", "xecho/config.go", "./xecho", "TestStart_ServerTimeoutsFromConfig",
       swap('\t\tReadHeaderTimeout: c.ReadHeaderTimeout,\n', '\t\tReadHeaderTimeout: c.IdleTimeout,\n'))
mutate("XEcho UseH2C 交给了 web.Server", "xecho/config.go", "./xecho", "TestStop_H2C",
       swap('\t\tUseH2C:            c.UseH2C,\n', ''))
mutate("XEcho ClientCAFile 交给了 web.Server", "xecho/config.go", "./xecho", "TestStart_ClientCAFileEnablesMutualTLS",
       swap('\t\tClientCAFile:      c.ClientCAFile,\n', ''))
mutate("XEcho MinVersion 交给了 web.Server", "xecho/config.go", "./xecho", "TestStart_MinVersion",
       swap('\t\tMinVersion:        c.MinVersion,\n', '\t\tMinVersion:        "1.2",\n'))
# 调用点：规矩写在 internal/web，XEcho 的 Validate 得真的调到它们
mutate("XEcho Validate 查了端口和 TLS", "xecho/config.go", "./xecho", "TestValidate",
       swap('\tif err := s.ValidateListen(); err != nil {\n\t\treturn err\n\t}\n', ''))
mutate("XEcho Validate 查了超时", "xecho/config.go", "./xecho", "TestValidate",
       swap('\tif err := s.ValidateTimeouts(); err != nil {\n\t\treturn err\n\t}\n', ''))
mutate("XEcho Validate 查了代理网段", "xecho/config.go", "./xecho", "TestValidate",
       swap('\tif err := web.ValidateProxies(c.TrustedProxies); err != nil {\n\t\treturn err\n\t}\n', ''))
# echo 不拒绝不以 / 开头的路径，而是悄悄改写：留空就把指标挂到了根路径 / 上，被首页盖掉
mutate("XEcho 指标路径不以 / 开头要启动失败", "xecho/config.go", "./xecho", "TestValidate|TestLoadConfig",
       swap('if c.Metric && !strings.HasPrefix(c.MetricPath, "/") {', 'if false && !strings.HasPrefix(c.MetricPath, "/") {'))
# 配置文件里的 XEcho 块由 StageServer 的钩子认领并校验。丢掉这个错误的话，配错的值要拖到 Start 才报
mutate("配错的 XEcho 块在启动阶段就失败", "xecho/xecho.go", "./xecho", "TestLoadConfig",
       swap('\t_, err := fileConfig()\n\treturn err\n', '\t_, _ = fileConfig()\n\treturn nil\n'))
# 解到一半的非法配置里 TrustedProxies 可能正是 0.0.0.0/0
mutate("XEcho 块不合法时退回默认值", "xecho/xecho.go", "./xecho", "TestCurrentConfig|TestEngine_InvalidConfigFallsBackToSafeDefaults",
       swap('\tif err := xconfig.Unmarshal(ConfigKey, &c); err != nil {\n\t\treturn DefaultConfig(), err\n',
            '\tif err := xconfig.Unmarshal(ConfigKey, &c); err != nil {\n\t\treturn c, err\n'))
mutate("XEcho WithConfig 给的那份也要校验", "xecho/xecho.go", "./xecho", "TestStart_DoesNotListenOnInvalidConfig",
       swap('\tif err := x.override.Validate(); err != nil {', '\tif err := x.override.Validate(); false && err != nil {'))

section("错误在中间件里渲染")
# echo 在整条链返回之后才渲染错误：中间件拿到错误时 Status 200、Size 0。三个中间件各自
# 在自己这一层渲染（finish），打在每个调用点上——哪一个退回「原样返回错误」，它记下的就全是 200
mutate("访问日志记的是渲染之后的状态码", "xecho/middleware/log.go", "./xecho", "TestLog_ErrorStatus",
       swap('\t\t\tfinish(c, next(c))\n\t\t\treturn nil\n', '\t\t\treturn next(c)\n'))
mutate("指标记的是渲染之后的状态码", "xecho/middleware/metric.go", "./xecho", "TestMetric_ReturnedErrorRecordedWithRenderedStatus",
       swap('\t\t\tfinish(c, next(c))\n\t\t\treturn nil\n', '\t\t\treturn next(c)\n'))
mutate("Span 记的是渲染之后的状态码", "xecho/middleware/trace.go", "./xecho", "TestTrace_OnlyServerErrorsMarkedAsError|TestTrace_ReturnedError",
       swap('\t\t\tfinish(c, next(c))\n\t\t\treturn nil\n', '\t\t\treturn next(c)\n'))
# 渲染完照样把错误往外返回的话，echo 会再调一遍 HTTPErrorHandler：自定义的错误处理把错误响应写两遍
mutate("错误响应只渲染一次", "xecho/middleware/metric.go", "./xecho", "TestFinish_HTTPErrorHandlerRunsOnce",
       swap('\t\t\tfinish(c, next(c))\n\t\t\treturn nil\n', '\t\t\terr := next(c)\n\t\t\tfinish(c, err)\n\t\t\treturn err\n'))
# 外面几层拿到的是 nil：错误不记在 c 上的话，访问日志的 errors、Span 的 echo.errors 都是空的
mutate("渲染时把错误记在请求上", "xecho/middleware/middleware.go", "./xecho", "TestLog_ErrorStatus|TestTrace_ReturnedError",
       swap('\tc.Set(errKey, err)\n\tc.Error(err)\n', '\tc.Error(err)\n'))
mutate("访问日志带上 handler 返回的错误", "xecho/middleware/log.go", "./xecho", "TestLog_ErrorStatus",
       swap('\t\t\t\t\ta.Errors = err.Error()\n', ''))
mutate("Span 带上 handler 返回的错误", "xecho/middleware/trace.go", "./xecho", "TestTrace_ReturnedError",
       swap('span.SetAttributes(attribute.String("echo.errors", web.RedactText(err.Error())))', '_ = err'))

section("路由标签")
# 取值写在 routeOf 里，三个中间件共用；前两条改坏它本身，后三条各打一个调用点
mutate("XEcho 没匹配上的路由记 unmatched", "xecho/middleware/middleware.go", "./xecho",
       "TestLog_UnmatchedRouteIsUnmatched|TestMetric_UnmatchedRouteUsesFixedValue|TestTrace_UnmatchedRouteUsesFixedValue",
       swap('\treturn web.RouteUnmatched\n', '\treturn c.Request().URL.Path\n'))
# echo 在 405 时给的是那条路由的模板：照记的话和 xgin（记 unmatched）对不上
mutate("方法不对（405）也记 unmatched", "xecho/middleware/middleware.go", "./xecho",
       "TestMetric_UnmatchedRouteUsesFixedValue|TestTrace_UnmatchedRouteUsesFixedValue|TestLog_ErrorStatus",
       swap(' && c.Get(echo.ContextKeyHeaderAllow) == nil', ''))
mutate("XEcho 访问日志的路由取 routeOf", "xecho/middleware/log.go", "./xecho", "TestLog_UnmatchedRouteIsUnmatched",
       swap('Route:      routeOf(c),', 'Route:      c.Path(),'))
mutate("XEcho 指标的路由取 routeOf", "xecho/middleware/metric.go", "./xecho", "TestMetric_UnmatchedRouteUsesFixedValue",
       swap('route, method, code := routeOf(c),', 'route, method, code := c.Path(),'))
mutate("XEcho Span 的路由取 routeOf", "xecho/middleware/trace.go", "./xecho", "TestTrace_UnmatchedRouteUsesFixedValue",
       swap('\t\t\troute := routeOf(c) //', '\t\t\troute := c.Path() //'))
mutate("XEcho 指标的 method 标签收敛", "xecho/middleware/metric.go", "./xecho", "TestMetric_CustomMethod",
       swap('web.NormalizeMethod(c.Request().Method)', 'c.Request().Method'), swap('\t\tlatency *prometheus.HistogramVec\n\t)\n', '\t\tlatency *prometheus.HistogramVec\n\t)\n\t_ = web.NormalizeMethod\n'))
mutate("XEcho Span 的 method 收敛", "xecho/middleware/trace.go", "./xecho", "TestTrace_CustomMethod",
       swap('method := web.NormalizeMethod(r.Method)', 'method := r.Method'))

section("panic 恢复")
# echo 默认不兜 panic：连接直接断掉，栈进 stderr
mutate("XEcho 装上了 Recover", "xecho/xecho.go", "./xecho", "TestBuild_PanicRecovered",
       swap('\t\te.Use(middleware.Recover(x.recover))\n', ''))
# Recover 排在外面的话，panic 穿过 Log / Metric 时它们记下的是 200
mutate("Recover 是内置里最内层的", "xecho/xecho.go", "./xecho", "TestBuild_PanicRecoveredAndStillLogged",
       swap('\t\te.Use(middleware.Recover(x.recover))\n', ''),
       swap('\t\tif c.Log {\n\t\t\te.Use(middleware.LogScope())', '\t\te.Use(middleware.Recover(x.recover))\n\t\tif c.Log {\n\t\t\te.Use(middleware.LogScope())'))
mutate("用户中间件排在 Recover 之内", "xecho/xecho.go", "./xecho", "TestBuild_UserMiddlewareInsideRecover",
       swap('\t\te.Use(middleware.Recover(x.recover))\n\t\te.Use(x.extra...)\n', '\t\te.Use(x.extra...)\n\t\te.Use(middleware.Recover(x.recover))\n'))
mutate("WithRecoverFunc 接到 Recover 上", "xecho/xecho.go", "./xecho", "TestWithRecoverFunc",
       swap('e.Use(middleware.Recover(x.recover))', 'e.Use(middleware.Recover(nil))'))
mutate("默认的 recover 回 echo 的 500", "xecho/middleware/middleware.go", "./xecho", "TestRecover_CatchesPanicAndReturns500|TestBuild_PanicRecovered",
       swap('return echo.ErrInternalServerError }', 'return nil }'))
# ErrAbortHandler 是「断掉这个连接」的约定写法。兜住它的话本该中止的响应被写成 500 发出去
mutate("XEcho ErrAbortHandler 原样抛给 net/http", "xecho/middleware/middleware.go", "./xecho", "TestRecover",
       cut('\t\t\t\tif r == http.ErrAbortHandler {\n', '\t\t\t\t\tpanic(r)\n\t\t\t\t}\n'))
mutate("XEcho ErrAbortHandler 中止的请求登记下来", "xecho/middleware/middleware.go", "./xecho",
       "TestLog_ErrAbortHandlerAbortLoggedAs499|TestMetric_ErrAbortHandlerAbortRecordedAs499|TestTrace_ErrAbortHandlerAbortRecordedAsError",
       swap('\t\t\t\t\tc.Set(errKey, http.ErrAbortHandler)\n', ''))
mutate("XEcho 访问日志把中止的请求记成 499", "xecho/middleware/log.go", "./xecho", "TestLog_ErrAbortHandler",
       swap('Status:     status(c),', 'Status:     c.Response().Status,'))
mutate("XEcho 指标把中止的请求记成 499", "xecho/middleware/metric.go", "./xecho", "TestMetric_ErrAbortHandler",
       swap('strconv.Itoa(status(c))', 'strconv.Itoa(c.Response().Status)'))
mutate("XEcho 链路把中止的请求记成错误", "xecho/middleware/trace.go", "./xecho", "TestTrace_ErrAbortHandler",
       swap('st := status(c)', 'st := c.Response().Status'))
mutate("XEcho 断连不打栈", "xecho/middleware/middleware.go", "./xecho", "TestRecover_BrokenPipe",
       swap('if web.IsBrokenPipe(r) {', 'if false {'))
# 响应已经开始往外写了，再调 recover 函数只会把它写的东西接在半截响应后面
mutate("响应开始之后不再调 recover 函数", "xecho/middleware/middleware.go", "./xecho", "TestRecover_CustomFuncNotCalledOnceResponseStarted",
       swap('\t\t\t\tif c.Response().Committed {', '\t\t\t\tif false {'))

section("client_ip 与透传的信任边界")
# echo 的默认（IPExtractor 为 nil）是谁发来的 X-Forwarded-For 都信
mutate("client_ip 按 TrustedProxies 算", "xecho/xecho.go", "./xecho", "TestBuild_TrustsOnlyPrivateProxiesByDefault",
       swap('\te.IPExtractor = web.ParseProxies(c.TrustedProxies).ClientIP\n', '\t_ = web.ParseProxies(c.TrustedProxies).ClientIP\n'))
mutate("client_ip 的可信网段来自配置", "xecho/xecho.go", "./xecho", "TestBuild_TrustedProxiesEmptyListTrustsNone|TestBuild_ForwardedHeadersOnlyWithProxiesConfigured|TestLoadConfig_EmptyTrustedProxies",
       swap('e.IPExtractor = web.ParseProxies(c.TrustedProxies).ClientIP', 'e.IPExtractor = web.ParseProxies([]string{web.TrustPrivate}).ClientIP'))
# 规则本身在 internal/web（core.py 里有它自己的变异），这里验 xecho 真的用上了它的那一条：公网对端伪造不了
mutate("公网对端的 X-Forwarded-For 不认", "internal/web/proxy.go", "./xecho", "TestBuild_TrustsOnlyPrivateProxiesByDefault",
       swap('\tif ps.contains(remote) {\n', '\tif true {\n'))
mutate("TrustedProxies 默认是 private", "xecho/config.go", "./xecho", "TestBuild_TrustsOnlyPrivateProxiesByDefault|TestBuild_PassthroughHeadersOnlyFromPrivateByDefault",
       swap('\t\tTrustedProxies:    []string{web.TrustPrivate},\n', ''))
# 回调在配置落到 echo 上之后才跑，所以回调里换的 IPExtractor 以回调为准。
# 两种改坏的写法：配置挪到回调之后落，或者 Start 时再落一遍
mutate("回调里的 echo 设置盖得过配置", "xecho/xecho.go", "./xecho", "TestBuild_CallbackEngineSettingsOverrideConfig",
       swap('\t\tapplyConfig(e, c)\n', ''),
       swap('\t\tfor _, f := range x.routes {\n\t\t\tf(e)\n\t\t}\n', '\t\tfor _, f := range x.routes {\n\t\t\tf(e)\n\t\t}\n\t\tapplyConfig(e, c)\n'))
mutate("Start 不再把配置落一遍", "xecho/xecho.go", "./xecho", "TestBuild_CallbackEngineSettingsOverrideConfig",
       swap('\treturn x.server.Start(x.conf.server(), x.engine)\n', '\tapplyConfig(x.engine, x.conf)\n\treturn x.server.Start(x.conf.server(), x.engine)\n'))
# 「谁是自己人」只看 TrustedProxies、只看直连的那一跳。记号打错一次，要么伪造的头被带进内网，要么透传整个失效
mutate("XEcho 对端在 TrustedProxies 里才算可信", "xecho/xecho.go", "./xecho", "TestBuild_PassthroughHeadersOnlyFromTrustedProxies|TestBuild_PassthroughHeadersOnlyFromPrivateByDefault",
       swap('if x.trusted.Trusts(web.RemoteIP(c.Request())) {', 'if len(x.trusted) > 0 {'))
mutate("可信判断看直连对端而不是 c.RealIP()", "xecho/xecho.go", "./xecho", "TestBuild_PassthroughHeadersOnlyFromTrustedProxies|TestBuild_PassthroughHeadersOnlyFromPrivateByDefault",
       swap('if x.trusted.Trusts(web.RemoteIP(c.Request())) {', 'if x.trusted.Trusts(c.RealIP()) {'))
mutate("XEcho 可信网段来自装配时的配置", "xecho/xecho.go", "./xecho", "TestBuild_PassthroughHeadersOnlyFromTrustedProxies",
       swap('x.trusted = web.ParseProxies(c.TrustedProxies)', 'x.trusted = web.ParseProxies(nil)'))
mutate("XEcho 装配时先判对端再开链路", "xecho/xecho.go", "./xecho", "TestBuild_PassthroughHeadersOnlyFromTrustedProxies",
       swap('e.Use(x.markTrustedPeer, middleware.Trace())', 'e.Use(middleware.Trace())'))
# XEcho.Trace 只管 Span：关掉它不该连上游的链路标识和透传头一起摘掉
mutate("XEcho.Trace 关掉照样接上游的链路和透传", "xecho/xecho.go", "./xecho", "TestBuild_TraceDisabledOnlySkipsSpan_StillPropagates",
       swap('e.Use(x.markTrustedPeer, middleware.Propagate())', 'e.Use(x.markTrustedPeer)'))
mutate("XEcho.Trace 关掉时可信规则不变", "xecho/xecho.go", "./xecho", "TestBuild_TraceDisabledOnlySkipsSpan_StillPropagates",
       swap('e.Use(x.markTrustedPeer, middleware.Propagate())', 'e.Use(middleware.Propagate())'))
mutate("XEcho Propagate 也把可信记号交给 xtrace", "xecho/middleware/trace.go", "./xecho", "TestTrace_PeerTrustFollowsXechoMarker",
       swap('c.SetRequest(r.WithContext(otel.GetTextMapPropagator().Extract(r.Context(), inbound(c))))',
            'c.SetRequest(r.WithContext(otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))))'))
mutate("XEcho 链路中间件把可信记号交给 xtrace", "xecho/middleware/trace.go", "./xecho", "TestTrace_PeerTrustFollowsXechoMarker",
       swap('\tif trusted, _ := c.Get(peer.TrustedKey).(bool); trusted {\n\t\treturn trustedCarrier{h}\n\t}\n', '\t_ = peer.TrustedKey\n'))

section("中间件与开关")
mutate("XEcho 访问日志的开关读的是配置", "xecho/xecho.go", "./xecho", "TestLog_", swap('\t\tif c.Log {\n', '\t\tif true {\n', 2))
mutate("XEcho 链路的开关读的是配置", "xecho/xecho.go", "./xecho", "TestTrace_", swap('\t\tif c.Trace {\n', '\t\tif true {\n'))
mutate("XEcho 关掉指标就不挂指标中间件", "xecho/xecho.go", "./xecho", "TestMetric_",
       swap('\t\tif c.Metric {\n\t\t\te.Use(middleware.Metric())', '\t\tif true {\n\t\t\te.Use(middleware.Metric())'))
mutate("XEcho 关掉指标就不注册端点", "xecho/xecho.go", "./xecho", "TestBuild_NoMetricsEndpointWhenDisabled",
       swap('\t\tif c.Metric {\n\t\t\te.GET(c.MetricPath, serveMetrics)', '\t\tif true {\n\t\t\te.GET(c.MetricPath, serveMetrics)'))
mutate("XEcho 指标端点挂在配置的路径上", "xecho/xecho.go", "./xecho", "TestBuild_MetricsPathConfigurable",
       swap('e.GET(c.MetricPath, serveMetrics)', 'e.GET("/metrics", serveMetrics)'))
# 指标端点每次请求再取 handler：装配早于 xmetric 初始化时，定死的是一个永远是空的兜底 registry
mutate("XEcho 指标端点每次取当前的 registry", "xecho/xecho.go", "./xecho", "TestBuild_MetricsEndpointOKWhenEngineFetchedFirst",
       swap('func serveMetrics(c echo.Context) error {\n\txmetric.Handler().ServeHTTP(c.Response(), c.Request())\n\treturn nil\n}',
            'var metricsHandler = xmetric.Handler()\n\nfunc serveMetrics(c echo.Context) error {\n\tmetricsHandler.ServeHTTP(c.Response(), c.Request())\n\treturn nil\n}'))
mutate("XEcho 用户中间件挂上了", "xecho/xecho.go", "./xecho", "TestBuild_UserMiddlewareAfterBuiltin|TestBuild_MetricsEndpointRunsUserMiddleware",
       swap('\t\te.Use(x.extra...)\n', ''))
mutate("XEcho 跳过日志的路径读的是配置", "xecho/xecho.go", "./xecho", "TestLogSkipPaths",
       swap('skip := slices.Clone(c.LogSkipPaths)', 'skip := slices.Clone([]string{})'))
mutate("XEcho 指标端点自动不记访问日志", "xecho/xecho.go", "./xecho", "TestLogSkipPaths_MetricsPathAddedAutomatically",
       swap('\t\t\t\tskip = append(skip, c.MetricPath)\n', ''))
# 接反了的话，只开了请求体的人，响应体（可能带着令牌）进了日志
mutate("XEcho 请求体和响应体的开关各管各的", "xecho/xecho.go", "./xecho", "TestLogBody",
       swap('middleware.WithBody(c.LogRequestBody, c.LogResponseBody)', 'middleware.WithBody(c.LogResponseBody, c.LogRequestBody)'))
mutate("XEcho 查询串的开关读的是配置", "xecho/xecho.go", "./xecho", "TestLogQueryAndHeaders",
       swap('middleware.WithQuery(c.LogQuery)', 'middleware.WithQuery(true)'))
mutate("XEcho 请求头和响应头的开关各管各的", "xecho/xecho.go", "./xecho", "TestLogQueryAndHeaders",
       swap('middleware.WithHeaders(c.LogRequestHeaders, c.LogResponseHeaders)', 'middleware.WithHeaders(c.LogResponseHeaders, c.LogRequestHeaders)'))
mutate("XEcho LogScope 开出请求级的字段作用域", "xecho/middleware/middleware.go", "./xecho", "TestLogScope",
       swap('\t\t\tc.SetRequest(r.WithContext(xlog.CtxWithScope(r.Context())))\n', '\t\t\t_, _ = r, xlog.CtxWithScope\n'))
# 词表进程里只有一张（internal/web）：公开的这两个名字不转过去的话，使用者补的词谁都不认
mutate("XEcho AddSensitiveFields 写进共用的词表", "xecho/middleware/redact.go", "./xecho", "TestAddSensitive",
       swap('{ web.AddSensitiveFields(fields...) }', '{}'))
mutate("XEcho AddSensitiveHeaders 写进共用的名单", "xecho/middleware/redact.go", "./xecho", "TestAddSensitive",
       swap('{ web.AddSensitiveHeaders(headers...) }', '{}'))

section("访问日志的取值")
mutate("XEcho 访问日志拿到的是真实的响应头", "xecho/middleware/log.go", "./xecho", "TestLog_ResponseHeaders",
       swap('RespHeader: resp.Header(),', 'RespHeader: http.Header{},'))
mutate("XEcho 访问日志的 bytes_out 是写出的字节数", "xecho/middleware/log.go", "./xecho", "TestLog_RecordsRequestAndResponseSizes|TestLog_ErrorStatus",
       swap('BytesOut:   int(resp.Size),', 'BytesOut:   0,'))
# client_ip 取 c.RealIP()：按 e.IPExtractor 算，回调里换掉的也算
mutate("XEcho 访问日志的 client_ip 按 IPExtractor 算", "xecho/middleware/log.go", "./xecho", "TestLog_ClientIPFollowsIPExtractor|TestBuild_TrustsOnlyPrivateProxiesByDefault",
       swap('ClientIP:   c.RealIP(),', 'ClientIP:   web.RemoteIP(c.Request()),'))
# echo 的 Flush 走 http.ResponseController：包装层不交出原来的 writer，SSE 一 Flush 就 panic
mutate("截响应的 writer 不挡 Flush", "xecho/middleware/log.go", "./xecho", "TestLog_ResponseCaptureKeepsFlushWorking",
       swap('func (w *captureWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }', ''))
mutate("截响应的 writer 请求结束前换回去", "xecho/middleware/log.go", "./xecho", "TestLog_ResponseWriterRestoredAfterRequest",
       swap('\t\t\t\t\tresp.Writer = orig\n', ''))
mutate("XEcho 响应体只截前缀", "xecho/middleware/log.go", "./xecho", "TestLog_ResponseBodyTruncatedToPrefixOverLimit",
       swap('w.buf.Write(b[:min(len(b), maxResponseBody-w.buf.Len())])', 'w.buf.Write(b)'))

section("链路")
# 用了 xecho 就有链路，使用者不用记得另外 import xtrace
mutate("用了 xecho 不另外 import xtrace 也有链路", "xecho/xecho.go", "./xecho", "TestTracingWorksWithoutImportingXtrace",
       swap('\t_ "github.com/xiaoshicae/xone/xtrace"\n', ''))
mutate("handler 的 ctx 带着服务端 Span", "xecho/middleware/trace.go", "./xecho", "TestTrace_StartsSpanAndReturnsTraceID",
       swap('\t\t\tc.SetRequest(r.WithContext(ctx))\n', '\t\t\t_ = ctx\n'))
mutate("XEcho 响应回带 X-Trace-Id", "xecho/middleware/trace.go", "./xecho", "TestTrace_StartsSpanAndReturnsTraceID|TestTrace_ReturnedError",
       swap('\t\t\t\tc.Response().Header().Set(TraceIDHeader, sc.TraceID().String())\n', ''))

section("echo 自己的日志")
# gommon 默认写 os.Stdout、是它自己的 JSON 格式：进不了日志平台的检索和告警
mutate("echo 自己的日志接到 slog", "xecho/xecho.go", "./xecho", "TestEchoLogger_RoutedToSlog",
       swap('\te.Logger.SetOutput(echoLog{})\n', ''))
mutate("echo 日志的级别从行首取", "xecho/xecho.go", "./xecho", "TestEchoLogger_RoutedToSlog",
       swap('\te.Logger.SetHeader("${level}")\n', ''))

section("错误文本脱敏")
mutate("访问日志的 errors 字段脱过敏（xecho）", "internal/web/accesslog.go", "./xecho", "TestLog_ErrorTextIsRedacted",
       swap('slog.String("errors", RedactText(a.Errors))', 'slog.String("errors", a.Errors)'))
mutate("Span 上的 echo.errors 脱过敏", "xecho/middleware/trace.go", "./xecho", "TestTrace_ErrorTextIsRedactedOnSpan",
       swap('web.RedactText(err.Error())', 'err.Error()'))
