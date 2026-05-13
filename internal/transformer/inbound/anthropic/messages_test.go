package anthropic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

func TestMessagesInbound_Reset_ClearsStreamState(t *testing.T) {
	adapter := &MessagesInbound{}

	// Simulate first attempt: set various stream state
	adapter.hasStarted = true
	adapter.hasTextContentStarted = true
	adapter.hasThinkingContentStarted = true
	adapter.hasToolContentStarted = true
	adapter.hasFinished = true
	adapter.messageStopped = true
	adapter.messageID = "msg-1"
	adapter.modelName = "claude-3"
	adapter.contentIndex = 5
	adapter.stopReason = ptrStr("end_turn")
	adapter.toolCallIndices = map[int]bool{0: true}
	adapter.inputToken = 12345
	adapter.streamChunks = append(adapter.streamChunks, &model.InternalLLMResponse{ID: "1"})
	adapter.storedResponse = &model.InternalLLMResponse{ID: "resp-1"}

	adapter.Reset()

	// All attempt-level state should be cleared
	if adapter.hasStarted {
		t.Error("hasStarted should be false")
	}
	if adapter.hasTextContentStarted {
		t.Error("hasTextContentStarted should be false")
	}
	if adapter.hasThinkingContentStarted {
		t.Error("hasThinkingContentStarted should be false")
	}
	if adapter.hasToolContentStarted {
		t.Error("hasToolContentStarted should be false")
	}
	if adapter.hasFinished {
		t.Error("hasFinished should be false")
	}
	if adapter.messageStopped {
		t.Error("messageStopped should be false")
	}
	if adapter.messageID != "" {
		t.Errorf("messageID should be empty, got %s", adapter.messageID)
	}
	if adapter.modelName != "" {
		t.Errorf("modelName should be empty, got %s", adapter.modelName)
	}
	if adapter.contentIndex != 0 {
		t.Errorf("contentIndex should be 0, got %d", adapter.contentIndex)
	}
	if adapter.stopReason != nil {
		t.Error("stopReason should be nil")
	}
	if adapter.toolCallIndices != nil {
		t.Error("toolCallIndices should be nil")
	}
	if len(adapter.streamChunks) != 0 {
		t.Errorf("streamChunks should be empty, got %d", len(adapter.streamChunks))
	}
	if adapter.storedResponse != nil {
		t.Error("storedResponse should be nil")
	}

	// CRITICAL: inputToken must be preserved (request-level state)
	if adapter.inputToken != 12345 {
		t.Errorf("inputToken should be preserved after reset, got %d, want 12345", adapter.inputToken)
	}
}

func TestMessagesInbound_Reset_PreservesInputToken(t *testing.T) {
	adapter := &MessagesInbound{}
	adapter.inputToken = 99999

	adapter.Reset()

	if adapter.inputToken != 99999 {
		t.Errorf("inputToken must survive reset for Anthropic streaming usage, got %d, want 99999", adapter.inputToken)
	}
}

func TestMessagesInbound_Reset_AllowsRetryWithCleanState(t *testing.T) {
	adapter := &MessagesInbound{}
	adapter.inputToken = 5000

	// First attempt: pollute state
	adapter.streamChunks = append(adapter.streamChunks,
		&model.InternalLLMResponse{ID: "1", Model: "wrong-model"},
	)
	adapter.hasStarted = true
	adapter.messageID = "old-msg"

	// Reset before retry
	adapter.Reset()

	// Verify clean state for second attempt
	if adapter.hasStarted {
		t.Error("hasStarted should be false after reset")
	}
	if adapter.messageID != "" {
		t.Error("messageID should be empty after reset")
	}
	if len(adapter.streamChunks) != 0 {
		t.Error("streamChunks should be empty after reset")
	}
	// inputToken preserved
	if adapter.inputToken != 5000 {
		t.Errorf("inputToken should be 5000, got %d", adapter.inputToken)
	}
}

func TestMessagesInbound_UsageKeepsInputTokensTotal(t *testing.T) {
	adapter := &MessagesInbound{}
	content := "hello"
	finishReason := "stop"
	resp := &model.InternalLLMResponse{
		ID:    "msg_123",
		Model: "claude-3-5-sonnet",
		Choices: []model.Choice{{
			Index:        0,
			Message:      &model.Message{Role: "assistant", Content: model.MessageContent{Content: &content}},
			FinishReason: &finishReason,
		}},
		Usage: &model.Usage{
			PromptTokens:     100,
			CompletionTokens: 20,
			TotalTokens:      120,
			PromptTokensDetails: &model.PromptTokensDetails{
				CachedTokens: 30,
			},
		},
	}

	body, err := adapter.ConvertResponseToClientFormat(context.Background(), resp)
	if err != nil {
		t.Fatalf("ConvertResponseToClientFormat error: %v", err)
	}

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("invalid Anthropic message JSON: %v", err)
	}
	if msg.Usage == nil {
		t.Fatal("usage is nil")
	}
	if msg.Usage.InputTokens != 100 {
		t.Fatalf("input_tokens = %d, want total 100", msg.Usage.InputTokens)
	}
	if msg.Usage.CacheReadInputTokens != 30 {
		t.Fatalf("cache_read_input_tokens = %d, want 30", msg.Usage.CacheReadInputTokens)
	}
}

func ptrStr(s string) *string { return &s }
