package xhttp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
)

// secret 一个长随机串：「日志里不该出现」的检查用它，短的数字会和时间戳、耗时撞上
func secret(t *testing.T, prefix string) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return prefix + "-" + hex.EncodeToString(b)
}

// captureLogs 把 slog 默认 logger 换成写 JSON 进 buffer 的，返回取「全部原文」和「解析后的各行」的函数
func captureLogs(t *testing.T) (raw func() string, parsed func() []map[string]any) {
	t.Helper()
	var buf strings.Builder
	w := &lockedWriter{w: &buf}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	raw = func() string {
		w.mu.Lock()
		defer w.mu.Unlock()
		return buf.String()
	}
	parsed = func() []map[string]any {
		var out []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(raw()), "\n") {
			if line == "" {
				continue
			}
			m := map[string]any{}
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("日志不是 JSON：%v，内容=%q", err, line)
			}
			out = append(out, m)
		}
		return out
	}
	return raw, parsed
}

// requestLines 取这几种消息的日志
func requestLines(all []map[string]any, msgs ...string) []map[string]any {
	var out []map[string]any
	for _, l := range all {
		for _, m := range msgs {
			if l["msg"] == m {
				out = append(out, l)
			}
		}
	}
	return out
}

var allRequestMsgs = []string{"http request", "http request failed", "slow http request"}

// logClient Log 开着、链路和指标关着的客户端；不装 newQuiet 的 discardLogger，resty 的日志照样进 slog
func logClient(t *testing.T, mutate ...func(*Config)) *resty.Client {
	t.Helper()
	c := DefaultConfig()
	c.Log, c.Trace, c.Metric = true, false, false
	c.RetryWaitTime, c.RetryMaxWaitTime = time.Millisecond, time.Millisecond
	for _, m := range mutate {
		m(&c)
	}
	client, closer, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closer.Close() })
	return client
}

// cutServer 每个请求都把连接掐断：传输层错误，resty 会重试
func cutServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLog_SuccessOneInfoLineWithoutQuery(t *testing.T) {
	token := secret(t, "token")
	srv, _ := echo(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	raw, lines := captureLogs(t)
	client := logClient(t)
	if _, err := client.R().SetHeader("Authorization", "Bearer "+token).SetBody(token).
		Post(srv.URL + "/api/items?token=" + token + "#frag"); err != nil {
		t.Fatal(err)
	}

	got := requestLines(lines(), allRequestMsgs...)
	if len(got) != 1 {
		t.Fatalf("一个请求一行，got=%v", lines())
	}
	l := got[0]
	host := strings.TrimPrefix(srv.URL, "http://")
	if l["msg"] != "http request" || l["level"] != "INFO" || l["method"] != "POST" || l["host"] != host ||
		l["path"] != "/api/items" || l["status"] != float64(200) || l["attempts"] != float64(1) {
		t.Errorf("字段不对，got=%v", l)
	}
	if _, ok := l["elapsed_ms"].(float64); !ok {
		t.Errorf("该带 elapsed_ms，got=%v", l)
	}
	if _, ok := l["error"]; ok {
		t.Errorf("成功的请求不带 error，got=%v", l)
	}
	if strings.Contains(raw(), token) {
		t.Errorf("查询串、Header、body 里的令牌进了日志：%s", raw())
	}
}

func TestLog_5xxWithRetriesConfiguredIsOneWarnLine(t *testing.T) {
	// 拿到了响应就不重试，5xx 也不重试：一次尝试，一行 WARN
	token := secret(t, "token")
	var hits atomic.Int32
	srv, _ := echo(t, func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(500) })
	raw, lines := captureLogs(t)
	client := logClient(t, func(c *Config) { c.RetryCount = 2 })
	resp, err := client.R().Get(srv.URL + "/x?token=" + token)
	if err != nil || resp.StatusCode() != 500 {
		t.Fatalf("前提：该拿到 500，got=%v %v", resp, err)
	}
	got := requestLines(lines(), allRequestMsgs...)
	if len(got) != 1 {
		t.Fatalf("一个请求一行，got=%v", lines())
	}
	if l := got[0]; l["msg"] != "http request failed" || l["level"] != "WARN" || l["status"] != float64(500) || l["attempts"] != float64(hits.Load()) {
		t.Errorf("5xx 该记 WARN 的 http request failed、带状态码和尝试次数（下游收到 %d 次），got=%v", hits.Load(), l)
	}
	if strings.Contains(raw(), token) {
		t.Errorf("令牌进了日志：%s", raw())
	}
}

func TestLog_4xxIsInfo(t *testing.T) {
	srv, _ := echo(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	_, lines := captureLogs(t)
	client := logClient(t)
	client.R().Get(srv.URL + "/missing")
	got := requestLines(lines(), allRequestMsgs...)
	if len(got) != 1 || got[0]["msg"] != "http request" || got[0]["status"] != float64(404) {
		t.Errorf("4xx 是下游的业务回答，记 INFO，got=%v", lines())
	}
}

func TestLog_TransportErrorAfterRetriesOneLineNoQuery(t *testing.T) {
	// resty v2.17.2 在所有重试结束之后只调一次 OnError，Attempt 是用掉的尝试次数
	token := secret(t, "token")
	var hits atomic.Int32
	srv := cutServer(t, &hits)
	raw, lines := captureLogs(t)
	client := logClient(t, func(c *Config) { c.RetryCount = 2 })
	u := strings.Replace(srv.URL, "http://", "http://someone:"+token+"@", 1) + "/cut?token=" + token
	if _, err := client.R().Get(u); err == nil {
		t.Fatal("前提：连接被掐断，该失败")
	}

	all := lines()
	got := requestLines(all, allRequestMsgs...)
	if len(got) != 1 {
		t.Fatalf("重试 2 次也只记一行，got=%v", all)
	}
	l := got[0]
	if l["msg"] != "http request failed" || l["level"] != "WARN" || l["status"] != float64(0) || l["attempts"] != float64(3) || l["path"] != "/cut" {
		t.Errorf("传输层错误：WARN、status 0、attempts 3，got=%v", l)
	}
	e, _ := l["error"].(string)
	if !strings.Contains(e, "/cut") || !strings.Contains(e, "EOF") {
		t.Errorf("error 该留着去掉查询串的 URL 和原因，got=%q", e)
	}
	if strings.Contains(raw(), token) || strings.Contains(raw(), "someone") {
		t.Errorf("查询串或 userinfo 从错误原文进了日志：%s", raw())
	}
	// 同一件事不说两遍：resty 自己重试路径上的 WARN（Attempt N）和 ERROR 不再打
	if n := len(requestLines(all, "xhttp resty log")); n != 0 {
		t.Errorf("Log 开着时 resty 的重试日志不该再打，got %d 行：%v", n, all)
	}
}

func TestLog_TransportErrorRefusedNoQuery(t *testing.T) {
	token := secret(t, "token")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	raw, lines := captureLogs(t)
	client := logClient(t)
	client.R().Get("http://" + addr + "/x?token=" + token)
	got := requestLines(lines(), "http request failed")
	if len(got) != 1 || !strings.Contains(fmt.Sprint(got[0]["error"]), "connection refused") {
		t.Fatalf("连不上该记一行 http request failed、带原因，got=%v", lines())
	}
	if strings.Contains(raw(), token) {
		t.Errorf("令牌进了日志：%s", raw())
	}
}

func TestLog_OffMeansNoRequestLinesAndRestyLogsUnchanged(t *testing.T) {
	var hits atomic.Int32
	srv := cutServer(t, &hits)
	_, lines := captureLogs(t)
	client := logClient(t, func(c *Config) { c.Log, c.RetryCount = false, 1 })
	client.R().Get(srv.URL)
	all := lines()
	if got := requestLines(all, allRequestMsgs...); len(got) != 0 {
		t.Errorf("Log 默认关着，不该有请求日志，got=%v", got)
	}
	// 关着时 resty 自己的重试日志照旧（每次尝试一行 WARN，用完一行 ERROR）
	if n := len(requestLines(all, "xhttp resty log")); n != 3 {
		t.Errorf("Log 关着时 resty 的重试日志照旧 3 行，got=%v", all)
	}
}

func TestLog_OtherRestyWarningsStillLogged(t *testing.T) {
	// 只挑掉重试路径那两种，resty 别的提醒照旧：比如明文 HTTP 上用 Basic Auth
	srv, _ := echo(t, nil)
	_, lines := captureLogs(t)
	client := logClient(t)
	client.R().SetBasicAuth("u", "p").Get(srv.URL)
	for _, l := range requestLines(lines(), "xhttp resty log") {
		if strings.Contains(fmt.Sprint(l["detail"]), "Basic Auth") {
			return
		}
	}
	t.Errorf("resty 的其余提醒不该被吞掉，got=%v", lines())
}

func TestLog_SlowThreshold(t *testing.T) {
	for _, c := range []struct {
		name      string
		threshold time.Duration
		wantMsg   string
	}{
		{"over threshold", 20 * time.Millisecond, "slow http request"},
		{"zero disables", 0, "http request"},
		{"under threshold", time.Second, "http request"},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv, _ := echo(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(60 * time.Millisecond) })
			_, lines := captureLogs(t)
			client := logClient(t, func(cc *Config) { cc.SlowThreshold = c.threshold })
			client.R().Get(srv.URL)
			got := requestLines(lines(), allRequestMsgs...)
			if len(got) != 1 || got[0]["msg"] != c.wantMsg {
				t.Fatalf("该是一行 %s，got=%v", c.wantMsg, lines())
			}
			if c.wantMsg == "slow http request" && (got[0]["level"] != "WARN" || got[0]["threshold_ms"] != float64(20)) {
				t.Errorf("慢请求该是 WARN、带 threshold_ms，got=%v", got[0])
			}
		})
	}
}

func TestLog_ElapsedCoversRetriesWithMetricOff(t *testing.T) {
	// 起点要在 Metric 关着时照样记下，否则耗时只是最后一次尝试
	var hits atomic.Int32
	srv := cutServer(t, &hits)
	_, lines := captureLogs(t)
	const backoff = 60 * time.Millisecond
	client := logClient(t, func(c *Config) {
		c.RetryCount, c.RetryWaitTime, c.RetryMaxWaitTime = 2, backoff, backoff
	})
	client.R().Get(srv.URL)
	got := requestLines(lines(), allRequestMsgs...)
	if len(got) != 1 {
		t.Fatalf("got=%v", lines())
	}
	// 两次退避，每次 resty 在 [wait, maxWait] 里取值，这里上下界相同
	if e, _ := got[0]["elapsed_ms"].(float64); e < float64(2*backoff/time.Millisecond)*0.9 {
		t.Errorf("耗时该算整次逻辑请求（含两次 %v 的退避），got %vms", backoff, e)
	}
}

func TestLog_UsesCallerCtx(t *testing.T) {
	// ctx 原样交给 slog：xlog 从这里取 trace_id / span_id
	type key struct{}
	srv, _ := echo(t, nil)
	var mu sync.Mutex
	var seen []any
	old := slog.Default()
	slog.SetDefault(slog.New(ctxHandler{fn: func(ctx context.Context, r slog.Record) {
		if r.Message == "http request" {
			mu.Lock()
			seen = append(seen, ctx.Value(key{}))
			mu.Unlock()
		}
	}}))
	t.Cleanup(func() { slog.SetDefault(old) })
	client := logClient(t)
	client.R().SetContext(context.WithValue(context.Background(), key{}, "caller")).Get(srv.URL)
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "caller" {
		t.Errorf("日志该用调用方的 ctx，got=%v", seen)
	}
}

type ctxHandler struct {
	fn func(context.Context, slog.Record)
}

func (h ctxHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h ctxHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h ctxHandler) WithGroup(string) slog.Handler            { return h }
func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	h.fn(ctx, r)
	return nil
}

func TestScrubErrorText(t *testing.T) {
	for in, want := range map[string]string{
		`Get "http://user:***@h:1/x?token=a": EOF`:    `Get "http://h:1/x": EOF`,
		`Get "https://u@h/x": EOF`:                    `Get "https://h/x": EOF`,
		`Get "http://h/x": context deadline exceeded`: `Get "http://h/x": context deadline exceeded`,
	} {
		if got := scrubErrorText(in); got != want {
			t.Errorf("scrubErrorText(%q)\n got=%q\nwant=%q", in, got, want)
		}
	}
}

func TestConfig_LogDefaults(t *testing.T) {
	c := DefaultConfig()
	if c.Log || c.SlowThreshold != time.Second {
		t.Errorf("默认 Log=false、SlowThreshold=1s，got %v %v", c.Log, c.SlowThreshold)
	}
	c.SlowThreshold = -time.Millisecond
	if err := c.Validate(); err == nil {
		t.Error("SlowThreshold 为负应当报错")
	}
}
