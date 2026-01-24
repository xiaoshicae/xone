package xutil

import (
	"fmt"
	"net"
	"strings"
)

// GetLocalIp 获取本机ip，优先返回内网ip
func GetLocalIp() (string, error) {
	return ExtractRealIP("0.0.0.0")
}

// ExtractRealIP returns a real ip
func ExtractRealIP(addr string) (string, error) {
	// if addr specified then its returned
	if len(addr) > 0 && (addr != "0.0.0.0" && addr != "[::]" && addr != "::") {
		candidate := strings.TrimSpace(addr)
		if host, _, err := net.SplitHostPort(candidate); err == nil {
			candidate = host
		}
		candidate = strings.TrimPrefix(candidate, "[")
		candidate = strings.TrimSuffix(candidate, "]")

		return validateIP(candidate, addr)
	}

	iFaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to get interfaces, error: %v", err)
	}

	// 预分配容量
	addrs := make([]net.Addr, 0, len(iFaces)*2)
	loAddrs := make([]net.Addr, 0, len(iFaces))

	for _, iface := range iFaces {
		ifaceAddrs, err := iface.Addrs()
		if err != nil {
			// ignore error, interface can disappear from system
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			loAddrs = append(loAddrs, ifaceAddrs...)
			continue
		}
		addrs = append(addrs, ifaceAddrs...)
	}
	addrs = append(addrs, loAddrs...)

	var ipAddr string
	var publicIP string

	for _, rawAddr := range addrs {
		var ip net.IP
		switch addr := rawAddr.(type) {
		case *net.IPAddr:
			ip = addr.IP
		case *net.IPNet:
			ip = addr.IP
		default:
			continue
		}

		// Skip non-IPv4 addresses
		if ip.To4() == nil {
			continue
		}

		if ip.IsUnspecified() || ip.IsMulticast() {
			continue
		}

		if !isPrivateIP(ip) {
			if publicIP == "" {
				publicIP = ip.String()
			}
			continue
		}

		ipAddr = ip.String()
		break
	}

	// return private ip
	if len(ipAddr) > 0 {
		return validateIP(ipAddr, ipAddr)
	}

	// return public or virtual ip
	if len(publicIP) > 0 {
		return validateIP(publicIP, publicIP)
	}

	return "", fmt.Errorf("no IP address found, and explicit IP not provided")
}

// validateIP 验证并解析 IP 地址，减少重复代码
func validateIP(candidate, original string) (string, error) {
	a := net.ParseIP(candidate)
	if a == nil {
		return "", fmt.Errorf("ip addr %s is invalid", original)
	}
	return a.String(), nil
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}
