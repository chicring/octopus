package op

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

func TestChannelUpdate_ProviderIDOnlyAlsoPersistsLegacyType(t *testing.T) {
	ctx := context.Background()
	setupTestDBAndCache(t)

	channel := &model.Channel{
		Name:       "provider-id-update",
		Type:       outbound.OutboundTypeOpenAIChat,
		ProviderID: "openai-chat",
		Enabled:    true,
		BaseUrls:   []model.BaseUrl{{URL: "https://example.com"}},
		Keys:       []model.ChannelKey{{Enabled: true, ChannelKey: "key"}},
	}
	if err := ChannelCreate(channel, ctx); err != nil {
		t.Fatalf("ChannelCreate() error = %v", err)
	}

	newProviderID := "openai-embedding"
	updated, err := ChannelUpdate(&model.ChannelUpdateRequest{ID: channel.ID, ProviderID: &newProviderID, Type: outboundTypePtr(outbound.OutboundTypeOpenAIEmbedding)}, ctx)
	if err != nil {
		t.Fatalf("ChannelUpdate() error = %v", err)
	}
	if updated.ProviderID != "openai-embedding" {
		t.Fatalf("updated.ProviderID = %q", updated.ProviderID)
	}
	if updated.Type != outbound.OutboundTypeOpenAIEmbedding {
		t.Fatalf("updated.Type = %d", updated.Type)
	}

	persisted, err := ChannelGet(channel.ID, ctx)
	if err != nil {
		t.Fatalf("ChannelGet() error = %v", err)
	}
	if persisted.ProviderID != "openai-embedding" || persisted.Type != outbound.OutboundTypeOpenAIEmbedding {
		t.Fatalf("persisted channel mismatch: provider_id=%q type=%d", persisted.ProviderID, persisted.Type)
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

func outboundTypePtr(v outbound.OutboundType) *outbound.OutboundType { return &v }
