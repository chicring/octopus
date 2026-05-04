package model

import "time"

// UsageCard 用量卡片配置
type UsageCard struct {
	ID                 uint            `json:"id" gorm:"primaryKey"`
	Name               string          `json:"name" gorm:"not null"`
	TemplateID         string          `json:"template_id" gorm:"size:64;not null"`
	Account            string          `json:"account"`
	Endpoint           string          `json:"endpoint"`
	Method             string          `json:"method" gorm:"size:10;default:GET"`
	AuthType           string          `json:"auth_type" gorm:"size:32;default:none"` // none/bearer/x-api-key/custom-header/cookie
	AuthHeader         string          `json:"auth_header"`                           // custom-header 时的 header 名
	EncryptedSecret    string          `json:"-" gorm:"column:encrypted_secret"`
	HasSecret          bool            `json:"has_secret" gorm:"-"`
	ExtraHeaders       []UsageHeader   `json:"extra_headers" gorm:"serializer:json"`
	Config             UsageCardConfig `json:"config" gorm:"serializer:json"`
	Enabled            bool            `json:"enabled" gorm:"default:true"`
	UseProxy           bool            `json:"use_proxy" gorm:"default:false"`
	RefreshIntervalSec int             `json:"refresh_interval_sec" gorm:"default:300"`
	LastResult         UsageSnapshot   `json:"last_result" gorm:"serializer:json"`
	LastError          string          `json:"last_error"`
	LastRefreshAt      *time.Time      `json:"last_refresh_at"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// UsageHeader 额外请求头
type UsageHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// UsageCardConfig 卡片自定义配置（通用模板的字段路径等）
type UsageCardConfig struct {
	Metrics []UsageMetricConfig `json:"metrics,omitempty"`
}

// UsageMetricConfig 单个指标的提取配置
type UsageMetricConfig struct {
	ID        string     `json:"id"`
	Label     string     `json:"label"`
	Kind      string     `json:"kind"`   // quota/counter/rate_limit/billing
	Unit      string     `json:"unit"`   // requests/times/tokens/credits/usd/percent
	Window    string     `json:"window"` // 5h/weekly/monthly/hourly/minute/custom
	Limit     *FieldSpec `json:"limit,omitempty"`
	Used      *FieldSpec `json:"used,omitempty"`
	Remaining *FieldSpec `json:"remaining,omitempty"`
	ResetAt   *FieldSpec `json:"reset_at,omitempty"`
}

// FieldSpec 字段提取规格
type FieldSpec struct {
	Source    string   `json:"source"`              // body/header/static
	Path      string   `json:"path"`                // $.resources.core.limit 或 x-ratelimit-limit-requests
	Transform []string `json:"transform,omitempty"`  // epoch_to_iso/epoch_ms_to_iso/percent_to_float/to_float
	Optional  bool     `json:"optional,omitempty"`
}

// UsageSnapshot 刷新后的归一化快照
type UsageSnapshot struct {
	Metrics []UsageMetric `json:"metrics,omitempty"`
}

// UsageMetric 归一化指标
type UsageMetric struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Kind      string   `json:"kind"`
	Unit      string   `json:"unit"`
	Window    string   `json:"window"`
	Limit     *float64 `json:"limit,omitempty"`
	Used      *float64 `json:"used,omitempty"`
	Remaining *float64 `json:"remaining,omitempty"`
	Percent   *float64 `json:"percent,omitempty"`
	ResetAt   *string  `json:"reset_at,omitempty"`
	Status    string   `json:"status"` // ok/warning/exhausted/unknown/error
	Message   string   `json:"message,omitempty"`
}

// UsageCardCreateRequest 创建卡片请求
type UsageCardCreateRequest struct {
	Name               string             `json:"name" binding:"required"`
	TemplateID         string             `json:"template_id" binding:"required"`
	Account            string             `json:"account"`
	Endpoint           string             `json:"endpoint"`
	Method             string             `json:"method"`
	AuthType           string             `json:"auth_type"`
	AuthHeader         string             `json:"auth_header"`
	Secret             string             `json:"secret"`
	ExtraHeaders       []UsageHeader      `json:"extra_headers"`
	Config             UsageCardConfig    `json:"config"`
	Enabled            *bool              `json:"enabled"`
	UseProxy           *bool              `json:"use_proxy"`
	RefreshIntervalSec *int               `json:"refresh_interval_sec"`
}

// UsageCardUpdateRequest 更新卡片请求
type UsageCardUpdateRequest struct {
	ID                 uint               `json:"id" binding:"required"`
	Name               *string            `json:"name,omitempty"`
	TemplateID         *string            `json:"template_id,omitempty"`
	Account            *string            `json:"account,omitempty"`
	Endpoint           *string            `json:"endpoint,omitempty"`
	Method             *string            `json:"method,omitempty"`
	AuthType           *string            `json:"auth_type,omitempty"`
	AuthHeader         *string            `json:"auth_header,omitempty"`
	Secret             *string            `json:"secret,omitempty"`
	ExtraHeaders       *[]UsageHeader     `json:"extra_headers,omitempty"`
	Config             *UsageCardConfig   `json:"config,omitempty"`
	Enabled            *bool              `json:"enabled,omitempty"`
	UseProxy           *bool              `json:"use_proxy,omitempty"`
	RefreshIntervalSec *int               `json:"refresh_interval_sec,omitempty"`
}
