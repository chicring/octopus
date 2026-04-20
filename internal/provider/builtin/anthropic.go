package builtin

import (
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/transformer/outbound/authropic"
)

type anthropicProvider struct{}

var (
	_ provider.Provider           = (*anthropicProvider)(nil)
	_ provider.SchemaProvider     = (*anthropicProvider)(nil)
	_ provider.AuthProvider       = (*anthropicProvider)(nil)
	_ provider.ModelFetchProvider = (*anthropicProvider)(nil)
)

func (p *anthropicProvider) ID() provider.ProviderID { return "anthropic" }
func (p *anthropicProvider) DisplayName() string      { return "Anthropic" }

func (p *anthropicProvider) LegacyType() *outbound.OutboundType {
	t := outbound.OutboundTypeAnthropic
	return &t
}

func (p *anthropicProvider) OutboundFactory() func() model.Outbound {
	return func() model.Outbound { return &authropic.MessageOutbound{} }
}

func (p *anthropicProvider) Capabilities() provider.ProviderCapabilities {
	return provider.ProviderCapabilities{Chat: true, Embedding: false}
}

func (p *anthropicProvider) CredentialSchema() provider.CredentialSchema {
	return commonCredentialSchema()
}

func (p *anthropicProvider) AuthFlow() provider.AuthFlow {
	return manualAuth
}

func (p *anthropicProvider) ModelFetcher() provider.ModelFetcher {
	return fetchAnthropicModels
}
