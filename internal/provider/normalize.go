package provider

import (
	"errors"

	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

var ErrTypeMismatch = errors.New("provider: type and provider_id are inconsistent")

// NormalizeChannelType 归一化 type 和 provider_id，确保两者一致
// 当两者同时提供但不一致时返回 ErrTypeMismatch
// 当 provider_id 未注册时返回错误
// 返回值: (providerID, legacyType, error)
func NormalizeChannelType(providerID *string, legacyType *outbound.OutboundType) (string, outbound.OutboundType, error) {
	pid := ""
	if providerID != nil {
		pid = *providerID
	}

	var lt outbound.OutboundType
	hasLegacy := false
	if legacyType != nil {
		lt = *legacyType
		hasLegacy = true
	}

	// 两者都未提供
	if pid == "" && !hasLegacy {
		return "", 0, nil
	}

	// 仅 provider_id
	if pid != "" && !hasLegacy {
		p := Get(ProviderID(pid))
		if p == nil {
			return "", 0, errors.New("provider: unknown provider_id " + pid)
		}
		if p.LegacyType() != nil {
			lt = *p.LegacyType()
			hasLegacy = true
		}
		return pid, lt, nil
	}

	// 仅 legacy type
	if pid == "" && hasLegacy {
		resolvedID := ResolveProviderIDFromType(lt)
		if resolvedID != "" {
			return string(resolvedID), lt, nil
		}
		return "", lt, nil
	}

	// 两者都提供 — 检查一致性
	p := Get(ProviderID(pid))
	if p == nil {
		return "", 0, errors.New("provider: unknown provider_id " + pid)
	}
	if p.LegacyType() != nil && *p.LegacyType() != lt {
		return "", 0, ErrTypeMismatch
	}
	if p.LegacyType() == nil && hasLegacy {
		return "", 0, ErrTypeMismatch
	}

	return pid, lt, nil
}