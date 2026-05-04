package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/provider"
)

// Codex OAuth 常量
const (
	CodexAuthURL        = "https://auth.openai.com/oauth/authorize"
	CodexTokenURL       = "https://auth.openai.com/oauth/token"
	CodexClientID       = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexDefaultBaseURL = "https://chatgpt.com/backend-api/codex"
)

// CodexOAuthFlow 实现 Codex 的 OAuth 2.0 Authorization Code + PKCE 流程
type CodexOAuthFlow struct {
	ClientID     string
	AuthorizeURL string
	TokenURL     string
	RedirectURL  string
	Scopes       []string
}

var _ provider.AuthFlow = (*CodexOAuthFlow)(nil)

func (f *CodexOAuthFlow) Type() provider.AuthType {
	return provider.AuthTypeOAuthWeb
}

// Start 启动 Codex OAuth 授权流程，生成 PKCE code_verifier/code_challenge
func (f *CodexOAuthFlow) Start(ctx context.Context, params provider.AuthParams) (*provider.AuthSession, error) {
	clientID := params.ClientID
	if clientID == "" {
		clientID = f.ClientID
	}
	authorizeURL := params.AuthorizeURL
	if authorizeURL == "" {
		authorizeURL = f.AuthorizeURL
	}
	redirectURL := params.RedirectURL
	if redirectURL == "" {
		redirectURL = f.RedirectURL
	}

	// 生成 state 防止 CSRF
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	// 生成 PKCE code_verifier（96 字节随机数，base64url 无填充）
	codeVerifierBytes := make([]byte, 96)
	if _, err := rand.Read(codeVerifierBytes); err != nil {
		return nil, fmt.Errorf("failed to generate code_verifier: %w", err)
	}
	codeVerifier := base64urlNoPadding(codeVerifierBytes)

	// 生成 code_challenge = BASE64URL(SHA256(code_verifier))
	codeChallenge := sha256Base64url(codeVerifier)

	scopes := params.Scopes
	if len(scopes) == 0 {
		scopes = f.Scopes
	}

	// 构建 Codex 授权 URL
	authURL, err := url.Parse(authorizeURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse authorize URL: %w", err)
	}
	q := authURL.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURL)
	q.Set("state", state)
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	// Codex 特有参数
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	authURL.RawQuery = q.Encode()

	return &provider.AuthSession{
		VerificationURI: authURL.String(),
		State:           state,
		Extra: map[string]string{
			"code_verifier":  codeVerifier,
			"code_challenge": codeChallenge,
		},
	}, nil
}

// Poll Web 流程通过 callback 完成授权，poll 不适用
func (f *CodexOAuthFlow) Poll(_ context.Context, _ *provider.AuthSession) (*provider.AuthResult, error) {
	return nil, nil
}

// Callback 处理 Codex OAuth 回调，用 authorization_code + code_verifier 换取 token
func (f *CodexOAuthFlow) Callback(ctx context.Context, session *provider.AuthSession, code string) (*provider.AuthResult, error) {
	if session.Extra == nil {
		return nil, fmt.Errorf("missing PKCE code_verifier in session")
	}
	codeVerifier, ok := session.Extra["code_verifier"]
	if !ok {
		return nil, fmt.Errorf("missing PKCE code_verifier in session")
	}

	tokenURL := f.TokenURL

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", f.ClientID)
	form.Set("redirect_uri", f.RedirectURL)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	var tokResp codexTokenResponse
	if err := json.Unmarshal(body, &tokResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokResp.Error != "" {
		return nil, fmt.Errorf("token error: %s: %s", tokResp.Error, tokResp.ErrorDesc)
	}

	// 解析 id_token JWT payload 提取 email 和 chatgpt_account_id
	extra := map[string]string{
		"id_token": tokResp.IDToken,
	}
	if tokResp.IDToken != "" {
		email, accountID := ParseIDTokenPayload(tokResp.IDToken)
		if email != "" {
			extra["email"] = email
		}
		if accountID != "" {
			extra["account_id"] = accountID
		}
	}
	if tokResp.ExpiresIn > 0 {
		extra["expires_at"] = time.Now().Add(time.Duration(tokResp.ExpiresIn) * time.Second).Format(time.RFC3339)
	}

	return &provider.AuthResult{
		AccessToken:  tokResp.AccessToken,
		TokenType:    tokResp.TokenType,
		ExpiresIn:    tokResp.ExpiresIn,
		RefreshToken: tokResp.RefreshToken,
		Scope:        tokResp.Scope,
		Extra:        extra,
	}, nil
}

// RefreshToken 使用 refresh_token 刷新 access_token
func (f *CodexOAuthFlow) RefreshToken(ctx context.Context, refreshToken string) (*provider.AuthResult, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", f.ClientID)
	form.Set("scope", "openid profile email")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	var tokResp codexTokenResponse
	if err := json.Unmarshal(body, &tokResp); err != nil {
		return nil, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	if tokResp.Error != "" {
		return nil, fmt.Errorf("refresh error: %s: %s", tokResp.Error, tokResp.ErrorDesc)
	}

	extra := map[string]string{
		"id_token": tokResp.IDToken,
	}
	if tokResp.IDToken != "" {
		email, accountID := ParseIDTokenPayload(tokResp.IDToken)
		if email != "" {
			extra["email"] = email
		}
		if accountID != "" {
			extra["account_id"] = accountID
		}
	}
	if tokResp.ExpiresIn > 0 {
		extra["expires_at"] = time.Now().Add(time.Duration(tokResp.ExpiresIn) * time.Second).Format(time.RFC3339)
	}

	return &provider.AuthResult{
		AccessToken:  tokResp.AccessToken,
		TokenType:    tokResp.TokenType,
		ExpiresIn:    tokResp.ExpiresIn,
		RefreshToken: tokResp.RefreshToken,
		Scope:        tokResp.Scope,
		Extra:        extra,
	}, nil
}

// codexTokenResponse Codex token 端点响应
type codexTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// ParseIDTokenPayload 解析 JWT id_token 的 payload 部分（不做签名验证）
// 提取 email 和 codex_auth_info.chatgpt_account_id
func ParseIDTokenPayload(idToken string) (email, accountID string) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return
	}
	payload, err := base64urlDecode(parts[1])
	if err != nil {
		return
	}

	var claims struct {
		Email string `json:"email"`
		CodexAuthInfo struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
		} `json:"codex_auth_info"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return
	}

	email = claims.Email
	accountID = claims.CodexAuthInfo.ChatGPTAccountID
	return
}

// base64urlNoPadding 返回 base64url 编码（无填充）
func base64urlNoPadding(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// sha256Base64url 计算 SHA256 哈希并返回 base64url 编码（无填充）
func sha256Base64url(data string) string {
	h := sha256.Sum256([]byte(data))
	return base64urlNoPadding(h[:])
}

// base64urlDecode 解码 base64url（自动补填充）
func base64urlDecode(s string) ([]byte, error) {
	// 补齐填充
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// CodexCredential Codex 凭证结构，存储在 ChannelKey.ChannelKey 字段
type CodexCredential struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	Email        string `json:"email,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
}

// IsExpired 检查凭证是否已过期（5 分钟内视为即将过期）
func (c *CodexCredential) IsExpired() bool {
	if c.ExpiresAt == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, c.ExpiresAt)
	if err != nil {
		return false
	}
	return time.Now().Add(5*time.Minute).After(expiresAt)
}

// ParseCodexCredential 从 key 字符串解析 Codex 凭证
// 如果 key 不是 JSON 格式（不以 '{' 开头），则视为纯 access_token
func ParseCodexCredential(key string) (*CodexCredential, error) {
	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, "{") {
		// 非 JSON 格式，视为纯 access_token
		return &CodexCredential{
			AccessToken: key,
			TokenType:   "Bearer",
		}, nil
	}

	var cred CodexCredential
	if err := json.Unmarshal([]byte(key), &cred); err != nil {
		return nil, fmt.Errorf("failed to parse codex credential: %w", err)
	}
	return &cred, nil
}

// String 将凭证序列化为 JSON 字符串
func (c *CodexCredential) String() string {
	data, _ := json.Marshal(c)
	return string(data)
}

// BuildCodexCredentialFromAuthResult 从认证结果构建 Codex 凭证
func BuildCodexCredentialFromAuthResult(result *provider.AuthResult) *CodexCredential {
	cred := &CodexCredential{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    result.TokenType,
	}
	if result.Extra != nil {
		cred.IDToken = result.Extra["id_token"]
		cred.ExpiresAt = result.Extra["expires_at"]
		cred.AccountID = result.Extra["account_id"]
		cred.Email = result.Extra["email"]
	}
	// 兼容：如果 ExpiresAt 为空但 ExpiresIn 有值，则计算
	if cred.ExpiresAt == "" && result.ExpiresIn > 0 {
		cred.ExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	return cred
}

// Ensure CodexOAuthFlow implements RefreshProvider
var _ RefreshProvider = (*CodexOAuthFlow)(nil)

// RefreshProvider 可选接口：支持 token 刷新
type RefreshProvider interface {
	RefreshToken(ctx context.Context, refreshToken string) (*provider.AuthResult, error)
}

// ParseExpiresAt 解析 expires_at 字符串为时间
func ParseExpiresAt(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty expires_at")
	}
	// 尝试 RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// 尝试 Unix 时间戳
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(ts, 0), nil
	}
	return time.Time{}, fmt.Errorf("invalid expires_at format: %s", s)
}
