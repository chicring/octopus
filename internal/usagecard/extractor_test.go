package usagecard

import (
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestExtractFromJSON(t *testing.T) {
	body := map[string]interface{}{
		"resources": map[string]interface{}{
			"core": map[string]interface{}{
				"limit":     float64(5000),
				"remaining": float64(4999),
				"used":      float64(1),
				"reset":     float64(1372700873),
			},
			"search": map[string]interface{}{
				"limit":     float64(30),
				"remaining": float64(18),
			},
		},
		"items": []interface{}{
			map[string]interface{}{"name": "a", "value": float64(10)},
			map[string]interface{}{"name": "b", "value": float64(20)},
		},
	}

	tests := []struct {
		name    string
		path    string
		want    interface{}
		wantErr bool
	}{
		{"simple key", "$.resources.core.limit", float64(5000), false},
		{"nested key", "$.resources.core.remaining", float64(4999), false},
		{"array index", "$.items[0].value", float64(10), false},
		{"second array item", "$.items[1].name", "b", false},
		{"missing key", "$.resources.core.nonexistent", nil, true},
		{"invalid path no prefix", "resources.core", nil, true},
		{"index out of bounds", "$.items[5].value", nil, true},
		{"non-object path", "$.resources.core.limit.foo", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractFromJSON(body, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractFromJSON(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("extractFromJSON(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractFromHeaders(t *testing.T) {
	headers := map[string][]string{
		"X-Ratelimit-Limit-Requests":     {"500"},
		"x-ratelimit-remaining-requests": {"499"},
		"X-Ratelimit-Reset-Requests":     {"1672531200"},
	}

	tests := []struct {
		name    string
		key     string
		want    string
		wantErr bool
	}{
		{"exact case", "X-Ratelimit-Limit-Requests", "500", false},
		{"case insensitive", "x-ratelimit-limit-requests", "500", false},
		{"mixed case", "X-Ratelimit-Remaining-Requests", "499", false},
		{"missing header", "X-Not-Exist", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractFromHeaders(headers, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractFromHeaders(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("extractFromHeaders(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestExtract(t *testing.T) {
	body := []byte(`{
		"limit": 100,
		"used": 62,
		"remaining": 38,
		"reset_at": 1672531200
	}`)
	headers := map[string][]string{
		"X-Ratelimit-Limit-Requests":     {"60"},
		"X-Ratelimit-Remaining-Requests": {"45"},
		"X-Ratelimit-Reset-Requests":     {"1672531260"},
	}

	t.Run("body extraction with all fields", func(t *testing.T) {
		config := buildTestConfig(
			"test",
			&model.FieldSpec{Source: "body", Path: "$.limit"},
			&model.FieldSpec{Source: "body", Path: "$.used"},
			&model.FieldSpec{Source: "body", Path: "$.remaining"},
			&model.FieldSpec{Source: "body", Path: "$.reset_at", Transform: []string{"epoch_to_iso"}},
		)

		result := Extract(body, headers, config)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		if len(result.Metrics) != 1 {
			t.Fatalf("expected 1 metric, got %d", len(result.Metrics))
		}
		m := result.Metrics[0]
		if m.Limit == nil || *m.Limit != 100 {
			t.Errorf("limit = %v, want 100", m.Limit)
		}
		if m.Used == nil || *m.Used != 62 {
			t.Errorf("used = %v, want 62", m.Used)
		}
		if m.Remaining == nil || *m.Remaining != 38 {
			t.Errorf("remaining = %v, want 38", m.Remaining)
		}
		if m.Percent == nil || *m.Percent != 62.0 {
			t.Errorf("percent = %v, want 62", m.Percent)
		}
		if m.Status != "ok" {
			t.Errorf("status = %s, want ok", m.Status)
		}
		if m.ResetAt == nil {
			t.Error("reset_at should not be nil")
		}
	})

	t.Run("header extraction", func(t *testing.T) {
		config := buildTestConfig(
			"rpm",
			&model.FieldSpec{Source: "header", Path: "X-Ratelimit-Limit-Requests"},
			nil,
			&model.FieldSpec{Source: "header", Path: "X-Ratelimit-Remaining-Requests"},
			nil,
		)

		result := Extract(body, headers, config)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		m := result.Metrics[0]
		if m.Limit == nil || *m.Limit != 60 {
			t.Errorf("limit = %v, want 60", m.Limit)
		}
		if m.Remaining == nil || *m.Remaining != 45 {
			t.Errorf("remaining = %v, want 45", m.Remaining)
		}
		// used should be computed: 60 - 45 = 15
		if m.Used == nil || *m.Used != 15 {
			t.Errorf("used = %v, want 15 (computed)", m.Used)
		}
	})

	t.Run("counter without limit", func(t *testing.T) {
		counterBody := []byte(`{"count": 123}`)
		config := buildTestConfig("counter", nil, &model.FieldSpec{Source: "body", Path: "$.count"}, nil, nil)
		config.Metrics[0].Kind = "counter"

		result := Extract(counterBody, headers, config)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		m := result.Metrics[0]
		if m.Used == nil || *m.Used != 123 {
			t.Errorf("used = %v, want 123", m.Used)
		}
		if m.Limit != nil {
			t.Errorf("limit should be nil for counter, got %v", m.Limit)
		}
		if m.Status != "ok" {
			t.Errorf("status = %s, want ok", m.Status)
		}
	})

	t.Run("exhausted status", func(t *testing.T) {
		exhaustedBody := []byte(`{"limit": 100, "used": 100, "remaining": 0}`)
		config := buildTestConfig(
			"exhausted",
			&model.FieldSpec{Source: "body", Path: "$.limit"},
			&model.FieldSpec{Source: "body", Path: "$.used"},
			&model.FieldSpec{Source: "body", Path: "$.remaining"},
			nil,
		)

		result := Extract(exhaustedBody, headers, config)
		m := result.Metrics[0]
		if m.Status != "exhausted" {
			t.Errorf("status = %s, want exhausted", m.Status)
		}
	})

	t.Run("warning status at 80%", func(t *testing.T) {
		warningBody := []byte(`{"limit": 100, "used": 85, "remaining": 15}`)
		config := buildTestConfig(
			"warning",
			&model.FieldSpec{Source: "body", Path: "$.limit"},
			&model.FieldSpec{Source: "body", Path: "$.used"},
			&model.FieldSpec{Source: "body", Path: "$.remaining"},
			nil,
		)

		result := Extract(warningBody, headers, config)
		m := result.Metrics[0]
		if m.Status != "warning" {
			t.Errorf("status = %s, want warning", m.Status)
		}
	})

	t.Run("missing required limit returns error status", func(t *testing.T) {
		emptyBody := []byte(`{}`)
		config := buildTestConfig(
			"missing",
			&model.FieldSpec{Source: "body", Path: "$.limit"},
			nil,
			nil,
			nil,
		)

		result := Extract(emptyBody, headers, config)
		m := result.Metrics[0]
		if m.Status != "error" {
			t.Errorf("status = %s, want error", m.Status)
		}
	})

	t.Run("optional field missing is ok", func(t *testing.T) {
		emptyBody := []byte(`{"limit": 100}`)
		config := buildTestConfig(
			"optional",
			&model.FieldSpec{Source: "body", Path: "$.limit"},
			&model.FieldSpec{Source: "body", Path: "$.used", Optional: true},
			&model.FieldSpec{Source: "body", Path: "$.remaining", Optional: true},
			&model.FieldSpec{Source: "body", Path: "$.reset_at", Optional: true},
		)

		result := Extract(emptyBody, headers, config)
		m := result.Metrics[0]
		if m.Status == "error" {
			t.Errorf("optional fields should not cause error status, got %s", m.Status)
		}
		if m.Limit == nil || *m.Limit != 100 {
			t.Errorf("limit = %v, want 100", m.Limit)
		}
	})

	t.Run("invalid JSON body", func(t *testing.T) {
		badBody := []byte(`not json`)
		config := buildTestConfig("bad", &model.FieldSpec{Source: "body", Path: "$.limit"}, nil, nil, nil)

		result := Extract(badBody, headers, config)
		if result.Error == "" {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("xfyun coding plan response", func(t *testing.T) {
		xfyunBody := []byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"rows": [{
					"codingPlanUsageDTO": {
						"packageUsage": 50,
						"packageLimit": 200,
						"packageLeft": 150,
						"rp5hUsage": 3,
						"rp5hLimit": 10,
						"rpwUsage": 30,
						"rpwLimit": 100
					}
				}]
			}
		}`)

		config := model.UsageCardConfig{
			Metrics: []model.UsageMetricConfig{
				{
					ID: "package", Label: "套餐总量", Kind: "quota", Unit: "requests", Window: "monthly",
					Limit:     &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.packageLimit"},
					Used:      &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.packageUsage"},
					Remaining: &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.packageLeft"},
				},
				{
					ID: "rp5h", Label: "5 小时限", Kind: "rate_limit", Unit: "requests", Window: "5h",
					Limit: &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.rp5hLimit"},
					Used:  &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.rp5hUsage"},
				},
				{
					ID: "rpw", Label: "周限", Kind: "rate_limit", Unit: "requests", Window: "weekly",
					Limit: &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.rpwLimit"},
					Used:  &model.FieldSpec{Source: "body", Path: "$.data.rows[0].codingPlanUsageDTO.rpwUsage"},
				},
			},
		}

		result := Extract(xfyunBody, nil, config)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		if len(result.Metrics) != 3 {
			t.Fatalf("expected 3 metrics, got %d", len(result.Metrics))
		}

		// Package: 50/200, remaining 150
		pkg := result.Metrics[0]
		if pkg.Limit == nil || *pkg.Limit != 200 {
			t.Errorf("package limit = %v, want 200", pkg.Limit)
		}
		if pkg.Used == nil || *pkg.Used != 50 {
			t.Errorf("package used = %v, want 50", pkg.Used)
		}
		if pkg.Remaining == nil || *pkg.Remaining != 150 {
			t.Errorf("package remaining = %v, want 150", pkg.Remaining)
		}
		if pkg.Status != "ok" {
			t.Errorf("package status = %s, want ok", pkg.Status)
		}

		// 5h limit: 3/10
		rp5h := result.Metrics[1]
		if rp5h.Limit == nil || *rp5h.Limit != 10 {
			t.Errorf("rp5h limit = %v, want 10", rp5h.Limit)
		}
		if rp5h.Used == nil || *rp5h.Used != 3 {
			t.Errorf("rp5h used = %v, want 3", rp5h.Used)
		}
		if rp5h.Remaining == nil || *rp5h.Remaining != 7 {
			t.Errorf("rp5h remaining = %v, want 7 (computed)", rp5h.Remaining)
		}

		// Weekly: 30/100
		rpw := result.Metrics[2]
		if rpw.Limit == nil || *rp5h.Limit != 10 {
			t.Errorf("rpw limit = %v, want 100", rpw.Limit)
		}
		if rpw.Used == nil || *rpw.Used != 30 {
			t.Errorf("rpw used = %v, want 30", rpw.Used)
		}
	})
}

func TestComputeDerived(t *testing.T) {
	hundred := float64(100)
	sixty := float64(60)
	forty := float64(40)
	eighty := float64(80)
	zero := float64(0)

	tests := []struct {
		name          string
		kind          string
		limit         *float64
		used          *float64
		remaining     *float64
		wantUsed      *float64
		wantRemaining *float64
		wantPercent   *float64
		wantStatus    string
	}{
		{
			name: "compute used from limit-remaining",
			kind: "quota", limit: &hundred, used: nil, remaining: &forty,
			wantUsed: &sixty, wantRemaining: &forty, wantPercent: ptrFloat(60), wantStatus: "ok",
		},
		{
			name: "compute remaining from limit-used",
			kind: "quota", limit: &hundred, used: &sixty, remaining: nil,
			wantUsed: &sixty, wantRemaining: &forty, wantPercent: ptrFloat(60), wantStatus: "ok",
		},
		{
			name: "counter without limit is ok",
			kind: "counter", limit: nil, used: &sixty, remaining: nil,
			wantUsed: &sixty, wantRemaining: nil, wantPercent: nil, wantStatus: "ok",
		},
		{
			name: "exhausted when remaining is 0",
			kind: "quota", limit: &hundred, used: &hundred, remaining: &zero,
			wantUsed: &hundred, wantRemaining: &zero, wantPercent: ptrFloat(100), wantStatus: "exhausted",
		},
		{
			name: "warning at 80%",
			kind: "quota", limit: &hundred, used: &eighty, remaining: ptrFloat(20),
			wantUsed: &eighty, wantRemaining: ptrFloat(20), wantPercent: ptrFloat(80), wantStatus: "warning",
		},
		{
			name: "all nil is unknown",
			kind: "quota", limit: nil, used: nil, remaining: nil,
			wantUsed: nil, wantRemaining: nil, wantPercent: nil, wantStatus: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &model.UsageMetric{Kind: tt.kind, Limit: tt.limit, Used: tt.used, Remaining: tt.remaining}
			computeDerived(m)
			if tt.wantUsed != nil && (m.Used == nil || *m.Used != *tt.wantUsed) {
				t.Errorf("used = %v, want %v", m.Used, tt.wantUsed)
			}
			if tt.wantRemaining != nil && (m.Remaining == nil || *m.Remaining != *tt.wantRemaining) {
				t.Errorf("remaining = %v, want %v", m.Remaining, tt.wantRemaining)
			}
			if tt.wantPercent != nil && (m.Percent == nil || *m.Percent != *tt.wantPercent) {
				t.Errorf("percent = %v, want %v", m.Percent, tt.wantPercent)
			}
			if m.Status != tt.wantStatus {
				t.Errorf("status = %s, want %s", m.Status, tt.wantStatus)
			}
		})
	}
}

func TestTransformEpochToISO(t *testing.T) {
	raw := interface{}(float64(1672531200))
	result, err := transformValue(raw, []string{"epoch_to_iso"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "2023-01-01T00:00:00Z" {
		t.Errorf("got %q, want 2023-01-01T00:00:00Z", s)
	}
}

func TestTransformEpochMsToISO(t *testing.T) {
	raw := interface{}(float64(1672531200000))
	result, err := transformValue(raw, []string{"epoch_ms_to_iso"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "2023-01-01T00:00:00Z" {
		t.Errorf("got %q, want 2023-01-01T00:00:00Z", s)
	}
}

func TestStringToFloat(t *testing.T) {
	tests := []struct {
		input      string
		transforms []string
		want       float64
		wantErr    bool
	}{
		{"42", nil, 42, false},
		{"3.14", nil, 3.14, false},
		{"75%", []string{"percent_to_float"}, 75, false},
		{"  100  ", nil, 100, false},
		{"abc", nil, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := stringToFloat(tt.input, tt.transforms)
			if (err != nil) != tt.wantErr {
				t.Errorf("stringToFloat(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != nil && *got != tt.want {
				t.Errorf("stringToFloat(%q) = %v, want %v", tt.input, *got, tt.want)
			}
		})
	}
}

func TestParseJSONPath(t *testing.T) {
	tests := []struct {
		input   string
		wantLen int
		wantErr bool
	}{
		{"resources.core.limit", 3, false},
		{"items[0].value", 3, false},
		{"a", 1, false},
		{"a.b[2].c.d", 5, false},
		{"", 0, false},
		{"a..b", 0, true},
		{"a[", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parts, err := parseJSONPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseJSONPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(parts) != tt.wantLen {
				t.Errorf("parseJSONPath(%q) returned %d parts, want %d", tt.input, len(parts), tt.wantLen)
			}
		})
	}
}

// helper to build a test config
func buildTestConfig(id string, limit, used, remaining, resetAt *model.FieldSpec) model.UsageCardConfig {
	return model.UsageCardConfig{
		Metrics: []model.UsageMetricConfig{
			{
				ID:        id,
				Label:     "Test",
				Kind:      "quota",
				Unit:      "requests",
				Window:    "hourly",
				Limit:     limit,
				Used:      used,
				Remaining: remaining,
				ResetAt:   resetAt,
			},
		},
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}
