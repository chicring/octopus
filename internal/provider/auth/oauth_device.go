package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/provider"
)

// OAuthDeviceFlow 实现 RFC 8628 Device Authorization Grant
type OAuthDeviceFlow struct {
	ClientID      string
	DeviceCodeURL string
	TokenURL      string
	Scopes        []string
}

var _ provider.AuthFlow = (*OAuthDeviceFlow)(nil)

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func (f *OAuthDeviceFlow) Type() provider.AuthType {
	return provider.AuthTypeOAuthDevice
}

func (f *OAuthDeviceFlow) Start(ctx context.Context, params provider.AuthParams) (*provider.AuthSession, error) {
	clientID := params.ClientID
	if clientID == "" {
		clientID = f.ClientID
	}
	deviceCodeURL := params.DeviceCodeURL
	if deviceCodeURL == "" {
		deviceCodeURL = f.DeviceCodeURL
	}
	scopes := params.Scopes
	if len(scopes) == 0 {
		scopes = f.Scopes
	}

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", joinScopes(scopes))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request device code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed: %d", resp.StatusCode)
	}

	var dcResp deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcResp); err != nil {
		return nil, fmt.Errorf("failed to decode device code response: %w", err)
	}

	interval := dcResp.Interval
	if interval == 0 {
		interval = 5
	}

	return &provider.AuthSession{
		DeviceCode:      dcResp.DeviceCode,
		UserCode:        dcResp.UserCode,
		VerificationURI: dcResp.VerificationURI,
		ExpiresAt:       time.Now().Add(time.Duration(dcResp.ExpiresIn) * time.Second).Unix(),
		Interval:        interval,
	}, nil
}

func (f *OAuthDeviceFlow) Poll(ctx context.Context, session *provider.AuthSession) (*provider.AuthResult, error) {
	tokenURL := f.TokenURL

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", session.DeviceCode)
	form.Set("client_id", f.ClientID)

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
		switch tokResp.Error {
		case "authorization_pending":
			return nil, nil // 还在等待用户授权
		case "slow_down":
			return nil, nil
		case "expired_token":
			return nil, fmt.Errorf("device code expired: %s", tokResp.ErrorDesc)
		default:
			return nil, fmt.Errorf("token error: %s: %s", tokResp.Error, tokResp.ErrorDesc)
		}
	}

	return &provider.AuthResult{
		AccessToken:  tokResp.AccessToken,
		TokenType:    tokResp.TokenType,
		ExpiresIn:    tokResp.ExpiresIn,
		RefreshToken: tokResp.RefreshToken,
		Scope:        tokResp.Scope,
	}, nil
}

func (f *OAuthDeviceFlow) Callback(_ context.Context, _ *provider.AuthSession, _ string) (*provider.AuthResult, error) {
	return nil, fmt.Errorf("device flow does not support callback")
}

func joinScopes(scopes []string) string {
	result := ""
	for i, s := range scopes {
		if i > 0 {
			result += "+"
		}
		result += s
	}
	return result
}
