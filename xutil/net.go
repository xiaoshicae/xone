package xutil

import (
	"fmt"
	"net"
)

// GetLocalIP 获取本机 IP，优先外网 IPv4
// 优先级：public IPv4 → public IPv6 → private IPv4 → private IPv6
func GetLocalIP() (string, error) {
	pub4, pub6, pri4, pri6, err := collectLocalIPs()
	if err != nil {
		return "", err
	}
	if len(pub4) > 0 {
		return pub4[0].String(), nil
	}
	if len(pub6) > 0 {
		return pub6[0].String(), nil
	}
	if len(pri4) > 0 {
		return pri4[0].String(), nil
	}
	if len(pri6) > 0 {
		return pri6[0].String(), nil
	}
	return "", fmt.Errorf("no IP address found")
}

// GetLocalPublicIP 获取本机外网 IP，优先 IPv4
// 优先级：public IPv4 → public IPv6
func GetLocalPublicIP() (string, error) {
	pub4, pub6, _, _, err := collectLocalIPs()
	if err != nil {
		return "", err
	}
	if len(pub4) > 0 {
		return pub4[0].String(), nil
	}
	if len(pub6) > 0 {
		return pub6[0].String(), nil
	}
	return "", fmt.Errorf("no public IP address found")
}

// GetLocalPrivateIP 获取本机内网 IP，优先 IPv4
// 优先级：private IPv4 → private IPv6
func GetLocalPrivateIP() (string, error) {
	_, _, pri4, pri6, err := collectLocalIPs()
	if err != nil {
		return "", err
	}
	if len(pri4) > 0 {
		return pri4[0].String(), nil
	}
	if len(pri6) > 0 {
		return pri6[0].String(), nil
	}
	return "", fmt.Errorf("no private IP address found")
}

// collectLocalIPs 遍历网卡，按类型和协议分 4 组收集 IP
// 跳过 loopback、unspecified、multicast 地址
func collectLocalIPs() (public4, public6, private4, private6 []net.IP, err error) {
	iFaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get interfaces, error: %v", err)
	}

	for _, iface := range iFaces {
		// 跳过 loopback 网卡
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPAddr:
				ip = v.IP
			case *net.IPNet:
				ip = v.IP
			default:
				continue
			}

			if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() {
				continue
			}

			isV4 := ip.To4() != nil
			if isPrivateIP(ip) {
				if isV4 {
					private4 = append(private4, ip)
				} else {
					private6 = append(private6, ip)
				}
			} else {
				if isV4 {
					public4 = append(public4, ip)
				} else {
					public6 = append(public6, ip)
				}
			}
		}
	}
	return
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}
