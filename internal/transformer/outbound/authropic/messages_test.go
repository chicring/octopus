package authropic

import (
	"context"
	"testing"

	"github.com/samber/lo"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

func TestTransformStream_ErrorEvent(t *testing.T) {
	o := &MessageOutbound{}

	// Anthropic stream error event: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}
	eventData := []byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)

	result, err := o.TransformStream(context.Background(), eventData)

	if result != nil {
		t.Errorf("expected nil result for error event, got %v", result)
	}
	if err == nil {
		t.Fatal("expected error for error event, got nil")
	}

	respErr, ok := err.(*model.ResponseError)
	if !ok {
		t.Fatalf("expected ResponseError, got %T: %v", err, err)
	}
	if respErr.Detail.Type != "overloaded_error" {
		t.Errorf("error type: got %s, want overloaded_error", respErr.Detail.Type)
	}
	if respErr.Detail.Message != "Overloaded" {
		t.Errorf("error message: got %s, want Overloaded", respErr.Detail.Message)
	}
}

func TestTransformStream_ErrorEvent_RateLimit(t *testing.T) {
	o := &MessageOutbound{}

	eventData := []byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Rate limit exceeded"}}`)

	result, err := o.TransformStream(context.Background(), eventData)

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}

	respErr, ok := err.(*model.ResponseError)
	if !ok {
		t.Fatalf("expected ResponseError, got %T: %v", err, err)
	}
	if respErr.Detail.Type != "rate_limit_error" {
		t.Errorf("error type: got %s, want rate_limit_error", respErr.Detail.Type)
	}
}

func TestTransformStream_ErrorEvent_InvalidJSON(t *testing.T) {
	o := &MessageOutbound{}

	// type is "error" but can't parse the error detail
	eventData := []byte(`{"type":"error","malformed":"data"}`)

	result, err := o.TransformStream(context.Background(), eventData)

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if err == nil {
		t.Fatal("expected error for malformed error event, got nil")
	}

	// Should fall through to the generic error message
	if !isResponseError(err) {
		// Not a ResponseError, just a generic fmt.Errorf
		if err.Error() == "" {
			t.Errorf("expected non-empty error message")
		}
	}
}

func TestTransformStream_PingEvent(t *testing.T) {
	o := &MessageOutbound{}

	eventData := []byte(`{"type":"ping"}`)
	result, err := o.TransformStream(context.Background(), eventData)

	if result != nil {
		t.Errorf("expected nil result for ping, got %v", result)
	}
	if err != nil {
		t.Errorf("expected nil error for ping, got %v", err)
	}
}

func TestTransformStream_ContentBlockStop(t *testing.T) {
	o := &MessageOutbound{}

	eventData := []byte(`{"type":"content_block_stop","index":0}`)
	result, err := o.TransformStream(context.Background(), eventData)

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func isResponseError(err error) bool {
	_, ok := err.(*model.ResponseError)
	return ok
}

func TestConvertAssistantMessage_ThinkingModeEmptyReasoningContent(t *testing.T) {
	// When thinking is enabled and ReasoningContent is nil,
	// the assistant message must still include a thinking block.
	msg := model.Message{
		Role: "assistant",
		Content: model.MessageContent{
			Content: lo.ToPtr("hello"),
		},
		ReasoningContent: nil, // field absent
	}

	result := convertAssistantMessage(msg, true)
	if len(result) != 1 {
		t.Fatalf("expected 1 message param, got %d", len(result))
	}

	blocks := result[0].Content.MultipleContent
	// Should have: thinking block + text block
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks (thinking + text), got %d", len(blocks))
	}
	if blocks[0].Type != "thinking" {
		t.Errorf("first block type: got %s, want thinking", blocks[0].Type)
	}
	if blocks[0].Thinking == nil || *blocks[0].Thinking != "" {
		t.Errorf("thinking content: got %v, want empty string", blocks[0].Thinking)
	}
	if blocks[1].Type != "text" {
		t.Errorf("second block type: got %s, want text", blocks[1].Type)
	}
}

func TestConvertAssistantMessage_ThinkingModeEmptyStringReasoningContent(t *testing.T) {
	// When thinking is enabled and ReasoningContent is explicitly empty string "",
	// it must be preserved as a thinking block (not dropped).
	msg := model.Message{
		Role: "assistant",
		Content: model.MessageContent{
			Content: lo.ToPtr("hello"),
		},
		ReasoningContent: lo.ToPtr(""), // explicit empty string
	}

	result := convertAssistantMessage(msg, true)
	if len(result) != 1 {
		t.Fatalf("expected 1 message param, got %d", len(result))
	}

	blocks := result[0].Content.MultipleContent
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks (thinking + text), got %d", len(blocks))
	}
	if blocks[0].Type != "thinking" {
		t.Errorf("first block type: got %s, want thinking", blocks[0].Type)
	}
	if blocks[0].Thinking == nil || *blocks[0].Thinking != "" {
		t.Errorf("thinking content: got %v, want empty string", blocks[0].Thinking)
	}
}

func TestConvertAssistantMessage_ThinkingDisabledNoReasoningContent(t *testing.T) {
	// When thinking is disabled and ReasoningContent is nil,
	// no thinking block should be generated.
	msg := model.Message{
		Role: "assistant",
		Content: model.MessageContent{
			Content: lo.ToPtr("hello"),
		},
		ReasoningContent: nil,
	}

	result := convertAssistantMessage(msg, false)
	if len(result) != 1 {
		t.Fatalf("expected 1 message param, got %d", len(result))
	}

	// Should be simple content, not multiple content blocks
	if result[0].Content.Content == nil || *result[0].Content.Content != "hello" {
		t.Errorf("expected simple content 'hello', got %v", result[0].Content.Content)
	}
	if len(result[0].Content.MultipleContent) != 0 {
		t.Errorf("expected no multiple content blocks, got %d", len(result[0].Content.MultipleContent))
	}
}

func TestConvertAssistantWithToolCalls_ThinkingModeNilReasoningContent(t *testing.T) {
	// When thinking is enabled and assistant has tool calls but no ReasoningContent,
	// a thinking block must still be generated.
	msg := model.Message{
		Role: "assistant",
		Content: model.MessageContent{
			Content: lo.ToPtr(""),
		},
		ReasoningContent: nil,
		ToolCalls: []model.ToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: model.FunctionCall{
					Name:      "get_weather",
					Arguments: `{"city":"北京"}`,
				},
			},
		},
	}

	result := convertAssistantWithToolCalls(msg, true)
	if len(result) != 1 {
		t.Fatalf("expected 1 message param, got %d", len(result))
	}

	blocks := result[0].Content.MultipleContent
	// Should have: thinking block + tool_use block
	foundThinking := false
	foundToolUse := false
	for _, block := range blocks {
		if block.Type == "thinking" {
			foundThinking = true
			if block.Thinking == nil || *block.Thinking != "" {
				t.Errorf("thinking content: got %v, want empty string", block.Thinking)
			}
		}
		if block.Type == "tool_use" {
			foundToolUse = true
		}
	}
	if !foundThinking {
		t.Error("expected thinking block to be present")
	}
	if !foundToolUse {
		t.Error("expected tool_use block to be present")
	}
}

func TestConvertAssistantWithToolCalls_ThinkingModeEmptyStringReasoningContent(t *testing.T) {
	// When thinking is enabled and ReasoningContent is explicit empty string "",
	// the thinking block must preserve the empty string value.
	msg := model.Message{
		Role: "assistant",
		Content: model.MessageContent{
			Content: lo.ToPtr(""),
		},
		ReasoningContent: lo.ToPtr(""), // explicit empty string
		ReasoningSignature: lo.ToPtr("sig123"),
		ToolCalls: []model.ToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: model.FunctionCall{
					Name:      "get_weather",
					Arguments: `{"city":"北京"}`,
				},
			},
		},
	}

	result := convertAssistantWithToolCalls(msg, true)
	if len(result) != 1 {
		t.Fatalf("expected 1 message param, got %d", len(result))
	}

	blocks := result[0].Content.MultipleContent
	foundThinking := false
	for _, block := range blocks {
		if block.Type == "thinking" {
			foundThinking = true
			if block.Thinking == nil || *block.Thinking != "" {
				t.Errorf("thinking content: got %v, want empty string", block.Thinking)
			}
			if block.Signature == nil || *block.Signature != "sig123" {
				t.Errorf("signature: got %v, want sig123", block.Signature)
			}
		}
	}
	if !foundThinking {
		t.Error("expected thinking block to be present")
	}
}

func TestClearHelpFields_PreservesReasoningContent(t *testing.T) {
	// ClearHelpFields should NOT clear ReasoningContent and Reasoning.
	// Only ReasoningSignature should be cleared.
	emptyStr := ""
	msg := model.Message{
		Role:               "assistant",
		ReasoningContent:   &emptyStr,
		Reasoning:          lo.ToPtr("some reasoning"),
		ReasoningSignature: lo.ToPtr("sig123"),
	}

	msg.ClearHelpFields()

	if msg.ReasoningContent == nil {
		t.Error("ReasoningContent should not be cleared")
	} else if *msg.ReasoningContent != "" {
		t.Errorf("ReasoningContent value: got %s, want empty string", *msg.ReasoningContent)
	}
	if msg.Reasoning == nil {
		t.Error("Reasoning should not be cleared")
	} else if *msg.Reasoning != "some reasoning" {
		t.Errorf("Reasoning value: got %s, want 'some reasoning'", *msg.Reasoning)
	}
	if msg.ReasoningSignature != nil {
		t.Error("ReasoningSignature should be cleared")
	}
}