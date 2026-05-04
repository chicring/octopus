package provider

import "context"

// AuthFlow 认证流程接口
type AuthFlow interface {
	Type() AuthType
	Start(ctx context.Context, params AuthParams) (*AuthSession, error)
	Poll(ctx context.Context, session *AuthSession) (*AuthResult, error)
	Callback(ctx context.Context, session *AuthSession, code string) (*AuthResult, error)
}

// AuthParams 认证启动参数
type AuthParams struct {
	ClientID     string            `json:"client_id"`
	ClientSecret string            `json:"client_secret"`
	DeviceCodeURL string           `json:"device_code_url"`
	TokenURL     string            `json:"token_url"`
	AuthorizeURL string            `json:"authorize_url"`
	RedirectURL  string            `json:"redirect_url"`
	Scopes       []string          `json:"scopes"`
	Extra        map[string]string `json:"extra,omitempty"`
}

// AuthSession 认证会话状态
type AuthSession struct {
	DeviceCode      string            `json:"device_code,omitempty"`
	UserCode        string            `json:"user_code,omitempty"`
	VerificationURI string            `json:"verification_uri,omitempty"`
	State           string            `json:"state,omitempty"`
	ExpiresAt       int64             `json:"expires_at,omitempty"`
	Interval        int               `json:"interval,omitempty"`
	Extra           map[string]string `json:"extra,omitempty"`
}

// AuthResult 认证结果
type AuthResult struct {
	AccessToken string            `json:"access_token"`
	TokenType   string            `json:"token_type,omitempty"`
	ExpiresIn   int64             `json:"expires_in,omitempty"`
	RefreshToken string           `json:"refresh_token,omitempty"`
	Scope       string            `json:"scope,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}
