package usagecard

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/provider/auth"
)

const (
	maxResponseBodySize = 1 << 20 // 1 MiB
	requestTimeout      = 10 * time.Second
)

// RefreshResult 刷新结果
type RefreshResult struct {
	Snapshot model.UsageSnapshot
	Error    string
}

// Refresh 刷新单张卡片的用量数据
func Refresh(ctx context.Context, card model.UsageCard) RefreshResult {
	secret, err := decryptSecret(card.EncryptedSecret)
	if err != nil {
		return RefreshResult{Error: fmt.Sprintf("解密密钥失败: %v", err)}
	}

	// 校验 endpoint
	if err := validateEndpoint(card.Endpoint); err != nil {
		return RefreshResult{Error: fmt.Sprintf("无效接口地址: %v", err)}
	}

	// 构建请求
	method := card.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, card.Endpoint, nil)
	if err != nil {
		return RefreshResult{Error: fmt.Sprintf("创建请求失败: %v", err)}
	}

	// 设置认证
	setAuth(req, card.AuthType, card.AuthHeader, secret)

	// 设置额外请求头
	for _, h := range card.ExtraHeaders {
		if h.Key != "" {
			req.Header.Set(h.Key, h.Value)
		}
	}

	// 发送请求
	dialer := &net.Dialer{}
	client := &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, fmt.Errorf("无效地址: %v", err)
				}
				ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil {
					return nil, fmt.Errorf("DNS 解析失败: %v", err)
				}
				for _, ip := range ips {
					if isBlockedIP(ip.IP) {
						return nil, fmt.Errorf("目标地址解析到内网/保留 IP: %s", ip.IP)
					}
				}
				return dialer.DialContext(ctx, network, addr)
			},
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			// 检查重定向目标
			host := req.URL.Hostname()
			if isBlockedHostname(host) {
				return fmt.Errorf("重定向到禁止的地址: %s", host)
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return RefreshResult{Error: fmt.Sprintf("请求失败: %v", err)}
	}
	defer resp.Body.Close()

	// 限制响应体大小
	limitedReader := io.LimitReader(resp.Body, maxResponseBodySize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return RefreshResult{Error: fmt.Sprintf("读取响应失败: %v", err)}
	}

	if resp.StatusCode >= 400 {
		errMsg := fmt.Sprintf("接口返回 HTTP %d", resp.StatusCode)
		bodyStr := string(body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200]
		}
		if bodyStr != "" {
			errMsg += ": " + bodyStr
		}
		return RefreshResult{Error: errMsg}
	}

	// 提取 headers（小写 key）
	headers := make(map[string][]string)
	for k, v := range resp.Header {
		headers[k] = v
	}

	// 提取指标
	result := Extract(body, headers, card.Config)
	if result.Error != "" {
		return RefreshResult{Error: result.Error}
	}

	return RefreshResult{Snapshot: model.UsageSnapshot{Metrics: result.Metrics}}
}

// setAuth 设置请求认证
func setAuth(req *http.Request, authType, authHeader, secret string) {
	switch authType {
	case "bearer":
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
	case "x-api-key":
		if secret != "" {
			req.Header.Set("x-api-key", secret)
		}
	case "custom-header":
		if authHeader != "" && secret != "" {
			req.Header.Set(authHeader, secret)
		}
	case "cookie":
		if secret != "" {
			req.Header.Set("Cookie", secret)
		}
	}
}

// decryptSecret 解密密钥
func decryptSecret(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}
	decrypted, err := auth.Decrypt([]byte(encrypted))
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}

// EncryptSecret 加密密钥
func EncryptSecret(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	encrypted, err := auth.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return string(encrypted), nil
}
