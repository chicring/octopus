package op

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/provider"
	providerauth "github.com/bestruirui/octopus/internal/provider/auth"
)

func TestOAuthSessionLifecycle_CreatePollComplete(t *testing.T) {
	ctx := context.Background()
	setupOAuthTestDB(t)

	session := &provider.AuthSession{
		DeviceCode:      "device-code",
		UserCode:        "user-code",
		VerificationURI: "https://verify.example",
		State:           "state-123",
		ExpiresAt:       time.Now().Add(5 * time.Minute).Unix(),
		Interval:        5,
	}

	created, err := CreateOAuthSession(ctx, "openai-chat", 42, session)
	if err != nil {
		t.Fatalf("CreateOAuthSession() error = %v", err)
	}
	if created.Status != "pending" {
		t.Fatalf("created.Status = %q", created.Status)
	}

	loaded, err := GetOAuthSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetOAuthSession() error = %v", err)
	}
	if loaded.ProviderID != "openai-chat" || loaded.ChannelID != 42 {
		t.Fatalf("loaded session mismatch: provider=%q channel=%d", loaded.ProviderID, loaded.ChannelID)
	}
	decryptedSession, err := providerauth.DecryptSessionData(loaded.SessionData)
	if err != nil {
		t.Fatalf("DecryptSessionData() error = %v", err)
	}
	if decryptedSession.State != "state-123" {
		t.Fatalf("decrypted session state = %q", decryptedSession.State)
	}

	byState, err := GetOAuthSessionByState(ctx, "state-123")
	if err != nil {
		t.Fatalf("GetOAuthSessionByState() error = %v", err)
	}
	if byState.ID != created.ID {
		t.Fatalf("GetOAuthSessionByState() returned %q, want %q", byState.ID, created.ID)
	}

	result := &provider.AuthResult{AccessToken: "access-token", RefreshToken: "refresh-token"}
	if err := CompleteOAuthSession(ctx, created.ID, result); err != nil {
		t.Fatalf("CompleteOAuthSession() error = %v", err)
	}

	completed, err := GetOAuthSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetOAuthSession() after completion error = %v", err)
	}
	if completed.Status != "completed" {
		t.Fatalf("completed.Status = %q", completed.Status)
	}
	decryptedResult, err := providerauth.DecryptResultData(completed.ResultData)
	if err != nil {
		t.Fatalf("DecryptResultData() error = %v", err)
	}
	if decryptedResult.AccessToken != "access-token" {
		t.Fatalf("decryptedResult.AccessToken = %q", decryptedResult.AccessToken)
	}

	if _, err := GetOAuthSessionByState(ctx, "state-123"); err == nil {
		t.Fatal("expected completed session to disappear from pending state lookup")
	}
}

func TestOAuthSessionLifecycle_Fail(t *testing.T) {
	ctx := context.Background()
	setupOAuthTestDB(t)

	created, err := CreateOAuthSession(ctx, "openai-chat", 1, &provider.AuthSession{State: "state-fail", ExpiresAt: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatalf("CreateOAuthSession() error = %v", err)
	}
	if err := FailOAuthSession(ctx, created.ID); err != nil {
		t.Fatalf("FailOAuthSession() error = %v", err)
	}
	loaded, err := GetOAuthSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetOAuthSession() error = %v", err)
	}
	if loaded.Status != "failed" {
		t.Fatalf("loaded.Status = %q", loaded.Status)
	}
}

func setupOAuthTestDB(t *testing.T) {
	t.Helper()
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "oauth-session.db"), false); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
}
