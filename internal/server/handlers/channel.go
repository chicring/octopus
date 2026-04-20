package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/task"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/api/v1/channel").
		Use(middleware.Auth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/list", http.MethodGet).
				Handle(listChannel),
		).
		AddRoute(
			router.NewRoute("/create", http.MethodPost).
				Handle(createChannel),
		).
		AddRoute(
			router.NewRoute("/update", http.MethodPost).
				Handle(updateChannel),
		).
		AddRoute(
			router.NewRoute("/enable", http.MethodPost).
				Handle(enableChannel),
		).
		AddRoute(
			router.NewRoute("/delete/:id", http.MethodDelete).
				Handle(deleteChannel),
		).
		AddRoute(
			router.NewRoute("/fetch-model", http.MethodPost).
				Handle(fetchModel),
		).
		AddRoute(
			router.NewRoute("/test-models", http.MethodPost).
				Handle(testModels),
		).
		AddRoute(
			router.NewRoute("/test-models-by-config", http.MethodPost).
				Handle(testModelsByConfig),
		).
		AddRoute(
			router.NewRoute("/test-models-by-key", http.MethodPost).
				Handle(testModelsByKey),
		)
	router.NewGroupRouter("/api/v1/channel").
		Use(middleware.Auth()).
		AddRoute(
			router.NewRoute("/sync", http.MethodPost).
				Handle(syncChannel),
		).
		AddRoute(
			router.NewRoute("/last-sync-time", http.MethodGet).
				Handle(getLastSyncTime),
		)
}

func listChannel(c *gin.Context) {
	channels, err := op.ChannelList(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	for i, channel := range channels {
		stats := op.StatsChannelGet(channel.ID)
		channels[i].Stats = &stats
	}
	resp.Success(c, channels)
}

func createChannel(c *gin.Context) {
	var channel model.Channel
	if err := c.ShouldBindJSON(&channel); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	// 归一化 provider_id 和 type
	// 优先使用 provider_id：如果提供了 provider_id，从 provider_id 推导 type
	// 否则从 type 推导 provider_id
	if channel.ProviderID != "" {
		pid, lt, err := provider.NormalizeChannelType(&channel.ProviderID, nil)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		channel.ProviderID = pid
		channel.Type = lt
	} else {
		pid, lt, err := provider.NormalizeChannelType(nil, &channel.Type)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		channel.ProviderID = pid
		channel.Type = lt
	}
	if err := op.ChannelCreate(&channel, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	stats := op.StatsChannelGet(channel.ID)
	channel.Stats = &stats
	go func(channel *model.Channel) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		modelStr := channel.Model + "," + channel.CustomModel
		modelArray := strings.Split(modelStr, ",")
		helper.LLMPriceAddToDB(modelArray, ctx)
		helper.ChannelBaseUrlDelayUpdate(channel, ctx)
		helper.ChannelAutoGroup(channel, ctx)
	}(&channel)
	resp.Success(c, channel)
}

func updateChannel(c *gin.Context) {
	var req model.ChannelUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	// 归一化 provider_id 和 type
	if req.ProviderID != nil || req.Type != nil {
		pid, lt, err := provider.NormalizeChannelType(req.ProviderID, req.Type)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		hadProviderID := req.ProviderID != nil
		hadType := req.Type != nil
		// 只设置实际提供的字段，避免零值覆盖
		if hadProviderID {
			req.ProviderID = &pid
		}
		if hadType {
			req.Type = &lt
		}
		// 如果只提供了 provider_id，推导的 type 也需要持久化
		if hadProviderID && !hadType {
			p := provider.Get(provider.ProviderID(pid))
			if p != nil && p.LegacyType() != nil {
				req.Type = &lt
			}
		}
		// 如果只提供了 type，推导的 provider_id 也需要持久化
		if hadType && !hadProviderID && pid != "" {
			req.ProviderID = &pid
		}
	}
	channel, err := op.ChannelUpdate(&req, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	stats := op.StatsChannelGet(channel.ID)
	channel.Stats = &stats
	go func(channel *model.Channel) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		modelStr := channel.Model + "," + channel.CustomModel
		modelArray := strings.Split(modelStr, ",")
		helper.LLMPriceAddToDB(modelArray, ctx)
		helper.ChannelBaseUrlDelayUpdate(channel, ctx)
		helper.ChannelAutoGroup(channel, ctx)
	}(channel)
	resp.Success(c, channel)
}

func enableChannel(c *gin.Context) {
	var request struct {
		ID      int  `json:"id"`
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	if err := op.ChannelEnabled(request.ID, request.Enabled, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, nil)
}

func deleteChannel(c *gin.Context) {
	id := c.Param("id")
	idNum, err := strconv.Atoi(id)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidParam)
		return
	}
	if err := op.ChannelDel(idNum, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, nil)
}
func fetchModel(c *gin.Context) {
	var request model.Channel
	if err := c.ShouldBindJSON(&request); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	models, err := helper.FetchModels(c.Request.Context(), request)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, models)
}

func syncChannel(c *gin.Context) {
	task.SyncModelsTask()
	resp.Success(c, nil)
}

func getLastSyncTime(c *gin.Context) {
	time := task.GetLastSyncModelsTime()
	resp.Success(c, time)
}

func testModels(c *gin.Context) {
	var req struct {
		ChannelID int      `json:"channel_id"`
		Models    []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	if req.ChannelID <= 0 || len(req.Models) == 0 {
		resp.Error(c, http.StatusBadRequest, "channel_id and models are required")
		return
	}

	channel, err := op.ChannelGet(req.ChannelID, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "channel not found")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	results := helper.TestModels(ctx, channel, req.Models)
	resp.Success(c, results)
}

func testModelsByConfig(c *gin.Context) {
	var channel model.Channel
	if err := c.ShouldBindJSON(&channel); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	modelsStr := strings.Split(channel.Model, ",")
	var models []string
	for _, m := range modelsStr {
		m = strings.TrimSpace(m)
		if m != "" {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		resp.Error(c, http.StatusBadRequest, "models are required")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	results := helper.TestModels(ctx, &channel, models)
	resp.Success(c, results)
}

func testModelsByKey(c *gin.Context) {
	var req struct {
		ChannelID int      `json:"channel_id"`
		KeyID     int      `json:"key_id"`
		Models    []string `json:"models"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	if req.ChannelID <= 0 || req.KeyID <= 0 || len(req.Models) == 0 {
		resp.Error(c, http.StatusBadRequest, "channel_id, key_id and models are required")
		return
	}

	channel, err := op.ChannelGet(req.ChannelID, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "channel not found")
		return
	}

	// 查找指定的 Key
	var targetKey string
	for _, k := range channel.Keys {
		if k.ID == req.KeyID {
			targetKey = k.ChannelKey
			break
		}
	}
	if targetKey == "" {
		resp.Error(c, http.StatusNotFound, "key not found")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	results := helper.TestModelsWithKey(ctx, channel, targetKey, req.Models)
	resp.Success(c, results)
}
