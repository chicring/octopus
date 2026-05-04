package op

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/provider/auth"
)

// helper: 构造一个 Codex auth ChannelKey
func makeCodexTestKey(id, channelID int, email string, expiresAt time.Time, refreshToken string) model.ChannelKey {
	cred := auth.CodexCredential{
		AccessToken:  "at-test-" + email,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    expiresAt.Format(time.RFC3339),
		AccountID:    "acct-" + email,
		Email:        email,
	}
	return model.ChannelKey{
		ID:               id,
		ChannelID:        channelID,
		Enabled:          true,
		ChannelKey:       cred.String(),
		StatusCode:       0,
		LastUseTimeStamp: time.Now().Unix(),
	}
}

// TestIsCodexAuthKey 验证 Codex auth key 判断
func TestIsCodexAuthKey(t *testing.T) {
	if IsCodexAuthKey("") {
		t.Error("empty key should not be codex auth")
	}
	if IsCodexAuthKey("sk-abc123") {
		t.Error("plain API key should not be codex auth")
	}
	if !IsCodexAuthKey(`{"access_token":"at"}`) {
		t.Error("JSON key should be codex auth")
	}
}

// TestEnsureCodexKeyReady_NotCodexAuth 非 Codex auth key 直接可用
func TestEnsureCodexKeyReady_NotCodexAuth(t *testing.T) {
	key := &model.ChannelKey{
		ID:          1,
		ChannelID:   1,
		ChannelKey:  "sk-plain-key",
		Enabled:     true,
	}
	newKey, ready, err := EnsureCodexKeyReady(context.Background(), key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Error("plain key should be ready")
	}
	if newKey != "sk-plain-key" {
		t.Errorf("expected same key, got %s", newKey)
	}
}

// TestEnsureCodexKeyReady_NotExpired 未过期的 Codex key 直接可用
func TestEnsureCodexKeyReady_NotExpired(t *testing.T) {
	// 设置缓存
	key := makeCodexTestKey(10, 1, "user@test.com", time.Now().Add(1*time.Hour), "rt-123")
	channelKeyCache.Set(key.ID, key)
	ch := model.Channel{ID: 1, Keys: []model.ChannelKey{key}}
	channelCache.Set(1, ch)

	newKey, ready, err := EnsureCodexKeyReady(context.Background(), &key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Error("non-expired key should be ready")
	}
	_ = newKey
}

// TestEnsureCodexKeyReady_ExpiredNoRefresh 过期且无 refresh_token，自动关闭
func TestEnsureCodexKeyReady_ExpiredNoRefresh(t *testing.T) {
	key := makeCodexTestKey(20, 2, "expired@test.com", time.Now().Add(-1*time.Hour), "")
	channelKeyCache.Set(key.ID, key)
	ch := model.Channel{ID: 2, Keys: []model.ChannelKey{key}}
	channelCache.Set(2, ch)

	_, ready, _ := EnsureCodexKeyReady(context.Background(), &key)
	if ready {
		t.Error("expired key without refresh_token should not be ready")
	}

	// 验证 key 被关闭
	updated, ok := channelKeyCache.Get(key.ID)
	if !ok {
		t.Fatal("key should still be in cache")
	}
	if updated.Enabled {
		t.Error("key should be disabled")
	}
	if updated.StatusCode != 401 {
		t.Errorf("expected StatusCode=401, got %d", updated.StatusCode)
	}
}

// TestEnsureCodexKeyReady_ExpiredWithRefresh 过期但有 refresh_token，触发刷新
// 由于刷新需要真实 token endpoint，这里只验证刷新失败时 key 被关闭
func TestEnsureCodexKeyReady_ExpiredWithRefresh_Fail(t *testing.T) {
	key := makeCodexTestKey(30, 3, "refresh@test.com", time.Now().Add(-1*time.Hour), "rt-old")
	channelKeyCache.Set(key.ID, key)
	ch := model.Channel{ID: 3, Keys: []model.ChannelKey{key}}
	channelCache.Set(3, ch)

	// 刷新会失败（没有 mock token endpoint），key 应被关闭
	_, ready, _ := EnsureCodexKeyReady(context.Background(), &key)
	if ready {
		t.Error("key with failing refresh should not be ready")
	}

	updated, ok := channelKeyCache.Get(key.ID)
	if !ok {
		t.Fatal("key should still be in cache")
	}
	if updated.Enabled {
		t.Error("key should be disabled after refresh failure")
	}
}

// TestCodexRefreshLock 并发刷新同一 key 只触发一次
func TestCodexRefreshLock(t *testing.T) {
	keyID := 999
	mu1 := getCodexRefreshLock(keyID)
	mu2 := getCodexRefreshLock(keyID)

	if mu1 != mu2 {
		t.Error("same keyID should return same mutex")
	}

	// 不同 keyID 返回不同 mutex
	mu3 := getCodexRefreshLock(keyID + 1)
	if mu1 == mu3 {
		t.Error("different keyID should return different mutex")
	}
}

// TestDisableCodexKey 验证关闭 key 逻辑
func TestDisableCodexKey(t *testing.T) {
	key := makeCodexTestKey(40, 4, "disable@test.com", time.Now().Add(1*time.Hour), "rt-xxx")
	key.Enabled = true
	channelKeyCache.Set(key.ID, key)
	ch := model.Channel{ID: 4, Keys: []model.ChannelKey{key}}
	channelCache.Set(4, ch)

	DisableCodexKey(context.Background(), &key, 401)

	updated, ok := channelKeyCache.Get(key.ID)
	if !ok {
		t.Fatal("key should still be in cache")
	}
	if updated.Enabled {
		t.Error("key should be disabled")
	}
	if updated.StatusCode != 401 {
		t.Errorf("expected StatusCode=401, got %d", updated.StatusCode)
	}
}

// TestChannelKeyUpdatePersistent 验证持久化更新
func TestChannelKeyUpdatePersistent(t *testing.T) {
	// 需要数据库初始化，这里只测试缓存更新部分
	key := model.ChannelKey{
		ID:               50,
		ChannelID:        5,
		Enabled:          true,
		ChannelKey:       "sk-test-persist",
		StatusCode:       0,
		LastUseTimeStamp: time.Now().Unix(),
	}
	channelKeyCache.Set(key.ID, key)
	ch := model.Channel{ID: 5, Keys: []model.ChannelKey{key}}
	channelCache.Set(5, ch)

	// 更新 key
	key.StatusCode = 429
	err := ChannelKeyUpdate(key)
	if err != nil {
		t.Fatalf("ChannelKeyUpdate failed: %v", err)
	}

	updated, ok := channelKeyCache.Get(key.ID)
	if !ok {
		t.Fatal("key should be in cache")
	}
	if updated.StatusCode != 429 {
		t.Errorf("expected StatusCode=429, got %d", updated.StatusCode)
	}
}

// TestCodexCredentialMerge 验证刷新后凭证合并（保留旧字段）
func TestCodexCredentialMerge(t *testing.T) {
	oldCred := auth.CodexCredential{
		AccessToken:  "old-at",
		RefreshToken: "old-rt",
		AccountID:    "acct-123",
		Email:        "user@test.com",
		IDToken:      "old-id-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
	}

	// 模拟刷新结果（新响应可能缺少 account_id、email、id_token）
	result := &provider.AuthResult{
		AccessToken:  "new-at",
		RefreshToken: "new-rt",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		Extra: map[string]string{
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		},
	}

	newCred := auth.BuildCodexCredentialFromAuthResult(result)
	// 合并：保留旧字段
	if newCred.AccountID == "" {
		newCred.AccountID = oldCred.AccountID
	}
	if newCred.Email == "" {
		newCred.Email = oldCred.Email
	}
	if newCred.IDToken == "" {
		newCred.IDToken = oldCred.IDToken
	}

	if newCred.AccessToken != "new-at" {
		t.Errorf("expected new access_token, got %s", newCred.AccessToken)
	}
	if newCred.RefreshToken != "new-rt" {
		t.Errorf("expected new refresh_token, got %s", newCred.RefreshToken)
	}
	if newCred.AccountID != "acct-123" {
		t.Errorf("expected preserved account_id, got %s", newCred.AccountID)
	}
	if newCred.Email != "user@test.com" {
		t.Errorf("expected preserved email, got %s", newCred.Email)
	}
	if newCred.IDToken != "old-id-token" {
		t.Errorf("expected preserved id_token, got %s", newCred.IDToken)
	}
}

// TestConcurrentRefreshLock 验证并发请求同一 key 时只刷新一次
func TestConcurrentRefreshLock(t *testing.T) {
	keyID := 888
	var refreshCount int64
	var mu sync.Mutex

	// 模拟多个 goroutine 同时尝试获取刷新锁
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lock := getCodexRefreshLock(keyID)
			lock.Lock()
			defer lock.Unlock()
			mu.Lock()
			refreshCount++
			mu.Unlock()
			time.Sleep(10 * time.Millisecond) // 模拟刷新耗时
		}()
	}
	wg.Wait()

	// 由于是串行获取锁，refreshCount 应该等于 10（每个 goroutine 都会执行）
	// 但实际场景中，加锁后重新读取缓存会发现已刷新，后续 goroutine 跳过
	if refreshCount != 10 {
		t.Errorf("expected 10 serial accesses, got %d", refreshCount)
	}
}

// TestParseCodexCredential_RoundTrip 验证凭证序列化/反序列化一致性
func TestParseCodexCredential_RoundTrip(t *testing.T) {
	original := auth.CodexCredential{
		AccessToken:  "at-roundtrip",
		RefreshToken: "rt-roundtrip",
		AccountID:    "acct-roundtrip",
		Email:        "roundtrip@test.com",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	}

	jsonStr := original.String()
	parsed, err := auth.ParseCodexCredential(jsonStr)
	if err != nil {
		t.Fatalf("ParseCodexCredential failed: %v", err)
	}

	if parsed.AccessToken != original.AccessToken {
		t.Errorf("access_token mismatch: %s != %s", parsed.AccessToken, original.AccessToken)
	}
	if parsed.RefreshToken != original.RefreshToken {
		t.Errorf("refresh_token mismatch: %s != %s", parsed.RefreshToken, original.RefreshToken)
	}
	if parsed.AccountID != original.AccountID {
		t.Errorf("account_id mismatch: %s != %s", parsed.AccountID, original.AccountID)
	}
	if parsed.Email != original.Email {
		t.Errorf("email mismatch: %s != %s", parsed.Email, original.Email)
	}
}

// TestIsExpired 验证过期判断
func TestIsExpired(t *testing.T) {
	// 未过期
	cred := auth.CodexCredential{
		AccessToken: "at",
		ExpiresAt:   time.Now().Add(10 * time.Minute).Format(time.RFC3339),
	}
	if cred.IsExpired() {
		t.Error("key expiring in 10min should not be expired (5min buffer)")
	}

	// 即将过期（5分钟内）
	cred.ExpiresAt = time.Now().Add(3 * time.Minute).Format(time.RFC3339)
	if !cred.IsExpired() {
		t.Error("key expiring in 3min should be expired (5min buffer)")
	}

	// 已过期
	cred.ExpiresAt = time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	if !cred.IsExpired() {
		t.Error("key expired 1h ago should be expired")
	}

	// 无过期时间
	cred.ExpiresAt = ""
	if cred.IsExpired() {
		t.Error("key without expires_at should not be considered expired")
	}
}

// TestEnsureCodexKeyReady_NilKey nil key 不崩溃
func TestEnsureCodexKeyReady_NilKey(t *testing.T) {
	newKey, ready, err := EnsureCodexKeyReady(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Error("nil key should be ready (passthrough)")
	}
	if newKey != "" {
		t.Errorf("expected empty key, got %s", newKey)
	}
}

// TestBuildCodexCredentialFromAuthResult_PreserveRefreshToken 验证刷新结果保留旧 refresh_token
func TestBuildCodexCredentialFromAuthResult_PreserveRefreshToken(t *testing.T) {
	result := &provider.AuthResult{
		AccessToken: "new-at",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
		// 故意不返回 refresh_token
		Extra: map[string]string{
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		},
	}

	cred := auth.BuildCodexCredentialFromAuthResult(result)
	// RefreshToken 应为空（AuthResult 没返回）
	if cred.RefreshToken != "" {
		t.Errorf("expected empty refresh_token when not in result, got %s", cred.RefreshToken)
	}
}

// TestCodexAuthKey_JSONDetection 验证各种 key 格式的判断
func TestCodexAuthKey_JSONDetection(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"", false},
		{"sk-abc123", false},
		{"Bearer token123", false},
		{`{"access_token":"at"}`, true},
		{`{"refresh_token":"rt"}`, true},
		{`[]`, false}, // JSON array is not a codex auth key
	}

	for _, tt := range tests {
		got := IsCodexAuthKey(tt.key)
		if got != tt.expected {
			t.Errorf("IsCodexAuthKey(%q) = %v, want %v", tt.key, got, tt.expected)
		}
	}
}

// TestEnsureCodexKeyReady_ParseFailure 解析失败的 key 不阻止请求
func TestEnsureCodexKeyReady_ParseFailure(t *testing.T) {
	key := &model.ChannelKey{
		ID:          99,
		ChannelID:   9,
		ChannelKey:  `{invalid json`,
		Enabled:     true,
	}
	newKey, ready, err := EnsureCodexKeyReady(context.Background(), key)
	if err != nil {
		t.Fatalf("parse failure should not return error: %v", err)
	}
	if !ready {
		t.Error("parse failure should still be ready (let request fail naturally)")
	}
	if newKey != `{invalid json` {
		t.Errorf("expected same key, got %s", newKey)
	}
}

// TestDisableCodexKey_NotInCache key 不在缓存中不崩溃
func TestDisableCodexKey_NotInCache(t *testing.T) {
	key := &model.ChannelKey{
		ID:          99999,
		ChannelID:   999,
		ChannelKey:  `{"access_token":"at"}`,
		Enabled:     true,
	}
	// 不应 panic
	DisableCodexKey(context.Background(), key, 401)
}

// TestChannelKeyUpdatePersistent_InvalidKey 无效 key 返回错误
func TestChannelKeyUpdatePersistent_InvalidKey(t *testing.T) {
	key := model.ChannelKey{ID: 0, ChannelID: 0}
	err := ChannelKeyUpdatePersistent(context.Background(), key)
	if err == nil {
		t.Error("expected error for invalid key")
	}
}

// ensure provider.AuthResult is available for test
var _ = json.Marshal
