package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

const (
	FilteredValue = "***FILTERED***"

	// maxRequestBodySize 请求 body 读取上限 256KB
	maxRequestBodySize = 256 * 1024

	// maxResponseBodyCapture 响应 body 捕获上限 4KB
	maxResponseBodyCapture = 4 * 1024
)

// newlineReplacer 复用的换行符替换器，避免每请求创建新实例
var newlineReplacer = strings.NewReplacer("\r\n", "", "\r", "", "\n", "")

// responseBodyWriter 包装 gin.ResponseWriter，捕获响应 body（仅文本类型且 <= 4KB）
type responseBodyWriter struct {
	gin.ResponseWriter
	body        *bytes.Buffer
	captureBody bool
}

func (w *responseBodyWriter) Write(b []byte) (int, error) {
	if w.captureBody && w.body.Len()+len(b) <= maxResponseBodyCapture {
		w.body.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// isTextContentType 判断是否为文本类型的 Content-Type
func isTextContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "text/") ||
		strings.Contains(ct, "application/xml") ||
		strings.Contains(ct, "application/javascript")
}

// formatElapsed 将耗时格式化为人性化字符串
func formatElapsed(d time.Duration) string {
	if d < time.Millisecond {
		return strconv.FormatFloat(float64(d.Microseconds()), 'f', 2, 64) + "us"
	}
	if d < time.Second {
		return strconv.FormatFloat(float64(d.Microseconds())/1000.0, 'f', 2, 64) + "ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 2, 64) + "s"
}

// 默认敏感字段列表（用于 request body）
var defaultSensitiveFields = []string{
	"password", "token", "secret", "authorization",
	"api_key", "apikey", "access_token", "refresh_token",
}

// 默认敏感头列表（用于 request header）
var defaultSensitiveHeaders = []string{
	"Authorization", "X-Api-Key", "X-Auth-Token",
}

var (
	// sensitiveMu 保护敏感字段列表的读写
	sensitiveMu sync.RWMutex
	// sensitiveFields 敏感字段列表，支持用户自定义追加
	sensitiveFields = make([]string, 0)
	// sensitiveHeaders 敏感头列表，支持用户自定义追加
	sensitiveHeaders = make([]string, 0)
	// cachedFieldMap lowercase key → true，用于 O(1) 敏感字段查找
	cachedFieldMap map[string]bool
	// cachedFieldBytes 敏感字段名的小写字节切片，用于 body 快速预检
	cachedFieldBytes [][]byte
	// cachedHeaderMap lowercase key → true，用于 O(1) 敏感头查找
	cachedHeaderMap map[string]bool
)

// LogOptions 日志中间件配置
type LogOptions struct {
	SkipPaths []string // 忽略日志记录的路由列表
}

// LogOption 配置函数类型
type LogOption func(*LogOptions)

// WithSkipPaths 设置忽略日志记录的路由
// 支持精确匹配和前缀匹配，例如：
//   - "/health" 精确匹配 /health
//   - "/health/" 前缀匹配 /health/live, /health/ready 等
func WithSkipPaths(paths ...string) LogOption {
	return func(o *LogOptions) {
		o.SkipPaths = append(o.SkipPaths, paths...)
	}
}

// AddSensitiveFields 添加自定义敏感字段（线程安全）
func AddSensitiveFields(fields ...string) {
	sensitiveMu.Lock()
	defer sensitiveMu.Unlock()
	sensitiveFields = append(sensitiveFields, fields...)
	rebuildFieldsCache()
}

// AddSensitiveHeaders 添加自定义敏感头（线程安全）
func AddSensitiveHeaders(headers ...string) {
	sensitiveMu.Lock()
	defer sensitiveMu.Unlock()
	sensitiveHeaders = append(sensitiveHeaders, headers...)
	rebuildHeadersCache()
}

// rebuildFieldsCache 重建敏感字段缓存（调用方已持有写锁）
func rebuildFieldsCache() {
	all := make([]string, 0, len(defaultSensitiveFields)+len(sensitiveFields))
	all = append(all, defaultSensitiveFields...)
	all = append(all, sensitiveFields...)

	m := make(map[string]bool, len(all))
	fb := make([][]byte, len(all))
	for i, f := range all {
		lower := strings.ToLower(f)
		m[lower] = true
		fb[i] = []byte(lower)
	}
	cachedFieldMap = m
	cachedFieldBytes = fb
}

// rebuildHeadersCache 重建敏感头缓存（调用方已持有写锁）
func rebuildHeadersCache() {
	all := make([]string, 0, len(defaultSensitiveHeaders)+len(sensitiveHeaders))
	all = append(all, defaultSensitiveHeaders...)
	all = append(all, sensitiveHeaders...)

	m := make(map[string]bool, len(all))
	for _, h := range all {
		m[strings.ToLower(h)] = true
	}
	cachedHeaderMap = m
}

// rbwPool 复用 responseBodyWriter，避免每请求分配
var rbwPool = sync.Pool{
	New: func() any {
		return &responseBodyWriter{body: &bytes.Buffer{}}
	},
}

// LogMiddleware 请求日志中间件
// 使用示例：LogMiddleware(WithSkipPaths("/health", "/metrics", "/api/internal/"))
func LogMiddleware(opts ...LogOption) gin.HandlerFunc {
	// 应用配置
	options := &LogOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// 预处理 skipPaths：精确匹配用 map，前缀匹配用 slice
	exactSkip := make(map[string]bool)
	prefixSkip := make([]string, 0)
	for _, p := range options.SkipPaths {
		if strings.HasSuffix(p, "/") {
			prefixSkip = append(prefixSkip, p)
		} else {
			exactSkip[p] = true
		}
	}

	return func(c *gin.Context) {
		// 检查是否跳过日志记录
		if shouldSkipLog(c.Request.URL.Path, exactSkip, prefixSkip) {
			c.Next()
			return
		}

		begin := time.Now()

		// 在处理前准备 body 快照（不消耗原始 body）
		bodyBytes, bodyBuf := getBodySnapshot(c.Request)

		// 从 pool 获取 responseBodyWriter，保存原始 writer 以便归还后恢复
		origWriter := c.Writer
		rbw := rbwPool.Get().(*responseBodyWriter)
		rbw.ResponseWriter = origWriter
		rbw.body.Reset()
		rbw.captureBody = true
		c.Writer = rbw

		// 继续处理
		c.Next()

		elapsed := time.Since(begin)

		// 如果 body 未预读，则从读取过程中捕获的 buffer 获取（拷贝一份，避免引用被后续修改）
		if bodyBytes == nil && bodyBuf != nil {
			bodyBytes = append([]byte(nil), bodyBuf.Bytes()...)
		}

		requestInfo := ParseRequestInfoWithBody(c.Request, bodyBytes)
		requestInfo["process_latency"] = elapsed.Milliseconds()
		requestInfo["process_latency_human"] = formatElapsed(elapsed)
		requestInfo["response_header"] = ToJsonString(filterSensitiveHeaders(c.Writer.Header()))
		requestInfo["response_status"] = c.Writer.Status()

		// 捕获响应 body（仅文本类型且 <= 4KB）
		respContentType := c.Writer.Header().Get("Content-Type")
		if isTextContentType(respContentType) && rbw.body.Len() > 0 && rbw.body.Len() <= maxResponseBodyCapture {
			requestInfo["response_body"] = rbw.body.String()
		}

		// 恢复原始 writer（防止外层中间件访问已归还的 rbw），然后归还 pool
		c.Writer = origWriter
		rbw.ResponseWriter = nil
		rbwPool.Put(rbw)

		// 构建日志描述：METHOD 路由 (HandlerName)
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		desc := c.Request.Method + " " + route
		if handlerName := GetHandlerSimpleName(c.HandlerName()); handlerName != "" {
			desc += " (" + handlerName + ")"
		}
		logrus.WithContext(c.Request.Context()).WithFields(requestInfo).Infof("[XGin-LogMiddleware] %s request processed.", desc)
	}
}

// shouldSkipLog 检查是否应该跳过日志记录
// exactSkip 用于精确匹配（O(1)），prefixSkip 用于前缀匹配
func shouldSkipLog(path string, exactSkip map[string]bool, prefixSkip []string) bool {
	if exactSkip[path] {
		return true
	}
	for _, prefix := range prefixSkip {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func ParseRequestInfo(req *http.Request) map[string]any {
	return parseRequestInfo(req, nil)
}

func ParseRequestInfoWithBody(req *http.Request, bodyBytes []byte) map[string]any {
	return parseRequestInfo(req, bodyBytes)
}

func parseRequestInfo(req *http.Request, bodyBytes []byte) map[string]any {
	contentType := req.Header.Get("Content-Type")

	// 对 body 进行敏感字段过滤（全程保持 []byte，减少转换）
	body := filterSensitiveBody(bodyBytes, contentType)

	// 对 header 进行敏感字段过滤
	filteredHeader := filterSensitiveHeaders(req.Header)

	return map[string]any{
		"request_method":      req.Method,
		"request_urlPath":     req.URL.Path,
		"request_uri":         req.RequestURI,
		"request_contentType": contentType,
		"request_body":        body,
		"request_header":      ToJsonString(filteredHeader),
		"request_clientIP":    ParseClientIP(req),
	}
}

// getBodySnapshot 读取 body 快照，并重新包装 req.Body 和 req.GetBody 供后续使用
type bodyCapture struct {
	rc    io.ReadCloser
	buf   *bytes.Buffer
	limit int
}

func (b *bodyCapture) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 && b.buf != nil && b.buf.Len() < b.limit {
		remain := b.limit - b.buf.Len()
		if remain > 0 {
			toWrite := n
			if toWrite > remain {
				toWrite = remain
			}
			_, _ = b.buf.Write(p[:toWrite])
		}
	}
	return n, err
}

func (b *bodyCapture) Close() error {
	return b.rc.Close()
}

// getBodySnapshot 读取 body 快照，并在必要时包装 req.Body 供后续读取时捕获
// 优先使用 GetBody 获取副本，不消耗原 Body
// 若无法获取副本，则包装 Body，在下游读取时捕获（最多 maxRequestBodySize）
func getBodySnapshot(req *http.Request) ([]byte, *bytes.Buffer) {
	if req == nil || req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}

	contentType := req.Header.Get("Content-Type")

	// 文件上传不读取 body 内容，避免性能损耗
	if strings.Contains(contentType, "multipart/form-data") {
		return []byte("[multipart/form-data body omitted]"), nil
	}

	// 二进制流不读取
	if strings.Contains(contentType, "application/octet-stream") {
		return []byte("[binary body omitted]"), nil
	}

	// 优先使用 GetBody 获取副本（不消耗原 Body）
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err == nil {
			defer body.Close()
			bodyBytes, _ := io.ReadAll(io.LimitReader(body, maxRequestBodySize))
			return bodyBytes, nil
		}
	}

	// 降级：包装 Body，在下游读取时捕获快照（不影响读取）
	buf := &bytes.Buffer{}
	req.Body = &bodyCapture{rc: req.Body, buf: buf, limit: maxRequestBodySize}
	return nil, buf
}

func ParseClientIP(req *http.Request) string {
	// 需要上游nginx配置 X-Forwarded-For 或者 X-Real-IP
	if header := req.Header.Get("X-Forwarded-For"); header != "" {
		ip := header
		if idx := strings.IndexByte(header, ','); idx >= 0 {
			ip = header[:idx]
		}
		ip = strings.TrimSpace(ip)
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	if header := req.Header.Get("X-Real-IP"); net.ParseIP(header) != nil {
		return header
	}

	// 回退到 RemoteAddr
	if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		return host
	}
	return req.RemoteAddr
}

func ToJsonString(v any) string {
	s, _ := json.Marshal(v)
	return string(s)
}

func GetTraceIDFromCtx(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span != nil && span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

func GetHandlerSimpleName(handlerName string) string {
	if handlerName == "" {
		return ""
	}

	// 提取最后一个 "/" 后的部分（去掉模块路径）
	if index := strings.LastIndex(handlerName, "/"); index >= 0 && index < len(handlerName)-1 {
		handlerName = handlerName[index+1:]
	}

	// 去掉包名前缀（第一个 "." 之前的部分）
	if index := strings.Index(handlerName, "."); index >= 0 && index < len(handlerName)-1 {
		handlerName = handlerName[index+1:]
	}

	// 去掉末尾的匿名函数后缀（如 .func1, .func1.func2）
	for {
		dotIdx := strings.LastIndex(handlerName, ".")
		if dotIdx == -1 {
			break
		}
		if isAnonymousFuncName(handlerName[dotIdx+1:]) {
			handlerName = handlerName[:dotIdx]
		} else {
			break
		}
	}

	// 如果整体就是匿名函数名（如 func1），返回空
	if isAnonymousFuncName(handlerName) {
		return ""
	}

	return handlerName
}

// isAnonymousFuncName 判断是否为 Go 匿名函数名（如 func1, func2）
func isAnonymousFuncName(name string) bool {
	if !strings.HasPrefix(name, "func") || len(name) <= 4 {
		return false
	}
	for _, c := range name[4:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// getSensitiveFieldMap 获取敏感字段 map（lazy 初始化，线程安全）
func getSensitiveFieldMap() map[string]bool {
	sensitiveMu.RLock()
	m := cachedFieldMap
	sensitiveMu.RUnlock()
	if m != nil {
		return m
	}

	sensitiveMu.Lock()
	defer sensitiveMu.Unlock()
	if cachedFieldMap == nil {
		rebuildFieldsCache()
	}
	return cachedFieldMap
}

// getSensitiveFieldBytes 获取敏感字段字节切片（lazy 初始化，线程安全）
func getSensitiveFieldBytes() [][]byte {
	sensitiveMu.RLock()
	fb := cachedFieldBytes
	sensitiveMu.RUnlock()
	if fb != nil {
		return fb
	}

	sensitiveMu.Lock()
	defer sensitiveMu.Unlock()
	if cachedFieldBytes == nil {
		rebuildFieldsCache()
	}
	return cachedFieldBytes
}

// getSensitiveHeaderMap 获取敏感头 map（lazy 初始化，线程安全）
func getSensitiveHeaderMap() map[string]bool {
	sensitiveMu.RLock()
	m := cachedHeaderMap
	sensitiveMu.RUnlock()
	if m != nil {
		return m
	}

	sensitiveMu.Lock()
	defer sensitiveMu.Unlock()
	if cachedHeaderMap == nil {
		rebuildHeadersCache()
	}
	return cachedHeaderMap
}

// filterSensitiveBody 过滤 body 中的敏感字段，入参为 []byte 避免多余转换
func filterSensitiveBody(bodyBytes []byte, contentType string) string {
	if len(bodyBytes) == 0 {
		return ""
	}

	// 处理 JSON 格式的 body
	if strings.Contains(contentType, "application/json") {
		return filterJSONBody(bodyBytes)
	}

	// 处理 form-urlencoded 格式的 body（需要字符串操作，此处转换一次）
	if strings.Contains(contentType, "x-www-form-urlencoded") {
		return filterFormBody(newlineReplacer.Replace(string(bodyBytes)))
	}

	return newlineReplacer.Replace(string(bodyBytes))
}

// filterJSONBody 过滤 JSON 格式 body 中的敏感字段，入参为 []byte 避免多余转换
func filterJSONBody(bodyBytes []byte) string {
	// 快速路径：body 中不包含任何敏感字段名时，跳过 JSON 解析
	fieldBytes := getSensitiveFieldBytes()
	if !bodyMayContainSensitiveField(bodyBytes, fieldBytes) {
		return newlineReplacer.Replace(string(bodyBytes))
	}

	var data any
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		// JSON 解析失败，回退为去换行后的字符串
		return newlineReplacer.Replace(string(bodyBytes))
	}

	fieldMap := getSensitiveFieldMap()
	filterAnySensitiveFields(data, fieldMap)

	result, err := json.Marshal(data)
	if err != nil {
		return newlineReplacer.Replace(string(bodyBytes))
	}
	return string(result)
}

// bodyMayContainSensitiveField 快速预检 body 是否可能包含敏感字段
// 使用 bytes.ToLower + bytes.Contains，比 JSON 解析+遍历+序列化 开销小得多
func bodyMayContainSensitiveField(body []byte, fieldBytes [][]byte) bool {
	lowerBody := bytes.ToLower(body)
	for _, fb := range fieldBytes {
		if bytes.Contains(lowerBody, fb) {
			return true
		}
	}
	return false
}

func filterAnySensitiveFields(data any, fieldMap map[string]bool) {
	switch v := data.(type) {
	case map[string]any:
		filterMapSensitiveFields(v, fieldMap)
	case []any:
		for _, item := range v {
			filterAnySensitiveFields(item, fieldMap)
		}
	}
}

// filterMapSensitiveFields 递归过滤 map 中的敏感字段（使用 map O(1) 查找）
func filterMapSensitiveFields(data map[string]any, fieldMap map[string]bool) {
	for key, value := range data {
		if fieldMap[strings.ToLower(key)] {
			data[key] = FilteredValue
			continue
		}
		filterAnySensitiveFields(value, fieldMap)
	}
}

// filterFormBody 过滤 form-urlencoded 格式 body 中的敏感字段
func filterFormBody(body string) string {
	fieldMap := getSensitiveFieldMap()
	pairs := strings.Split(body, "&")
	result := make([]string, 0, len(pairs))

	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			result = append(result, pair)
			continue
		}

		key := parts[0]
		if fieldMap[strings.ToLower(key)] {
			result = append(result, key+"="+FilteredValue)
		} else {
			result = append(result, pair)
		}
	}

	return strings.Join(result, "&")
}

// filterSensitiveHeaders 过滤请求头中的敏感字段
func filterSensitiveHeaders(header http.Header) http.Header {
	headerMap := getSensitiveHeaderMap()
	filtered := make(http.Header, len(header))

	for key, values := range header {
		if headerMap[strings.ToLower(key)] {
			filtered[key] = []string{FilteredValue}
		} else {
			filtered[key] = values
		}
	}

	return filtered
}
