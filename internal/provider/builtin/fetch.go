package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/provider"
)

// fetchOpenAIModels 复用 OpenAI /models 获取逻辑
func fetchOpenAIModels(client *http.Client, ctx context.Context, channel model.Channel) ([]string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, channel.GetBaseUrl()+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+channel.GetChannelKey().ChannelKey)
	for _, header := range channel.CustomHeader {
		if header.HeaderKey != "" {
			req.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result model.OpenAIModelList
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

// fetchGeminiModels 复用 Gemini /models 获取逻辑
func fetchGeminiModels(client *http.Client, ctx context.Context, channel model.Channel) ([]string, error) {
	var allModels []string
	pageToken := ""

	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, channel.GetBaseUrl()+"/models", nil)
		req.Header.Set("X-Goog-Api-Key", channel.GetChannelKey().ChannelKey)
		for _, header := range channel.CustomHeader {
			if header.HeaderKey != "" {
				req.Header.Set(header.HeaderKey, header.HeaderValue)
			}
		}
		if pageToken != "" {
			q := req.URL.Query()
			q.Add("pageToken", pageToken)
			req.URL.RawQuery = q.Encode()
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		var result model.GeminiModelList
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}

		for _, m := range result.Models {
			name := strings.TrimPrefix(m.Name, "models/")
			allModels = append(allModels, name)
		}

		if result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}
	if len(allModels) == 0 {
		return fetchOpenAIModels(client, ctx, channel)
	}
	return allModels, nil
}

// fetchAnthropicModels 复用 Anthropic /models 获取逻辑
func fetchAnthropicModels(client *http.Client, ctx context.Context, channel model.Channel) ([]string, error) {
	var allModels []string
	var afterID string

	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, channel.GetBaseUrl()+"/models", nil)
		req.Header.Set("X-Api-Key", channel.GetChannelKey().ChannelKey)
		req.Header.Set("Anthropic-Version", "2023-06-01")
		for _, header := range channel.CustomHeader {
			if header.HeaderKey != "" {
				req.Header.Set(header.HeaderKey, header.HeaderValue)
			}
		}
		q := req.URL.Query()
		if afterID != "" {
			q.Set("after_id", afterID)
		}
		req.URL.RawQuery = q.Encode()

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		var result model.AnthropicModelList
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}

		for _, m := range result.Data {
			allModels = append(allModels, m.ID)
		}

		if !result.HasMore {
			break
		}
		afterID = result.LastID
	}
	if len(allModels) == 0 {
		return fetchOpenAIModels(client, ctx, channel)
	}
	return allModels, nil
}

var (
	_ provider.ModelFetcher = fetchOpenAIModels
	_ provider.ModelFetcher = fetchGeminiModels
	_ provider.ModelFetcher = fetchAnthropicModels
)
