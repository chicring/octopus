package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/samber/lo"

	"github.com/bestruirui/octopus/internal/client"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/provider/auth"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/usagecard"
	"github.com/gin-gonic/gin"
)

func init() {
	usagecard.GetProxyHTTPClient = client.GetHTTPClientSystemProxy

	router.NewGroupRouter("/api/v1/usage-card").
		Use(middleware.Auth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/templates", http.MethodGet).
				Handle(listUsageCardTemplates),
		).
		AddRoute(
			router.NewRoute("/list", http.MethodGet).
				Handle(listUsageCards),
		).
		AddRoute(
			router.NewRoute("/create", http.MethodPost).
				Handle(createUsageCard),
		).
		AddRoute(
			router.NewRoute("/update", http.MethodPost).
				Handle(updateUsageCard),
		).
		AddRoute(
			router.NewRoute("/delete/:id", http.MethodDelete).
				Handle(deleteUsageCard),
		).
		AddRoute(
			router.NewRoute("/batch-delete", http.MethodPost).
				Handle(batchDeleteUsageCards),
		).
		AddRoute(
			router.NewRoute("/refresh/:id", http.MethodPost).
				Handle(refreshUsageCard),
		).
		AddRoute(
			router.NewRoute("/import/codex-channel", http.MethodPost).
				Handle(importCodexChannelUsageCard),
		).
		AddRoute(
			router.NewRoute("/batch-import/codex-channel", http.MethodPost).
				Handle(batchImportCodexChannelUsageCards),
		)
}

func listUsageCardTemplates(c *gin.Context) {
	templates := usagecard.ListTemplates()
	resp.Success(c, templates)
}

func listUsageCards(c *gin.Context) {
	cards, err := op.UsageCardList(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, cards)
}

func createUsageCard(c *gin.Context) {
	var req model.UsageCardCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	card, err := op.UsageCardCreate(&req, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, card)
}

func updateUsageCard(c *gin.Context) {
	var req model.UsageCardUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	card, err := op.UsageCardUpdate(&req, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, card)
}

func deleteUsageCard(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidParam)
		return
	}

	if err := op.UsageCardDelete(uint(id), c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, nil)
}

func refreshUsageCard(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidParam)
		return
	}

	// 刷新超时 15 秒
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	card, err := op.UsageCardRefresh(uint(id), ctx)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, card)
}

// importCodexChannelRequest 从 Codex 渠道导入用量卡片的请求
type importCodexChannelRequest struct {
	ChannelID int `json:"channel_id" binding:"required"`
	KeyID     int `json:"key_id"`
}

// importCodexChannelUsageCard 从 Codex 渠道导入用量卡片
// POST /api/v1/usage-card/import/codex-channel
func importCodexChannelUsageCard(c *gin.Context) {
	var req importCodexChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	// 获取渠道
	channel, err := op.ChannelGet(req.ChannelID, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "渠道不存在")
		return
	}

	// 验证渠道类型
	if !channel.HasProvider("codex") {
		resp.Error(c, http.StatusBadRequest, "渠道不是 Codex 类型")
		return
	}

	// 查找指定的 Key
	var targetKey *model.ChannelKey
	for i := range channel.Keys {
		if req.KeyID == 0 || channel.Keys[i].ID == req.KeyID {
			targetKey = &channel.Keys[i]
			break
		}
	}
	if targetKey == nil {
		resp.Error(c, http.StatusNotFound, "未找到指定的 Key")
		return
	}

	// 解析凭证以获取 email 用于命名
	cred, credErr := auth.ParseCodexCredential(targetKey.ChannelKey)
	displayName := channel.Name
	if credErr == nil && cred.Email != "" {
		displayName = cred.Email
	}

	// 去重检查：如果已存在相同 account 的卡片，直接返回
	account := fmt.Sprintf("codex:%d:%d", req.ChannelID, targetKey.ID)
	if existing, found := op.UsageCardGetByAccount(account); found {
		resp.Success(c, existing)
		return
	}

	// 获取模板
	tmpl, ok := usagecard.GetTemplate("codex-usage")
	if !ok {
		resp.Error(c, http.StatusInternalServerError, "Codex 用量模板不存在")
		return
	}

	// 创建用量卡片
	createReq := &model.UsageCardCreateRequest{
		Name:               fmt.Sprintf("Codex 用量 - %s", displayName),
		TemplateID:         "codex-usage",
		Account:            account,
		Endpoint:           tmpl.DefaultEndpoint,
		Method:             tmpl.Method,
		AuthType:           "bearer",
		Secret:             targetKey.ChannelKey,
		ExtraHeaders:       usagecard.BuildExtraHeaders(tmpl),
		Config:             usagecard.BuildCardConfig(tmpl),
		RefreshIntervalSec: lo.ToPtr(300),
		Enabled:            lo.ToPtr(true),
		UseProxy:           lo.ToPtr(false),
	}

	card, err := op.UsageCardCreate(createReq, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, card)
}

// batchDeleteRequest 批量删除用量卡片请求
type batchDeleteRequest struct {
	IDs []uint `json:"ids" binding:"required"`
}

// batchDeleteUsageCards 批量删除用量卡片
// POST /api/v1/usage-card/batch-delete
func batchDeleteUsageCards(c *gin.Context) {
	var req batchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	if len(req.IDs) == 0 {
		resp.Error(c, http.StatusBadRequest, "ids 不能为空")
		return
	}
	if err := op.UsageCardBatchDelete(req.IDs, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, nil)
}

// batchImportCodexRequest 批量导入 Codex 渠道用量卡片请求
type batchImportCodexRequest struct {
	Items []importCodexChannelRequest `json:"items" binding:"required"`
}

// batchImportCodexChannelUsageCards 批量从 Codex 渠道导入用量卡片
// POST /api/v1/usage-card/batch-import/codex-channel
func batchImportCodexChannelUsageCards(c *gin.Context) {
	var req batchImportCodexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	if len(req.Items) == 0 {
		resp.Error(c, http.StatusBadRequest, "items 不能为空")
		return
	}

	type importResult struct {
		ChannelID int    `json:"channel_id"`
		KeyID     int    `json:"key_id"`
		CardID    uint   `json:"card_id,omitempty"`
		Skipped   bool   `json:"skipped,omitempty"`
		Error     string `json:"error,omitempty"`
	}

	results := make([]importResult, 0, len(req.Items))

	for _, item := range req.Items {
		// 获取渠道
		channel, err := op.ChannelGet(item.ChannelID, c.Request.Context())
		if err != nil {
			results = append(results, importResult{ChannelID: item.ChannelID, KeyID: item.KeyID, Error: "渠道不存在"})
			continue
		}

		if !channel.HasProvider("codex") {
			results = append(results, importResult{ChannelID: item.ChannelID, KeyID: item.KeyID, Error: "渠道不是 Codex 类型"})
			continue
		}

		var targetKey *model.ChannelKey
		for i := range channel.Keys {
			if item.KeyID == 0 || channel.Keys[i].ID == item.KeyID {
				targetKey = &channel.Keys[i]
				break
			}
		}
		if targetKey == nil {
			results = append(results, importResult{ChannelID: item.ChannelID, KeyID: item.KeyID, Error: "未找到指定的 Key"})
			continue
		}

		cred, credErr := auth.ParseCodexCredential(targetKey.ChannelKey)
		displayName := channel.Name
		if credErr == nil && cred.Email != "" {
			displayName = cred.Email
		}

		account := fmt.Sprintf("codex:%d:%d", item.ChannelID, targetKey.ID)
		if existing, found := op.UsageCardGetByAccount(account); found {
			results = append(results, importResult{ChannelID: item.ChannelID, KeyID: item.KeyID, CardID: existing.ID, Skipped: true})
			continue
		}

		tmpl, ok := usagecard.GetTemplate("codex-usage")
		if !ok {
			results = append(results, importResult{ChannelID: item.ChannelID, KeyID: item.KeyID, Error: "Codex 用量模板不存在"})
			continue
		}

		createReq := &model.UsageCardCreateRequest{
			Name:               fmt.Sprintf("Codex - %s", displayName),
			TemplateID:         "codex-usage",
			Account:            account,
			Endpoint:           tmpl.DefaultEndpoint,
			Method:             tmpl.Method,
			AuthType:           "bearer",
			Secret:             targetKey.ChannelKey,
			ExtraHeaders:       usagecard.BuildExtraHeaders(tmpl),
			Config:             usagecard.BuildCardConfig(tmpl),
			RefreshIntervalSec: lo.ToPtr(300),
			Enabled:            lo.ToPtr(true),
			UseProxy:           lo.ToPtr(false),
		}

		card, err := op.UsageCardCreate(createReq, c.Request.Context())
		if err != nil {
			results = append(results, importResult{ChannelID: item.ChannelID, KeyID: item.KeyID, Error: err.Error()})
			continue
		}
		results = append(results, importResult{ChannelID: item.ChannelID, KeyID: item.KeyID, CardID: card.ID})
	}

	resp.Success(c, results)
}
