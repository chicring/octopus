package provider

import (
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

// ResolveProviderIDFromType 从 legacy OutboundType 解析 ProviderID
func ResolveProviderIDFromType(t outbound.OutboundType) ProviderID {
	providerMu.RLock()
	defer providerMu.RUnlock()
	if id, ok := legacyMap[t]; ok {
		return id
	}
	return ""
}

// IsChatProvider 判断 provider 是否支持 chat
func IsChatProvider(id ProviderID) bool {
	p := Get(id)
	if p == nil {
		return false
	}
	return p.Capabilities().Chat
}

// IsEmbeddingProvider 判断 provider 是否支持 embedding
func IsEmbeddingProvider(id ProviderID) bool {
	p := Get(id)
	if p == nil {
		return false
	}
	return p.Capabilities().Embedding
}

// GetCredentialSchema 获取 provider 的凭证 schema（如果实现了 SchemaProvider）
func GetCredentialSchema(id ProviderID) *CredentialSchema {
	p := Get(id)
	if p == nil {
		return nil
	}
	if sp, ok := p.(SchemaProvider); ok {
		schema := sp.CredentialSchema()
		return &schema
	}
	return nil
}

// GetAuthFlow 获取 provider 的认证流程（如果实现了 AuthProvider）
func GetAuthFlow(id ProviderID) AuthFlow {
	p := Get(id)
	if p == nil {
		return nil
	}
	if ap, ok := p.(AuthProvider); ok {
		return ap.AuthFlow()
	}
	return nil
}

// GetModelFetcher 获取 provider 的模型获取函数（如果实现了 ModelFetchProvider）
func GetModelFetcher(id ProviderID) ModelFetcher {
	p := Get(id)
	if p == nil {
		return nil
	}
	if mfp, ok := p.(ModelFetchProvider); ok {
		return mfp.ModelFetcher()
	}
	return nil
}
