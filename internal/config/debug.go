package config

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// DebugEnvKey 设成 1 / true / yes / on 时，框架把启动过程打给人看：用了哪份配置、怎么找到的、
// 激活了哪些 profile、按优先级读了哪些文件、合并之后的最终配置（凭证已遮掉）、启动钩子的顺序。
//
// 写的是 stderr、多行、给人读的文本，不是 JSON——只在本地排查时开，别带进生产的日志管道。
const DebugEnvKey = "XONE_DEBUG"

// Debug XONE_DEBUG 开着没有
func Debug() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(DebugEnvKey))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// DebugOut 调试输出写到哪里。测试换成 buffer
var DebugOut io.Writer = os.Stderr

// Debugf 在 XONE_DEBUG 开着时写一行调试输出，前缀 [xone debug]
func Debugf(format string, args ...any) {
	if Debug() {
		fmt.Fprintf(DebugOut, "[xone debug] "+format+"\n", args...)
	}
}

// report 一次加载的经过，XONE_DEBUG 开着时打出来
type report struct {
	files        []string   // 按优先级从低到高
	profiles     []string   // 激活的 profile
	profilesFrom string     // profile 是从哪来的
	root         *yaml.Node // 合并、展开之后的整份配置；nil 表示什么都没写
}

// debugReport 把一次加载的经过写进 DebugOut
func debugReport(base, from string, r *report) {
	if !Debug() {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[xone debug] config file: %s (%s)\n", base, from)
	if len(r.profiles) == 0 {
		b.WriteString("[xone debug] profiles: none\n")
	} else {
		fmt.Fprintf(&b, "[xone debug] profiles: %s (from %s)\n", strings.Join(r.profiles, ", "), r.profilesFrom)
	}
	b.WriteString("[xone debug] files read, lowest to highest priority:\n")
	for i, f := range r.files {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, f)
	}
	b.WriteString("[xone debug] effective config (secrets redacted):\n")
	if r.root == nil {
		b.WriteString("  (empty)\n")
	} else {
		var out strings.Builder
		enc := yaml.NewEncoder(&out)
		enc.SetIndent(2)
		if err := enc.Encode(redacted(r.root)); err != nil {
			fmt.Fprintf(&b, "  (cannot render: %v)\n", err)
		} else {
			for _, l := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
				b.WriteString("  " + l + "\n")
			}
		}
	}
	fmt.Fprint(DebugOut, b.String())
}

// redactedValue 遮掉之后显示的值
const redactedValue = "***"

// sensitiveKeys key 里含这些词（比较前转小写、去掉 _ - . 和空格）的值整个遮掉。
// 不含 key：KeyPrefix、KeyFile 这类不是凭证，遮了反而看不出配了什么
var sensitiveKeys = []string{"password", "passwd", "secret", "token", "credential", "apikey", "accesskey", "privatekey"}

func sensitiveKey(k string) bool {
	k = strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(k))
	for _, s := range sensitiveKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// 值里夹着的凭证：DSN、URL 里的密码
var (
	urlUserinfo = regexp.MustCompile(`(://[^:/?#@\s]*):[^@/\s]+@`)                    // postgres://u:p@h、redis://u:p@h
	mysqlDSN    = regexp.MustCompile(`^([^:@/\s]+):[^@\s]+@`)                         // u:p@tcp(h:3306)/db
	kvPassword  = regexp.MustCompile(`(?i)\b(password|passwd|pwd)=('[^']*'|[^\s&]+)`) // password=p（PG 关键字写法、查询串）
)

func redactString(s string) string {
	s = urlUserinfo.ReplaceAllString(s, "$1:"+redactedValue+"@")
	if !strings.Contains(s, "://") {
		s = mysqlDSN.ReplaceAllString(s, "$1:"+redactedValue+"@")
	}
	return kvPassword.ReplaceAllString(s, "$1="+redactedValue)
}

// redacted 返回 n 的深拷贝，凭证已遮掉、注释去掉；n 本身不动。
// 注释不留：合并之后看不出一条注释是哪个文件写的，留着只会让人对着错的文件找
func redacted(n *yaml.Node) *yaml.Node {
	c := *n
	c.HeadComment, c.LineComment, c.FootComment = "", "", ""
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, ch := range n.Content {
		c.Content[i] = redacted(ch)
	}
	if c.Kind == yaml.ScalarNode {
		c.Value = redactString(c.Value)
	}
	if c.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(c.Content); i += 2 {
			if v := c.Content[i+1]; sensitiveKey(c.Content[i].Value) && v.Kind == yaml.ScalarNode && v.Value != "" {
				v.Value, v.Tag, v.Style = redactedValue, "!!str", 0
			}
		}
	}
	return &c
}
