package anthropic

import (
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

func ptrStr(s string) *string { return &s }
