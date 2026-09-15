package middleware

// 敏感信息脱敏：字段名/请求头的配置与缓存、body 与 header 的过滤
//
// 从 log_middleware.go 拆出——那个文件已经超过项目规定的 500 行上限，
// 而脱敏是一组自洽的逻辑：外部只通过 AddSensitiveFields / AddSensitiveHeaders
// 配置，通过 filterSensitiveBody / marshalFilteredHeaders 使用。

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

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
	// cachedFieldFirstByte 敏感字段名首字母（含大小写两种形态）的位图
	// body 预检时绝大多数字节都不在其中，一次数组查表即可跳过
	cachedFieldFirstByte [256]bool
)

// newlineReplacer 复用的换行符替换器，避免每请求创建新实例
var newlineReplacer = strings.NewReplacer("\r\n", "", "\r", "", "\n", "")

// 默认敏感字段列表（用于 request body）
var defaultSensitiveFields = []string{
	"password", "token", "secret", "authorization",
	"api_key", "apikey", "access_token", "refresh_token",
}

// 默认敏感头列表（用于 request header）
var defaultSensitiveHeaders = []string{
	"Authorization", "X-Api-Key", "X-Auth-Token",
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
	var first [256]bool
	for i, f := range all {
		lower := strings.ToLower(f)
		m[lower] = true
		fb[i] = []byte(lower)
		if len(lower) > 0 {
			c := lower[0]
			first[c] = true
			first[upperASCII(c)] = true
		}
	}
	cachedFieldMap = m
	cachedFieldBytes = fb
	cachedFieldFirstByte = first
}

// lowerASCII / upperASCII 只处理 ASCII 字母
//
// 敏感字段名都是 ASCII，不必为此走 unicode 表
func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func upperASCII(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
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

// getSensitiveFieldFirstByte 获取敏感字段首字节位图（lazy 初始化，线程安全）
func getSensitiveFieldFirstByte() [256]bool {
	sensitiveMu.RLock()
	fb := cachedFieldBytes
	first := cachedFieldFirstByte
	sensitiveMu.RUnlock()
	if fb != nil {
		return first
	}

	sensitiveMu.Lock()
	defer sensitiveMu.Unlock()
	if cachedFieldBytes == nil {
		rebuildFieldsCache()
	}
	return cachedFieldFirstByte
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
//
// 预检存在的意义是跳过「JSON 解析 + 递归遍历 + 重新序列化」这条重路径，
// 绝大多数请求体里并没有敏感字段。
//
// 不用 bytes.ToLower(body)：那会为每个请求复制一份完整的 body——
// 64KB 的请求体就是 64KB 的临时分配，只为做一次大小写不敏感的比较。
// 改成单趟扫描：先查首字节位图（一次数组访问），命中了才逐个字段做折叠比较，
// 因此正常 body 上摊销下来是 O(n) 且零分配。
func bodyMayContainSensitiveField(body []byte, fieldBytes [][]byte) bool {
	firstByte := getSensitiveFieldFirstByte()
	for i := 0; i < len(body); i++ {
		if !firstByte[body[i]] {
			continue
		}
		for _, fb := range fieldBytes {
			if hasPrefixFold(body[i:], fb) {
				return true
			}
		}
	}
	return false
}

// hasPrefixFold 判断 b 是否以 lowerPrefix 开头，比较时忽略 ASCII 大小写
//
// lowerPrefix 必须已是小写（由 rebuildFieldsCache 保证）。
func hasPrefixFold(b, lowerPrefix []byte) bool {
	if len(b) < len(lowerPrefix) {
		return false
	}
	for i, c := range lowerPrefix {
		if lowerASCII(b[i]) != c {
			return false
		}
	}
	return true
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

// filteredValues 脱敏后的占位值，所有被过滤的头共用
//
// 内容恒定且只读，没有必要每个敏感头都新建一个单元素切片
var filteredValues = []string{FilteredValue}

// headerPool 复用脱敏用的中间 header map
//
// 这个 map 唯一的用途就是立刻被 json.Marshal 成字符串，
// 每请求为它分配一次纯属浪费——而请求头脱敏一次请求要做两遍（请求 + 响应）
var headerPool = sync.Pool{
	New: func() any { return make(http.Header, 16) },
}

// marshalFilteredHeaders 过滤敏感头并直接序列化为 JSON 字符串
//
// 不手写序列化：encoding/json 默认会做 HTML 转义（< 变成 \u003c）、并按 key 排序，
// 手写极易在这些细节上与它产生差异，而这里省下的只是一次反射调用。
// 改为复用中间 map，输出与原来逐字节一致。
func marshalFilteredHeaders(header http.Header) string {
	headerMap := getSensitiveHeaderMap()

	filtered := headerPool.Get().(http.Header)
	for key, values := range header {
		if headerMap[strings.ToLower(key)] {
			filtered[key] = filteredValues
		} else {
			filtered[key] = values
		}
	}

	out := ToJsonString(filtered)

	// 归还前清空：values 是对原 header 切片的引用，留着会让它们无法回收
	clear(filtered)
	headerPool.Put(filtered)
	return out
}
