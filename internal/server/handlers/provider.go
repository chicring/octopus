package handlers

import (
	"net/http"
	"net/url"

	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/provider/auth"
	"github.com/bestruirui/octopus/internal/provider/presets"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/api/v1/provider").
		Use(middleware.Auth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/list", http.MethodGet).
				Handle(listProviders),
		).
		AddRoute(
			router.NewRoute("/presets", http.MethodGet).
				Handle(listPresets),
		).
		AddRoute(
			router.NewRoute("/auth/start", http.MethodPost).
				Handle(startAuth),
		).
		AddRoute(
			router.NewRoute("/auth/poll", http.MethodPost).
				Handle(pollAuth),
		).
		AddRoute(
			router.NewRoute("/auth/submit-callback", http.MethodPost).
				Handle(submitCallback),
		)

	router.NewGroupRouter("/api/v1/provider/auth").
		AddRoute(
			router.NewRoute("/callback/:provider_id", http.MethodGet).
				Handle(authCallback),
		)
	// 兼容 Codex CLI 标准 redirect_uri: http://localhost:1455/auth/callback
	router.NewGroupRouter("/auth").
		AddRoute(
			router.NewRoute("/callback/:provider_id", http.MethodGet).
				Handle(authCallback),
		)
}

type providerInfo struct {
	ID                string                     `json:"id"`
	DisplayName       string                     `json:"display_name"`
	AuthType          string                     `json:"auth_type"`
	SupportsChat      bool                       `json:"supports_chat"`
	SupportsEmbedding bool                       `json:"supports_embedding"`
	CredentialSchema  *provider.CredentialSchema `json:"credential_schema,omitempty"`
}

func listProviders(c *gin.Context) {
	providers := provider.List()
	result := make([]providerInfo, 0, len(providers))
	for _, p := range providers {
		info := providerInfo{
			ID:                string(p.ID()),
			DisplayName:       p.DisplayName(),
			SupportsChat:      p.Capabilities().Chat,
			SupportsEmbedding: p.Capabilities().Embedding,
		}
		if sp, ok := p.(provider.SchemaProvider); ok {
			schema := sp.CredentialSchema()
			info.CredentialSchema = &schema
			info.AuthType = string(schema.AuthType)
		} else if ap, ok := p.(provider.AuthProvider); ok {
			if flow := ap.AuthFlow(); flow != nil {
				info.AuthType = string(flow.Type())
			}
		}
		result = append(result, info)
	}
	resp.Success(c, result)
}

func listPresets(c *gin.Context) {
	resp.Success(c, presets.List())
}

type startAuthRequest struct {
	ProviderID string `json:"provider_id" binding:"required"`
	ChannelID  int    `json:"channel_id"`
}

func startAuth(c *gin.Context) {
	var req startAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	p := provider.Get(provider.ProviderID(req.ProviderID))
	if p == nil {
		resp.Error(c, http.StatusBadRequest, "unknown provider")
		return
	}

	ap, ok := p.(provider.AuthProvider)
	if !ok || ap.AuthFlow() == nil {
		resp.Error(c, http.StatusBadRequest, "provider does not support auth flow")
		return
	}

	flow := ap.AuthFlow()
	session, err := flow.Start(c.Request.Context(), provider.AuthParams{})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	oauthSession, err := op.CreateOAuthSession(c.Request.Context(), req.ProviderID, req.ChannelID, session)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	resp.Success(c, gin.H{
		"session_id":       oauthSession.ID,
		"user_code":        session.UserCode,
		"verification_uri": session.VerificationURI,
		"expires_at":       session.ExpiresAt,
		"interval":         session.Interval,
	})
}

type pollAuthRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

func pollAuth(c *gin.Context) {
	var req pollAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	oauthSession, err := op.GetOAuthSession(c.Request.Context(), req.SessionID)
	if err != nil {
		resp.Error(c, http.StatusNotFound, "session not found")
		return
	}

	switch oauthSession.Status {
	case "completed":
		result, err := auth.DecryptResultData(oauthSession.ResultData)
		if err != nil {
			resp.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		resp.Success(c, gin.H{"status": "completed", "result": result})
	case "failed":
		resp.Success(c, gin.H{"status": "failed"})
	case "pending":
		sessionData, err := auth.DecryptSessionData(oauthSession.SessionData)
		if err != nil {
			resp.Error(c, http.StatusInternalServerError, err.Error())
			return
		}

		p := provider.Get(provider.ProviderID(oauthSession.ProviderID))
		if p == nil {
			resp.Error(c, http.StatusInternalServerError, "provider not found")
			return
		}
		ap, ok := p.(provider.AuthProvider)
		if !ok {
			resp.Error(c, http.StatusInternalServerError, "provider auth error")
			return
		}

		result, err := ap.AuthFlow().Poll(c.Request.Context(), sessionData)
		if err != nil {
			op.FailOAuthSession(c.Request.Context(), req.SessionID)
			resp.Error(c, http.StatusInternalServerError, err.Error())
			return
		}

		if result != nil {
			if err := op.CompleteOAuthSession(c.Request.Context(), req.SessionID, result); err != nil {
				resp.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
			resp.Success(c, gin.H{"status": "completed", "result": result})
		} else {
			resp.Success(c, gin.H{"status": "pending"})
		}
	default:
		resp.Error(c, http.StatusInternalServerError, "unknown session status")
	}
}

func authCallback(c *gin.Context) {
	providerID := c.Param("provider_id")
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		resp.Error(c, http.StatusBadRequest, "missing code or state")
		return
	}

	oauthSession, err := op.GetOAuthSessionByState(c.Request.Context(), state)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, "invalid or expired state")
		return
	}

	if oauthSession.ProviderID != providerID {
		resp.Error(c, http.StatusBadRequest, "provider mismatch")
		return
	}

	p := provider.Get(provider.ProviderID(providerID))
	if p == nil {
		resp.Error(c, http.StatusBadRequest, "unknown provider")
		return
	}

	ap, ok := p.(provider.AuthProvider)
	if !ok {
		resp.Error(c, http.StatusBadRequest, "provider does not support auth flow")
		return
	}

	sessionData, err := auth.DecryptSessionData(oauthSession.SessionData)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	result, err := ap.AuthFlow().Callback(c.Request.Context(), sessionData, code)
	if err != nil {
		op.FailOAuthSession(c.Request.Context(), oauthSession.ID)
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	if err := op.CompleteOAuthSession(c.Request.Context(), oauthSession.ID, result); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `<!DOCTYPE html><html><body><h2>Authorization successful!</h2><p>You can close this window.</p></body></html>`)
}

// submitCallbackRequest 手动回调提交请求
type submitCallbackRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	CallbackURL string `json:"callback_url" binding:"required"`
}

// submitCallback 处理手动回调模式：用户从浏览器地址栏复制回调 URL，粘贴提交
// 解析 URL 中的 code 和 state，验证 state 后换取 token
func submitCallback(c *gin.Context) {
	var req submitCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	// 解析回调 URL，提取 code 和 state
	parsedURL, err := url.Parse(req.CallbackURL)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, "invalid callback URL")
		return
	}

	code := parsedURL.Query().Get("code")
	state := parsedURL.Query().Get("state")
	if code == "" || state == "" {
		resp.Error(c, http.StatusBadRequest, "callback URL missing code or state parameter")
		return
	}

	// 加载 OAuth 会话
	oauthSession, err := op.GetOAuthSession(c.Request.Context(), req.SessionID)
	if err != nil {
		resp.Error(c, http.StatusNotFound, "session not found")
		return
	}

	// 验证 state 防止 CSRF
	if oauthSession.State != state {
		resp.Error(c, http.StatusBadRequest, "state mismatch")
		return
	}

	// 获取 provider 和 auth flow
	p := provider.Get(provider.ProviderID(oauthSession.ProviderID))
	if p == nil {
		resp.Error(c, http.StatusBadRequest, "unknown provider")
		return
	}

	ap, ok := p.(provider.AuthProvider)
	if !ok {
		resp.Error(c, http.StatusBadRequest, "provider does not support auth flow")
		return
	}

	// 解密会话数据
	sessionData, err := auth.DecryptSessionData(oauthSession.SessionData)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 用 code 换取 token
	authFlow := ap.AuthFlow()
	if authFlow == nil {
		resp.Error(c, http.StatusBadRequest, "provider auth flow is nil")
		return
	}
	result, err := authFlow.Callback(c.Request.Context(), sessionData, code)
	if err != nil {
		op.FailOAuthSession(c.Request.Context(), req.SessionID)
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 完成会话
	if err := op.CompleteOAuthSession(c.Request.Context(), req.SessionID, result); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	resp.Success(c, result)
}
