package provider

// AuthType 认证类型
type AuthType string

const (
	AuthTypeManual     AuthType = "manual"
	AuthTypeOAuthDevice AuthType = "oauth_device"
	AuthTypeOAuthWeb    AuthType = "oauth_web"
)

// CredentialSchema 凭证 schema，描述 provider 需要的凭证字段
type CredentialSchema struct {
	AuthType AuthType          `json:"auth_type"`
	Fields   []CredentialField `json:"fields"`
}

// CredentialField 单个凭证字段定义
type CredentialField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"` // "text" | "password" | "url" | "hidden"
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Secret      bool   `json:"secret"`
	Order       int    `json:"order"`
}
