package builtin

import (
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/provider/auth"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/transformer/outbound/codex"
)

type codexProvider struct{}

var (
	_ provider.Provider       = (*codexProvider)(nil)
	_ provider.SchemaProvider = (*codexProvider)(nil)
	_ provider.AuthProvider   = (*codexProvider)(nil)
)

func (p *codexProvider) ID() provider.ProviderID { return "codex" }
func (p *codexProvider) DisplayName() string      { return "Codex" }

func (p *codexProvider) LegacyType() *outbound.OutboundType {
	// Codex 使用 provider_id 路由，无遗留类型映射
	return nil
}

func (p *codexProvider) OutboundFactory() func() model.Outbound {
	return func() model.Outbound { return &codex.CodexOutbound{} }
}

func (p *codexProvider) Capabilities() provider.ProviderCapabilities {
	return provider.ProviderCapabilities{Chat: true, Embedding: false}
}

func (p *codexProvider) CredentialSchema() provider.CredentialSchema {
	return provider.CredentialSchema{
		AuthType: provider.AuthTypeOAuthWeb,
		Fields: []provider.CredentialField{
			{Key: "base_url", Label: "Base URL", Type: "url", Required: true, Default: auth.CodexDefaultBaseURL, Order: 0},
		},
	}
}

// codexAuth 共享实例
// RedirectURL 使用 Codex CLI 标准回调地址，OpenAI 识别此 redirect_uri
// 用户在浏览器完成登录后，浏览器会跳转到 localhost:1455（页面报错但 URL 包含 code 和 state）
// 用户复制完整 URL 粘贴回 Octopus 管理界面完成授权（手动回调模式）
var codexAuth provider.AuthFlow = &auth.CodexOAuthFlow{
	ClientID:     auth.CodexClientID,
	AuthorizeURL: auth.CodexAuthURL,
	TokenURL:     auth.CodexTokenURL,
	RedirectURL:  "http://localhost:1455/auth/callback",
	Scopes:       []string{"openid", "email", "profile", "offline_access"},
}

func (p *codexProvider) AuthFlow() provider.AuthFlow {
	return codexAuth
}
