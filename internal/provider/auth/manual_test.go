package auth

import (
	"context"
	"testing"

	"github.com/bestruirui/octopus/internal/provider"
)

func TestManualAuth_Type(t *testing.T) {
	m := &ManualAuth{}
	if m.Type() != provider.AuthTypeManual {
		t.Errorf("Type() = %q, want %q", m.Type(), provider.AuthTypeManual)
	}
}

func TestManualAuth_Start(t *testing.T) {
	m := &ManualAuth{}
	sess, err := m.Start(context.Background(), provider.AuthParams{})
	if sess != nil || err != nil {
		t.Errorf("Start() = %v, %v; want nil, nil", sess, err)
	}
}

func TestManualAuth_Poll(t *testing.T) {
	m := &ManualAuth{}
	result, err := m.Poll(context.Background(), &provider.AuthSession{})
	if result != nil || err != nil {
		t.Errorf("Poll() = %v, %v; want nil, nil", result, err)
	}
}

func TestManualAuth_Callback(t *testing.T) {
	m := &ManualAuth{}
	result, err := m.Callback(context.Background(), &provider.AuthSession{}, "code")
	if result != nil || err != nil {
		t.Errorf("Callback() = %v, %v; want nil, nil", result, err)
	}
}
