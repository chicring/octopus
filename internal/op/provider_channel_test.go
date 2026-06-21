package op

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

func TestChannelUpdate_BaseUrlsPersistTypeAndProviderID(t *testing.T) {
	ctx := context.Background()
	setupTestDBAndCache(t)

	channel := &model.Channel{
		Name:     "baseurl-type-update",
		Enabled:  true,
		BaseUrls: []model.BaseUrl{{URL: "https://example.com", Type: outbound.OutboundTypeOpenAIChat, ProviderID: "openai-chat"}},
		Keys:     []model.ChannelKey{{Enabled: true, ChannelKey: "key"}},
	}
	if err := ChannelCreate(channel, ctx); err != nil {
		t.Fatalf("ChannelCreate() error = %v", err)
	}

	// 更新 base_urls，切换到 embedding 类型
	updated, err := ChannelUpdate(&model.ChannelUpdateRequest{
		ID: channel.ID,
		BaseUrls: &[]model.BaseUrl{
			{URL: "https://example.com", Type: outbound.OutboundTypeOpenAIEmbedding, ProviderID: "openai-embedding"},
		},
	}, ctx)
	if err != nil {
		t.Fatalf("ChannelUpdate() error = %v", err)
	}
	if len(updated.BaseUrls) != 1 {
		t.Fatalf("expected 1 base url, got %d", len(updated.BaseUrls))
	}
	if updated.BaseUrls[0].Type != outbound.OutboundTypeOpenAIEmbedding {
		t.Fatalf("updated.BaseUrls[0].Type = %d", updated.BaseUrls[0].Type)
	}
	if updated.BaseUrls[0].ProviderID != "openai-embedding" {
		t.Fatalf("updated.BaseUrls[0].ProviderID = %q", updated.BaseUrls[0].ProviderID)
	}

	persisted, err := ChannelGet(channel.ID, ctx)
	if err != nil {
		t.Fatalf("ChannelGet() error = %v", err)
	}
	if persisted.BaseUrls[0].ProviderID != "openai-embedding" || persisted.BaseUrls[0].Type != outbound.OutboundTypeOpenAIEmbedding {
		t.Fatalf("persisted base url mismatch: provider_id=%q type=%d", persisted.BaseUrls[0].ProviderID, persisted.BaseUrls[0].Type)
	}
}

func setupTestDBAndCache(t *testing.T) {
	t.Helper()
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "provider-channel.db"), false); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		channelCache.Clear()
		channelKeyCache.Clear()
	})
	if err := InitCache(); err != nil {
		t.Fatalf("InitCache() error = %v", err)
	}
}
