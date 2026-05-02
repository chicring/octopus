package op

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/cache"
	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var statsDailyCache model.StatsDaily
var statsDailyCacheLock sync.RWMutex

var statsTotalCache model.StatsTotal
var statsTotalCacheLock sync.RWMutex

// hourlyKey 按 (date, hour) 唯一标识一条小时统计，
// 跨天后不同日期的同一小时槽不会互相覆盖。
type hourlyKey struct {
	Date string
	Hour int
}

var statsHourlyCache = cache.New[hourlyKey, model.StatsHourly](96)
var statsHourlyCacheNeedUpdate = make(map[hourlyKey]struct{})
var statsHourlyCacheNeedUpdateLock sync.Mutex

var statsChannelCache = cache.New[int, model.StatsChannel](16)
var statsChannelCacheNeedUpdate = make(map[int]struct{})
var statsChannelCacheNeedUpdateLock sync.Mutex
var statsChannelUpdateLock sync.Mutex // 保护 StatsChannelUpdate 的读改写原子性

var statsModelCache = cache.New[string, model.StatsModel](16)
var statsModelCacheNeedUpdate = make(map[string]struct{})
var statsModelCacheNeedUpdateLock sync.Mutex
var statsModelUpdateLock sync.Mutex // 保护 StatsModelUpdate 的读改写原子性

var statsAPIKeyCache = cache.New[int, model.StatsAPIKey](16)
var statsAPIKeyCacheLock sync.Mutex
var statsAPIKeyCacheNeedUpdate = make(map[int]struct{})
var statsAPIKeyCacheNeedUpdateLock sync.Mutex

// StatsAPIKeyDaily: 按 API Key + 日期维度的统计
type apiKeyDailyKey struct {
	APIKeyID uint
	Date     string
}

var statsAPIKeyDailyCache = cache.New[apiKeyDailyKey, model.StatsAPIKeyDaily](64)
var statsAPIKeyDailyCacheLock sync.Mutex
var statsAPIKeyDailyCacheNeedUpdate = make(map[apiKeyDailyKey]struct{})
var statsAPIKeyDailyCacheNeedUpdateLock sync.Mutex

func StatsSaveDBTask() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	log.Debugf("stats save db task started")
	startTime := time.Now()
	defer func() {
		log.Debugf("stats save db task finished, save time: %s", time.Since(startTime))
	}()
	if err := StatsSaveDB(ctx); err != nil {
		log.Errorf("stats save db error: %v", err)
	}
	if err := ChannelKeySaveDB(ctx); err != nil {
		log.Errorf("channel key save db error: %v", err)
	}
}

// StatsSaveDB 将所有内存统计快照写入数据库。
// 每个维度使用 dirty set 标记哪些条目需要刷盘，
// 仅在事务成功后才清空对应的 dirty set。
func StatsSaveDB(ctx context.Context) error {
	// 1. 采集快照（在锁内拷贝，锁外构造写入列表）
	statsTotalCacheLock.RLock()
	totalSnap := statsTotalCache
	statsTotalCacheLock.RUnlock()
	if totalSnap.ID == 0 {
		totalSnap.ID = 1
	}

	statsDailyCacheLock.RLock()
	dailySnap := statsDailyCache
	statsDailyCacheLock.RUnlock()

	// hourly: 收集所有 dirty key 对应的快照
	statsHourlyCacheNeedUpdateLock.Lock()
	hourlyDirtyKeys := make([]hourlyKey, 0, len(statsHourlyCacheNeedUpdate))
	for k := range statsHourlyCacheNeedUpdate {
		hourlyDirtyKeys = append(hourlyDirtyKeys, k)
	}
	statsHourlyCacheNeedUpdateLock.Unlock()

	hourlySnaps := make([]model.StatsHourly, 0, len(hourlyDirtyKeys))
	for _, k := range hourlyDirtyKeys {
		if h, ok := statsHourlyCache.Get(k); ok {
			hourlySnaps = append(hourlySnaps, h)
		}
	}

	statsChannelCacheNeedUpdateLock.Lock()
	channelIDs := make([]int, 0, len(statsChannelCacheNeedUpdate))
	for id := range statsChannelCacheNeedUpdate {
		channelIDs = append(channelIDs, id)
	}
	statsChannelCacheNeedUpdateLock.Unlock()

	statsModelCacheNeedUpdateLock.Lock()
	modelNames := make([]string, 0, len(statsModelCacheNeedUpdate))
	for name := range statsModelCacheNeedUpdate {
		modelNames = append(modelNames, name)
	}
	statsModelCacheNeedUpdateLock.Unlock()

	statsAPIKeyCacheNeedUpdateLock.Lock()
	apiKeyIDs := make([]int, 0, len(statsAPIKeyCacheNeedUpdate))
	for id := range statsAPIKeyCacheNeedUpdate {
		apiKeyIDs = append(apiKeyIDs, id)
	}
	statsAPIKeyCacheNeedUpdateLock.Unlock()

	statsAPIKeyDailyCacheNeedUpdateLock.Lock()
	apiKeyDailyKeys := make([]apiKeyDailyKey, 0, len(statsAPIKeyDailyCacheNeedUpdate))
	for k := range statsAPIKeyDailyCacheNeedUpdate {
		apiKeyDailyKeys = append(apiKeyDailyKeys, k)
	}
	statsAPIKeyDailyCacheNeedUpdateLock.Unlock()

	err := persistStatsSnapshots(ctx, totalSnap, dailySnap, hourlySnaps, channelIDs, modelNames, apiKeyIDs, apiKeyDailyKeys)

	// 仅在持久化成功后清空 dirty set
	if err == nil {
		statsHourlyCacheNeedUpdateLock.Lock()
		for _, k := range hourlyDirtyKeys {
			delete(statsHourlyCacheNeedUpdate, k)
		}
		statsHourlyCacheNeedUpdateLock.Unlock()

		statsChannelCacheNeedUpdateLock.Lock()
		for _, id := range channelIDs {
			delete(statsChannelCacheNeedUpdate, id)
		}
		statsChannelCacheNeedUpdateLock.Unlock()

		statsModelCacheNeedUpdateLock.Lock()
		for _, name := range modelNames {
			delete(statsModelCacheNeedUpdate, name)
		}
		statsModelCacheNeedUpdateLock.Unlock()

		statsAPIKeyCacheNeedUpdateLock.Lock()
		for _, id := range apiKeyIDs {
			delete(statsAPIKeyCacheNeedUpdate, id)
		}
		statsAPIKeyCacheNeedUpdateLock.Unlock()

		statsAPIKeyDailyCacheNeedUpdateLock.Lock()
		for _, k := range apiKeyDailyKeys {
			delete(statsAPIKeyDailyCacheNeedUpdate, k)
		}
		statsAPIKeyDailyCacheNeedUpdateLock.Unlock()
	}

	return err
}

// persistStatsSnapshots 在单个事务内写入所有统计快照。
// 所有数据在进入事务前已构造完毕，事务只负责写入。
func persistStatsSnapshots(
	ctx context.Context,
	totalSnap model.StatsTotal,
	dailySnap model.StatsDaily,
	hourlySnaps []model.StatsHourly,
	channelIDs []int,
	modelNames []string,
	apiKeyIDs []int,
	apiKeyDailyKeys []apiKeyDailyKey,
) error {
	return db.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if result := tx.Save(&totalSnap); result.Error != nil {
			return result.Error
		}
		if result := tx.Save(&dailySnap); result.Error != nil {
			return result.Error
		}

		// 写入所有 dirty hourly（不再只写当天）
		if len(hourlySnaps) > 0 {
			if result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
				UpdateAll: true,
			}).Create(&hourlySnaps); result.Error != nil {
				return result.Error
			}
		}

		// 批量收集 Channel 统计
		if len(channelIDs) > 0 {
			var channels []model.StatsChannel
			for _, id := range channelIDs {
				if ch, ok := statsChannelCache.Get(id); ok {
					channels = append(channels, ch)
				}
			}
			if len(channels) > 0 {
				if result := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "channel_id"}},
					UpdateAll: true,
				}).Create(&channels); result.Error != nil {
					return result.Error
				}
			}
		}

		// 批量收集 Model 统计
		if len(modelNames) > 0 {
			var models []model.StatsModel
			for _, name := range modelNames {
				if m, ok := statsModelCache.Get(name); ok {
					models = append(models, m)
				}
			}
			if len(models) > 0 {
				if result := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "name"}},
					UpdateAll: true,
				}).Create(&models); result.Error != nil {
					return result.Error
				}
			}
		}

		// 批量收集 APIKey 统计
		if len(apiKeyIDs) > 0 {
			var apiKeys []model.StatsAPIKey
			for _, id := range apiKeyIDs {
				if ak, ok := statsAPIKeyCache.Get(id); ok {
					apiKeys = append(apiKeys, ak)
				}
			}
			if len(apiKeys) > 0 {
				if result := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "api_key_id"}},
					UpdateAll: true,
				}).Create(&apiKeys); result.Error != nil {
					return result.Error
				}
			}
		}

		// 批量收集 APIKeyDaily 统计
		if len(apiKeyDailyKeys) > 0 {
			var apiKeyDailies []model.StatsAPIKeyDaily
			for _, k := range apiKeyDailyKeys {
				if akd, ok := statsAPIKeyDailyCache.Get(k); ok {
					apiKeyDailies = append(apiKeyDailies, akd)
				}
			}
			if len(apiKeyDailies) > 0 {
				if result := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "api_key_id"}, {Name: "date"}},
					UpdateAll: true,
				}).Create(&apiKeyDailies); result.Error != nil {
					return result.Error
				}
			}
		}

		return nil
	})
}

func StatsDailyUpdate(ctx context.Context, metrics model.StatsMetrics) error {
	today := time.Now().Format("20060102")

	statsDailyCacheLock.Lock()
	if statsDailyCache.Date == today {
		statsDailyCache.StatsMetrics.Add(metrics)
		statsDailyCacheLock.Unlock()
		return nil
	}

	// 日期变更：保存旧日数据，重置当天缓存
	prevDaily := statsDailyCache
	statsDailyCache = model.StatsDaily{Date: today}
	statsDailyCache.StatsMetrics.Add(metrics)
	statsDailyCacheLock.Unlock()

	// 异步刷盘旧日数据（包括旧日的 hourly 和 APIKeyDaily）
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := statsSaveDBWithDailyOverride(bgCtx, prevDaily); err != nil {
			log.Errorf("async save previous daily stats failed: %v", err)
		}
	}()

	return nil
}

func StatsTotalUpdate(metrics model.StatsMetrics) error {
	statsTotalCacheLock.Lock()
	defer statsTotalCacheLock.Unlock()
	if statsTotalCache.ID == 0 {
		statsTotalCache.ID = 1
	}
	statsTotalCache.StatsMetrics.Add(metrics)
	return nil
}

func StatsChannelUpdate(channelID int, metrics model.StatsMetrics) error {
	statsChannelUpdateLock.Lock()
	defer statsChannelUpdateLock.Unlock()

	channelCache, ok := statsChannelCache.Get(channelID)
	if !ok {
		channelCache = model.StatsChannel{
			ChannelID: channelID,
		}
	}
	channelCache.StatsMetrics.Add(metrics)
	statsChannelCache.Set(channelID, channelCache)
	statsChannelCacheNeedUpdateLock.Lock()
	statsChannelCacheNeedUpdate[channelID] = struct{}{}
	statsChannelCacheNeedUpdateLock.Unlock()
	return nil
}

func StatsHourlyUpdate(metrics model.StatsMetrics) error {
	now := time.Now()
	nowHour := now.Hour()
	todayDate := time.Now().Format("20060102")
	key := hourlyKey{Date: todayDate, Hour: nowHour}

	h, ok := statsHourlyCache.Get(key)
	if !ok {
		h = model.StatsHourly{
			Hour: nowHour,
			Date: todayDate,
		}
	}
	h.StatsMetrics.Add(metrics)
	statsHourlyCache.Set(key, h)

	statsHourlyCacheNeedUpdateLock.Lock()
	statsHourlyCacheNeedUpdate[key] = struct{}{}
	statsHourlyCacheNeedUpdateLock.Unlock()
	return nil
}

func StatsModelUpdate(modelName string, metrics model.StatsMetrics) error {
	statsModelUpdateLock.Lock()
	defer statsModelUpdateLock.Unlock()

	modelCache, ok := statsModelCache.Get(modelName)
	if !ok {
		modelCache = model.StatsModel{
			Name: modelName,
		}
	}
	modelCache.StatsMetrics.Add(metrics)
	statsModelCache.Set(modelName, modelCache)
	statsModelCacheNeedUpdateLock.Lock()
	statsModelCacheNeedUpdate[modelName] = struct{}{}
	statsModelCacheNeedUpdateLock.Unlock()
	return nil
}

func StatsModelList() []model.StatsModel {
	models := make([]model.StatsModel, 0, statsModelCache.Len())
	for _, v := range statsModelCache.GetAll() {
		models = append(models, v)
	}
	return models
}

func StatsAPIKeyUpdate(apiKeyID int, metrics model.StatsMetrics) error {
	statsAPIKeyCacheLock.Lock()
	defer statsAPIKeyCacheLock.Unlock()

	apiKeyCache, ok := statsAPIKeyCache.Get(apiKeyID)
	if !ok {
		apiKeyCache = model.StatsAPIKey{
			APIKeyID: apiKeyID,
		}
	}
	apiKeyCache.StatsMetrics.Add(metrics)
	statsAPIKeyCache.Set(apiKeyID, apiKeyCache)
	statsAPIKeyCacheNeedUpdateLock.Lock()
	statsAPIKeyCacheNeedUpdate[apiKeyID] = struct{}{}
	statsAPIKeyCacheNeedUpdateLock.Unlock()
	return nil
}

// StatsAPIKeyDailyUpdate 更新指定 API Key 当天的每日统计
func StatsAPIKeyDailyUpdate(apiKeyID uint, metrics model.StatsMetrics) {
	statsAPIKeyDailyCacheLock.Lock()
	defer statsAPIKeyDailyCacheLock.Unlock()

	today := time.Now().Format("20060102")
	key := apiKeyDailyKey{APIKeyID: apiKeyID, Date: today}

	akd, ok := statsAPIKeyDailyCache.Get(key)
	if !ok {
		akd = model.StatsAPIKeyDaily{APIKeyID: apiKeyID, Date: today}
	}
	akd.StatsMetrics.Add(metrics)
	statsAPIKeyDailyCache.Set(key, akd)

	statsAPIKeyDailyCacheNeedUpdateLock.Lock()
	statsAPIKeyDailyCacheNeedUpdate[key] = struct{}{}
	statsAPIKeyDailyCacheNeedUpdateLock.Unlock()
}

// StatsAPIKeyDailyGet 返回指定 API Key 的每日统计（最近 N 天），合并内存缓存中未刷盘的数据
func StatsAPIKeyDailyGet(ctx context.Context, apiKeyID uint, days int) ([]model.StatsAPIKeyDaily, error) {
	var result []model.StatsAPIKeyDaily
	err := db.GetDB().WithContext(ctx).
		Where("api_key_id = ? AND date >= ?", apiKeyID, time.Now().AddDate(0, 0, -days).Format("20060102")).
		Order("date ASC").
		Find(&result).Error
	if err != nil {
		return nil, err
	}

	// 合并内存缓存中所有未刷盘的该 API Key 数据
	statsAPIKeyDailyCacheLock.Lock()
	allCached := statsAPIKeyDailyCache.GetAll()
	statsAPIKeyDailyCacheLock.Unlock()

	for k, v := range allCached {
		if k.APIKeyID != apiKeyID {
			continue
		}
		found := false
		for i := range result {
			if result[i].Date == k.Date {
				result[i] = v
				found = true
				break
			}
		}
		if !found {
			result = append(result, v)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Date < result[j].Date
	})

	return result, nil
}

func StatsChannelDel(id int) error {
	if _, ok := statsChannelCache.Get(id); !ok {
		return nil
	}
	statsChannelCache.Del(id)
	statsChannelCacheNeedUpdateLock.Lock()
	delete(statsChannelCacheNeedUpdate, id)
	statsChannelCacheNeedUpdateLock.Unlock()
	return db.GetDB().Delete(&model.StatsChannel{}, id).Error
}

func StatsAPIKeyDel(id int) error {
	if _, ok := statsAPIKeyCache.Get(id); !ok {
		return nil
	}
	statsAPIKeyCache.Del(id)
	statsAPIKeyCacheNeedUpdateLock.Lock()
	delete(statsAPIKeyCacheNeedUpdate, id)
	statsAPIKeyCacheNeedUpdateLock.Unlock()
	return db.GetDB().Delete(&model.StatsAPIKey{}, id).Error
}

func StatsTotalGet() model.StatsTotal {
	statsTotalCacheLock.RLock()
	defer statsTotalCacheLock.RUnlock()
	return statsTotalCache
}

func StatsTodayGet() model.StatsDaily {
	statsDailyCacheLock.RLock()
	defer statsDailyCacheLock.RUnlock()
	return statsDailyCache
}

func StatsChannelGet(id int) model.StatsChannel {
	stats, ok := statsChannelCache.Get(id)
	if !ok {
		tmp := model.StatsChannel{
			ChannelID: id,
		}
		statsChannelCache.Set(id, tmp)
		statsChannelCacheNeedUpdateLock.Lock()
		statsChannelCacheNeedUpdate[id] = struct{}{}
		statsChannelCacheNeedUpdateLock.Unlock()
		return tmp
	}
	return stats
}

func StatsAPIKeyGet(id int) model.StatsAPIKey {
	stats, ok := statsAPIKeyCache.Get(id)
	if !ok {
		tmp := model.StatsAPIKey{
			APIKeyID: id,
		}
		statsAPIKeyCache.Set(id, tmp)
		statsAPIKeyCacheNeedUpdateLock.Lock()
		statsAPIKeyCacheNeedUpdate[id] = struct{}{}
		statsAPIKeyCacheNeedUpdateLock.Unlock()
		return stats
	}
	return stats
}

func StatsAPIKeyList() []model.StatsAPIKey {
	apiKeys := make([]model.StatsAPIKey, 0, statsAPIKeyCache.Len())
	for _, v := range statsAPIKeyCache.GetAll() {
		apiKeys = append(apiKeys, v)
	}
	return apiKeys
}

// StatsHourlyGet 返回当天 0..currentHour 的统计数据，
// 从 hourly map cache 查询，不存在则返回零值。
func StatsHourlyGet() []model.StatsHourly {
	now := time.Now()
	currentHour := now.Hour()
	todayDate := time.Now().Format("20060102")

	result := make([]model.StatsHourly, 0, currentHour+1)

	for hour := 0; hour <= currentHour; hour++ {
		key := hourlyKey{Date: todayDate, Hour: hour}
		if h, ok := statsHourlyCache.Get(key); ok {
			result = append(result, h)
		} else {
			result = append(result, model.StatsHourly{
				Hour: hour,
				Date: todayDate,
			})
		}
	}

	return result
}

// StatsGetDaily 返回所有历史 Daily 统计，并合并内存中当天未刷盘的数据。
func StatsGetDaily(ctx context.Context) ([]model.StatsDaily, error) {
	var statsDaily []model.StatsDaily
	result := db.GetDB().WithContext(ctx).Order("date ASC").Find(&statsDaily)
	if result.Error != nil {
		return nil, result.Error
	}

	// 合并内存缓存中当天未刷盘的数据
	statsDailyCacheLock.RLock()
	cachedDaily := statsDailyCache
	statsDailyCacheLock.RUnlock()

	if cachedDaily.Date != "" {
		found := false
		for i := range statsDaily {
			if statsDaily[i].Date == cachedDaily.Date {
				statsDaily[i] = cachedDaily
				found = true
				break
			}
		}
		if !found {
			statsDaily = append(statsDaily, cachedDaily)
			sort.Slice(statsDaily, func(i, j int) bool {
				return statsDaily[i].Date < statsDaily[j].Date
			})
		}
	}

	return statsDaily, nil
}

// statsSaveDBWithDailyOverride 用于跨天异步刷盘旧日数据。
// 它将所有 dirty 维度一起写入，确保旧日的 hourly/APIKeyDaily 不遗漏。
func statsSaveDBWithDailyOverride(ctx context.Context, dailyOverride model.StatsDaily) error {
	statsTotalCacheLock.RLock()
	totalSnap := statsTotalCache
	statsTotalCacheLock.RUnlock()
	if totalSnap.ID == 0 {
		totalSnap.ID = 1
	}

	// 收集所有 dirty hourly
	statsHourlyCacheNeedUpdateLock.Lock()
	hourlyDirtyKeys := make([]hourlyKey, 0, len(statsHourlyCacheNeedUpdate))
	for k := range statsHourlyCacheNeedUpdate {
		hourlyDirtyKeys = append(hourlyDirtyKeys, k)
	}
	statsHourlyCacheNeedUpdateLock.Unlock()

	hourlySnaps := make([]model.StatsHourly, 0, len(hourlyDirtyKeys))
	for _, k := range hourlyDirtyKeys {
		if h, ok := statsHourlyCache.Get(k); ok {
			hourlySnaps = append(hourlySnaps, h)
		}
	}

	// 收集需要持久化的 key
	statsChannelCacheNeedUpdateLock.Lock()
	channelIDs := make([]int, 0, len(statsChannelCacheNeedUpdate))
	for id := range statsChannelCacheNeedUpdate {
		channelIDs = append(channelIDs, id)
	}
	statsChannelCacheNeedUpdateLock.Unlock()

	statsModelCacheNeedUpdateLock.Lock()
	modelNames := make([]string, 0, len(statsModelCacheNeedUpdate))
	for name := range statsModelCacheNeedUpdate {
		modelNames = append(modelNames, name)
	}
	statsModelCacheNeedUpdateLock.Unlock()

	statsAPIKeyCacheNeedUpdateLock.Lock()
	apiKeyIDs := make([]int, 0, len(statsAPIKeyCacheNeedUpdate))
	for id := range statsAPIKeyCacheNeedUpdate {
		apiKeyIDs = append(apiKeyIDs, id)
	}
	statsAPIKeyCacheNeedUpdateLock.Unlock()

	statsAPIKeyDailyCacheNeedUpdateLock.Lock()
	apiKeyDailyKeys := make([]apiKeyDailyKey, 0, len(statsAPIKeyDailyCacheNeedUpdate))
	for k := range statsAPIKeyDailyCacheNeedUpdate {
		apiKeyDailyKeys = append(apiKeyDailyKeys, k)
	}
	statsAPIKeyDailyCacheNeedUpdateLock.Unlock()

	err := persistStatsSnapshots(ctx, totalSnap, dailyOverride, hourlySnaps, channelIDs, modelNames, apiKeyIDs, apiKeyDailyKeys)

	// 仅在持久化成功后清空 dirty set
	if err == nil {
		statsHourlyCacheNeedUpdateLock.Lock()
		for _, k := range hourlyDirtyKeys {
			delete(statsHourlyCacheNeedUpdate, k)
		}
		statsHourlyCacheNeedUpdateLock.Unlock()

		statsChannelCacheNeedUpdateLock.Lock()
		for _, id := range channelIDs {
			delete(statsChannelCacheNeedUpdate, id)
		}
		statsChannelCacheNeedUpdateLock.Unlock()

		statsModelCacheNeedUpdateLock.Lock()
		for _, name := range modelNames {
			delete(statsModelCacheNeedUpdate, name)
		}
		statsModelCacheNeedUpdateLock.Unlock()

		statsAPIKeyCacheNeedUpdateLock.Lock()
		for _, id := range apiKeyIDs {
			delete(statsAPIKeyCacheNeedUpdate, id)
		}
		statsAPIKeyCacheNeedUpdateLock.Unlock()

		statsAPIKeyDailyCacheNeedUpdateLock.Lock()
		for _, k := range apiKeyDailyKeys {
			delete(statsAPIKeyDailyCacheNeedUpdate, k)
		}
		statsAPIKeyDailyCacheNeedUpdateLock.Unlock()
	}

	return err
}

func statsRefreshCache(ctx context.Context) error {
	dbConn := db.GetDB().WithContext(ctx)
	today := time.Now().Format("20060102")

	// Daily: 精确查当天，不再用 Last()
	var loadedDaily model.StatsDaily
	result := dbConn.Where("date = ?", today).First(&loadedDaily)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get daily stats: %v", result.Error)
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		loadedDaily = model.StatsDaily{Date: today}
	}

	var loadedTotal model.StatsTotal
	result = dbConn.First(&loadedTotal)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get total stats: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		loadedTotal = model.StatsTotal{ID: 1}
	} else if loadedTotal.ID == 0 {
		loadedTotal.ID = 1
	}

	var loadedChannels []model.StatsChannel
	result = dbConn.Find(&loadedChannels)
	if result.Error != nil {
		return fmt.Errorf("failed to get channels: %v", result.Error)
	}

	// Hourly: 加载最近 35 天的所有小时数据到内存缓存
	var loadedHourly []model.StatsHourly
	result = dbConn.Where("date >= ?", time.Now().AddDate(0, 0, -35).Format("20060102")).Find(&loadedHourly)
	if result.Error != nil {
		return fmt.Errorf("failed to get hourly stats: %v", result.Error)
	}

	statsDailyCacheLock.Lock()
	statsDailyCache = loadedDaily
	statsDailyCacheLock.Unlock()

	statsTotalCacheLock.Lock()
	statsTotalCache = loadedTotal
	statsTotalCacheLock.Unlock()

	statsChannelCache.Clear()
	statsChannelCacheNeedUpdateLock.Lock()
	statsChannelCacheNeedUpdate = make(map[int]struct{})
	statsChannelCacheNeedUpdateLock.Unlock()
	for _, v := range loadedChannels {
		statsChannelCache.Set(v.ChannelID, v)
	}

	var loadedAPIKeys []model.StatsAPIKey
	result = dbConn.Find(&loadedAPIKeys)
	if result.Error != nil {
		return fmt.Errorf("failed to get api key stats: %v", result.Error)
	}

	statsAPIKeyCache.Clear()
	statsAPIKeyCacheNeedUpdateLock.Lock()
	statsAPIKeyCacheNeedUpdate = make(map[int]struct{})
	statsAPIKeyCacheNeedUpdateLock.Unlock()
	for _, v := range loadedAPIKeys {
		statsAPIKeyCache.Set(v.APIKeyID, v)
	}

	// Hourly: 填充 map cache
	statsHourlyCache.Clear()
	statsHourlyCacheNeedUpdateLock.Lock()
	statsHourlyCacheNeedUpdate = make(map[hourlyKey]struct{})
	statsHourlyCacheNeedUpdateLock.Unlock()
	for _, v := range loadedHourly {
		key := hourlyKey{Date: v.Date, Hour: v.Hour}
		statsHourlyCache.Set(key, v)
	}

	var loadedModels []model.StatsModel
	result = dbConn.Find(&loadedModels)
	if result.Error != nil {
		return fmt.Errorf("failed to get model stats: %v", result.Error)
	}

	statsModelCache.Clear()
	statsModelCacheNeedUpdateLock.Lock()
	statsModelCacheNeedUpdate = make(map[string]struct{})
	statsModelCacheNeedUpdateLock.Unlock()
	for _, v := range loadedModels {
		statsModelCache.Set(v.Name, v)
	}

	// APIKeyDaily: 加载最近 35 天的数据
	var loadedAPIKeyDailies []model.StatsAPIKeyDaily
	result = dbConn.Where("date >= ?", time.Now().AddDate(0, 0, -35).Format("20060102")).Find(&loadedAPIKeyDailies)
	if result.Error != nil {
		return fmt.Errorf("failed to get api key daily stats: %v", result.Error)
	}

	statsAPIKeyDailyCacheLock.Lock()
	statsAPIKeyDailyCache.Clear()
	for _, v := range loadedAPIKeyDailies {
		statsAPIKeyDailyCache.Set(apiKeyDailyKey{APIKeyID: v.APIKeyID, Date: v.Date}, v)
	}
	statsAPIKeyDailyCacheLock.Unlock()

	statsAPIKeyDailyCacheNeedUpdateLock.Lock()
	statsAPIKeyDailyCacheNeedUpdate = make(map[apiKeyDailyKey]struct{})
	statsAPIKeyDailyCacheNeedUpdateLock.Unlock()

	return nil
}
