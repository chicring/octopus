package usagecard

import (
	"fmt"
	"net"
	"strings"
)

// isBlockedIP 检查 IP 是否为内网/保留地址
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	if ip.IsUnspecified() {
		return true
	}

	// 检查 IPv4 私有网段
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
		// 169.254.0.0/16 (link-local)
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		// 127.0.0.0/8 (loopback)
		if ip4[0] == 127 {
			return true
		}
		// 0.0.0.0
		if ip4[0] == 0 && ip4[1] == 0 && ip4[2] == 0 && ip4[3] == 0 {
			return true
		}
	}

	return false
}

// isBlockedHostname 检查主机名是否为内网/保留地址
func isBlockedHostname(host string) bool {
	lower := strings.ToLower(host)

	// localhost
	if lower == "localhost" {
		return true
	}

	// .local 和 .internal 后缀
	if strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") {
		return true
	}

	// localhost 别名 *.lvh.me
	if strings.HasSuffix(lower, ".lvh.me") {
		return true
	}

	return false
}

// validateEndpoint 校验 endpoint URL
func validateEndpoint(endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("接口地址不能为空")
	}

	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return fmt.Errorf("接口地址必须以 http:// 或 https:// 开头")
	}

	// 解析 URL 获取主机名
	host := extractHost(endpoint)
	if host == "" {
		return fmt.Errorf("无法解析主机名")
	}

	if isBlockedHostname(host) {
		return fmt.Errorf("禁止访问内网/保留地址: %s", host)
	}

	return nil
}

func extractHost(endpoint string) string {
	// 去掉协议前缀
	url := endpoint
	if strings.HasPrefix(url, "http://") {
		url = url[7:]
	} else if strings.HasPrefix(url, "https://") {
		url = url[8:]
	}

	// 去掉路径和端口
	idx := strings.Index(url, "/")
	if idx >= 0 {
		url = url[:idx]
	}

	// 去掉端口
	idx = strings.Index(url, ":")
	if idx >= 0 {
		url = url[:idx]
	}

	return url
}
