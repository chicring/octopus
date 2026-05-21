package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/price"
	transformerModel "github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/utils/log"
)

// RelayMetrics 负责最终的日志收集与持久化
type RelayMetrics struct {
	APIKeyID     int
	RequestModel string
	StartTime    time.Time

	// 活跃请求追踪
	activeRequestID int64

	// 首 Token 时间
	FirstTokenTime time.Time

	// 请求和响应内容
	InternalRequest    *transformerModel.InternalLLMRequest
	InternalResponse   *transformerModel.InternalLLMResponse
	clientResponseBody []byte

	// inbound adapter，用于将 InternalResponse 转为客户端格式记录日志
	inAdapter transformerModel.Inbound

	// 调试信息（差异、错误、转换失败时记录）
	debugContent *model.RelayLogDebugContent

	// 是否流式响应
	streaming bool

	// 统计指标
	ActualModel string
	Stats       model.StatsMetrics

	// 客户端识别
	UserAgent  string
	ClientName string
}

const maxLoggedResponseBodyBytes = 1 << 20
const maxLoggedDebugBodyBytes = 256 << 10

type debugBodyBuffer struct {
	buf       bytes.Buffer
	truncated bool
}

func newDebugBodyBuffer() *debugBodyBuffer {
	return &debugBodyBuffer{}
}

func (b *debugBodyBuffer) Write(p []byte) (int, error) {
	if b.buf.Len() < maxLoggedDebugBodyBytes {
		remaining := maxLoggedDebugBodyBytes - b.buf.Len()
		if len(p) <= remaining {
			_, _ = b.buf.Write(p)
		} else {
			_, _ = b.buf.Write(p[:remaining])
			b.truncated = true
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return len(p), nil
}

func (b *debugBodyBuffer) String() string {
	return b.buf.String()
}

func (b *debugBodyBuffer) Truncated() bool {
	return b.truncated
}

type teeReadCloser struct {
	io.Reader
	io.Closer
}

func NewRelayMetrics(apiKeyID int, requestModel string, req *transformerModel.InternalLLMRequest, userAgent string) *RelayMetrics {
	m := &RelayMetrics{
		APIKeyID:        apiKeyID,
		RequestModel:    requestModel,
		StartTime:       time.Now(),
		InternalRequest: req,
		UserAgent:       userAgent,
		ClientName:      DetectClient(userAgent),
	}
	return m
}

func (m *RelayMetrics) SetStreaming(v bool) {
	m.streaming = v
}

func (m *RelayMetrics) SetFirstTokenTime(t time.Time) {
	m.FirstTokenTime = t
	if m.activeRequestID > 0 {
		op.ActiveRequestUpdateStatus(m.activeRequestID, op.ActiveRequestStreaming)
	}
}

// EnsureDebugContent 确保 debugContent 已初始化
func (m *RelayMetrics) EnsureDebugContent() *model.RelayLogDebugContent {
	if m.debugContent == nil {
		m.debugContent = &model.RelayLogDebugContent{}
	}
	return m.debugContent
}

// SetDebugClientRequest 记录原始客户端请求 body
func (m *RelayMetrics) SetDebugClientRequest(raw string) {
	value, truncated := truncateDebugContent(raw)
	m.setDebugClientRequest(value, truncated)
}

func (m *RelayMetrics) SetDebugClientRequestBytes(raw []byte) {
	value, truncated := truncateDebugBytes(raw)
	m.setDebugClientRequest(value, truncated)
}

func (m *RelayMetrics) setDebugClientRequest(value string, truncated bool) {
	m.EnsureDebugContent().ClientRequest = value
	if truncated {
		m.AddDebugNote("client_request_truncated")
	}
}

// SetDebugUpstreamResponse 记录上游原始响应 body
func (m *RelayMetrics) SetDebugUpstreamResponse(raw string) {
	value, truncated := truncateDebugContent(raw)
	m.SetDebugUpstreamResponseValue(value, truncated)
}

func (m *RelayMetrics) SetDebugUpstreamResponseValue(value string, truncated bool) {
	m.EnsureDebugContent().UpstreamResponse = value
	if truncated {
		m.AddDebugNote("upstream_response_truncated")
	}
}

// SetDebugStreamWire 记录流式失败时的截断 SSE
func (m *RelayMetrics) SetDebugStreamWire(data string) {
	value, truncated := truncateDebugContent(data)
	m.EnsureDebugContent().StreamWire = value
	if truncated {
		m.AddDebugNote("stream_wire_truncated")
	}
}

// AddDebugNote 添加调试说明
func (m *RelayMetrics) AddDebugNote(note string) {
	dc := m.EnsureDebugContent()
	for _, existing := range dc.Notes {
		if existing == note {
			return
		}
	}
	dc.Notes = append(dc.Notes, note)
}

// DebugContentJSON 返回 debugContent 的 JSON 字符串（仅在有内容时返回）
func (m *RelayMetrics) DebugContentJSON() string {
	if m.debugContent == nil {
		return ""
	}
	if m.debugContent.ClientRequest == "" &&
		m.debugContent.UpstreamResponse == "" &&
		m.debugContent.StreamWire == "" &&
		len(m.debugContent.Notes) == 0 {
		return ""
	}
	data, err := json.Marshal(m.debugContent)
	if err != nil {
		return ""
	}
	return string(data)
}

func truncateDebugContent(raw string) (string, bool) {
	if len(raw) <= maxLoggedDebugBodyBytes {
		return raw, false
	}
	return raw[:maxLoggedDebugBodyBytes], true
}

func truncateDebugBytes(raw []byte) (string, bool) {
	if len(raw) <= maxLoggedDebugBodyBytes {
		return string(raw), false
	}
	return string(raw[:maxLoggedDebugBodyBytes]), true
}

func (m *RelayMetrics) SetActiveRequestID(id int64) {
	m.activeRequestID = id
}

func (m *RelayMetrics) ActiveRequestID() int64 {
	return m.activeRequestID
}

func (m *RelayMetrics) SetInternalResponse(resp *transformerModel.InternalLLMResponse, actualModel string) {
	m.InternalResponse = resp
	m.ActualModel = actualModel

	if resp == nil || resp.Usage == nil {
		return
	}

	usage := resp.Usage
	m.Stats.InputToken = usage.PromptTokens
	m.Stats.OutputToken = usage.CompletionTokens

	modelPrice := price.GetLLMPrice(actualModel)
	if modelPrice == nil {
		return
	}
	if usage.PromptTokensDetails == nil {
		usage.PromptTokensDetails = &transformerModel.PromptTokensDetails{
			CachedTokens: 0,
		}
	}
	if usage.AnthropicUsage {
		m.Stats.InputCost = (float64(usage.PromptTokensDetails.CachedTokens)*modelPrice.CacheRead +
			float64(usage.PromptTokens)*modelPrice.Input +
			float64(usage.CacheCreationInputTokens)*modelPrice.CacheWrite) * 1e-6
	} else {
		m.Stats.InputCost = (float64(usage.PromptTokensDetails.CachedTokens)*modelPrice.CacheRead + float64(usage.PromptTokens-usage.PromptTokensDetails.CachedTokens)*modelPrice.Input) * 1e-6
	}
	m.Stats.OutputCost = float64(usage.CompletionTokens) * modelPrice.Output * 1e-6
}

func (m *RelayMetrics) SetClientResponseBody(body []byte) {
	if len(body) > maxLoggedResponseBodyBytes {
		body = body[:maxLoggedResponseBodyBytes]
	}
	m.clientResponseBody = append(m.clientResponseBody[:0], body...)
}

func (m *RelayMetrics) AppendClientResponseBody(chunk []byte) {
	if len(chunk) == 0 || len(m.clientResponseBody) >= maxLoggedResponseBodyBytes {
		return
	}
	remaining := maxLoggedResponseBodyBytes - len(m.clientResponseBody)
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
	}
	m.clientResponseBody = append(m.clientResponseBody, chunk...)
}

func (m *RelayMetrics) ClientResponseBody() []byte {
	return m.clientResponseBody
}

// SaveEarlyFailure 记录早期失败（解析失败、模型不支持等），此时无渠道信息
func (m *RelayMetrics) SaveEarlyFailure(err error) {
	duration := time.Since(m.StartTime)
	globalStats := model.StatsMetrics{
		WaitTime:      duration.Milliseconds(),
		RequestFailed: 1,
	}
	op.StatsTotalUpdate(globalStats)
	op.StatsHourlyUpdate(globalStats)
	op.StatsDailyUpdate(context.Background(), globalStats)
	op.StatsAPIKeyUpdate(m.APIKeyID, globalStats)
	if m.APIKeyID > 0 {
		op.StatsAPIKeyDailyUpdate(uint(m.APIKeyID), globalStats)
		op.StatsAPIKeyHourlyUpdate(uint(m.APIKeyID), globalStats)
	}
	op.StatsRealtimeRecord(0) // 早期失败无输出 Token
	if m.RequestModel != "" {
		op.StatsModelUpdate(m.RequestModel, globalStats)
	}
	m.saveLog(context.Background(), err, duration, nil, 0, "")
}

func (m *RelayMetrics) Save(ctx context.Context, success bool, err error, attempts []model.ChannelAttempt) {
	duration := time.Since(m.StartTime)

	// 计算输出阶段耗时（总耗时 - 首 Token 时间）
	outputTime := duration.Milliseconds()
	if !m.FirstTokenTime.IsZero() {
		ftutMs := m.FirstTokenTime.Sub(m.StartTime).Milliseconds()
		if ftutMs > 0 && outputTime > ftutMs {
			outputTime = outputTime - ftutMs
		}
	}

	globalStats := model.StatsMetrics{
		WaitTime:    duration.Milliseconds(),
		OutputTime:  outputTime,
		InputToken:  m.Stats.InputToken,
		OutputToken: m.Stats.OutputToken,
		InputCost:   m.Stats.InputCost,
		OutputCost:  m.Stats.OutputCost,
	}
	if success {
		globalStats.RequestSuccess = 1
	} else {
		globalStats.RequestFailed = 1
	}

	channelID, channelName := finalChannel(attempts)
	op.StatsTotalUpdate(globalStats)
	op.StatsHourlyUpdate(globalStats)
	op.StatsDailyUpdate(context.Background(), globalStats)
	op.StatsAPIKeyUpdate(m.APIKeyID, globalStats)
	if m.APIKeyID > 0 {
		op.StatsAPIKeyDailyUpdate(uint(m.APIKeyID), globalStats)
		op.StatsAPIKeyHourlyUpdate(uint(m.APIKeyID), globalStats)
	}
	// 仅在有有效渠道时更新渠道统计（避免 channel=0 的无效记录）
	// 渠道维度：Token/Cost/WaitTime 由 Save 统一记录，成功时记录 success，
	// 失败时不记录 RequestFailed（attempt() 已按每次尝试记录）
	if channelID > 0 {
		channelStats := model.StatsMetrics{
			WaitTime:    duration.Milliseconds(),
			OutputTime:  outputTime,
			InputToken:  m.Stats.InputToken,
			OutputToken: m.Stats.OutputToken,
			InputCost:   m.Stats.InputCost,
			OutputCost:  m.Stats.OutputCost,
		}
		if success {
			channelStats.RequestSuccess = 1
		}
		op.StatsChannelUpdate(channelID, channelStats)
	}
	// 更新模型统计（使用实际模型名，fallback 到请求模型名）
	modelName := m.ActualModel
	if modelName == "" {
		modelName = m.RequestModel
	}
	if modelName != "" {
		op.StatsModelUpdate(modelName, globalStats)
	}

	// 实时监控计数
	op.StatsRealtimeRecord(m.Stats.OutputToken)

	log.Infof("relay complete: model=%s, channel=%d(%s), success=%t, duration=%dms, input_token=%d, output_token=%d, input_cost=%f, output_cost=%f, total_cost=%f, attempts=%d",
		m.RequestModel, channelID, channelName, success, duration.Milliseconds(),
		m.Stats.InputToken, m.Stats.OutputToken,
		m.Stats.InputCost, m.Stats.OutputCost, m.Stats.InputCost+m.Stats.OutputCost,
		len(attempts))

	m.saveLog(context.Background(), err, duration, attempts, channelID, channelName)
}

func finalChannel(attempts []model.ChannelAttempt) (int, string) {
	var lastID int
	var lastName string
	for i := len(attempts) - 1; i >= 0; i-- {
		a := attempts[i]
		if a.Status == model.AttemptSuccess {
			name := a.ChannelName
			if name == "" {
				name = fmt.Sprintf("channel_%d", a.ChannelID)
			}
			return a.ChannelID, name
		}
		if a.Status == model.AttemptFailed && lastID == 0 {
			lastID = a.ChannelID
			lastName = a.ChannelName
		}
	}
	// 如果没有 success/failed，取最后一个被跳过的通道用于日志记录
	if lastID == 0 && len(attempts) > 0 {
		last := attempts[len(attempts)-1]
		lastID = last.ChannelID
		lastName = last.ChannelName
	}
	// 确保 channelName 不为空
	if lastName == "" && lastID > 0 {
		lastName = fmt.Sprintf("channel_%d", lastID)
	}
	return lastID, lastName
}

func (m *RelayMetrics) saveLog(ctx context.Context, err error, duration time.Duration, attempts []model.ChannelAttempt, channelID int, channelName string) {
	actualModel := m.ActualModel
	if actualModel == "" {
		actualModel = m.RequestModel
	}

	var reasoningEffort string
	if m.InternalRequest != nil {
		reasoningEffort = m.InternalRequest.ReasoningEffort
	}

	relayLog := model.RelayLog{
		Time:             m.StartTime.Unix(),
		RequestModelName: m.RequestModel,
		ChannelName:      channelName,
		ChannelId:        channelID,
		ActualModelName:  actualModel,
		ReasoningEffort:  reasoningEffort,
		UseTime:          int(duration.Milliseconds()),
		Attempts:         attempts,
		TotalAttempts:    len(attempts),
		UserAgent:        m.UserAgent,
		ClientName:       m.ClientName,
	}

	if apiKey, getErr := op.APIKeyGet(m.APIKeyID, context.Background()); getErr == nil {
		relayLog.RequestAPIKeyName = apiKey.Name
	}

	// 首字时间
	if !m.FirstTokenTime.IsZero() {
		relayLog.Ftut = int(m.FirstTokenTime.Sub(m.StartTime).Milliseconds())
	}

	// Usage
	if m.InternalResponse != nil && m.InternalResponse.Usage != nil {
		relayLog.InputTokens = int(m.InternalResponse.Usage.PromptTokens)
		relayLog.OutputTokens = int(m.InternalResponse.Usage.CompletionTokens)
		relayLog.Cost = m.Stats.InputCost + m.Stats.OutputCost
		if m.InternalResponse.Usage.PromptTokensDetails != nil {
			relayLog.CachedTokens = int(m.InternalResponse.Usage.PromptTokensDetails.CachedTokens)
		}
		relayLog.CacheCreationTokens = int(m.InternalResponse.Usage.CacheCreationInputTokens)
	}

	// 请求内容：优先记录实际发送给上游的 body（含 model/patch 变更），
	// 回退到原始入站 body，最后记录内部 JSON 格式。
	if m.InternalRequest != nil {
		if len(m.InternalRequest.UpstreamRequestBody) > 0 {
			relayLog.RequestContent = string(m.InternalRequest.UpstreamRequestBody)
		} else if m.InternalRequest.RawAPIFormat != "" && len(m.InternalRequest.RawRequest) > 0 {
			relayLog.RequestContent = string(m.InternalRequest.RawRequest)
		} else if reqJSON, jsonErr := json.Marshal(m.InternalRequest); jsonErr == nil {
			relayLog.RequestContent = string(reqJSON)
		}
	}

	// 响应内容
	if m.streaming && m.InternalResponse != nil {
		// 流式成功：使用聚合后的最终 JSON，而非 SSE 原始帧
		relayLog.ResponseContent = m.renderInternalResponseContent("streaming")
	} else if len(m.clientResponseBody) > 0 {
		// 非流式（passthrough/conversion）或流式失败时：使用客户端实际收到的 body
		relayLog.ResponseContent = strings.TrimSuffix(string(m.clientResponseBody), "\n\n")
	} else if m.InternalResponse != nil {
		relayLog.ResponseContent = m.renderInternalResponseContent("")
	}

	// 调试信息：记录 passthrough patch 差异、conversion 上游原始响应、流式失败截断等
	if dc := m.DebugContentJSON(); dc != "" {
		relayLog.DebugContent = dc
	} else if m.InternalRequest != nil {
		// 自动检测 passthrough 差异：request_content 是 UpstreamRequestBody，如果 RawRequest 不同则记录
		hasUpstream := len(m.InternalRequest.UpstreamRequestBody) > 0
		hasRaw := m.InternalRequest.RawAPIFormat != "" && len(m.InternalRequest.RawRequest) > 0
		if hasUpstream && hasRaw && !bytes.Equal(m.InternalRequest.UpstreamRequestBody, m.InternalRequest.RawRequest) {
			m.SetDebugClientRequestBytes(m.InternalRequest.RawRequest)
			relayLog.DebugContent = m.DebugContentJSON()
		}
	}

	// 错误信息
	if err != nil {
		relayLog.Error = err.Error()
	}

	if logErr := op.RelayLogAdd(context.Background(), relayLog); logErr != nil {
		log.Warnf("failed to save relay log: %v", logErr)
	}
}

func (m *RelayMetrics) renderInternalResponseContent(contextName string) string {
	if m.InternalResponse == nil {
		return ""
	}
	if m.inAdapter != nil {
		if clientBody, err := m.inAdapter.ConvertResponseToClientFormat(context.Background(), m.InternalResponse); err == nil && len(clientBody) > 0 {
			return string(clientBody)
		} else if err != nil {
			if contextName != "" {
				log.Warnf("failed to convert %s response content for log: %v", contextName, err)
			} else {
				log.Warnf("failed to convert response content for log: %v", err)
			}
		}
	}
	respForLog := m.filterResponseForLog(m.InternalResponse)
	if respJSON, jsonErr := json.Marshal(respForLog); jsonErr == nil {
		return string(respJSON)
	}
	return ""
}

// filterResponseForLog 创建响应的浅拷贝，过滤掉 images、MultipleContent 中的图片数据和 Audio.Data 以减少存储压力
func (m *RelayMetrics) filterResponseForLog(resp *transformerModel.InternalLLMResponse) *transformerModel.InternalLLMResponse {
	if resp == nil {
		return nil
	}

	filterMsg := func(msg *transformerModel.Message) *transformerModel.Message {
		if msg == nil {
			return nil
		}
		c := *msg
		c.Images = nil
		if len(c.Content.MultipleContent) > 0 {
			parts := make([]transformerModel.MessageContentPart, 0, len(c.Content.MultipleContent))
			for _, p := range c.Content.MultipleContent {
				if p.Type == "image_url" && p.ImageURL != nil {
					parts = append(parts, transformerModel.MessageContentPart{
						Type:     "image_url",
						ImageURL: &transformerModel.ImageURL{URL: "[image data omitted for storage]"},
					})
				} else {
					parts = append(parts, p)
				}
			}
			c.Content = transformerModel.MessageContent{Content: c.Content.Content, MultipleContent: parts}
		}
		if c.Audio != nil && c.Audio.Data != "" {
			a := *c.Audio
			a.Data = "[audio data omitted for storage]"
			c.Audio = &a
		}
		return &c
	}

	filtered := *resp
	filtered.Choices = make([]transformerModel.Choice, len(resp.Choices))
	for i, choice := range resp.Choices {
		filtered.Choices[i] = choice
		filtered.Choices[i].Message = filterMsg(choice.Message)
		filtered.Choices[i].Delta = filterMsg(choice.Delta)
	}
	return &filtered
}
