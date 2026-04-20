package builtin

import (
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/provider/auth"
)

func init() {
	provider.Register(&openaiChatProvider{})
	provider.Register(&openaiResponseProvider{})
	provider.Register(&openaiEmbeddingProvider{})
	provider.Register(&anthropicProvider{})
	provider.Register(&geminiProvider{})
	provider.Register(&volcengineProvider{})
}

// manualAuth 共享实例
var manualAuth provider.AuthFlow = &auth.ManualAuth{}

// commonCredentialSchema 返回标准 API Key 认证的凭证 schema
func commonCredentialSchema() provider.CredentialSchema {
	return provider.CredentialSchema{
		AuthType: provider.AuthTypeManual,
		Fields: []provider.CredentialField{
			{Key: "base_url", Label: "Base URL", Type: "url", Required: true, Order: 0},
			{Key: "api_key", Label: "API Key", Type: "password", Required: true, Secret: true, Order: 1},
		},
	}
}
