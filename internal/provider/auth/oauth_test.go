package auth

import (
	"testing"

	"github.com/bestruirui/octopus/internal/provider"
)

func TestOAuthDeviceFlow_Type(t *testing.T) {
	f := &OAuthDeviceFlow{}
	if f.Type() != provider.AuthTypeOAuthDevice {
		t.Errorf("Type() = %q, want %q", f.Type(), provider.AuthTypeOAuthDevice)
	}
}

func TestOAuthDeviceFlow_Callback(t *testing.T) {
	f := &OAuthDeviceFlow{}
	_, err := f.Callback(nil, nil, "code")
	if err == nil {
		t.Error("expected error: device flow does not support callback")
	}
}

func TestOAuthWebFlow_Type(t *testing.T) {
	f := &OAuthWebFlow{}
	if f.Type() != provider.AuthTypeOAuthWeb {
		t.Errorf("Type() = %q, want %q", f.Type(), provider.AuthTypeOAuthWeb)
	}
}

func TestOAuthWebFlow_Poll(t *testing.T) {
	f := &OAuthWebFlow{}
	result, err := f.Poll(nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for web flow poll")
	}
}