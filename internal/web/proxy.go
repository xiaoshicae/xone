package web

import (
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
)

// TrustPrivate TrustedProxies 里代表 privateNetworks 的关键字
const TrustPrivate = "private"

// privateNetworks TrustedProxies 里写 private 时展开成的网段：
// 回环、RFC 1918 私有网段、运营商级 NAT（有的 CNI 和云厂商拿它当 Pod 网段）、IPv6 的回环和 ULA
var privateNetworks = []string{
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10",
	"::1/128", "fc00::/7",
}

// ExpandProxies 把 TrustedProxies 里的 private 展开成网段，其余原样保留。
//
// 交给框架自己的代理设置（gin 的 SetTrustedProxies）用的是它：框架不认识 private 这个词
func ExpandProxies(list []string) []string {
	out := make([]string, 0, len(list))
	for _, p := range list {
		if p == TrustPrivate {
			out = append(out, privateNetworks...)
			continue
		}
		out = append(out, p)
	}
	return out
}

// ValidateProxies 检查 TrustedProxies 的每一项都是 IP、网段或 private。
//
// 网段写错了就直接起不来。gin 那边的行为是解析到出错为止、把已经解出来的
// 留下，于是前半段代理被信任、后半段被悄悄丢掉——日志里的 client_ip
// 一半真一半假，是比起不来难查得多的状态
func ValidateProxies(list []string) error {
	for _, p := range list {
		if p != TrustPrivate && !isIPOrCIDR(p) {
			return fmt.Errorf("TrustedProxies contains an invalid address, want an IP, a CIDR or %q, got=%q", TrustPrivate, p)
		}
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

// Proxies 解好的可信网段，见 ParseProxies
type Proxies []netip.Prefix

// ParseProxies 把 TrustedProxies（可以含 private）解成网段，单个 IP 当作只含它自己的网段。
// 写错的在 ValidateProxies 里就拦下了，这里解不出的直接跳过——拿不准就不信
func ParseProxies(list []string) Proxies {
	expanded := ExpandProxies(list)
	out := make(Proxies, 0, len(expanded))
	for _, s := range expanded {
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
		} else if a, err := netip.ParseAddr(s); err == nil {
			a = a.Unmap()
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return out
}

// Trusts ip 是否落在任一网段里。ip 是不带端口的地址，解不出来的一律不信。
//
// 该问的是直连的对端（RemoteAddr），不是框架算出来的 client IP：后者正是从
// X-Forwarded-For 这些可伪造的头里推出来的。
//
// Unmap 是为了 ::ffff:10.0.0.1 这种写法的 IPv4 也能对上 10.0.0.0/8
func (ps Proxies) Trusts(ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.Unmap()
	return slices.ContainsFunc(ps, func(p netip.Prefix) bool { return p.Contains(a) })
}
