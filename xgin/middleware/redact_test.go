package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const secret = "hunter2"

// mustNotLeak 断言脱敏结果里不含密码
func mustNotLeak(t *testing.T, got string) {
	t.Helper()
	if strings.Contains(got, secret) {
		t.Fatalf("密码泄漏到日志里了：%s", got)
	}
}

func TestRedactBody_JSON(t *testing.T) {
	got := RedactBody([]byte(`{"user":"alice","password":"`+secret+`"}`), "application/json")
	mustNotLeak(t, got)
	if !strings.Contains(got, `"user":"alice"`) {
		t.Errorf("非敏感字段应保留，got=%s", got)
	}
}

func TestRedactBody_JSONEscapedFieldNames(t *testing.T) {
	// 回归用例。JSON 允许在字符串里写 \uXXXX，于是 password 可以写成
	// \u0070assword，原始字节里根本没有这个子串。
	// 上一版只做字面扫描，这种 body 会走快路径原样进日志——密码明文落盘。
	cases := map[string]string{
		"首字母转义":    `{"` + esc('p') + `assword":"` + secret + `"}`,
		"中间转义":     `{"pass` + esc('w') + `ord":"` + secret + `"}`,
		"多个字符加大小写": `{"` + esc('P') + esc('A') + `SSWORD":"` + secret + `"}`,
		"嵌套":       `{"a":{"` + esc('t') + `oken":"` + secret + `"}}`,
		"数组里":      `[{"` + esc('s') + `ecret":"` + secret + `"}]`,
	}
	for name, body := range cases {
		// 先确认这个用例真的绕过了字面扫描，否则它什么也没测到
		if strings.Contains(strings.ToLower(body), "password") ||
			strings.Contains(strings.ToLower(body), "token") ||
			strings.Contains(strings.ToLower(body), "secret") {
			t.Fatalf("%s：用例里仍然出现了字段名字面量，说明转义在传输途中被还原了：%s", name, body)
		}
		got := RedactBody([]byte(body), "application/json")
		mustNotLeak(t, got)
		if !strings.Contains(got, Redacted) {
			t.Errorf("%s 应当被遮掉，body=%s got=%s", name, body, got)
		}
	}
}

func TestRedactBody_JSONParseFailureMasksAll(t *testing.T) {
	// 字面扫描认为里面有敏感字段名，但解析不了——定位不了就不能放行
	got := RedactBody([]byte(`{"password": "`+secret), "application/json")
	mustNotLeak(t, got)
	if got != Redacted {
		t.Errorf("解析失败应整个遮掉，got=%s", got)
	}
}

func TestRedactBody_KeptAsIsWithoutSensitiveFields(t *testing.T) {
	// 快路径的意义就在这里：绝大多数请求体不含敏感字段，不该为它们付解析的代价
	body := `{"user":"alice","age":30}`
	if got := RedactBody([]byte(body), "application/json"); got != body {
		t.Errorf("不含敏感字段就该原样返回，got=%s", got)
	}
}

func TestRedactBody_CaseInsensitive(t *testing.T) {
	for _, body := range []string{
		`{"PASSWORD":"` + secret + `"}`,
		`{"PassWord":"` + secret + `"}`,
		`{"Api_Key":"` + secret + `"}`,
	} {
		mustNotLeak(t, RedactBody([]byte(body), "application/json"))
	}
}

func TestRedactBody_NestedAndArrays(t *testing.T) {
	body := `{"a":{"b":[{"token":"` + secret + `"}]},"c":"keep"}`
	got := RedactBody([]byte(body), "application/json")
	mustNotLeak(t, got)
	if !strings.Contains(got, "keep") {
		t.Errorf("非敏感内容应保留，got=%s", got)
	}
}

func TestRedactBody_MasksFieldsContainingSensitiveWord(t *testing.T) {
	// 回归用例。原先是精确匹配，new_password、client_secret、sessionToken
	// 全都原样进了日志。脱敏的失败模式是不对称的：多遮一个字段只是少点
	// 排查信息，漏遮一个就是凭证明文落盘。所以去掉大小写和分隔符之后按「含」算
	for _, key := range []string{
		"new_password", "old-password", "user.password", "client_secret",
		"sessionToken", "X-API-KEY", "api-key", "AccessToken", "user_passwd",
	} {
		body := `{"a":{"b":[{"` + key + `":"` + secret + `"}]},"keep":"yes"}`
		got := RedactBody([]byte(body), "application/json")
		mustNotLeak(t, got)
		if !strings.Contains(got, `"keep":"yes"`) {
			t.Errorf("%s：非敏感字段应保留，got=%s", key, got)
		}

		got = RedactBody([]byte(url.Values{key: {secret}, "keep": {"yes"}}.Encode()), "application/x-www-form-urlencoded")
		mustNotLeak(t, got)
		if !strings.Contains(got, "keep=yes") {
			t.Errorf("%s：表单里非敏感字段应保留，got=%s", key, got)
		}
	}
}

func TestRedactBody_ReserializeKeepsOtherValues(t *testing.T) {
	// 回归用例。原先解进 any 再 json.Marshal：大整数走 float64，
	// 12345678901234567890 变成 12345678901234567000；< > & 被写成反斜杠 u 开头的转义序列，
	// 日志里的订单号对不上、检索也搜不到原文
	body := `{"password":"` + secret + `","id":12345678901234567890,"n":0.1,"q":"a<b>&c"}`
	got := RedactBody([]byte(body), "application/json")
	mustNotLeak(t, got)
	for _, want := range []string{`"id":12345678901234567890`, `"n":0.1`, `"q":"a<b>&c"`} {
		if !strings.Contains(got, want) {
			t.Errorf("应原样保留 %s，got=%s", want, got)
		}
	}
	if strings.ContainsAny(got, "\n") {
		t.Errorf("不该带出换行，got=%q", got)
	}
}

func TestRedactBody_JSONWithTrailingDataMasksAll(t *testing.T) {
	// 逐个值解码的话第一个值解得出来，后面那一截就被当作不存在——
	// 而它可能恰好就是含凭证的那部分
	got := RedactBody([]byte(`{"a":1} {"password":"`+secret+`"}`), "application/json")
	if got != Redacted {
		t.Errorf("没法完整解析就该整个遮掉，got=%s", got)
	}
}

func TestRedactBody_Form(t *testing.T) {
	got := RedactBody([]byte("user=alice&password="+secret), "application/x-www-form-urlencoded")
	mustNotLeak(t, got)
	if !strings.Contains(got, "user=alice") {
		t.Errorf("非敏感字段应保留，got=%s", got)
	}
}

func TestRedactBody_FormPercentEncodedKeys(t *testing.T) {
	// 回归用例。表单的键是百分号编码的，p%61ssword 按字面切出来跟 password
	// 对不上，于是照样进日志。要跟服务端实际读到的键比，就得先解码
	for _, body := range []string{
		"p%61ssword=" + secret,
		"%50assword=" + secret,
		"%74oken=" + secret,
	} {
		got := RedactBody([]byte(body), "application/x-www-form-urlencoded")
		mustNotLeak(t, got)
		// url.Values.Encode 会把 marker 里的 * 百分号编码，先解码再比
		decoded, err := url.QueryUnescape(got)
		if err != nil {
			t.Fatalf("结果不是合法的 query：%v", err)
		}
		if !strings.Contains(decoded, Redacted) {
			t.Errorf("应当被遮掉，body=%s got=%s", body, got)
		}
	}
}

func TestRedactBody_FormParseFailureMasksAll(t *testing.T) {
	got := RedactBody([]byte("password=%zz"), "application/x-www-form-urlencoded")
	if got != Redacted {
		t.Errorf("解析失败应整个遮掉，got=%s", got)
	}
}

func TestRedactBody_OtherTypesStripNewlines(t *testing.T) {
	// 一条日志被撑成好几行，后面的采集和检索都会错位
	got := RedactBody([]byte("line1\nline2\r\nline3"), "text/plain")
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("应去掉换行，got=%q", got)
	}
}

func TestRedactBody_JSONVariantsAllRedacted(t *testing.T) {
	// 之前是精确匹配 application/json，于是 +json 的各种子类型、
	// text/json、以及大写写法全都绕过了脱敏 —— 密码原样进日志。
	// Content-Type 按 RFC 9110 本就是大小写不敏感的。
	body := []byte(`{"password":"hunter2"}`)
	for _, ct := range []string{
		"application/json",
		"application/json; charset=utf-8",
		"application/vnd.api+json",
		"application/problem+json",
		"application/ld+json",
		"text/json",
		"Application/JSON",
		"APPLICATION/JSON; CHARSET=UTF-8",
	} {
		got := RedactBody(body, ct)
		if strings.Contains(got, "hunter2") {
			t.Errorf("Content-Type=%q 漏遮了密码，got=%s", ct, got)
		}
	}
}

func TestRedactBody_UppercaseFormTypeRedacted(t *testing.T) {
	got := RedactBody([]byte("password=hunter2"), "Application/X-WWW-Form-Urlencoded")
	if strings.Contains(got, "hunter2") {
		t.Errorf("大写的 Content-Type 同样应脱敏，got=%s", got)
	}
}

func TestRedactBody_UnknownFormatWithSensitiveNameMasksAll(t *testing.T) {
	// text/plain、xml、没带 Content-Type 的 body 都定位不到具体字段，
	// 但「里面有没有敏感字段名」看得出来。有就整个遮掉 ——
	// 与 JSON 解析失败时是同一条规矩：定位不了就不能放行。
	for _, ct := range []string{"text/plain", "application/xml", "text/xml", ""} {
		got := RedactBody([]byte("<user><password>hunter2</password></user>"), ct)
		if got != Redacted {
			t.Errorf("Content-Type=%q 应整个遮掉，got=%s", ct, got)
		}
	}
}

func TestRedactBody_UnknownFormatWithoutSensitiveNameKept(t *testing.T) {
	// 过度遮蔽会把排查信息一起抹掉，只在确实出现敏感字段名时才动手
	const body = "just a plain note about the weather"
	if got := RedactBody([]byte(body), "text/plain"); got != body {
		t.Errorf("没有敏感字段名就该原样保留，got=%s", got)
	}
}

func TestRedactBody_BackslashInPlainTextDoesNotMaskAll(t *testing.T) {
	// 反斜杠是 JSON 的转义语法，纯文本里它就是个普通字符。
	// 照搬 JSON 那条「见到反斜杠一律可疑」的规矩，
	// 一条带 Windows 路径的日志会被整段遮掉
	const body = `open failed: C:\Users\app\data.txt`
	if got := RedactBody([]byte(body), "text/plain"); got != body {
		t.Errorf("纯文本里的反斜杠不该触发整体遮蔽，got=%s", got)
	}
}

func TestRedactBody_EmptyBody(t *testing.T) {
	if got := RedactBody(nil, "application/json"); got != "" {
		t.Errorf("空 body 应返回空串，got=%q", got)
	}
}

func TestAddSensitiveFields(t *testing.T) {
	reset := func() {
		mu.Lock()
		extraFields, fieldWords = nil, nil
		mu.Unlock()
	}
	reset()
	t.Cleanup(reset)

	body := `{"my_custom_key":"` + secret + `"}`
	if got := RedactBody([]byte(body), "application/json"); strings.Contains(got, Redacted) {
		t.Fatal("还没添加就不该被遮")
	}

	AddSensitiveFields("My_Custom_Key") // 大小写不敏感
	mustNotLeak(t, RedactBody([]byte(body), "application/json"))
}

// headerLog 把脱敏后的请求头渲染成它在日志里的样子
func headerLog(h http.Header) string {
	var buf strings.Builder
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("req", "请求头", RedactHeaders(h))
	return buf.String()
}

func TestRedactHeaders(t *testing.T) {
	h := http.Header{
		"Authorization": {"Bearer " + secret},
		"Cookie":        {"session=" + secret},
		"Content-Type":  {"application/json"},
	}
	got := headerLog(h)
	mustNotLeak(t, got)
	if !strings.Contains(got, "application/json") {
		t.Errorf("非敏感头应保留，got=%s", got)
	}
	// 原 header 不能被改动
	if h.Get("Authorization") != "Bearer "+secret {
		t.Error("不该改动传进来的 header")
	}
}

func TestRedactHeaders_LoggedAsNestedObjectNotEscapedString(t *testing.T) {
	// 交出序列化好的字符串的话，slog 会把它当普通字符串字段再转义一遍，
	// 日志里就是 "请求头":"{\"X-A\":\"1\"}" —— 检索时要先解一层字符串
	got := headerLog(http.Header{"X-A": {"1"}})
	if strings.Contains(got, `\"`) {
		t.Errorf("不该出现双重转义，got=%s", got)
	}
	if !strings.Contains(got, `"请求头":{"X-A":"1"}`) {
		t.Errorf("应当是一个嵌套对象，got=%s", got)
	}
}

func TestRedactHeaders_JoinsMultiValueHeader(t *testing.T) {
	// 同一个字段名忽而是字符串忽而是数组，日志系统建索引时会直接拒收
	got := headerLog(http.Header{"X-Multi": {"a", "b"}})
	if !strings.Contains(got, `"X-Multi":"a, b"`) {
		t.Errorf("多值应拼成一个字符串，got=%s", got)
	}
}

func TestRedactHeaders_FieldsSortedByKey(t *testing.T) {
	// map 遍历顺序是随机的，不排序的话每行日志的字段顺序都不一样
	h := http.Header{"X-C": {"3"}, "X-A": {"1"}, "X-B": {"2"}}
	for i := 0; i < 20; i++ {
		got := headerLog(h)
		if !strings.Contains(got, `{"X-A":"1","X-B":"2","X-C":"3"}`) {
			t.Fatalf("字段顺序应当稳定，got=%s", got)
		}
	}
}

func TestRedactHeaders_CookieMaskedByDefault(t *testing.T) {
	// Cookie 里几乎总有会话标识，等价于凭证
	mustNotLeak(t, headerLog(http.Header{"Cookie": {"sid=" + secret}}))
	mustNotLeak(t, headerLog(http.Header{"Set-Cookie": {"sid=" + secret}}))
}

func TestRedactHeaders_MasksAddedSensitiveHeaders(t *testing.T) {
	// 名字里没有敏感词的头只能靠追加的名单：这条用例也是
	// 「按名单遮」那一段代码唯一的看守——默认名单里的头都带着敏感词
	reset := func() {
		mu.Lock()
		extraHead, headerSet = nil, nil
		mu.Unlock()
	}
	reset()
	t.Cleanup(reset)

	h := http.Header{"X-Tenant-Id": {secret}}
	if !strings.Contains(headerLog(h), secret) {
		t.Fatal("还没添加就不该被遮")
	}
	AddSensitiveHeaders("x-tenant-id")
	mustNotLeak(t, headerLog(h))
}

func TestRedactHeaders_MasksNamesContainingSensitiveWord(t *testing.T) {
	// 回归用例。默认名单漏了 Proxy-Authorization，它和 Authorization
	// 一样带凭证。名单永远列不全，所以另加一条：名字里带敏感词就遮
	for _, name := range []string{
		"Proxy-Authorization", "X-Csrf-Token", "X-Session-Id", "X-Goog-Api-Key",
		"X-Amz-Security-Token", "X-Client-Secret", "X-Hub-Signature-256",
	} {
		mustNotLeak(t, headerLog(http.Header{name: {secret}}))
	}

	// 反过来也不能遮过头：这些是排查时最常看的头，名字里只是「沾边」
	benign := http.Header{
		"User-Agent": {"ua"}, "Accept": {"a"}, "Content-Type": {"ct"}, "X-Request-Id": {"rid"},
		"Keep-Alive": {"ka"}, "Idempotency-Key": {"ik"}, "Sec-Websocket-Key": {"wk"},
		"Www-Authenticate": {"wa"}, "Traceparent": {"tp"}, "X-Forwarded-For": {"xff"},
	}
	got := headerLog(benign)
	for _, v := range []string{"ua", `"a"`, "ct", "rid", "ka", "ik", "wk", "wa", "tp", "xff"} {
		if !strings.Contains(got, v) {
			t.Errorf("%s 不该被遮，got=%s", v, got)
		}
	}
}

func TestRedactHeaders_NoLeakBetweenCalls(t *testing.T) {
	// 回归用例：早先的实现从池子里取一个 map 复用，忘了清空的话
	// 上一次请求的头会漏进下一条日志
	first := headerLog(http.Header{"X-One": {"1"}})
	second := headerLog(http.Header{"X-Two": {"2"}})
	if strings.Contains(second, "X-One") {
		t.Errorf("上一次请求的头漏到了这一次：first=%s second=%s", first, second)
	}
}

func TestMayContainField_IgnoresCaseAndSeparators(t *testing.T) {
	ws := words()
	for _, c := range []struct {
		body string
		want bool
	}{
		{"PASSWORD=1", true},
		{"PaSsWoRd", true},
		{`{"Api-Key":1}`, true}, // 分隔符夹在词中间
		{"passwor", false},      // 比词短
		{"passwerd", false},
		{"", false},
	} {
		if got := mayContainField(c.body, ws); got != c.want {
			t.Errorf("mayContainField(%q)=%v want %v", c.body, got, c.want)
		}
	}
}

func TestMayContainField_BackslashAlwaysTakesSlowPath(t *testing.T) {
	ws := words()
	if !mayContainField(`{"a":"b\\c"}`, ws) {
		t.Error("有反斜杠就该走慢路径——字面扫描在转义面前不可靠")
	}
	if mayContainField(`{"a":"b"}`, ws) {
		t.Error("没有反斜杠也没有字段名时该走快路径")
	}
}

// 证明这组用例真的在测旧实现会漏掉的东西
func TestRedact_CasesOldImplementationMissed(t *testing.T) {
	escaped := `{"` + esc('p') + `assword":"` + secret + `"}`
	encoded := "p%61ssword=" + secret

	// 旧实现一：只对原始字节做字面扫描，没有反斜杠检查
	legacyJSON := func(body []byte) bool { return sensitive(string(body), words()) }
	if legacyJSON([]byte(escaped)) {
		t.Fatal("这个用例对旧实现不成立，说明它没测到要测的东西")
	}

	// 旧实现二：按 & 和 = 切表单，不解码键
	legacyForm := func(body string) bool {
		for _, pair := range strings.Split(body, "&") {
			k, _, _ := strings.Cut(pair, "=")
			if strings.Contains(strings.ToLower(k), "password") {
				return true
			}
		}
		return false
	}
	if legacyForm(encoded) {
		t.Fatal("这个用例对旧实现不成立，说明它没测到要测的东西")
	}

	// 同样的输入，新实现都遮住了
	mustNotLeak(t, RedactBody([]byte(escaped), "application/json"))
	mustNotLeak(t, RedactBody([]byte(encoded), "application/x-www-form-urlencoded"))
}

// esc 把一个 ASCII 字符写成 JSON 的 \uXXXX 转义形式。
//
// 不直接写字面量：这份文件要经过好几层文本传输，literal 的「反斜杠 u」
// 很容易在路上被某一层解释掉。第一版就是这么栽的——用例里的
// \u0070assword 被还原成了 password，于是整组「转义绕过」的回归用例
// 测的都是普通字段名，全部通过，却什么也没证明。
func esc(c byte) string { return fmt.Sprintf("%cu%04x", 92, c) }

func TestRedactBody_UnicodeFoldedSensitiveWordMasked(t *testing.T) {
	// encoding/json 匹配字段名用的是 Unicode 大小写折叠：长 s（U+017F）
	// 折叠成 s，开尔文符号（U+212A）折叠成 k。只按 ASCII 比的话，
	// 这种 key 在预检里认不出来，body 原样进日志，而服务端读到的就是那个字段
	var bound struct{ Secret, Token string }
	body := `{"ſecret":"` + secret + `","to` + "K" + `en":"` + secret + `","备注":"你好"}`
	if err := json.Unmarshal([]byte(body), &bound); err != nil || bound.Secret != secret || bound.Token != secret {
		t.Fatalf("前提不成立：encoding/json 该把这两个 key 绑到 Secret、Token 上，got=%+v err=%v", bound, err)
	}

	got := RedactBody([]byte(body), "application/json")
	mustNotLeak(t, got)
	if !strings.Contains(got, `"备注":"你好"`) {
		t.Errorf("非敏感字段应保留，got=%s", got)
	}
	mustNotLeak(t, RedactBody([]byte("ſecret="+secret), "application/x-www-form-urlencoded"))
	if got := RedactBody([]byte("ſecret: "+secret), "text/plain"); got != Redacted {
		t.Errorf("认不出结构时有折叠后的敏感词就该整个遮掉，got=%s", got)
	}
}

func TestRedactBody_ChineseContentTakesFastPath(t *testing.T) {
	// 多字节字符要解码后再折叠，但不能因此把普通的中文 body 当成可疑
	body := `{"备注":"你好，世界"}`
	if got := RedactBody([]byte(body), "text/plain"); got != body {
		t.Errorf("没有敏感词的中文 body 该原样保留，got=%s", got)
	}
}

func TestRedactHeaders_URLHeadersStripQueryString(t *testing.T) {
	// 访问日志的 path 特意不带查询串，Referer 却带着上一个页面的完整 URL：
	// OAuth 回调的 ?code=、带在链接上的 ?token= 从这里又进了日志
	got := headerLog(http.Header{
		"Referer":        {"https://app.example.com/cb?code=" + secret + "#frag"},
		"X-Original-Url": {"/login?token=" + secret},
	})
	mustNotLeak(t, got)
	if !strings.Contains(got, `"Referer":"https://app.example.com/cb"`) || !strings.Contains(got, `"X-Original-Url":"/login"`) {
		t.Errorf("该只留下查询串之前的部分，got=%s", got)
	}
}
