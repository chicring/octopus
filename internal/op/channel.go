package op

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/utils/cache"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/xstrings"
	"gorm.io/gorm"
)

var channelCache = cache.New[int, model.Channel](16)
var channelKeyCache = cache.New[int, model.ChannelKey](16)
var channelKeyCacheNeedUpdate = make(map[int]struct{})
var channelKeyCacheNeedUpdateLock sync.Mutex

// channelKeyRotationCounters 用于渠道内 Key 轮询的原子计数器，key 为 channelID
var channelKeyRotationCounters sync.Map // map[int]*uint64

// ChannelGetKey 从缓存中获取渠道，并使用最低成本优先 + 轮询 tiebreaker 策略选择可用 Key。
// 当多个 Key 具有相同最低成本时，通过原子计数器实现轮询，确保全轮询。
func ChannelGetKey(channelID int) model.ChannelKey {
	return ChannelGetKeyForModel(channelID, "")
}

// ChannelGetKeyForModel 在渠道内按模型白名单、429 冷却和倍率评分选择 Key。
// 倍率作为成本放大系数参与选择：score = total_cost * multiplier，越小越优先。
func ChannelGetKeyForModel(channelID int, modelName string) model.ChannelKey {
	ch, ok := channelCache.Get(channelID)
	if !ok || len(ch.Keys) == 0 {
		return model.ChannelKey{}
	}

	nowSec := time.Now().Unix()

	// 筛选可用 Key
	var eligible []model.ChannelKey
	for _, k := range ch.Keys {
		if !k.Enabled || k.ChannelKey == "" {
			continue
		}
		if !k.SupportsModel(modelName) {
			continue
		}
		if k.StatusCode == 429 && k.LastUseTimeStamp > 0 {
			if nowSec-k.LastUseTimeStamp < int64(5*time.Minute/time.Second) {
				continue
			}
		}
		eligible = append(eligible, k)
	}

	if len(eligible) == 0 {
		return model.ChannelKey{}
	}

	// 找出最低成本
	minScore := eligible[0].SelectionScore()
	for _, k := range eligible[1:] {
		if k.SelectionScore() < minScore {
			minScore = k.SelectionScore()
		}
	}

	// 收集所有同最低成本的 Key
	var tied []model.ChannelKey
	for _, k := range eligible {
		if k.SelectionScore() == minScore {
			tied = append(tied, k)
		}
	}

	// 单个 Key 直接返回
	if len(tied) == 1 {
		return tied[0]
	}

	// 多个同成本 Key：原子计数器轮询
	counterPtr, _ := channelKeyRotationCounters.LoadOrStore(channelID, new(uint64))
	idx := atomic.AddUint64(counterPtr.(*uint64), 1) % uint64(len(tied))
	return tied[idx]
}

func ChannelList(ctx context.Context) ([]model.Channel, error) {
	channels := make([]model.Channel, 0, channelCache.Len())
	for _, channel := range channelCache.GetAll() {
		channels = append(channels, channel)
	}
	return channels, nil
}

func ChannelCreate(channel *model.Channel, ctx context.Context) error {
	normalizeChannelBeforeSave(channel)
	if err := db.GetDB().WithContext(ctx).Create(channel).Error; err != nil {
		return err
	}
	channelCache.Set(channel.ID, *channel)
	for _, k := range channel.Keys {
		if k.ID != 0 {
			channelKeyCache.Set(k.ID, k)
		}
	}
	return nil
}

// ChannelKeyUpdate 仅更新 ChannelKey 的内存缓存（不落库），并标记为需要在 SaveCache 时写入数据库。
func ChannelKeyUpdate(key model.ChannelKey) error {
	if key.ID == 0 || key.ChannelID == 0 {
		return fmt.Errorf("invalid channel key")
	}
	ch, ok := channelCache.Get(key.ChannelID)
	if !ok {
		return fmt.Errorf("channel not found")
	}
	if len(ch.Keys) > 0 {
		keys := make([]model.ChannelKey, len(ch.Keys))
		copy(keys, ch.Keys)
		for i := range keys {
			if keys[i].ID == key.ID {
				keys[i] = key
				break
			}
		}
		ch.Keys = keys
	}
	channelCache.Set(key.ChannelID, ch)
	channelKeyCache.Set(key.ID, key)
	channelKeyCacheNeedUpdateLock.Lock()
	channelKeyCacheNeedUpdate[key.ID] = struct{}{}
	channelKeyCacheNeedUpdateLock.Unlock()
	return nil
}
func ChannelBaseUrlUpdate(channelID int, baseUrl []model.BaseUrl) error {
	ch, ok := channelCache.Get(channelID)
	if !ok {
		return fmt.Errorf("channel not found")
	}
	// Copy to decouple callers from internal cache storage.
	if baseUrl == nil {
		ch.BaseUrls = nil
	} else {
		cp := make([]model.BaseUrl, len(baseUrl))
		copy(cp, baseUrl)
		ch.BaseUrls = cp
	}
	channelCache.Set(channelID, ch)
	return nil
}

// ChannelKeySaveDB 将运行时更新过的 ChannelKey 缓存写入数据库。
func ChannelKeySaveDB(ctx context.Context) error {
	channelKeyCacheNeedUpdateLock.Lock()
	keyIDs := make([]int, 0, len(channelKeyCacheNeedUpdate))
	for id := range channelKeyCacheNeedUpdate {
		keyIDs = append(keyIDs, id)
	}
	channelKeyCacheNeedUpdateLock.Unlock()

	if len(keyIDs) == 0 {
		return nil
	}

	// 收集所有需要更新的 Key
	var keys []model.ChannelKey
	for _, id := range keyIDs {
		if k, ok := channelKeyCache.Get(id); ok {
			keys = append(keys, k)
		}
	}

	if len(keys) == 0 {
		return nil
	}

	// 使用事务批量写入，减少磁盘 fsync 次数
	err := db.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range keys {
			if err := tx.Save(&keys[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		log.Errorf("failed to batch save channel keys: %v", err)
		return err
	}

	// 成功后清空 dirty set
	channelKeyCacheNeedUpdateLock.Lock()
	for _, id := range keyIDs {
		delete(channelKeyCacheNeedUpdate, id)
	}
	channelKeyCacheNeedUpdateLock.Unlock()

	return nil
}

func ChannelUpdate(req *model.ChannelUpdateRequest, ctx context.Context) (*model.Channel, error) {
	_, ok := channelCache.Get(req.ID)
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}

	tx := db.GetDB().WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var selectFields []string
	updates := model.Channel{ID: req.ID}

	if req.Name != nil {
		selectFields = append(selectFields, "name")
		updates.Name = *req.Name
	}
	if req.Type != nil {
		selectFields = append(selectFields, "type")
		updates.Type = *req.Type
	}
	if req.ProviderID != nil {
		selectFields = append(selectFields, "provider_id")
		updates.ProviderID = *req.ProviderID
	}
	if req.OfficialURL != nil {
		selectFields = append(selectFields, "official_url")
		updates.OfficialURL = *req.OfficialURL
	}
	if req.Type != nil && req.ProviderID == nil {
		// type 更新但 provider_id 未显式提供，从 type 推导 provider_id
		if pid := provider.ResolveProviderIDFromType(*req.Type); pid != "" {
			selectFields = append(selectFields, "provider_id")
			updates.ProviderID = string(pid)
		}
	}
	if req.Enabled != nil {
		selectFields = append(selectFields, "enabled")
		updates.Enabled = *req.Enabled
	}
	if req.BaseUrls != nil {
		selectFields = append(selectFields, "base_urls")
		updates.BaseUrls = *req.BaseUrls
	}
	if req.Model != nil {
		selectFields = append(selectFields, "model")
		updates.Model = *req.Model
	}
	if req.CustomModel != nil {
		selectFields = append(selectFields, "custom_model")
		updates.CustomModel = *req.CustomModel
	}
	if req.Proxy != nil {
		selectFields = append(selectFields, "proxy")
		updates.Proxy = *req.Proxy
	}
	if req.AutoSync != nil {
		selectFields = append(selectFields, "auto_sync")
		updates.AutoSync = *req.AutoSync
	}
	if req.AutoGroup != nil {
		selectFields = append(selectFields, "auto_group")
		updates.AutoGroup = *req.AutoGroup
	}
	if req.CustomHeader != nil {
		selectFields = append(selectFields, "custom_header")
		updates.CustomHeader = *req.CustomHeader
	}
	if req.ChannelProxy != nil {
		selectFields = append(selectFields, "channel_proxy")
		updates.ChannelProxy = req.ChannelProxy
	}
	if req.ParamOverride != nil {
		selectFields = append(selectFields, "param_override")
		updates.ParamOverride = req.ParamOverride
	}
	if req.UsageQuery != nil {
		selectFields = append(selectFields, "usage_query")
		updates.UsageQuery = normalizeUsageQuery(*req.UsageQuery)
	}
	if req.MatchRegex != nil {
		selectFields = append(selectFields, "match_regex")
		updates.MatchRegex = req.MatchRegex
	}

	// 只有当有字段需要更新时才执行 UPDATE
	if len(selectFields) > 0 {
		if err := tx.Model(&model.Channel{}).Where("id = ?", req.ID).Select(selectFields).Updates(&updates).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to update channel: %w", err)
		}
	}

	// 删除 keys
	if len(req.KeysToDelete) > 0 {
		if err := tx.Where("id IN ? AND channel_id = ?", req.KeysToDelete, req.ID).Delete(&model.ChannelKey{}).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to delete channel keys: %w", err)
		}
		// 联动删除 Codex 用量卡片（account 格式 codex:{channelID}:{keyID}）
		for _, keyID := range req.KeysToDelete {
			account := fmt.Sprintf("codex:%d:%d", req.ID, keyID)
			if card, ok := UsageCardGetByAccount(account); ok {
				if err := tx.Delete(&model.UsageCard{}, card.ID).Error; err != nil {
					// 用量卡片删除失败不影响主流程，仅记录
					log.Errorf("failed to delete usage card %d for key %d: %v", card.ID, keyID, err)
				} else {
					usageCardCache.Del(card.ID)
				}
			}
		}
	}

	// 更新 keys（逐条，只更新提供的字段）
	if len(req.KeysToUpdate) > 0 {
		for _, ku := range req.KeysToUpdate {
			updates := map[string]interface{}{}
			if ku.Enabled != nil {
				updates["enabled"] = *ku.Enabled
			}
			if ku.ChannelKey != nil {
				updates["channel_key"] = *ku.ChannelKey
			}
			if ku.IsCLI != nil {
				updates["is_cli"] = *ku.IsCLI
			}
			if ku.Multiplier != nil {
				updates["multiplier"] = normalizeMultiplier(*ku.Multiplier)
			}
			if ku.Models != nil {
				updates["models"] = *ku.Models
			}
			if ku.Remark != nil {
				updates["remark"] = *ku.Remark
			}
			if len(updates) == 0 {
				continue
			}
			if err := tx.Model(&model.ChannelKey{}).
				Where("id = ? AND channel_id = ?", ku.ID, req.ID).
				Updates(updates).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to update channel key %d: %w", ku.ID, err)
			}
		}
	}

	// 新增 keys
	if len(req.KeysToAdd) > 0 {
		newKeys := make([]model.ChannelKey, 0, len(req.KeysToAdd))
		for _, ka := range req.KeysToAdd {
			newKeys = append(newKeys, model.ChannelKey{
				ChannelID:  req.ID,
				Enabled:    ka.Enabled,
				ChannelKey: ka.ChannelKey,
				IsCLI:      ka.IsCLI,
				Multiplier: normalizeMultiplier(ka.Multiplier),
				Models:     ka.Models,
				Remark:     ka.Remark,
			})
		}
		if err := tx.Create(&newKeys).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create channel keys: %w", err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 刷新缓存并返回最新数据
	if err := channelRefreshCacheByID(req.ID, ctx); err != nil {
		return nil, err
	}

	channel, _ := channelCache.Get(req.ID)
	return &channel, nil
}

func normalizeChannelBeforeSave(channel *model.Channel) {
	if channel == nil {
		return
	}
	channel.UsageQuery = normalizeUsageQuery(channel.UsageQuery)
	for i := range channel.Keys {
		channel.Keys[i].Multiplier = normalizeMultiplier(channel.Keys[i].Multiplier)
	}
}

func normalizeUsageQuery(in model.UsageQueryConfig) model.UsageQueryConfig {
	if in.Preset == "" {
		in.Preset = model.UsageQueryPresetCustom
	}
	if in.Method == "" {
		in.Method = "GET"
	}
	if in.TimeoutSec <= 0 {
		in.TimeoutSec = 30
	}
	if in.IntervalMin < 0 {
		in.IntervalMin = 0
	}
	return in
}

func normalizeMultiplier(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}

func ChannelEnabled(id int, enabled bool, ctx context.Context) error {
	oldChannel, ok := channelCache.Get(id)
	if !ok {
		return fmt.Errorf("channel not found")
	}
	if err := db.GetDB().WithContext(ctx).Model(&model.Channel{}).Where("id = ?", id).Update("enabled", enabled).Error; err != nil {
		return err
	}
	oldChannel.Enabled = enabled
	channelCache.Set(id, oldChannel)
	return nil
}

func ChannelDel(id int, ctx context.Context) error {
	ch, ok := channelCache.Get(id)
	if !ok {
		return fmt.Errorf("channel not found")
	}

	// 开启事务
	tx := db.GetDB().WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取所有受影响的 GroupID，用于刷新缓存
	var affectedGroupIDs []int
	if err := tx.Model(&model.GroupItem{}).
		Where("channel_id = ?", id).
		Pluck("group_id", &affectedGroupIDs).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get affected groups: %w", err)
	}

	// 删除所有引用该渠道的 GroupItem
	if err := tx.Where("channel_id = ?", id).Delete(&model.GroupItem{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete group items: %w", err)
	}

	// 删除渠道 keys
	if err := tx.Where("channel_id = ?", id).Delete(&model.ChannelKey{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete channel keys: %w", err)
	}

	// 联动删除该渠道所有 Codex 用量卡片（account 格式 codex:{channelID}:{keyID}）
	for _, k := range ch.Keys {
		if k.ID != 0 {
			account := fmt.Sprintf("codex:%d:%d", id, k.ID)
			if card, ok := UsageCardGetByAccount(account); ok {
				if err := tx.Delete(&model.UsageCard{}, card.ID).Error; err != nil {
					log.Errorf("failed to delete usage card %d for channel %d key %d: %v", card.ID, id, k.ID, err)
				} else {
					usageCardCache.Del(card.ID)
				}
			}
		}
	}

	// 删除统计数据
	if err := tx.Where("channel_id = ?", id).Delete(&model.StatsChannel{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete channel stats: %w", err)
	}

	// 删除渠道
	if err := tx.Delete(&model.Channel{}, id).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete channel: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 删除缓存
	channelCache.Del(id)
	channelKeyRotationCounters.Delete(id)
	for _, k := range ch.Keys {
		if k.ID != 0 {
			channelKeyCache.Del(k.ID)
		}
	}
	StatsChannelDel(id)

	// 刷新受影响的分组缓存
	for _, groupID := range affectedGroupIDs {
		if err := groupRefreshCacheByID(groupID, ctx); err != nil {
			log.Warnf("failed to refresh group cache for group %d: %v", groupID, err)
		}
	}

	return nil
}

func ChannelLLMList(ctx context.Context) ([]model.LLMChannel, error) {
	models := []model.LLMChannel{}
	for _, channel := range channelCache.GetAll() {
		modelNames := xstrings.SplitTrimCompact(",", channel.Model, channel.CustomModel)
		for _, modelName := range modelNames {
			if modelName == "" {
				continue
			}
			models = append(models, model.LLMChannel{
				Name:        modelName,
				Enabled:     channel.Enabled,
				ChannelID:   channel.ID,
				ChannelName: channel.Name,
			})
		}
	}
	return models, nil
}

func ChannelGet(id int, ctx context.Context) (*model.Channel, error) {
	channel, ok := channelCache.Get(id)
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}
	return &channel, nil
}

func channelRefreshCache(ctx context.Context) error {
	channels := []model.Channel{}
	if err := db.GetDB().WithContext(ctx).
		Preload("Keys").
		Preload("Stats").
		Find(&channels).Error; err != nil {
		log.Warnf("failed to get channels: %v", err)
		return err
	}
	channelKeyCache.Clear()
	channelKeyCacheNeedUpdateLock.Lock()
	channelKeyCacheNeedUpdate = make(map[int]struct{})
	channelKeyCacheNeedUpdateLock.Unlock()
	for _, channel := range channels {
		channelCache.Set(channel.ID, channel)
		for _, k := range channel.Keys {
			if k.ID != 0 {
				channelKeyCache.Set(k.ID, k)
			}
		}
	}
	return nil
}

func channelRefreshCacheByID(id int, ctx context.Context) error {
	// 先将该渠道的脏 ChannelKey 落库，避免内存中累积的统计被 DB 旧值覆盖
	if old, ok := channelCache.Get(id); ok {
		dirtyKeyIDs := make([]int, 0)
		channelKeyCacheNeedUpdateLock.Lock()
		for _, k := range old.Keys {
			if k.ID != 0 {
				if _, dirty := channelKeyCacheNeedUpdate[k.ID]; dirty {
					dirtyKeyIDs = append(dirtyKeyIDs, k.ID)
				}
			}
		}
		channelKeyCacheNeedUpdateLock.Unlock()
		if len(dirtyKeyIDs) > 0 {
			dbConn := db.GetDB().WithContext(ctx)
			for _, keyID := range dirtyKeyIDs {
				if k, ok := channelKeyCache.Get(keyID); ok {
					if err := dbConn.Save(&k).Error; err != nil {
						log.Errorf("failed to flush dirty channel key %d before refresh: %v", keyID, err)
					}
				}
			}
			channelKeyCacheNeedUpdateLock.Lock()
			for _, keyID := range dirtyKeyIDs {
				delete(channelKeyCacheNeedUpdate, keyID)
			}
			channelKeyCacheNeedUpdateLock.Unlock()
		}
		for _, k := range old.Keys {
			if k.ID != 0 {
				channelKeyCache.Del(k.ID)
			}
		}
	}
	var channel model.Channel
	if err := db.GetDB().WithContext(ctx).
		Preload("Keys").
		Preload("Stats").
		First(&channel, id).Error; err != nil {
		return err
	}
	channelCache.Set(channel.ID, channel)
	for _, k := range channel.Keys {
		if k.ID != 0 {
			channelKeyCache.Set(k.ID, k)
		}
	}
	return nil
}
