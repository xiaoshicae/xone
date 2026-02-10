package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
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
		return fmt.Sprintf("%.2fus", float64(d.Microseconds()))
	}
	if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000.0)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
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
	// SensitiveFields 敏感字段列表，支持用户自定义追加
	sensitiveFields = make([]string, 0)
	// SensitiveHeaders 敏感头列表，支持用户自定义追加
	sensitiveHeaders = make([]string, 0)
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
}

// AddSensitiveHeaders 添加自定义敏感头（线程安全）
func AddSensitiveHeaders(headers ...string) {
	sensitiveMu.Lock()
	defer sensitiveMu.Unlock()
	sensitiveHeaders = append(sensitiveHeaders, headers...)
}

// LogMiddleware 请求日志中间件
// 使用示例：LogMiddleware(WithSkipPaths("/health", "/metrics", "/api/internal/"))
func LogMiddleware(opts ...LogOption) gin.HandlerFunc {
	// 应用配置
	options := &LogOptions{}
	for _, opt := range opts {
		opt(options)
	}

	return func(c *gin.Context) {
		// 检查是否跳过日志记录
		if shouldSkipLog(c.Request.URL.Path, options.SkipPaths) {
			c.Next()
			return
		}

		begin := time.Now()

		// 在处理前读取请求 body（此时 body 还可读）
		bodyBytes := getBodySnapshot(c.Request)

		// 包装 ResponseWriter 以捕获响应 body
		rbw := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
			captureBody:    true,
		}
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
func shouldSkipLog(path string, skipPaths []string) bool {
	for _, skip := range skipPaths {
		// 前缀匹配（以 / 结尾）或精确匹配
		if strings.HasSuffix(skip, "/") {
			if strings.HasPrefix(path, skip) {
				return true
			}
		} else {
			if path == skip {
				return true
			}
		}
	}
	return false
}

func ParseRequestInfo(req *http.Request) map[string]interface{} {
	return parseRequestInfo(req, nil)
}

func ParseRequestInfoWithBody(req *http.Request, bodyBytes []byte) map[string]interface{} {
	return parseRequestInfo(req, bodyBytes)
}

func parseRequestInfo(req *http.Request, bodyBytes []byte) map[string]interface{} {
	contentType := req.Header.Get("Content-Type")

	body := strings.NewReplacer("\r\n", "", "\r", "", "\n", "").Replace(string(bodyBytes))

	// 对 body 进行敏感字段过滤
	body = filterSensitiveBody(body, contentType)

	// 对 header 进行敏感字段过滤
	filteredHeader := filterSensitiveHeaders(req.Header)

	return map[string]interface{}{
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

func ToJsonString(v interface{}) string {
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

// getAllSensitiveFields 获取所有敏感字段（默认 + 用户自定义）
func getAllSensitiveFields() []string {
	sensitiveMu.RLock()
	defer sensitiveMu.RUnlock()
	result := make([]string, 0, len(defaultSensitiveFields)+len(sensitiveFields))
	result = append(result, defaultSensitiveFields...)
	result = append(result, sensitiveFields...)
	return result
}

// getAllSensitiveHeaders 获取所有敏感头（默认 + 用户自定义）
func getAllSensitiveHeaders() []string {
	sensitiveMu.RLock()
	defer sensitiveMu.RUnlock()
	result := make([]string, 0, len(defaultSensitiveHeaders)+len(sensitiveHeaders))
	result = append(result, defaultSensitiveHeaders...)
	result = append(result, sensitiveHeaders...)
	return result
}

// filterSensitiveBody 过滤 body 中的敏感字段
func filterSensitiveBody(body, contentType string) string {
	if body == "" {
		return body
	}

	// 处理 JSON 格式的 body
	if strings.Contains(contentType, "application/json") {
		return filterJSONBody(body)
	}

	// 处理 form-urlencoded 格式的 body
	if strings.Contains(contentType, "x-www-form-urlencoded") {
		return filterFormBody(body)
	}

	return body
}

// filterJSONBody 过滤 JSON 格式 body 中的敏感字段
func filterJSONBody(body string) string {
	var data interface{}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return body
	}

	// 提前获取敏感字段列表，避免递归时重复调用
	fields := getAllSensitiveFields()
	filterAnySensitiveFields(data, fields)

	result, err := json.Marshal(data)
	if err != nil {
		return body
	}
	return string(result)
}

func filterAnySensitiveFields(data interface{}, fields []string) {
	switch v := data.(type) {
	case map[string]interface{}:
		filterMapSensitiveFields(v, fields)
	case []interface{}:
		for _, item := range v {
			filterAnySensitiveFields(item, fields)
		}
	}
}

// filterMapSensitiveFields 递归过滤 map 中的敏感字段
func filterMapSensitiveFields(data map[string]interface{}, fields []string) {
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
	needleLower := strings.ToLower(needle)
	for _, item := range haystack {
		if strings.ToLower(item) == needleLower {
			return true
		}
	}
	return false
}
