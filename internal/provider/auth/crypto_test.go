package auth

import (
	"testing"
)

func TestEncryptDecrypt_NoKey(t *testing.T) {
	// 无密钥时，加密是透传
	plaintext := []byte("hello world")
	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("got %q, want %q", decrypted, plaintext)
	}
}

func TestSetEncryptionKey_InvalidHex(t *testing.T) {
	err := SetEncryptionKey("not-hex")
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestSetEncryptionKey_WrongLength(t *testing.T) {
	// 16 字节而非 32
	err := SetEncryptionKey("0123456789abcdef0123456789abcdef")
	if err == nil {
		t.Error("expected error for wrong key length")
	}
}

func TestEncryptDecrypt_WithKey(t *testing.T) {
	// 设置 32 字节密钥
	err := SetEncryptionKey("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("SetEncryptionKey error: %v", err)
	}

	plaintext := []byte("sensitive token data")
	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	if string(encrypted) == string(plaintext) {
		t.Error("encrypted should differ from plaintext")
	}

	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("got %q, want %q", decrypted, plaintext)
	}

	// 清除密钥（恢复无密钥状态）
	keyMu.Lock()
	encryptionKey = nil
	keyMu.Unlock()
}

func TestEncryptDecrypt_EmptyInput(t *testing.T) {
	plaintext := []byte("")
	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("got %q, want %q", decrypted, plaintext)
	}
}

func TestDecrypt_ShortCiphertext(t *testing.T) {
	err := SetEncryptionKey("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("SetEncryptionKey error: %v", err)
	}
	_, err = Decrypt([]byte("short"))
	if err == nil {
		t.Error("expected error for short ciphertext with key set")
	}

	// 清除密钥
	keyMu.Lock()
	encryptionKey = nil
	keyMu.Unlock()
}

func TestDecrypt_InvalidCiphertext(t *testing.T) {
	err := SetEncryptionKey("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("SetEncryptionKey error: %v", err)
	}
	// 12+ bytes but invalid GCM tag
	_, err = Decrypt([]byte("012345678901234567890123456789"))
	if err == nil {
		t.Error("expected error for invalid ciphertext")
	}

	// 清除密钥
	keyMu.Lock()
	encryptionKey = nil
	keyMu.Unlock()
}

func TestEncryptDecrypt_LargeInput(t *testing.T) {
	err := SetEncryptionKey("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("SetEncryptionKey error: %v", err)
	}

	plaintext := make([]byte, 10000)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}
	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Error("large input roundtrip failed")
	}

	// 清除密钥
	keyMu.Lock()
	encryptionKey = nil
	keyMu.Unlock()
}

func TestSetEncryptionKey_ValidKey(t *testing.T) {
	err := SetEncryptionKey("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Errorf("expected no error for valid key: %v", err)
	}

	// 清除密钥
	keyMu.Lock()
	encryptionKey = nil
	keyMu.Unlock()
}
