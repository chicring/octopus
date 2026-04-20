package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/bestruirui/octopus/internal/provider"
)

// OAuthWebFlow 实现 OAuth 2.0 Authorization Code Grant
type OAuthWebFlow struct {
	ClientID     string
	AuthorizeURL string
	TokenURL     string
	RedirectURL  string
	Scopes       []string
}

var _ provider.AuthFlow = (*OAuthWebFlow)(nil)

func (f *OAuthWebFlow) Type() provider.AuthType {
	return provider.AuthTypeOAuthWeb
}

func (f *OAuthWebFlow) Start(ctx context.Context, params provider.AuthParams) (*provider.AuthSession, error) {
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

	scopes := params.Scopes
	if len(scopes) == 0 {
		scopes = f.Scopes
	}

	authURL := fmt.Sprintf("%s?response_type=code&client_id=%s&redirect_uri=%s&state=%s&scope=%s",
		authorizeURL, clientID, redirectURL, state, joinScopes(scopes))

	return &provider.AuthSession{
		VerificationURI: authURL,
		State:           state,
	}, nil
}

func (f *OAuthWebFlow) Poll(_ context.Context, _ *provider.AuthSession) (*provider.AuthResult, error) {
	// Web flow 通过 callback 完成授权，poll 不适用
	return nil, nil
}

func (f *OAuthWebFlow) Callback(ctx context.Context, session *provider.AuthSession, code string) (*provider.AuthResult, error) {
	tokenURL := f.TokenURL

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", f.ClientID)
	form.Set("redirect_uri", f.RedirectURL)

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

	var tokResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokResp.Error != "" {
		return nil, fmt.Errorf("token error: %s: %s", tokResp.Error, tokResp.ErrorDesc)
	}

	return &provider.AuthResult{
		AccessToken:  tokResp.AccessToken,
		TokenType:    tokResp.TokenType,
		ExpiresIn:    tokResp.ExpiresIn,
		RefreshToken: tokResp.RefreshToken,
		Scope:        tokResp.Scope,
	}, nil
}
