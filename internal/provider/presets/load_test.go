package presets

import (
	"testing"
)

func TestList(t *testing.T) {
	list := List()
	if len(list) == 0 {
		t.Error("List() returned empty presets")
	}
	// 验证内置预设包含基本 provider
	found := false
	for _, p := range list {
		if p.ProviderID == "openai-chat" && p.Name == "OpenAI" {
			found = true
			break
		}
	}
	if !found {
		t.Error("presets should contain OpenAI preset")
	}
}

func TestPresetFields(t *testing.T) {
	list := List()
	for _, p := range list {
		if p.Name == "" {
			t.Error("preset name should not be empty")
		}
		if p.ProviderID == "" {
			t.Error("preset provider_id should not be empty")
		}
		if p.DefaultBaseURL == "" {
			t.Errorf("preset %q should have default_base_url", p.Name)
		}
	}
}