package helper

import (
	"context"
	"net/http"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/provider"
	transformermodel "github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

type TestModelResult struct {
	Model  string `json:"model"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
	Delay  int64  `json:"delay,omitempty"` // 毫秒
}

// resolveFirstBaseUrl 返回渠道中第一个非空 BaseUrl 及其类型信息。
// 用于模型获取/连通性测试场景：仅使用第一个 BaseUrl。
// 返回 (url, type, providerID, ok)。
func resolveFirstBaseUrl(channel *model.Channel) (string, outbound.OutboundType, string, bool) {
	if channel == nil {
		return "", 0, "", false
	}
	for i := range channel.BaseUrls {
		bu := &channel.BaseUrls[i]
		if bu.URL != "" {
			return bu.URL, bu.Type, bu.ProviderID, true
		}
	}
	return "", 0, "", false
}

// resolveOutbound 根据 type/providerID 解析出站适配器。
func resolveOutbound(outType outbound.OutboundType, providerID string) (transformermodel.Outbound, provider.ProviderID) {
	pid := provider.ResolveProviderIDFromType(outType)
	if providerID != "" {
		pid = provider.ProviderID(providerID)
	}
	var transformer transformermodel.Outbound
	if pid != "" {
		transformer = provider.GetOutbound(pid)
	}
	if transformer == nil {
		transformer = outbound.Get(outType)
	}
	return transformer, pid
}

// TestModels 对渠道中的指定模型进行连通性测试
// 每个模型发送一个最小请求（"1+1=?"，max_tokens=1），30s 超时
// 使用第一个 BaseUrl 及其类型。
func TestModels(ctx context.Context, channel *model.Channel, models []string) []TestModelResult {
	baseUrl, outType, providerID, ok := resolveFirstBaseUrl(channel)
	if !ok {
		results := make([]TestModelResult, 0, len(models))
		for _, m := range models {
			results = append(results, TestModelResult{Model: m, Passed: false, Error: "no base url"})
		}
		return results
	}

	transformer, pid := resolveOutbound(outType, providerID)
	if transformer == nil {
		results := make([]TestModelResult, 0, len(models))
		for _, m := range models {
			results = append(results, TestModelResult{Model: m, Passed: false, Error: "unsupported channel type"})
		}
		return results
	}

	httpClient, err := ChannelHttpClient(channel)
	if err != nil {
		results := make([]TestModelResult, 0, len(models))
		for _, m := range models {
			results = append(results, TestModelResult{Model: m, Passed: false, Error: "failed to create http client: " + err.Error()})
		}
		return results
	}

	var keyStr string
	if channel.ID > 0 {
		// 已保存的渠道使用轮询策略选 Key
		k := op.ChannelGetKey(channel.ID)
		keyStr = k.ChannelKey
	} else {
		k := channel.GetChannelKey()
		keyStr = k.ChannelKey
	}

	if baseUrl == "" || keyStr == "" {
		results := make([]TestModelResult, 0, len(models))
		for _, m := range models {
			results = append(results, TestModelResult{Model: m, Passed: false, Error: "base url or key is empty"})
		}
		return results
	}

	isEmbedding := provider.IsEmbeddingProvider(pid) || outbound.IsEmbeddingChannelType(outType)

	results := make([]TestModelResult, 0, len(models))
	for _, modelName := range models {
		if modelName == "" {
			continue
		}
		results = append(results, testSingleModel(ctx, transformer, httpClient, baseUrl, keyStr, modelName, channel, isEmbedding))
	}
	return results
}

// TestModelsWithKey 使用指定的 Key 对渠道模型进行连通性测试
// 使用第一个 BaseUrl 及其类型。
func TestModelsWithKey(ctx context.Context, channel *model.Channel, key string, models []string) []TestModelResult {
	baseUrl, outType, providerID, ok := resolveFirstBaseUrl(channel)
	if !ok {
		results := make([]TestModelResult, 0, len(models))
		for _, m := range models {
			results = append(results, TestModelResult{Model: m, Passed: false, Error: "no base url"})
		}
		return results
	}

	transformer, pid := resolveOutbound(outType, providerID)
	if transformer == nil {
		results := make([]TestModelResult, 0, len(models))
		for _, m := range models {
			results = append(results, TestModelResult{Model: m, Passed: false, Error: "unsupported channel type"})
		}
		return results
	}

	httpClient, err := ChannelHttpClient(channel)
	if err != nil {
		results := make([]TestModelResult, 0, len(models))
		for _, m := range models {
			results = append(results, TestModelResult{Model: m, Passed: false, Error: "failed to create http client: " + err.Error()})
		}
		return results
	}

	if baseUrl == "" || key == "" {
		results := make([]TestModelResult, 0, len(models))
		for _, m := range models {
			results = append(results, TestModelResult{Model: m, Passed: false, Error: "base url or key is empty"})
		}
		return results
	}

	isEmbedding := provider.IsEmbeddingProvider(pid) || outbound.IsEmbeddingChannelType(outType)

	results := make([]TestModelResult, 0, len(models))
	for _, modelName := range models {
		if modelName == "" {
			continue
		}
		results = append(results, testSingleModel(ctx, transformer, httpClient, baseUrl, key, modelName, channel, isEmbedding))
	}
	return results
}

func testSingleModel(
	ctx context.Context,
	transformer transformermodel.Outbound,
	httpClient *http.Client,
	baseUrl string,
	key string,
	modelName string,
	channel *model.Channel,
	isEmbedding bool,
) TestModelResult {
	var req *transformermodel.InternalLLMRequest

	if isEmbedding {
		req = &transformermodel.InternalLLMRequest{
			Model: modelName,
			EmbeddingInput: &transformermodel.EmbeddingInput{
				Single: ptrStr("test"),
			},
		}
	} else {
		falseVal := false
		one := int64(1)
		req = &transformermodel.InternalLLMRequest{
			Model: modelName,
			Messages: []transformermodel.Message{
				{
					Role: "user",
					Content: transformermodel.MessageContent{
						Content: ptrStr("1+1=?"),
					},
				},
			},
			MaxTokens: &one,
			Stream:    &falseVal,
		}
	}

	start := time.Now()

	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	httpReq, err := transformer.TransformRequest(testCtx, req, baseUrl, key)
	if err != nil {
		return TestModelResult{
			Model:  modelName,
			Passed: false,
			Error:  "build request failed: " + err.Error(),
			Delay:  time.Since(start).Milliseconds(),
		}
	}

	// Apply custom headers
	if len(channel.CustomHeader) > 0 {
		for _, header := range channel.CustomHeader {
			httpReq.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}

	resp, err := httpClient.Do(httpReq.WithContext(testCtx))
	delay := time.Since(start).Milliseconds()

	if err != nil {
		return TestModelResult{
			Model:  modelName,
			Passed: false,
			Error:  err.Error(),
			Delay:  delay,
		}
	}
	defer resp.Body.Close()

	result := TestModelResult{
		Model: modelName,
		Delay: delay,
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Passed = true
	} else if resp.StatusCode == http.StatusTooManyRequests {
		result.Passed = true
		result.Error = "Rate limited (429), but channel is reachable"
	} else {
		result.Passed = false
		result.Error = resp.Status
	}

	return result
}

func ptrStr(s string) *string {
	return &s
}
