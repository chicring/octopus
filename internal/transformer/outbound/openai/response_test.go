package openai

import (
	"context"
	"encoding/json"
	"testing"

	anthropicModel "github.com/bestruirui/octopus/internal/transformer/inbound/anthropic"
	"github.com/bestruirui/octopus/internal/transformer/model"
	authropic "github.com/bestruirui/octopus/internal/transformer/outbound/authropic"
	"github.com/bestruirui/octopus/internal/transformer/outbound/gemini"
)

func TestResponseOutboundCompletedWithoutStatusSetsFinishReason(t *testing.T) {
	out := &ResponseOutbound{}
	body := []byte(`{"type":"response.completed","response":{"id":"resp_123","model":"gpt-4o","created_at":123,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`)

	resp, err := out.TransformStream(context.Background(), body)
	if err != nil {
		t.Fatalf("TransformStream error: %v", err)
	}
	if resp == nil || len(resp.Choices) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Choices[0].FinishReason == nil || *resp.Choices[0].FinishReason != "stop" {
		t.Fatalf("finish_reason = %v, want stop", resp.Choices[0].FinishReason)
	}
	if resp.Choices[0].Message == nil || resp.Choices[0].Message.Content.Content == nil || *resp.Choices[0].Message.Content.Content != "hello" {
		t.Fatalf("message content was not preserved: %+v", resp.Choices[0].Message)
	}
}

func TestResponseOutboundPassthroughPreservesRequestBodyAndMarksRequest(t *testing.T) {
	out := &ResponseOutbound{}
	raw := []byte(`{"model":"gpt-4o","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}],"stream":true}`)
	internalReq := &model.InternalLLMRequest{
		Model:        "gpt-4o",
		RawRequest:   raw,
		RawAPIFormat: model.APIFormatOpenAIResponse,
	}
	req, err := out.TransformRequest(context.Background(), internalReq, "https://api.openai.com/v1", "key")
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}
	if !model.IsPassthrough(internalReq, model.APIFormatOpenAIResponse) {
		t.Fatal("passthrough was not marked on request")
	}

	var got json.RawMessage
	if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
		t.Fatalf("request body is not valid JSON: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("request body changed\ngot:  %s\nwant: %s", got, raw)
	}
}

func TestChatOutboundStreamKeepsRawChunkForPassthrough(t *testing.T) {
	out := &ChatOutbound{}
	raw := []byte(`{"id":"chatcmpl_123","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`)

	resp, err := out.TransformStream(context.Background(), raw)
	if err != nil {
		t.Fatalf("TransformStream error: %v", err)
	}
	if string(resp.RawChunk) != string(raw) {
		t.Fatalf("RawChunk changed\ngot:  %s\nwant: %s", resp.RawChunk, raw)
	}
}

func TestResponseOutboundStreamKeepsRawChunkForPassthrough(t *testing.T) {
	out := &ResponseOutbound{}
	raw := []byte(`{"type":"response.output_text.delta","item_id":"item_abc","output_index":0,"content_index":0,"delta":"hello"}`)

	resp, err := out.TransformStream(context.Background(), raw)
	if err != nil {
		t.Fatalf("TransformStream error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if string(resp.RawChunk) != string(raw) {
		t.Fatalf("RawChunk changed\ngot:  %s\nwant: %s", resp.RawChunk, raw)
	}
}

func TestResponseOutboundStreamKeepsRawChunkForUnhandledEvents(t *testing.T) {
	out := &ResponseOutbound{}
	raw := []byte(`{"type":"response.content_part.done","item_id":"item_abc","output_index":0,"content_index":0,"part":{"type":"output_text","text":"hello"}}`)

	resp, err := out.TransformStream(context.Background(), raw)
	if err != nil {
		t.Fatalf("TransformStream error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response for raw passthrough")
	}
	if string(resp.RawChunk) != string(raw) {
		t.Fatalf("RawChunk changed\ngot:  %s\nwant: %s", resp.RawChunk, raw)
	}
}

func TestAnthropicOutboundStreamKeepsRawChunkForPassthroughOnlyEvents(t *testing.T) {
	out := &authropic.MessageOutbound{}
	raw := []byte(`{"type":"ping"}`)

	resp, err := out.TransformStream(context.Background(), raw)
	if err != nil {
		t.Fatalf("TransformStream error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response for raw passthrough")
	}
	if string(resp.RawChunk) != string(raw) {
		t.Fatalf("RawChunk changed\ngot:  %s\nwant: %s", resp.RawChunk, raw)
	}
}

func TestResponseOutboundStreamCompletedOutputNonEmpty(t *testing.T) {
	out := &ResponseOutbound{}
	body := []byte(`{"type":"response.completed","response":{"id":"resp_456","model":"gpt-4o","created_at":456,"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi there"}]}],"usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8}}}`)

	resp, err := out.TransformStream(context.Background(), body)
	if err != nil {
		t.Fatalf("TransformStream error: %v", err)
	}
	if resp == nil || len(resp.Choices) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	// Verify output content is preserved
	choice := resp.Choices[0]
	if choice.Message == nil {
		t.Fatal("expected non-nil message")
	}
	if choice.Message.Content.Content == nil || *choice.Message.Content.Content != "hi there" {
		t.Fatalf("content = %v, want 'hi there'", choice.Message.Content.Content)
	}
	// Verify finish_reason
	if choice.FinishReason == nil || *choice.FinishReason != "stop" {
		t.Fatalf("finish_reason = %v, want stop", choice.FinishReason)
	}
	// Verify RawChunk is set
	if len(resp.RawChunk) == 0 {
		t.Fatal("RawChunk should be set for passthrough")
	}
}

func TestResponseOutboundSetsUpstreamRequestBody(t *testing.T) {
	out := &ResponseOutbound{}
	raw := []byte(`{"model":"gpt-4o","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}],"stream":true}`)
	internalReq := &model.InternalLLMRequest{
		Model:        "gpt-4o",
		RawRequest:   raw,
		RawAPIFormat: model.APIFormatOpenAIResponse,
	}

	_, err := out.TransformRequest(context.Background(), internalReq, "https://api.openai.com/v1", "key")
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}

	if len(internalReq.UpstreamRequestBody) == 0 {
		t.Fatal("UpstreamRequestBody should be set")
	}
	if string(internalReq.UpstreamRequestBody) != string(raw) {
		t.Fatalf("UpstreamRequestBody changed\ngot:  %s\nwant: %s", internalReq.UpstreamRequestBody, raw)
	}
}

func TestEmbeddingOutboundSetsUpstreamRequestBodyAndMarksPassthrough(t *testing.T) {
	out := &EmbeddingOutbound{}
	raw := []byte(`{"model":"text-embedding-3-small","input":"hello world"}`)
	internalReq := &model.InternalLLMRequest{
		Model:        "text-embedding-3-small",
		RawRequest:   raw,
		RawAPIFormat: model.APIFormatOpenAIEmbedding,
		EmbeddingInput: &model.EmbeddingInput{
			Single: strPtr("hello world"),
		},
	}

	_, err := out.TransformRequest(context.Background(), internalReq, "https://api.openai.com/v1", "key")
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}

	if !model.IsPassthrough(internalReq, model.APIFormatOpenAIEmbedding) {
		t.Fatal("passthrough was not marked for embedding")
	}
	if len(internalReq.UpstreamRequestBody) == 0 {
		t.Fatal("UpstreamRequestBody should be set")
	}
	if string(internalReq.UpstreamRequestBody) != string(raw) {
		t.Fatalf("UpstreamRequestBody changed\ngot:  %s\nwant: %s", internalReq.UpstreamRequestBody, raw)
	}
}

func TestChatOutboundSetsUpstreamRequestBody(t *testing.T) {
	out := &ChatOutbound{}
	raw := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	internalReq := &model.InternalLLMRequest{
		Model:        "gpt-4o",
		RawRequest:   raw,
		RawAPIFormat: model.APIFormatOpenAIChatCompletion,
		Messages: []model.Message{
			{Role: "user", Content: model.MessageContent{Content: strPtr("hello")}},
		},
	}

	_, err := out.TransformRequest(context.Background(), internalReq, "https://api.openai.com/v1", "key")
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}

	if !model.IsPassthrough(internalReq, model.APIFormatOpenAIChatCompletion) {
		t.Fatal("passthrough was not marked for chat")
	}
	if len(internalReq.UpstreamRequestBody) == 0 {
		t.Fatal("UpstreamRequestBody should be set")
	}
}

func TestGeminiOutboundStreamKeepsRawChunkForPassthrough(t *testing.T) {
	out := &gemini.MessagesOutbound{}
	raw := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP","index":0}]}`)

	resp, err := out.TransformStream(context.Background(), raw)
	if err != nil {
		t.Fatalf("TransformStream error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if string(resp.RawChunk) != string(raw) {
		t.Fatalf("RawChunk changed\ngot:  %s\nwant: %s", resp.RawChunk, raw)
	}
}

func TestAnthropicPassthroughUpstreamRequestBodyIncludesThinkingPatch(t *testing.T) {
	out := &authropic.MessageOutbound{}
	raw := []byte(`{"model":"claude-sonnet-4-5","max_tokens":1024,"thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","content":"hello"}]}`)
	internalReq := &model.InternalLLMRequest{
		Model:           "claude-sonnet-4-5",
		RawRequest:      raw,
		RawAPIFormat:    model.APIFormatAnthropicMessage,
		ReasoningEffort: "medium",
		Messages: []model.Message{
			{Role: "assistant", Content: model.MessageContent{Content: strPtr("hello")}},
		},
	}

	_, err := out.TransformRequest(context.Background(), internalReq, "https://api.anthropic.com", "key")
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}
	if !model.IsPassthrough(internalReq, model.APIFormatAnthropicMessage) {
		t.Fatal("passthrough was not marked for anthropic")
	}

	var got anthropicModel.MessageRequest
	if err := json.Unmarshal(internalReq.UpstreamRequestBody, &got); err != nil {
		t.Fatalf("UpstreamRequestBody is not valid Anthropic request: %v", err)
	}
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(internalReq.UpstreamRequestBody, &rawMap); err != nil {
		t.Fatalf("UpstreamRequestBody is not valid JSON: %v", err)
	}
	if _, ok := rawMap["reasoning"]; ok {
		t.Fatal("Anthropic passthrough body must not contain OpenAI reasoning field")
	}
	if got.Thinking == nil || got.Thinking.Type != anthropicModel.ThinkingTypeEnabled {
		t.Fatalf("thinking config not preserved/patched: %+v", got.Thinking)
	}
	if len(got.Messages) != 1 || len(got.Messages[0].Content.MultipleContent) == 0 {
		t.Fatalf("expected patched multiple content, got %+v", got.Messages)
	}
	first := got.Messages[0].Content.MultipleContent[0]
	if first.Type != "thinking" || first.Thinking == nil || *first.Thinking != "" {
		t.Fatalf("thinking block not patched: %+v", first)
	}
}

func strPtr(s string) *string { return &s }
