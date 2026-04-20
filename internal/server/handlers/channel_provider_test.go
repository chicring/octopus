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
		Name:       "handler-update",
		Type:       outbound.OutboundTypeOpenAIChat,
		ProviderID: "openai-chat",
		Enabled:    true,
		BaseUrls:   []model.BaseUrl{{URL: "https://example.com"}},
		Keys:       []model.ChannelKey{{Enabled: true, ChannelKey: "key"}},
	}
	if err := op.ChannelCreate(channel, context.Background()); err != nil {
		t.Fatalf("ChannelCreate() error = %v", err)
	}

	newType := outbound.OutboundTypeAnthropic
	body, _ := json.Marshal(model.ChannelUpdateRequest{ID: channel.ID, Type: &newType})
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
	if updated.ProviderID != "anthropic" {
		t.Fatalf("updated.ProviderID = %q", updated.ProviderID)
	}
	if updated.Type != outbound.OutboundTypeAnthropic {
		t.Fatalf("updated.Type = %d", updated.Type)
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
