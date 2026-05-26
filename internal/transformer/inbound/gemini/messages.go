/**
 * @Project: Octopus
 * @File: messages.go
 * @Description: Gemini native inbound adapter for model-path requests and Gemini response serialization.
 * @Author: Ying Xinyao
 * @Contact: admin@loserrc.com | QQ: 1129414920
 * @Date: 2026-05-27
 * @Version: v1.0.0
 * @Copyright: (c) 2026 Ying Xinyao. All rights reserved.
 */

package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

type MessagesInbound struct {
	streamChunks   []*model.InternalLLMResponse
	storedResponse *model.InternalLLMResponse
}

func (i *MessagesInbound) Reset() {
	i.streamChunks = nil
	i.storedResponse = nil
}

func (i *MessagesInbound) TransformRequest(ctx context.Context, body []byte) (*model.InternalLLMRequest, error) {
	var geminiReq model.GeminiGenerateContentRequest
	if err := json.Unmarshal(body, &geminiReq); err != nil {
		return nil, err
	}

	internalReq := &model.InternalLLMRequest{
		Messages:            convertGeminiContentsToMessages(&geminiReq),
		TransformerMetadata: map[string]string{},
	}

	if geminiReq.GenerationConfig != nil {
		internalReq.Temperature = geminiReq.GenerationConfig.Temperature
		internalReq.TopP = geminiReq.GenerationConfig.TopP
		if geminiReq.GenerationConfig.TopK != nil {
			topK := int64(*geminiReq.GenerationConfig.TopK)
			internalReq.TopK = &topK
		}
		if geminiReq.GenerationConfig.MaxOutputTokens > 0 {
			maxTokens := int64(geminiReq.GenerationConfig.MaxOutputTokens)
			internalReq.MaxTokens = &maxTokens
		}
		if len(geminiReq.GenerationConfig.StopSequences) > 0 {
			internalReq.Stop = &model.Stop{MultipleStop: geminiReq.GenerationConfig.StopSequences}
		}
		if geminiReq.GenerationConfig.ThinkingConfig != nil {
			if geminiReq.GenerationConfig.ThinkingConfig.ThinkingBudget != nil {
				reasoningBudget := int64(*geminiReq.GenerationConfig.ThinkingConfig.ThinkingBudget)
				internalReq.ReasoningBudget = &reasoningBudget
			}
		}
	}

	return internalReq, nil
}

func (i *MessagesInbound) TransformResponse(ctx context.Context, response *model.InternalLLMResponse) ([]byte, error) {
	i.storedResponse = response
	return i.ConvertResponseToClientFormat(ctx, response)
}

func (i *MessagesInbound) ConvertResponseToClientFormat(ctx context.Context, response *model.InternalLLMResponse) ([]byte, error) {
	geminiResp := convertLLMResponseToGemini(response)
	return json.Marshal(geminiResp)
}

func (i *MessagesInbound) TransformStream(ctx context.Context, stream *model.InternalLLMResponse) ([]byte, error) {
	if stream == nil || stream.Object == "[DONE]" {
		return nil, nil
	}
	i.streamChunks = append(i.streamChunks, stream)
	geminiResp := convertLLMResponseToGemini(stream)
	body, err := json.Marshal(geminiResp)
	if err != nil {
		return nil, err
	}
	return []byte("data: " + string(body) + "\n\n"), nil
}

func (i *MessagesInbound) GetInternalResponse(ctx context.Context) (*model.InternalLLMResponse, error) {
	if i.storedResponse != nil {
		return i.storedResponse, nil
	}
	if len(i.streamChunks) == 0 {
		return nil, nil
	}

	result := &model.InternalLLMResponse{
		Object:  "chat.completion",
		Choices: []model.Choice{},
	}
	choices := make(map[int]*model.Choice, len(i.streamChunks))
	for _, chunk := range i.streamChunks {
		if chunk == nil {
			continue
		}
		if chunk.Model != "" {
			result.Model = chunk.Model
		}
		if chunk.Usage != nil {
			result.Usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			current, ok := choices[choice.Index]
			if !ok {
				current = &model.Choice{Index: choice.Index, Message: &model.Message{Role: "assistant"}}
				choices[choice.Index] = current
			}
			if choice.FinishReason != nil {
				current.FinishReason = choice.FinishReason
			}
			if choice.Delta != nil {
				mergeMessage(current.Message, choice.Delta)
			} else if choice.Message != nil {
				mergeMessage(current.Message, choice.Message)
			}
		}
	}
	for idx := 0; idx < len(choices); idx++ {
		if choice, ok := choices[idx]; ok {
			result.Choices = append(result.Choices, *choice)
		}
	}
	i.streamChunks = nil
	return result, nil
}

func convertGeminiContentsToMessages(geminiReq *model.GeminiGenerateContentRequest) []model.Message {
	messages := make([]model.Message, 0, len(geminiReq.Contents)+1)
	if geminiReq.SystemInstruction != nil {
		if text := geminiPartsText(geminiReq.SystemInstruction.Parts); text != "" {
			messages = append(messages, model.Message{
				Role:    "system",
				Content: model.MessageContent{Content: &text},
			})
		}
	}

	for contentIndex, content := range geminiReq.Contents {
		role := "user"
		if content.Role == "model" {
			role = "assistant"
		}
		msg := model.Message{Role: role}
		parts := make([]model.MessageContentPart, 0, len(content.Parts))
		for partIndex, part := range content.Parts {
			switch {
			case part.Text != "":
				text := part.Text
				parts = append(parts, model.MessageContentPart{Type: "text", Text: &text})
			case part.InlineData != nil:
				parts = append(parts, model.MessageContentPart{
					Type: "image_url",
					ImageURL: &model.ImageURL{
						URL: fmt.Sprintf("data:%s;base64,%s", part.InlineData.MimeType, part.InlineData.Data),
					},
				})
			case part.FileData != nil:
				parts = append(parts, model.MessageContentPart{
					Type: "image_url",
					ImageURL: &model.ImageURL{
						URL: part.FileData.FileURI,
					},
				})
			case part.FunctionCall != nil:
				args, _ := json.Marshal(part.FunctionCall.Args)
				msg.ToolCalls = append(msg.ToolCalls, model.ToolCall{
					Index: partIndex,
					ID:    fmt.Sprintf("call_%d_%d", contentIndex, partIndex),
					Type:  "function",
					Function: model.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(args),
					},
				})
			case part.FunctionResponse != nil:
				msg.Role = "tool"
				msg.ToolCallName = &part.FunctionResponse.Name
				payload, _ := json.Marshal(part.FunctionResponse.Response)
				text := string(payload)
				msg.Content = model.MessageContent{Content: &text}
			}
		}
		if len(parts) == 1 && parts[0].Type == "text" {
			msg.Content = model.MessageContent{Content: parts[0].Text}
		} else if len(parts) > 0 {
			msg.Content = model.MessageContent{MultipleContent: parts}
		}
		messages = append(messages, msg)
	}
	return messages
}

func geminiPartsText(parts []*model.GeminiPart) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Text != "" {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

func convertLLMResponseToGemini(response *model.InternalLLMResponse) *model.GeminiGenerateContentResponse {
	geminiResp := &model.GeminiGenerateContentResponse{}
	if response == nil {
		return geminiResp
	}
	for _, choice := range response.Choices {
		message := choice.Message
		if message == nil {
			message = choice.Delta
		}
		candidate := &model.GeminiCandidate{
			Index:        choice.Index,
			Content:      convertMessageToGeminiContent(message),
			FinishReason: convertFinishReasonToGemini(choice.FinishReason),
		}
		geminiResp.Candidates = append(geminiResp.Candidates, candidate)
	}
	if response.Usage != nil {
		geminiResp.UsageMetadata = &model.GeminiUsageMetadata{
			PromptTokenCount:     int(response.Usage.PromptTokens),
			CandidatesTokenCount: int(response.Usage.CompletionTokens),
			TotalTokenCount:      int(response.Usage.TotalTokens),
		}
		if response.Usage.PromptTokensDetails != nil {
			geminiResp.UsageMetadata.CachedContentTokenCount = int(response.Usage.PromptTokensDetails.CachedTokens)
		}
		if response.Usage.CompletionTokensDetails != nil {
			geminiResp.UsageMetadata.ThoughtsTokenCount = int(response.Usage.CompletionTokensDetails.ReasoningTokens)
		}
	}
	return geminiResp
}

func convertMessageToGeminiContent(message *model.Message) *model.GeminiContent {
	content := &model.GeminiContent{Role: "model", Parts: []*model.GeminiPart{}}
	if message == nil {
		return content
	}
	if message.Role == "user" || message.Role == "tool" {
		content.Role = "user"
	}
	if reasoning := message.GetReasoningContent(); reasoning != "" {
		content.Parts = append(content.Parts, &model.GeminiPart{Text: reasoning, Thought: true})
	}
	if message.Content.Content != nil && *message.Content.Content != "" {
		content.Parts = append(content.Parts, &model.GeminiPart{Text: *message.Content.Content})
	}
	for _, part := range message.Content.MultipleContent {
		if part.Type == "text" && part.Text != nil {
			content.Parts = append(content.Parts, &model.GeminiPart{Text: *part.Text})
		}
	}
	for _, toolCall := range message.ToolCalls {
		var args map[string]interface{}
		if toolCall.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
		}
		content.Parts = append(content.Parts, &model.GeminiPart{
			FunctionCall: &model.GeminiFunctionCall{
				Name: toolCall.Function.Name,
				Args: args,
			},
		})
	}
	return content
}

func convertFinishReasonToGemini(reason *string) *string {
	if reason == nil {
		return nil
	}
	mapping := map[string]string{
		"stop":           "STOP",
		"length":         "MAX_TOKENS",
		"content_filter": "SAFETY",
		"tool_calls":     "STOP",
	}
	if mapped, ok := mapping[*reason]; ok {
		return &mapped
	}
	fallback := "STOP"
	return &fallback
}

func mergeMessage(dst *model.Message, src *model.Message) {
	if dst == nil || src == nil {
		return
	}
	if src.Role != "" {
		dst.Role = src.Role
	}
	if src.Content.Content != nil {
		if dst.Content.Content == nil {
			empty := ""
			dst.Content.Content = &empty
		}
		*dst.Content.Content += *src.Content.Content
	}
	if len(src.Content.MultipleContent) > 0 {
		dst.Content.MultipleContent = append(dst.Content.MultipleContent, src.Content.MultipleContent...)
	}
	if reasoning := src.GetReasoningContent(); reasoning != "" {
		current := dst.GetReasoningContent()
		dst.SetReasoningContent(current + reasoning)
	}
	if len(src.ToolCalls) > 0 {
		dst.ToolCalls = append(dst.ToolCalls, src.ToolCalls...)
	}
}
