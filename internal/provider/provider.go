package provider

import (
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

// ProviderID 唯一标识一个 provider
type ProviderID string

// Provider 是所有 provider 必须实现的核心接口
type Provider interface {
	ID() ProviderID
	DisplayName() string
	LegacyType() *outbound.OutboundType // nil 表示新 provider，无遗留类型映射
	OutboundFactory() func() model.Outbound
	Capabilities() ProviderCapabilities
}

// ProviderCapabilities 描述 provider 支持的功能
type ProviderCapabilities struct {
	Chat     bool `json:"chat"`
	Embedding bool `json:"embedding"`
}

// SchemaProvider 可选接口：提供凭证 schema
type SchemaProvider interface {
	CredentialSchema() CredentialSchema
}

// AuthProvider 可选接口：提供认证流程
type AuthProvider interface {
	AuthFlow() AuthFlow // nil = manual
}

// ModelFetchProvider 可选接口：提供模型获取能力
type ModelFetchProvider interface {
	ModelFetcher() ModelFetcher // nil = default OpenAI fetch
}
