package agent

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"hephaestus-backend/pkg/machineidentity"
)

const (
	credentialCipherPrefix = "enc:v1:"
	credentialMaskedValue  = "********"
)

var (
	credentialKeyOnce sync.Once
	credentialAESKey  []byte
	credentialKeyErr  error
)

func isMaskedCredentialInput(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false
	}
	if trimmed == credentialMaskedValue {
		return true
	}
	// 兼容前端使用任意长度星号作为脱敏占位
	return len(trimmed) >= 6 && strings.Trim(trimmed, "*") == ""
}

func isEncryptedCredential(input string) bool {
	return strings.HasPrefix(strings.TrimSpace(input), credentialCipherPrefix)
}

func encryptCredential(plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", nil
	}
	if isEncryptedCredential(plain) {
		return plain, nil
	}
	key, err := getCredentialAESKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	cipherText := gcm.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, cipherText...)
	return credentialCipherPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

func decryptCredential(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", nil
	}
	if !isEncryptedCredential(secret) {
		// 兼容历史明文
		return secret, nil
	}
	raw := strings.TrimPrefix(secret, credentialCipherPrefix)
	payload, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("密文解码失败: %w", err)
	}
	key, err := getCredentialAESKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(payload) < nonceSize {
		return "", errors.New("密文格式错误")
	}
	nonce := payload[:nonceSize]
	cipherText := payload[nonceSize:]
	plain, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return "", fmt.Errorf("密文解密失败: %w", err)
	}
	return string(plain), nil
}

func getCredentialAESKey() ([]byte, error) {
	credentialKeyOnce.Do(func() {
		material, err := resolveCredentialKeyMaterial()
		if err != nil {
			credentialKeyErr = err
			return
		}
		sum := sha256.Sum256(material)
		credentialAESKey = sum[:]
	})
	if credentialKeyErr != nil {
		return nil, credentialKeyErr
	}
	if len(credentialAESKey) != 32 {
		return nil, errors.New("凭据加密密钥长度异常")
	}
	return credentialAESKey, nil
}

func resolveCredentialKeyMaterial() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("HEPHAESTUS_CREDENTIAL_AES_KEY"))
	if raw != "" {
		if strings.HasPrefix(raw, "base64:") {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(raw, "base64:")))
			if err != nil {
				return nil, fmt.Errorf("HEPHAESTUS_CREDENTIAL_AES_KEY base64 解码失败: %w", err)
			}
			if len(decoded) == 0 {
				return nil, errors.New("HEPHAESTUS_CREDENTIAL_AES_KEY(base64) 不能为空")
			}
			return decoded, nil
		}
		if len(raw) == 64 {
			if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) > 0 {
				return decoded, nil
			}
		}
		return []byte(raw), nil
	}

	// 回退到机器绑定值，避免未配置环境变量时不可用
	machineCode := strings.TrimSpace(machineidentity.GetMachineInfo().MachineCode)
	if machineCode != "" {
		return []byte("hephaestus-agent-credential:" + machineCode), nil
	}
	hostname, _ := os.Hostname()
	hostname = strings.TrimSpace(hostname)
	if hostname != "" {
		return []byte("hephaestus-agent-credential:" + hostname), nil
	}
	return nil, errors.New("无法生成凭据加密密钥，请设置 HEPHAESTUS_CREDENTIAL_AES_KEY")
}
