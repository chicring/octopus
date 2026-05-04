package op

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/usagecard"
	"github.com/bestruirui/octopus/internal/utils/cache"
)

var usageCardCache = cache.New[uint, model.UsageCard](4)

// UsageCardList 返回所有用量卡片（不返回密钥）
func UsageCardList(ctx context.Context) ([]model.UsageCard, error) {
	cards := make([]model.UsageCard, 0, usageCardCache.Len())
	for _, card := range usageCardCache.GetAll() {
		card.HasSecret = card.EncryptedSecret != ""
		card.EncryptedSecret = ""
		cards = append(cards, card)
	}
	return cards, nil
}

// UsageCardGet 获取单张卡片
func UsageCardGet(id uint, ctx context.Context) (model.UsageCard, error) {
	card, ok := usageCardCache.Get(id)
	if !ok {
		return model.UsageCard{}, fmt.Errorf("usage card not found")
	}
	return card, nil
}

// UsageCardCreate 创建用量卡片
func UsageCardCreate(req *model.UsageCardCreateRequest, ctx context.Context) (*model.UsageCard, error) {
	card := model.UsageCard{
		Name:       req.Name,
		TemplateID: req.TemplateID,
		Account:    req.Account,
		Endpoint:   req.Endpoint,
		AuthType:   req.AuthType,
		AuthHeader: req.AuthHeader,
		Config:     req.Config,
	}

	// 方法
	if req.Method != "" {
		card.Method = req.Method
	} else {
		card.Method = "GET"
	}

	// 认证方式
	if card.AuthType == "" {
		card.AuthType = "none"
	}

	// 启用
	if req.Enabled != nil {
		card.Enabled = *req.Enabled
	} else {
		card.Enabled = true
	}

	// 代理
	if req.UseProxy != nil {
		card.UseProxy = *req.UseProxy
	}

	// 刷新间隔
	if req.RefreshIntervalSec != nil {
		card.RefreshIntervalSec = *req.RefreshIntervalSec
	} else {
		card.RefreshIntervalSec = 300
	}

	// 额外请求头
	if req.ExtraHeaders != nil {
		card.ExtraHeaders = req.ExtraHeaders
	}

	// 从模板填充默认值
	if t, ok := usagecard.GetTemplate(req.TemplateID); ok {
		if card.Endpoint == "" {
			card.Endpoint = t.DefaultEndpoint
		}
		if card.Method == "" {
			card.Method = t.Method
		}
		if len(card.Config.Metrics) == 0 {
			card.Config = usagecard.BuildCardConfig(t)
		}
		if len(card.ExtraHeaders) == 0 {
			card.ExtraHeaders = usagecard.BuildExtraHeaders(t)
		}
	}

	// 加密密钥
	if req.Secret != "" {
		encrypted, err := usagecard.EncryptSecret(req.Secret)
		if err != nil {
			return nil, err
		}
		card.EncryptedSecret = encrypted
	}

	if err := db.GetDB().WithContext(ctx).Create(&card).Error; err != nil {
		return nil, err
	}

	card.HasSecret = card.EncryptedSecret != ""
	usageCardCache.Set(card.ID, card)
	card.EncryptedSecret = ""

	return &card, nil
}

// UsageCardUpdate 更新用量卡片
func UsageCardUpdate(req *model.UsageCardUpdateRequest, ctx context.Context) (*model.UsageCard, error) {
	existing, ok := usageCardCache.Get(req.ID)
	if !ok {
		return nil, fmt.Errorf("usage card not found")
	}

	updates := model.UsageCard{ID: req.ID}
	var selectFields []string

	if req.Name != nil {
		selectFields = append(selectFields, "name")
		updates.Name = *req.Name
	}
	if req.TemplateID != nil {
		selectFields = append(selectFields, "template_id")
		updates.TemplateID = *req.TemplateID
	}
	if req.Account != nil {
		selectFields = append(selectFields, "account")
		updates.Account = *req.Account
	}
	if req.Endpoint != nil {
		selectFields = append(selectFields, "endpoint")
		updates.Endpoint = *req.Endpoint
	}
	if req.Method != nil {
		selectFields = append(selectFields, "method")
		updates.Method = *req.Method
	}
	if req.AuthType != nil {
		selectFields = append(selectFields, "auth_type")
		updates.AuthType = *req.AuthType
	}
	if req.AuthHeader != nil {
		selectFields = append(selectFields, "auth_header")
		updates.AuthHeader = *req.AuthHeader
	}
	if req.ExtraHeaders != nil {
		selectFields = append(selectFields, "extra_headers")
		updates.ExtraHeaders = *req.ExtraHeaders
	}
	if req.Config != nil {
		selectFields = append(selectFields, "config")
		updates.Config = *req.Config
	}
	if req.Enabled != nil {
		selectFields = append(selectFields, "enabled")
		updates.Enabled = *req.Enabled
	}
	if req.UseProxy != nil {
		selectFields = append(selectFields, "use_proxy")
		updates.UseProxy = *req.UseProxy
	}
	if req.RefreshIntervalSec != nil {
		selectFields = append(selectFields, "refresh_interval_sec")
		updates.RefreshIntervalSec = *req.RefreshIntervalSec
	}

	// 密钥更新：传了新值就更新，不传就保留
	if req.Secret != nil {
		selectFields = append(selectFields, "encrypted_secret")
		if *req.Secret == "" {
			updates.EncryptedSecret = ""
		} else {
			encrypted, err := usagecard.EncryptSecret(*req.Secret)
			if err != nil {
				return nil, err
			}
			updates.EncryptedSecret = encrypted
		}
	}

	if len(selectFields) == 0 {
		existing.HasSecret = existing.EncryptedSecret != ""
		existing.EncryptedSecret = ""
		return &existing, nil
	}

	if err := db.GetDB().WithContext(ctx).Model(&model.UsageCard{}).Where("id = ?", req.ID).Select(selectFields).Updates(&updates).Error; err != nil {
		return nil, err
	}

	// 重新从 DB 加载
	var updated model.UsageCard
	if err := db.GetDB().WithContext(ctx).First(&updated, req.ID).Error; err != nil {
		return nil, err
	}
	usageCardCache.Set(updated.ID, updated)
	updated.HasSecret = updated.EncryptedSecret != ""
	updated.EncryptedSecret = ""

	return &updated, nil
}

// UsageCardDelete 删除用量卡片
func UsageCardDelete(id uint, ctx context.Context) error {
	if err := db.GetDB().WithContext(ctx).Delete(&model.UsageCard{}, id).Error; err != nil {
		return err
	}
	usageCardCache.Del(id)
	return nil
}

// UsageCardRefresh 刷新单张卡片的用量数据
func UsageCardRefresh(id uint, ctx context.Context) (*model.UsageCard, error) {
	card, ok := usageCardCache.Get(id)
	if !ok {
		return nil, fmt.Errorf("usage card not found")
	}

	result := usagecard.Refresh(ctx, card)

	now := time.Now()
	card.LastRefreshAt = &now

	if result.Error != "" {
		card.LastError = result.Error
	} else {
		card.LastResult = result.Snapshot
		card.LastError = ""
	}

	// 如果 token 被刷新，持久化新的加密凭证
	selectFields := []string{"last_result", "last_error", "last_refresh_at"}
	if result.RefreshedCred != "" {
		card.EncryptedSecret = result.RefreshedCred
		selectFields = append(selectFields, "encrypted_secret")
	}

	// 保存到数据库
	if err := db.GetDB().WithContext(ctx).Model(&model.UsageCard{}).Where("id = ?", id).
		Select(selectFields).
		Updates(&card).Error; err != nil {
		return nil, err
	}

	usageCardCache.Set(card.ID, card)
	card.HasSecret = card.EncryptedSecret != ""
	card.EncryptedSecret = ""

	return &card, nil
}

// UsageCardLoadCache 从数据库加载缓存
func UsageCardLoadCache(ctx context.Context) error {
	var cards []model.UsageCard
	if err := db.GetDB().WithContext(ctx).Find(&cards).Error; err != nil {
		return err
	}
	for _, card := range cards {
		usageCardCache.Set(card.ID, card)
	}
	return nil
}

// UsageCardGetByAccount 根据账户标识查找用量卡片（用于去重）
func UsageCardGetByAccount(account string) (*model.UsageCard, bool) {
	for _, card := range usageCardCache.GetAll() {
		if card.Account == account {
			card.HasSecret = card.EncryptedSecret != ""
			card.EncryptedSecret = ""
			return &card, true
		}
	}
	return nil, false
}

// AutoCreateCodexUsageCard 为 Codex 渠道自动创建用量卡片
// 如果渠道的 provider_id 为 "codex" 且有 key，则为每个 key 创建一张 Codex 用量卡片
// 失败不影响渠道创建，仅记录 warning
func AutoCreateCodexUsageCard(channel *model.Channel, ctx context.Context) {
	if channel.ProviderID != "codex" || len(channel.Keys) == 0 {
		return
	}

	for _, key := range channel.Keys {
		if key.ChannelKey == "" {
			continue
		}

		// 从 key.Remark 提取邮箱标识（格式 "[auth-file] email@..." 或 OAuth 流程的邮箱）
		emailLabel := extractEmailFromRemark(key.Remark)
		account := fmt.Sprintf("codex:%d:%d", channel.ID, key.ID)
		if _, exists := UsageCardGetByAccount(account); exists {
			continue
		}

		// 卡片名称包含邮箱标识
		cardName := "Codex"
		if emailLabel != "" {
			cardName = fmt.Sprintf("Codex - %s", emailLabel)
		}

		// 构建 Codex 用量卡片（UsageCardCreate 会从模板填充 ExtraHeaders/Config）
		req := &model.UsageCardCreateRequest{
			Name:               cardName,
			TemplateID:         "codex-usage",
			Account:            account,
			AuthType:           "bearer",
			Secret:             key.ChannelKey,
			Enabled:            lo.ToPtr(true),
			RefreshIntervalSec: lo.ToPtr(300),
		}

		if _, err := UsageCardCreate(req, ctx); err != nil {
			log.Printf("[codex] auto-create usage card failed for channel %d key %d: %v", channel.ID, key.ID, err)
		}
	}
}

// extractEmailFromRemark 从 key.Remark 中提取邮箱标识
// remark 现在直接存邮箱，无需额外解析
func extractEmailFromRemark(remark string) string {
	return strings.TrimSpace(remark)
}
