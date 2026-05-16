package anthropic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

// TestMessagesInbound_StreamGetInternalResponseThenTransformResponse
// 验证流式场景下：streamChunks 聚合 → GetInternalResponse → TransformResponse
// 产出的 ClientResponseBody 是正确的 Anthropic Messages 格式
func TestMessagesInbound_StreamGetInternalResponseThenTransformResponse(t *testing.T) {
	adapter := &MessagesInbound{}

	content := "Hello from Claude!"
	finishReason := "stop"

	chunks := []*model.InternalLLMResponse{
		{
			ID:    "msg_123",
			Model: "claude-sonnet-4-20250514",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						Role: "assistant",
					},
				},
			},
		},
		{
			ID:    "msg_123",
			Model: "claude-sonnet-4-20250514",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						Content: model.MessageContent{Content: &content},
					},
				},
			},
		},
		{
			ID:    "msg_123",
			Model: "claude-sonnet-4-20250514",
			Choices: []model.Choice{
				{
					Index:        0,
					FinishReason: &finishReason,
				},
			},
		},
	}

	for _, chunk := range chunks {
		adapter.streamChunks = append(adapter.streamChunks, chunk)
	}

	internalResp, err := adapter.GetInternalResponse(context.Background())
	if err != nil {
		t.Fatalf("GetInternalResponse error: %v", err)
	}
	if internalResp == nil {
		t.Fatal("GetInternalResponse returned nil")
	}

	msg := internalResp.Choices[0].Message
	if msg.Role != "assistant" {
		t.Errorf("role = %q, want assistant", msg.Role)
	}
	if msg.Content.Content == nil || *msg.Content.Content != content {
		t.Errorf("content = %v, want %q", msg.Content.Content, content)
	}

	clientBody, err := adapter.TransformResponse(context.Background(), internalResp)
	if err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}
	if len(clientBody) == 0 {
		t.Fatal("TransformResponse returned empty body")
	}

	var resp map[string]any
	if err := json.Unmarshal(clientBody, &resp); err != nil {
		t.Fatalf("client body is not valid JSON: %v", err)
	}

	if resp["type"] != "message" {
		t.Errorf("type = %v, want message", resp["type"])
	}
	if resp["role"] != "assistant" {
		t.Errorf("role = %v, want assistant", resp["role"])
	}

	contentArr, ok := resp["content"].([]any)
	if !ok {
		t.Fatalf("content is not an array, got %T", resp["content"])
	}

	foundText := false
	for _, item := range contentArr {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if itemMap["type"] == "text" {
			foundText = true
			if itemMap["text"] != content {
				t.Errorf("text = %v, want %q", itemMap["text"], content)
			}
		}
	}
	if !foundText {
		t.Error("no text block found in content")
	}

	t.Logf("ClientResponseBody: %s", string(clientBody))
}

// TestMessagesInbound_StreamWithThinkingAndToolUse
// 验证流式场景下 thinking + tool_use 也被正确聚合和转换
func TestMessagesInbound_StreamWithThinkingAndToolUse(t *testing.T) {
	adapter := &MessagesInbound{}

	thinking := "I need to check the weather..."
	finishReason := "tool_calls"

	chunks := []*model.InternalLLMResponse{
		{
			ID:    "msg_456",
			Model: "claude-sonnet-4-20250514",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						Role:             "assistant",
						ReasoningContent: &thinking,
					},
				},
			},
		},
		{
			ID:    "msg_456",
			Model: "claude-sonnet-4-20250514",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						ToolCalls: []model.ToolCall{
							{
								ID:   "toolu_abc",
								Type: "function",
								Function: model.FunctionCall{
									Name:      "get_weather",
									Arguments: `{"city":"Tokyo"}`,
								},
							},
						},
					},
				},
			},
		},
		{
			ID:    "msg_456",
			Model: "claude-sonnet-4-20250514",
			Choices: []model.Choice{
				{
					Index:        0,
					FinishReason: &finishReason,
				},
			},
		},
	}

	for _, chunk := range chunks {
		adapter.streamChunks = append(adapter.streamChunks, chunk)
	}

	internalResp, err := adapter.GetInternalResponse(context.Background())
	if err != nil {
		t.Fatalf("GetInternalResponse error: %v", err)
	}

	msg := internalResp.Choices[0].Message
	if msg.ReasoningContent == nil || *msg.ReasoningContent != thinking {
		t.Errorf("reasoning_content = %v, want %q", msg.ReasoningContent, thinking)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("tool call name = %q, want get_weather", msg.ToolCalls[0].Function.Name)
	}

	clientBody, err := adapter.TransformResponse(context.Background(), internalResp)
	if err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(clientBody, &resp); err != nil {
		t.Fatalf("client body is not valid JSON: %v", err)
	}

	contentArr, ok := resp["content"].([]any)
	if !ok {
		t.Fatalf("content is not an array, got %T", resp["content"])
	}

	foundThinking := false
	foundToolUse := false
	for _, item := range contentArr {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if itemMap["type"] == "thinking" {
			foundThinking = true
		}
		if itemMap["type"] == "tool_use" {
			foundToolUse = true
			if itemMap["name"] != "get_weather" {
				t.Errorf("tool_use name = %v, want get_weather", itemMap["name"])
			}
		}
	}
	if !foundThinking {
		t.Error("no thinking block found in content")
	}
	if !foundToolUse {
		t.Error("no tool_use block found in content")
	}

	t.Logf("ClientResponseBody: %s", string(clientBody))
}

func TestMessagesInbound_StreamWithReasoningAliasAndToolUse(t *testing.T) {
	adapter := &MessagesInbound{}

	thinking := "Need the current date before calling weather."
	finishReason := "tool_calls"

	chunks := []*model.InternalLLMResponse{
		{
			ID:    "msg_deepseek",
			Model: "deepseek-reasoner",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						Role:      "assistant",
						Reasoning: &thinking,
					},
				},
			},
		},
		{
			ID:    "msg_deepseek",
			Model: "deepseek-reasoner",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						ToolCalls: []model.ToolCall{
							{
								ID:   "call_date",
								Type: "function",
								Function: model.FunctionCall{
									Name:      "get_date",
									Arguments: `{}`,
								},
							},
						},
					},
				},
			},
		},
		{
			ID:    "msg_deepseek",
			Model: "deepseek-reasoner",
			Choices: []model.Choice{
				{
					Index:        0,
					FinishReason: &finishReason,
				},
			},
		},
	}

	for _, chunk := range chunks {
		adapter.streamChunks = append(adapter.streamChunks, chunk)
	}

	internalResp, err := adapter.GetInternalResponse(context.Background())
	if err != nil {
		t.Fatalf("GetInternalResponse error: %v", err)
	}

	msg := internalResp.Choices[0].Message
	if msg.ReasoningContent == nil || *msg.ReasoningContent != thinking {
		t.Fatalf("reasoning_content = %v, want %q", msg.ReasoningContent, thinking)
	}

	clientBody, err := adapter.TransformResponse(context.Background(), internalResp)
	if err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(clientBody, &resp); err != nil {
		t.Fatalf("client body is not valid JSON: %v", err)
	}

	contentArr, ok := resp["content"].([]any)
	if !ok {
		t.Fatalf("content is not an array, got %T", resp["content"])
	}

	if len(contentArr) < 2 {
		t.Fatalf("content blocks = %v, want thinking and tool_use", contentArr)
	}

	thinkingBlock, _ := contentArr[0].(map[string]any)
	if thinkingBlock["type"] != "thinking" || thinkingBlock["thinking"] != thinking {
		t.Fatalf("first block = %v, want thinking block with reasoning content", thinkingBlock)
	}

	toolBlock, _ := contentArr[1].(map[string]any)
	if toolBlock["type"] != "tool_use" || toolBlock["id"] != "call_date" {
		t.Fatalf("second block = %v, want tool_use block", toolBlock)
	}
}
