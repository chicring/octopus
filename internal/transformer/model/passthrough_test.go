package model

import (
	"encoding/json"
	"testing"
)

func TestShouldPassthrough(t *testing.T) {
	tests := []struct {
		name           string
		request        *InternalLLMRequest
		outboundFormat APIFormat
		want           bool
	}{
		{
			name: "matching format with raw request",
			request: &InternalLLMRequest{
				RawAPIFormat: APIFormatOpenAIChatCompletion,
				RawRequest:   []byte(`{"model":"gpt-4"}`),
			},
			outboundFormat: APIFormatOpenAIChatCompletion,
			want:           true,
		},
		{
			name: "matching format without raw request",
			request: &InternalLLMRequest{
				RawAPIFormat: APIFormatOpenAIChatCompletion,
				RawRequest:   nil,
			},
			outboundFormat: APIFormatOpenAIChatCompletion,
			want:           false,
		},
		{
			name: "mismatched format",
			request: &InternalLLMRequest{
				RawAPIFormat: APIFormatOpenAIChatCompletion,
				RawRequest:   []byte(`{"model":"gpt-4"}`),
			},
			outboundFormat: APIFormatOpenAIResponse,
			want:           false,
		},
		{
			name: "empty raw request",
			request: &InternalLLMRequest{
				RawAPIFormat: APIFormatOpenAIResponse,
				RawRequest:   []byte{},
			},
			outboundFormat: APIFormatOpenAIResponse,
			want:           false,
		},
		{
			name: "anthropic matching",
			request: &InternalLLMRequest{
				RawAPIFormat: APIFormatAnthropicMessage,
				RawRequest:   []byte(`{"model":"claude-3"}`),
			},
			outboundFormat: APIFormatAnthropicMessage,
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldPassthrough(tt.request, tt.outboundFormat)
			if got != tt.want {
				t.Errorf("ShouldPassthrough() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPatchRawRequest(t *testing.T) {
	tests := []struct {
		name    string
		raw     []byte
		request *InternalLLMRequest
		want    map[string]any
	}{
		{
			name: "patch model name",
			raw:  []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`),
			request: &InternalLLMRequest{
				Model: "gpt-4o-mini",
			},
			want: map[string]any{
				"model":    "gpt-4o-mini",
				"messages": []any{map[string]any{"role": "user", "content": "hello"}},
			},
		},
		{
			name: "no patch needed when model matches",
			raw:  []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`),
			request: &InternalLLMRequest{
				Model: "gpt-4o",
			},
			want: map[string]any{
				"model":    "gpt-4o",
				"messages": []any{map[string]any{"role": "user", "content": "hello"}},
			},
		},
		{
			name: "patch reasoning effort",
			raw:  []byte(`{"model":"o3","messages":[]}`),
			request: &InternalLLMRequest{
				Model:           "o3",
				ReasoningEffort: "high",
			},
			want: map[string]any{
				"model":     "o3",
				"messages":  []any{},
				"reasoning": map[string]any{"effort": "high"},
			},
		},
		{
			name: "patch both model and reasoning",
			raw:  []byte(`{"model":"o3-mini","messages":[]}`),
			request: &InternalLLMRequest{
				Model:           "o3",
				ReasoningEffort: "medium",
			},
			want: map[string]any{
				"model":     "o3",
				"messages":  []any{},
				"reasoning": map[string]any{"effort": "medium"},
			},
		},
		{
			name: "patch reasoning with budget",
			raw:  []byte(`{"model":"o3","messages":[]}`),
			request: &InternalLLMRequest{
				Model:           "o3",
				ReasoningEffort: "high",
				ReasoningBudget: int64Ptr(32768),
			},
			want: map[string]any{
				"model":     "o3",
				"messages":  []any{},
				"reasoning": map[string]any{"effort": "high", "max_tokens": float64(32768)},
			},
		},
		{
			name: "invalid JSON returns original",
			raw:  []byte(`{invalid json}`),
			request: &InternalLLMRequest{
				Model: "gpt-4o",
			},
			want: nil, // special case: returns original bytes
		},
		{
			name: "preserve all other fields",
			raw:  []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"stream":true,"temperature":0.7,"previous_response_id":"resp_123"}`),
			request: &InternalLLMRequest{
				Model: "gpt-4o",
			},
			want: map[string]any{
				"model":                "gpt-4o",
				"messages":             []any{map[string]any{"role": "user", "content": "hello"}},
				"stream":               true,
				"temperature":          0.7,
				"previous_response_id": "resp_123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PatchRawRequest(tt.raw, tt.request)

			if tt.want == nil {
				// Special case: should return original bytes
				if string(result) != string(tt.raw) {
					t.Errorf("PatchRawRequest() = %s, want original %s", result, tt.raw)
				}
				return
			}

			var got map[string]any
			if err := json.Unmarshal(result, &got); err != nil {
				t.Fatalf("PatchRawRequest() result is not valid JSON: %v", err)
			}

			// Compare specific fields
			for key, wantVal := range tt.want {
				gotVal, ok := got[key]
				if !ok {
					t.Errorf("PatchRawRequest() missing key %q", key)
					continue
				}
				// Marshal both to JSON for comparison (handles nested maps/slices)
				wantJSON, _ := json.Marshal(wantVal)
				gotJSON, _ := json.Marshal(gotVal)
				if string(wantJSON) != string(gotJSON) {
					t.Errorf("PatchRawRequest()[%q] = %s, want %s", key, gotJSON, wantJSON)
				}
			}
		})
	}
}

func TestPatchRawRequestPreservesSerialization(t *testing.T) {
	// 测试当不需要 patch 时，返回的是原始 bytes（不经过 json.Marshal），
	// 确保 prompt cache 前缀匹配不会被破坏。
	original := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"stream":true}`)

	request := &InternalLLMRequest{
		Model: "gpt-4o", // same as original, no patch needed
	}

	result := PatchRawRequest(original, request)

	if string(result) != string(original) {
		t.Errorf("PatchRawRequest() should return original bytes when no patch needed.\ngot:  %s\nwant: %s", result, original)
	}
}

func TestPatchRawRequestPreservesImageResponseWhenReasoningMatches(t *testing.T) {
	original := []byte(`{"model":"gpt-4o","input":[{"role":"user","content":[{"type":"input_text","text":"describe"},{"type":"input_image","image_url":"data:image/png;base64,abc","detail":"low"}]}],"reasoning":{ "effort" : "medium" },"stream":true}`)

	request := &InternalLLMRequest{
		Model:           "gpt-4o",
		ReasoningEffort: "medium",
	}

	result := PatchRawRequest(original, request)

	if string(result) != string(original) {
		t.Errorf("PatchRawRequest() should preserve raw image request when effective reasoning is unchanged.\ngot:  %s\nwant: %s", result, original)
	}
}

func TestPatchRawRequestMovesThinkingWrapperToReasoningContent(t *testing.T) {
	original := []byte(`{"model":"deepseek-v4-flash","stream":true,"messages":[{"role":"assistant","content":"<thinking>\nLet me start by reading the macOS UI design principles document as requested.\n</thinking>","tool_calls":[{"id":"call_00_S7C4wPEA3gZgX0SylKvs7524","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"/Users/chenjh/Dev/stable/Themby-kmp/doc/design-docs/macos-ui-design-principles.md\"}"},"index":0}]},{"role":"tool","tool_call_id":"call_00_S7C4wPEA3gZgX0SylKvs7524","content":"# macOS UI Design Principles"}]}`)
	request := &InternalLLMRequest{
		Model: "deepseek-v4-flash",
	}

	result := PatchRawRequest(original, request)

	var got struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("PatchRawRequest() result is not valid JSON: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(got.Messages))
	}

	assistant := got.Messages[0]
	wantReasoning := "Let me start by reading the macOS UI design principles document as requested."
	if assistant.ReasoningContent == nil || *assistant.ReasoningContent != wantReasoning {
		t.Fatalf("reasoning_content = %v, want %q; body=%s", assistant.ReasoningContent, wantReasoning, result)
	}
	if assistant.Content.Content != nil {
		t.Fatalf("thinking wrapper must not remain as content: %q", *assistant.Content.Content)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "call_00_S7C4wPEA3gZgX0SylKvs7524" {
		t.Fatalf("tool_calls = %+v, want call_00_S7C4wPEA3gZgX0SylKvs7524", assistant.ToolCalls)
	}
}

func TestPatchRawRequestPreservesFieldOrder(t *testing.T) {
	// 测试当需要 patch 时，其他字段的顺序和值保持不变
	original := []byte(`{"stream":true,"model":"o3-mini","temperature":0.5,"messages":[{"role":"user","content":"hi"}]}`)

	request := &InternalLLMRequest{
		Model:           "o3",
		ReasoningEffort: "high",
	}

	result := PatchRawRequest(original, request)

	var got map[string]any
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	if got["model"] != "o3" {
		t.Errorf("model = %v, want o3", got["model"])
	}
	if got["stream"] != true {
		t.Errorf("stream = %v, want true", got["stream"])
	}
	if got["temperature"] != 0.5 {
		t.Errorf("temperature = %v, want 0.5", got["temperature"])
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}
