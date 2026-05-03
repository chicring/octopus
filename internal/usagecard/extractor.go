package usagecard

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

// ExtractResult 提取结果
type ExtractResult struct {
	Metrics []model.UsageMetric
	Error   string
}

// Extract 从响应 body 和 headers 中提取用量指标
func Extract(body []byte, headers map[string][]string, config model.UsageCardConfig) ExtractResult {
	var bodyJSON interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &bodyJSON); err != nil {
			return ExtractResult{Error: fmt.Sprintf("解析 JSON 失败: %v", err)}
		}
	}

	metrics := make([]model.UsageMetric, 0, len(config.Metrics))
	for _, mc := range config.Metrics {
		m := extractMetric(bodyJSON, headers, mc)
		metrics = append(metrics, m)
	}
	return ExtractResult{Metrics: metrics}
}

func extractMetric(bodyJSON interface{}, headers map[string][]string, mc model.UsageMetricConfig) model.UsageMetric {
	m := model.UsageMetric{
		ID:     mc.ID,
		Label:  mc.Label,
		Kind:   mc.Kind,
		Unit:   mc.Unit,
		Window: mc.Window,
		Status: "unknown",
	}

	// 提取 limit
	if mc.Limit != nil {
		if v, err := extractField(bodyJSON, headers, mc.Limit); err == nil && v != nil {
			m.Limit = v
		} else if !mc.Limit.Optional {
			// limit 是必需字段但提取失败
			m.Status = "error"
			m.Message = fmt.Sprintf("提取 limit 失败: %v", err)
			return m
		}
	}

	// 提取 used
	if mc.Used != nil {
		if v, err := extractField(bodyJSON, headers, mc.Used); err == nil && v != nil {
			m.Used = v
		}
	}

	// 提取 remaining
	if mc.Remaining != nil {
		if v, err := extractField(bodyJSON, headers, mc.Remaining); err == nil && v != nil {
			m.Remaining = v
		}
	}

	// 提取 reset_at
	if mc.ResetAt != nil {
		if v, err := extractFieldString(bodyJSON, headers, mc.ResetAt); err == nil && v != nil {
			m.ResetAt = v
		}
	}

	// 补全计算
	computeDerived(&m)

	return m
}

// computeDerived 补全 used/remaining/percent/status
func computeDerived(m *model.UsageMetric) {
	// 如果缺少 used 但有 limit 和 remaining
	if m.Used == nil && m.Limit != nil && m.Remaining != nil {
		used := *m.Limit - *m.Remaining
		m.Used = &used
	}

	// 如果缺少 remaining 但有 limit 和 used
	if m.Remaining == nil && m.Limit != nil && m.Used != nil {
		remaining := *m.Limit - *m.Used
		m.Remaining = &remaining
	}

	// 计算 percent
	if m.Limit != nil && m.Used != nil && *m.Limit > 0 {
		pct := *m.Used / *m.Limit * 100
		m.Percent = &pct
	}

	// 状态判断
	if m.Limit == nil && m.Used == nil && m.Remaining == nil {
		m.Status = "unknown"
		return
	}

	// counter 类型没有 limit，只展示数值
	if m.Kind == "counter" && m.Limit == nil {
		m.Status = "ok"
		return
	}

	if m.Remaining != nil && *m.Remaining <= 0 {
		m.Status = "exhausted"
		m.Message = "额度已耗尽"
		return
	}

	if m.Percent != nil {
		if *m.Percent >= 80 {
			m.Status = "warning"
			m.Message = "额度即将耗尽"
			return
		}
	}

	m.Status = "ok"
}

// extractField 提取数值字段
func extractField(bodyJSON interface{}, headers map[string][]string, spec *model.FieldSpec) (*float64, error) {
	raw, err := extractRaw(bodyJSON, headers, spec)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("字段为空")
	}

	// 先尝试直接转 float
	if f, ok := raw.(float64); ok {
		return &f, nil
	}

	// 尝试字符串转 float
	if s, ok := raw.(string); ok {
		return stringToFloat(s, spec.Transform)
	}

	return nil, fmt.Errorf("无法转换为数值: %T", raw)
}

// extractFieldString 提取字符串字段（如 reset_at）
func extractFieldString(bodyJSON interface{}, headers map[string][]string, spec *model.FieldSpec) (*string, error) {
	raw, err := extractRaw(bodyJSON, headers, spec)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("字段为空")
	}

	result, err := transformValue(raw, spec.Transform)
	if err != nil {
		return nil, err
	}

	s := fmt.Sprintf("%v", result)
	return &s, nil
}

// extractRaw 提取原始值
func extractRaw(bodyJSON interface{}, headers map[string][]string, spec *model.FieldSpec) (interface{}, error) {
	switch spec.Source {
	case "header":
		return extractFromHeaders(headers, spec.Path)
	case "body":
		return extractFromJSON(bodyJSON, spec.Path)
	case "static":
		return spec.Path, nil
	default:
		return nil, fmt.Errorf("不支持的 source: %s", spec.Source)
	}
}

// extractFromHeaders 从 HTTP 响应头提取值（大小写不敏感）
func extractFromHeaders(headers map[string][]string, key string) (interface{}, error) {
	lowerKey := strings.ToLower(key)
	for k, vals := range headers {
		if strings.ToLower(k) == lowerKey {
			if len(vals) > 0 {
				return vals[0], nil
			}
		}
	}
	return nil, fmt.Errorf("header %s 不存在", key)
}

// extractFromJSON 从 JSON body 提取值（轻量 JSON path 子集）
// 支持 $.a.b.c 和 $.items[0].limit
func extractFromJSON(data interface{}, path string) (interface{}, error) {
	if !strings.HasPrefix(path, "$.") {
		return nil, fmt.Errorf("JSON path 必须以 $. 开头")
	}

	parts, err := parseJSONPath(path[2:])
	if err != nil {
		return nil, err
	}

	current := data
	for _, part := range parts {
		if current == nil {
			return nil, fmt.Errorf("路径 %s 中遇到 nil", part)
		}

		switch p := part.(type) {
		case keyPart: // 对象键
			obj, ok := current.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("路径 %s 处不是对象", string(p))
			}
			var exists bool
			current, exists = obj[string(p)]
			if !exists {
				return nil, fmt.Errorf("键 %s 不存在", string(p))
			}
		case indexPart: // 数组索引
			arr, ok := current.([]interface{})
			if !ok {
				return nil, fmt.Errorf("路径 [%d] 处不是数组", int(p))
			}
			if int(p) < 0 || int(p) >= len(arr) {
				return nil, fmt.Errorf("数组索引 %d 越界", int(p))
			}
			current = arr[int(p)]
		}
	}

	return current, nil
}

// pathPart 表示 JSON path 的一段
type pathPart interface {
	isPathPart()
}

type keyPart string

func (keyPart) isPathPart() {}

type indexPart int

func (indexPart) isPathPart() {}

// parseJSONPath 解析 JSON path 片段
// "resources.core.limit" -> [keyPart("resources"), keyPart("core"), keyPart("limit")]
// "items[0].limit" -> [keyPart("items"), indexPart(0), keyPart("limit")]
func parseJSONPath(path string) ([]pathPart, error) {
	var parts []pathPart
	i := 0
	for i < len(path) {
		if path[i] == '[' {
			// 数组索引
			j := i + 1
			for j < len(path) && path[j] != ']' {
				j++
			}
			if j >= len(path) {
				return nil, fmt.Errorf("未闭合的数组索引")
			}
			idxStr := path[i+1 : j]
			idx, err := strconv.Atoi(idxStr)
			if err != nil {
				return nil, fmt.Errorf("无效数组索引: %s", idxStr)
			}
			parts = append(parts, indexPart(idx))
			i = j + 1
			if i < len(path) && path[i] == '.' {
				i++
			}
		} else {
			// 对象键
			j := i
			for j < len(path) && path[j] != '.' && path[j] != '[' {
				j++
			}
			key := path[i:j]
			if key == "" {
				return nil, fmt.Errorf("空键名")
			}
			parts = append(parts, keyPart(key))
			i = j
			if i < len(path) && path[i] == '.' {
				i++
			}
		}
	}
	return parts, nil
}

// stringToFloat 字符串转 float64，支持 transform
func stringToFloat(s string, transforms []string) (*float64, error) {
	s = strings.TrimSpace(s)

	for _, t := range transforms {
		switch t {
		case "percent_to_float":
			s = strings.TrimSuffix(s, "%")
		case "to_float":
			// 无额外处理
		}
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("无法转换为数值: %q", s)
	}
	return &f, nil
}

// transformValue 对提取的值应用 transform
func transformValue(raw interface{}, transforms []string) (interface{}, error) {
	if len(transforms) == 0 {
		return raw, nil
	}

	value := raw
	for _, t := range transforms {
		switch t {
		case "epoch_to_iso":
			f, ok := toFloat64(value)
			if !ok {
				return nil, fmt.Errorf("epoch_to_iso: 无法转换为数值")
			}
			// 秒级 epoch
			ts := time.Unix(int64(f), 0).UTC()
			s := ts.Format(time.RFC3339)
			value = s
		case "epoch_ms_to_iso":
			f, ok := toFloat64(value)
			if !ok {
				return nil, fmt.Errorf("epoch_ms_to_iso: 无法转换为数值")
			}
			ts := time.UnixMilli(int64(f)).UTC()
			s := ts.Format(time.RFC3339)
			value = s
		case "percent_to_float":
			s, ok := value.(string)
			if ok {
				s = strings.TrimSuffix(s, "%")
				f, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return nil, fmt.Errorf("percent_to_float: %v", err)
				}
				value = f
			}
		case "to_float":
			s, ok := value.(string)
			if ok {
				f, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return nil, fmt.Errorf("to_float: %v", err)
				}
				value = f
			}
		}
	}
	return value, nil
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}
