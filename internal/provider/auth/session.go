package auth

import "time"

// OAuthSession OAuth 认证会话持久化模型
type OAuthSession struct {
	ID          string    `json:"id" gorm:"primaryKey;size:64"`
	ProviderID  string    `json:"provider_id" gorm:"size:64;index"`
	ChannelID   int       `json:"channel_id" gorm:"default:0"`
	SessionData string    `json:"session_data" gorm:"type:text"`  // AES-GCM 加密
	ResultData  string    `json:"result_data" gorm:"type:text"`   // AES-GCM 加密
	Status      string    `json:"status" gorm:"size:20;index"`    // "pending", "completed", "failed"
	State       string    `json:"state" gorm:"size:64;index"`     // CSRF state 验证
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at" gorm:"index"`
}
