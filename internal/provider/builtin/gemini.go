package builtin

import (
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/transformer/outbound/gemini"
)

type geminiProvider struct{}

var (
	_ provider.Provider           = (*geminiProvider)(nil)
	_ provider.SchemaProvider     = (*geminiProvider)(nil)
	_ provider.AuthProvider       = (*geminiProvider)(nil)
	_ provider.ModelFetchProvider = (*geminiProvider)(nil)
)

func (p *geminiProvider) ID() provider.ProviderID { return "gemini" }
func (p *geminiProvider) DisplayName() string      { return "Gemini" }

func (p *geminiProvider) LegacyType() *outbound.OutboundType {
	t := outbound.OutboundTypeGemini
	return &t
}

func (p *geminiProvider) OutboundFactory() func() model.Outbound {
	return func() model.Outbound { return &gemini.MessagesOutbound{} }
}

func (p *geminiProvider) Capabilities() provider.ProviderCapabilities {
	return provider.ProviderCapabilities{Chat: true, Embedding: false}
}

func (p *geminiProvider) CredentialSchema() provider.CredentialSchema {
	return commonCredentialSchema()
}

func (p *geminiProvider) AuthFlow() provider.AuthFlow {
	return manualAuth
}

func (p *geminiProvider) ModelFetcher() provider.ModelFetcher {
	return fetchGeminiModels
}
