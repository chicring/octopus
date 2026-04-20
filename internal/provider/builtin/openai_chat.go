package builtin

import (
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/transformer/outbound/openai"
)

type openaiChatProvider struct{}

var (
	_ provider.Provider           = (*openaiChatProvider)(nil)
	_ provider.SchemaProvider     = (*openaiChatProvider)(nil)
	_ provider.AuthProvider       = (*openaiChatProvider)(nil)
	_ provider.ModelFetchProvider = (*openaiChatProvider)(nil)
)

func (p *openaiChatProvider) ID() provider.ProviderID { return "openai-chat" }
func (p *openaiChatProvider) DisplayName() string      { return "OpenAI Chat" }

func (p *openaiChatProvider) LegacyType() *outbound.OutboundType {
	t := outbound.OutboundTypeOpenAIChat
	return &t
}

func (p *openaiChatProvider) OutboundFactory() func() model.Outbound {
	return func() model.Outbound { return &openai.ChatOutbound{} }
}

func (p *openaiChatProvider) Capabilities() provider.ProviderCapabilities {
	return provider.ProviderCapabilities{Chat: true, Embedding: false}
}

func (p *openaiChatProvider) CredentialSchema() provider.CredentialSchema {
	return commonCredentialSchema()
}

func (p *openaiChatProvider) AuthFlow() provider.AuthFlow {
	return manualAuth
}

func (p *openaiChatProvider) ModelFetcher() provider.ModelFetcher {
	return fetchOpenAIModels
}
