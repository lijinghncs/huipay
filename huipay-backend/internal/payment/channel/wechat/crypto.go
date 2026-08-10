// 回调解密：微信支付回调报文使用 APIv3 密钥 AES-256-GCM 加密。
package wechat

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
)

// aesGCMDecrypt 使用 APIv3 密钥解密微信回调的 resource.ciphertext。
// apiV3Key 必须为 32 字节；微信回调的 resource.nonce 为 12 字节。
func aesGCMDecrypt(apiV3Key, associatedData, nonce, ciphertextB64 string) ([]byte, error) {
	key := []byte(apiV3Key)
	if len(key) != 32 {
		return nil, fmt.Errorf("wechat: api_v3_key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("wechat: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("wechat: new gcm: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("wechat: decode ciphertext: %w", err)
	}
	plaintext, err := gcm.Open(nil, []byte(nonce), ciphertext, []byte(associatedData))
	if err != nil {
		return nil, fmt.Errorf("wechat: decrypt notify fail: %w", err)
	}
	return plaintext, nil
}