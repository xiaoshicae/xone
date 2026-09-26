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
# resty 的默认 logger 绕开 slog 直写 stderr，重试失败时连查询串里的令牌一起打
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
