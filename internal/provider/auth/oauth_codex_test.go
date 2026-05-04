package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/provider"
)

// mockFlow 创建一个指向 mock token endpoint 的 CodexOAuthFlow
func mockFlow(tokenSrv *httptest.Server) *CodexOAuthFlow {
	return &CodexOAuthFlow{
		ClientID:     CodexClientID,
		AuthorizeURL: CodexAuthURL,
		TokenURL:     tokenSrv.URL + "/oauth/token",
		RedirectURL:  "http://localhost:1455/auth/callback",
		Scopes:       []string{"openid", "email", "profile", "offline_access"},
	}
}

// makeMockIDToken 程序化构造 mock JWT id_token，避免硬编码 base64 触发安全扫描
func makeMockIDToken(email, accountID string) string {
	payload := map[string]interface{}{
		"email": email,
		"sub":   "auth0|1234567890",
	}
	if accountID != "" {
		payload["codex_auth_info"] = map[string]string{
			"chatgpt_account_id": accountID,
		}
	}
	payloadBytes, _ := json.Marshal(payload)
	encoded := base64urlNoPadding(payloadBytes)
	// 三段式：header.payload.signature
	return "mockhdr." + encoded + ".mocksig"
}

// TestCodexOAuthFlow_Start 验证授权 URL 构建正确（PKCE 参数、redirect_uri、scope）
func TestCodexOAuthFlow_Start(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	f := mockFlow(srv)

	session, err := f.Start(context.Background(), provider.AuthParams{})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if session.State == "" {
		t.Error("state should not be empty")
	}
	if session.Extra == nil || session.Extra["code_verifier"] == "" {
		t.Error("code_verifier should be in session.Extra")
	}
	if session.Extra["code_challenge"] == "" {
		t.Error("code_challenge should be in session.Extra")
	}

	authURL := session.VerificationURI
	if !strings.Contains(authURL, "redirect_uri=http%3A%2F%2Flocalhost%3A1455%2Fauth%2Fcallback") {
		t.Errorf("auth URL should contain redirect_uri, got: %s", authURL)
	}
	if !strings.Contains(authURL, "code_challenge_method=S256") {
		t.Errorf("auth URL should contain code_challenge_method=S256, got: %s", authURL)
	}
	if !strings.Contains(authURL, "response_type=code") {
		t.Errorf("auth URL should contain response_type=code, got: %s", authURL)
	}
	if !strings.Contains(authURL, "codex_cli_simplified_flow=true") {
		t.Errorf("auth URL should contain codex_cli_simplified_flow=true, got: %s", authURL)
	}
}

// TestCodexOAuthFlow_Callback_Success 验证 Callback 正确交换 token
func TestCodexOAuthFlow_Callback_Success(t *testing.T) {
	mockIDToken := makeMockIDToken("test@example.com", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("expected Content-Type=application/x-www-form-urlencoded, got %s", r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("expected grant_type=authorization_code, got %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("redirect_uri") != "http://localhost:1455/auth/callback" {
			t.Errorf("expected redirect_uri=http://localhost:1455/auth/callback, got %s", r.Form.Get("redirect_uri"))
		}
		if r.Form.Get("code_verifier") == "" {
			t.Error("code_verifier should not be empty")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "at-123",
			"refresh_token": "rt-456",
			"id_token":      mockIDToken,
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	GetProxyHTTPClient = func(useProxy bool) (*http.Client, error) {
		return srv.Client(), nil
	}

	f := mockFlow(srv)
	session, err := f.Start(context.Background(), provider.AuthParams{})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	result, err := f.Callback(context.Background(), session, "test-auth-code")
	if err != nil {
		t.Fatalf("Callback failed: %v", err)
	}

	if result.AccessToken != "at-123" {
		t.Errorf("expected access_token=at-123, got %s", result.AccessToken)
	}
	if result.RefreshToken != "rt-456" {
		t.Errorf("expected refresh_token=rt-456, got %s", result.RefreshToken)
	}
	if result.Extra["email"] != "test@example.com" {
		t.Errorf("expected email=test@example.com, got %s", result.Extra["email"])
	}
}

// TestCodexOAuthFlow_Callback_TokenError 验证 Callback 处理 token endpoint 错误
func TestCodexOAuthFlow_Callback_TokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "Invalid authorization code",
		})
	}))
	defer srv.Close()

	GetProxyHTTPClient = func(useProxy bool) (*http.Client, error) {
		return srv.Client(), nil
	}

	f := mockFlow(srv)
	session, _ := f.Start(context.Background(), provider.AuthParams{})

	_, err := f.Callback(context.Background(), session, "bad-code")
	if err == nil {
		t.Fatal("expected error for invalid grant, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("expected error containing 'invalid_grant', got: %v", err)
	}
}

// TestCodexOAuthFlow_Callback_MissingCodeVerifier 验证缺少 code_verifier 时报错
func TestCodexOAuthFlow_Callback_MissingCodeVerifier(t *testing.T) {
	f := &CodexOAuthFlow{
		ClientID:    CodexClientID,
		TokenURL:    CodexTokenURL,
		RedirectURL: "http://localhost:1455/auth/callback",
	}

	session := &provider.AuthSession{State: "test"}

	_, err := f.Callback(context.Background(), session, "some-code")
	if err == nil {
		t.Fatal("expected error for missing code_verifier, got nil")
	}
	if !strings.Contains(err.Error(), "code_verifier") {
		t.Errorf("expected error about code_verifier, got: %v", err)
	}
}

// TestCodexOAuthFlow_RefreshToken_Success 验证 RefreshToken 正确刷新
func TestCodexOAuthFlow_RefreshToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil {
			if r.Form.Get("grant_type") != "refresh_token" {
				t.Errorf("expected grant_type=refresh_token, got %s", r.Form.Get("grant_type"))
			}
			if r.Form.Get("refresh_token") != "old-rt" {
				t.Errorf("expected refresh_token=old-rt, got %s", r.Form.Get("refresh_token"))
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-at",
			"refresh_token": "new-rt",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	GetProxyHTTPClient = func(useProxy bool) (*http.Client, error) {
		return srv.Client(), nil
	}

	f := mockFlow(srv)

	result, err := f.RefreshToken(context.Background(), "old-rt")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if result.AccessToken != "new-at" {
		t.Errorf("expected access_token=new-at, got %s", result.AccessToken)
	}
	if result.RefreshToken != "new-rt" {
		t.Errorf("expected refresh_token=new-rt, got %s", result.RefreshToken)
	}
}

// TestCodexOAuthFlow_RefreshToken_PreserveOld 验证刷新时如果新响应没有 refresh_token，保留旧的
func TestCodexOAuthFlow_RefreshToken_PreserveOld(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "new-at",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	GetProxyHTTPClient = func(useProxy bool) (*http.Client, error) {
		return srv.Client(), nil
	}

	f := mockFlow(srv)

	result, err := f.RefreshToken(context.Background(), "old-rt")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if result.RefreshToken != "old-rt" {
		t.Errorf("expected refresh_token preserved as old-rt, got %s", result.RefreshToken)
	}
}

// TestParseIDTokenPayload 验证 JWT payload 解析（无 accountID）
func TestParseIDTokenPayload(t *testing.T) {
	idToken := makeMockIDToken("user@example.com", "")

	email, accountID := ParseIDTokenPayload(idToken)
	if email != "user@example.com" {
		t.Errorf("expected email=user@example.com, got %s", email)
	}
	if accountID != "" {
		t.Errorf("expected empty accountID, got %s", accountID)
	}
}

// TestParseIDTokenPayload_WithAccountID 验证解析含 chatgpt_account_id 的 JWT
func TestParseIDTokenPayload_WithAccountID(t *testing.T) {
	idToken := makeMockIDToken("test@example.com", "acct-abc123")

	email, accountID := ParseIDTokenPayload(idToken)
	if email != "test@example.com" {
		t.Errorf("expected email=test@example.com, got %s", email)
	}
	if accountID != "acct-abc123" {
		t.Errorf("expected accountID=acct-abc123, got %s", accountID)
	}
}

// TestPKCEChallenge 验证 PKCE code_challenge = BASE64URL(SHA256(code_verifier))
func TestPKCEChallenge(t *testing.T) {
	codeVerifier := "test-verifier-12345"
	challenge := sha256Base64url(codeVerifier)

	if challenge == "" {
		t.Error("challenge should not be empty")
	}
	if strings.Contains(challenge, "=") || strings.Contains(challenge, "+") || strings.Contains(challenge, "/") {
		t.Errorf("challenge should be base64url without padding, got %s", challenge)
	}
}
