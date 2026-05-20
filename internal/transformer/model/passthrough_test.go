package model

import (
	"encoding/json"
	"strings"
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

func TestPatchRawRequestPatchesReasoningWithoutReserializingInputPrefix(t *testing.T) {
	raw := []byte(`{"model":"gpt-4o","input":[{"id":"msg_1","role":"user","content":[{"id":"part_1","type":"input_text","text":"describe"},{"id":"part_2","type":"input_image","image_url":"data:image/png;base64,abc","detail":"low"}]}],"previous_response_id":"resp_123","reasoning":{ "summary":"auto", "effort" : "medium" },"stream":true}`)
	wantPrefix := `{"model":"gpt-4o","input":[{"id":"msg_1","role":"user","content":[{"id":"part_1","type":"input_text","text":"describe"},{"id":"part_2","type":"input_image","image_url":"data:image/png;base64,abc","detail":"low"}]}],"previous_response_id":"resp_123","reasoning":{ "summary":"auto", "effort" : `

	request := &InternalLLMRequest{
		Model:           "gpt-4o",
		ReasoningEffort: "high",
	}

	result := PatchRawRequest(raw, request)

	if !strings.HasPrefix(string(result), wantPrefix) {
		t.Fatalf("PatchRawRequest() changed cache-sensitive prefix.\ngot:  %s\nwant prefix: %s", result, wantPrefix)
	}
	if !strings.Contains(string(result), `"summary":"auto"`) {
		t.Fatalf("PatchRawRequest() dropped existing reasoning fields: %s", result)
	}
	if !strings.Contains(string(result), `"effort" : "high"`) {
		t.Fatalf("PatchRawRequest() did not patch reasoning effort in place: %s", result)
	}
}

func TestPatchRawRequestAddsReasoningAtEndWhenMissing(t *testing.T) {
	raw := []byte(`{"model":"gpt-4o","input":[{"role":"user","content":"hello"}],"stream":true}`)
	wantPrefix := string(raw[:len(raw)-1])

	request := &InternalLLMRequest{
		Model:           "gpt-4o",
		ReasoningEffort: "medium",
	}

	result := PatchRawRequest(raw, request)

	if !strings.HasPrefix(string(result), wantPrefix) {
		t.Fatalf("PatchRawRequest() should only append reasoning when missing.\ngot:  %s\nwant prefix: %s", result, wantPrefix)
	}
	if !strings.HasSuffix(string(result), `,"reasoning":{"effort":"medium"}}`) {
		t.Fatalf("PatchRawRequest() appended unexpected reasoning: %s", result)
	}
}

func TestPatchRawRequestPreservesReasoningMaxTokensWhenOverrideHasNoBudget(t *testing.T) {
	raw := []byte(`{"model":"o3","input":"hello","reasoning":{"effort":"low","max_tokens":1234}}`)
	request := &InternalLLMRequest{
		Model:           "o3",
		ReasoningEffort: "high",
	}

	result := PatchRawRequest(raw, request)

	if !strings.Contains(string(result), `"max_tokens":1234`) {
		t.Fatalf("PatchRawRequest() dropped max_tokens: %s", result)
	}
	if !strings.Contains(string(result), `"effort":"high"`) {
		t.Fatalf("PatchRawRequest() did not patch effort: %s", result)
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

func TestPatchRawRequestMovesThinkingContentPartToReasoningContent(t *testing.T) {
	original := []byte(`{"model":"deepseek-v4-flash","stream":true,"messages":[{"role":"assistant","content":[{"type":"text","text":"<thinking>\nNow I have a good understanding of the Android server card and resource card implementations.\n</thinking>"},{"type":"text","text":"Now let me find the iOS counterparts for reference."}],"tool_calls":[{"id":"call_00_o8BU6zw6r3ZRiAdpg20m3142","type":"function","function":{"name":"TodoWrite","arguments":"{\"todos\":\"1. [completed] Find Android server card implementation(s)\"}"},"index":0}]},{"role":"tool","tool_call_id":"call_00_o8BU6zw6r3ZRiAdpg20m3142","content":"TODO List Updated"}]}`)
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
	wantReasoning := "Now I have a good understanding of the Android server card and resource card implementations."
	if assistant.ReasoningContent == nil || *assistant.ReasoningContent != wantReasoning {
		t.Fatalf("reasoning_content = %v, want %q; body=%s", assistant.ReasoningContent, wantReasoning, result)
	}
	if assistant.Content.Content != nil {
		t.Fatalf("content should remain an array, got string: %q", *assistant.Content.Content)
	}
	if len(assistant.Content.MultipleContent) != 1 {
		t.Fatalf("content parts len = %d, want 1; body=%s", len(assistant.Content.MultipleContent), result)
	}
	part := assistant.Content.MultipleContent[0]
	if part.Type != "text" || part.Text == nil || *part.Text != "Now let me find the iOS counterparts for reference." {
		t.Fatalf("remaining content part = %+v, want visible text only", part)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "call_00_o8BU6zw6r3ZRiAdpg20m3142" {
		t.Fatalf("tool_calls = %+v, want call_00_o8BU6zw6r3ZRiAdpg20m3142", assistant.ToolCalls)
	}
}

func TestNormalizeReasoningContentReplay(t *testing.T) {
	content := "<thinking>\nI should call the tool.\n</thinking>"
	request := &InternalLLMRequest{
		Model: "deepseek-v4-flash",
		Messages: []Message{
			{
				Role:    "assistant",
				Content: MessageContent{Content: &content},
				ToolCalls: []ToolCall{{
					ID:   "call_read",
					Type: "function",
					Function: FunctionCall{
						Name:      "Read",
						Arguments: `{"file_path":"/tmp/example.txt"}`,
					},
				}},
			},
		},
	}

	NormalizeReasoningContentReplay(request)

	msg := request.Messages[0]
	if msg.ReasoningContent == nil || *msg.ReasoningContent != "I should call the tool." {
		t.Fatalf("reasoning_content = %v, want extracted thinking", msg.ReasoningContent)
	}
	if msg.Content.Content != nil {
		t.Fatalf("thinking wrapper must not remain as content: %q", *msg.Content.Content)
	}
}

func TestNormalizeReasoningContentReplayMovesThinkingContentPart(t *testing.T) {
	thinkingPart := "<thinking>\nI should call the tool.\n</thinking>"
	visiblePart := "Now I will call the tool."
	request := &InternalLLMRequest{
		Model: "deepseek-v4-flash",
		Messages: []Message{
			{
				Role: "assistant",
				Content: MessageContent{MultipleContent: []MessageContentPart{
					{Type: "text", Text: &thinkingPart},
					{Type: "text", Text: &visiblePart},
				}},
				ToolCalls: []ToolCall{{
					ID:   "call_read",
					Type: "function",
					Function: FunctionCall{
						Name:      "Read",
						Arguments: `{"file_path":"/tmp/example.txt"}`,
					},
				}},
			},
		},
	}

	NormalizeReasoningContentReplay(request)

	msg := request.Messages[0]
	if msg.ReasoningContent == nil || *msg.ReasoningContent != "I should call the tool." {
		t.Fatalf("reasoning_content = %v, want extracted thinking", msg.ReasoningContent)
	}
	if len(msg.Content.MultipleContent) != 1 {
		t.Fatalf("content parts len = %d, want 1", len(msg.Content.MultipleContent))
	}
	if msg.Content.MultipleContent[0].Text == nil || *msg.Content.MultipleContent[0].Text != visiblePart {
		t.Fatalf("remaining content = %+v, want visible text", msg.Content.MultipleContent[0])
	}
}

func TestNormalizeReasoningContentReplayDoesNotAffectNonCompatibleModels(t *testing.T) {
	content := "<thinking>\nI should call the tool.\n</thinking>"
	request := &InternalLLMRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{
				Role:    "assistant",
				Content: MessageContent{Content: &content},
				ToolCalls: []ToolCall{{
					ID:   "call_read",
					Type: "function",
					Function: FunctionCall{
						Name:      "Read",
						Arguments: `{}`,
					},
				}},
			},
		},
	}

	NormalizeReasoningContentReplay(request)

	msg := request.Messages[0]
	if msg.ReasoningContent != nil {
		t.Fatalf("non-compatible model must not get reasoning_content: %v", *msg.ReasoningContent)
	}
	if msg.Content.Content == nil || *msg.Content.Content != content {
		t.Fatalf("non-compatible model content changed: %v", msg.Content.Content)
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
