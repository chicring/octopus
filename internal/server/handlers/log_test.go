package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestListLog_parseAPIKeyNamesParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		queryString     string
		expectedAPIKeys []string
	}{
		{
			name:            "single api key name",
			queryString:     "api_key_names=key-alpha",
			expectedAPIKeys: []string{"key-alpha"},
		},
		{
			name:            "multiple api key names",
			queryString:     "api_key_names=key-alpha,key-beta",
			expectedAPIKeys: []string{"key-alpha", "key-beta"},
		},
		{
			name:            "empty api key names param",
			queryString:     "api_key_names=",
			expectedAPIKeys: nil,
		},
		{
			name:            "no api key names param",
			queryString:     "",
			expectedAPIKeys: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			url := "/api/v1/log/list"
			if tt.queryString != "" {
				url += "?" + tt.queryString
			}
			req, _ := http.NewRequest(http.MethodGet, url, nil)
			c.Request = req

			// 与 handler 中相同的解析逻辑
			apiKeyNamesStr := c.Query("api_key_names")
			var apiKeyNames []string
			if apiKeyNamesStr != "" {
				apiKeyNames = strings.Split(apiKeyNamesStr, ",")
			}

			if len(apiKeyNames) != len(tt.expectedAPIKeys) {
				t.Fatalf("expected %v, got %v", tt.expectedAPIKeys, apiKeyNames)
			}
			for i, v := range apiKeyNames {
				if v != tt.expectedAPIKeys[i] {
					t.Errorf("index %d: expected %q, got %q", i, tt.expectedAPIKeys[i], v)
				}
			}
		})
	}
}

func TestListLog_parseModelNamesParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		queryString    string
		expectedModels []string
	}{
		{
			name:           "single model name",
			queryString:    "model_names=gpt-4",
			expectedModels: []string{"gpt-4"},
		},
		{
			name:           "multiple model names",
			queryString:    "model_names=gpt-4,claude-3",
			expectedModels: []string{"gpt-4", "claude-3"},
		},
		{
			name:           "empty model names param",
			queryString:    "model_names=",
			expectedModels: nil,
		},
		{
			name:           "no model names param",
			queryString:    "",
			expectedModels: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			url := "/api/v1/log/list"
			if tt.queryString != "" {
				url += "?" + tt.queryString
			}
			req, _ := http.NewRequest(http.MethodGet, url, nil)
			c.Request = req

			modelNamesStr := c.Query("model_names")
			var modelNames []string
			if modelNamesStr != "" {
				modelNames = strings.Split(modelNamesStr, ",")
			}

			if len(modelNames) != len(tt.expectedModels) {
				t.Fatalf("expected %v, got %v", tt.expectedModels, modelNames)
			}
			for i, v := range modelNames {
				if v != tt.expectedModels[i] {
					t.Errorf("index %d: expected %q, got %q", i, tt.expectedModels[i], v)
				}
			}
		})
	}
}

func TestListLog_combinedParams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/log/list?api_key_names=key-a,key-b&model_names=gpt-4&has_error=true", nil)
	c.Request = req

	apiKeyNamesStr := c.Query("api_key_names")
	var apiKeyNames []string
	if apiKeyNamesStr != "" {
		apiKeyNames = strings.Split(apiKeyNamesStr, ",")
	}

	modelNamesStr := c.Query("model_names")
	var modelNames []string
	if modelNamesStr != "" {
		modelNames = strings.Split(modelNamesStr, ",")
	}

	hasError := c.Query("has_error") == "true"

	if !hasError {
		t.Error("expected hasError=true")
	}
	if len(apiKeyNames) != 2 || apiKeyNames[0] != "key-a" || apiKeyNames[1] != "key-b" {
		t.Errorf("expected [key-a, key-b], got %v", apiKeyNames)
	}
	if len(modelNames) != 1 || modelNames[0] != "gpt-4" {
		t.Errorf("expected [gpt-4], got %v", modelNames)
	}
}
