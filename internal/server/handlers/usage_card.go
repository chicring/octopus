package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/usagecard"
	"github.com/gin-gonic/gin"
)

func init() {
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
			router.NewRoute("/refresh/:id", http.MethodPost).
				Handle(refreshUsageCard),
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
