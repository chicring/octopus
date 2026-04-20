package provider

import (
	"context"
	"net/http"

	"github.com/bestruirui/octopus/internal/model"
)

// ModelFetcher 模型获取函数类型
type ModelFetcher func(client *http.Client, ctx context.Context, channel model.Channel) ([]string, error)
