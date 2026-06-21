package model

import (
	"time"

	transformermodel "github.com/bestruirui/octopus/internal/transformer/model"
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
	ID            int            `json:"id" gorm:"primaryKey"`
	Name          string         `json:"name" gorm:"unique;not null"`
	Enabled       bool           `json:"enabled" gorm:"default:true"`
	BaseUrls      []BaseUrl      `json:"base_urls" gorm:"serializer:json"`
	Keys          []ChannelKey   `json:"keys" gorm:"foreignKey:ChannelID"`
	Model         string         `json:"model"`
	CustomModel   string         `json:"custom_model"`
	Proxy         bool           `json:"proxy" gorm:"default:false"`
	AutoSync      bool           `json:"auto_sync" gorm:"default:false"`
	AutoGroup     AutoGroupType  `json:"auto_group" gorm:"default:0"`
	CustomHeader  []CustomHeader `json:"custom_header" gorm:"serializer:json"`
	ParamOverride *string        `json:"param_override"`
	ChannelProxy  *string        `json:"channel_proxy"`
	Stats         *StatsChannel  `json:"stats,omitempty" gorm:"foreignKey:ChannelID"`
	MatchRegex    *string        `json:"match_regex"`
}

// BaseUrl 表示渠道的一个上游地址，每个地址可独立指定渠道类型。
type BaseUrl struct {
	URL        string                `json:"url"`
	Delay      int                   `json:"delay"`
	Type       outbound.OutboundType `json:"type"`
	ProviderID string                `json:"provider_id,omitempty"`
}

type CustomHeader struct {
	HeaderKey   string `json:"header_key"`
	HeaderValue string `json:"header_value"`
}

type ChannelKey struct {
	ID               int     `json:"id" gorm:"primaryKey"`
	ChannelID        int     `json:"channel_id"`
	Enabled          bool    `json:"enabled" gorm:"default:true"`
	ChannelKey       string  `json:"channel_key"`
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
	ID            int             `json:"id" binding:"required"`
	Name          *string         `json:"name,omitempty"`
	Enabled       *bool           `json:"enabled,omitempty"`
	BaseUrls      *[]BaseUrl      `json:"base_urls,omitempty"`
	Model         *string         `json:"model,omitempty"`
	CustomModel   *string         `json:"custom_model,omitempty"`
	Proxy         *bool           `json:"proxy,omitempty"`
	AutoSync      *bool           `json:"auto_sync,omitempty"`
	AutoGroup     *AutoGroupType  `json:"auto_group,omitempty"`
	CustomHeader  *[]CustomHeader `json:"custom_header,omitempty"`
	ChannelProxy  *string         `json:"channel_proxy,omitempty"`
	ParamOverride *string         `json:"param_override,omitempty"`
	MatchRegex    *string         `json:"match_regex,omitempty"`

	KeysToAdd    []ChannelKeyAddRequest    `json:"keys_to_add,omitempty"`
	KeysToUpdate []ChannelKeyUpdateRequest `json:"keys_to_update,omitempty"`
	KeysToDelete []int                     `json:"keys_to_delete,omitempty"`
}

type ChannelKeyAddRequest struct {
	Enabled    bool   `json:"enabled"`
	ChannelKey string `json:"channel_key" binding:"required"`
	Remark     string `json:"remark"`
}

type ChannelKeyUpdateRequest struct {
	ID         int     `json:"id" binding:"required"`
	Enabled    *bool   `json:"enabled,omitempty"`
	ChannelKey *string `json:"channel_key,omitempty"`
	Remark     *string `json:"remark,omitempty"`
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
	for _, bu := range c.BaseUrls {
		if bu.URL != "" {
			return bu.URL
		}
	}
	return ""
}

// SelectBaseUrl 根据请求格式选择最合适的 BaseUrl 及其渠道类型。
// 优先返回出站 APIFormat 与 rawAPIFormat 匹配的 URL（原生透传）；
// 无匹配时按延迟升序返回首个兼容 URL。
// isEmbeddingRequest=true 时仅考虑 embedding 类型，否则仅考虑 chat 类型。
// 返回 (url, outboundType, providerID, ok)。
func (c *Channel) SelectBaseUrl(rawAPIFormat transformermodel.APIFormat, isEmbeddingRequest bool) (string, outbound.OutboundType, string, bool) {
	if c == nil || len(c.BaseUrls) == 0 {
		return "", 0, "", false
	}

	type candidate struct {
		url        string
		outType    outbound.OutboundType
		providerID string
		delay      int
	}

	var compatible []candidate
	var passthrough *candidate

	for i := range c.BaseUrls {
		bu := &c.BaseUrls[i]
		if bu.URL == "" {
			continue
		}

		if isEmbeddingRequest {
			if !outbound.IsEmbeddingChannelType(bu.Type) {
				continue
			}
		} else {
			if !outbound.IsChatChannelType(bu.Type) {
				continue
			}
		}

		cand := candidate{url: bu.URL, outType: bu.Type, providerID: bu.ProviderID, delay: bu.Delay}
		compatible = append(compatible, cand)

		if passthrough == nil && rawAPIFormat != "" {
			if outFmt := outbound.OutboundTypeToAPIFormat(bu.Type); outFmt != "" && outFmt == rawAPIFormat {
				c := cand
				passthrough = &c
			}
		}
	}

	if passthrough != nil {
		return passthrough.url, passthrough.outType, passthrough.providerID, true
	}

	if len(compatible) == 0 {
		return "", 0, "", false
	}

	best := compatible[0]
	for i := 1; i < len(compatible); i++ {
		if compatible[i].delay < best.delay {
			best = compatible[i]
		}
	}
	return best.url, best.outType, best.providerID, true
}

// HasProvider 检查渠道的任一 BaseUrl 是否使用了指定 provider。
func (c *Channel) HasProvider(pid string) bool {
	if c == nil {
		return false
	}
	for _, bu := range c.BaseUrls {
		if bu.ProviderID == pid {
			return true
		}
	}
	return false
}

func (c *Channel) GetChannelKey() ChannelKey {
	if c == nil || len(c.Keys) == 0 {
		return ChannelKey{}
	}

	nowSec := time.Now().Unix()

	best := ChannelKey{}
	bestCost := 0.0
	bestSet := false

	for _, k := range c.Keys {
		if !k.Enabled || k.ChannelKey == "" {
			continue
		}
		if k.StatusCode == 429 && k.LastUseTimeStamp > 0 {
			if nowSec-k.LastUseTimeStamp < int64(5*time.Minute/time.Second) {
				continue
			}
		}
		if !bestSet || k.TotalCost < bestCost {
			best = k
			bestCost = k.TotalCost
			bestSet = true
		}
	}

	if !bestSet {
		return ChannelKey{}
	}
	return best
}
