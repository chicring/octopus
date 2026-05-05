package usagecard

import "github.com/bestruirui/octopus/internal/model"

// UsageTemplate 用量接口模板定义
type UsageTemplate struct {
	ID               string                `json:"id"`
	Name             string                `json:"name"`
	Description      string                `json:"description"`
	DefaultEndpoint  string                `json:"default_endpoint"`
	Method           string                `json:"method"`
	AuthTypes        []string              `json:"auth_types"`
	RequiredHeaders  []UsageHeaderTemplate `json:"required_headers"`
	Metrics          []UsageMetricTemplate `json:"metrics"`
	PrimaryMetricIDs []string              `json:"primary_metric_ids"`
}

// UsageHeaderTemplate 模板预置请求头
type UsageHeaderTemplate struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Placeholder string `json:"placeholder,omitempty"`
}

// UsageMetricTemplate 模板指标定义
type UsageMetricTemplate struct {
	ID        string             `json:"id"`
	Label     string             `json:"label"`
	Kind      string             `json:"kind"`
	Unit      string             `json:"unit"`
	Window    string             `json:"window"`
	Limit     *model.FieldSpec   `json:"limit"`
	Used      *model.FieldSpec   `json:"used,omitempty"`
	Remaining *model.FieldSpec   `json:"remaining,omitempty"`
	ResetAt   *model.FieldSpec   `json:"reset_at,omitempty"`
}

// builtinTemplates 内置模板注册表
var builtinTemplates = map[string]UsageTemplate{}

func init() {
	register(genericJSONTemplate())
	register(xfyunCodingPlanTemplate())
	register(codexUsageTemplate())
}

func register(t UsageTemplate) {
	builtinTemplates[t.ID] = t
}

// GetTemplate 获取模板
func GetTemplate(id string) (UsageTemplate, bool) {
	t, ok := builtinTemplates[id]
	return t, ok
}

// ListTemplates 返回所有内置模板
func ListTemplates() []UsageTemplate {
	result := make([]UsageTemplate, 0, len(builtinTemplates))
	for _, t := range builtinTemplates {
		result = append(result, t)
	}
	return result
}

// BuildCardConfig 从模板构建卡片配置
func BuildCardConfig(t UsageTemplate) model.UsageCardConfig {
	metrics := make([]model.UsageMetricConfig, 0, len(t.Metrics))
	for _, m := range t.Metrics {
		metrics = append(metrics, model.UsageMetricConfig{
			ID:        m.ID,
			Label:     m.Label,
			Kind:      m.Kind,
			Unit:      m.Unit,
			Window:    m.Window,
			Limit:     m.Limit,
			Used:      m.Used,
			Remaining: m.Remaining,
			ResetAt:   m.ResetAt,
		})
	}
	return model.UsageCardConfig{Metrics: metrics}
}

// BuildExtraHeaders 从模板构建额外请求头
func BuildExtraHeaders(t UsageTemplate) []model.UsageHeader {
	headers := make([]model.UsageHeader, 0, len(t.RequiredHeaders))
	for _, h := range t.RequiredHeaders {
		headers = append(headers, model.UsageHeader{
			Key:   h.Key,
			Value: h.Value,
		})
	}
	return headers
}

func genericJSONTemplate() UsageTemplate {
	return UsageTemplate{
		ID:              "generic-json",
		Name:            "通用 JSON 接口",
		Description:     "自定义 JSON 接口，支持从 body/header 提取用量数据",
		DefaultEndpoint: "",
		Method:          "GET",
		AuthTypes:       []string{"none", "bearer", "x-api-key", "custom-header", "cookie"},
		Metrics: []UsageMetricTemplate{
			{
				ID:     "metric_1",
				Label:  "指标 1",
				Kind:   "quota",
				Unit:   "requests",
				Window: "monthly",
				Limit:     &model.FieldSpec{Source: "body", Path: "$.limit"},
				Used:      &model.FieldSpec{Source: "body", Path: "$.used"},
				Remaining: &model.FieldSpec{Source: "body", Path: "$.remaining", Optional: true},
				ResetAt:   &model.FieldSpec{Source: "body", Path: "$.reset_at", Optional: true},
			},
		},
		PrimaryMetricIDs: []string{"metric_1"},
	}
}

func githubRateLimitTemplate() UsageTemplate {
	return UsageTemplate{
		ID:              "github-rate-limit",
		Name:            "GitHub Rate Limit",
		Description:     "GitHub REST API 速率限制，展示 core/search/graphql 等资源窗口",
		DefaultEndpoint: "https://api.github.com/rate_limit",
		Method:          "GET",
		AuthTypes:       []string{"bearer"},
		RequiredHeaders: []UsageHeaderTemplate{
			{Key: "Accept", Value: "application/vnd.github+json"},
		},
		Metrics: []UsageMetricTemplate{
			{
				ID:        "core",
				Label:     "Core 请求",
				Kind:      "rate_limit",
				Unit:      "requests",
				Window:    "hourly",
				Limit:     &model.FieldSpec{Source: "body", Path: "$.resources.core.limit"},
				Remaining: &model.FieldSpec{Source: "body", Path: "$.resources.core.remaining"},
				Used:      &model.FieldSpec{Source: "body", Path: "$.resources.core.used", Optional: true},
				ResetAt:   &model.FieldSpec{Source: "body", Path: "$.resources.core.reset", Transform: []string{"epoch_to_iso"}},
			},
			{
				ID:        "search",
				Label:     "Search 请求",
				Kind:      "rate_limit",
				Unit:      "requests",
				Window:    "minute",
				Limit:     &model.FieldSpec{Source: "body", Path: "$.resources.search.limit"},
				Remaining: &model.FieldSpec{Source: "body", Path: "$.resources.search.remaining"},
				Used:      &model.FieldSpec{Source: "body", Path: "$.resources.search.used", Optional: true},
				ResetAt:   &model.FieldSpec{Source: "body", Path: "$.resources.search.reset", Transform: []string{"epoch_to_iso"}},
			},
			{
				ID:        "graphql",
				Label:     "GraphQL 请求",
				Kind:      "rate_limit",
				Unit:      "requests",
				Window:    "hourly",
				Limit:     &model.FieldSpec{Source: "body", Path: "$.resources.graphql.limit"},
				Remaining: &model.FieldSpec{Source: "body", Path: "$.resources.graphql.remaining"},
				ResetAt:   &model.FieldSpec{Source: "body", Path: "$.resources.graphql.reset", Transform: []string{"epoch_to_iso"}},
			},
		},
		PrimaryMetricIDs: []string{"core", "search", "graphql"},
	}
}

func openAIRateLimitTemplate() UsageTemplate {
	return UsageTemplate{
		ID:              "openai-rate-limit",
		Name:            "OpenAI Rate Limit",
		Description:     "OpenAI API 速率限制（通过响应头获取 RPM/TPM 窗口额度）",
		DefaultEndpoint: "https://api.openai.com/v1/models",
		Method:          "GET",
		AuthTypes:       []string{"bearer"},
		Metrics: []UsageMetricTemplate{
			{
				ID:        "requests",
				Label:     "RPM 请求",
				Kind:      "rate_limit",
				Unit:      "requests",
				Window:    "minute",
				Limit:     &model.FieldSpec{Source: "header", Path: "x-ratelimit-limit-requests"},
				Remaining: &model.FieldSpec{Source: "header", Path: "x-ratelimit-remaining-requests"},
				ResetAt:   &model.FieldSpec{Source: "header", Path: "x-ratelimit-reset-requests", Transform: []string{"epoch_to_iso"}, Optional: true},
			},
			{
				ID:        "tokens",
				Label:     "TPM Token",
				Kind:      "rate_limit",
				Unit:      "tokens",
				Window:    "minute",
				Limit:     &model.FieldSpec{Source: "header", Path: "x-ratelimit-limit-tokens"},
				Remaining: &model.FieldSpec{Source: "header", Path: "x-ratelimit-remaining-tokens"},
				ResetAt:   &model.FieldSpec{Source: "header", Path: "x-ratelimit-reset-tokens", Transform: []string{"epoch_to_iso"}, Optional: true},
			},
		},
		PrimaryMetricIDs: []string{"requests", "tokens"},
	}
}

func anthropicRateLimitTemplate() UsageTemplate {
	return UsageTemplate{
		ID:              "anthropic-rate-limit",
		Name:            "Anthropic Rate Limit",
		Description:     "Anthropic API 速率限制（通过响应头获取 RPM/TPM/Unified 窗口额度）",
		DefaultEndpoint: "https://api.anthropic.com/v1/models",
		Method:          "GET",
		AuthTypes:       []string{"x-api-key"},
		RequiredHeaders: []UsageHeaderTemplate{
			{Key: "anthropic-version", Value: "2023-06-01"},
		},
		Metrics: []UsageMetricTemplate{
			{
				ID:        "requests",
				Label:     "RPM 请求",
				Kind:      "rate_limit",
				Unit:      "requests",
				Window:    "minute",
				Limit:     &model.FieldSpec{Source: "header", Path: "anthropic-ratelimit-requests-limit"},
				Remaining: &model.FieldSpec{Source: "header", Path: "anthropic-ratelimit-requests-remaining"},
			},
			{
				ID:        "tokens",
				Label:     "TPM Token",
				Kind:      "rate_limit",
				Unit:      "tokens",
				Window:    "minute",
				Limit:     &model.FieldSpec{Source: "header", Path: "anthropic-ratelimit-tokens-limit"},
				Remaining: &model.FieldSpec{Source: "header", Path: "anthropic-ratelimit-tokens-remaining"},
			},
		},
		PrimaryMetricIDs: []string{"requests", "tokens"},
	}
}

func xfyunCodingPlanTemplate() UsageTemplate {
	return UsageTemplate{
		ID:              "xfyun-coding-plan",
		Name:            "讯飞",
		Description:     "讯飞星火 Coding Plan 用量查询，展示 5 小时限、周限、套餐总量",
		DefaultEndpoint: "https://maas.xfyun.cn/api/v1/gpt-finetune/coding-plan/list?page=1&size=6",
		Method:          "GET",
		AuthTypes:       []string{"cookie"},
		Metrics: []UsageMetricTemplate{
			{
				ID:        "rp5h",
				Label:     "5 小时限",
				Kind:      "rate_limit",
				Unit:      "requests",
				Window:    "5h",
				Limit:     &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.rp5hLimit"},
				Used:      &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.rp5hUsage"},
			},
			{
				ID:        "rpw",
				Label:     "周限",
				Kind:      "rate_limit",
				Unit:      "requests",
				Window:    "weekly",
				Limit:     &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.rpwLimit"},
				Used:      &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.rpwUsage"},
			},
			{
				ID:        "package",
				Label:     "套餐总量",
				Kind:      "quota",
				Unit:      "requests",
				Window:    "monthly",
				Limit:     &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.packageLimit"},
				Used:      &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.packageUsage"},
				Remaining: &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.packageLeft"},
			},
		},
		PrimaryMetricIDs: []string{"rp5h", "rpw", "package"},
	}
}

func codexUsageTemplate() UsageTemplate {
	return UsageTemplate{
		ID:              "codex-usage",
		Name:            "Codex",
		Description:     "ChatGPT Codex 用量查询，展示计划类型、5小时限、周限等窗口",
		DefaultEndpoint: "https://chatgpt.com/backend-api/wham/usage",
		Method:          "GET",
		AuthTypes:       []string{"bearer"},
		RequiredHeaders: []UsageHeaderTemplate{
			{Key: "Accept", Value: "application/json"},
			{Key: "Origin", Value: "https://chatgpt.com"},
			{Key: "Referer", Value: "https://chatgpt.com/"},
			{Key: "User-Agent", Value: "Mozilla/5.0"},
		},
		Metrics: []UsageMetricTemplate{
			{
				ID:     "plan",
				Label:  "计划",
				Kind:   "counter",
				Unit:   "plan",
				Used:   &model.FieldSpec{Source: "body", Path: "$.plan_type", Optional: true},
			},
			{
				ID:       "primary_window",
				Label:    "5 小时限",
				Kind:     "rate_limit",
				Unit:     "percent",
				Window:   "5h",
				Limit:    &model.FieldSpec{Source: "const", Path: "100"},
				Used:     &model.FieldSpec{Source: "body", Path: "$.rate_limit.primary_window.used_percent", Optional: true},
				ResetAt:  &model.FieldSpec{Source: "body", Path: "$.rate_limit.primary_window.reset_at", Transform: []string{"epoch_to_iso"}, Optional: true},
			},
			{
				ID:       "secondary_window",
				Label:    "周限",
				Kind:     "rate_limit",
				Unit:     "percent",
				Window:   "weekly",
				Limit:    &model.FieldSpec{Source: "const", Path: "100"},
				Used:     &model.FieldSpec{Source: "body", Path: "$.rate_limit.secondary_window.used_percent", Optional: true},
				ResetAt:  &model.FieldSpec{Source: "body", Path: "$.rate_limit.secondary_window.reset_at", Transform: []string{"epoch_to_iso"}, Optional: true},
			},
		},
		PrimaryMetricIDs: []string{"plan", "primary_window", "secondary_window"},
	}
}
