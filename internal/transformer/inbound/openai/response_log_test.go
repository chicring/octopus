package openai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

// TestResponseInbound_StreamGetInternalResponseThenTransformResponse
// 验证流式场景下：streamChunks 聚合 → GetInternalResponse → TransformResponse
// 产出的 ClientResponseBody 是正确的 Responses API 格式
func TestResponseInbound_StreamGetInternalResponseThenTransformResponse(t *testing.T) {
	adapter := &ResponseInbound{}

	content := "Hello, world!"
	finishReason := "stop"

	// 模拟流式 chunk：第一个 chunk 带 role，第二个带 content，第三个带 finish_reason
	chunks := []*model.InternalLLMResponse{
		{
			ID:     "chatcmpl-123",
			Object: "chat.completion.chunk",
			Model:  "gpt-4o",
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
			ID:     "chatcmpl-123",
			Object: "chat.completion.chunk",
			Model:  "gpt-4o",
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
			ID:     "chatcmpl-123",
			Object: "chat.completion.chunk",
			Model:  "gpt-4o",
			Choices: []model.Choice{
				{
					Index:        0,
					FinishReason: &finishReason,
				},
			},
		},
	}

	// 模拟 inbound TransformStream 保存 streamChunks
	for _, chunk := range chunks {
		adapter.streamChunks = append(adapter.streamChunks, chunk)
	}

	// 1. GetInternalResponse 聚合
	internalResp, err := adapter.GetInternalResponse(context.Background())
	if err != nil {
		t.Fatalf("GetInternalResponse error: %v", err)
	}
	if internalResp == nil {
		t.Fatal("GetInternalResponse returned nil")
	}

	// 验证聚合后的 InternalResponse 内容完整
	if len(internalResp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(internalResp.Choices))
	}
	msg := internalResp.Choices[0].Message
	if msg == nil {
		t.Fatal("message is nil")
	}
	if msg.Role != "assistant" {
		t.Errorf("role = %q, want assistant", msg.Role)
	}
	if msg.Content.Content == nil || *msg.Content.Content != content {
		t.Errorf("content = %v, want %q", msg.Content.Content, content)
	}

	// 2. TransformResponse 转回客户端格式
	// 注意：GetInternalResponse 会清空 streamChunks 并设置 storedResponse
	// 所以这里 storedResponse 已经被设置了
	clientBody, err := adapter.TransformResponse(context.Background(), internalResp)
	if err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}
	if len(clientBody) == 0 {
		t.Fatal("TransformResponse returned empty body")
	}

	// 验证客户端格式是 Responses API 格式
	var resp map[string]any
	if err := json.Unmarshal(clientBody, &resp); err != nil {
		t.Fatalf("client body is not valid JSON: %v", err)
	}

	if resp["object"] != "response" {
		t.Errorf("object = %v, want response", resp["object"])
	}
	if resp["model"] != "gpt-4o" {
		t.Errorf("model = %v, want gpt-4o", resp["model"])
	}

	output, ok := resp["output"].([]any)
	if !ok {
		t.Fatalf("output is not an array, got %T", resp["output"])
	}

	// 应该有 message output item
	foundMessage := false
	for _, item := range output {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if itemMap["type"] == "message" {
			foundMessage = true
			if itemMap["role"] != "assistant" {
				t.Errorf("message role = %v, want assistant", itemMap["role"])
			}
		}
	}
	if !foundMessage {
		t.Error("no message item found in output")
	}

	t.Logf("ClientResponseBody: %s", string(clientBody))
}

// TestResponseInbound_NonStreamTransformResponse
// 验证非流式场景下 TransformResponse 产出正确的 Responses API 格式
func TestResponseInbound_NonStreamTransformResponse(t *testing.T) {
	adapter := &ResponseInbound{}

	content := "Hi there!"
	finishReason := "stop"

	internalResp := &model.InternalLLMResponse{
		ID:      "chatcmpl-456",
		Object:  "chat.completion",
		Model:   "gpt-4o",
		Created: 1234567890,
		Choices: []model.Choice{
			{
				Index:        0,
				Message:      &model.Message{Role: "assistant", Content: model.MessageContent{Content: &content}},
				FinishReason: &finishReason,
			},
		},
		Usage: &model.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	clientBody, err := adapter.TransformResponse(context.Background(), internalResp)
	if err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(clientBody, &resp); err != nil {
		t.Fatalf("client body is not valid JSON: %v", err)
	}

	if resp["object"] != "response" {
		t.Errorf("object = %v, want response", resp["object"])
	}

	output, ok := resp["output"].([]any)
	if !ok {
		t.Fatalf("output is not an array, got %T", resp["output"])
	}

	foundMessage := false
	for _, item := range output {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if itemMap["type"] == "message" {
			foundMessage = true
			// 验证 message content 包含文本
			contentRaw, hasContent := itemMap["content"]
			if !hasContent {
				t.Error("message has no content field")
			}
			t.Logf("message content: %v", contentRaw)
		}
	}
	if !foundMessage {
		t.Error("no message item found in output")
	}

	t.Logf("ClientResponseBody: %s", string(clientBody))
}

// TestResponseInbound_StreamWithToolCalls
// 验证流式场景下 tool_calls 也被正确聚合和转换
func TestResponseInbound_StreamWithToolCalls(t *testing.T) {
	adapter := &ResponseInbound{}

	finishReason := "tool_calls"

	chunks := []*model.InternalLLMResponse{
		{
			ID:    "chatcmpl-789",
			Model: "gpt-4o",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						Role: "assistant",
						ToolCalls: []model.ToolCall{
							{
								ID:   "call_abc",
								Type: "function",
								Function: model.FunctionCall{
									Name:      "get_weather",
									Arguments: "",
								},
							},
						},
					},
				},
			},
		},
		{
			ID:    "chatcmpl-789",
			Model: "gpt-4o",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						ToolCalls: []model.ToolCall{
							{
								ID:   "call_abc",
								Type: "function",
								Function: model.FunctionCall{
									Arguments: `{"city":"Beijing"}`,
								},
							},
						},
					},
				},
			},
		},
		{
			ID:    "chatcmpl-789",
			Model: "gpt-4o",
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

	// 验证 tool calls 被聚合
	if len(internalResp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(internalResp.Choices))
	}
	msg := internalResp.Choices[0].Message
	if msg == nil {
		t.Fatal("message is nil")
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("tool call name = %q, want get_weather", msg.ToolCalls[0].Function.Name)
	}
	if msg.ToolCalls[0].Function.Arguments != `{"city":"Beijing"}` {
		t.Errorf("tool call arguments = %q, want {\"city\":\"Beijing\"}", msg.ToolCalls[0].Function.Arguments)
	}

	// 转回客户端格式
	clientBody, err := adapter.TransformResponse(context.Background(), internalResp)
	if err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(clientBody, &resp); err != nil {
		t.Fatalf("client body is not valid JSON: %v", err)
	}

	output, ok := resp["output"].([]any)
	if !ok {
		t.Fatalf("output is not an array, got %T", resp["output"])
	}

	foundToolCall := false
	for _, item := range output {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if itemMap["type"] == "function_call" {
			foundToolCall = true
			if itemMap["name"] != "get_weather" {
				t.Errorf("function_call name = %v, want get_weather", itemMap["name"])
			}
		}
	}
	if !foundToolCall {
		t.Error("no function_call item found in output")
	}

	t.Logf("ClientResponseBody: %s", string(clientBody))
}

// TestResponseInbound_StreamWithReasoningContent
// 验证流式场景下 reasoning_content 也被正确聚合和转换
func TestResponseInbound_StreamWithReasoningContent(t *testing.T) {
	adapter := &ResponseInbound{}

	reasoning := "Let me think about this..."
	content := "The answer is 42."
	finishReason := "stop"

	chunks := []*model.InternalLLMResponse{
		{
			ID:    "chatcmpl-reasoning",
			Model: "o3",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						Role:             "assistant",
						ReasoningContent: &reasoning,
					},
				},
			},
		},
		{
			ID:    "chatcmpl-reasoning",
			Model: "o3",
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
			ID:    "chatcmpl-reasoning",
			Model: "o3",
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
	if msg.ReasoningContent == nil || *msg.ReasoningContent != reasoning {
		t.Errorf("reasoning_content = %v, want %q", msg.ReasoningContent, reasoning)
	}
	if msg.Content.Content == nil || *msg.Content.Content != content {
		t.Errorf("content = %v, want %q", msg.Content.Content, content)
	}

	clientBody, err := adapter.TransformResponse(context.Background(), internalResp)
	if err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(clientBody, &resp); err != nil {
		t.Fatalf("client body is not valid JSON: %v", err)
	}

	output, ok := resp["output"].([]any)
	if !ok {
		t.Fatalf("output is not an array, got %T", resp["output"])
	}

	foundReasoning := false
	foundMessage := false
	for _, item := range output {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if itemMap["type"] == "reasoning" {
			foundReasoning = true
		}
		if itemMap["type"] == "message" {
			foundMessage = true
		}
	}
	if !foundReasoning {
		t.Error("no reasoning item found in output")
	}
	if !foundMessage {
		t.Error("no message item found in output")
	}

	t.Logf("ClientResponseBody: %s", string(clientBody))
}

func TestResponseInbound_StreamCompletedIncludesOutput(t *testing.T) {
	adapter := &ResponseInbound{}

	content := "Hello from stream"
	finishReason := "stop"
	usage := &model.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7}

	chunks := []*model.InternalLLMResponse{
		{
			ID:      "resp-123",
			Object:  "chat.completion.chunk",
			Model:   "gpt-4o",
			Created: 123,
			Choices: []model.Choice{{Index: 0, Delta: &model.Message{Role: "assistant"}}},
		},
		{
			ID:      "resp-123",
			Object:  "chat.completion.chunk",
			Model:   "gpt-4o",
			Created: 123,
			Choices: []model.Choice{{Index: 0, Delta: &model.Message{Content: model.MessageContent{Content: &content}}}},
		},
		{
			ID:      "resp-123",
			Object:  "chat.completion.chunk",
			Model:   "gpt-4o",
			Created: 123,
			Choices: []model.Choice{{Index: 0, FinishReason: &finishReason}},
			Usage:   usage,
		},
	}

	var completed *ResponsesResponse
	for _, chunk := range chunks {
		data, err := adapter.TransformStream(context.Background(), chunk)
		if err != nil {
			t.Fatalf("TransformStream error: %v", err)
		}
		for _, line := range strings.Split(string(data), "\n\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event ResponsesStreamEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatalf("event is not valid JSON: %v", err)
			}
			if event.Type == "response.completed" {
				completed = event.Response
			}
		}
	}

	if completed == nil {
		t.Fatal("response.completed was not emitted")
	}
	if len(completed.Output) == 0 {
		t.Fatal("response.completed output is empty")
	}
	item := completed.Output[0]
	if item.Type != "message" || item.Content == nil || len(item.Content.Items) != 1 {
		t.Fatalf("unexpected completed output item: %+v", item)
	}
	if item.Content.Items[0].Text == nil || *item.Content.Items[0].Text != content {
		t.Fatalf("completed output text = %v, want %q", item.Content.Items[0].Text, content)
	}
}
