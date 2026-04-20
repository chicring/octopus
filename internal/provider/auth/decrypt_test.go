package auth

import (
	"encoding/json"
	"testing"

	"github.com/bestruirui/octopus/internal/provider"
)

func TestDecryptSessionData_Empty(t *testing.T) {
	result, err := DecryptSessionData("")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil for empty input")
	}
}

func TestDecryptResultData_Empty(t *testing.T) {
	result, err := DecryptResultData("")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil for empty input")
	}
}

func TestDecryptSessionData_NoKey(t *testing.T) {
	// 无密钥时，输入是 JSON 明文
	session := provider.AuthSession{
		DeviceCode:      "test-device-code",
		UserCode:        "ABCD-1234",
		VerificationURI: "https://github.com/login/device",
	}
	data, _ := json.Marshal(session)
	result, err := DecryptSessionData(string(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.DeviceCode != "test-device-code" {
		t.Errorf("DeviceCode = %q, want %q", result.DeviceCode, "test-device-code")
	}
	if result.UserCode != "ABCD-1234" {
		t.Errorf("UserCode = %q, want %q", result.UserCode, "ABCD-1234")
	}
}

func TestDecryptResultData_NoKey(t *testing.T) {
	result := provider.AuthResult{
		AccessToken: "test-token",
		TokenType:   "bearer",
	}
	data, _ := json.Marshal(result)
	got, err := DecryptResultData(string(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.AccessToken != "test-token" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "test-token")
	}
}

func TestDecryptSessionData_InvalidJSON(t *testing.T) {
	_, err := DecryptSessionData("not-json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDecryptResultData_InvalidJSON(t *testing.T) {
	_, err := DecryptResultData("not-json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
