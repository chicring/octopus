package balancer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

// setupOpCacheForBalancer 初始化 op 缓存 + 一个带多 key 的渠道，供 Iterator.AvailableKeys 集成测试使用。
func setupOpCacheForBalancer(t *testing.T, channelID int, keys []model.ChannelKey) {
	t.Helper()
	ctx := context.Background()
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "balancer-iter.db"), false); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := op.InitCache(); err != nil {
		t.Fatalf("InitCache: %v", err)
	}
	ch := &model.Channel{
		ID:      channelID,
		Name:    "iter-test",
		Type:    outbound.OutboundTypeOpenAIChat,
		Enabled: true,
		BaseUrls: []model.BaseUrl{{URL: "https://example.com"}},
		Keys:    keys,
	}
	if err := op.ChannelCreate(ch, ctx); err != nil {
		t.Fatalf("ChannelCreate: %v", err)
	}
}

// TestIterator_AvailableKeys_ReturnsEligibleKeys 验证 Iterator.AvailableKeys 返回渠道内所有可用 key
// （过滤 disabled/空/429 冷却），且首个为最低成本。
func TestIterator_AvailableKeys_ReturnsEligibleKeys(t *testing.T) {
	channelID := 7001
	setupOpCacheForBalancer(t, channelID, []model.ChannelKey{
		{Enabled: true, ChannelKey: "k1", TotalCost: 5.0},
		{Enabled: true, ChannelKey: "k2"}, // 创建后禁用（GORM default:true 会把 false 当零值）
		{Enabled: true, ChannelKey: "", TotalCost: 1.0}, // 空 key 被过滤
		{Enabled: true, ChannelKey: "k4", TotalCost: 1.0}, // 最低成本
		{Enabled: true, ChannelKey: "k5", TotalCost: 10.0},
	})

	// 禁用 k2
	ch, _ := op.ChannelGet(channelID, context.Background())
	disabled := false
	if _, err := op.ChannelUpdate(&model.ChannelUpdateRequest{
		ID: channelID,
		KeysToUpdate: []model.ChannelKeyUpdateRequest{
			{ID: ch.Keys[1].ID, Enabled: &disabled},
		},
	}, context.Background()); err != nil {
		t.Fatalf("disable k2: %v", err)
	}

	group := model.Group{
		ID:   1,
		Mode: model.GroupModeRoundRobin,
		Items: []model.GroupItem{
			{ChannelID: channelID, ModelName: "gpt-4"},
		},
	}
	iter := NewIterator(group, 1, "gpt-4")
	keys := iter.AvailableKeys(channelID)

	if len(keys) != 3 {
		t.Fatalf("expected 3 eligible keys, got %d", len(keys))
	}
	// 首个应是最低成本（k4，TotalCost=1.0）
	if keys[0].ChannelKey != "k4" {
		t.Errorf("expected lowest-cost key first, got %s (cost=%v)", keys[0].ChannelKey, keys[0].TotalCost)
	}

	// 收集所有 channel_key，确认过滤正确
	gotKeys := map[string]bool{}
	for _, k := range keys {
		gotKeys[k.ChannelKey] = true
	}
	if !gotKeys["k1"] || !gotKeys["k4"] || !gotKeys["k5"] {
		t.Errorf("expected k1/k4/k5 eligible, got %v", gotKeys)
	}
	if gotKeys["k2"] {
		t.Error("disabled k2 should be filtered out")
	}
}

// TestIterator_AvailableKeys_EmptyForUnknownChannel 验证未知渠道返回空。
func TestIterator_AvailableKeys_EmptyForUnknownChannel(t *testing.T) {
	setupOpCacheForBalancer(t, 7002, []model.ChannelKey{
		{Enabled: true, ChannelKey: "only"},
	})

	group := model.Group{
		ID:   1,
		Mode: model.GroupModeRoundRobin,
		Items: []model.GroupItem{
			{ChannelID: 7002, ModelName: "gpt-4"},
		},
	}
	iter := NewIterator(group, 1, "gpt-4")
	if keys := iter.AvailableKeys(999999); len(keys) != 0 {
		t.Errorf("unknown channel should return empty, got %d keys", len(keys))
	}
}
