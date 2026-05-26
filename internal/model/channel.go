package model

import (
	"math"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

type AutoGroupType int

const (
	AutoGroupTypeNone  AutoGroupType = 0 //不自动分组
	AutoGroupTypeFuzzy AutoGroupType = 1 //模糊匹配
	AutoGroupTypeExact AutoGroupType = 2 //准确匹配
	AutoGroupTypeRegex AutoGroupType = 3 //正则匹配
)

type Channel struct {
	ID            int                   `json:"id" gorm:"primaryKey"`
	Name          string                `json:"name" gorm:"unique;not null"`
	Type          outbound.OutboundType `json:"type"`
	ProviderID    string                `json:"provider_id" gorm:"size:64;index"`
	OfficialURL   string                `json:"official_url" gorm:"size:512"`
	Enabled       bool                  `json:"enabled" gorm:"default:true"`
	BaseUrls      []BaseUrl             `json:"base_urls" gorm:"serializer:json"`
	Keys          []ChannelKey          `json:"keys" gorm:"foreignKey:ChannelID"`
	Model         string                `json:"model"`
	CustomModel   string                `json:"custom_model"`
	Proxy         bool                  `json:"proxy" gorm:"default:false"`
	AutoSync      bool                  `json:"auto_sync" gorm:"default:false"`
	AutoGroup     AutoGroupType         `json:"auto_group" gorm:"default:0"`
	CustomHeader  []CustomHeader        `json:"custom_header" gorm:"serializer:json"`
	ParamOverride *string               `json:"param_override"`
	ChannelProxy  *string               `json:"channel_proxy"`
	UsageQuery    UsageQueryConfig      `json:"usage_query" gorm:"serializer:json"`
	Stats         *StatsChannel         `json:"stats,omitempty" gorm:"foreignKey:ChannelID"`
	MatchRegex    *string               `json:"match_regex"`
}

type BaseUrl struct {
	URL   string `json:"url"`
	Delay int    `json:"delay"`
}

type CustomHeader struct {
	HeaderKey   string `json:"header_key"`
	HeaderValue string `json:"header_value"`
}

type UsageQueryPreset string

const (
	UsageQueryPresetCustom            UsageQueryPreset = "custom"
	UsageQueryPresetGeneric           UsageQueryPreset = "generic"
	UsageQueryPresetNewAPI            UsageQueryPreset = "newapi"
	UsageQueryPresetTokenPlanOfficial UsageQueryPreset = "tokenplan_official"
)

type UsageQueryConfig struct {
	Enabled       bool             `json:"enabled"`
	Preset        UsageQueryPreset `json:"preset"`
	RequestURL    string           `json:"request_url"`
	Method        string           `json:"method"`
	Headers       []CustomHeader   `json:"headers"`
	TimeoutSec    int              `json:"timeout_sec"`
	IntervalMin   int              `json:"interval_min"`
	APIKey        string           `json:"api_key"`
	AccessToken   string           `json:"access_token"`
	UserID        string           `json:"user_id"`
	TemplateCode  string           `json:"template_code"`
	ExtractorCode string           `json:"extractor_code"`
}

type ChannelKey struct {
	ID               int     `json:"id" gorm:"primaryKey"`
	ChannelID        int     `json:"channel_id"`
	Enabled          bool    `json:"enabled" gorm:"default:true"`
	ChannelKey       string  `json:"channel_key"`
	IsCLI            bool    `json:"is_cli" gorm:"default:false"`
	Multiplier       float64 `json:"multiplier" gorm:"default:1"`
	Models           string  `json:"models"`
	StatusCode       int     `json:"status_code"`
	LastUseTimeStamp int64   `json:"last_use_time_stamp"`
	TotalCost        float64 `json:"total_cost"`
	TotalRequests    int64   `json:"total_requests" gorm:"default:0"`
	TotalInputToken  int64   `json:"total_input_token" gorm:"default:0"`
	TotalOutputToken int64   `json:"total_output_token" gorm:"default:0"`
	Remark           string  `json:"remark"`
}

// ChannelUpdateRequest 渠道更新请求 - 仅包含变更的数据
type ChannelUpdateRequest struct {
	ID            int                    `json:"id" binding:"required"`
	Name          *string                `json:"name,omitempty"`
	Type          *outbound.OutboundType `json:"type,omitempty"`
	ProviderID    *string                `json:"provider_id,omitempty"`
	OfficialURL   *string                `json:"official_url,omitempty"`
	Enabled       *bool                  `json:"enabled,omitempty"`
	BaseUrls      *[]BaseUrl             `json:"base_urls,omitempty"`
	Model         *string                `json:"model,omitempty"`
	CustomModel   *string                `json:"custom_model,omitempty"`
	Proxy         *bool                  `json:"proxy,omitempty"`
	AutoSync      *bool                  `json:"auto_sync,omitempty"`
	AutoGroup     *AutoGroupType         `json:"auto_group,omitempty"`
	CustomHeader  *[]CustomHeader        `json:"custom_header,omitempty"`
	ChannelProxy  *string                `json:"channel_proxy,omitempty"`
	ParamOverride *string                `json:"param_override,omitempty"`
	UsageQuery    *UsageQueryConfig      `json:"usage_query,omitempty"`
	MatchRegex    *string                `json:"match_regex,omitempty"`

	KeysToAdd    []ChannelKeyAddRequest    `json:"keys_to_add,omitempty"`
	KeysToUpdate []ChannelKeyUpdateRequest `json:"keys_to_update,omitempty"`
	KeysToDelete []int                     `json:"keys_to_delete,omitempty"`
}

type ChannelKeyAddRequest struct {
	Enabled    bool    `json:"enabled"`
	ChannelKey string  `json:"channel_key" binding:"required"`
	IsCLI      bool    `json:"is_cli"`
	Multiplier float64 `json:"multiplier"`
	Models     string  `json:"models"`
	Remark     string  `json:"remark"`
}

type ChannelKeyUpdateRequest struct {
	ID         int      `json:"id" binding:"required"`
	Enabled    *bool    `json:"enabled,omitempty"`
	ChannelKey *string  `json:"channel_key,omitempty"`
	IsCLI      *bool    `json:"is_cli,omitempty"`
	Multiplier *float64 `json:"multiplier,omitempty"`
	Models     *string  `json:"models,omitempty"`
	Remark     *string  `json:"remark,omitempty"`
}

// ChannelFetchModelRequest is used by /channel/fetch-model (not persisted).
type ChannelFetchModelRequest struct {
	Type    outbound.OutboundType `json:"type" binding:"required"`
	BaseURL string                `json:"base_url" binding:"required"`
	Key     string                `json:"key" binding:"required"`
	Proxy   bool                  `json:"proxy"`
}

func (c *Channel) GetBaseUrl() string {
	if c == nil || len(c.BaseUrls) == 0 {
		return ""
	}

	bestURL := ""
	bestDelay := 0
	bestSet := false

	for _, bu := range c.BaseUrls {
		if bu.URL == "" {
			continue
		}
		if !bestSet || bu.Delay < bestDelay {
			bestURL = bu.URL
			bestDelay = bu.Delay
			bestSet = true
		}
	}

	return bestURL
}

func (c *Channel) GetChannelKey() ChannelKey {
	return c.GetChannelKeyForModel("")
}

func (c *Channel) GetChannelKeyForModel(modelName string) ChannelKey {
	if c == nil || len(c.Keys) == 0 {
		return ChannelKey{}
	}

	nowSec := time.Now().Unix()

	best := ChannelKey{}
	bestScore := 0.0
	bestSet := false

	for _, k := range c.Keys {
		if !k.Enabled || k.ChannelKey == "" {
			continue
		}
		if !k.SupportsModel(modelName) {
			continue
		}
		if k.StatusCode == 429 && k.LastUseTimeStamp > 0 {
			if nowSec-k.LastUseTimeStamp < int64(5*time.Minute/time.Second) {
				continue
			}
		}
		score := k.SelectionScore()
		if !bestSet || score < bestScore {
			best = k
			bestScore = score
			bestSet = true
		}
	}

	if !bestSet {
		return ChannelKey{}
	}
	return best
}

func (c *Channel) GetModelFetchKey() ChannelKey {
	if c == nil || len(c.Keys) == 0 {
		return ChannelKey{}
	}
	probe := *c
	probe.Keys = make([]ChannelKey, 0, len(c.Keys))
	for _, k := range c.Keys {
		if k.IsCLI {
			continue
		}
		probe.Keys = append(probe.Keys, k)
	}
	return probe.GetChannelKey()
}

func (k ChannelKey) SupportsModel(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || strings.TrimSpace(k.Models) == "" {
		return true
	}
	for _, candidate := range strings.Split(k.Models, ",") {
		if strings.TrimSpace(candidate) == modelName {
			return true
		}
	}
	return false
}

func (k ChannelKey) NormalizedMultiplier() float64 {
	if k.Multiplier <= 0 || math.IsNaN(k.Multiplier) || math.IsInf(k.Multiplier, 0) {
		return 1
	}
	return k.Multiplier
}

func (k ChannelKey) SelectionScore() float64 {
	return k.TotalCost * k.NormalizedMultiplier()
}
