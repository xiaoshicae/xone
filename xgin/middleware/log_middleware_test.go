package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/bytedance/mockey"
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"go.opentelemetry.io/otel/trace"
)

func TestParseRequestInfo(t *testing.T) {
	body := `{"name":"test","value":123}`
	req := httptest.NewRequest("POST", "/api/test?query=1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom-Header", "custom-value")

	info := ParseRequestInfo(req)

	if info["request_method"] != "POST" {
		t.Errorf("expected method POST, got %v", info["request_method"])
	}
	if info["request_urlPath"] != "/api/test" {
		t.Errorf("expected urlPath /api/test, got %v", info["request_urlPath"])
	}
	if info["request_contentType"] != "application/json" {
		t.Errorf("expected contentType application/json, got %v", info["request_contentType"])
	}
}

func TestParseRequestInfoFormUrlEncoded(t *testing.T) {
	body := "name=test&value=123"
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	info := ParseRequestInfo(req)

	if info["request_method"] != "POST" {
		t.Errorf("expected method POST, got %v", info["request_method"])
	}
	if info["request_contentType"] != "application/x-www-form-urlencoded" {
		t.Errorf("expected contentType application/x-www-form-urlencoded, got %v", info["request_contentType"])
	}
}

func TestParseClientIP(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "X-Forwarded-For single IP",
			headers:  map[string]string{"X-Forwarded-For": "192.168.1.1"},
			expected: "192.168.1.1",
		},
		{
			name:     "X-Forwarded-For multiple IPs",
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.1, 192.168.1.1"},
			expected: "10.0.0.1",
		},
		{
			name:     "X-Real-IP",
			headers:  map[string]string{"X-Real-IP": "172.16.0.1"},
			expected: "172.16.0.1",
		},
		{
			name:     "X-Forwarded-For takes precedence",
			headers:  map[string]string{"X-Forwarded-For": "192.168.1.1", "X-Real-IP": "172.16.0.1"},
			expected: "192.168.1.1",
		},
		{
			name:     "No IP headers - fallback to RemoteAddr",
			headers:  map[string]string{},
			expected: "192.0.2.1", // httptest.NewRequest 默认 RemoteAddr
		},
		{
			name:     "Invalid IP in X-Forwarded-For - fallback to RemoteAddr",
			headers:  map[string]string{"X-Forwarded-For": "invalid-ip"},
			expected: "192.0.2.1", // httptest.NewRequest 默认 RemoteAddr
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			result := ParseClientIP(req)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestSensitiveFieldsFilterJSON(t *testing.T) {
	body := `{"username":"john","password":"secret123","token":"abc123"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	info := ParseRequestInfoWithBody(req, []byte(body))
	bodyStr := info["request_body"].(string)

	if strings.Contains(bodyStr, "secret123") {
		t.Error("password should be filtered")
	}
	if strings.Contains(bodyStr, "abc123") {
		t.Error("token should be filtered")
	}
	if !strings.Contains(bodyStr, FilteredValue) {
		t.Error("filtered value should be present")
	}
	if !strings.Contains(bodyStr, "john") {
		t.Error("username should not be filtered")
	}
}

func TestParseRequestInfoDoesNotReadBody(t *testing.T) {
	body := `{"username":"john","password":"secret123"}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	called := 0
	req.GetBody = func() (io.ReadCloser, error) {
		called++
		return io.NopCloser(strings.NewReader(body)), nil
	}

	_ = ParseRequestInfo(req)

	if called != 0 {
		t.Errorf("expected ParseRequestInfo not to read body, called=%d", called)
	}
}

func TestSensitiveFieldsFilterJSONArray(t *testing.T) {
	body := `[{"username":"john","password":"secret123"},{"username":"jane","token":"abc123"}]`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	info := ParseRequestInfoWithBody(req, []byte(body))
	bodyStr := info["request_body"].(string)

	if strings.Contains(bodyStr, "secret123") {
		t.Error("password in array should be filtered")
	}
	if strings.Contains(bodyStr, "abc123") {
		t.Error("token in array should be filtered")
	}
	if !strings.Contains(bodyStr, FilteredValue) {
		t.Error("filtered value should be present")
	}
}

func TestSensitiveFieldsFilterFormUrlEncoded(t *testing.T) {
	body := "username=john&password=secret123&token=abc123"
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	info := ParseRequestInfoWithBody(req, []byte(body))
	bodyStr := info["request_body"].(string)

	if strings.Contains(bodyStr, "secret123") {
		t.Error("password should be filtered")
	}
	if strings.Contains(bodyStr, "abc123") {
		t.Error("token should be filtered")
	}
	if !strings.Contains(bodyStr, FilteredValue) {
		t.Error("filtered value should be present")
	}
}

func TestSensitiveHeadersFilter(t *testing.T) {
	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-Custom-Header", "custom-value")

	info := ParseRequestInfo(req)
	headerStr := info["request_header"].(string)

	if strings.Contains(headerStr, "secret-token") {
		t.Error("Authorization header value should be filtered")
	}
	if !strings.Contains(headerStr, FilteredValue) {
		t.Error("filtered value should be present in headers")
	}
	if !strings.Contains(headerStr, "custom-value") {
		t.Error("non-sensitive header should not be filtered")
	}
}

func TestCustomSensitiveFields(t *testing.T) {
	// 添加自定义敏感字段
	AddSensitiveFields("custom_secret")

	body := `{"custom_secret":"my-secret","name":"test"}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	info := ParseRequestInfoWithBody(req, []byte(body))
	bodyStr := info["request_body"].(string)

	if strings.Contains(bodyStr, "my-secret") {
		t.Error("custom_secret should be filtered")
	}
}

func TestNestedJSONSensitiveFields(t *testing.T) {
	body := `{"user":{"name":"john","password":"secret123"},"data":"test"}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	info := ParseRequestInfoWithBody(req, []byte(body))
	bodyStr := info["request_body"].(string)

	if strings.Contains(bodyStr, "secret123") {
		t.Error("nested password should be filtered")
	}
	if !strings.Contains(bodyStr, "john") {
		t.Error("nested name should not be filtered")
	}
}

func TestGetHandlerSimpleName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "github.com/example/project/handler.UserHandler.GetUser-fm",
			expected: "UserHandler.GetUser-fm",
		},
		{
			input:    "main.handleRequest",
			expected: "handleRequest",
		},
		{
			input:    "simpleHandler",
			expected: "simpleHandler",
		},
		{
			input:    "/path/to/handler.Method",
			expected: "Method",
		},
		{
			input:    "github.com/xiaoshicae/xone/v2/xgin/middleware.LogMiddleware.func1",
			expected: "LogMiddleware",
		},
		{
			input:    "main.main.func1",
			expected: "main",
		},
		{
			input:    "main.func1",
			expected: "",
		},
		{
			input:    "github.com/project/handler.Setup.func1.func2",
			expected: "Setup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := GetHandlerSimpleName(tt.input)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestToJsonString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "simple map",
			input:    map[string]string{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "http header",
			input:    http.Header{"Content-Type": []string{"application/json"}},
			expected: `{"Content-Type":["application/json"]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToJsonString(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestFilterJSONBodyInvalidJSON(t *testing.T) {
	invalidJSON := []byte("not a json")
	result := filterJSONBody(invalidJSON)
	if result != string(invalidJSON) {
		t.Error("invalid JSON should be returned as-is")
	}
}

func TestFilterFormBodyEmptyPair(t *testing.T) {
	body := "key1=value1&invalid&key2=value2"
	result := filterFormBody(body)
	if !strings.Contains(result, "invalid") {
		t.Error("invalid pair should be preserved")
	}
}

func TestContainsIgnoreCaseCaseInsensitive(t *testing.T) {
	fields := []string{"password", "TOKEN"}

	if !containsIgnoreCase("PASSWORD", fields) {
		t.Error("should match case-insensitively")
	}
	if !containsIgnoreCase("token", fields) {
		t.Error("should match case-insensitively")
	}
	if containsIgnoreCase("username", fields) {
		t.Error("should not match non-sensitive field")
	}
}

func TestFilterSensitiveBodyEmptyBody(t *testing.T) {
	result := filterSensitiveBody(nil, "application/json")
	if result != "" {
		t.Error("empty body should return empty string")
	}
}

func TestFilterSensitiveBodyUnknownContentType(t *testing.T) {
	body := "password=secret"
	result := filterSensitiveBody([]byte(body), "text/plain")
	if result != body {
		t.Error("unknown content type should return body as-is")
	}
}

func TestArrayInJSONSensitiveFields(t *testing.T) {
	body := `{"users":[{"name":"john","password":"secret1"},{"name":"jane","password":"secret2"}]}`
	req := httptest.NewRequest("POST", "/api/test", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")

	info := ParseRequestInfoWithBody(req, []byte(body))
	bodyStr := info["request_body"].(string)

	if strings.Contains(bodyStr, "secret1") || strings.Contains(bodyStr, "secret2") {
		t.Error("passwords in array should be filtered")
	}
	if !strings.Contains(bodyStr, "john") || !strings.Contains(bodyStr, "jane") {
		t.Error("names should not be filtered")
	}
}

func TestAddSensitiveHeaders(t *testing.T) {
	// 添加自定义敏感头
	AddSensitiveHeaders("X-Custom-Secret")

	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom-Secret", "my-secret-value")
	req.Header.Set("X-Normal-Header", "normal-value")

	info := ParseRequestInfo(req)
	headerStr := info["request_header"].(string)

	if strings.Contains(headerStr, "my-secret-value") {
		t.Error("X-Custom-Secret header value should be filtered")
	}
	if !strings.Contains(headerStr, "normal-value") {
		t.Error("non-sensitive header should not be filtered")
	}
}

func TestGetTraceIDFromCtx(t *testing.T) {
	// 测试没有 span 的 context
	ctx := context.Background()
	traceID := GetTraceIDFromCtx(ctx)
	if traceID != "" {
		t.Error("should return empty string when no span in context")
	}
}

func TestGetHandlerSimpleNameEmpty(t *testing.T) {
	result := GetHandlerSimpleName("")
	if result != "" {
		t.Error("empty input should return empty string")
	}
}

func TestGetHandlerSimpleNameEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "complex path",
			input:    "github.com/user/project/handler.Controller.Method",
			expected: "Controller.Method",
		},
		{
			name:     "simple with dot",
			input:    "handler.Method",
			expected: "Method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetHandlerSimpleName(tt.input)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestParseClientIPRemoteAddrWithoutPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100" // 没有端口

	result := ParseClientIP(req)
	if result != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %s", result)
	}
}

func TestGetAllSensitiveFields(t *testing.T) {
	fields := getAllSensitiveFields()

	// 验证默认敏感字段存在
	found := false
	for _, f := range fields {
		if f == "password" {
			found = true
			break
		}
	}
	if !found {
		t.Error("default sensitive field 'password' should be present")
	}
}

func TestGetAllSensitiveHeaders(t *testing.T) {
	headers := getAllSensitiveHeaders()

	// 验证默认敏感头存在
	found := false
	for _, h := range headers {
		if h == "Authorization" {
			found = true
			break
		}
	}
	if !found {
		t.Error("default sensitive header 'Authorization' should be present")
	}
}

func TestFilterMapSensitiveFieldsDeepNested(t *testing.T) {
	data := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"password": "deep-secret",
				"name":     "test",
			},
		},
	}

	fields := []string{"password"}
	filterMapSensitiveFields(data, fields)

	level1 := data["level1"].(map[string]interface{})
	level2 := level1["level2"].(map[string]interface{})

	if level2["password"] != FilteredValue {
		t.Error("deep nested password should be filtered")
	}
	if level2["name"] != "test" {
		t.Error("non-sensitive field should not be filtered")
	}
}

func TestShouldSkipLog(t *testing.T) {
	// 预处理 skipPaths（与 LogMiddleware 中的逻辑一致）
	exactSkip := map[string]bool{"/health": true, "/metrics": true}
	prefixSkip := []string{"/api/v1/"}

	tests := []struct {
		path     string
		expected bool
	}{
		{"/health", true},        // 精确匹配
		{"/health/live", false},  // /health 不是前缀匹配
		{"/metrics", true},       // 精确匹配
		{"/api/v1/users", true},  // 前缀匹配
		{"/api/v1/orders", true}, // 前缀匹配
		{"/api/v2/users", false}, // 不匹配
		{"/other", false},        // 不匹配
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := shouldSkipLog(tt.path, exactSkip, prefixSkip)
			if result != tt.expected {
				t.Errorf("shouldSkipLog(%s) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestWithSkipPaths(t *testing.T) {
	opts := &LogOptions{}
	WithSkipPaths("/health", "/metrics")(opts)

	if len(opts.SkipPaths) != 2 {
		t.Errorf("expected 2 skip paths, got %d", len(opts.SkipPaths))
	}
	if opts.SkipPaths[0] != "/health" {
		t.Errorf("expected /health, got %s", opts.SkipPaths[0])
	}
}

// ==================== LogMiddleware 集成测试 ====================

func TestLogMiddleware_NormalRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LogMiddleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLogMiddleware_WithBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LogMiddleware())
	r.POST("/api/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	body := `{"username":"test","password":"secret"}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLogMiddleware_SkipPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LogMiddleware(WithSkipPaths("/health")))
	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLogMiddleware_NonTextResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LogMiddleware())
	r.GET("/binary", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/octet-stream", []byte{0x00, 0x01, 0x02})
	})

	req := httptest.NewRequest("GET", "/binary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLogMiddleware_NoRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LogMiddleware())

	req := httptest.NewRequest("GET", "/not-found", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ==================== responseBodyWriter.Write 测试 ====================

func TestResponseBodyWriter_Write(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	rbw := &responseBodyWriter{
		ResponseWriter: c.Writer,
		body:           &bytes.Buffer{},
		captureBody:    true,
	}

	data := []byte("hello world")
	n, err := rbw.Write(data)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected %d bytes written, got %d", len(data), n)
	}
	if rbw.body.String() != "hello world" {
		t.Errorf("expected body 'hello world', got '%s'", rbw.body.String())
	}
}

func TestResponseBodyWriter_Write_ExceedsLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	rbw := &responseBodyWriter{
		ResponseWriter: c.Writer,
		body:           &bytes.Buffer{},
		captureBody:    true,
	}

	// 写入超过 maxResponseBodyCapture 的数据
	bigData := make([]byte, maxResponseBodyCapture+1)
	for i := range bigData {
		bigData[i] = 'x'
	}
	_, err := rbw.Write(bigData)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	// body 不应被捕获（超过限制）
	if rbw.body.Len() != 0 {
		t.Errorf("expected empty body buffer, got %d bytes", rbw.body.Len())
	}
}

func TestResponseBodyWriter_Write_NoCaptureBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	rbw := &responseBodyWriter{
		ResponseWriter: c.Writer,
		body:           &bytes.Buffer{},
		captureBody:    false,
	}

	data := []byte("hello")
	_, err := rbw.Write(data)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if rbw.body.Len() != 0 {
		t.Errorf("expected empty body when captureBody=false, got %d bytes", rbw.body.Len())
	}
}

// ==================== isTextContentType 测试 ====================

func TestIsTextContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"text/html", true},
		{"text/plain", true},
		{"application/xml", true},
		{"application/javascript", true},
		{"APPLICATION/JSON", true},
		{"application/octet-stream", false},
		{"multipart/form-data", false},
		{"image/png", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := isTextContentType(tt.contentType)
			if result != tt.expected {
				t.Errorf("isTextContentType(%q) = %v, want %v", tt.contentType, result, tt.expected)
			}
		})
	}
}

// ==================== formatElapsed 测试 ====================

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		suffix   string
	}{
		{"microseconds", 500 * time.Microsecond, "us"},
		{"milliseconds", 50 * time.Millisecond, "ms"},
		{"seconds", 2 * time.Second, "s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatElapsed(tt.duration)
			if !strings.HasSuffix(result, tt.suffix) {
				t.Errorf("formatElapsed(%v) = %s, want suffix %s", tt.duration, result, tt.suffix)
			}
		})
	}
}

// ==================== getBodySnapshot 测试 ====================

func TestGetBodySnapshot_NilRequest(t *testing.T) {
	result := getBodySnapshot(nil)
	if result != nil {
		t.Error("nil request should return nil")
	}
}

func TestGetBodySnapshot_NoBody(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", http.NoBody)
	result := getBodySnapshot(req)
	if result != nil {
		t.Error("NoBody should return nil")
	}
}

func TestGetBodySnapshot_NilBody(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.Body = nil
	result := getBodySnapshot(req)
	if result != nil {
		t.Error("nil body should return nil")
	}
}

func TestGetBodySnapshot_MultipartFormData(t *testing.T) {
	req := httptest.NewRequest("POST", "/upload", strings.NewReader("file data"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")

	result := getBodySnapshot(req)
	if string(result) != "[multipart/form-data body omitted]" {
		t.Errorf("expected multipart omitted message, got %s", string(result))
	}
}

func TestGetBodySnapshot_OctetStream(t *testing.T) {
	req := httptest.NewRequest("POST", "/upload", strings.NewReader("binary data"))
	req.Header.Set("Content-Type", "application/octet-stream")

	result := getBodySnapshot(req)
	if string(result) != "[binary body omitted]" {
		t.Errorf("expected binary omitted message, got %s", string(result))
	}
}

func TestGetBodySnapshot_WithGetBody(t *testing.T) {
	bodyContent := `{"key":"value"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(bodyContent))
	req.Header.Set("Content-Type", "application/json")
	// httptest.NewRequest 不会自动设置 GetBody，需要显式设置
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(bodyContent)), nil
	}

	result := getBodySnapshot(req)
	if string(result) != bodyContent {
		t.Errorf("expected %s, got %s", bodyContent, string(result))
	}
}

func TestGetBodySnapshot_WithoutGetBody(t *testing.T) {
	bodyContent := `{"key":"value"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(bodyContent))
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = nil // 清除 GetBody，走降级路径

	result := getBodySnapshot(req)
	if string(result) != bodyContent {
		t.Errorf("expected %s, got %s", bodyContent, string(result))
	}

	// 验证 body 被重新包装
	if req.Body == nil {
		t.Error("body should be re-wrapped")
	}
	if req.GetBody == nil {
		t.Error("GetBody should be set")
	}

	// 验证 GetBody 可以多次获取
	body, err := req.GetBody()
	if err != nil {
		t.Fatalf("GetBody returned error: %v", err)
	}
	data, _ := io.ReadAll(body)
	if string(data) != bodyContent {
		t.Errorf("GetBody should return original body, got %s", string(data))
	}
}

func TestGetBodySnapshot_GetBodyError(t *testing.T) {
	bodyContent := `{"key":"value"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(bodyContent))
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = func() (io.ReadCloser, error) {
		return nil, errors.New("get body error")
	}

	// GetBody 失败时，降级为直接读取 Body
	result := getBodySnapshot(req)
	if string(result) != bodyContent {
		t.Errorf("expected %s, got %s", bodyContent, string(result))
	}
}

func TestGetBodySnapshot_ReadError(t *testing.T) {
	req := httptest.NewRequest("POST", "/test", &errorReader{})
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = nil

	result := getBodySnapshot(req)
	if result != nil {
		t.Error("read error should return nil")
	}
}

// errorReader 用于模拟读取错误
type errorReader struct{}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func (r *errorReader) Close() error {
	return nil
}

// ==================== GetTraceIDFromCtx 补充测试 ====================

func TestGetTraceIDFromCtx_ValidSpan(t *testing.T) {
	// 创建一个有效的 span context
	traceID := trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	spanID := trace.SpanID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	result := GetTraceIDFromCtx(ctx)
	if result != traceID.String() {
		t.Errorf("expected trace ID %s, got %s", traceID.String(), result)
	}
}

// ==================== isAnonymousFuncName 补充测试 ====================

func TestIsAnonymousFuncName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"func1", "func1", true},
		{"func12", "func12", true},
		{"func - exact 4 chars", "func", false},
		{"empty", "", false},
		{"not func", "handler", false},
		{"func with letter", "funcA", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAnonymousFuncName(tt.input)
			if result != tt.expected {
				t.Errorf("isAnonymousFuncName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// ==================== LogMiddleware 前缀跳过测试 ====================

func TestLogMiddleware_PrefixSkipPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 使用以 "/" 结尾的路径，触发 prefixSkip 分支
	r.Use(LogMiddleware(WithSkipPaths("/api/internal/")))
	r.GET("/api/internal/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/api/internal/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ==================== filterJSONBody json.Marshal 错误测试 ====================

func TestFilterJSONBody_MarshalError(t *testing.T) {
	PatchConvey("TestFilterJSONBody_MarshalError", t, func() {
		Mock(json.Marshal).Return(nil, errors.New("marshal error")).Build()

		body := []byte(`{"name":"test"}`)
		result := filterJSONBody(body)
		// json.Marshal 失败时，回退为去换行后的原始字符串
		So(result, ShouldEqual, `{"name":"test"}`)
	})
}
