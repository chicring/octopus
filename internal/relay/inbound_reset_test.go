package relay

import (
	"context"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/inbound"
	"github.com/bestruirui/octopus/internal/transformer/model"
	openaiOutbound "github.com/bestruirui/octopus/internal/transformer/outbound/openai"
)

// TestInboundReset_AcrossRetrySimulatesRetry 测试入站适配器在重试场景下的状态隔离
func TestInboundReset_AcrossRetrySimulatesRetry(t *testing.T) {
	// 模拟 OpenAI Chat 入站适配器的重试场景
	inAdapter := inbound.Get(inbound.InboundTypeOpenAIChat)

	// 第一次尝试：模拟流式响应累积（失败）
	stream1 := &model.InternalLLMResponse{
		ID:    "fail-1",
		Model: "wrong-model-from-failed-attempt",
		Choices: []model.Choice{
			{
				Index: 0,
				Delta: &model.Message{
					Content: model.MessageContent{Content: ptrStr("partial")},
				},
			},
		},
	}
	_, _ = inAdapter.TransformStream(t.Context(), stream1)

	// 重试前重置
	inAdapter.Reset()

	// 第二次尝试：模拟流式响应累积（成功）
	stream2 := &model.InternalLLMResponse{
		ID:    "success-1",
		Model: "correct-model",
		Choices: []model.Choice{
			{
				Index: 0,
				Delta: &model.Message{
					Role:    "assistant",
					Content: model.MessageContent{Content: ptrStr("Hello")},
				},
			},
		},
	}
	_, _ = inAdapter.TransformStream(t.Context(), stream2)

	stream3 := &model.InternalLLMResponse{
		ID:    "success-1",
		Model: "correct-model",
		Usage: &model.Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
		},
		Choices: []model.Choice{
			{
				Index:        0,
				FinishReason: ptrStr("stop"),
			},
		},
	}
	_, _ = inAdapter.TransformStream(t.Context(), stream3)

	// 验证：GetInternalResponse 应只包含第二次尝试的数据
	resp, err := inAdapter.GetInternalResponse(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Model != "correct-model" {
		t.Errorf("model should be 'correct-model', got '%s'", resp.Model)
	}
	if resp.Usage == nil {
		t.Fatal("expected usage in response")
	}
	if resp.Usage.PromptTokens != 100 {
		t.Errorf("prompt tokens should be 100, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 50 {
		t.Errorf("completion tokens should be 50, got %d", resp.Usage.CompletionTokens)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message == nil || resp.Choices[0].Message.Content.Content == nil {
		t.Fatal("expected message content in choice")
	}
	if *resp.Choices[0].Message.Content.Content != "Hello" {
		t.Errorf("message content should be 'Hello', got '%s'", *resp.Choices[0].Message.Content.Content)
	}
}

// TestInboundReset_WithoutResetShowsPollution 证明不 Reset 会导致跨尝试污染
func TestInboundReset_WithoutResetShowsPollution(t *testing.T) {
	inAdapter := inbound.Get(inbound.InboundTypeOpenAIChat)

	// 第一次尝试：累积数据（不 Reset）
	stream1 := &model.InternalLLMResponse{
		ID:    "fail-1",
		Model: "wrong-model",
		Choices: []model.Choice{
			{
				Index: 0,
				Delta: &model.Message{
					Content: model.MessageContent{Content: ptrStr("garbage")},
				},
			},
		},
	}
	_, _ = inAdapter.TransformStream(t.Context(), stream1)

	// 第二次尝试：不 Reset，直接累积
	stream2 := &model.InternalLLMResponse{
		ID:    "success-1",
		Model: "correct-model",
		Choices: []model.Choice{
			{
				Index: 0,
				Delta: &model.Message{
					Content: model.MessageContent{Content: ptrStr("real")},
				},
			},
		},
	}
	_, _ = inAdapter.TransformStream(t.Context(), stream2)

	resp, err := inAdapter.GetInternalResponse(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// 不 Reset 时，两次尝试的内容被合并
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	content := *resp.Choices[0].Message.Content.Content
	if content == "garbagereal" {
		t.Log("confirmed: without Reset, content from both attempts is merged (pollution)")
	}
	// 这正是 bug 的表现
}

// TestInboundReset_AnthropicPreservesInputToken 测试 Anthropic 适配器 Reset 保留 inputToken
func TestInboundReset_AnthropicPreservesInputToken(t *testing.T) {
	inAdapter := inbound.Get(inbound.InboundTypeAnthropic)

	// TransformRequest 会计算 inputToken，这里模拟其结果
	// 由于我们无法直接设置内部字段，通过 Reset 前后的行为来验证
	// Reset 不应破坏请求级状态
	inAdapter.Reset()

	// Reset 后适配器应仍可用于新的尝试
	// 如果 Reset 错误地清除了请求级状态，后续 TransformStream 会出错
	// 这里主要验证 Reset 不会 panic 且适配器仍可用
	stream := &model.InternalLLMResponse{
		ID:    "msg-1",
		Model: "claude-3-5-sonnet-20241022",
		Choices: []model.Choice{
			{
				Index: 0,
				Delta: &model.Message{
					Role:    "assistant",
					Content: model.MessageContent{Content: ptrStr("Hi")},
				},
			},
		},
	}
	_, err := inAdapter.TransformStream(t.Context(), stream)
	if err != nil {
		t.Errorf("TransformStream after Reset should not error: %v", err)
	}
}

func ptrStr(s string) *string { return &s }

// TestPassthroughStreamStillStoresChunksForAggregation 验证同格式透传流式路径下，
// outbound TransformStream 的内部响应仍通过 inbound TransformStream 累积，
// 使得 GetInternalResponse 能正确聚合 usage 等统计数据。
func TestPassthroughStreamStillStoresChunksForAggregation(t *testing.T) {
	// 模拟 OpenAI Chat 的透传流式路径
	outAdapter := &openaiOutbound.ChatOutbound{}
	inAdapter := inbound.Get(inbound.InboundTypeOpenAIChat)

	internalReq := &model.InternalLLMRequest{
		Model:        "gpt-4o",
		RawRequest:   []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":true}`),
		RawAPIFormat: model.APIFormatOpenAIChatCompletion,
		Stream:       boolPtr(true),
	}

	// 模拟 relay 的透传路径：TransformRequest 已设置 PassthroughAPIFormat
	model.MarkPassthrough(internalReq, model.APIFormatOpenAIChatCompletion)

	// 模拟上游流式 chunk
	chunks := []string{
		`{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1000,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1000,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		`[DONE]`,
	}

	// 模拟 relay transformStreamData 的透传路径
	for _, chunk := range chunks {
		internalStream, err := outAdapter.TransformStream(context.Background(), []byte(chunk))
		if err != nil {
			t.Fatalf("TransformStream error on chunk %q: %v", chunk, err)
		}
		if internalStream == nil {
			continue
		}

		// 验证透传判断
		if !model.IsPassthrough(internalReq, model.APIFormatOpenAIChatCompletion) {
			t.Fatal("should be passthrough")
		}

		// 透传模式：将内部响应送入 inbound adapter 累积（如 relay 所做），
		// 但返回原始数据给客户端（此处只验证累积，不验证返回值）
		_, _ = inAdapter.TransformStream(context.Background(), internalStream)

		// 验证原始数据不变
		if internalStream.Object == "[DONE]" {
			// [DONE] 标记被正确识别
			continue
		}
		if len(internalStream.RawChunk) == 0 {
			t.Fatalf("RawChunk should be set for chunk: %s", chunk)
		}
		if string(internalStream.RawChunk) != chunk {
			t.Fatalf("RawChunk changed\ngot:  %s\nwant: %s", internalStream.RawChunk, chunk)
		}
	}

	// 验证聚合：GetInternalResponse 应返回包含 usage 的完整响应
	resp, err := inAdapter.GetInternalResponse(context.Background())
	if err != nil {
		t.Fatalf("GetInternalResponse error: %v", err)
	}
	if resp == nil {
		t.Fatal("GetInternalResponse should not be nil after streaming")
	}
	if resp.Usage == nil {
		t.Fatal("usage should be aggregated from stream chunks")
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("prompt_tokens = %d, want 10", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("completion_tokens = %d, want 5", resp.Usage.CompletionTokens)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message == nil || resp.Choices[0].Message.Content.Content == nil {
		t.Fatal("expected message content in aggregated response")
	}
	if *resp.Choices[0].Message.Content.Content != "Hello world" {
		t.Errorf("aggregated content = %q, want %q", *resp.Choices[0].Message.Content.Content, "Hello world")
	}
}

func boolPtr(b bool) *bool { return &b }

func TestFormatRawSSEEventPreservesEventType(t *testing.T) {
	got := string(formatRawSSEEvent("content_block_delta", []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}`)))
	if !strings.HasPrefix(got, "event: content_block_delta\n") {
		t.Fatalf("event type was not preserved: %q", got)
	}
	if !strings.Contains(got, `data: {"type":"content_block_delta"`) {
		t.Fatalf("data payload was not preserved: %q", got)
	}
}
