package web

import (
	"slices"
	"strings"
	"testing"
)

func TestExpandProxies_PrivateExpandsOthersKept(t *testing.T) {
	got := ExpandProxies([]string{TrustPrivate, "203.0.113.0/24"})
	want := append(slices.Clone(privateNetworks), "203.0.113.0/24")
	if !slices.Equal(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
	if got := ExpandProxies(nil); len(got) != 0 {
		t.Errorf("empty list should stay empty, got=%v", got)
	}
}

func TestProxies_Trusts(t *testing.T) {
	ps := ParseProxies([]string{TrustPrivate, "203.0.113.7", "not-an-ip"})
	for ip, want := range map[string]bool{
		"10.1.2.3":        true,
		"::ffff:10.1.2.3": true, // IPv4 映射成 IPv6 的写法也认
		"100.64.0.1":      true, // 运营商级 NAT
		"fd00::1":         true, // ULA
		"203.0.113.7":     true, // 单个 IP 当作只含它自己的网段
		"203.0.113.8":     false,
		"8.8.8.8":         false,
		"":                false, // 解不出来的一律不信
		"10.1.2.3:8080":   false, // 带端口的不是地址
	} {
		if got := ps.Trusts(ip); got != want {
			t.Errorf("Trusts(%q)=%v want %v", ip, got, want)
		}
	}
	if ParseProxies(nil).Trusts("127.0.0.1") {
		t.Error("an empty list trusts nobody")
	}
}

func TestValidateProxies(t *testing.T) {
	if err := ValidateProxies([]string{TrustPrivate, "10.0.0.0/8", "::1", "203.0.113.7"}); err != nil {
		t.Errorf("valid list rejected: %v", err)
	}
	err := ValidateProxies([]string{"10.0.0.0/8", "10.0.0.0/33"})
	if err == nil || !strings.Contains(err.Error(), `got="10.0.0.0/33"`) {
		t.Errorf("an invalid CIDR must be reported by name, got=%v", err)
	}
}
