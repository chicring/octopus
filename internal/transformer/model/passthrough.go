package model

import (
	"encoding/json"
	"reflect"
	"sort"
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
	if request == nil {
		return raw
	}

	info, ok := parseJSONObject(raw)
	if !ok {
		return raw
	}

	var patches []rawPatch
	var insertFields [][]byte

	// 覆盖 model 名（relay 层可能将客户端请求的 model 映射为实际模型名）
	if request.Model != "" {
		if modelBytes, err := json.Marshal(request.Model); err == nil {
			if field, ok := findJSONField(info, "model"); ok {
				if !jsonEqual(raw[field.valueStart:field.valueEnd], modelBytes) {
					patches = append(patches, rawPatch{
						start: field.valueStart,
						end:   field.valueEnd,
						value: modelBytes,
					})
				}
			} else {
				insertFields = append(insertFields, makeJSONField("model", modelBytes))
			}
		}
	}

	// 覆盖 reasoning effort（分组可能 override 了 reasoning_effort）
	if patchReasoning && request.ReasoningEffort != "" {
		if field, ok := findJSONField(info, "reasoning"); ok {
			if reasoningBytes, changed := patchReasoningObject(raw[field.valueStart:field.valueEnd], request); changed {
				patches = append(patches, rawPatch{
					start: field.valueStart,
					end:   field.valueEnd,
					value: reasoningBytes,
				})
			}
		} else {
			insertFields = append(insertFields, makeJSONField("reasoning", buildReasoningObject(request)))
		}
	}

	if patchReasoning {
		if patch, ok := rawReasoningContentReplayPatch(info, raw, request.Model); ok {
			patches = append(patches, patch)
		}
	}

	if len(insertFields) > 0 {
		patches = append(patches, rawPatch{
			start: info.closeIndex,
			end:   info.closeIndex,
			value: buildObjectInsertion(info, insertFields),
		})
	}

	// 无需修改时直接透传原始 body，避免改变序列化格式
	if len(patches) == 0 {
		return raw
	}

	return applyRawPatches(raw, patches)
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

type jsonObjectInfo struct {
	fields     []jsonField
	closeIndex int
}

type jsonField struct {
	key        string
	keyStart   int
	keyEnd     int
	valueStart int
	valueEnd   int
}

type rawPatch struct {
	start int
	end   int
	value []byte
}

func patchReasoningObject(raw []byte, request *InternalLLMRequest) ([]byte, bool) {
	reasoning := buildReasoningObject(request)
	info, ok := parseJSONObject(raw)
	if !ok {
		return reasoning, !jsonEqual(raw, reasoning)
	}

	var patches []rawPatch
	var insertFields [][]byte

	if effortBytes, err := json.Marshal(request.ReasoningEffort); err == nil {
		if field, ok := findJSONField(info, "effort"); ok {
			if !jsonEqual(raw[field.valueStart:field.valueEnd], effortBytes) {
				patches = append(patches, rawPatch{
					start: field.valueStart,
					end:   field.valueEnd,
					value: effortBytes,
				})
			}
		} else {
			insertFields = append(insertFields, makeJSONField("effort", effortBytes))
		}
	}

	if request.ReasoningBudget != nil {
		if budgetBytes, err := json.Marshal(*request.ReasoningBudget); err == nil {
			if field, ok := findJSONField(info, "max_tokens"); ok {
				if !jsonEqual(raw[field.valueStart:field.valueEnd], budgetBytes) {
					patches = append(patches, rawPatch{
						start: field.valueStart,
						end:   field.valueEnd,
						value: budgetBytes,
					})
				}
			} else {
				insertFields = append(insertFields, makeJSONField("max_tokens", budgetBytes))
			}
		}
	}

	if len(insertFields) > 0 {
		patches = append(patches, rawPatch{
			start: info.closeIndex,
			end:   info.closeIndex,
			value: buildObjectInsertion(info, insertFields),
		})
	}
	if len(patches) == 0 {
		return raw, false
	}
	return applyRawPatches(raw, patches), true
}

func buildReasoningObject(request *InternalLLMRequest) []byte {
	effortBytes, _ := json.Marshal(request.ReasoningEffort)
	fields := [][]byte{makeJSONField("effort", effortBytes)}
	if request.ReasoningBudget != nil {
		budgetBytes, _ := json.Marshal(*request.ReasoningBudget)
		fields = append(fields, makeJSONField("max_tokens", budgetBytes))
	}
	body := []byte{'{'}
	for i, field := range fields {
		if i > 0 {
			body = append(body, ',')
		}
		body = append(body, field...)
	}
	body = append(body, '}')
	return body
}

func rawReasoningContentReplayPatch(info jsonObjectInfo, raw []byte, model string) (rawPatch, bool) {
	if !supportsReasoningContentReplay(model) {
		return rawPatch{}, false
	}

	field, ok := findJSONField(info, "messages")
	if !ok {
		return rawPatch{}, false
	}

	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(raw[field.valueStart:field.valueEnd], &messages); err != nil {
		return rawPatch{}, false
	}

	patched := false
	for idx := range messages {
		if patchRawMessageReasoningContentReplay(messages[idx]) {
			patched = true
		}
	}
	if !patched {
		return rawPatch{}, false
	}

	body, err := json.Marshal(messages)
	if err != nil {
		return rawPatch{}, false
	}
	return rawPatch{start: field.valueStart, end: field.valueEnd, value: body}, true
}

func makeJSONField(key string, value []byte) []byte {
	keyBytes, _ := json.Marshal(key)
	field := make([]byte, 0, len(keyBytes)+1+len(value))
	field = append(field, keyBytes...)
	field = append(field, ':')
	field = append(field, value...)
	return field
}

func buildObjectInsertion(info jsonObjectInfo, fields [][]byte) []byte {
	if len(fields) == 0 {
		return nil
	}
	out := make([]byte, 0)
	if len(info.fields) > 0 {
		out = append(out, ',')
	}
	for idx, field := range fields {
		if idx > 0 {
			out = append(out, ',')
		}
		out = append(out, field...)
	}
	return out
}

func applyRawPatches(raw []byte, patches []rawPatch) []byte {
	sort.SliceStable(patches, func(i, j int) bool {
		if patches[i].start == patches[j].start {
			return patches[i].end > patches[j].end
		}
		return patches[i].start > patches[j].start
	})

	out := append([]byte(nil), raw...)
	for _, patch := range patches {
		if patch.start < 0 || patch.end < patch.start || patch.end > len(out) {
			return raw
		}
		next := make([]byte, 0, len(out)-(patch.end-patch.start)+len(patch.value))
		next = append(next, out[:patch.start]...)
		next = append(next, patch.value...)
		next = append(next, out[patch.end:]...)
		out = next
	}
	return out
}

func findJSONField(info jsonObjectInfo, key string) (jsonField, bool) {
	for idx := len(info.fields) - 1; idx >= 0; idx-- {
		if info.fields[idx].key == key {
			return info.fields[idx], true
		}
	}
	return jsonField{}, false
}

func parseJSONObject(raw []byte) (jsonObjectInfo, bool) {
	var info jsonObjectInfo
	i := skipJSONSpace(raw, 0)
	if i >= len(raw) || raw[i] != '{' {
		return info, false
	}
	i++

	for {
		i = skipJSONSpace(raw, i)
		if i >= len(raw) {
			return info, false
		}
		if raw[i] == '}' {
			info.closeIndex = i
			return info, true
		}
		if raw[i] != '"' {
			return info, false
		}

		keyStart := i
		keyEnd, ok := scanJSONString(raw, keyStart)
		if !ok {
			return info, false
		}
		var key string
		if err := json.Unmarshal(raw[keyStart:keyEnd], &key); err != nil {
			return info, false
		}

		i = skipJSONSpace(raw, keyEnd)
		if i >= len(raw) || raw[i] != ':' {
			return info, false
		}
		i = skipJSONSpace(raw, i+1)
		if i >= len(raw) {
			return info, false
		}
		valueStart := i
		valueEnd, ok := scanJSONValue(raw, valueStart)
		if !ok {
			return info, false
		}

		info.fields = append(info.fields, jsonField{
			key:        key,
			keyStart:   keyStart,
			keyEnd:     keyEnd,
			valueStart: valueStart,
			valueEnd:   valueEnd,
		})

		i = skipJSONSpace(raw, valueEnd)
		if i >= len(raw) {
			return info, false
		}
		switch raw[i] {
		case ',':
			i++
		case '}':
			info.closeIndex = i
			return info, true
		default:
			return info, false
		}
	}
}

func skipJSONSpace(raw []byte, i int) int {
	for i < len(raw) {
		switch raw[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			return i
		}
	}
	return i
}

func scanJSONString(raw []byte, start int) (int, bool) {
	if start >= len(raw) || raw[start] != '"' {
		return 0, false
	}
	escaped := false
	for i := start + 1; i < len(raw); i++ {
		switch {
		case escaped:
			escaped = false
		case raw[i] == '\\':
			escaped = true
		case raw[i] == '"':
			return i + 1, true
		}
	}
	return 0, false
}

func scanJSONValue(raw []byte, start int) (int, bool) {
	if start >= len(raw) {
		return 0, false
	}
	switch raw[start] {
	case '"':
		return scanJSONString(raw, start)
	case '{', '[':
		return scanJSONComposite(raw, start)
	default:
		i := start
		for i < len(raw) && !isJSONValueTerminator(raw[i]) {
			i++
		}
		return i, i > start
	}
}

func scanJSONComposite(raw []byte, start int) (int, bool) {
	stack := []byte{raw[start]}
	for i := start + 1; i < len(raw); i++ {
		switch raw[i] {
		case '"':
			end, ok := scanJSONString(raw, i)
			if !ok {
				return 0, false
			}
			i = end - 1
		case '{', '[':
			stack = append(stack, raw[i])
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return 0, false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1, true
			}
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				return 0, false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

func isJSONValueTerminator(b byte) bool {
	switch b {
	case ',', '}', ']', ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
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
