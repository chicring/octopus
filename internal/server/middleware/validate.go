package middleware

import (
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/gin-gonic/gin"
)

func RequireJSON() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet ||
			c.Request.Method == http.MethodDelete ||
			c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		contentType := c.GetHeader("Content-Type")
		if strings.Contains(contentType, "application/json") {
			c.Next()
			return
		}

		// POST/PUT with empty body (content-length=0) is allowed
		if c.Request.ContentLength == 0 {
			c.Next()
			return
		}

		resp.Error(c, http.StatusUnsupportedMediaType, resp.ErrInvalidJSON)
		c.Abort()

		c.Next()
	}
}
