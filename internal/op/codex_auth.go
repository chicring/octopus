package op

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/provider/auth"
	"github.com/bestruirui/octopus/internal/utils/log"
)

// codexRefreshLocks per-key 刷新锁，避免并发请求重复刷新同一个 key
var codexRefreshLocks sync.Map // map[int]*sync.Mutex

func getCodexRefreshLock(keyID int) *sync.Mutex {
	v, _ := codexRefreshLocks.LoadOrStore(keyID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// IsCodexAuthKey 判断 key 是否为 Codex OAuth 凭证（JSON 格式含 access_token）
func IsCodexAuthKey(key string) bool {
	return len(key) > 0 && key[0] == '{'
}

// EnsureCodexKeyReady 请求前检查 Codex auth 凭证状态，必要时刷新。
// 返回：(更新后的 key 字符串, 是否可用, error)
// 如果不可用（过期且无 refresh_token，或刷新失败），会自动关闭该 key。
func EnsureCodexKeyReady(ctx context.Context, key *model.ChannelKey) (newKeyStr string, ready bool, err error) {
	if key == nil {
		return "", true, nil
	}
	if key.ChannelKey == "" || !IsCodexAuthKey(key.ChannelKey) {
		// 非 Codex auth key，直接可用
		return key.ChannelKey, true, nil
	}

	cred, parseErr := auth.ParseCodexCredential(key.ChannelKey)
	if parseErr != nil {
		log.Warnf("[codex-auth] key %d: failed to parse credential: %v", key.ID, parseErr)
		return key.ChannelKey, true, nil // 解析失败不阻止，让请求自行失败
	}

	// 未过期，直接可用
	if !cred.IsExpired() {
		return key.ChannelKey, true, nil
	}

	// 过期但没有 refresh_token，不可恢复，自动关闭
	if cred.RefreshToken == "" {
		log.Warnf("[codex-auth] key %d: token expired and no refresh_token, disabling key", key.ID)
		DisableCodexKey(ctx, key, 401)
		return "", false, fmt.Errorf("codex key %d: token expired, no refresh_token", key.ID)
	}

	// 过期且有 refresh_token，加锁刷新
	newKeyStr, refreshErr := RefreshCodexKey(ctx, key, false)
	if refreshErr != nil {
		log.Warnf("[codex-auth] key %d: refresh failed: %v, disabling key", key.ID, refreshErr)
		DisableCodexKey(ctx, key, 401)
		return "", false, refreshErr
	}

	return newKeyStr, true, nil
}

// RefreshCodexKey 刷新 Codex auth 凭证。
// force=false：仅过期/即将过期时刷新；force=true：强制刷新（401 后）。
// 加 per-key 锁，避免并发刷新。
// 刷新成功后回写缓存和数据库。
// 返回更新后的 key 字符串。
func RefreshCodexKey(ctx context.Context, key *model.ChannelKey, force bool) (string, error) {
	mu := getCodexRefreshLock(key.ID)
	mu.Lock()
	defer mu.Unlock()

	// 加锁后重新读取缓存中的最新 key，避免重复刷新
	freshKey, ok := channelKeyCache.Get(key.ID)
	if !ok {
		return "", fmt.Errorf("codex key %d: not found in cache", key.ID)
	}

	cred, parseErr := auth.ParseCodexCredential(freshKey.ChannelKey)
	if parseErr != nil {
		return "", fmt.Errorf("codex key %d: failed to parse credential: %w", key.ID, parseErr)
	}

	// 非强制模式下，如果未过期则跳过
	if !force && !cred.IsExpired() {
		return freshKey.ChannelKey, nil
	}

	if cred.RefreshToken == "" {
		return "", fmt.Errorf("codex key %d: no refresh_token available", key.ID)
	}

	// 执行刷新
	flow := &auth.CodexOAuthFlow{
		ClientID: auth.CodexClientID,
		TokenURL: auth.CodexTokenURL,
	}
	result, refreshErr := flow.RefreshToken(ctx, cred.RefreshToken)
	if refreshErr != nil {
		return "", fmt.Errorf("codex key %d: refresh failed: %w", key.ID, refreshErr)
	}

	// 合并新旧凭证：保留新响应可能缺失的字段
	newCred := auth.BuildCodexCredentialFromAuthResult(result)
	if newCred.AccountID == "" {
		newCred.AccountID = cred.AccountID
	}
	if newCred.Email == "" {
		newCred.Email = cred.Email
	}
	if newCred.IDToken == "" {
		newCred.IDToken = cred.IDToken
	}

	newKeyStr := newCred.String()

	// 回写缓存和数据库
	freshKey.ChannelKey = newKeyStr
	if err := ChannelKeyUpdatePersistent(ctx, freshKey); err != nil {
		log.Errorf("[codex-auth] key %d: failed to persist refreshed credential: %v", key.ID, err)
	} else {
		log.Infof("[codex-auth] key %d: token refreshed successfully, new expires_at: %s", key.ID, newCred.ExpiresAt)
	}

	return newKeyStr, nil
}

// DisableCodexKey 自动关闭 Codex auth key（Enabled=false, StatusCode=401），并持久化。
func DisableCodexKey(ctx context.Context, key *model.ChannelKey, statusCode int) {
	freshKey, ok := channelKeyCache.Get(key.ID)
	if !ok {
		log.Warnf("[codex-auth] key %d: not found in cache when disabling", key.ID)
		return
	}

	freshKey.Enabled = false
	freshKey.StatusCode = statusCode
	freshKey.LastUseTimeStamp = time.Now().Unix()

	if err := ChannelKeyUpdatePersistent(ctx, freshKey); err != nil {
		log.Errorf("[codex-auth] key %d: failed to persist disabled key: %v", key.ID, err)
	} else {
		log.Warnf("[codex-auth] key %d: auto-disabled (status=%d)", key.ID, statusCode)
	}
}

// ChannelKeyUpdatePersistent 更新 ChannelKey 内存缓存并立即写入数据库。
func ChannelKeyUpdatePersistent(ctx context.Context, key model.ChannelKey) error {
	if key.ID == 0 || key.ChannelID == 0 {
		return fmt.Errorf("invalid channel key")
	}

	// 先更新内存缓存（复用 ChannelKeyUpdate 逻辑）
	if err := ChannelKeyUpdate(key); err != nil {
		return err
	}

	// 立即写入数据库（如果 DB 可用）
	dbConn := db.GetDB()
	if dbConn != nil {
		if err := dbConn.WithContext(ctx).Save(&key).Error; err != nil {
			log.Warnf("[codex-auth] failed to persist key %d to db: %v", key.ID, err)
			// DB 写入失败不影响缓存更新，dirty set 保留让 SaveCache 兜底
			return nil
		}
		// 从 dirty set 中移除（因为已经落库了）
		channelKeyCacheNeedUpdateLock.Lock()
		delete(channelKeyCacheNeedUpdate, key.ID)
		channelKeyCacheNeedUpdateLock.Unlock()
	}

	return nil
}
