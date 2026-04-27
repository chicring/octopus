package handlers

import (
	"net/http"
	"strconv"

	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/api/v1/stats").
		Use(middleware.Auth()).
		AddRoute(
			router.NewRoute("/today", http.MethodGet).
				Handle(getStatsToday),
		).
		AddRoute(
			router.NewRoute("/daily", http.MethodGet).
				Handle(getStatsDaily),
		).
		AddRoute(
			router.NewRoute("/hourly", http.MethodGet).
				Handle(getStatsHourly),
		).
		AddRoute(
			router.NewRoute("/total", http.MethodGet).
				Handle(getStatsTotal),
		).
		AddRoute(
			router.NewRoute("/apikey", http.MethodGet).
				Handle(getStatsAPIKey),
		).
		AddRoute(
			router.NewRoute("/model", http.MethodGet).
				Handle(getStatsModel),
		).
		AddRoute(
			router.NewRoute("/realtime", http.MethodGet).
				Handle(getStatsRealtime),
		).
		AddRoute(
			router.NewRoute("/apikey/:id/daily", http.MethodGet).
				Handle(getStatsAPIKeyDaily),
		)
}

func getStatsToday(c *gin.Context) {
	resp.Success(c, op.StatsTodayGet())
}

func getStatsDaily(c *gin.Context) {
	statsDaily, err := op.StatsGetDaily(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, statsDaily)
}

func getStatsHourly(c *gin.Context) {
	resp.Success(c, op.StatsHourlyGet())
}

func getStatsTotal(c *gin.Context) {
	resp.Success(c, op.StatsTotalGet())
}

func getStatsAPIKey(c *gin.Context) {
	resp.Success(c, op.StatsAPIKeyList())
}

func getStatsModel(c *gin.Context) {
	resp.Success(c, op.StatsModelList())
}

func getStatsRealtime(c *gin.Context) {
	resp.Success(c, op.StatsRealtimeGet())
}

func getStatsAPIKeyDaily(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, "invalid api key id")
		return
	}
	daysStr := c.DefaultQuery("days", "30")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days < 1 || days > 365 {
		days = 30
	}
	stats, err := op.StatsAPIKeyDailyGet(c.Request.Context(), uint(id), days)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, stats)
}
