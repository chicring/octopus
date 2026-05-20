package anthropic

import (
	"context"
	"encoding/json"
	"strings"
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
	if _, ok := thinkingBlock["signature"]; ok {
		t.Fatalf("thinking block must not synthesize signature for DeepSeek reasoning_content: %v", thinkingBlock)
	}

	toolBlock, _ := contentArr[1].(map[string]any)
	if toolBlock["type"] != "tool_use" || toolBlock["id"] != "call_date" {
		t.Fatalf("second block = %v, want tool_use block", toolBlock)
	}
}

func TestMessagesInbound_StreamReasoningContentDoesNotSynthesizeSignature(t *testing.T) {
	adapter := &MessagesInbound{}
	thinking := "Need the current date before calling weather."

	resp, err := adapter.TransformStream(context.Background(), &model.InternalLLMResponse{
		ID:    "msg_deepseek",
		Model: "deepseek-reasoner",
		Choices: []model.Choice{
			{
				Index: 0,
				Delta: &model.Message{
					Role:             "assistant",
					ReasoningContent: &thinking,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("TransformStream error: %v", err)
	}

	events := parseSSEEvents(t, resp)
	for _, event := range events {
		if event["type"] != "content_block_start" {
			continue
		}
		block, ok := event["content_block"].(map[string]any)
		if !ok || block["type"] != "thinking" {
			continue
		}
		if _, ok := block["signature"]; ok {
			t.Fatalf("thinking stream start must not synthesize signature: %v", block)
		}
		return
	}
	t.Fatalf("missing thinking content_block_start in events: %v", events)
}

func TestMessagesInbound_TransformRequestPreservesEmptyThinkingForToolUseReplay(t *testing.T) {
	adapter := &MessagesInbound{}
	body := []byte(`{
		"model":"deepseek-v4-flash",
		"max_tokens":1024,
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[
			{
				"role":"assistant",
				"content":[
					{"type":"thinking","thinking":""},
					{"type":"tool_use","id":"call_date","name":"get_date","input":{}}
				]
			},
			{
				"role":"user",
				"content":[
					{"type":"tool_result","tool_use_id":"call_date","content":"2026-05-20"},
					{"type":"text","text":"continue"}
				]
			}
		]
	}`)

	internalReq, err := adapter.TransformRequest(context.Background(), body)
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}
	if len(internalReq.Messages) < 2 {
		t.Fatalf("messages len = %d, want at least 2", len(internalReq.Messages))
	}

	assistant := internalReq.Messages[0]
	if assistant.Role != "assistant" {
		t.Fatalf("message[0].role = %q, want assistant", assistant.Role)
	}
	if assistant.ReasoningContent == nil {
		t.Fatalf("reasoning_content = nil, want explicit empty string")
	}
	if *assistant.ReasoningContent != "" {
		t.Fatalf("reasoning_content = %q, want empty string", *assistant.ReasoningContent)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "call_date" {
		t.Fatalf("tool_calls = %+v, want call_date", assistant.ToolCalls)
	}
}

func TestMessagesInbound_StreamDoneFinalizesWithoutUsageChunk(t *testing.T) {
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
						Role:             "assistant",
						ReasoningContent: &thinking,
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
								Index: 0,
								ID:    "call_date",
								Type:  "function",
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
		{Object: "[DONE]"},
	}

	var clientBody []byte
	for _, chunk := range chunks {
		data, err := adapter.TransformStream(context.Background(), chunk)
		if err != nil {
			t.Fatalf("TransformStream error: %v", err)
		}
		clientBody = append(clientBody, data...)
	}

	events := parseSSEEvents(t, clientBody)
	hasMessageDelta := false
	hasMessageStop := false
	for _, event := range events {
		switch event["type"] {
		case "message_delta":
			hasMessageDelta = true
			delta, _ := event["delta"].(map[string]any)
			if delta["stop_reason"] != "tool_use" {
				t.Fatalf("message_delta stop_reason = %v, want tool_use", delta["stop_reason"])
			}
		case "message_stop":
			hasMessageStop = true
		}
	}
	if !hasMessageDelta || !hasMessageStop {
		t.Fatalf("stream must finalize on [DONE] without usage chunk; message_delta=%t message_stop=%t events=%v", hasMessageDelta, hasMessageStop, events)
	}

	internalResp, err := adapter.GetInternalResponse(context.Background())
	if err != nil {
		t.Fatalf("GetInternalResponse error: %v", err)
	}
	msg := internalResp.Choices[0].Message
	if msg.ReasoningContent == nil || *msg.ReasoningContent != thinking {
		t.Fatalf("aggregated reasoning_content = %v, want %q", msg.ReasoningContent, thinking)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "call_date" {
		t.Fatalf("aggregated tool_calls = %+v, want call_date", msg.ToolCalls)
	}
}

func parseSSEEvents(t *testing.T, data []byte) []map[string]any {
	t.Helper()

	parts := strings.Split(string(data), "\n\n")
	events := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		for _, line := range strings.Split(part, "\n") {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &event); err != nil {
				t.Fatalf("event is not valid JSON: %v", err)
			}
			events = append(events, event)
		}
	}
	return events
}
