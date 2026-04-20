package builtin

import (
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/transformer/outbound/volcengine"
)

type volcengineProvider struct{}

var (
	_ provider.Provider           = (*volcengineProvider)(nil)
	_ provider.SchemaProvider     = (*volcengineProvider)(nil)
	_ provider.AuthProvider       = (*volcengineProvider)(nil)
	_ provider.ModelFetchProvider = (*volcengineProvider)(nil)
)

func (p *volcengineProvider) ID() provider.ProviderID { return "volcengine" }
func (p *volcengineProvider) DisplayName() string      { return "Volcengine" }

func (p *volcengineProvider) LegacyType() *outbound.OutboundType {
	t := outbound.OutboundTypeVolcengine
	return &t
}

func (p *volcengineProvider) OutboundFactory() func() model.Outbound {
	return func() model.Outbound { return &volcengine.ResponseOutbound{} }
}

func (p *volcengineProvider) Capabilities() provider.ProviderCapabilities {
	return provider.ProviderCapabilities{Chat: true, Embedding: false}
}

func (p *volcengineProvider) CredentialSchema() provider.CredentialSchema {
	return commonCredentialSchema()
}

func (p *volcengineProvider) AuthFlow() provider.AuthFlow {
	return manualAuth
}

func (p *volcengineProvider) ModelFetcher() provider.ModelFetcher {
	return fetchOpenAIModels
}
