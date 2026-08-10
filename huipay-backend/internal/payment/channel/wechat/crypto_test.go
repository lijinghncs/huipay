package wechat

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// aesGCMEncrypt 用 APIv3 密钥加密（与 aesGCMDecrypt 对称，测试用）。
func aesGCMEncrypt(t *testing.T, key, aad, nonce, plaintext []byte) string {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new gcm: %v", err)
	}
	ct := gcm.Seal(nil, nonce, plaintext, aad)
	return base64.StdEncoding.EncodeToString(ct)
}

func TestAesGCMDecryptRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef") // 32 字节
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand: %v", err)
	}
	aad := "transaction"
	plaintext := []byte(`{"out_trade_no":"HP20260801","trade_state":"SUCCESS"}`)

	cipherB64 := aesGCMEncrypt(t, key, []byte(aad), nonce, plaintext)

	got, err := aesGCMDecrypt(string(key), aad, string(nonce), cipherB64)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("decrypt mismatch:\n got %s\nwant %s", got, plaintext)
	}
}

func TestAesGCMDecryptWrongKey(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand: %v", err)
	}
	cipherB64 := aesGCMEncrypt(t, key, []byte("aad1"), nonce, []byte("hello"))

	wrongKey := string([]byte("00000000000000000000000000000000"))
	if _, err := aesGCMDecrypt(wrongKey, "aad1", string(nonce), cipherB64); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}

func TestAesGCMDecryptWrongKeyLen(t *testing.T) {
	if _, err := aesGCMDecrypt("short", "aad", "nonce", "cipher"); err == nil {
		t.Fatal("api_v3_key length check should reject short key")
	}
}