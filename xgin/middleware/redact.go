package middleware

// 请求/响应里的敏感信息脱敏。
//
// 这里的每一条判断都必须「拿不准就当作敏感」。脱敏逻辑的失败模式是不对称的：
// 多遮一个字段只是少一点排查信息，漏遮一个就是密码进了日志、而且没人会发现。

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Redacted 被遮掉的值统一写成这个
const Redacted = "***REDACTED***"

// 默认的敏感词。
//
// 按「含」匹配，不是按「等于」：比较前双方都去掉大小写和分隔符（_ - . 空格），
// 于是 password 能认出 new_password、old-password、user.password，
// token 能认出 sessionToken、X-Csrf-Token。原先是精确匹配，这些全都原样进了日志。
//
// 词要挑得够「专」：auth 会误中 author，key 会误中 primary_key、Idempotency-Key，
// pwd 这样的短词会在随机串里撞上、把整段纯文本 body 遮掉，所以这里写的是
// apikey、authorization 这样的整词。在这个范围之内，拿不准的词宁可放进来：
// 误遮只是少一点排查信息，漏遮就是凭证落盘。
var defaultWords = []string{
	"password", "passwd", "secret", "token", "authorization",
	"apikey", "accesskey", "privatekey", "credential", "cookie", "session", "signature",
}

// 默认按名字精确遮掉的请求头。
//
// 它们的名字里本来就带着敏感词，词表已经能认出来；单独列一遍是为了
// 不让它们的安危系于词表——哪天有人把词表收窄了，这几个仍然是遮的。
var defaultHeaders = []string{
	"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "X-Api-Key", "X-Auth-Token",
}

var (
	mu          sync.RWMutex
	extraFields []string
	extraHead   []string
	fieldWords  []string // 规范化之后的敏感词：默认词表 + AddSensitiveFields
	headerSet   map[string]bool
)

// AddSensitiveFields 追加敏感词，大小写与分隔符不敏感。
//
// 与默认词表同一条规则：字段名里含这个词就遮，body 和请求头都算。
// 比如加了 id_card，idCard、user_id_card、X-Id-Card 都会被遮掉。
func AddSensitiveFields(fields ...string) {
	mu.Lock()
	defer mu.Unlock()
	extraFields = append(extraFields, fields...)
	fieldWords = nil
}

// AddSensitiveHeaders 追加按名字精确遮掉的请求头，大小写不敏感。
//
// 用于名字里没有敏感词、内容却是凭证的头，比如 X-Tenant-Id。
// 名字里带敏感词的头不需要加，默认就遮。
func AddSensitiveHeaders(headers ...string) {
	mu.Lock()
	defer mu.Unlock()
	extraHead = append(extraHead, headers...)
	headerSet = nil
}

// words 取规范化之后的敏感词
func words() []string {
	mu.RLock()
	w := fieldWords
	mu.RUnlock()
	if w != nil {
		return w
	}

	mu.Lock()
	defer mu.Unlock()
	if fieldWords == nil {
		all := append(append([]string{}, defaultWords...), extraFields...)
		fieldWords = make([]string, 0, len(all))
		for _, f := range all {
			if n := normalize(f); n != "" {
				fieldWords = append(fieldWords, n)
			}
		}
	}
	return fieldWords
}

func headers() map[string]bool {
	mu.RLock()
	set := headerSet
	mu.RUnlock()
	if set != nil {
		return set
	}

	mu.Lock()
	defer mu.Unlock()
	if headerSet == nil {
		all := append(append([]string{}, defaultHeaders...), extraHead...)
		headerSet = make(map[string]bool, len(all))
		for _, h := range all {
			headerSet[strings.ToLower(h)] = true
		}
	}
	return headerSet
}

// isSeparator 比较字段名时忽略的字符
func isSeparator(c byte) bool { return c == '_' || c == '-' || c == '.' || c == ' ' }

// normalize 做大小写折叠并去掉分隔符：New_Password、new-password、newPassword
// 规范化之后都是 newpassword
func normalize(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r >= utf8.RuneSelf || !isSeparator(byte(r)) {
			b.WriteRune(foldRune(r))
		}
	}
	return b.String()
}

// foldRune 取 r 所在大小写折叠等价类的代表，两个字符折叠后相等，
// 当且仅当 encoding/json 认为它们是同一个字符。
//
// 不能只处理 ASCII：encoding/json 匹配字段名用的是 Unicode 折叠，
// 实测 {"ſecret":...}（长 s，U+017F）绑得上 Secret，
// {"toKen":...}（开尔文符号，U+212A）绑得上 Token。只转 ASCII 小写的话，
// 这两个 key 在词表里认不出来，body 走快路径原样进日志，
// 而服务端读到的明明就是那个字段。
//
// 代表取等价类里最小的那个字符（encoding/json 内部也是这么取的），
// 是大写 ASCII 字母的话再转成小写，好让规范化之后的词表仍然是读得懂的样子
func foldRune(r rune) rune {
	if r >= utf8.RuneSelf {
		// SimpleFold 在等价类里按升序轮转，第一次不升反降的那个就是最小的
		next := unicode.SimpleFold(r)
		for next > r {
			r, next = next, unicode.SimpleFold(next)
		}
		r = next
	}
	if 'A' <= r && r <= 'Z' {
		r += 'a' - 'A'
	}
	return r
}

// sensitive 判断字段名是否敏感：规范化之后含任一敏感词。
// body 预检也用它，把整段 body 当作一个「字段名」，两处因此只有一份规则。
//
// 代价量过：五个请求头的 RedactHeaders 从精确匹配时的 780ns/7 allocs
// 变成 1460ns/11 allocs（每个头规范化一次）。试过逐字节手写扫描来省掉
// 这次分配，反而慢得多——strings.Contains 比手写循环快得多。body 预检
// 原先就是那样写的：每个字节位置对每个词各比一次，非 ASCII 的位置每次都要
// 解码、折叠。230 字节、带几个汉字的 JSON，RedactBody 要 23µs，
// 接近上限的 256KB 要 13ms（纯 ASCII）/ 46ms（带中文）；换成规范化一次
// 再 Contains 之后是 2.4µs、2.5ms / 2.9ms，代价是多一份 body 大小的分配。
func sensitive(name string, ws []string) bool {
	n := normalize(name)
	for _, w := range ws {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// newlines 把换行去掉，免得一条日志被撑成好几行
//
// 只写单字节的 \r 和 \n：\r\n 逐个字节删掉结果一样。写成单字节，
// Replacer 走的是逐字节的实现，没有换行时原样返回、不分配；
// 多写一个 \r\n 就退回通用实现，每次都要拷两遍
var newlines = strings.NewReplacer("\r", "", "\n", "")

// RedactBody 按 Content-Type 脱敏请求体
//
// Content-Type 先转小写：这个头按 RFC 9110 是大小写不敏感的，
// 照字面比的话 "Application/JSON" 就走不进 JSON 那一支。
//
// JSON 的判断是「含 json」而不是精确匹配 application/json：
// application/vnd.api+json、application/problem+json、text/json
// 都是 JSON，精确匹配会把它们整个漏过去。认错了也不会更糟——
// 解不出 JSON 的那一支本来就是整个遮掉。
func RedactBody(body []byte, contentType string) string {
	if len(body) == 0 {
		return ""
	}
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "json"):
		return redactJSON(body)
	case strings.Contains(ct, "x-www-form-urlencoded"):
		return redactForm(string(body))
	default:
		return redactOpaque(body)
	}
}

// redactOpaque 处理认不出结构的 body（text/plain、xml、没带 Content-Type 的……）
//
// 定位不了具体字段，但「里面有没有敏感字段名」是看得出来的。有就整个遮掉。
// 这与 JSON 那一支解析失败时的处理是同一条规矩：定位不了就不能放行。
//
// 这里只做字面扫描，不像 JSON 那样见到反斜杠就一律当作可疑：
// 反斜杠是 JSON 的转义语法，纯文本里它就是个普通字符，
// 一条带 Windows 路径的日志不该因此被整段遮掉。
func redactOpaque(body []byte) string {
	s := string(body)
	if sensitive(s, words()) {
		return Redacted
	}
	return newlines.Replace(s)
}

// redactJSON 解析 JSON 并遮掉敏感字段
//
// 解码用 UseNumber、编码关掉 HTML 转义：默认的 any 会把数字解成 float64，
// 12345678901234567890 重新写出来成了 12345678901234567000；
// 默认的编码器又把 < > & 写成反斜杠 u 开头的转义序列。两样都让日志里的值跟请求里的对不上。
func redactJSON(body []byte) string {
	ws := words()
	s := string(body)
	if !mayContainField(s, ws) {
		return newlines.Replace(s)
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var data any
	if err := dec.Decode(&data); err != nil || dec.Decode(new(any)) != io.EOF {
		// 解析不了（或者后面还跟着别的东西）就整个遮掉。这里不能原样返回：
		// 能走到这一步说明字面扫描认为里面有敏感字段名，只是我们没能力定位它
		return Redacted
	}

	redactValue(data, ws)
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(data); err != nil {
		return Redacted
	}
	return strings.TrimSuffix(out.String(), "\n") // Encode 总在末尾补一个换行
}

// mayContainField 判断 body 里是否可能出现敏感字段名。
//
// 只在「确定不含」时返回 false——它的作用是跳过「解析 + 遍历 + 重新序列化」
// 这条重路径，而不是做安全判断，所以拿不准一律返回 true。
//
// 反斜杠是硬性的分水岭：JSON 允许在字符串里写 \uXXXX，于是
//
//	{"password": "hunter2"}
//
// 的原始字节里根本没有 password 这个子串。上一版只做字面扫描，
// 这种 body 会走快路径原样进日志——密码明文落盘，而且不会有任何迹象。
// 所以只要出现反斜杠就交给解析器，由它把转义还原成真实的 key。
//
// 字面扫描用的就是字段名比对的那个 sensitive：同一种折叠、同样忽略分隔符，
// api_key 对得上 apikey，{"ſecret":...} 对得上 secret。
func mayContainField(body string, ws []string) bool {
	return strings.IndexByte(body, '\\') >= 0 || sensitive(body, ws)
}

func redactValue(v any, ws []string) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if sensitive(k, ws) {
				t[k] = Redacted
				continue
			}
			redactValue(val, ws)
		}
	case []any:
		for _, item := range t {
			redactValue(item, ws)
		}
	}
}

// redactForm 脱敏 form-urlencoded 请求体
//
// 用 url.ParseQuery 而不是按 & 和 = 切：表单的键是百分号编码的，
//
//	p%61ssword=hunter2
//
// 按字面切出来的键是 p%61ssword，跟 password 对不上，于是照样进日志。
// 解析一遍再比，才是跟服务端实际读到的键同一个东西。
func redactForm(body string) string {
	ws := words()

	values, err := url.ParseQuery(body)
	if err != nil {
		// 解析不了就整个遮掉，理由同 JSON：定位不了就不能放行
		return Redacted
	}
	for k := range values {
		if sensitive(k, ws) {
			values[k] = []string{Redacted}
		}
	}
	return values.Encode()
}

// RedactHeaders 脱敏请求头，返回一个可以直接交给 slog 的值。
//
// 返回 slog.Value 而不是序列化好的字符串：交出字符串的话，slog 还要把它
// 当成一个普通字符串字段再转义一遍，日志里就成了
//
//	"请求头":"{\"Authorization\":[\"***REDACTED***\"]}"
//
// 检索时得先解一层字符串才能解 JSON。交出 slog.Value，序列化只发生一次，
// 这个字段在日志里就是一个正常的嵌套对象，顺带省掉了那次 json.Marshal
// （实测整条日志从 2777ns/19 allocs 降到 1630ns/9 allocs）。
//
// 多值的头拼成逗号分隔的一个字符串，而不是数组：同一个字段名在不同日志行里
// 忽而是字符串忽而是数组，日志系统建索引时会直接拒收。
//
// 两条规则，满足一条就遮：名字在名单里（默认名单 + AddSensitiveHeaders），
// 或者名字里带敏感词（与 body 字段同一张词表）。名单永远列不全——
// Proxy-Authorization 就曾经漏在外面——所以词表是兜底的那一层。
func RedactHeaders(h http.Header) slog.Value {
	set := headers()
	ws := words()

	// 按 key 排序：map 的遍历顺序是随机的，不排的话同样一组请求头
	// 每行日志的字段顺序都不一样，对不上也没法 diff
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	attrs := make([]slog.Attr, 0, len(keys))
	for _, k := range keys {
		name := strings.ToLower(k) // 两张名单都按小写查，转一次就够了
		if set[name] {
			attrs = append(attrs, slog.String(k, Redacted))
			continue
		}
		if sensitive(k, ws) {
			attrs = append(attrs, slog.String(k, Redacted))
			continue
		}
		v := headerValue(h[k])
		if urlHeaders[name] {
			v = stripQuery(v)
		}
		attrs = append(attrs, slog.String(k, v))
	}
	return slog.GroupValue(attrs...)
}

// urlHeaders 值是一个 URL 的请求头，记日志时去掉查询串和片段。
//
// 访问日志的 path 特意不带查询串（凭证常在那里：?token=、OAuth 回调的 ?code=），
// 而 Referer 带的正是上一个页面的完整 URL，照抄的话等于从侧门又记了一遍。
// 后面几个是反向代理转发原始请求 URI 用的，内容就是这次请求的 path 加查询串。
// Origin 按规范只有协议、主机和端口，不在此列
var urlHeaders = map[string]bool{
	"referer":         true,
	"x-original-url":  true,
	"x-original-uri":  true,
	"x-rewrite-url":   true,
	"x-forwarded-uri": true,
}

// stripQuery 去掉第一个 ? 或 # 之后的全部内容。
//
// 按字符截断而不是 url.Parse：解析失败时还得决定怎么办，截断没有失败这回事。
// 多值拼在一起时会连后面的值一起去掉——多遮一点，不会漏
func stripQuery(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		return u[:i]
	}
	return u
}

// headerValue 把一个头的多个取值拼成一个字符串
func headerValue(v []string) string {
	switch len(v) {
	case 0:
		return ""
	case 1:
		return v[0] // 绝大多数头都是单值，这一支不分配
	default:
		return strings.Join(v, ", ")
	}
}
