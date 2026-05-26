package handlers

import (
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/relay"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/transformer/inbound"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/v1").
		Use(middleware.APIKeyAuth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/chat/completions", http.MethodPost).
				Handle(chat),
		).
		AddRoute(
			router.NewRoute("/responses", http.MethodPost).
				Handle(response),
		).
		AddRoute(
			router.NewRoute("/messages", http.MethodPost).
				Handle(message),
		).
		AddRoute(
			router.NewRoute("/embeddings", http.MethodPost).
				Handle(embedding),
		)
	router.NewGroupRouter("/v1beta").
		Use(middleware.APIKeyAuth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/models/*action", http.MethodPost).
				Handle(geminiContent),
		)
}

func chat(c *gin.Context) {
	relay.Handler(inbound.InboundTypeOpenAIChat, c)
}
func response(c *gin.Context) {
	relay.Handler(inbound.InboundTypeOpenAIResponse, c)
}
func message(c *gin.Context) {
	relay.Handler(inbound.InboundTypeAnthropic, c)
}
func embedding(c *gin.Context) {
	relay.Handler(inbound.InboundTypeOpenAIEmbedding, c)
}

func geminiContent(c *gin.Context) {
	action := c.Param("action")
	modelName, stream, ok := parseGeminiModelAction(action)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "unsupported Gemini endpoint"}})
		return
	}
	c.Set("gemini_model", modelName)
	c.Set("gemini_stream", stream)
	relay.Handler(inbound.InboundTypeGemini, c)
}

func parseGeminiModelAction(action string) (string, bool, bool) {
	action = strings.TrimPrefix(action, "/")
	modelName, method, ok := strings.Cut(action, ":")
	if !ok || modelName == "" {
		return "", false, false
	}
	modelName = strings.TrimPrefix(modelName, "models/")
	switch method {
	case "generateContent":
		return modelName, false, true
	case "streamGenerateContent":
		return modelName, true, true
	default:
		return "", false, false
	}
}
