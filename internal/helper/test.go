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

// TestModels 对渠道中的指定模型进行连通性测试
// 每个模型发送一个最小请求（"1+1=?"，max_tokens=1），30s 超时
func TestModels(ctx context.Context, channel *model.Channel, models []string) []TestModelResult {
	// 优先 provider-based 查找，回退到 legacy
	pid := provider.ResolveProviderIDFromType(channel.Type)
	if channel.ProviderID != "" {
		pid = provider.ProviderID(channel.ProviderID)
	}
	var transformer transformermodel.Outbound
	if pid != "" {
		transformer = provider.GetOutbound(pid)
	}
	if transformer == nil {
		transformer = outbound.Get(channel.Type)
	}
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

	baseUrl := channel.GetBaseUrl()
	if baseUrl == "" {
		results := make([]TestModelResult, 0, len(models))
		for _, m := range models {
			results = append(results, TestModelResult{Model: m, Passed: false, Error: "base url is empty"})
		}
		return results
	}

	isEmbedding := provider.IsEmbeddingProvider(pid) || outbound.IsEmbeddingChannelType(channel.Type)

	results := make([]TestModelResult, 0, len(models))
	for _, modelName := range models {
		if modelName == "" {
			continue
		}
		var key model.ChannelKey
		if channel.ID > 0 {
			key = op.ChannelGetKeyForModel(channel.ID, modelName)
		} else {
			key = channel.GetChannelKeyForModel(modelName)
		}
		if key.ChannelKey == "" {
			results = append(results, TestModelResult{Model: modelName, Passed: false, Error: "no available key for model"})
			continue
		}
		if key.IsCLI && !isCLIProvider(pid) {
			results = append(results, TestModelResult{Model: modelName, Passed: false, Error: "CLI key requires a CLI-capable provider"})
			continue
		}
		results = append(results, testSingleModel(ctx, transformer, httpClient, baseUrl, key.ChannelKey, modelName, channel, isEmbedding))
	}
	return results
}

// TestModelsWithKey 使用指定的 Key 对渠道模型进行连通性测试
func TestModelsWithKey(ctx context.Context, channel *model.Channel, key model.ChannelKey, models []string) []TestModelResult {
	// 优先 provider-based 查找，回退到 legacy
	pid := provider.ResolveProviderIDFromType(channel.Type)
	if channel.ProviderID != "" {
		pid = provider.ProviderID(channel.ProviderID)
	}
	var transformer transformermodel.Outbound
	if pid != "" {
		transformer = provider.GetOutbound(pid)
	}
	if transformer == nil {
		transformer = outbound.Get(channel.Type)
	}
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

	baseUrl := channel.GetBaseUrl()
	if baseUrl == "" || key.ChannelKey == "" {
		results := make([]TestModelResult, 0, len(models))
		for _, m := range models {
			results = append(results, TestModelResult{Model: m, Passed: false, Error: "base url or key is empty"})
		}
		return results
	}

	isEmbedding := provider.IsEmbeddingProvider(pid) || outbound.IsEmbeddingChannelType(channel.Type)

	results := make([]TestModelResult, 0, len(models))
	for _, modelName := range models {
		if modelName == "" {
			continue
		}
		if !key.SupportsModel(modelName) {
			results = append(results, TestModelResult{Model: modelName, Passed: false, Error: "key does not support this model"})
			continue
		}
		if key.IsCLI && !isCLIProvider(pid) {
			results = append(results, TestModelResult{Model: modelName, Passed: false, Error: "CLI key requires a CLI-capable provider"})
			continue
		}
		results = append(results, testSingleModel(ctx, transformer, httpClient, baseUrl, key.ChannelKey, modelName, channel, isEmbedding))
	}
	return results
}

func isCLIProvider(pid provider.ProviderID) bool {
	return pid == provider.ProviderID("codex")
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
