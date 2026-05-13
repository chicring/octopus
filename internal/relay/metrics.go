package relay

import (
	"context"
	"encoding/json"
	"fmt"
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
	InternalRequest  *transformerModel.InternalLLMRequest
	InternalResponse *transformerModel.InternalLLMResponse

	// inbound adapter，用于将 InternalResponse 转为客户端格式记录日志
	inAdapter transformerModel.Inbound

	// 统计指标
	ActualModel string
	Stats       model.StatsMetrics

	// 客户端识别
	UserAgent  string
	ClientName string
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

func (m *RelayMetrics) SetFirstTokenTime(t time.Time) {
	m.FirstTokenTime = t
	if m.activeRequestID > 0 {
		op.ActiveRequestUpdateStatus(m.activeRequestID, op.ActiveRequestStreaming)
	}
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

	// 响应内容：通过 inbound adapter 转为客户端实际格式记录
	if m.InternalResponse != nil {
		if m.InternalRequest != nil && transformerModel.IsPassthrough(m.InternalRequest, m.InternalRequest.RawAPIFormat) {
			respForLog := m.filterResponseForLog(m.InternalResponse)
			if respJSON, jsonErr := json.Marshal(respForLog); jsonErr == nil {
				relayLog.ResponseContent = string(respJSON)
			}
		} else if m.inAdapter != nil {
			if clientBody, err := m.inAdapter.ConvertResponseToClientFormat(context.Background(), m.InternalResponse); err == nil && len(clientBody) > 0 {
				relayLog.ResponseContent = string(clientBody)
			} else {
				respForLog := m.filterResponseForLog(m.InternalResponse)
				if respJSON, jsonErr := json.Marshal(respForLog); jsonErr == nil {
					relayLog.ResponseContent = string(respJSON)
				}
				if err != nil {
					log.Warnf("failed to convert response content for log: %v", err)
				}
			}
		} else {
			// fallback: 记录内部格式
			respForLog := m.filterResponseForLog(m.InternalResponse)
			if respJSON, jsonErr := json.Marshal(respForLog); jsonErr == nil {
				relayLog.ResponseContent = string(respJSON)
			}
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
