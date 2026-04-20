package auth

import (
	"context"

	"github.com/bestruirui/octopus/internal/provider"
)

// ManualAuth 手动认证流程（用户直接输入 API Key）
type ManualAuth struct{}

var _ provider.AuthFlow = (*ManualAuth)(nil)

func (m *ManualAuth) Type() provider.AuthType {
	return provider.AuthTypeManual
}

func (m *ManualAuth) Start(_ context.Context, _ provider.AuthParams) (*provider.AuthSession, error) {
	return nil, nil
}

func (m *ManualAuth) Poll(_ context.Context, _ *provider.AuthSession) (*provider.AuthResult, error) {
	return nil, nil
}

func (m *ManualAuth) Callback(_ context.Context, _ *provider.AuthSession, _ string) (*provider.AuthResult, error) {
	return nil, nil
}
