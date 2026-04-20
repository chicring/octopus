package builtin

import (
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/transformer/outbound/openai"
)

type openaiResponseProvider struct{}

var (
	_ provider.Provider           = (*openaiResponseProvider)(nil)
	_ provider.SchemaProvider     = (*openaiResponseProvider)(nil)
	_ provider.AuthProvider       = (*openaiResponseProvider)(nil)
	_ provider.ModelFetchProvider = (*openaiResponseProvider)(nil)
)

func (p *openaiResponseProvider) ID() provider.ProviderID { return "openai-response" }
func (p *openaiResponseProvider) DisplayName() string      { return "OpenAI Response" }

func (p *openaiResponseProvider) LegacyType() *outbound.OutboundType {
	t := outbound.OutboundTypeOpenAIResponse
	return &t
}

func (p *openaiResponseProvider) OutboundFactory() func() model.Outbound {
	return func() model.Outbound { return &openai.ResponseOutbound{} }
}

func (p *openaiResponseProvider) Capabilities() provider.ProviderCapabilities {
	return provider.ProviderCapabilities{Chat: true, Embedding: false}
}

func (p *openaiResponseProvider) CredentialSchema() provider.CredentialSchema {
	return commonCredentialSchema()
}

func (p *openaiResponseProvider) AuthFlow() provider.AuthFlow {
	return manualAuth
}

func (p *openaiResponseProvider) ModelFetcher() provider.ModelFetcher {
	return fetchOpenAIModels
}
