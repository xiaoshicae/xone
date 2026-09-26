// Package xgin 按配置装好 Gin，使用者拿到的是原生的 *gin.Engine。
//
//	func main() {
//		gx := xgin.New().WithRoutes(registerRoutes)
//		xone.MustRun(gx)
//	}
//
// 内置访问日志、链路、指标、panic 恢复四个中间件，顺序由框架管（访问日志开着时
// 最外层另有一个 LogScope，给 xlog.AddKV 开请求级的字段作用域；链路关着时换成
// 只透传、不开 Span 的 Propagate）；
// 开关、监听地址、超时、TLS 都在配置文件的 XGin 块里，业务代码里没有初始化。
package xgin

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// ConfigKey 本模块在配置文件里的顶层 key
const ConfigKey = "XGin"

// Config 服务端配置
type Config struct {
	// Host 监听地址。默认 0.0.0.0。
	Host string `yaml:"Host"`

	// Port 监听端口。默认 8080。
	Port int `yaml:"Port"`

	// UseH2C 是否在非 TLS 下启用 HTTP/2。默认关闭。
	//
	// TLS 模式下 HTTP/2 本来就是自动的，不需要这一项。
	//
	// 只认「先验知识」：客户端一上来就发 HTTP/2 前言（gRPC、
	// curl --http2-prior-knowledge 都是）。HTTP/1.1 的 Upgrade: h2c 握手不支持，
	// 发它的客户端拿到的是一个普通的 HTTP/1.1 响应。理由见 protocols。
	UseH2C bool `yaml:"UseH2C"`

	// CertFile TLS 证书路径。与 KeyFile 必须同时配或同时不配。
	CertFile string `yaml:"CertFile"`

	// KeyFile TLS 私钥路径。
	KeyFile string `yaml:"KeyFile"`

	// ClientCAFile 校验客户端证书用的 CA（PEM，可以放好几张）。默认空，不要客户端证书。
	//
	// 配了就是双向认证（tls.RequireAndVerifyClientCert）：客户端必须出示这个 CA 签的证书，
	// 不出示、或者不是它签的，握手就失败，请求到不了任何 handler——/metrics 这类
	// 框架挂的路由也一样。只能和 CertFile / KeyFile 一起配。
	ClientCAFile string `yaml:"ClientCAFile"`

	// MinVersion 接受的最低 TLS 版本："1.2" 或 "1.3"。默认 "1.2"。只在配了证书时生效。
	//
	// 更低的版本不收：TLS 1.0 / 1.1 早已被弃用（RFC 8996）。
	MinVersion string `yaml:"MinVersion"`

	// ReadHeaderTimeout 读请求头的超时。默认 10s，必须大于 0。
	//
	// 这是慢连接攻击的主要防线：不限制的话，慢客户端可以一直占着连接不放，
	// 连接数打满之后服务整体不可用。
	//
	// 0 不是「用个默认值」：net/http 会退到 ReadTimeout，而那个默认也是 0，
	// 结果是不限时（实测发半个请求头的连接一直不被断开）。所以 0 和负数都拦住。
	ReadHeaderTimeout time.Duration `yaml:"ReadHeaderTimeout"`

	// ReadTimeout 读完整个请求（含 body）的超时。默认不限制。
	//
	// 默认不限制是因为它会打断大文件上传。按业务上限配一个值更好，
	// 但那个值只有业务自己知道。0 是不限制，负数启动失败。
	ReadTimeout time.Duration `yaml:"ReadTimeout"`

	// WriteTimeout 写响应的超时。默认不限制。
	//
	// 默认不限制是因为它会打断 SSE、长轮询和大文件下载。0 是不限制，负数启动失败。
	WriteTimeout time.Duration `yaml:"WriteTimeout"`

	// IdleTimeout keep-alive 连接的空闲超时。默认 60s，必须大于 0。
	//
	// 与 ReadHeaderTimeout 同理：0 退到 ReadTimeout（默认 0），等于空闲连接永不回收。
	IdleTimeout time.Duration `yaml:"IdleTimeout"`

	// MaxMultipartMemory 解析 multipart 表单时在内存里留多少，单位字节。默认 8MB。
	//
	// 它不是「请求体上限」，是「超过多少才落盘」：超出的部分写进临时文件，
	// 不会被拒绝。实际代价是这个数的三倍左右——一次 60MB 的上传，
	// 配 32MB 时解析这一步让堆多占 96MB，配 8MB 是 24MB，配 1MB 是 3MB。
	// gin 自己默认 32MB，二十个并发上传就是两个 G。
	//
	// 框架不替业务定请求体上限，那得按接口来。要限的话在中间件里：
	//
	//	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
	MaxMultipartMemory int64 `yaml:"MaxMultipartMemory"`

	// TrustedProxies 信任哪些直连的对端：它们发来的 X-Forwarded-For / X-Real-IP 才用来算 client_ip，
	// 它们发来的透传 Header（XTrace.ForwardHeaders / ForwardHeaderRules）和 baggage 才会被收下、带给下游。
	// 「谁是自己人」只在这一处说。
	//
	// 默认是 [private]：回环、私有网段、100.64.0.0/10 和 IPv6 的 ::1、fc00::/7（见 privateNetworks）。
	// 负载均衡、K8s 的 Ingress 和 Pod、sidecar 都落在这些网段里，Pod IP 再随机也不用一个个写；
	// 直接暴露在公网的服务，对端是公网地址，一个都不信，X-Forwarded-For 照样伪造不了 client_ip。
	// gin 自己的默认是「全都信」：任何人发一个 X-Forwarded-For: 1.2.3.4 就能决定 client_ip。
	//
	// 在默认之外再加网段时把 private 一起写上（列表整体替换默认值）；要谁都不信就写 []：
	//
	//	TrustedProxies: [private, 203.0.113.0/24]
	//	TrustedProxies: []
	//
	// 内网里也有不可信的客户端（办公网、VPN 用户能直连服务）时，别用 private，写确切的那几段。
	TrustedProxies []string `yaml:"TrustedProxies"`

	// Mode Gin 的运行模式：release / debug / test。默认 release。
	//
	// 默认 release 而不是跟随 GIN_MODE：debug 模式会打印每一条路由、
	// 每个请求多打一行日志，还会在响应里带上调试信息。
	// 线上忘了设环境变量的代价比本地少一行提示大得多。
	Mode string `yaml:"Mode"`

	// Log 是否启用访问日志中间件。默认启用。
	Log bool `yaml:"Log"`

	// LogSkipPaths 不记访问日志的路径。默认没有。
	//
	// 以 / 结尾的按前缀匹配，其余精确匹配。Metric 开着时 MetricPath 会自动加进来，不用自己写。
	LogSkipPaths []string `yaml:"LogSkipPaths"`

	// LogRequestBody 是否把请求体记进访问日志。默认不记。
	//
	// 记 body 要缓存请求体的前 256KB 并对每个字段做脱敏，代价和风险都不小。
	// 打开前先确认敏感词配全了（middleware.AddSensitiveFields，规则见 xgin/README.md「访问日志」）。
	LogRequestBody bool `yaml:"LogRequestBody"`

	// LogResponseBody 是否把响应体记进访问日志。默认不记。
	LogResponseBody bool `yaml:"LogResponseBody"`

	// Trace 是否为入站请求开服务端 Span。默认启用。
	//
	// 只管 Span。关掉之后照样接上游传来的 traceparent、baggage 和透传 Header
	// （可信规则同 TrustedProxies），日志里的 trace_id 是上游的那条，
	// 经 xhttp 发出的请求也照样带给下游；响应里不再回带 X-Trace-Id。
	//
	// Span 和 X-Trace-Id 响应头要 import xtrace 才有：没装 xtrace 时全局
	// TracerProvider 是 OTel 的 noop，Span 无效，也就没有可回带的 trace id。
	Trace bool `yaml:"Trace"`

	// Metric 是否启用指标中间件。默认启用。
	//
	// 启用时会自动注册 MetricPath（默认 /metrics）路由，不需要自己挂。
	Metric bool `yaml:"Metric"`

	// MetricPath 指标端点的路径。默认 /metrics，Metric 开着时必须以 / 开头。
	//
	// gin 不会拒绝别的写法，而是把它拼到 / 后面（实测 v1.12.0）：写 metrics
	// 注册的是 /metrics，访问日志要跳过的却是 metrics，对不上；留空则注册在
	// 根路径 / 上，业务再注册首页时 gin 在业务自己的路由代码里 panic，
	// 看不出是这一项配错了。所以这两种都在启动时拦住。
	MetricPath string `yaml:"MetricPath"`

	// ZHTranslations 是否把 validator 的报错翻成中文。默认不翻。
	ZHTranslations bool `yaml:"ZHTranslations"`
}

// DefaultConfig 全部默认值集中在这里
func DefaultConfig() Config {
	return Config{
		Host:               "0.0.0.0",
		Port:               8080,
		ReadHeaderTimeout:  10 * time.Second,
		IdleTimeout:        60 * time.Second,
		MaxMultipartMemory: 8 << 20,
		Mode:               "release",
		Log:                true,
		Trace:              true,
		Metric:             true,
		MetricPath:         "/metrics",
		MinVersion:         "1.2",
		TrustedProxies:     []string{trustPrivate},
	}
}

// trustPrivate TrustedProxies 里代表 privateNetworks 的关键字
const trustPrivate = "private"

// privateNetworks TrustedProxies 里写 private 时展开成的网段：
// 回环、RFC 1918 私有网段、运营商级 NAT（有的 CNI 和云厂商拿它当 Pod 网段）、IPv6 的回环和 ULA
var privateNetworks = []string{
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10",
	"::1/128", "fc00::/7",
}

// trustedProxies 把 TrustedProxies 里的 private 展开成网段，其余原样保留
func (c Config) trustedProxies() []string {
	out := make([]string, 0, len(c.TrustedProxies))
	for _, p := range c.TrustedProxies {
		if p == trustPrivate {
			out = append(out, privateNetworks...)
			continue
		}
		out = append(out, p)
	}
	return out
}

// Validate 检查配置本身说不通的地方。
//
// xconfig.Unmarshal 解完配置文件里的 XGin 块会调它，于是配错的配置在启动阶段
// 就失败，不必等到服务 Start；WithConfig 给的那份在装配时校验，见 XGin.cfg。
func (c Config) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("Port must be within 1..65535, got=%d", c.Port)
	}
	// 只配一半的 TLS 是最危险的一种配错：服务会以明文起来，
	// 而配置文件看上去是配了证书的
	if (c.CertFile == "") != (c.KeyFile == "") {
		return fmt.Errorf("CertFile and KeyFile must both be set or both be empty")
	}
	// 同理：以为开了双向认证，实际是谁都能连的明文
	if c.ClientCAFile != "" && !c.tlsEnabled() {
		return fmt.Errorf("ClientCAFile requires CertFile and KeyFile, mutual TLS runs on top of TLS")
	}
	if _, ok := tlsVersions[c.MinVersion]; !ok {
		return fmt.Errorf("unknown MinVersion=%q, supported: 1.2 / 1.3", c.MinVersion)
	}
	if c.MaxMultipartMemory <= 0 {
		return fmt.Errorf("MaxMultipartMemory must be > 0, got=%d", c.MaxMultipartMemory)
	}
	// net/http 对这几项的规矩（Go 1.25 的文档如此，ReadHeaderTimeout 那条有测试钉着）：
	//   ReadHeaderTimeout 为 0 退到 ReadTimeout，负数不限时；
	//   IdleTimeout 为 0 退到 ReadTimeout，负数不限时；
	//   ReadTimeout / WriteTimeout 为 0 或负数都是不限时。
	// ReadTimeout 默认就是 0，所以前两项写 0 等于「不设防」：慢客户端发半个头
	// 就能一直占着连接，空闲的 keep-alive 连接永不回收——都是连接数被打满。
	// 配置文件看上去只是写了个 0，所以这里拦住。
	for _, d := range []struct {
		name string
		val  time.Duration
	}{
		{"ReadHeaderTimeout", c.ReadHeaderTimeout},
		{"IdleTimeout", c.IdleTimeout},
	} {
		if d.val <= 0 {
			return fmt.Errorf("%s must be > 0 (0 or negative disables it), got=%v", d.name, d.val)
		}
	}
	// 这两项的 0 是有意的「不限制」（默认值就是它）；负数跟 0 效果一样，
	// 却不像是想要「不限制」的写法，多半是写错了
	if c.ReadTimeout < 0 || c.WriteTimeout < 0 {
		return fmt.Errorf("ReadTimeout and WriteTimeout must not be negative (use 0 for unlimited), got ReadTimeout=%v WriteTimeout=%v", c.ReadTimeout, c.WriteTimeout)
	}
	switch c.Mode {
	case "release", "debug", "test":
	default:
		return fmt.Errorf("unknown Mode=%q, supported: release / debug / test", c.Mode)
	}
	// 网段写错了就直接起不来。gin 那边的行为是解析到出错为止、把已经解出来的
	// 留下，于是前半段代理被信任、后半段被悄悄丢掉——日志里的 client_ip
	// 一半真一半假，是比起不来难查得多的状态
	for _, p := range c.TrustedProxies {
		if p != trustPrivate && !isIPOrCIDR(p) {
			return fmt.Errorf("TrustedProxies contains an invalid address, want an IP, a CIDR or %q, got=%q", trustPrivate, p)
		}
	}
	// 理由见 MetricPath：gin 不拒绝，而是悄悄改写成另一个路径。
	// 关掉指标时这一项不用，写成什么都不影响
	if c.Metric && !strings.HasPrefix(c.MetricPath, "/") {
		return fmt.Errorf("MetricPath must start with /, got=%q", c.MetricPath)
	}
	return nil
}

// isIPOrCIDR 判断一段是不是合法的 IP 或者网段，与 gin 接受的写法一致
func isIPOrCIDR(s string) bool {
	if strings.Contains(s, "/") {
		_, _, err := net.ParseCIDR(s)
		return err == nil
	}
	return net.ParseIP(s) != nil
}

// tlsEnabled 是否配了 TLS
func (c Config) tlsEnabled() bool { return c.CertFile != "" && c.KeyFile != "" }

// tlsVersions MinVersion 收的写法
var tlsVersions = map[string]uint16{"1.2": tls.VersionTLS12, "1.3": tls.VersionTLS13}

// serverTLS 服务端的 TLS 设置，没配证书时是 nil。证书本身由 ListenAndServeTLS 读。
//
// Go 1.25 的服务端默认最低也是 TLS 1.2，这里照样显式写上：默认值会随 Go 版本变，
// 配置文件里写着的 1.2 不该跟着变。
func (c Config) serverTLS() (*tls.Config, error) {
	if !c.tlsEnabled() {
		return nil, nil
	}
	cfg := &tls.Config{MinVersion: tlsVersions[c.MinVersion]}
	if c.ClientCAFile != "" {
		pem, err := os.ReadFile(c.ClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("read ClientCAFile: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ClientCAFile %s contains no PEM certificate", c.ClientCAFile)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg, nil
}
