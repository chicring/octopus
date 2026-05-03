package usagecard

import (
	"testing"
)

func TestListTemplates(t *testing.T) {
	templates := ListTemplates()
	if len(templates) < 4 {
		t.Errorf("expected at least 4 built-in templates, got %d", len(templates))
	}
}

func TestGetTemplate(t *testing.T) {
	t.Run("existing template", func(t *testing.T) {
		tmpl, ok := GetTemplate("github-rate-limit")
		if !ok {
			t.Error("github-rate-limit template should exist")
		}
		if tmpl.Name == "" {
			t.Error("template name should not be empty")
		}
		if tmpl.DefaultEndpoint == "" {
			t.Error("github template should have default endpoint")
		}
	})

	t.Run("nonexistent template", func(t *testing.T) {
		_, ok := GetTemplate("nonexistent")
		if ok {
			t.Error("nonexistent template should not exist")
		}
	})
}

func TestBuildCardConfig(t *testing.T) {
	tmpl, ok := GetTemplate("github-rate-limit")
	if !ok {
		t.Fatal("github-rate-limit template not found")
	}

	config := BuildCardConfig(tmpl)
	if len(config.Metrics) == 0 {
		t.Error("config should have metrics")
	}

	// Check that metric IDs match template
	for i, mc := range config.Metrics {
		if mc.ID != tmpl.Metrics[i].ID {
			t.Errorf("metric %d ID = %s, want %s", i, mc.ID, tmpl.Metrics[i].ID)
		}
		if mc.Kind != tmpl.Metrics[i].Kind {
			t.Errorf("metric %d Kind = %s, want %s", i, mc.Kind, tmpl.Metrics[i].Kind)
		}
	}
}

func TestBuildExtraHeaders(t *testing.T) {
	tmpl, ok := GetTemplate("github-rate-limit")
	if !ok {
		t.Fatal("github-rate-limit template not found")
	}

	headers := BuildExtraHeaders(tmpl)
	if len(headers) == 0 {
		t.Error("github template should have extra headers (Accept)")
	}

	foundAccept := false
	for _, h := range headers {
		if h.Key == "Accept" {
			foundAccept = true
			break
		}
	}
	if !foundAccept {
		t.Error("github template should include Accept header")
	}
}

func TestGenericTemplateHasFlexibleAuth(t *testing.T) {
	tmpl, ok := GetTemplate("generic-json")
	if !ok {
		t.Fatal("generic-json template not found")
	}

	// Should support all auth types
	expectedAuthTypes := []string{"none", "bearer", "x-api-key", "custom-header", "cookie"}
	for _, at := range expectedAuthTypes {
		found := false
		for _, ta := range tmpl.AuthTypes {
			if ta == at {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("generic-json should support auth type %q", at)
		}
	}
}

func TestAnthropicTemplateHasApiKeyAuth(t *testing.T) {
	tmpl, ok := GetTemplate("anthropic-rate-limit")
	if !ok {
		t.Fatal("anthropic-rate-limit template not found")
	}

	foundApiKey := false
	for _, at := range tmpl.AuthTypes {
		if at == "x-api-key" {
			foundApiKey = true
			break
		}
	}
	if !foundApiKey {
		t.Error("anthropic template should support x-api-key auth")
	}

	// Should have anthropic-version header
	foundVersion := false
	for _, h := range tmpl.RequiredHeaders {
		if h.Key == "anthropic-version" {
			foundVersion = true
			break
		}
	}
	if !foundVersion {
		t.Error("anthropic template should require anthropic-version header")
	}
}

func TestOpenAITemplateMetrics(t *testing.T) {
	tmpl, ok := GetTemplate("openai-rate-limit")
	if !ok {
		t.Fatal("openai-rate-limit template not found")
	}

	if len(tmpl.Metrics) != 2 {
		t.Errorf("openai template should have 2 metrics (requests + tokens), got %d", len(tmpl.Metrics))
	}

	// Both metrics should use header source
	for _, m := range tmpl.Metrics {
		if m.Limit == nil || m.Limit.Source != "header" {
			t.Errorf("metric %s limit should be from header", m.ID)
		}
		if m.Remaining == nil || m.Remaining.Source != "header" {
			t.Errorf("metric %s remaining should be from header", m.ID)
		}
	}
}

func TestXfyunCodingPlanTemplate(t *testing.T) {
	tmpl, ok := GetTemplate("xfyun-coding-plan")
	if !ok {
		t.Fatal("xfyun-coding-plan template not found")
	}

	if tmpl.Name != "讯飞 Coding Plan" {
		t.Errorf("name = %s, want 讯飞 Coding Plan", tmpl.Name)
	}

	// Should support cookie auth
	foundCookie := false
	for _, at := range tmpl.AuthTypes {
		if at == "cookie" {
			foundCookie = true
			break
		}
	}
	if !foundCookie {
		t.Error("xfyun template should support cookie auth")
	}

	// Should have 3 metrics: package, rp5h, rpw
	if len(tmpl.Metrics) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(tmpl.Metrics))
	}

	expectedIDs := map[string]string{
		"package": "monthly",
		"rp5h":    "5h",
		"rpw":     "weekly",
	}
	for _, m := range tmpl.Metrics {
		wantWindow, ok := expectedIDs[m.ID]
		if !ok {
			t.Errorf("unexpected metric ID: %s", m.ID)
			continue
		}
		if m.Window != wantWindow {
			t.Errorf("metric %s window = %s, want %s", m.ID, m.Window, wantWindow)
		}
		if m.Limit == nil || m.Limit.Source != "body" {
			t.Errorf("metric %s limit should be from body", m.ID)
		}
	}

	// PrimaryMetricIDs should be all 3
	if len(tmpl.PrimaryMetricIDs) != 3 {
		t.Errorf("expected 3 primary metric IDs, got %d", len(tmpl.PrimaryMetricIDs))
	}
}
