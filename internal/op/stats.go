package op

import (
	"context"
	"errors"
	"fmt"
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

var statsHourlyCache [24]model.StatsHourly
var statsHourlyCacheLock sync.RWMutex

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

func StatsSaveDB(ctx context.Context) error {
	statsTotalCacheLock.RLock()
	totalSnap := statsTotalCache
	statsTotalCacheLock.RUnlock()
	if totalSnap.ID == 0 {
		totalSnap.ID = 1
	}

	statsDailyCacheLock.RLock()
	dailySnap := statsDailyCache
	statsDailyCacheLock.RUnlock()

	statsHourlyCacheLock.RLock()
	hourlyAll := statsHourlyCache
	statsHourlyCacheLock.RUnlock()

	// 收集需要持久化的 key，但不立即清空 dirty set
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

	err := persistStatsSnapshots(ctx, totalSnap, dailySnap, hourlyAll, channelIDs, modelNames, apiKeyIDs, apiKeyDailyKeys)

	// 仅在持久化成功后清空 dirty set
	if err == nil {
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

func persistStatsSnapshots(
	ctx context.Context,
	totalSnap model.StatsTotal,
	dailySnap model.StatsDaily,
	hourlyAll [24]model.StatsHourly,
	channelIDs []int,
	modelNames []string,
	apiKeyIDs []int,
	apiKeyDailyKeys []apiKeyDailyKey,
) error {
	dbConn := db.GetDB().WithContext(ctx)

	if result := dbConn.Save(&totalSnap); result.Error != nil {
		return result.Error
	}
	if result := dbConn.Save(&dailySnap); result.Error != nil {
		return result.Error
	}

	todayDate := time.Now().Format("20060102")
	hourlyStats := make([]model.StatsHourly, 0, 24)
	for hour := 0; hour < 24; hour++ {
		if hourlyAll[hour].Date == todayDate {
			hourlyStats = append(hourlyStats, hourlyAll[hour])
		}
	}
	if len(hourlyStats) > 0 {
		if result := dbConn.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "date"}, {Name: "hour"}},
			UpdateAll: true,
		}).Create(&hourlyStats); result.Error != nil {
			return result.Error
		}
	}

	for _, id := range channelIDs {
		ch, ok := statsChannelCache.Get(id)
		if !ok {
			continue
		}
		if result := dbConn.Save(&ch); result.Error != nil {
			return result.Error
		}
	}

	for _, name := range modelNames {
		m, ok := statsModelCache.Get(name)
		if !ok {
			continue
		}
		if result := dbConn.Save(&m); result.Error != nil {
			return result.Error
		}
	}

	for _, id := range apiKeyIDs {
		ak, ok := statsAPIKeyCache.Get(id)
		if !ok {
			continue
		}
		if result := dbConn.Save(&ak); result.Error != nil {
			return result.Error
		}
	}

	for _, k := range apiKeyDailyKeys {
		akd, ok := statsAPIKeyDailyCache.Get(k)
		if !ok {
			continue
		}
		if result := dbConn.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "api_key_id"}, {Name: "date"}},
			UpdateAll: true,
		}).Create(&akd); result.Error != nil {
			return result.Error
		}
	}

	return nil
}

func statsSaveDBWithDailyOverride(ctx context.Context, dailyOverride model.StatsDaily) error {
	statsTotalCacheLock.RLock()
	totalSnap := statsTotalCache
	statsTotalCacheLock.RUnlock()
	if totalSnap.ID == 0 {
		totalSnap.ID = 1
	}

	statsHourlyCacheLock.RLock()
	hourlyAll := statsHourlyCache
	statsHourlyCacheLock.RUnlock()

	// 收集需要持久化的 key，但不立即清空 dirty set
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

	err := persistStatsSnapshots(ctx, totalSnap, dailyOverride, hourlyAll, channelIDs, modelNames, apiKeyIDs, apiKeyDailyKeys)

	// 仅在持久化成功后清空 dirty set
	if err == nil {
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

func StatsDailyUpdate(ctx context.Context, metrics model.StatsMetrics) error {
	today := time.Now().Format("20060102")

	statsDailyCacheLock.Lock()
	if statsDailyCache.Date == today {
		statsDailyCache.StatsMetrics.Add(metrics)
		statsDailyCacheLock.Unlock()
		return nil
	}

	prevDaily := statsDailyCache
	statsDailyCache = model.StatsDaily{Date: today}
	statsDailyCache.StatsMetrics.Add(metrics)
	statsDailyCacheLock.Unlock()

	return statsSaveDBWithDailyOverride(ctx, prevDaily)
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

	statsHourlyCacheLock.Lock()
	defer statsHourlyCacheLock.Unlock()

	if statsHourlyCache[nowHour].Date != todayDate {
		statsHourlyCache[nowHour] = model.StatsHourly{
			Hour: nowHour,
			Date: todayDate,
		}
	}

	statsHourlyCache[nowHour].StatsMetrics.Add(metrics)
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

// StatsAPIKeyDailyGet 返回指定 API Key 的每日统计（最近 N 天），合并内存缓存中当天未刷盘的数据
func StatsAPIKeyDailyGet(ctx context.Context, apiKeyID uint, days int) ([]model.StatsAPIKeyDaily, error) {
	var result []model.StatsAPIKeyDaily
	err := db.GetDB().WithContext(ctx).
		Where("api_key_id = ? AND date >= ?", apiKeyID, time.Now().AddDate(0, 0, -days).Format("20060102")).
		Order("date ASC").
		Find(&result).Error
	if err != nil {
		return nil, err
	}

	// 合并内存缓存中当天未刷盘的数据
	today := time.Now().Format("20060102")
	todayKey := apiKeyDailyKey{APIKeyID: apiKeyID, Date: today}
	statsAPIKeyDailyCacheLock.Lock()
	if cached, ok := statsAPIKeyDailyCache.Get(todayKey); ok {
		statsAPIKeyDailyCacheLock.Unlock()
		// 查找 result 中是否已有今天的记录
		found := false
		for i := range result {
			if result[i].Date == today {
				result[i] = cached
				found = true
				break
			}
		}
		if !found {
			result = append(result, cached)
		}
	} else {
		statsAPIKeyDailyCacheLock.Unlock()
	}

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
		return tmp
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

func StatsHourlyGet() []model.StatsHourly {
	now := time.Now()
	currentHour := now.Hour()
	todayDate := time.Now().Format("20060102")

	statsHourlyCacheLock.RLock()
	defer statsHourlyCacheLock.RUnlock()

	result := make([]model.StatsHourly, 0, currentHour+1)

	for hour := 0; hour <= currentHour; hour++ {
		if statsHourlyCache[hour].Date == todayDate {
			result = append(result, statsHourlyCache[hour])
		} else {
			result = append(result, model.StatsHourly{
				Hour: hour,
				Date: todayDate,
			})
		}
	}

	return result
}

func StatsGetDaily(ctx context.Context) ([]model.StatsDaily, error) {
	var statsDaily []model.StatsDaily
	result := db.GetDB().WithContext(ctx).Find(&statsDaily)
	if result.Error != nil {
		return nil, result.Error
	}
	return statsDaily, nil
}

func statsRefreshCache(ctx context.Context) error {
	dbConn := db.GetDB().WithContext(ctx)
	today := time.Now().Format("20060102")

	var loadedDaily model.StatsDaily
	result := dbConn.Last(&loadedDaily)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get daily stats: %v", result.Error)
	}
	if result.RowsAffected == 0 || loadedDaily.Date != today {
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

	var loadedHourly []model.StatsHourly
	result = dbConn.Where("date = ?", today).Find(&loadedHourly)
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

	statsHourlyCacheLock.Lock()
	statsHourlyCache = [24]model.StatsHourly{}
	for _, v := range loadedHourly {
		if v.Hour >= 0 && v.Hour < 24 {
			statsHourlyCache[v.Hour] = v
		}
	}
	statsHourlyCacheLock.Unlock()

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

	var loadedAPIKeyDailies []model.StatsAPIKeyDaily
	result = dbConn.Where("date = ?", today).Find(&loadedAPIKeyDailies)
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
