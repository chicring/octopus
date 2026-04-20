package op

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/provider/auth"
)

// CreateOAuthSession 创建 OAuth 认证会话
func CreateOAuthSession(ctx context.Context, providerID string, channelID int, session *provider.AuthSession) (*auth.OAuthSession, error) {
	idBytes := make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, err
	}

	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	encryptedSession, err := auth.Encrypt(sessionData)
	if err != nil {
		return nil, err
	}

	s := &auth.OAuthSession{
		ID:          hex.EncodeToString(idBytes),
		ProviderID:  providerID,
		ChannelID:   channelID,
		SessionData: string(encryptedSession),
		Status:      "pending",
		State:       session.State,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Unix(session.ExpiresAt, 0),
	}

	if err := db.GetDB().WithContext(ctx).Create(s).Error; err != nil {
		return nil, err
	}
	return s, nil
}

// GetOAuthSession 获取 OAuth 认证会话
func GetOAuthSession(ctx context.Context, id string) (*auth.OAuthSession, error) {
	var s auth.OAuthSession
	if err := db.GetDB().WithContext(ctx).Where("id = ?", id).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// CompleteOAuthSession 原子性地将 session 从 pending → completed
func CompleteOAuthSession(ctx context.Context, id string, result *provider.AuthResult) error {
	resultData, err := json.Marshal(result)
	if err != nil {
		return err
	}
	encryptedResult, err := auth.Encrypt(resultData)
	if err != nil {
		return err
	}

	return db.GetDB().WithContext(ctx).
		Model(&auth.OAuthSession{}).
		Where("id = ? AND status = ?", id, "pending").
		Updates(map[string]interface{}{
			"status":      "completed",
			"result_data": string(encryptedResult),
		}).Error
}

// FailOAuthSession 原子性地将 session 从 pending → failed
func FailOAuthSession(ctx context.Context, id string) error {
	return db.GetDB().WithContext(ctx).
		Model(&auth.OAuthSession{}).
		Where("id = ? AND status = ?", id, "pending").
		Update("status", "failed").Error
}

// GetOAuthSessionByState 根据 state 查找 session
func GetOAuthSessionByState(ctx context.Context, state string) (*auth.OAuthSession, error) {
	var s auth.OAuthSession
	if err := db.GetDB().WithContext(ctx).Where("state = ? AND status = ?", state, "pending").First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// CleanupExpiredOAuthSessions 清理过期 session
func CleanupExpiredOAuthSessions() {
	ctx := context.Background()
	db.GetDB().WithContext(ctx).
		Where("status = ? AND expires_at < ?", "pending", time.Now()).
		Delete(&auth.OAuthSession{})
	db.GetDB().WithContext(ctx).
		Where("status IN ? AND expires_at < ?", []string{"completed", "failed"}, time.Now().Add(-24*time.Hour)).
		Delete(&auth.OAuthSession{})
}