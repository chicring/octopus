package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/transformer/inbound"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
	"github.com/tmaxmax/go-sse"
)

var errClientDisconnected = errors.New("client disconnected during stream")
var errStreamTransformAfterWrite = errors.New("stream transform error after response started")

// Handler 处理入站请求并转发到上游服务
func Handler(inboundType inbound.InboundType, c *gin.Context) {
	// 解析请求
	internalRequest, inAdapter, err := parseRequest(inboundType, c)
	if err != nil {
		return
	}

	requestModel := internalRequest.Model
	apiKeyID := c.GetInt("api_key_id")

	// 提前初始化 Metrics，确保早期失败也有日志记录
	metrics := NewRelayMetrics(apiKeyID, requestModel, internalRequest, c.Request.UserAgent())
	metrics.inAdapter = inAdapter

	// 注册活跃请求
	var apiKeyName string
	if apiKey, getErr := op.APIKeyGet(apiKeyID, c.Request.Context()); getErr == nil {
		apiKeyName = apiKey.Name
	}
	activeID := op.ActiveRequestRegister(requestModel, apiKeyName)
	metrics.SetActiveRequestID(activeID)
	defer func() {
		op.ActiveRequestUnregister(activeID)
	}()

	supportedModels := c.GetString("supported_models")
	if supportedModels != "" {
		supportedModelsArray := strings.Split(supportedModels, ",")
		if !slices.Contains(supportedModelsArray, internalRequest.Model) {
			resp.Error(c, http.StatusBadRequest, "model not supported")
			metrics.SaveEarlyFailure(fmt.Errorf("model not supported: %s", internalRequest.Model))
			return
		}
	}

	// 获取通道分组
	group, err := op.GroupGetEnabledMap(requestModel, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "model not found")
		metrics.SaveEarlyFailure(fmt.Errorf("model not found: %s", requestModel))
		return
	}

	// 分组覆盖思考等级
	if group.ReasoningEffortOverride != "" {
		internalRequest.ReasoningEffort = group.ReasoningEffortOverride
		internalRequest.ReasoningBudget = nil
		internalRequest.AdaptiveThinking = false
	}

	// 创建迭代器（策略排序 + 粘性优先）
	iter := balancer.NewIterator(group, apiKeyID, requestModel)
	if iter.Len() == 0 {
		resp.Error(c, http.StatusServiceUnavailable, "no available channel")
		metrics.SaveEarlyFailure(fmt.Errorf("no available channel for model: %s", requestModel))
		return
	}

	// 请求级上下文
	req := &relayRequest{
		c:               c,
		inAdapter:       inAdapter,
		internalRequest: internalRequest,
		metrics:         metrics,
		apiKeyID:        apiKeyID,
		requestModel:    requestModel,
		iter:            iter,
	}

	var lastErr error

	for iter.Next() {
		select {
		case <-c.Request.Context().Done():
			log.Infof("request context canceled, stopping retry")
			metrics.Save(c.Request.Context(), false, context.Canceled, iter.Attempts())
			return
		default:
		}

		item := iter.Item()

		// 获取通道
		channel, err := op.ChannelGet(item.ChannelID, c.Request.Context())
		if err != nil {
			log.Warnf("failed to get channel %d: %v", item.ChannelID, err)
			iter.Skip(item.ChannelID, 0, fmt.Sprintf("channel_%d", item.ChannelID), fmt.Sprintf("channel not found: %v", err))
			lastErr = err
			continue
		}

		// 更新活跃请求的渠道信息
		if metrics.ActiveRequestID() > 0 {
			op.ActiveRequestUpdateChannel(metrics.ActiveRequestID(), channel.Name, iter.Index()+1)
			op.ActiveRequestUpdateStatus(metrics.ActiveRequestID(), op.ActiveRequestForwarding)
		}
		if !channel.Enabled {
			iter.Skip(channel.ID, 0, channel.Name, "channel disabled")
			lastErr = fmt.Errorf("channel %s disabled", channel.Name)
			continue
		}

		usedKey := op.ChannelGetKeyForModel(channel.ID, requestModel)
		if usedKey.ChannelKey == "" {
			iter.Skip(channel.ID, 0, channel.Name, "no available key for model")
			lastErr = fmt.Errorf("channel %s: no available key for model %s", channel.Name, requestModel)
			continue
		}

		// 熔断检查
		if iter.SkipCircuitBreak(channel.ID, usedKey.ID, channel.Name) {
			lastErr = fmt.Errorf("channel %s: circuit breaker tripped", channel.Name)
			continue
		}

		// Codex auth 凭证：请求前检查过期并刷新
		if op.IsCodexAuthKey(usedKey.ChannelKey) {
			newKeyStr, ready, ensureErr := op.EnsureCodexKeyReady(c.Request.Context(), &usedKey)
			if !ready {
				iter.Skip(channel.ID, usedKey.ID, channel.Name, fmt.Sprintf("codex auth not ready: %v", ensureErr))
				lastErr = ensureErr
				continue
			}
			usedKey.ChannelKey = newKeyStr
		}

		// 出站适配器 — 优先 provider_id，回退 legacy type
		pid := provider.ResolveProviderIDFromType(channel.Type)
		if channel.ProviderID != "" {
			pid = provider.ProviderID(channel.ProviderID)
		}
		var outAdapter model.Outbound
		if pid != "" {
			outAdapter = provider.GetOutbound(pid)
		}
		if outAdapter == nil {
			outAdapter = outbound.Get(channel.Type)
		}
		if outAdapter == nil {
			iter.Skip(channel.ID, usedKey.ID, channel.Name, fmt.Sprintf("unsupported channel type: %d", channel.Type))
			continue
		}

		// 类型兼容性检查
		if internalRequest.IsEmbeddingRequest() && !provider.IsEmbeddingProvider(pid) && !outbound.IsEmbeddingChannelType(channel.Type) {
			iter.Skip(channel.ID, usedKey.ID, channel.Name, "channel type not compatible with embedding request")
			continue
		}
		if internalRequest.IsChatRequest() && !provider.IsChatProvider(pid) && !outbound.IsChatChannelType(channel.Type) {
			iter.Skip(channel.ID, usedKey.ID, channel.Name, "channel type not compatible with chat request")
			continue
		}

		// 设置实际模型
		internalRequest.Model = item.ModelName
		model.ClearPassthrough(internalRequest)

		// 重置入站适配器的尝试级响应状态，避免前次尝试的残留数据
		// （streamChunks/storedResponse 等）污染后续的响应聚合
		inAdapter.Reset()

		log.Infof("request model %s, mode: %d, forwarding to channel: %s model: %s (attempt %d/%d, sticky=%t)",
			requestModel, group.Mode, channel.Name, item.ModelName,
			iter.Index()+1, iter.Len(), iter.IsSticky())

		// 构造尝试级上下文 -- 只写变化的 4 个字段
		ra := &relayAttempt{
			relayRequest:         req,
			outAdapter:           outAdapter,
			channel:              channel,
			usedKey:              usedKey,
			firstTokenTimeOutSec: group.FirstTokenTimeOut,
		}

		result := ra.attempt()
		if result.Success {
			metrics.Save(c.Request.Context(), true, nil, iter.Attempts())
			return
		}
		if result.Written {
			metrics.Save(c.Request.Context(), false, result.Err, iter.Attempts())
			return
		}
		lastErr = result.Err
	}

	// 所有通道都失败或被跳过
	var hasAttempt bool
	for _, a := range iter.Attempts() {
		if a.Status == dbmodel.AttemptSuccess || a.Status == dbmodel.AttemptFailed {
			hasAttempt = true
			break
		}
	}
	metrics.Save(c.Request.Context(), false, lastErr, iter.Attempts())
	if hasAttempt {
		resp.Error(c, http.StatusBadGateway, "all channels failed")
	} else {
		resp.Error(c, http.StatusServiceUnavailable, "no available channel")
	}
}

// attempt 统一管理一次通道尝试的完整生命周期
func (ra *relayAttempt) attempt() attemptResult {
	span := ra.iter.StartAttempt(ra.channel.ID, ra.usedKey.ID, ra.channel.Name, ra.usedKey.Remark)

	// 更新活跃请求状态
	if ra.metrics.ActiveRequestID() > 0 {
		if ra.internalRequest.Stream != nil && *ra.internalRequest.Stream {
			op.ActiveRequestUpdateStatus(ra.metrics.ActiveRequestID(), op.ActiveRequestWaitingFirstTok)
		} else {
			op.ActiveRequestUpdateStatus(ra.metrics.ActiveRequestID(), op.ActiveRequestProcessing)
		}
	}

	// 转发请求
	statusCode, fwdErr := ra.forward()

	// 更新 channel key 状态
	ra.usedKey.StatusCode = statusCode
	ra.usedKey.LastUseTimeStamp = time.Now().Unix()

	if fwdErr == nil {
		// ====== 成功 ======
		ok, emptyReason := ra.collectResponse()
		if !ok {
			if ra.c.Writer.Written() {
				// 响应已经写给客户端时不能再把“日志聚合为空”当失败重试，
				// 否则会破坏同格式透传场景的 prompt cache，造成重复扣费。
				log.Warnf("channel %s response was written but log aggregation is empty (status %d): %s", ra.channel.Name, statusCode, emptyReason)
			} else {
				// 上游返回 2xx 但响应为空（如余额不足返回空响应），视为失败以触发重试
				emptyRespErr := fmt.Errorf("channel %s returned empty response (status %d): %s", ra.channel.Name, statusCode, emptyReason)
				ra.usedKey.TotalRequests++
				op.ChannelKeyUpdate(ra.usedKey)
				span.End(dbmodel.AttemptFailed, statusCode, emptyRespErr.Error())
				op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{RequestFailed: 1})
				balancer.RecordFailure(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)
				return attemptResult{
					Success: false,
					Written: false,
					Err:     emptyRespErr,
				}
			}
		}
		ra.usedKey.TotalCost += ra.metrics.Stats.InputCost + ra.metrics.Stats.OutputCost
		ra.usedKey.TotalRequests++
		ra.usedKey.TotalInputToken += ra.metrics.Stats.InputToken
		ra.usedKey.TotalOutputToken += ra.metrics.Stats.OutputToken
		op.ChannelKeyUpdate(ra.usedKey)

		span.End(dbmodel.AttemptSuccess, statusCode, "")

		// 熔断器：记录成功
		balancer.RecordSuccess(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)
		// 会话保持：更新粘性记录
		balancer.SetSticky(ra.apiKeyID, ra.requestModel, ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)

		return attemptResult{Success: true}
	}

	// ====== 失败 ======

	// Codex auth 401：刷新 token 后重试一次
	if statusCode == 401 && op.IsCodexAuthKey(ra.usedKey.ChannelKey) && !ra.c.Writer.Written() {
		log.Infof("[codex-auth] key %d: received 401, attempting forced refresh and retry", ra.usedKey.ID)
		newKeyStr, refreshErr := op.RefreshCodexKey(ra.c.Request.Context(), &ra.usedKey, true)
		if refreshErr == nil {
			// 刷新成功，用新 key 重试一次
			ra.usedKey.ChannelKey = newKeyStr
			retryStatusCode, retryErr := ra.forward()
			ra.usedKey.StatusCode = retryStatusCode
			ra.usedKey.LastUseTimeStamp = time.Now().Unix()

			if retryErr == nil {
				// 重试成功
				ok, emptyReason := ra.collectResponse()
				if ok {
					ra.usedKey.TotalCost += ra.metrics.Stats.InputCost + ra.metrics.Stats.OutputCost
					ra.usedKey.TotalRequests++
					ra.usedKey.TotalInputToken += ra.metrics.Stats.InputToken
					ra.usedKey.TotalOutputToken += ra.metrics.Stats.OutputToken
					op.ChannelKeyUpdate(ra.usedKey)
					span.End(dbmodel.AttemptSuccess, retryStatusCode, "")
					balancer.RecordSuccess(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)
					balancer.SetSticky(ra.apiKeyID, ra.requestModel, ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)
					return attemptResult{Success: true}
				}
				if ra.c.Writer.Written() {
					log.Warnf("channel %s response was written after 401 retry but log aggregation is empty (status %d): %s", ra.channel.Name, retryStatusCode, emptyReason)
					return attemptResult{Success: true}
				}
				// 重试成功但响应为空
				emptyRespErr := fmt.Errorf("channel %s returned empty response after 401 retry (status %d): %s", ra.channel.Name, retryStatusCode, emptyReason)
				ra.usedKey.TotalRequests++
				op.ChannelKeyUpdate(ra.usedKey)
				span.End(dbmodel.AttemptFailed, retryStatusCode, emptyRespErr.Error())
				op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{RequestFailed: 1})
				balancer.RecordFailure(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)
				return attemptResult{Success: false, Written: false, Err: emptyRespErr}
			}
			// 重试仍失败，自动关闭 key
			log.Warnf("[codex-auth] key %d: 401 retry still failed (status %d), disabling key", ra.usedKey.ID, retryStatusCode)
			op.DisableCodexKey(ra.c.Request.Context(), &ra.usedKey, 401)
			ra.usedKey.TotalRequests++
			op.ChannelKeyUpdate(ra.usedKey)
			span.End(dbmodel.AttemptFailed, retryStatusCode, retryErr.Error())
			op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{RequestFailed: 1})
			balancer.RecordFailure(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)
			return attemptResult{Success: false, Written: ra.c.Writer.Written(), Err: fmt.Errorf("channel %s: 401 retry failed: %v", ra.channel.Name, retryErr)}
		}
		// 刷新失败，自动关闭 key
		log.Warnf("[codex-auth] key %d: 401 refresh failed: %v, disabling key", ra.usedKey.ID, refreshErr)
		op.DisableCodexKey(ra.c.Request.Context(), &ra.usedKey, 401)
	}

	ra.usedKey.TotalRequests++
	op.ChannelKeyUpdate(ra.usedKey)
	span.End(dbmodel.AttemptFailed, statusCode, fwdErr.Error())

	// Channel 维度统计：仅记录每次尝试的失败（成功由 metrics.Save 统一记录）
	op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{
		RequestFailed: 1,
	})

	// 熔断器：记录失败
	balancer.RecordFailure(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)

	written := ra.c.Writer.Written()
	if written {
		ra.collectResponse()
	}
	return attemptResult{
		Success: false,
		Written: written,
		Err:     fmt.Errorf("channel %s failed: %v", ra.channel.Name, fwdErr),
	}
}

// parseRequest 解析并验证入站请求
func parseRequest(inboundType inbound.InboundType, c *gin.Context) (*model.InternalLLMRequest, model.Inbound, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return nil, nil, err
	}

	inAdapter := inbound.Get(inboundType)
	internalRequest, err := inAdapter.TransformRequest(c.Request.Context(), body)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return nil, nil, err
	}

	// 保存原始请求 body 和 API 格式，用于相同格式时的直接透传
	internalRequest.RawRequest = body
	switch inboundType {
	case inbound.InboundTypeOpenAIChat:
		internalRequest.RawAPIFormat = model.APIFormatOpenAIChatCompletion
	case inbound.InboundTypeOpenAIResponse:
		internalRequest.RawAPIFormat = model.APIFormatOpenAIResponse
	case inbound.InboundTypeAnthropic:
		internalRequest.RawAPIFormat = model.APIFormatAnthropicMessage
	case inbound.InboundTypeGemini:
		internalRequest.RawAPIFormat = model.APIFormatGeminiContents
	case inbound.InboundTypeOpenAIEmbedding:
		internalRequest.RawAPIFormat = model.APIFormatOpenAIEmbedding
	}
	if inboundType == inbound.InboundTypeGemini {
		if routeModel := strings.TrimSpace(c.GetString("gemini_model")); routeModel != "" {
			internalRequest.Model = routeModel
		}
		if stream, exists := c.Get("gemini_stream"); exists {
			streamValue := stream.(bool)
			internalRequest.Stream = &streamValue
		}
	}

	// Pass through the original query parameters
	internalRequest.Query = c.Request.URL.Query()

	if err := internalRequest.Validate(); err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return nil, nil, err
	}

	return internalRequest, inAdapter, nil
}

// forward 转发请求到上游服务
func (ra *relayAttempt) forward() (int, error) {
	ctx := ra.c.Request.Context()

	// 构建出站请求
	outboundRequest, err := ra.outAdapter.TransformRequest(
		ctx,
		ra.internalRequest,
		ra.channel.GetBaseUrl(),
		ra.usedKey.ChannelKey,
	)
	if err != nil {
		log.Warnf("failed to create request: %v", err)
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// 复制请求头
	ra.copyHeaders(outboundRequest)

	// 发送请求
	response, err := ra.sendRequest(outboundRequest)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer response.Body.Close()

	// 检查响应状态
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return response.StatusCode, fmt.Errorf("upstream error: %d: failed to read body: %w", response.StatusCode, err)
		}
		return response.StatusCode, fmt.Errorf("upstream error: %d: %s", response.StatusCode, string(body))
	}

	// 处理响应
	if ra.internalRequest.Stream != nil && *ra.internalRequest.Stream {
		ra.metrics.SetStreaming(true)
		if err := ra.handleStreamResponse(ctx, response); err != nil {
			return 0, err
		}
		return response.StatusCode, nil
	}
	if err := ra.handleResponse(ctx, response); err != nil {
		return 0, err
	}
	return response.StatusCode, nil
}

// copyHeaders 复制请求头，过滤 hop-by-hop 头
func (ra *relayAttempt) copyHeaders(outboundRequest *http.Request) {
	for key, values := range ra.c.Request.Header {
		if hopByHopHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			outboundRequest.Header.Set(key, value)
		}
	}
	if len(ra.channel.CustomHeader) > 0 {
		for _, header := range ra.channel.CustomHeader {
			outboundRequest.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}
}

// sendRequest 发送 HTTP 请求
func (ra *relayAttempt) sendRequest(req *http.Request) (*http.Response, error) {
	httpClient, err := helper.ChannelHttpClient(ra.channel)
	if err != nil {
		log.Warnf("failed to get http client: %v", err)
		return nil, err
	}

	response, err := httpClient.Do(req)
	if err != nil {
		log.Warnf("failed to send request: %v", err)
		return nil, err
	}

	return response, nil
}

// handleStreamResponse 处理流式响应
func (ra *relayAttempt) handleStreamResponse(ctx context.Context, response *http.Response) error {
	if ct := response.Header.Get("Content-Type"); ct != "" && !strings.Contains(strings.ToLower(ct), "text/event-stream") {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 16*1024))
		return fmt.Errorf("upstream returned non-SSE content-type %q for stream request: %s", ct, string(body))
	}

	// 设置 SSE 响应头
	ra.c.Header("Content-Type", "text/event-stream")
	ra.c.Header("Cache-Control", "no-cache")
	ra.c.Header("Connection", "keep-alive")
	ra.c.Header("X-Accel-Buffering", "no")

	firstToken := true

	type sseReadResult struct {
		data string
		typ  string
		err  error
	}
	results := make(chan sseReadResult, 1)
	go func() {
		defer close(results)
		readCfg := &sse.ReadConfig{MaxEventSize: maxSSEEventSize}
		for ev, err := range sse.Read(response.Body, readCfg) {
			if err != nil {
				results <- sseReadResult{err: err}
				return
			}
			results <- sseReadResult{data: ev.Data, typ: ev.Type}
		}
	}()

	var firstTokenTimer *time.Timer
	var firstTokenC <-chan time.Time
	if firstToken && ra.firstTokenTimeOutSec > 0 {
		firstTokenTimer = time.NewTimer(time.Duration(ra.firstTokenTimeOutSec) * time.Second)
		firstTokenC = firstTokenTimer.C
		defer func() {
			if firstTokenTimer != nil {
				firstTokenTimer.Stop()
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			log.Infof("client disconnected, stopping stream")
			_ = response.Body.Close()
			return errClientDisconnected
		case <-firstTokenC:
			log.Warnf("first token timeout (%ds), switching channel", ra.firstTokenTimeOutSec)
			_ = response.Body.Close()
			return fmt.Errorf("first token timeout (%ds)", ra.firstTokenTimeOutSec)
		case r, ok := <-results:
			if !ok {
				log.Infof("stream end")
				return nil
			}
			if r.err != nil {
				log.Warnf("failed to read event: %v", r.err)
				ra.metrics.AddDebugNote("stream_read_error")
				ra.metrics.AddDebugNote(r.err.Error())
				if ssb := ra.metrics.ClientResponseBody(); len(ssb) > 0 {
					ra.metrics.SetDebugStreamWire(string(ssb))
				}
				return fmt.Errorf("failed to read stream event: %w", r.err)
			}

			data, err := ra.transformStreamData(ctx, r.data, r.typ)
			if err != nil {
				var respErr *model.ResponseError
				if errors.As(err, &respErr) {
					_ = response.Body.Close()
					return fmt.Errorf("upstream stream error: %w", respErr)
				}
				// 非 ResponseError 的转换错误
				if firstToken {
					// 尚未写入任何数据给客户端 → 终止本次尝试，返回错误以触发重试
					log.Warnf("stream transform error before first token: %v", err)
					_ = response.Body.Close()
					return fmt.Errorf("stream transform error: %w", err)
				}
				// 已经写入过客户端数据 → 停止流式传输，记录部分数据和调试信息
				log.Warnf("stream transform error after data written, stopping stream: %v", err)
				ra.metrics.AddDebugNote("stream_transform_error")
				ra.metrics.AddDebugNote("partial")
				ra.metrics.AddDebugNote(err.Error())
				if ssb := ra.metrics.ClientResponseBody(); len(ssb) > 0 {
					ra.metrics.SetDebugStreamWire(string(ssb))
				}
				_ = response.Body.Close()
				return errStreamTransformAfterWrite
			}
			if len(data) == 0 {
				continue
			}
			if firstToken {
				ra.metrics.SetFirstTokenTime(time.Now())
				firstToken = false
				if firstTokenTimer != nil {
					if !firstTokenTimer.Stop() {
						select {
						case <-firstTokenTimer.C:
						default:
						}
					}
					firstTokenTimer = nil
					firstTokenC = nil
				}
			}

			ra.metrics.AppendClientResponseBody(data)
			_, _ = ra.c.Writer.Write(data)
			ra.c.Writer.Flush()
		}
	}
}

// transformStreamData 转换流式数据
func (ra *relayAttempt) transformStreamData(ctx context.Context, data string, eventType string) ([]byte, error) {
	internalStream, err := ra.outAdapter.TransformStream(ctx, []byte(data))
	if err != nil {
		log.Warnf("failed to transform stream: %v", err)
		return nil, err
	}
	if internalStream == nil {
		return nil, nil
	}

	// 同格式透传：原始数据直接转发给客户端，同时仍将内部响应送入 inbound adapter
	// 以累积 streamChunks 供聚合/日志使用（TransformStream 的输出被丢弃）。
	if model.IsPassthrough(ra.internalRequest, ra.internalRequest.RawAPIFormat) {
		if _, err := ra.inAdapter.TransformStream(ctx, internalStream); err != nil {
			log.Warnf("failed to aggregate passthrough stream: %v", err)
		}

		if internalStream.Object == "[DONE]" {
			return []byte("data: [DONE]\n\n"), nil
		}
		if len(internalStream.RawChunk) > 0 {
			return formatRawSSEEvent(eventType, internalStream.RawChunk), nil
		}
		// 无 RawChunk 的内部事件（如 usage-only chunk）不发送给客户端
		return nil, nil
	}

	inStream, err := ra.inAdapter.TransformStream(ctx, internalStream)
	if err != nil {
		log.Warnf("failed to transform stream: %v", err)
		return nil, err
	}

	return inStream, nil
}

func formatRawSSEEvent(eventType string, data []byte) []byte {
	var b strings.Builder
	if eventType != "" {
		b.WriteString("event: ")
		b.WriteString(eventType)
		b.WriteByte('\n')
	}
	b.WriteString("data: ")
	b.Write(data)
	b.WriteString("\n\n")
	return []byte(b.String())
}

// handleResponse 处理非流式响应
func (ra *relayAttempt) handleResponse(ctx context.Context, response *http.Response) error {
	if model.IsPassthrough(ra.internalRequest, ra.internalRequest.RawAPIFormat) {
		rawBody, err := io.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("failed to read passthrough response body: %w", err)
		}
		ra.metrics.SetClientResponseBody(rawBody)

		response.Body = io.NopCloser(bytes.NewReader(rawBody))
		internalResponse, err := ra.outAdapter.TransformResponse(ctx, response)
		if err != nil {
			log.Warnf("failed to transform passthrough response for metrics: %v", err)
		} else if internalResponse != nil {
			ra.metrics.SetInternalResponse(internalResponse, ra.internalRequest.Model)
		}

		contentType := response.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
		}
		ra.c.Data(http.StatusOK, contentType, rawBody)
		return nil
	}

	// 非透传 conversion 路径：解析时顺带捕获截断后的上游原始响应用于调试。
	debugBody := newDebugBodyBuffer()
	originalBody := response.Body
	response.Body = &teeReadCloser{
		Reader: io.TeeReader(originalBody, debugBody),
		Closer: originalBody,
	}

	internalResponse, err := ra.outAdapter.TransformResponse(ctx, response)
	if err != nil {
		log.Warnf("failed to transform response: %v", err)
		ra.metrics.SetDebugUpstreamResponseValue(debugBody.String(), debugBody.Truncated())
		return fmt.Errorf("failed to transform outbound response: %w", err)
	}

	inResponse, err := ra.inAdapter.TransformResponse(ctx, internalResponse)
	if err != nil {
		log.Warnf("failed to transform response: %v", err)
		return fmt.Errorf("failed to transform inbound response: %w", err)
	}

	ra.metrics.SetDebugUpstreamResponseValue(debugBody.String(), debugBody.Truncated())
	ra.metrics.SetClientResponseBody(inResponse)
	ra.c.Data(http.StatusOK, "application/json", inResponse)
	return nil
}

// collectResponse 收集响应信息，返回是否收集到有效响应
func (ra *relayAttempt) collectResponse() (bool, string) {
	internalResponse, err := ra.inAdapter.GetInternalResponse(context.Background())
	if err != nil {
		return false, fmt.Sprintf("GetInternalResponse error: %v", err)
	}
	if internalResponse == nil {
		return false, "upstream returned empty response body"
	}

	ra.metrics.SetInternalResponse(internalResponse, ra.internalRequest.Model)
	return true, ""
}
