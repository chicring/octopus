package provider

import (
	"fmt"
	"sync"

	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

var (
	providers   = make(map[ProviderID]Provider)
	legacyMap   = make(map[outbound.OutboundType]ProviderID)
	providerMu  sync.RWMutex
)

// Register 注册一个 provider，重复 ID 会 panic
func Register(p Provider) {
	providerMu.Lock()
	defer providerMu.Unlock()

	id := p.ID()
	if _, exists := providers[id]; exists {
		panic(fmt.Sprintf("provider: duplicate registration for %q", id))
	}
	providers[id] = p

	if lt := p.LegacyType(); lt != nil {
		if _, exists := legacyMap[*lt]; exists {
			panic(fmt.Sprintf("provider: duplicate legacy type mapping for %d", *lt))
		}
		legacyMap[*lt] = id
	}
}

// Get 根据 ProviderID 获取 provider
func Get(id ProviderID) Provider {
	providerMu.RLock()
	defer providerMu.RUnlock()
	return providers[id]
}

// GetByLegacyType 根据遗留 OutboundType 获取 provider
func GetByLegacyType(t outbound.OutboundType) Provider {
	providerMu.RLock()
	defer providerMu.RUnlock()
	if id, ok := legacyMap[t]; ok {
		return providers[id]
	}
	return nil
}

// List 返回所有已注册的 provider
func List() []Provider {
	providerMu.RLock()
	defer providerMu.RUnlock()
	result := make([]Provider, 0, len(providers))
	for _, p := range providers {
		result = append(result, p)
	}
	return result
}

// GetOutbound 根据 ProviderID 获取 outbound 实例
func GetOutbound(id ProviderID) model.Outbound {
	p := Get(id)
	if p == nil {
		return nil
	}
	factory := p.OutboundFactory()
	if factory == nil {
		return nil
	}
	return factory()
}

// GetOutboundByLegacyType 根据遗留类型获取 outbound 实例
func GetOutboundByLegacyType(t outbound.OutboundType) model.Outbound {
	p := GetByLegacyType(t)
	if p == nil {
		return nil
	}
	factory := p.OutboundFactory()
	if factory == nil {
		return nil
	}
	return factory()
}
