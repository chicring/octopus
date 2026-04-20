package builtin

import (
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/transformer/outbound/openai"
)

type openaiEmbeddingProvider struct{}

var (
	_ provider.Provider           = (*openaiEmbeddingProvider)(nil)
	_ provider.SchemaProvider     = (*openaiEmbeddingProvider)(nil)
	_ provider.AuthProvider       = (*openaiEmbeddingProvider)(nil)
	_ provider.ModelFetchProvider = (*openaiEmbeddingProvider)(nil)
)

func (p *openaiEmbeddingProvider) ID() provider.ProviderID { return "openai-embedding" }
func (p *openaiEmbeddingProvider) DisplayName() string      { return "OpenAI Embedding" }

func (p *openaiEmbeddingProvider) LegacyType() *outbound.OutboundType {
	t := outbound.OutboundTypeOpenAIEmbedding
	return &t
}

func (p *openaiEmbeddingProvider) OutboundFactory() func() model.Outbound {
	return func() model.Outbound { return &openai.EmbeddingOutbound{} }
}

func (p *openaiEmbeddingProvider) Capabilities() provider.ProviderCapabilities {
	return provider.ProviderCapabilities{Chat: false, Embedding: true}
}

func (p *openaiEmbeddingProvider) CredentialSchema() provider.CredentialSchema {
	return commonCredentialSchema()
}

func (p *openaiEmbeddingProvider) AuthFlow() provider.AuthFlow {
	return manualAuth
}

func (p *openaiEmbeddingProvider) ModelFetcher() provider.ModelFetcher {
	return fetchOpenAIModels
}