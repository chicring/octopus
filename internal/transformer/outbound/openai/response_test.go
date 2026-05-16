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

func TestChatOutboundAnthropicToOpenAIChatPreservesReasoningContentWithToolCalls(t *testing.T) {
	out := &ChatOutbound{}
	reasoning := "Need the current date before calling weather."
	internalReq := &model.InternalLLMRequest{
		Model:        "deepseek-reasoner",
		RawAPIFormat: model.APIFormatAnthropicMessage,
		Messages: []model.Message{
			{
				Role:             "assistant",
				ReasoningContent: &reasoning,
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
	}

	_, err := out.TransformRequest(context.Background(), internalReq, "https://api.deepseek.com/v1", "key")
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}

	var got struct {
		Messages []model.Message `json:"messages"`
	}
	if err := json.Unmarshal(internalReq.UpstreamRequestBody, &got); err != nil {
		t.Fatalf("UpstreamRequestBody is not valid JSON: %v", err)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(got.Messages))
	}
	msg := got.Messages[0]
	if msg.ReasoningContent == nil || *msg.ReasoningContent != reasoning {
		t.Fatalf("reasoning_content = %v, want %q", msg.ReasoningContent, reasoning)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "call_date" {
		t.Fatalf("tool_calls = %+v, want call_date", msg.ToolCalls)
	}
}

func TestChatOutboundAnthropicToOpenAIChatPreservesReasoningAliasWithToolCalls(t *testing.T) {
	out := &ChatOutbound{}
	reasoning := "Need the current date before calling weather."
	internalReq := &model.InternalLLMRequest{
		Model:        "openrouter-model",
		RawAPIFormat: model.APIFormatAnthropicMessage,
		Messages: []model.Message{
			{
				Role:      "assistant",
				Reasoning: &reasoning,
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
	}

	_, err := out.TransformRequest(context.Background(), internalReq, "https://openrouter.ai/api/v1", "key")
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(internalReq.UpstreamRequestBody, &body); err != nil {
		t.Fatalf("UpstreamRequestBody is not valid JSON: %v", err)
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(body["messages"], &messages); err != nil {
		t.Fatalf("messages is not valid JSON: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if _, ok := messages[0]["reasoning"]; !ok {
		t.Fatalf("reasoning alias missing from message: %s", messages[0])
	}
	if _, ok := messages[0]["tool_calls"]; !ok {
		t.Fatalf("tool_calls missing from message: %s", messages[0])
	}
}

func TestDeepSeekReasoningContentRoundTripForToolCalls(t *testing.T) {
	reasoning := "Need the current date before calling weather."
	text := "Let me check the date first."
	finishReason := "tool_calls"
	upstream := &model.InternalLLMResponse{
		ID:    "chatcmpl_deepseek",
		Model: "deepseek-reasoner",
		Choices: []model.Choice{
			{
				Index: 0,
				Message: &model.Message{
					Role:             "assistant",
					ReasoningContent: &reasoning,
					Content:          model.MessageContent{Content: &text},
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
				FinishReason: &finishReason,
			},
		},
	}

	in := &anthropicModel.MessagesInbound{}
	claudeBody, err := in.TransformResponse(context.Background(), upstream)
	if err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}

	var claudeResp struct {
		Content []struct {
			Type      string          `json:"type"`
			Thinking  *string         `json:"thinking,omitempty"`
			Signature *string         `json:"signature,omitempty"`
			Text      *string         `json:"text,omitempty"`
			ID        string          `json:"id,omitempty"`
			Name      *string         `json:"name,omitempty"`
			Input     json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
	}
	if err := json.Unmarshal(claudeBody, &claudeResp); err != nil {
		t.Fatalf("Claude response is not valid JSON: %v", err)
	}
	if len(claudeResp.Content) != 3 {
		t.Fatalf("content blocks len = %d, want 3; body=%s", len(claudeResp.Content), claudeBody)
	}
	if claudeResp.Content[0].Type != "thinking" || claudeResp.Content[0].Thinking == nil || *claudeResp.Content[0].Thinking != reasoning {
		t.Fatalf("thinking block = %+v, want DeepSeek reasoning_content", claudeResp.Content[0])
	}
	if claudeResp.Content[0].Signature != nil {
		t.Fatalf("DeepSeek reasoning_content must not synthesize Anthropic signature: %+v", claudeResp.Content[0])
	}
	if claudeResp.Content[1].Type != "text" || claudeResp.Content[1].Text == nil || *claudeResp.Content[1].Text != text {
		t.Fatalf("text block = %+v, want %q", claudeResp.Content[1], text)
	}
	if claudeResp.Content[2].Type != "tool_use" || claudeResp.Content[2].ID != "call_date" {
		t.Fatalf("tool_use block = %+v, want call_date", claudeResp.Content[2])
	}

	followUpReq := struct {
		Model     string `json:"model"`
		MaxTokens int64  `json:"max_tokens"`
		Messages  []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}{
		Model:     "deepseek-reasoner",
		MaxTokens: 1024,
		Messages: []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}{
			{
				Role:    "assistant",
				Content: mustMarshalRaw(t, claudeResp.Content),
			},
		},
	}
	followUpBody, err := json.Marshal(followUpReq)
	if err != nil {
		t.Fatalf("marshal follow-up request: %v", err)
	}

	parsedFollowUp, err := in.TransformRequest(context.Background(), followUpBody)
	if err != nil {
		t.Fatalf("TransformRequest follow-up error: %v", err)
	}

	out := &ChatOutbound{}
	_, err = out.TransformRequest(context.Background(), parsedFollowUp, "https://api.deepseek.com/v1", "key")
	if err != nil {
		t.Fatalf("ChatOutbound TransformRequest error: %v", err)
	}

	var replayed struct {
		Messages []model.Message `json:"messages"`
	}
	if err := json.Unmarshal(parsedFollowUp.UpstreamRequestBody, &replayed); err != nil {
		t.Fatalf("OpenAI Chat replay body is not valid JSON: %v", err)
	}
	if len(replayed.Messages) != 1 {
		t.Fatalf("replayed messages len = %d, want 1", len(replayed.Messages))
	}
	replayedMsg := replayed.Messages[0]
	if replayedMsg.ReasoningContent == nil || *replayedMsg.ReasoningContent != reasoning {
		t.Fatalf("replayed reasoning_content = %v, want %q", replayedMsg.ReasoningContent, reasoning)
	}
	if replayedMsg.ReasoningSignature != nil {
		t.Fatalf("replayed message must not retain Anthropic signature: %v", *replayedMsg.ReasoningSignature)
	}
	if len(replayedMsg.ToolCalls) != 1 || replayedMsg.ToolCalls[0].ID != "call_date" {
		t.Fatalf("replayed tool_calls = %+v, want call_date", replayedMsg.ToolCalls)
	}
}

func TestAnthropicStringThinkingWrapperPreservesReasoningContent(t *testing.T) {
	reasoning := "Let me start by reading the macOS UI design principles document as requested."
	body := []byte(`{
		"model":"deepseek-v4-flash",
		"max_tokens":1024,
		"stream":true,
		"messages":[
			{
				"role":"assistant",
				"content":"<thinking>\nLet me start by reading the macOS UI design principles document as requested.\n</thinking>",
				"tool_calls":[
					{
						"id":"call_00_S7C4wPEA3gZgX0SylKvs7524",
						"type":"function",
						"function":{
							"name":"Read",
							"arguments":"{\"file_path\":\"/Users/chenjh/Dev/stable/Themby-kmp/doc/design-docs/macos-ui-design-principles.md\"}"
						},
						"index":0
					}
				]
			},
			{
				"role":"tool",
				"tool_call_id":"call_00_S7C4wPEA3gZgX0SylKvs7524",
				"content":"# macOS UI Design Principles"
			}
		]
	}`)

	var internalReq model.InternalLLMRequest
	if err := json.Unmarshal(body, &internalReq); err != nil {
		t.Fatalf("unmarshal raw chat request: %v", err)
	}
	internalReq.RawAPIFormat = model.APIFormatOpenAIChatCompletion
	internalReq.RawRequest = body
	out := &ChatOutbound{}
	_, err := out.TransformRequest(context.Background(), &internalReq, "https://api.deepseek.com/v1", "key")
	if err != nil {
		t.Fatalf("ChatOutbound TransformRequest error: %v", err)
	}

	var upstream struct {
		Messages []model.Message `json:"messages"`
	}
	if err := json.Unmarshal(internalReq.UpstreamRequestBody, &upstream); err != nil {
		t.Fatalf("UpstreamRequestBody is not valid JSON: %v", err)
	}
	if len(upstream.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(upstream.Messages))
	}

	assistant := upstream.Messages[0]
	if assistant.ReasoningContent == nil || *assistant.ReasoningContent != reasoning {
		t.Fatalf("reasoning_content = %v, want %q; body=%s", assistant.ReasoningContent, reasoning, internalReq.UpstreamRequestBody)
	}
	if assistant.Content.Content != nil {
		t.Fatalf("thinking wrapper must not remain as content: %q", *assistant.Content.Content)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "call_00_S7C4wPEA3gZgX0SylKvs7524" {
		t.Fatalf("tool_calls = %+v, want call_00_S7C4wPEA3gZgX0SylKvs7524", assistant.ToolCalls)
	}
}

func mustMarshalRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal raw: %v", err)
	}
	return b
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
