package usagecard

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/provider/auth"
)

const (
	codexFiveHourSeconds = 18000  // 5 小时
	codexWeekSeconds     = 604800 // 一周
)

// RefreshCodex 刷新 Codex 用量卡片
// 调用 wham/usage 端点，完整解析 rate_limit + code_review_rate_limit + additional_rate_limits
func RefreshCodex(ctx context.Context, card model.UsageCard) RefreshResult {
	secret, err := decryptSecret(card.EncryptedSecret)
	if err != nil {
		return RefreshResult{Error: fmt.Sprintf("解密密钥失败: %v", err)}
	}

	cred, err := auth.ParseCodexCredential(secret)
	if err != nil {
		return RefreshResult{Error: fmt.Sprintf("解析 Codex 凭证失败: %v", err)}
	}

	var refreshedCred *auth.CodexCredential
	if cred.IsExpired() {
		if cred.RefreshToken == "" {
			return RefreshResult{Error: "access_token 已过期且无 refresh_token"}
		}
		flow := &auth.CodexOAuthFlow{
			ClientID:     auth.CodexClientID,
			AuthorizeURL: auth.CodexAuthURL,
			TokenURL:     auth.CodexTokenURL,
		}
		result, err := flow.RefreshToken(ctx, cred.RefreshToken)
		if err != nil {
			log.Printf("[codex] token 刷新失败: %v，尝试使用现有 token", err)
		} else {
			newCred := auth.BuildCodexCredentialFromAuthResult(result)
			// OAuth 服务器可能不返回新的 refresh_token（RFC 6749 §6），保留旧的
			if newCred.RefreshToken == "" {
				newCred.RefreshToken = cred.RefreshToken
			}
			cred = newCred
			refreshedCred = newCred
		}
	}

	var httpClient *http.Client
	if card.UseProxy {
		if GetProxyHTTPClient == nil {
			return RefreshResult{Error: "代理客户端未初始化"}
		}
		proxyClient, err := GetProxyHTTPClient(true)
		if err != nil {
			return RefreshResult{Error: fmt.Sprintf("获取代理客户端失败: %v", err)}
		}
		httpClient = proxyClient
	} else {
		httpClient = codexHTTPClient()
	}
	statusCode, body, err := codexHTTPRequest(ctx, httpClient, "GET", "https://chatgpt.com/backend-api/wham/usage", cred)
	if err != nil {
		return RefreshResult{Error: fmt.Sprintf("请求失败: %v", err)}
	}
	if statusCode >= 400 {
		errMsg := fmt.Sprintf("接口返回 HTTP %d", statusCode)
		bodyStr := string(body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200]
		}
		if bodyStr != "" {
			errMsg += ": " + bodyStr
		}
		return RefreshResult{Error: errMsg}
	}

	metrics := normalizeCodexUsage(body)

	var refreshedCredStr string
	if refreshedCred != nil {
		if encrypted, encErr := EncryptSecret(refreshedCred.String()); encErr == nil {
			refreshedCredStr = encrypted
		} else {
			log.Printf("[codex] 加密刷新后的凭证失败: %v", encErr)
		}
	}

	return RefreshResult{Snapshot: model.UsageSnapshot{Metrics: metrics}, RefreshedCred: refreshedCredStr}
}

// ========== HTTP 辅助 ==========

func codexHTTPClient() *http.Client {
	dialer := &net.Dialer{}
	return &http.Client{
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
			if isBlockedHostname(req.URL.Hostname()) {
				return fmt.Errorf("重定向到禁止的地址: %s", req.URL.Hostname())
			}
			return nil
		},
	}
}

func codexHTTPRequest(ctx context.Context, client *http.Client, method, url string, cred *auth.CodexCredential) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Referer", "https://chatgpt.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if cred.AccountID != "" {
		req.Header.Set("ChatGPT-Account-Id", cred.AccountID)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("读取响应失败: %v", err)
	}
	return resp.StatusCode, body, nil
}

// ========== wham/usage 响应结构（兼容 snake_case + camelCase）==========

type codexUsagePayload struct {
	PlanType            string                      `json:"plan_type"`
	RateLimit           *codexRateLimitInfo         `json:"rate_limit"`
	CodeReviewRateLimit *codexRateLimitInfo          `json:"code_review_rate_limit"`
	AdditionalRateLimits []codexAdditionalRateLimit  `json:"additional_rate_limits"`
}

type codexRateLimitInfo struct {
	Allowed         bool              `json:"allowed"`
	LimitReached    bool              `json:"limit_reached"`
	PrimaryWindow   *codexWindowInfo  `json:"primary_window"`
	SecondaryWindow *codexWindowInfo  `json:"secondary_window"`
}

type codexWindowInfo struct {
	UsedPercent        *float64 `json:"used_percent"`
	LimitWindowSeconds *float64 `json:"limit_window_seconds"`
	ResetAfterSeconds  *float64 `json:"reset_after_seconds"`
	ResetAt            *float64 `json:"reset_at"`
}

type codexAdditionalRateLimit struct {
	LimitName   string             `json:"limit_name"`
	RateLimit   *codexRateLimitInfo `json:"rate_limit"`
}

// ========== plan_type 映射 ==========

var planTypeLabel = map[string]string{
	"free":       "Free",
	"plus":       "Plus",
	"team":       "Team",
	"pro":        "Pro",
	"prolite":    "Pro Lite",
	"pro-lite":   "Pro Lite",
	"pro_lite":   "Pro Lite",
	"business":   "Team",
	"go":         "Team",
	"enterprise": "Enterprise",
}

func getPlanLabel(planType string) (label string, status string) {
	if l, ok := planTypeLabel[planType]; ok {
		return l, "ok"
	}
	if planType != "" {
		return planType, "ok"
	}
	return "Unknown", "unknown"
}

// ========== 归一化逻辑 ==========

func normalizeCodexUsage(body []byte) []model.UsageMetric {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return []model.UsageMetric{
			{ID: "error", Label: "错误", Kind: "counter", Unit: "plan", Status: "error",
				Message: fmt.Sprintf("解析 JSON 失败: %v", err)},
		}
	}

	// 提取 plan_type
	planType := extractStringField(raw, "plan_type")
	planLabel, planStatus := getPlanLabel(planType)

	var metrics []model.UsageMetric

	// 1. 计划指标
	metrics = append(metrics, model.UsageMetric{
		ID:      "plan",
		Label:   "计划",
		Kind:    "counter",
		Unit:    "plan",
		Status:  planStatus,
		Message: planLabel,
	})

	// 2. 主 rate_limit 窗口
	if rlRaw, ok := raw["rate_limit"]; ok {
		var rl codexRateLimitInfo
		if json.Unmarshal(rlRaw, &rl) == nil {
			metrics = append(metrics, buildWindowMetrics(&rl, "")...)
		}
	}

	// 3. code_review_rate_limit 窗口
	if crRaw, ok := raw["code_review_rate_limit"]; ok {
		var cr codexRateLimitInfo
		if json.Unmarshal(crRaw, &cr) == nil {
			metrics = append(metrics, buildWindowMetrics(&cr, "代码审查 ")...)
		}
	}

	// 4. additional_rate_limits
	if arRaw, ok := raw["additional_rate_limits"]; ok {
		var additionals []codexAdditionalRateLimit
		if json.Unmarshal(arRaw, &additionals) == nil {
			for _, item := range additionals {
				if item.RateLimit != nil {
					prefix := item.LimitName
					if prefix != "" {
						prefix += " "
					}
					metrics = append(metrics, buildWindowMetrics(item.RateLimit, prefix)...)
				}
			}
		}
	}

	if len(metrics) <= 1 {
		metrics = append(metrics, model.UsageMetric{
			ID:      "primary_window",
			Label:   "5 小时限",
			Kind:    "rate_limit",
			Unit:    "percent",
			Window:  "5h",
			Status:  "unknown",
			Message: "未获取到用量数据",
		})
	}

	return metrics
}

// buildWindowMetrics 从一个 codexRateLimitInfo 提取 5h + weekly 两个窗口指标
func buildWindowMetrics(rl *codexRateLimitInfo, labelPrefix string) []model.UsageMetric {
	var metrics []model.UsageMetric

	// 分类窗口
	var fiveHour, weekly *codexWindowInfo
	for _, w := range []*codexWindowInfo{rl.PrimaryWindow, rl.SecondaryWindow} {
		if w == nil {
			continue
		}
		if w.LimitWindowSeconds != nil {
			secs := *w.LimitWindowSeconds
			if secs == codexFiveHourSeconds || secs <= codexFiveHourSeconds {
				fiveHour = w
				continue
			}
			if secs == codexWeekSeconds || secs > codexFiveHourSeconds {
				weekly = w
				continue
			}
		}
		// 没有 LimitWindowSeconds 时 fallback: primary → 5h, secondary → weekly
		if fiveHour == nil {
			fiveHour = w
		} else if weekly == nil {
			weekly = w
		}
	}

	if fiveHour != nil {
		metrics = append(metrics, buildWindowMetric(labelPrefix+"5 小时限", "5h", fiveHour, rl))
	}
	if weekly != nil {
		metrics = append(metrics, buildWindowMetric(labelPrefix+"周限", "weekly", weekly, rl))
	}

	return metrics
}

// buildWindowMetric 构建单个窗口指标
func buildWindowMetric(label, window string, w *codexWindowInfo, rl *codexRateLimitInfo) model.UsageMetric {
	m := model.UsageMetric{
		ID:     window + "_" + label,
		Label:  label,
		Kind:   "rate_limit",
		Unit:   "percent",
		Window: window,
	}

	// percent 模式
	limit100 := float64(100)
	m.Limit = &limit100

	usedPercent := float64(-1)
	if w.UsedPercent != nil {
		usedPercent = *w.UsedPercent
	} else if rl.LimitReached || !rl.Allowed {
		usedPercent = 100
	}

	if usedPercent >= 0 {
		m.Percent = &usedPercent
		m.Used = &usedPercent
		remaining := 100 - usedPercent
		m.Remaining = &remaining
		if usedPercent >= 100 {
			m.Status = "exhausted"
			m.Message = "额度已耗尽"
		} else if usedPercent >= 80 {
			m.Status = "warning"
			m.Message = "额度即将耗尽"
		} else {
			m.Status = "ok"
		}
	} else {
		m.Status = "unknown"
	}

	// 重置时间
	if w.ResetAt != nil && *w.ResetAt > 0 {
		resetAt := formatUnixTimestamp(int64(*w.ResetAt))
		m.ResetAt = &resetAt
	} else if w.ResetAfterSeconds != nil && *w.ResetAfterSeconds > 0 {
		resetAt := time.Now().Add(time.Duration(*w.ResetAfterSeconds) * time.Second).UTC().Format(time.RFC3339)
		m.ResetAt = &resetAt
	}

	return m
}

func extractStringField(raw map[string]json.RawMessage, key string) string {
	if v, ok := raw[key]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
	}
	return ""
}

func formatUnixTimestamp(ts int64) string {
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}
