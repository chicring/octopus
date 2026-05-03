package task

import (
	"context"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/price"
	"github.com/bestruirui/octopus/internal/utils/log"
)

const (
	TaskPriceUpdate  = "price_update"
	TaskStatsSave    = "stats_save"
	TaskRelayLogSave = "relay_log_save"
	TaskSyncLLM      = "sync_llm"
	TaskCleanLLM     = "clean_llm"
	TaskBaseUrlDelay = "base_url_delay"
	TaskOAuthCleanup = "oauth_cleanup"
)

func Init() {
	// 注册价格更新任务
	priceUpdateIntervalHours, err := op.SettingGetInt(model.SettingKeyModelInfoUpdateInterval)
	if err != nil {
		log.Errorf("failed to get model info update interval: %v", err)
	} else {
		priceUpdateInterval := time.Duration(priceUpdateIntervalHours) * time.Hour
		Register(string(model.SettingKeyModelInfoUpdateInterval), priceUpdateInterval, true, func() {
			if err := price.UpdateLLMPrice(context.Background()); err != nil {
				log.Warnf("failed to update price info: %v", err)
			}
		})
	}

	// 注册基础URL延迟任务
	Register(TaskBaseUrlDelay, 1*time.Hour, true, ChannelBaseUrlDelayTask)

	// 注册LLM同步任务
	syncLLMIntervalHours, err := op.SettingGetInt(model.SettingKeySyncLLMInterval)
	if err != nil {
		log.Warnf("failed to get sync LLM interval: %v", err)
	} else {
		syncLLMInterval := time.Duration(syncLLMIntervalHours) * time.Hour
		Register(string(model.SettingKeySyncLLMInterval), syncLLMInterval, true, SyncModelsTask)
	}

	// 注册统计保存任务
	statsSaveIntervalMinutes, err := op.SettingGetInt(model.SettingKeyStatsSaveInterval)
	if err != nil {
		log.Warnf("failed to get stats save interval: %v", err)
	} else {
		statsSaveInterval := time.Duration(statsSaveIntervalMinutes) * time.Minute
		Register(TaskStatsSave, statsSaveInterval, true, op.StatsSaveDBTask)
	}

	// 注册中继日志保存任务
	Register(TaskRelayLogSave, 10*time.Minute, false, func() {
		if err := op.RelayLogSaveDBTask(context.Background()); err != nil {
			log.Warnf("relay log save db task failed: %v", err)
		}
	})

	// 注册 OAuth session 清理任务
	Register(TaskOAuthCleanup, 1*time.Hour, false, op.CleanupExpiredOAuthSessions)
}
