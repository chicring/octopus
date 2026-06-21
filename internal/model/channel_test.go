package model

import (
	"testing"

	transformermodel "github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

func TestSelectBaseUrl_PassthroughPreferred(t *testing.T) {
	// 渠道有两个 URL：OpenAIChat (延迟低) 和 Anthropic (延迟高)
	// 客户端发来 Anthropic 格式请求，应优先选 Anthropic URL（透传）
	ch := &Channel{
		BaseUrls: []BaseUrl{
			{URL: "https://openai.example.com", Delay: 10, Type: outbound.OutboundTypeOpenAIChat, ProviderID: "openai-chat"},
			{URL: "https://anthropic.example.com", Delay: 500, Type: outbound.OutboundTypeAnthropic, ProviderID: "anthropic"},
		},
	}

	url, outType, pid, ok := ch.SelectBaseUrl(transformermodel.APIFormatAnthropicMessage, false)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if url != "https://anthropic.example.com" {
		t.Errorf("passthrough URL = %q, want anthropic URL", url)
	}
	if outType != outbound.OutboundTypeAnthropic {
		t.Errorf("passthrough type = %d, want Anthropic", outType)
	}
	if pid != "anthropic" {
		t.Errorf("passthrough provider_id = %q, want anthropic", pid)
	}
}

func TestSelectBaseUrl_FallbackToLowestDelay(t *testing.T) {
	// 客户端发来 Gemini 格式，但渠道没有 Gemini URL
	// 应回退到延迟最低的兼容 chat URL
	ch := &Channel{
		BaseUrls: []BaseUrl{
			{URL: "https://slow.example.com", Delay: 1000, Type: outbound.OutboundTypeOpenAIChat},
			{URL: "https://fast.example.com", Delay: 5, Type: outbound.OutboundTypeOpenAIChat},
		},
	}

	url, _, _, ok := ch.SelectBaseUrl(transformermodel.APIFormatGeminiContents, false)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if url != "https://fast.example.com" {
		t.Errorf("fallback URL = %q, want fast URL", url)
	}
}

func TestSelectBaseUrl_EmbeddingFiltersChat(t *testing.T) {
	// embedding 请求应只匹配 embedding 类型的 URL
	ch := &Channel{
		BaseUrls: []BaseUrl{
			{URL: "https://chat.example.com", Delay: 10, Type: outbound.OutboundTypeOpenAIChat},
			{URL: "https://embed.example.com", Delay: 50, Type: outbound.OutboundTypeOpenAIEmbedding},
		},
	}

	url, outType, _, ok := ch.SelectBaseUrl("", true)
	if !ok {
		t.Fatal("expected ok=true for embedding")
	}
	if url != "https://embed.example.com" {
		t.Errorf("embedding URL = %q, want embed URL", url)
	}
	if outType != outbound.OutboundTypeOpenAIEmbedding {
		t.Errorf("embedding type = %d, want OpenAIEmbedding", outType)
	}
}

func TestSelectBaseUrl_NoCompatibleUrl(t *testing.T) {
	// chat 请求但渠道只有 embedding URL
	ch := &Channel{
		BaseUrls: []BaseUrl{
			{URL: "https://embed.example.com", Delay: 10, Type: outbound.OutboundTypeOpenAIEmbedding},
		},
	}

	_, _, _, ok := ch.SelectBaseUrl("", false)
	if ok {
		t.Error("expected ok=false when no compatible URL for chat request")
	}
}

func TestSelectBaseUrl_EmptyChannel(t *testing.T) {
	ch := &Channel{}
	_, _, _, ok := ch.SelectBaseUrl("", false)
	if ok {
		t.Error("expected ok=false for empty channel")
	}
}

func TestSelectBaseUrl_SkipsEmptyURL(t *testing.T) {
	ch := &Channel{
		BaseUrls: []BaseUrl{
			{URL: "", Delay: 0, Type: outbound.OutboundTypeOpenAIChat},
			{URL: "https://valid.example.com", Delay: 10, Type: outbound.OutboundTypeOpenAIChat},
		},
	}

	url, _, _, ok := ch.SelectBaseUrl("", false)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if url != "https://valid.example.com" {
		t.Errorf("URL = %q, want valid URL", url)
	}
}

func TestHasProvider(t *testing.T) {
	ch := &Channel{
		BaseUrls: []BaseUrl{
			{URL: "https://a.example.com", Type: outbound.OutboundTypeOpenAIChat, ProviderID: "openai-chat"},
			{URL: "https://b.example.com", Type: outbound.OutboundTypeAnthropic, ProviderID: "anthropic"},
		},
	}

	if !ch.HasProvider("anthropic") {
		t.Error("expected HasProvider(anthropic)=true")
	}
	if !ch.HasProvider("openai-chat") {
		t.Error("expected HasProvider(openai-chat)=true")
	}
	if ch.HasProvider("codex") {
		t.Error("expected HasProvider(codex)=false")
	}
}

func TestGetBaseUrl_ReturnsFirstNonEmpty(t *testing.T) {
	ch := &Channel{
		BaseUrls: []BaseUrl{
			{URL: "", Delay: 0},
			{URL: "https://second.example.com", Delay: 10},
			{URL: "https://third.example.com", Delay: 5},
		},
	}

	if url := ch.GetBaseUrl(); url != "https://second.example.com" {
		t.Errorf("GetBaseUrl() = %q, want first non-empty", url)
	}
}
