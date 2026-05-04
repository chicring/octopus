package op

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/db/migrate"
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

// StatsAPIKeyHourly: 按 API Key + 日期 + 小时维度的统计
type apiKeyHourlyKey struct {
	APIKeyID uint
	Date     string
	Hour     int
}

var statsAPIKeyHourlyCache = cache.New[apiKeyHourlyKey, model.StatsAPIKeyHourly](128)
var statsAPIKeyHourlyCacheLock sync.Mutex
var statsAPIKeyHourlyCacheNeedUpdate = make(map[apiKeyHourlyKey]struct{})
var statsAPIKeyHourlyCacheNeedUpdateLock sync.Mutex

// statsEnsureOnce 确保 EnsureStatsCompositePK 在第一次 StatsSaveDBTask 时执行
var statsEnsureOnce sync.Once

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

func ensureStatsPersistenceSchema() {
	statsEnsureOnce.Do(func() {
		migrate.EnsureStatsModelSchema(db.GetDB())
		migrate.EnsureStatsCompositePK(db.GetDB())
	})
}

// StatsSaveDB 将所有内存统计快照写入数据库。
// StatsTotal/StatsDaily/StatsChannel/StatsModel/StatsAPIKey 无条件全量写入，
// 确保重启后数据不丢失；Hourly/APIKeyDaily/APIKeyHourly 使用 dirty set 按需写入。
func StatsSaveDB(ctx context.Context) error {
	ensureStatsPersistenceSchema()
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

	// Channel/Model/APIKey: 无条件全量快照（累计总量，数据量小，防止丢失）
	allChannels := statsChannelCache.GetAll()
	channelSnaps := make([]model.StatsChannel, 0, len(allChannels))
	for _, v := range allChannels {
		channelSnaps = append(channelSnaps, v)
	}

	allModels := statsModelCache.GetAll()
	modelSnaps := make([]model.StatsModel, 0, len(allModels))
	for _, v := range allModels {
		modelSnaps = append(modelSnaps, v)
	}

	allAPIKeys := statsAPIKeyCache.GetAll()
	apiKeySnaps := make([]model.StatsAPIKey, 0, len(allAPIKeys))
	for _, v := range allAPIKeys {
		apiKeySnaps = append(apiKeySnaps, v)
	}

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

	statsAPIKeyDailyCacheNeedUpdateLock.Lock()
	apiKeyDailyKeys := make([]apiKeyDailyKey, 0, len(statsAPIKeyDailyCacheNeedUpdate))
	for k := range statsAPIKeyDailyCacheNeedUpdate {
		apiKeyDailyKeys = append(apiKeyDailyKeys, k)
	}
	statsAPIKeyDailyCacheNeedUpdateLock.Unlock()

	statsAPIKeyHourlyCacheNeedUpdateLock.Lock()
	apiKeyHourlyKeys := make([]apiKeyHourlyKey, 0, len(statsAPIKeyHourlyCacheNeedUpdate))
	for k := range statsAPIKeyHourlyCacheNeedUpdate {
		apiKeyHourlyKeys = append(apiKeyHourlyKeys, k)
	}
	statsAPIKeyHourlyCacheNeedUpdateLock.Unlock()

	err := persistStatsSnapshots(ctx, totalSnap, dailySnap, hourlySnaps, channelSnaps, modelSnaps, apiKeySnaps, apiKeyDailyKeys, apiKeyHourlyKeys)

	// 仅在持久化成功后清空 dirty set
	if err == nil {
		statsHourlyCacheNeedUpdateLock.Lock()
		for _, k := range hourlyDirtyKeys {
			delete(statsHourlyCacheNeedUpdate, k)
		}
		statsHourlyCacheNeedUpdateLock.Unlock()

		statsAPIKeyDailyCacheNeedUpdateLock.Lock()
		for _, k := range apiKeyDailyKeys {
			delete(statsAPIKeyDailyCacheNeedUpdate, k)
		}
		statsAPIKeyDailyCacheNeedUpdateLock.Unlock()

		statsAPIKeyHourlyCacheNeedUpdateLock.Lock()
		for _, k := range apiKeyHourlyKeys {
			delete(statsAPIKeyHourlyCacheNeedUpdate, k)
		}
		statsAPIKeyHourlyCacheNeedUpdateLock.Unlock()
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
	channelSnaps []model.StatsChannel,
	modelSnaps []model.StatsModel,
	apiKeySnaps []model.StatsAPIKey,
	apiKeyDailyKeys []apiKeyDailyKey,
	apiKeyHourlyKeys []apiKeyHourlyKey,
) error {
	return db.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Total: upsert，使用 DoUpdates 明确指定更新列，避免零值覆盖
		if result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"input_token", "output_token", "input_cost", "output_cost",
				"wait_time", "output_time", "request_success", "request_failed",
			}),
		}).Create(&totalSnap); result.Error != nil {
			return fmt.Errorf("stats_total: %w", result.Error)
		}
		// Daily: upsert，使用 DoUpdates 明确指定更新列，避免零值覆盖
		if result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "date"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"input_token", "output_token", "input_cost", "output_cost",
				"wait_time", "output_time", "request_success", "request_failed",
			}),
		}).Create(&dailySnap); result.Error != nil {
			return fmt.Errorf("stats_daily: %w", result.Error)
		}

		// 写入所有 dirty hourly
		if len(hourlySnaps) > 0 {
			if result := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "date"}, {Name: "hour"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"input_token", "output_token", "input_cost", "output_cost",
					"wait_time", "output_time", "request_success", "request_failed",
				}),
			}).Create(&hourlySnaps); result.Error != nil {
				return fmt.Errorf("stats_hourlies: %w", result.Error)
			}
		}

		// Channel: 无条件全量 upsert（累计总量，防止丢失）
		if len(channelSnaps) > 0 {
			if result := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "channel_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"input_token", "output_token", "input_cost", "output_cost",
					"wait_time", "output_time", "request_success", "request_failed",
				}),
			}).Create(&channelSnaps); result.Error != nil {
				return fmt.Errorf("stats_channels: %w", result.Error)
			}
		}

		// Model: 无条件全量 upsert（累计总量，防止丢失）
		if len(modelSnaps) > 0 {
			if result := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "name"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"input_token", "output_token", "input_cost", "output_cost",
					"wait_time", "output_time", "request_success", "request_failed",
				}),
			}).Create(&modelSnaps); result.Error != nil {
				return fmt.Errorf("stats_models: %w", result.Error)
			}
		}

		// APIKey: 无条件全量 upsert（累计总量，防止丢失）
		if len(apiKeySnaps) > 0 {
			if result := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "api_key_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"input_token", "output_token", "input_cost", "output_cost",
					"wait_time", "output_time", "request_success", "request_failed",
				}),
			}).Create(&apiKeySnaps); result.Error != nil {
				return fmt.Errorf("stats_api_keys: %w", result.Error)
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
					Columns: []clause.Column{{Name: "api_key_id"}, {Name: "date"}},
					DoUpdates: clause.AssignmentColumns([]string{
						"input_token", "output_token", "input_cost", "output_cost",
						"wait_time", "output_time", "request_success", "request_failed",
					}),
				}).Create(&apiKeyDailies); result.Error != nil {
					return fmt.Errorf("stats_api_key_dailies: %w", result.Error)
				}
			}
		}

		// 批量收集 APIKeyHourly 统计
		if len(apiKeyHourlyKeys) > 0 {
			var apiKeyHourlies []model.StatsAPIKeyHourly
			for _, k := range apiKeyHourlyKeys {
				if akh, ok := statsAPIKeyHourlyCache.Get(k); ok {
					apiKeyHourlies = append(apiKeyHourlies, akh)
				}
			}
			if len(apiKeyHourlies) > 0 {
				if result := tx.Clauses(clause.OnConflict{
					Columns: []clause.Column{{Name: "api_key_id"}, {Name: "date"}, {Name: "hour"}},
					DoUpdates: clause.AssignmentColumns([]string{
						"input_token", "output_token", "input_cost", "output_cost",
						"wait_time", "output_time", "request_success", "request_failed",
					}),
				}).Create(&apiKeyHourlies); result.Error != nil {
					return fmt.Errorf("stats_api_key_hourlies: %w", result.Error)
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

// StatsAPIKeyHourlyUpdate 更新指定 API Key 当前小时的统计
func StatsAPIKeyHourlyUpdate(apiKeyID uint, metrics model.StatsMetrics) {
	statsAPIKeyHourlyCacheLock.Lock()
	defer statsAPIKeyHourlyCacheLock.Unlock()

	now := time.Now()
	todayDate := now.Format("20060102")
	nowHour := now.Hour()
	key := apiKeyHourlyKey{APIKeyID: apiKeyID, Date: todayDate, Hour: nowHour}

	akh, ok := statsAPIKeyHourlyCache.Get(key)
	if !ok {
		akh = model.StatsAPIKeyHourly{APIKeyID: apiKeyID, Date: todayDate, Hour: nowHour}
	}
	akh.StatsMetrics.Add(metrics)
	statsAPIKeyHourlyCache.Set(key, akh)

	statsAPIKeyHourlyCacheNeedUpdateLock.Lock()
	statsAPIKeyHourlyCacheNeedUpdate[key] = struct{}{}
	statsAPIKeyHourlyCacheNeedUpdateLock.Unlock()
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

// StatsAPIKeyHourlyGet 返回指定 API Key 当天的小时统计，合并内存缓存中未刷盘的数据
func StatsAPIKeyHourlyGet(ctx context.Context, apiKeyID uint) ([]model.StatsAPIKeyHourly, error) {
	now := time.Now()
	currentHour := now.Hour()
	todayDate := now.Format("20060102")

	var result []model.StatsAPIKeyHourly
	err := db.GetDB().WithContext(ctx).
		Where("api_key_id = ? AND date = ?", apiKeyID, todayDate).
		Order("hour ASC").
		Find(&result).Error
	if err != nil {
		return nil, err
	}

	// 合并内存缓存中所有未刷盘的该 API Key 当天数据
	statsAPIKeyHourlyCacheLock.Lock()
	allCached := statsAPIKeyHourlyCache.GetAll()
	statsAPIKeyHourlyCacheLock.Unlock()

	dbMap := make(map[int]int) // hour -> index in result
	for i, r := range result {
		dbMap[r.Hour] = i
	}

	for k, v := range allCached {
		if k.APIKeyID != apiKeyID || k.Date != todayDate {
			continue
		}
		if idx, ok := dbMap[k.Hour]; ok {
			result[idx] = v
		} else {
			result = append(result, v)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Hour < result[j].Hour
	})

	// 补齐 0..currentHour 的空槽
	full := make([]model.StatsAPIKeyHourly, 0, currentHour+1)
	resultMap := make(map[int]model.StatsAPIKeyHourly)
	for _, r := range result {
		resultMap[r.Hour] = r
	}
	for hour := 0; hour <= currentHour; hour++ {
		if r, ok := resultMap[hour]; ok {
			full = append(full, r)
		} else {
			full = append(full, model.StatsAPIKeyHourly{
				APIKeyID: apiKeyID,
				Date:     todayDate,
				Hour:     hour,
			})
		}
	}

	return full, nil
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

// StatsAPIKeyList 返回所有 API Key 的统计列表。
// 先从数据库加载所有 API Key，再合并内存缓存中的统计数据。
func StatsAPIKeyList() []model.StatsAPIKey {
	// 从内存缓存获取已有统计
	cachedMap := make(map[int]model.StatsAPIKey)
	for _, v := range statsAPIKeyCache.GetAll() {
		cachedMap[v.APIKeyID] = v
	}

	// 从数据库获取所有 API Key，确保没有遗漏
	var allKeys []model.APIKey
	db.GetDB().Find(&allKeys)

	result := make([]model.StatsAPIKey, 0, len(allKeys))
	for _, k := range allKeys {
		if cached, ok := cachedMap[k.ID]; ok {
			result = append(result, cached)
		} else {
			// 数据库中有但缓存中没有的 key，返回零值统计
			result = append(result, model.StatsAPIKey{APIKeyID: k.ID})
		}
	}
	return result
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

// statsSaveDBWithDailyOverride 与 StatsSaveDB 相同，但用 dailyOverride 替代当前 daily 快照。
// 用于跨天时异步保存前一天的 daily 数据。
func statsSaveDBWithDailyOverride(ctx context.Context, dailyOverride model.StatsDaily) error {
	ensureStatsPersistenceSchema()

	statsTotalCacheLock.RLock()
	totalSnap := statsTotalCache
	statsTotalCacheLock.RUnlock()
	if totalSnap.ID == 0 {
		totalSnap.ID = 1
	}

	// Channel/Model/APIKey: 无条件全量快照
	allChannels := statsChannelCache.GetAll()
	channelSnaps := make([]model.StatsChannel, 0, len(allChannels))
	for _, v := range allChannels {
		channelSnaps = append(channelSnaps, v)
	}

	allModels := statsModelCache.GetAll()
	modelSnaps := make([]model.StatsModel, 0, len(allModels))
	for _, v := range allModels {
		modelSnaps = append(modelSnaps, v)
	}

	allAPIKeys := statsAPIKeyCache.GetAll()
	apiKeySnaps := make([]model.StatsAPIKey, 0, len(allAPIKeys))
	for _, v := range allAPIKeys {
		apiKeySnaps = append(apiKeySnaps, v)
	}

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

	statsAPIKeyDailyCacheNeedUpdateLock.Lock()
	apiKeyDailyKeys := make([]apiKeyDailyKey, 0, len(statsAPIKeyDailyCacheNeedUpdate))
	for k := range statsAPIKeyDailyCacheNeedUpdate {
		apiKeyDailyKeys = append(apiKeyDailyKeys, k)
	}
	statsAPIKeyDailyCacheNeedUpdateLock.Unlock()

	statsAPIKeyHourlyCacheNeedUpdateLock.Lock()
	apiKeyHourlyKeys := make([]apiKeyHourlyKey, 0, len(statsAPIKeyHourlyCacheNeedUpdate))
	for k := range statsAPIKeyHourlyCacheNeedUpdate {
		apiKeyHourlyKeys = append(apiKeyHourlyKeys, k)
	}
	statsAPIKeyHourlyCacheNeedUpdateLock.Unlock()

	err := persistStatsSnapshots(ctx, totalSnap, dailyOverride, hourlySnaps, channelSnaps, modelSnaps, apiKeySnaps, apiKeyDailyKeys, apiKeyHourlyKeys)

	// 仅在持久化成功后清空 dirty set
	if err == nil {
		statsHourlyCacheNeedUpdateLock.Lock()
		for _, k := range hourlyDirtyKeys {
			delete(statsHourlyCacheNeedUpdate, k)
		}
		statsHourlyCacheNeedUpdateLock.Unlock()

		statsAPIKeyDailyCacheNeedUpdateLock.Lock()
		for _, k := range apiKeyDailyKeys {
			delete(statsAPIKeyDailyCacheNeedUpdate, k)
		}
		statsAPIKeyDailyCacheNeedUpdateLock.Unlock()

		statsAPIKeyHourlyCacheNeedUpdateLock.Lock()
		for _, k := range apiKeyHourlyKeys {
			delete(statsAPIKeyHourlyCacheNeedUpdate, k)
		}
		statsAPIKeyHourlyCacheNeedUpdateLock.Unlock()
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
	log.Infof("stats refresh cache: today=%s, daily_found=%v, daily_success=%d, daily_input_token=%d",
		today, result.RowsAffected > 0, loadedDaily.RequestSuccess, loadedDaily.InputToken)

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

	// APIKeyHourly: 加载最近 2 天的数据（当天 + 昨天防止跨天丢失）
	var loadedAPIKeyHourlies []model.StatsAPIKeyHourly
	result = dbConn.Where("date >= ?", time.Now().AddDate(0, 0, -2).Format("20060102")).Find(&loadedAPIKeyHourlies)
	if result.Error != nil {
		return fmt.Errorf("failed to get api key hourly stats: %v", result.Error)
	}

	statsAPIKeyHourlyCacheLock.Lock()
	statsAPIKeyHourlyCache.Clear()
	for _, v := range loadedAPIKeyHourlies {
		statsAPIKeyHourlyCache.Set(apiKeyHourlyKey{APIKeyID: v.APIKeyID, Date: v.Date, Hour: v.Hour}, v)
	}
	statsAPIKeyHourlyCacheLock.Unlock()

	statsAPIKeyHourlyCacheNeedUpdateLock.Lock()
	statsAPIKeyHourlyCacheNeedUpdate = make(map[apiKeyHourlyKey]struct{})
	statsAPIKeyHourlyCacheNeedUpdateLock.Unlock()

	return nil
}
