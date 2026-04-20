package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
)

var (
	encryptionKey []byte
	keyOnce       sync.Once
	keyMu         sync.RWMutex
)

// SetEncryptionKey 设置 AES-256-GCM 加密密钥（32 字节）
func SetEncryptionKey(key string) error {
	keyMu.Lock()
	defer keyMu.Unlock()
	decoded, err := hex.DecodeString(key)
	if err != nil {
		return fmt.Errorf("invalid hex key: %w", err)
	}
	if len(decoded) != 32 {
		return fmt.Errorf("key must be 32 bytes (64 hex chars), got %d", len(decoded))
	}
	encryptionKey = decoded
	return nil
}

// GetEncryptionKey 获取当前加密密钥
func GetEncryptionKey() []byte {
	keyMu.RLock()
	defer keyMu.RUnlock()
	return encryptionKey
}

// Encrypt 使用 AES-256-GCM 加密
func Encrypt(plaintext []byte) ([]byte, error) {
	key := GetEncryptionKey()
	if len(key) == 0 {
		// 无密钥时不加密（开发模式）
		return plaintext, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt 使用 AES-256-GCM 解密
func Decrypt(ciphertext []byte) ([]byte, error) {
	key := GetEncryptionKey()
	if len(key) == 0 {
		// 无密钥时不解密（开发模式）
		return ciphertext, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
