package openai

import (
	"context"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

func TestChatInbound_Reset_ClearsStreamChunks(t *testing.T) {
	adapter := &ChatInbound{}

	// Simulate first attempt: add stream chunks
	adapter.streamChunks = append(adapter.streamChunks, &model.InternalLLMResponse{ID: "1", Model: "gpt-4"})
	adapter.streamChunks = append(adapter.streamChunks, &model.InternalLLMResponse{ID: "2", Model: "gpt-4"})

	if len(adapter.streamChunks) != 2 {
		t.Fatalf("expected 2 chunks before reset, got %d", len(adapter.streamChunks))
	}

	adapter.Reset()

	if len(adapter.streamChunks) != 0 {
		t.Errorf("expected 0 chunks after reset, got %d", len(adapter.streamChunks))
	}
	if adapter.storedResponse != nil {
		t.Error("expected storedResponse to be nil after reset")
	}
}

func TestChatInbound_Reset_ClearsStoredResponse(t *testing.T) {
	adapter := &ChatInbound{}
	adapter.storedResponse = &model.InternalLLMResponse{ID: "resp-1", Model: "gpt-4o"}

	adapter.Reset()

	if adapter.storedResponse != nil {
		t.Error("expected storedResponse to be nil after reset")
	}
}

func TestChatInbound_Reset_AllowsSecondAttempt(t *testing.T) {
	adapter := &ChatInbound{}

	// First attempt: accumulate chunks
	adapter.streamChunks = append(adapter.streamChunks,
		&model.InternalLLMResponse{ID: "1", Model: "wrong-model"},
	)

	// Reset before retry
	adapter.Reset()

	// Second attempt: accumulate new chunks
	adapter.streamChunks = append(adapter.streamChunks,
		&model.InternalLLMResponse{ID: "2", Model: "correct-model"},
	)

	resp, err := adapter.GetInternalResponse(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Model != "correct-model" {
		t.Errorf("expected model 'correct-model', got '%s'", resp.Model)
	}
}

func TestChatInbound_NoReset_CrossAttemptPollution(t *testing.T) {
	adapter := &ChatInbound{}

	// First attempt: accumulate chunks (NOT reset)
	adapter.streamChunks = append(adapter.streamChunks,
		&model.InternalLLMResponse{ID: "1", Model: "wrong-model"},
	)

	// Second attempt without reset: chunks accumulate
	adapter.streamChunks = append(adapter.streamChunks,
		&model.InternalLLMResponse{ID: "2", Model: "correct-model"},
	)

	resp, err := adapter.GetInternalResponse(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// Without reset, the model from the LAST chunk wins (both are aggregated)
	// This demonstrates the pollution problem
	if resp.Model == "wrong-model" {
		t.Log("confirmed: without Reset, first attempt's model can pollute the response")
	}
}

func TestEmbeddingInbound_Reset_ClearsStoredResponse(t *testing.T) {
	adapter := &EmbeddingInbound{}
	adapter.storedResponse = &model.InternalLLMResponse{ID: "emb-1"}

	adapter.Reset()

	if adapter.storedResponse != nil {
		t.Error("expected storedResponse to be nil after reset")
	}
}

func TestResponseInbound_Reset_ClearsAllState(t *testing.T) {
	adapter := &ResponseInbound{}

	// Simulate first attempt: set various state
	adapter.hasResponseCreated = true
	adapter.hasMessageItemStarted = true
	adapter.hasFinished = true
	adapter.responseID = "resp-1"
	adapter.model = "gpt-4"
	adapter.outputIndex = 3
	adapter.contentIndex = 2
	adapter.sequenceNumber = 10
	adapter.currentItemID = "item-1"
	adapter.accumulatedText.WriteString("hello")
	adapter.accumulatedReasoning.WriteString("thinking")
	adapter.toolCalls = map[int]*model.ToolCall{0: {ID: "tc-1"}}
	adapter.toolCallItemStarted = map[int]bool{0: true}
	adapter.toolCallOutputIndex = map[int]int{0: 1}
	adapter.usage = &model.Usage{PromptTokens: 100, CompletionTokens: 50}
	adapter.streamChunks = append(adapter.streamChunks, &model.InternalLLMResponse{ID: "1"})
	adapter.storedResponse = &model.InternalLLMResponse{ID: "resp-1"}

	adapter.Reset()

	if adapter.hasResponseCreated {
		t.Error("hasResponseCreated should be false")
	}
	if adapter.hasMessageItemStarted {
		t.Error("hasMessageItemStarted should be false")
	}
	if adapter.hasFinished {
		t.Error("hasFinished should be false")
	}
	if adapter.responseID != "" {
		t.Errorf("responseID should be empty, got %s", adapter.responseID)
	}
	if adapter.model != "" {
		t.Errorf("model should be empty, got %s", adapter.model)
	}
	if adapter.outputIndex != 0 || adapter.contentIndex != 0 || adapter.sequenceNumber != 0 {
		t.Error("indices should be 0")
	}
	if adapter.currentItemID != "" {
		t.Errorf("currentItemID should be empty, got %s", adapter.currentItemID)
	}
	if adapter.accumulatedText.Len() != 0 {
		t.Error("accumulatedText should be empty")
	}
	if adapter.accumulatedReasoning.Len() != 0 {
		t.Error("accumulatedReasoning should be empty")
	}
	if adapter.toolCalls != nil {
		t.Error("toolCalls should be nil")
	}
	if adapter.usage != nil {
		t.Error("usage should be nil")
	}
	if len(adapter.streamChunks) != 0 {
		t.Errorf("streamChunks should be empty, got %d", len(adapter.streamChunks))
	}
	if adapter.storedResponse != nil {
		t.Error("storedResponse should be nil")
	}
}
