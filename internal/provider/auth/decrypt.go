package auth

import (
	"encoding/json"

	"github.com/bestruirui/octopus/internal/provider"
)

// DecryptSessionData 解密 session 数据
func DecryptSessionData(encrypted string) (*provider.AuthSession, error) {
	if encrypted == "" {
		return nil, nil
	}
	data, err := Decrypt([]byte(encrypted))
	if err != nil {
		return nil, err
	}
	var session provider.AuthSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// DecryptResultData 解密 result 数据
func DecryptResultData(encrypted string) (*provider.AuthResult, error) {
	if encrypted == "" {
		return nil, nil
	}
	data, err := Decrypt([]byte(encrypted))
	if err != nil {
		return nil, err
	}
	var result provider.AuthResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
