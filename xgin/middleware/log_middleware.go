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
	// cachedAllFields 缓存合并后的敏感字段，写入时重建
	cachedAllFields []string
	// cachedAllHeaders 缓存合并后的敏感头，写入时重建
	cachedAllHeaders []string
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
	result := make([]string, 0, len(defaultSensitiveFields)+len(sensitiveFields))
	result = append(result, defaultSensitiveFields...)
	result = append(result, sensitiveFields...)
	cachedAllFields = result
}

// rebuildHeadersCache 重建敏感头缓存（调用方已持有写锁）
func rebuildHeadersCache() {
	result := make([]string, 0, len(defaultSensitiveHeaders)+len(sensitiveHeaders))
	result = append(result, defaultSensitiveHeaders...)
	result = append(result, sensitiveHeaders...)
	cachedAllHeaders = result
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

		// 在处理前读取请求 body（此时 body 还可读）
		bodyBytes := getBodySnapshot(c.Request)

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
func getBodySnapshot(req *http.Request) []byte {
	if req == nil || req.Body == nil || req.Body == http.NoBody {
		return nil
	}

	contentType := req.Header.Get("Content-Type")

	// 文件上传不读取 body 内容，避免性能损耗
	if strings.Contains(contentType, "multipart/form-data") {
		return []byte("[multipart/form-data body omitted]")
	}

	// 二进制流不读取
	if strings.Contains(contentType, "application/octet-stream") {
		return []byte("[binary body omitted]")
	}

	// 优先使用 GetBody 获取副本（不消耗原 Body）
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err == nil {
			defer body.Close()
			bodyBytes, _ := io.ReadAll(io.LimitReader(body, maxRequestBodySize))
			return bodyBytes
		}
	}

	// 降级：直接读取 Body，然后重新包装
	bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, maxRequestBodySize))
	req.Body.Close()
	if err != nil {
		return nil
	}

	// 重新包装 Body，供后续 middleware 使用
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// 设置 GetBody，支持多次获取
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}

	return bodyBytes
}

func ParseClientIP(req *http.Request) string {
	// 需要上游nginx配置 X-Forwarded-For 或者 X-Real-IP
	header := req.Header.Get("X-Forwarded-For")
	items := strings.Split(header, ",")
	if len(items) > 0 {
		ip := strings.TrimSpace(items[0])
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	header = req.Header.Get("X-Real-IP")
	if net.ParseIP(header) != nil {
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

// getAllSensitiveFields 获取所有敏感字段（默认 + 用户自定义），优先返回缓存
func getAllSensitiveFields() []string {
	sensitiveMu.RLock()
	if cachedAllFields != nil {
		result := cachedAllFields
		sensitiveMu.RUnlock()
		return result
	}
	sensitiveMu.RUnlock()

	// 首次调用，懒初始化缓存
	sensitiveMu.Lock()
	defer sensitiveMu.Unlock()
	if cachedAllFields == nil {
		rebuildFieldsCache()
	}
	return cachedAllFields
}

// getAllSensitiveHeaders 获取所有敏感头（默认 + 用户自定义），优先返回缓存
func getAllSensitiveHeaders() []string {
	sensitiveMu.RLock()
	if cachedAllHeaders != nil {
		result := cachedAllHeaders
		sensitiveMu.RUnlock()
		return result
	}
	sensitiveMu.RUnlock()

	// 首次调用，懒初始化缓存
	sensitiveMu.Lock()
	defer sensitiveMu.Unlock()
	if cachedAllHeaders == nil {
		rebuildHeadersCache()
	}
	return cachedAllHeaders
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
	var data any
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		// JSON 解析失败，回退为去换行后的字符串
		return newlineReplacer.Replace(string(bodyBytes))
	}

	// 提前获取敏感字段列表，避免递归时重复调用
	fields := getAllSensitiveFields()
	filterAnySensitiveFields(data, fields)

	result, err := json.Marshal(data)
	if err != nil {
		return newlineReplacer.Replace(string(bodyBytes))
	}
	return string(result)
}

func filterAnySensitiveFields(data any, fields []string) {
	switch v := data.(type) {
	case map[string]any:
		filterMapSensitiveFields(v, fields)
	case []any:
		for _, item := range v {
			filterAnySensitiveFields(item, fields)
		}
	}
}

// filterMapSensitiveFields 递归过滤 map 中的敏感字段
func filterMapSensitiveFields(data map[string]any, fields []string) {
	for key, value := range data {
		// 检查当前字段是否敏感
		if containsIgnoreCase(key, fields) {
			data[key] = FilteredValue
			continue
		}

		filterAnySensitiveFields(value, fields)
	}
}

// filterFormBody 过滤 form-urlencoded 格式 body 中的敏感字段
func filterFormBody(body string) string {
	fields := getAllSensitiveFields()
	pairs := strings.Split(body, "&")
	result := make([]string, 0, len(pairs))

	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			result = append(result, pair)
			continue
		}

		key := parts[0]
		if containsIgnoreCase(key, fields) {
			result = append(result, key+"="+FilteredValue)
		} else {
			result = append(result, pair)
		}
	}

	return strings.Join(result, "&")
}

// filterSensitiveHeaders 过滤请求头中的敏感字段
func filterSensitiveHeaders(header http.Header) http.Header {
	headers := getAllSensitiveHeaders()
	filtered := make(http.Header)

	for key, values := range header {
		if containsIgnoreCase(key, headers) {
			filtered[key] = []string{FilteredValue}
		} else {
			filtered[key] = values
		}
	}

	return filtered
}

// containsIgnoreCase 判断字符串是否在列表中（不区分大小写）
func containsIgnoreCase(needle string, haystack []string) bool {
	for _, item := range haystack {
		if strings.EqualFold(needle, item) {
			return true
		}
	}
	return false
}
