package model

import "encoding/json"

// PatchRawRequest 对原始请求 body 做最小修改：
// 仅更新 model 字段（模型名映射）和 reasoning 字段（reasoning effort override），
// 其余字段原样保留，确保上游 prompt cache 前缀匹配不被破坏。
// 当入站和出站格式相同时调用，避免 round-trip 转换改变序列化结果。
func PatchRawRequest(raw []byte, request *InternalLLMRequest) []byte {
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
	if request.ReasoningEffort != "" {
		reasoning := map[string]any{"effort": request.ReasoningEffort}
		if request.ReasoningBudget != nil {
			reasoning["max_tokens"] = *request.ReasoningBudget
		}
		if reasoningBytes, err := json.Marshal(reasoning); err == nil {
			req["reasoning"] = reasoningBytes
			patched = true
		}
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
