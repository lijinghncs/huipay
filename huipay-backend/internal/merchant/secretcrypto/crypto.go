// 包 secretcrypto 提供商户敏感字段的 AES-256-GCM 加解密。
package secretcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

const (
	// encKeyEnv 加密密钥环境变量名。
	encKeyEnv = "HUIPAY_WECHAT_CFG_ENC_KEY"
	// prefix 密文格式前缀；非此前缀按明文兼容读（迁移旧数据容错）。
	prefix = "v1:"
)

var (
	mu     sync.RWMutex
	aead   cipher.AEAD
	loaded bool
)

// appEnvEnv 环境变量：识别生产环境用于 fail-closed。
const appEnvEnv = "HUIPAY_APP_ENV"

// keyMissing 判定：生产环境密钥缺失时必须失败关闭（fail-closed），
// 避免退回随机密钥导致已存密文在重启后不可解。
func keyMissing() error {
	if os.Getenv(appEnvEnv) == "production" && os.Getenv(encKeyEnv) == "" {
		return fmt.Errorf("secretcrypto: %s is required in production (fail-closed)", encKeyEnv)
	}
	return nil
}

// load 加载并预构建 GCM AEAD。密钥来自环境变量；缺失时用随机密钥并告警（不阻断，提示生产必配）。
func load() cipher.AEAD {
	mu.RLock()
	if loaded {
		a := aead
		mu.RUnlock()
		return a
	}
	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()
	if loaded {
		return aead
	}

	key := []byte(os.Getenv(encKeyEnv))
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			panic(fmt.Errorf("secretcrypto: generate random key: %w", err))
		}
		fmt.Printf("[secretcrypto] WARNING: %s not set, using RANDOM key; stored secrets cannot be decoded after restart. Set it in production.\n", encKeyEnv)
	}
	// 归一化到 32 字节（AES-256）：超长截断，过短补零。
	if len(key) > 32 {
		key = key[:32]
	} else if len(key) < 32 {
		padded := make([]byte, 32)
		copy(padded, key)
		key = padded
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		panic(fmt.Errorf("secretcrypto: new cipher: %w", err))
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(fmt.Errorf("secretcrypto: new gcm: %w", err))
	}
	aead = gcm
	loaded = true
	return aead
}

// Encrypt 加密明文，返回带前缀的密文；空串原样返回。
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if err := keyMissing(); err != nil {
		return "", err
	}
	a := load()
	nonce := make([]byte, a.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secretcrypto: nonce: %w", err)
	}
	sealed := a.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密密文；空串返回空；非 v1: 前缀按明文兼容返回（容错旧数据）。
func Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if err := keyMissing(); err != nil {
		return "", err
	}
	if len(ciphertext) < len(prefix) || ciphertext[:len(prefix)] != prefix {
		return ciphertext, nil
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext[len(prefix):])
	if err != nil {
		return "", fmt.Errorf("secretcrypto: decode: %w", err)
	}
	a := load()
	if len(raw) < a.NonceSize() {
		return "", errors.New("secretcrypto: ciphertext too short")
	}
	nonce := raw[:a.NonceSize()]
	body := raw[a.NonceSize():]
	plain, err := a.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("secretcrypto: decrypt: %w", err)
	}
	return string(plain), nil
}