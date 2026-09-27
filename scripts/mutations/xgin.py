# xgin 的变异：module xgin 里的承诺。写法见 scripts/mutations/__init__.py
from . import cut, mutate, section, swap

section("HTTP 服务")
# 等多久只看调用方的 ctx（xone.Run 给的是服务那一段停止预算）。换掉它的话
# 挂住的请求让 Stop 一直不返回，「整个退出流程只有一份预算」就成了空话
mutate("服务不超过调用方给的截止时间", "xgin/xgin.go", "./xgin", "TestStop",
       swap('shutCtx, cancel := shutdownCtx(ctx)', 'shutCtx, cancel := shutdownCtx(context.WithoutCancel(ctx))'))
mutate("超时后强制断掉在途连接", "xgin/xgin.go", "./xgin", "TestStop",
       swap('\t\tif cerr := srv.Close(); cerr != nil {\n\t\t\tslog.Warn("xgin force close failed", "error", cerr)\n\t\t}\n',''))
# Close 只关连接、取消请求的 ctx，handler 的协程照跑。Close 完就返回的话，
# 正在收尾的 handler 还没返回，框架就去关数据库了
mutate("断连之后等 handler 真正返回", "xgin/xgin.go", "./xgin", "TestStop_WaitsForHandlersAfterForceClose|TestStop_HandlerIgnoringCtxReportsRemainingCount",
       swap('if n := g.waitHandlers(ctx); n > 0 {', 'if n := int64(0); n > 0 {'))
mutate("每个请求都记进在途计数", "xgin/xgin.go", "./xgin", "TestStop_WaitsForHandlersAfterForceClose",
       swap('Handler:           g.track(g.engine.Handler()),', 'Handler:           g.engine.Handler(),'))
# Shutdown 用满全部时间的话，Close 落下时预算已经花完，收尾的 handler 没人等
mutate("Shutdown 给等 handler 留出一截", "xgin/xgin.go", "./xgin", "TestStop_WaitsForHandlersAfterForceClose",
       swap('shutCtx, cancel := shutdownCtx(ctx)', 'shutCtx, cancel := context.WithCancel(ctx)'))
mutate("内置路由也走用户中间件", "xgin/xgin.go", "./xgin", "TestBuild",
       swap('\t\te.Use(middleware.Recover(g.recover))\n\t\te.Use(g.extra...)\n', '\t\te.Use(middleware.Recover(g.recover))\n'),
       swap('\t\tfor _, f := range g.routes {', '\t\te.Use(g.extra...)\n\t\tfor _, f := range g.routes {'))
mutate("公网对端的 X-Forwarded-For 不认", "xgin/xgin.go", "./xgin", "TestBuild|TestLog",
       cut('\tif err := e.SetTrustedProxies(c.trustedProxies()); err != nil {',
    '_ = e.SetTrustedProxies([]string{})\n\t}\n'))
# h2c 原先靠 x/net 的 h2c.NewHandler：连接被劫持，Shutdown 约 100µs 就返回 nil、
# 在途请求照跑，框架紧接着去关数据库。换回那个写法，这条承诺就没了
mutate("h2c 的在途请求也等它做完", "xgin/xgin.go", "./xgin", "TestStop_H2C",
       swap('\t"github.com/gin-gonic/gin"\n', '\t"github.com/gin-gonic/gin"\n\t"golang.org/x/net/http2"\n\t"golang.org/x/net/http2/h2c"\n'),
       swap('\t\tHandler:           g.track(g.engine.Handler()),\n\t\tProtocols:         protocols(c),\n',
     '\t\tHandler:           h2c.NewHandler(g.track(g.engine.Handler()), &http2.Server{}),\n'))
mutate("不开 UseH2C 时不接受明文 HTTP/2", "xgin/xgin.go", "./xgin", "TestStart_CleartextHTTP2RejectedWithoutH2C",
       swap('p.SetUnencryptedHTTP2(c.UseH2C && !c.tlsEnabled())', 'p.SetUnencryptedHTTP2(true)'))
# net/http 把 ReadHeaderTimeout 的 0 当成「退到 ReadTimeout」，而后者默认也是 0：
# 慢连接攻击的主要防线整个消失，配置文件看上去只是写了个 0
mutate("读请求头超时写 0 要拦住", "xgin/config.go", "./xgin", "TestValidate", swap('\t\tif d.val <= 0 {', '\t\tif d.val < 0 {'))
mutate("负的读写超时要拦住", "xgin/config.go", "./xgin", "TestValidate",
       swap('if c.ReadTimeout < 0 || c.WriteTimeout < 0 {', 'if false {'))
# gin 不拒绝不以 / 开头的路径，而是悄悄改写：留空就把指标挂到了根路径 / 上
mutate("指标路径不以 / 开头要启动失败", "xgin/config.go", "./xgin", "TestValidate|TestLoadConfig",
       swap('if c.Metric && !strings.HasPrefix(c.MetricPath, "/") {', 'if false {'))
# 配置文件里的 XGin 块由 StageServer 的钩子认领并校验（Unmarshal 调 Validate）。
# 丢掉这个错误的话，配错的值要拖到服务 Start 才报出来，那时别的组件都已经连好了
mutate("配错的 XGin 块在启动阶段就失败", "xgin/xgin.go", "./xgin", "TestLoadConfig",
       swap('\t_, err := fileConfig()\n\treturn err\n', '\t_, _ = fileConfig()\n\treturn nil\n'))
# CurrentConfig 和装配都靠它兜底：解到一半的非法配置里 TrustedProxies 可能正是 0.0.0.0/0
mutate("XGin 块不合法时退回默认值", "xgin/xgin.go", "./xgin", "TestCurrentConfig|TestEngine_InvalidConfigFallsBackToSafeDefaults",
       swap('\tif err := xconfig.Unmarshal(ConfigKey, &c); err != nil {\n\t\treturn DefaultConfig(), err\n',
     '\tif err := xconfig.Unmarshal(ConfigKey, &c); err != nil {\n\t\treturn c, err\n'))
mutate("WithConfig 给的那份也要校验", "xgin/xgin.go", "./xgin", "TestStart_DoesNotListenOnInvalidConfig",
       swap('\tif err := g.override.Validate(); err != nil {', '\tif err := g.override.Validate(); false && err != nil {'))
# 监听地址、TLS 和 engine 上的中间件、信任的代理必须出自同一份配置。
# Start 自己再读一遍的话，装配之后换过的配置只生效一半
mutate("Start 用的是装配时的那份配置", "xgin/xgin.go", "./xgin", "TestStart_UsesConfigFromBuildTime",
       swap('\tc := g.conf\n', '\tc, _ := g.cfg()\n'))
# gin.New 在 debug 模式下会打一段「切到 release」的警告。先建后设的话，
# 配成 release 的服务照样打出它（使用者的二进制里没设 GIN_MODE 时默认就是 debug）
mutate("Mode 在建 engine 之前设", "xgin/xgin.go", "./xgin", "TestBuild_Mode",
       swap('\t\tgin.SetMode(c.Mode)\n\t\te := gin.New()\n', '\t\te := gin.New()\n\t\tgin.SetMode(c.Mode)\n'))

section("中间件")
mutate("代理网段写错要启动失败", "xgin/config.go", "./xgin", "TestValidate", swap('if p != trustPrivate && !isIPOrCIDR(p) {','if false {'))
mutate("指标的 method 标签收敛", "xgin/middleware/metric.go", "./xgin", "TestMetric",
       swap('normalizeMethod(c.Request.Method)', 'c.Request.Method'))
mutate("请求头里的凭证被遮掉", "xgin/middleware/redact.go", "./xgin", "TestRedact",
       swap('set[name] || ', ''), swap('\tset := headers()\n', '\tset := headers()\n\t_ = set\n'))
mutate("配置在装配时落到 engine 上", "xgin/xgin.go", "./xgin", "TestBuild", swap('\t\tapplyConfig(e, c)\n', ''))
# 回调在配置落到 engine 上之后才跑，所以回调里明确设了的以回调为准。
# 两种改坏的写法：配置挪到回调之后落，或者 Start 时再落一遍（原先就是这样）
mutate("回调里的 engine 设置盖得过配置", "xgin/xgin.go", "./xgin", "TestBuild_CallbackEngineSettingsOverrideConfig",
       swap('\t\tapplyConfig(e, c)\n', ''),
       swap('\t\tfor _, f := range g.routes {\n\t\t\tf(e)\n\t\t}\n', '\t\tfor _, f := range g.routes {\n\t\t\tf(e)\n\t\t}\n\t\tapplyConfig(e, c)\n'))
mutate("Start 不再把配置落一遍", "xgin/xgin.go", "./xgin", "TestBuild_CallbackEngineSettingsOverrideConfig",
       swap('\tc := g.conf\n', '\tc := g.conf\n\tapplyConfig(g.engine, c)\n'))
# 开关写在配置里，读它们的是装配里的这几处。开关接不上是最难发现的一类 bug：
# 程序照常跑，只是那一项从来没生效
mutate("访问日志的开关读的是配置", "xgin/xgin.go", "./xgin", "TestLog_", swap('\t\tif c.Log {\n', '\t\tif true {\n', 2))
mutate("链路的开关读的是配置", "xgin/xgin.go", "./xgin", "TestTrace_", swap('\t\tif c.Trace {\n', '\t\tif true {\n'))
mutate("关掉指标就不挂指标中间件", "xgin/xgin.go", "./xgin", "TestMetric_",
       swap('\t\tif c.Metric {\n\t\t\te.Use(middleware.Metric())', '\t\tif true {\n\t\t\te.Use(middleware.Metric())'))
mutate("关掉指标就不注册端点", "xgin/xgin.go", "./xgin", "TestBuild_NoMetricsEndpointWhenDisabled",
       swap('\t\tif c.Metric {\n\t\t\te.GET(c.MetricPath, serveMetrics)', '\t\tif true {\n\t\t\te.GET(c.MetricPath, serveMetrics)'))
mutate("指标端点挂在配置的路径上", "xgin/xgin.go", "./xgin", "TestBuild_MetricsPathConfigurable",
       swap('e.GET(c.MetricPath, serveMetrics)', 'e.GET("/metrics", serveMetrics)'))
mutate("跳过日志的路径读的是配置", "xgin/xgin.go", "./xgin", "TestLogSkipPaths",
       swap('skip := slices.Clone(c.LogSkipPaths)', 'skip := []string{}'))
# 接反了的话，只开了请求体的人，响应体（可能带着令牌）进了日志
mutate("请求体和响应体的开关各管各的", "xgin/xgin.go", "./xgin", "TestLogBody",
       swap('middleware.WithBody(c.LogRequestBody, c.LogResponseBody)', 'middleware.WithBody(c.LogResponseBody, c.LogRequestBody)'))
# 开关各自接到对应的选项上：接成 true 就是默认把查询串、Authorization、Set-Cookie 写进日志
mutate("查询串的开关读的是配置", "xgin/xgin.go", "./xgin", "TestLogQueryAndHeaders",
       swap('middleware.WithQuery(c.LogQuery)', 'middleware.WithQuery(true)'))
mutate("请求头和响应头的开关各管各的", "xgin/xgin.go", "./xgin", "TestLogQueryAndHeaders",
       swap('middleware.WithHeaders(c.LogRequestHeaders, c.LogResponseHeaders)', 'middleware.WithHeaders(c.LogResponseHeaders, c.LogRequestHeaders)'))
mutate("中文翻译的开关读的是配置", "xgin/xgin.go", "./xgin", "TestZHTranslations",
       swap('\t\tif c.ZHTranslations {', '\t\tif false {'))
mutate("查询串不进访问日志", "xgin/middleware/log.go", "./xgin", "TestLog",
       swap('slog.String("path", c.Request.URL.Path),', 'slog.String("path", c.Request.URL.RequestURI()),'))
# 开了 LogQuery 也得逐字段遮：?access_token= 原样进日志就是凭证落盘
mutate("遮掉的表单值写成标记而不是转义串", "xgin/middleware/redact.go", "./xgin", "TestRedactBody_Form",
       swap('return strings.ReplaceAll(values.Encode(), url.QueryEscape(Redacted), Redacted)', 'return values.Encode()'))
mutate("记下的查询串脱过敏", "xgin/middleware/log.go", "./xgin", "TestLog_Query",
       swap('slog.String("query", redactForm(c.Request.URL.RawQuery))', 'slog.String("query", c.Request.URL.RawQuery)'))
mutate("记下的请求头脱过敏", "xgin/middleware/log.go", "./xgin", "TestLog_RedactsRequestHeaders",
       swap('Value: RedactHeaders(c.Request.Header)}', 'Value: slog.AnyValue(c.Request.Header)}'))
mutate("记下的响应头脱过敏", "xgin/middleware/log.go", "./xgin", "TestLog_ResponseHeaders",
       swap('Value: RedactHeaders(c.Writer.Header())}', 'Value: slog.AnyValue(c.Writer.Header())}'))
mutate("请求体只缓存前缀", "xgin/middleware/log.go", "./xgin", "TestSnapshotBody",
       swap('io.ReadAll(io.LimitReader(req.Body, maxRequestBody))','io.ReadAll(req.Body)',1))
mutate("预读时的错误接回下游", "xgin/middleware/log.go", "./xgin", "TestSnapshotBody",
       swap('\tif b.preErr != nil {\n\t\treturn 0, b.preErr\n\t}\n',''))
# ErrAbortHandler 是「断掉这个连接」的约定写法。兜住它的话本该中止的响应
# 被写成 500 发出去，还多一份毫无意义的 panic 栈
mutate("ErrAbortHandler 原样抛给 net/http", "xgin/middleware/middleware.go", "./xgin", "TestRecover",
       cut('\t\t\tif err == http.ErrAbortHandler {\n', '\t\t\t\tpanic(err)\n\t\t\t}\n'))
# 中止的请求往往已经写出了 200 的响应头：照读 c.Writer.Status() 的话，
# 被截断的响应在访问日志、指标、链路里都是一次成功
mutate("ErrAbortHandler 中止的请求登记下来", "xgin/middleware/middleware.go", "./xgin", "TestLog_ErrAbortHandlerAbortLoggedAs499|TestMetric_ErrAbortHandlerAbortRecordedAs499|TestTrace_ErrAbortHandlerAbortRecordedAsError",
       swap('\t\t\t\t_ = c.Error(http.ErrAbortHandler) //nolint:errcheck // 只是登记\n', ''))
mutate("访问日志把中止的请求记成 499", "xgin/middleware/log.go", "./xgin", "TestLog_ErrAbortHandler",
       swap('slog.Int("status", status(c)),', 'slog.Int("status", c.Writer.Status()),'))
mutate("指标把中止的请求记成 499", "xgin/middleware/metric.go", "./xgin", "TestMetric_ErrAbortHandler",
       swap('strconv.Itoa(status(c))', 'strconv.Itoa(c.Writer.Status())'))
mutate("链路把中止的请求记成错误", "xgin/middleware/trace.go", "./xgin", "TestTrace_ErrAbortHandler",
       swap('st := status(c)', 'st := c.Writer.Status()'))
# Content-Type 大小写不敏感：照字面比的话 Multipart/Form-Data 的文件内容整个进日志
mutate("上传和二进制流不读，不论大小写", "xgin/middleware/log.go", "./xgin", "TestSnapshotBody",
       swap('ct := strings.ToLower(req.Header.Get("Content-Type"))', 'ct := req.Header.Get("Content-Type")'))
# encoding/json 按 Unicode 折叠匹配字段名：{"ſecret":…} 绑得上 Secret。
# 只转小写的话字段名比对和预检都认不出它
mutate("字段名按 Unicode 折叠比对", "xgin/middleware/redact.go", "./xgin", "TestRedactBody_Unicode",
       swap('b.WriteRune(foldRune(r))', 'b.WriteRune(unicode.ToLower(r))'))
# 预检和字段名比对共用 sensitive，折叠本身由上一条盯着；这一条打在调用点上：
# 预检换成只转小写的朴素写法，{"ſecret":…} 就走快路径原样进日志
mutate("敏感词预检按 Unicode 折叠", "xgin/middleware/redact.go", "./xgin", "TestRedactBody_Unicode",
       swap('sensitive(s, words())', 'func(ws []string) bool { return slices.ContainsFunc(ws, func(w string) bool { return strings.Contains(strings.ToLower(s), w) }) }(words())'),
       swap('|| sensitive(body, ws)', '|| slices.ContainsFunc(ws, func(w string) bool { return strings.Contains(strings.ToLower(body), w) })'))
# path 特意不带查询串，Referer 却带着上一个页面的完整 URL
mutate("URL 类请求头去掉查询串", "xgin/middleware/redact.go", "./xgin", "TestRedactHeaders_URL",
       swap('\t\t\tv = stripQuery(v)\n', ''))
mutate("链路的 method 收敛", "xgin/middleware/trace.go", "./xgin", "TestTrace",
       swap('method := normalizeMethod(c.Request.Method)', 'method := c.Request.Method'))
# 原先是精确匹配：new_password、client_secret、sessionToken 原样进日志
mutate("JSON 字段名里带敏感词也遮", "xgin/middleware/redact.go", "./xgin", "TestRedactBody",
       swap('\t\t\tif sensitive(k, ws) {\n\t\t\t\tt[k] = Redacted', '\t\t\tif slices.Contains(ws, normalize(k)) {\n\t\t\t\tt[k] = Redacted'))
mutate("表单字段名里带敏感词也遮", "xgin/middleware/redact.go", "./xgin", "TestRedactBody",
       swap('\t\tif sensitive(k, ws) {\n\t\t\tvalues[k]', '\t\tif slices.Contains(ws, normalize(k)) {\n\t\t\tvalues[k]'))
# 名单永远列不全（Proxy-Authorization 就曾漏在外面），词表是兜底的那一层
mutate("请求头名字里带敏感词也遮", "xgin/middleware/redact.go", "./xgin", "TestRedactHeaders",
       swap(' || sensitive(k, ws)', ''), swap('\tset := headers()\n\tws := words()\n', '\tset := headers()\n\tws := words()\n\t_ = ws\n'))
# 预检认不出 api-key 的话，这种 body 走快路径原样进日志，根本到不了逐字段脱敏
mutate("敏感词预检忽略分隔符", "xgin/middleware/redact.go", "./xgin", "TestRedactBody",
       swap('sensitive(s, words())', 'func(ws []string) bool { return slices.ContainsFunc(ws, func(w string) bool { return strings.Contains(strings.Map(foldRune, s), w) }) }(words())'),
       swap('|| sensitive(body, ws)', '|| slices.ContainsFunc(ws, func(w string) bool { return strings.Contains(strings.Map(foldRune, body), w) })'))
mutate("脱敏后大整数不丢精度", "xgin/middleware/redact.go", "./xgin", "TestRedactBody", swap('\tdec.UseNumber()\n', ''))
mutate("脱敏后不转义 HTML 字符", "xgin/middleware/redact.go", "./xgin", "TestRedactBody",
       swap('\tenc.SetEscapeHTML(false)\n', ''))
mutate("JSON 后面跟着别的东西时整个遮掉", "xgin/middleware/redact.go", "./xgin", "TestRedactBody",
       swap('dec.Decode(new(any)) != io.EOF', '(dec.Decode(new(any)) != io.EOF && false)'))
# 「谁是自己人」只看 TrustedProxies。记号打错一次，要么伪造的头被带进内网，要么透传整个失效
mutate("对端在 TrustedProxies 里才算可信", "xgin/xgin.go", "./xgin", "TestBuild_PassthroughHeadersOnlyFromTrustedProxies|TestBuild_PassthroughHeadersOnlyFromPrivateByDefault",
       swap('if trustedAddr(g.trusted, c.RemoteIP()) {', 'if len(g.trusted) > 0 {'))
mutate("可信网段来自装配时的配置", "xgin/xgin.go", "./xgin", "TestBuild_PassthroughHeadersOnlyFromTrustedProxies",
       swap('g.trusted = prefixes(c.trustedProxies())', 'g.trusted = prefixes(nil)'))
# 默认只信私有网段：K8s 里 Pod IP 随机，Ingress、负载均衡、sidecar 转发来的默认就认
mutate("TrustedProxies 默认是 private", "xgin/config.go", "./xgin", "TestBuild_TrustsOnlyPrivateProxiesByDefault|TestBuild_PassthroughHeadersOnlyFromPrivateByDefault",
       swap('\t\tTrustedProxies:     []string{trustPrivate},\n', ''))
mutate("private 包含运营商级 NAT 网段", "xgin/config.go", "./xgin", "TestBuild_PassthroughHeadersOnlyFromPrivateByDefault",
       swap('"192.168.0.0/16", "100.64.0.0/10",', '"192.168.0.0/16",'))
mutate("private 包含 IPv6 ULA", "xgin/config.go", "./xgin", "TestBuild_PassthroughHeadersOnlyFromPrivateByDefault",
       swap('"::1/128", "fc00::/7",', '"::1/128",'))
mutate("private 和别的网段一起写时别的网段也算", "xgin/config.go", "./xgin", "TestBuild_TrustedProxiesMixesPrivateWithOtherCIDRs",
       swap('\t\tout = append(out, p)\n\t}\n\treturn out', '\t}\n\treturn out'))
# 调用点：client_ip 那边（gin）也得拿展开后的网段，直接给 "private" 它解析失败、退回谁都不信
mutate("gin 拿到的是展开后的网段", "xgin/xgin.go", "./xgin", "TestBuild_TrustsOnlyPrivateProxiesByDefault",
       swap('e.SetTrustedProxies(c.trustedProxies())', 'e.SetTrustedProxies(c.TrustedProxies)'))
mutate("装配时先判对端再开链路", "xgin/xgin.go", "./xgin", "TestBuild_PassthroughHeadersOnlyFromTrustedProxies",
       swap('e.Use(g.markTrustedPeer, middleware.Trace())', 'e.Use(middleware.Trace())'))
# XGin.Trace 只管 Span。原先关掉它连提取一起摘了，上游的链路标识和透传头都不收
mutate("XGin.Trace 关掉照样接上游的链路和透传", "xgin/xgin.go", "./xgin", "TestBuild_TraceDisabledOnlySkipsSpan_StillPropagates",
       swap('e.Use(g.markTrustedPeer, middleware.Propagate())', 'e.Use(g.markTrustedPeer)'))
mutate("XGin.Trace 关掉时可信规则不变", "xgin/xgin.go", "./xgin", "TestBuild_TraceDisabledOnlySkipsSpan_StillPropagates",
       swap('e.Use(g.markTrustedPeer, middleware.Propagate())', 'e.Use(middleware.Propagate())'))
mutate("Propagate 也把可信记号交给 xtrace", "xgin/middleware/trace.go", "./xgin", "TestTrace_PeerTrustFollowsXginMarker",
       swap('WithContext(otel.GetTextMapPropagator().Extract(c.Request.Context(), inbound(c)))', 'WithContext(otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header)))'))
mutate("链路中间件把可信记号交给 xtrace", "xgin/middleware/trace.go", "./xgin", "TestTrace_PeerTrustFollowsXginMarker",
       swap('\tif c.GetBool(peer.TrustedKey) {\n\t\treturn trustedCarrier{h}\n\t}\n', '\t_ = peer.TrustedKey\n'))

section("客户端")
mutate("XGin 服务端 TLS 设置交给 http.Server", "xgin/xgin.go", "./xgin", "TestStart_ClientCAFile|TestStart_MinVersion",
       swap('\tsrv.TLSConfig = tlsCfg', '\t_ = tlsCfg'))
mutate("XGin ClientCAFile 开双向认证", "xgin/config.go", "./xgin", "TestStart_ClientCAFileEnablesMutualTLS",
       swap('\t\tcfg.ClientAuth = tls.RequireAndVerifyClientCert\n', ''))
mutate("XGin MinVersion 生效", "xgin/config.go", "./xgin", "TestStart_MinVersion",
       swap('cfg := &tls.Config{MinVersion: tlsVersions[c.MinVersion]}', 'cfg := &tls.Config{}'))
# 以为开了双向认证，实际是谁都能连的明文
mutate("XGin ClientCAFile 要和证书一起配", "xgin/config.go", "./xgin", "TestValidate_ServerTLSNewFields|TestConfig_ServerTLSLoadedFromFile",
       swap('if c.ClientCAFile != "" && !c.tlsEnabled() {', 'if false {'))
mutate("XGin MinVersion 只收 1.2 / 1.3", "xgin/config.go", "./xgin", "TestValidate_ServerTLSNewFields",
       swap('if _, ok := tlsVersions[c.MinVersion]; !ok {', 'if false {'))
# gin 的 ResponseWriter 接口带 WriteString，handler 直接调它是常见写法。
# 包装层只包 Write 的话，这条路写出去的响应在日志里永远是空的
mutate("WriteString 写的响应也截得下来", "xgin/middleware/log.go", "./xgin", "TestLog",
       swap('''\tif w.capture && w.buf.Len() < maxResponseBody {
\t\tw.buf.WriteString(s[:min(len(s), maxResponseBody-w.buf.Len())])
\t}''', '\tif false {\n\t}'))
# gin 的默认是全都信，于是任何人发一个 X-Forwarded-For 就能决定
# 访问日志里的 client_ip。设置出错时必须退到安全的那一侧
mutate("代理网段设不上时退回谁都不信", "xgin/xgin.go", "./xgin", "TestApplyConfig|TestBuild_ForwardedHeadersOnlyWithProxiesConfigured",
       swap('\t\t_ = e.SetTrustedProxies([]string{})', ''))

section("链路")
# 用了 xgin 就有链路，使用者不用记得另外 import xtrace。摘掉这一行，
# 全局的 TracerProvider 就一直是 noop：Span 什么都不记、日志没有 trace_id，而且没有任何报错
mutate("用了 xgin 不另外 import xtrace 也有链路", "xgin/xgin.go", "./xgin", "TestTracingWorksWithoutImportingXtrace",
       swap('\t_ "github.com/xiaoshicae/xone/xtrace"\n', ''))

section("访问日志的字段")
# slog 的 JSON 把 Duration 写成纳秒整数：字段叫 elapsed_ms、值却是纳秒的话，照毫秒配的告警差出一百万倍
mutate("访问日志的耗时是毫秒", "xgin/middleware/log.go", "./xgin", "TestLog_ElapsedIsMilliseconds",
       swap('slog.Float64("elapsed_ms", millis(elapsed))', 'slog.Float64("elapsed_ms", float64(elapsed))'))
# 没匹配上路由时填真实路径，日志里分不出 /nope 是路由还是 404，也和指标、Span 对不上
# gin 在没写响应体时 Size() 返回 -1：原样记下来，204 看着像出了错，按 bytes_out 求和还会少算
# gin 默认的 404 / 405 正文在中间件链之后才写，不在链里写的话 bytes_out 记成 0
mutate("默认的 404 / 405 在链里写", "xgin/xgin.go", "./xgin", "TestDefault404And405",
       swap('\t\te.NoRoute(notFound)\n\t\te.NoMethod(methodNotAllowed)\n', ''))
mutate("没写响应体时 bytes_out 记 0", "xgin/middleware/log.go", "./xgin", "TestLog_BytesOut",
       swap('slog.Int("bytes_out", max(c.Writer.Size(), 0))', 'slog.Int("bytes_out", c.Writer.Size())'))
# 取值写在 routeOf 里，三个中间件共用；这一条改坏它本身，下面三条各打一个调用点
mutate("没匹配上的路由记 unmatched", "xgin/middleware/middleware.go", "./xgin",
       "TestLog_UnmatchedRouteIsUnmatched|TestMetric_UnmatchedRouteUsesFixedValue|TestTrace_UnmatchedRouteUsesFixedValue",
       swap('\treturn "unmatched"\n', '\treturn c.Request.URL.Path\n'))
mutate("访问日志里没匹配上的路由记 unmatched", "xgin/middleware/log.go", "./xgin", "TestLog_UnmatchedRouteIsUnmatched",
       swap('slog.String("route", routeOf(c))', 'slog.String("route", c.FullPath())'))
mutate("指标里没匹配上的路由记 unmatched", "xgin/middleware/metric.go", "./xgin", "TestMetric_UnmatchedRouteUsesFixedValue",
       swap('route, method, code := routeOf(c),', 'route, method, code := c.Request.URL.Path,'))
mutate("Span 里没匹配上的路由记 unmatched", "xgin/middleware/trace.go", "./xgin", "TestTrace_UnmatchedRouteUsesFixedValue",
       swap('\t\troute := routeOf(c)\n', '\t\troute := c.Request.URL.Path\n'))

