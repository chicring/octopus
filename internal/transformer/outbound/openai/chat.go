package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

type ChatOutbound struct{}

func (o *ChatOutbound) TransformRequest(ctx context.Context, request *model.InternalLLMRequest, baseUrl, key string) (*http.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("request is nil")
	}

	var body []byte

	// 当入站格式也是 Chat API 时，直接透传原始请求 body，
	// 避免 round-trip 转换破坏上游 prompt cache 的前缀匹配。
	if model.ShouldPassthrough(request, model.APIFormatOpenAIChatCompletion) {
		model.MarkPassthrough(request, model.APIFormatOpenAIChatCompletion)
		body = model.PatchRawRequest(request.RawRequest, request)
	} else {
		// 不同格式间转换，走完整转换
		request.ClearHelpFields()
		model.NormalizeReasoningContentReplay(request)

		// Convert developer role to system role for compatibility
		for i := range request.Messages {
			if request.Messages[i].Role == "developer" {
				request.Messages[i].Role = "system"
			}
		}

		if request.Stream != nil && *request.Stream {
			if request.StreamOptions == nil {
				request.StreamOptions = &model.StreamOptions{IncludeUsage: true}
			} else if !request.StreamOptions.IncludeUsage {
				request.StreamOptions.IncludeUsage = true
			}
		}

		var err error
		body, err = json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
	}

	request.UpstreamRequestBody = body

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	parsedUrl, err := url.Parse(strings.TrimSuffix(baseUrl, "/"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse base url: %w", err)
	}
	parsedUrl.Path = parsedUrl.Path + "/chat/completions"
	req.URL = parsedUrl
	req.Method = http.MethodPost
	return req, nil
}

func (o *ChatOutbound) TransformResponse(ctx context.Context, response *http.Response) (*model.InternalLLMResponse, error) {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("response body is empty")
	}

	var resp model.InternalLLMResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &resp, nil
}

func (o *ChatOutbound) TransformStream(ctx context.Context, eventData []byte) (*model.InternalLLMResponse, error) {
	if bytes.HasPrefix(eventData, []byte("[DONE]")) {
		return &model.InternalLLMResponse{
			Object: "[DONE]",
		}, nil
	}

	var errCheck struct {
		Error *model.ErrorDetail `json:"error"`
	}
	if err := json.Unmarshal(eventData, &errCheck); err == nil && errCheck.Error != nil {
		return nil, &model.ResponseError{
			Detail: *errCheck.Error,
		}
	}

	var resp model.InternalLLMResponse
	if err := json.Unmarshal(eventData, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stream chunk: %w", err)
	}
	resp.RawChunk = append([]byte(nil), eventData...)
	return &resp, nil
}
