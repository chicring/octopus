package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	_ "github.com/bestruirui/octopus/internal/provider/builtin"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/gin-gonic/gin"
)

func TestUpdateChannel_TypeOnlyBackfillsProviderID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupHandlerTestDB(t)

	channel := &model.Channel{
		Name:     "handler-update",
		Enabled:  true,
		BaseUrls: []model.BaseUrl{{URL: "https://example.com", Type: outbound.OutboundTypeOpenAIChat, ProviderID: "openai-chat"}},
		Keys:     []model.ChannelKey{{Enabled: true, ChannelKey: "key"}},
	}
	if err := op.ChannelCreate(channel, context.Background()); err != nil {
		t.Fatalf("ChannelCreate() error = %v", err)
	}

	// 更新 base_urls，切换到 Anthropic 类型
	newBaseUrls := []model.BaseUrl{{URL: "https://example.com", Type: outbound.OutboundTypeAnthropic}}
	body, _ := json.Marshal(model.ChannelUpdateRequest{ID: channel.ID, BaseUrls: &newBaseUrls})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/channel/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	updateChannel(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	var response resp.ResponseStruct
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	updated, err := op.ChannelGet(channel.ID, context.Background())
	if err != nil {
		t.Fatalf("ChannelGet() error = %v", err)
	}
	if len(updated.BaseUrls) != 1 {
		t.Fatalf("expected 1 base url, got %d", len(updated.BaseUrls))
	}
	if updated.BaseUrls[0].ProviderID != "anthropic" {
		t.Fatalf("updated.BaseUrls[0].ProviderID = %q", updated.BaseUrls[0].ProviderID)
	}
	if updated.BaseUrls[0].Type != outbound.OutboundTypeAnthropic {
		t.Fatalf("updated.BaseUrls[0].Type = %d", updated.BaseUrls[0].Type)
	}
}

func setupHandlerTestDB(t *testing.T) {
	t.Helper()
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "handler-provider.db"), false); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := op.InitCache(); err != nil {
		t.Fatalf("InitCache() error = %v", err)
	}
}
