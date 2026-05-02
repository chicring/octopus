package relay

import "testing"

func TestDetectClient(t *testing.T) {
	tests := []struct {
		ua   string
		want string
	}{
		// 已确认 UA 格式
		{"Claude-Code", "claude-code"},
		{"claude-code/1.0.0", "claude-code"},
		{"claude-cli/2.1.119 (external, cli)", "claude-code"},
		{"RooCode/1.0.0", "roo-code"},
		{"roo-cline/3.17.5", "roo-code"},
		{"Cline/3.5.0", "cline"},
		{"aider/0.80.0", "aider"},
		{"codex/1.0.0", "codex"},
		{"cursor/0.50.0", "cursor"},
		{"windsurf/1.0.0", "windsurf"},
		{"Codeium/1.0.0", "windsurf"},
		{"copilot/1.0.0", "copilot"},
		{"amazon-q/1.0.0", "amazon-q"},
		{"augment/1.0.0", "augment"},
		{"goose/1.0.0", "goose"},
		{"kiro/1.0.0", "kiro"},
		{"trae/1.0.0", "trae"},
		{"zed/1.0.0", "zed"},
		{"warp/1.0.0", "warp"},
		{"factory-cli/0.113.0", "factory-droid"},
		{"gemini-cli/1.0.0", "gemini-cli"},
		{"anthropic-typescript/0.92.0", "anthropic-ts"},
		{"openai/python/2.33.0", "openai-python"},
		{"openai/js/4.0.0", "openai-js"},
		{"RooCode/3.5.0", "roo-code"},
		{"roo-code/1.0", "roo-code"},
		{"Cline/3.5.0", "cline"},
		{"aider/0.50.0", "aider"},
		{"codex/1.0.0", "codex"},
		{"OpenAI/Python 1.30.0", "openai-python"},
		{"OpenAI/JS 4.50.0", "openai-js"},
		{"anthropic-typescript/0.30.0", "anthropic-ts"},
		{"anthropic/node/0.30.0", "anthropic-ts"},

		// 关键字匹配
		{"Cursor/1.0.0", "cursor"},
		{"Windsurf/1.0.0", "windsurf"},
		{"codeium/1.0", "windsurf"},
		{"GitHub Copilot/1.0", "copilot"},
		{"copilot-agent/1.0", "copilot"},

		// 短词边界匹配
		{"amp/1.0.0", "amp"},
		{"crush/1.0.0", "crush"},
		{"goose/1.0.0", "goose"},
		{"kiro/1.0.0", "kiro"},
		{"trae/1.0.0", "trae"},
		{"warp/1.0.0", "warp"},
		{"zed/1.0.0", "zed"},

		// Factory Droid / CLI
		{"factory-droid/1.0.0", "factory-droid"},
		{"factory/droid", "factory-droid"},
		{"factory-cli/0.113.0", "factory-droid"},
		{"Factory-CLI/0.114.1", "factory-droid"},
		{"Cline/3.5.0 OpenAI/JS 4.50.0", "cline"},

		// 边界匹配防误匹配
		{"champ/1.0", ""},           // 不应匹配 amp
		{"example/1.0", ""},         // 不应匹配 amp
		{"amplitude/1.0", ""},       // 不应匹配 amp
		{"crushed/1.0", ""},         // 不应匹配 crush
		{"recognized/1.0", ""},      // 不应匹配 zed
		{"mongoose/1.0", ""},        // 不应匹配 goose
		{"warped/1.0", ""},          // 不应匹配 warp
		{"classmate/1.0", ""},       // 不应匹配 comate
		{"Android/1.0", ""},         // 不应匹配 droid
		{"factory/1.0", ""},         // 不应匹配 factory-droid

		// 未知 UA
		{"Mozilla/5.0", ""},
		{"", ""},
		{"python-requests/2.28.0", ""},
	}

	for _, tt := range tests {
		got := DetectClient(tt.ua)
		if got != tt.want {
			t.Errorf("DetectClient(%q) = %q, want %q", tt.ua, got, tt.want)
		}
	}
}

func TestWordBoundaryContains(t *testing.T) {
	tests := []struct {
		ua      string
		pattern string
		want    bool
	}{
		{"amp/1.0", "amp", true},
		{"champ/1.0", "amp", false},
		{"example/1.0", "amp", false},
		{"amp", "amp", true},
		{"amp-1.0", "amp", true},
		{"x-amp/1.0", "amp", true},
		{"ampx/1.0", "amp", false},
		{"xamp/1.0", "amp", false},
	}

	for _, tt := range tests {
		got := wordBoundaryContains(tt.ua, tt.pattern)
		if got != tt.want {
			t.Errorf("wordBoundaryContains(%q, %q) = %v, want %v", tt.ua, tt.pattern, got, tt.want)
		}
	}
}
