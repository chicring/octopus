package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/provider/auth"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound/openai"
)

// CodexOutbound 实现 Codex /responses API 的出站适配器
// Codex API 使用与 OpenAI Responses API 相同的请求/响应格式，
// 但需要额外的认证头（Chatgpt-Account-Id、Originator 等）
type CodexOutbound struct {
	// 内嵌 OpenAI ResponseOutbound 用于复用 TransformResponse/TransformStream
	inner openai.ResponseOutbound
}

// TransformRequest 将内部请求转换为 Codex /responses API 请求
func (o *CodexOutbound) TransformRequest(ctx context.Context, request *model.InternalLLMRequest, baseUrl, key string) (*http.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("request is nil")
	}

	// 解析 Codex 凭证（token 刷新由 relay/op 层负责，这里只解析和设置 header）
	cred, err := auth.ParseCodexCredential(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse codex credential: %w", err)
	}

	// 复用 OpenAI Responses API 的请求转换逻辑
	responsesReq := openai.ConvertToResponsesRequest(request)

	// Codex API 要求 instructions 字段非空，若为空则设置默认值
	if responsesReq.Instructions == "" {
		responsesReq.Instructions = "You are a helpful assistant."
	}

	// Codex API 要求 input 必须是数组格式，不能是字符串
	// 当 ConvertToResponsesRequest 将简单 user 消息优化为字符串时，
	// 需要转换回数组格式
	if responsesReq.Input.Text != nil {
		text := *responsesReq.Input.Text
		responsesReq.Input = openai.ResponsesInput{
			Items: []openai.ResponsesItem{
				{
					Role: "user",
					Content: &openai.ResponsesInput{
						Items: []openai.ResponsesItem{
							{
								Type: "input_text",
								Text: &text,
							},
						},
					},
				},
			},
		}
	}

	// Codex API 要求 store 必须为 false
	storeFalse := false
	responsesReq.Store = &storeFalse

	// Codex API 要求 stream 必须为 true（只支持流式）
	// 同时强制修改内部请求的 Stream 标志，确保 relay 走流式处理路径
	streamTrue := true
	responsesReq.Stream = &streamTrue
	request.Stream = &streamTrue

	body, err := json.Marshal(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal codex request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置 Codex 特有请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("Originator", "codex-tui")
	req.Header.Set("User-Agent", "codex-tui/0.118.0")

	if cred.AccountID != "" {
		req.Header.Set("Chatgpt-Account-Id", cred.AccountID)
	}

	// Codex API 只支持流式，Accept 头始终为 text/event-stream
	req.Header.Set("Accept", "text/event-stream")

	// 构建 URL：baseUrl + /responses
	parsedUrl, err := url.Parse(strings.TrimSuffix(baseUrl, "/"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse base url: %w", err)
	}
	parsedUrl.Path = parsedUrl.Path + "/responses"
	req.URL = parsedUrl
	req.Method = http.MethodPost

	return req, nil
}

// TransformResponse 将 Codex 响应转换为内部通用响应格式
// 委托给 OpenAI ResponseOutbound 处理，因为响应格式相同
func (o *CodexOutbound) TransformResponse(ctx context.Context, response *http.Response) (*model.InternalLLMResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("response is nil")
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("response body is empty")
	}

	// 检查错误响应
	if response.StatusCode >= 400 {
		var errResp struct {
			Error model.ErrorDetail `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, &model.ResponseError{
				StatusCode: response.StatusCode,
				Detail:     errResp.Error,
			}
		}
		return nil, fmt.Errorf("HTTP error %d: %s", response.StatusCode, string(body))
	}

	var resp openai.ResponsesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal codex response: %w", err)
	}

	return convertToLLMResponseFromResponses(&resp), nil
}

// TransformStream 将 Codex 流式事件转换为内部通用流式响应格式
// 委托给 OpenAI ResponseOutbound 处理，因为流式格式相同
func (o *CodexOutbound) TransformStream(ctx context.Context, eventData []byte) (*model.InternalLLMResponse, error) {
	return o.inner.TransformStream(ctx, eventData)
}

// convertToLLMResponseFromResponses 将 OpenAI Responses API 响应转为内部格式
// 复用 openai 包中的转换逻辑
func convertToLLMResponseFromResponses(resp *openai.ResponsesResponse) *model.InternalLLMResponse {
	if resp == nil {
		return &model.InternalLLMResponse{
			Object: "chat.completion",
		}
	}

	result := &model.InternalLLMResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Model:   resp.Model,
		Created: resp.CreatedAt,
	}

	var (
		contentParts     []model.MessageContentPart
		textContent      strings.Builder
		reasoningContent strings.Builder
		toolCalls        []model.ToolCall
	)

	for _, outputItem := range resp.Output {
		switch outputItem.Type {
		case "message":
			if outputItem.Content != nil {
				for _, item := range outputItem.Content.Items {
					if item.Type == "output_text" && item.Text != nil {
						textContent.WriteString(*item.Text)
					}
				}
			}
		case "output_text":
			if outputItem.Text != nil {
				textContent.WriteString(*outputItem.Text)
			}
		case "function_call":
			toolCalls = append(toolCalls, model.ToolCall{
				ID:   outputItem.CallID,
				Type: "function",
				Function: model.FunctionCall{
					Name:      outputItem.Name,
					Arguments: outputItem.Arguments,
				},
			})
		case "reasoning":
			for _, summary := range outputItem.Summary {
				reasoningContent.WriteString(summary.Text)
			}
		}
	}

	choice := model.Choice{
		Index: 0,
		Message: &model.Message{
			Role:      "assistant",
			ToolCalls: toolCalls,
		},
	}

	if reasoningContent.Len() > 0 {
		choice.Message.ReasoningContent = loToPtr(reasoningContent.String())
	}

	if textContent.Len() > 0 {
		if len(contentParts) > 0 {
			textPart := model.MessageContentPart{
				Type: "text",
				Text: loToPtr(textContent.String()),
			}
			contentParts = append([]model.MessageContentPart{textPart}, contentParts...)
			choice.Message.Content = model.MessageContent{
				MultipleContent: contentParts,
			}
		} else {
			choice.Message.Content = model.MessageContent{
				Content: loToPtr(textContent.String()),
			}
		}
	} else if len(contentParts) > 0 {
		choice.Message.Content = model.MessageContent{
			MultipleContent: contentParts,
		}
	}

	// 设置 finish reason
	if len(toolCalls) > 0 {
		choice.FinishReason = loToPtr("tool_calls")
	} else if resp.Status != nil {
		switch *resp.Status {
		case "completed":
			choice.FinishReason = loToPtr("stop")
		case "failed":
			choice.FinishReason = loToPtr("error")
		case "incomplete":
			choice.FinishReason = loToPtr("length")
		}
	}

	result.Choices = []model.Choice{choice}
	result.Usage = convertResponsesUsage(resp.Usage)

	return result
}

func convertResponsesUsage(usage *openai.ResponsesUsage) *model.Usage {
	if usage == nil {
		return nil
	}

	result := &model.Usage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.TotalTokens,
	}

	if usage.InputTokenDetails.CachedTokens > 0 {
		result.PromptTokensDetails = &model.PromptTokensDetails{
			CachedTokens: usage.InputTokenDetails.CachedTokens,
		}
	}

	if usage.OutputTokenDetails.ReasoningTokens > 0 {
		result.CompletionTokensDetails = &model.CompletionTokensDetails{
			ReasoningTokens: usage.OutputTokenDetails.ReasoningTokens,
		}
	}

	return result
}

func loToPtr[T any](v T) *T { return &v }

// 确保编译时检查时间包引用
var _ = time.RFC3339
