package model

import (
	"encoding/json"
	"reflect"
	"strings"
)

// PatchRawRequest 对原始请求 body 做最小修改：
// 仅更新 model 字段（模型名映射）和 reasoning 字段（reasoning effort override），
// 其余字段原样保留，确保上游 prompt cache 前缀匹配不被破坏。
// 当入站和出站格式相同时调用，避免 round-trip 转换改变序列化结果。
func PatchRawRequest(raw []byte, request *InternalLLMRequest) []byte {
	return patchRawRequest(raw, request, true)
}

// PatchRawRequestModelOnly 仅更新原始请求中的 model 字段。
// 用于不支持 OpenAI reasoning 字段的协议（如 Anthropic Messages）。
func PatchRawRequestModelOnly(raw []byte, request *InternalLLMRequest) []byte {
	return patchRawRequest(raw, request, false)
}

func patchRawRequest(raw []byte, request *InternalLLMRequest, patchReasoning bool) []byte {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(raw, &req); err != nil {
		return raw
	}

	patched := false

	// 覆盖 model 名（relay 层可能将客户端请求的 model 映射为实际模型名）
	if request.Model != "" {
		var originalModel string
		if modelRaw, ok := req["model"]; ok {
			json.Unmarshal(modelRaw, &originalModel)
		}
		if originalModel != request.Model {
			if modelBytes, err := json.Marshal(request.Model); err == nil {
				req["model"] = modelBytes
				patched = true
			}
		}
	}

	// 覆盖 reasoning effort（分组可能 override 了 reasoning_effort）
	if patchReasoning && request.ReasoningEffort != "" {
		reasoning := map[string]any{"effort": request.ReasoningEffort}
		if request.ReasoningBudget != nil {
			reasoning["max_tokens"] = *request.ReasoningBudget
		}
		reasoningBytes, err := json.Marshal(reasoning)
		if err == nil && !jsonEqual(req["reasoning"], reasoningBytes) {
			req["reasoning"] = reasoningBytes
			patched = true
		}
	}

	if patchReasoning && isDeepSeekModel(request.Model) && patchThinkingWrappers(req) {
		patched = true
	}

	// 无需修改时直接透传原始 body，避免 json.Marshal 改变序列化格式
	if !patched {
		return raw
	}

	body, err := json.Marshal(req)
	if err != nil {
		return raw
	}
	return body
}

func isDeepSeekModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "deepseek")
}

func NormalizeDeepSeekThinkingWrappers(request *InternalLLMRequest) {
	if request == nil || !isDeepSeekModel(request.Model) {
		return
	}

	for idx := range request.Messages {
		msg := &request.Messages[idx]
		if msg.ReasoningContent != nil || msg.Reasoning != nil || len(msg.ToolCalls) == 0 || msg.Content.Content == nil {
			continue
		}
		thinking, ok := extractWholeThinkingContent(*msg.Content.Content)
		if !ok {
			continue
		}
		msg.ReasoningContent = &thinking
		msg.Content.Content = nil
	}
}

func patchThinkingWrappers(req map[string]json.RawMessage) bool {
	messagesRaw, ok := req["messages"]
	if !ok {
		return false
	}

	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(messagesRaw, &messages); err != nil {
		return false
	}

	patched := false
	for idx := range messages {
		if _, ok := messages[idx]["reasoning_content"]; ok {
			continue
		}
		if _, ok := messages[idx]["reasoning"]; ok {
			continue
		}
		if _, ok := messages[idx]["tool_calls"]; !ok {
			continue
		}

		var content string
		if err := json.Unmarshal(messages[idx]["content"], &content); err != nil {
			continue
		}
		thinking, ok := extractWholeThinkingContent(content)
		if !ok {
			continue
		}
		reasoningBytes, err := json.Marshal(thinking)
		if err != nil {
			continue
		}
		messages[idx]["reasoning_content"] = reasoningBytes
		delete(messages[idx], "content")
		patched = true
	}

	if !patched {
		return false
	}

	body, err := json.Marshal(messages)
	if err != nil {
		return false
	}
	req["messages"] = body
	return true
}

func extractWholeThinkingContent(content string) (string, bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "<thinking>") || !strings.HasSuffix(trimmed, "</thinking>") {
		return "", false
	}

	thinking := strings.TrimPrefix(trimmed, "<thinking>")
	thinking = strings.TrimSuffix(thinking, "</thinking>")
	return strings.TrimSpace(thinking), true
}

func jsonEqual(a, b []byte) bool {
	if len(a) == 0 {
		return len(b) == 0
	}
	var av any
	var bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// ShouldPassthrough 判断是否应该直接透传原始请求 body，
// 而不是做 round-trip 转换（inbound → internal → outbound）。
// 当入站格式和出站格式相同时返回 true。
func ShouldPassthrough(request *InternalLLMRequest, outboundFormat APIFormat) bool {
	return request.RawAPIFormat == outboundFormat && len(request.RawRequest) > 0
}

// MarkPassthrough 记录本次出站请求实际走了原始 body 透传。
func MarkPassthrough(request *InternalLLMRequest, outboundFormat APIFormat) {
	if request != nil {
		request.PassthroughAPIFormat = outboundFormat
	}
}

// ClearPassthrough 清除本次尝试的透传标记。
func ClearPassthrough(request *InternalLLMRequest) {
	if request != nil {
		request.PassthroughAPIFormat = ""
	}
}

// IsPassthrough 判断本次尝试是否实际透传到了指定格式。
func IsPassthrough(request *InternalLLMRequest, outboundFormat APIFormat) bool {
	return request != nil && request.PassthroughAPIFormat == outboundFormat
}
