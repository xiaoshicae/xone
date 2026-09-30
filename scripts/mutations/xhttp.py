# xhttp 的变异：module xhttp 里的承诺。写法见 scripts/mutations/__init__.py
from . import cut, mutate, section, swap

section("客户端")
# 挂上重试条件，resty 自己「不重试」的判断就作废了：200 + 坏 JSON 会被同一个 GET 重发
mutate("只重试传输层的错", "xhttp/xhttp.go", "./xhttp", "TestRetry",
       swap('\tif !isTransportError(err) {\n', '\tif err == nil {\n'))
mutate("关闭时清掉空闲连接", "xhttp/xhttp.go", "./xhttp", "TestNew",
       swap('\treturn client, &clientCloser{pool: pool}, nil','\treturn client, &clientCloser{pool: traced(cfg, pool)}, nil'))
mutate("重试耗时算整次逻辑请求", "xhttp/metric.go", "./xhttp", "TestMetric",
       swap('elapsed(resp.Request, resp.Time())','resp.Time()'))
# 调用点：字段在 Config 里、Validate 里都有，没赋给 Transport 就是一句空话
mutate("MaxConnsPerHost 传给连接池", "xhttp/xhttp.go", "./xhttp", "TestNew_MaxConnsPerHostApplied",
       swap('\tt.MaxConnsPerHost = cfg.MaxConnsPerHost\n', ''))
mutate("MaxConnsPerHost 为负要被拦住", "xhttp/config.go", "./xhttp", "TestValidate",
       swap(' || c.MaxConnsPerHost < 0 {', ' {'))
mutate("XHttp 校验 TLS 块", "xhttp/config.go", "./xhttp", "TestValidate_TLSBlock",
       swap('\treturn c.TLS.Validate()\n}', '\treturn nil\n}'))
mutate("XHttp 的 TLS 块交给连接池", "xhttp/xhttp.go", "./xhttp", "TestNew_TLS",
       swap('\t\tt.TLSClientConfig = tlsCfg\n', ''))
# 一个减号换来的静默故障：DialTimeout 为负时每次请求当场 i/o timeout，
# Timeout 为负反而被标准库当成「不限时」，超时保护整个消失
mutate("负的时长要被拦住", "xhttp/config.go", "./xhttp", "TestInitXHttp|TestValidate",
       swap('\t\tif d.val < 0 {', '\t\tif false {'))
mutate("XHttp 块在读配置时就校验", "xhttp/xhttp.go", "./xhttp", "TestInitXHttp",
       swap('xconfig.Unmarshal(ConfigKey, &c)', 'func() error { type raw Config; return xconfig.Unmarshal(ConfigKey, (*raw)(&c)) }()'))
mutate("直接调 xhttp.New 也校验", "xhttp/xhttp.go", "./xhttp", "TestNew_DirectCallAlsoValidatesConfig",
       swap('\tif err := cfg.Validate(); err != nil {', '\tif err := cfg.Validate(); false && err != nil {'))
# XHttp.Trace 只管 Span。原先关掉它连注入一起摘了，透传头和 traceparent 断在这一跳
mutate("XHttp.Trace 关掉照样注入链路标识和透传头", "xhttp/xhttp.go", "./xhttp", "TestNew_TraceOff",
       swap('next := http.RoundTripper(propagateOnly{next: pool})', 'next := pool'))
mutate("XHttp.Trace 关掉照样按域名透传", "xhttp/xhttp.go", "./xhttp", "TestNew_TraceOff",
       swap('\treturn &xtrace.Transport{Next: next}\n', '\tif !cfg.Trace {\n\t\treturn next\n\t}\n\treturn &xtrace.Transport{Next: next}\n'))
mutate("只注入那一层不改调用方的请求", "xhttp/xhttp.go", "./xhttp", "TestNew_TraceOff",
       swap('\tr = r.Clone(r.Context())\n', ''))
mutate("只注入那一层转发 CloseIdleConnections", "xhttp/xhttp.go", "./xhttp", "TestNew_TraceOff",
       swap('func (p propagateOnly) CloseIdleConnections() {', 'func (p propagateOnly) closeIdleConnections() {'))
# 方法是自由 token，照抄进标签的话谁都能把时间序列撑爆。打在两个调用点上
mutate("出站指标的 method 标签收敛", "xhttp/metric.go", "./xhttp", "TestMetric",
       swap('normalizeMethod(raw.Method)', 'raw.Method', 2))
mutate("出站指标注册失败不让 New 失败", "xhttp/xhttp.go", "./xhttp", "TestNew_MetricRegisterFailureOnlyLogs_ClientStillUsable",
       swap('\t\t\tslog.Error("xhttp failed to register the request duration metric', '\t\t\treturn nil, nil, err\n\t\t\tslog.Error("xhttp failed to register the request duration metric'))
# resty 的默认 logger 绕开 slog 直写 stderr，重试失败时连查询串里的令牌一起打
mutate("resty 自己的日志走 slog", "xhttp/xhttp.go", "./xhttp", "TestNew_RestyLogsGoToSlogWithoutQuery",
       swap('return resty.NewWithClient(hc).SetLogger(restyLogger{})', 'return resty.NewWithClient(hc)'))
mutate("resty 日志去掉查询串", "xhttp/xhttp.go", "./xhttp", "TestNew_RestyLogsGoToSlogWithoutQuery",
       swap('"detail", stripQuery(fmt.Sprintf(format, v...))', '"detail", fmt.Sprintf(format, v...)'))
# otelhttp 只去掉 user:password，查询串原样写进 url.full
mutate("出站 Span 的 url.full 不带查询串", "xhttp/xhttp.go", "./xhttp", "TestTransport_Span",
       swap('otelhttp.NewTransport(scrubURL{next: pool},', 'otelhttp.NewTransport(pool,'))
mutate("出站 Span 名只用方法", "xhttp/xhttp.go", "./xhttp", "TestTransport|TestSpanName",
       swap('\treturn r.Method\n}', '\treturn r.Method + " " + r.URL.Path\n}'))
# resty.New() 自带 cookie jar：初始化前、关闭后的请求会共享别人种下的会话
mutate("兜底实例不带 cookie jar", "xhttp/xhttp.go", "./xhttp", "TestC_",
       swap('var fallback = newResty(&http.Client{Timeout: fallbackTimeout})', 'var fallback = resty.New().SetTimeout(fallbackTimeout)'))

section("请求日志")
mutate("出站请求日志只在 Log 开着时挂", "xhttp/xhttp.go", "./xhttp", "TestLog_OffMeansNoRequestLinesAndRestyLogsUnchanged",
       swap('\tif cfg.Log {\n\t\tinstallLog(', '\tif true {\n\t\tinstallLog('))
mutate("出站慢请求阈值交给日志", "xhttp/xhttp.go", "./xhttp", "TestLog_SlowThreshold",
       swap('installLog(client, cfg.SlowThreshold)', 'installLog(client, 0)'))
# *url.Error 的原文是 Get "http://host/x?token=…": …，查询串里的令牌跟着错误进了日志
mutate("出站请求日志的错误去掉查询串", "xhttp/log.go", "./xhttp", "TestLog_TransportError",
       swap('"error", scrubErrorText(err.Error())', '"error", err.Error()'))
mutate("出站请求日志的错误去掉 userinfo", "xhttp/log.go", "./xhttp", "TestLog_TransportErrorAfterRetriesOneLineNoQuery|TestScrubErrorText",
       swap('urlUserinfo.ReplaceAllString(stripQuery(s), "$1")', 'stripQuery(s)'))
mutate("5xx 记成失败", "xhttp/log.go", "./xhttp", "TestLog_5xxWithRetriesConfiguredIsOneWarnLine",
       swap('failed := err != nil || status >= 500', 'failed := err != nil'))
# 起点只在 Metric 开着时记的话，Metric: false 的客户端日志里的耗时只是最后一次尝试
mutate("出站请求日志的耗时算整次逻辑请求", "xhttp/log.go", "./xhttp", "TestLog_ElapsedCoversRetriesWithMetricOff",
       swap('func installLog(client *resty.Client, slow time.Duration) {\n\tmarkStart(client)\n', 'func installLog(client *resty.Client, slow time.Duration) {\n'))
mutate("出站请求日志用调用方的 ctx", "xhttp/log.go", "./xhttp", "TestLog_UsesCallerCtx",
       swap('\tctx := req.Context()\n', '\tctx := resty.New().R().Context()\n'))
# 同一件事不说两遍：resty 在重试路径上每次尝试一行 WARN、用完一行 ERROR
mutate("Log 开着时不再打 resty 的重试日志", "xhttp/log.go", "./xhttp", "TestLog_TransportErrorAfterRetriesOneLineNoQuery",
       swap('\t\treq.SetLogger(quietRequestLogger{})\n', ''))
mutate("Log 开着时不打 resty 每次尝试的 WARN", "xhttp/log.go", "./xhttp", "TestLog_TransportErrorAfterRetriesOneLineNoQuery",
       swap('\tif format == "%v, Attempt %v" {\n\t\treturn\n\t}\n', ''))
mutate("Log 开着时不打 resty 重试用完的 ERROR", "xhttp/log.go", "./xhttp", "TestLog_TransportErrorAfterRetriesOneLineNoQuery",
       swap('func (quietRequestLogger) Errorf(string, ...any) {}', 'func (l quietRequestLogger) Errorf(f string, v ...any) { l.restyLogger.Errorf(f, v...) }'))
mutate("Log 开着时 resty 的其余提醒照旧", "xhttp/log.go", "./xhttp", "TestLog_OtherRestyWarningsStillLogged",
       swap('\tl.restyLogger.Warnf(format, v...)\n', ''))
mutate("XHttp SlowThreshold 为负要被拦住", "xhttp/config.go", "./xhttp", "TestConfig_LogDefaults",
       swap('\t\t{"SlowThreshold", c.SlowThreshold},\n', ''))
