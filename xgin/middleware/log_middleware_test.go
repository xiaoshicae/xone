package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
