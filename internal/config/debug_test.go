package config

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestDebug_OnlyAcceptsKnownEnableValues(t *testing.T) {
	for v, want := range map[string]bool{
		"1": true, "true": true, "TRUE": true, " yes ": true, "on": true,
		"": false, "0": false, "false": false, "off": false, "debug": false,
	} {
		t.Setenv(DebugEnvKey, v)
		if got := Debug(); got != want {
			t.Errorf("XONE_DEBUG=%q：Debug()=%v，want %v", v, got, want)
		}
	}
}

func TestRedacted_MasksCredentials_KeepsRest_LeavesOriginalNode(t *testing.T) {
	src := `
DB:
  Password: hunter2
  db_password: hunter3
  ApiKey: k-123
  AccessToken: t-123
  ClientSecret: s-123
  Empty: ""
  EmptyPassword: ""
  KeyPrefix: "order:"
  KeyFile: /etc/ssl/client-key.pem
  DSN: "postgres://app:pg-pw@db:5432/app?sslmode=disable"
  MySQL: "report:my-pw@tcp(report-db:3306)/report"
  Keyword: "host=db user=app password=kw-pw dbname=app"
  Query: "https://api.example.com/x?user=a&password=q-pw"
  Redis: "redis://:redis-pw@cache:6379/0"
  Plain: "no secrets here" # 注释不带出来
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(redacted(&doc))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, secret := range []string{"hunter2", "hunter3", "k-123", "t-123", "s-123", "pg-pw", "my-pw", "kw-pw", "q-pw", "redis-pw"} {
		if strings.Contains(got, secret) {
			t.Errorf("%q 该被遮掉，got=\n%s", secret, got)
		}
	}
	for _, keep := range []string{"order:", "/etc/ssl/client-key.pem", "postgres://app:***@db:5432/app?sslmode=disable",
		"report:***@tcp(report-db:3306)/report", "password=***", "no secrets here", `Empty: ""`,
		`EmptyPassword: ""`} {
		if !strings.Contains(got, keep) {
			t.Errorf("该保留 %q，got=\n%s", keep, got)
		}
	}
	if strings.Contains(got, "注释不带出来") {
		t.Errorf("合并之后的配置不该带着注释，got=\n%s", got)
	}
	// 遮的是拷贝：真正的配置照样拿得到密码
	if orig, _ := yaml.Marshal(&doc); !strings.Contains(string(orig), "hunter2") || !strings.Contains(string(orig), "pg-pw") {
		t.Errorf("原节点不该被改，got=\n%s", orig)
	}
}

// debugLoad 在 dir 里写好 files，打开 XONE_DEBUG 加载 base，返回调试输出
func debugLoad(t *testing.T, dir string, files map[string]string, base string) string {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var b bytes.Buffer
	old := DebugOut
	DebugOut = &b
	t.Cleanup(func() { DebugOut = old })
	Reset()
	t.Cleanup(Reset)
	if err := Ensure(filepath.Join(dir, base), slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestEnsure_XONE_DEBUGPrintsFileOrderProfilesAndFinalConfig(t *testing.T) {
	t.Setenv(DebugEnvKey, "1")
	t.Setenv(ProfileEnvKey, "prod")
	t.Setenv("ORDER_TOKEN", "tok-s3cret")
	dir := t.TempDir()
	out := debugLoad(t, dir, map[string]string{
		"application.yml":      "XApp:\n  Import: [common/log.yml]\nOrder:\n  Channels: [a, b]\n",
		"common/log.yml":       "XLog:\n  Level: info\n",
		"application-prod.yml": "Order:\n  Channels: [a]\n  Token: \"${ORDER_TOKEN}\"\n",
	}, "application.yml")

	for _, want := range []string{
		"config file: " + filepath.Join(dir, "application.yml") + " (xone.WithConfigPath)",
		"profiles: prod (from XONE_PROFILE)",
		"1. " + filepath.Join(dir, "application.yml"),
		"2. " + filepath.Join(dir, "common/log.yml"),
		"3. " + filepath.Join(dir, "application-prod.yml"),
		"effective config",
		"Level: info",
		"Token: '***'",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("调试输出里该有 %q，got=\n%s", want, out)
		}
	}
	// 最终配置是合并之后的：prod 的列表整体替换了 base 的
	if !strings.Contains(out, "Channels: [a]") {
		t.Errorf("该打合并之后的最终值（Channels 只剩 a），got=\n%s", out)
	}
	if strings.Contains(out, "tok-s3cret") {
		t.Errorf("凭证不该出现在调试输出里，got=\n%s", out)
	}
}

func TestEnsure_PrintsNothingWithoutXONE_DEBUG(t *testing.T) {
	t.Setenv(DebugEnvKey, "")
	out := debugLoad(t, t.TempDir(), map[string]string{"application.yml": "XLog:\n  Level: info\n"}, "application.yml")
	if out != "" {
		t.Errorf("没开 XONE_DEBUG 不该有调试输出，got=\n%s", out)
	}
}
