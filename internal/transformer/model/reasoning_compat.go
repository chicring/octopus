package model

import (
	"encoding/json"
	"strings"
)

// NormalizeReasoningContentReplay preserves provider-native thinking content
// when replaying assistant tool calls to reasoning providers.
func NormalizeReasoningContentReplay(request *InternalLLMRequest) {
	if request == nil || !supportsReasoningContentReplay(request.Model) {
		return
	}

	for idx := range request.Messages {
		normalizeMessageReasoningContentReplay(&request.Messages[idx])
	}
}

func PatchRawReasoningContentReplay(req map[string]json.RawMessage, model string) bool {
	if !supportsReasoningContentReplay(model) {
		return false
	}

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
		if patchRawMessageReasoningContentReplay(messages[idx]) {
			patched = true
		}
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

func supportsReasoningContentReplay(model string) bool {
	model = strings.ToLower(model)
	return strings.Contains(model, "deepseek")
}

func normalizeMessageReasoningContentReplay(msg *Message) bool {
	if msg == nil || msg.ReasoningContent != nil || msg.Reasoning != nil || len(msg.ToolCalls) == 0 {
		return false
	}

	if msg.Content.Content != nil {
		thinking, ok := extractWholeThinkingContent(*msg.Content.Content)
		if !ok {
			return false
		}
		msg.ReasoningContent = &thinking
		msg.Content.Content = nil
		return true
	}

	thinking, parts, ok := extractThinkingPartContent(msg.Content.MultipleContent)
	if !ok {
		return false
	}
	msg.ReasoningContent = &thinking
	msg.Content.MultipleContent = parts
	return true
}

func patchRawMessageReasoningContentReplay(msg map[string]json.RawMessage) bool {
	if _, ok := msg["reasoning_content"]; ok {
		return false
	}
	if _, ok := msg["reasoning"]; ok {
		return false
	}
	if _, ok := msg["tool_calls"]; !ok {
		return false
	}

	var content string
	if err := json.Unmarshal(msg["content"], &content); err == nil {
		thinking, ok := extractWholeThinkingContent(content)
		if !ok {
			return false
		}

		reasoningBytes, err := json.Marshal(thinking)
		if err != nil {
			return false
		}
		msg["reasoning_content"] = reasoningBytes
		delete(msg, "content")
		return true
	}

	var parts []MessageContentPart
	if err := json.Unmarshal(msg["content"], &parts); err != nil {
		return false
	}
	thinking, patchedParts, ok := extractThinkingPartContent(parts)
	if !ok {
		return false
	}

	reasoningBytes, err := json.Marshal(thinking)
	if err != nil {
		return false
	}
	msg["reasoning_content"] = reasoningBytes
	if len(patchedParts) == 0 {
		delete(msg, "content")
	} else if contentBytes, err := json.Marshal(patchedParts); err == nil {
		msg["content"] = contentBytes
	} else {
		return false
	}
	return true
}

func extractThinkingPartContent(parts []MessageContentPart) (string, []MessageContentPart, bool) {
	for idx, part := range parts {
		if part.Type != "text" || part.Text == nil {
			continue
		}
		thinking, ok := extractWholeThinkingContent(*part.Text)
		if !ok {
			continue
		}

		patchedParts := make([]MessageContentPart, 0, len(parts)-1)
		patchedParts = append(patchedParts, parts[:idx]...)
		patchedParts = append(patchedParts, parts[idx+1:]...)
		return thinking, patchedParts, true
	}
	return "", nil, false
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
