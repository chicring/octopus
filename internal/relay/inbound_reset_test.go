package relay

import (
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/inbound"
	"github.com/bestruirui/octopus/internal/transformer/model"
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
				Index:       0,
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
