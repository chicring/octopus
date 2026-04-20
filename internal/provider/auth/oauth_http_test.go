package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/bestruirui/octopus/internal/provider"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func withHTTPClient(t *testing.T, fn roundTripFunc) {
	t.Helper()
	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: fn}
	t.Cleanup(func() { http.DefaultClient = oldClient })
}

func TestOAuthDeviceFlow_StartPostsFormBodyAndUsesDefaultScopes(t *testing.T) {
	withHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("content-type = %q", got)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("expected empty query string, got %q", r.URL.RawQuery)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse body: %v", err)
		}
		if got := values.Get("client_id"); got != "client-default" {
			t.Fatalf("client_id = %q", got)
		}
		if got := values.Get("scope"); got != "scope-a+scope-b" {
			t.Fatalf("scope = %q", got)
		}
		payload, _ := json.Marshal(map[string]any{
			"device_code":      "device-code",
			"user_code":        "user-code",
			"verification_uri": "https://verify.example",
			"expires_in":       600,
			"interval":         5,
		})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytesReader(payload)), Header: make(http.Header)}, nil
	})

	flow := &OAuthDeviceFlow{
		ClientID:      "client-default",
		DeviceCodeURL: "https://oauth.example/device",
		Scopes:        []string{"scope-a", "scope-b"},
	}

	session, err := flow.Start(context.Background(), provider.AuthParams{})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if session.DeviceCode != "device-code" {
		t.Fatalf("DeviceCode = %q", session.DeviceCode)
	}
	if session.UserCode != "user-code" {
		t.Fatalf("UserCode = %q", session.UserCode)
	}
}

func TestOAuthDeviceFlow_PollPostsFormBody(t *testing.T) {
	withHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("expected empty query string, got %q", r.URL.RawQuery)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse body: %v", err)
		}
		if got := values.Get("grant_type"); got != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Fatalf("grant_type = %q", got)
		}
		if got := values.Get("device_code"); got != "device-code" {
			t.Fatalf("device_code = %q", got)
		}
		if got := values.Get("client_id"); got != "client-default" {
			t.Fatalf("client_id = %q", got)
		}
		payload, _ := json.Marshal(map[string]any{
			"access_token":  "access-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "refresh-token",
			"scope":         "scope-a scope-b",
		})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytesReader(payload)), Header: make(http.Header)}, nil
	})

	flow := &OAuthDeviceFlow{ClientID: "client-default", TokenURL: "https://oauth.example/token"}
	result, err := flow.Poll(context.Background(), &provider.AuthSession{DeviceCode: "device-code"})
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if result == nil || result.AccessToken != "access-token" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestOAuthWebFlow_CallbackPostsFormBody(t *testing.T) {
	withHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("expected empty query string, got %q", r.URL.RawQuery)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("content-type = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse body: %v", err)
		}
		if got := values.Get("grant_type"); got != "authorization_code" {
			t.Fatalf("grant_type = %q", got)
		}
		if got := values.Get("code"); got != "callback-code" {
			t.Fatalf("code = %q", got)
		}
		if got := values.Get("client_id"); got != "client-default" {
			t.Fatalf("client_id = %q", got)
		}
		if got := values.Get("redirect_uri"); got != "https://octopus.example/callback" {
			t.Fatalf("redirect_uri = %q", got)
		}
		payload, _ := json.Marshal(map[string]any{
			"access_token":  "web-token",
			"token_type":    "Bearer",
			"expires_in":    1800,
			"refresh_token": "web-refresh",
		})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytesReader(payload)), Header: make(http.Header)}, nil
	})

	flow := &OAuthWebFlow{
		ClientID:    "client-default",
		TokenURL:    "https://oauth.example/token",
		RedirectURL: "https://octopus.example/callback",
	}

	result, err := flow.Callback(context.Background(), &provider.AuthSession{}, "callback-code")
	if err != nil {
		t.Fatalf("Callback() error = %v", err)
	}
	if result == nil || result.AccessToken != "web-token" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func bytesReader(b []byte) *readerAtEOF { return &readerAtEOF{data: b} }

type readerAtEOF struct{ data []byte }

func (r *readerAtEOF) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
