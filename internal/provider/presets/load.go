package presets

import (
	_ "embed"
	"encoding/json"
	"os"
	"sync"

	"github.com/bestruirui/octopus/internal/utils/log"
)

//go:embed presets.json
var embeddedPresets []byte

// ProviderPreset provider 预设
type ProviderPreset struct {
	Name           string `json:"name"`
	ProviderID     string `json:"provider_id"`
	DefaultBaseURL string `json:"default_base_url"`
	AuthType       string `json:"auth_type"`
}

var (
	presets []ProviderPreset
	once    sync.Once
)

// load 加载预设数据：优先从 data/providers.json 读取，回退到嵌入数据
func load() {
	dataDir := "data/providers.json"
	if data, err := os.ReadFile(dataDir); err == nil {
		if err := json.Unmarshal(data, &presets); err != nil {
			log.Errorf("failed to parse %s: %v, using embedded presets", dataDir, err)
			loadEmbedded()
		}
		return
	}
	loadEmbedded()
}

func loadEmbedded() {
	if err := json.Unmarshal(embeddedPresets, &presets); err != nil {
		log.Errorf("failed to parse embedded presets: %v", err)
		presets = []ProviderPreset{}
	}
}

// List 返回所有预设（懒加载）
func List() []ProviderPreset {
	once.Do(load)
	return presets
}
