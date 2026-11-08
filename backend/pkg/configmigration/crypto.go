package configmigration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// encryptedValuePrefix marks a secret value that was encrypted for transport in
// an exported configuration bundle. The prefix makes the encryption scheme
// explicit and lets plaintext bundles keep working.
const encryptedValuePrefix = "enc:v1:"

// migrationEncryptionKey is intentionally a fixed, compiled-in key. It is not a
// replacement for per-installation secrets; it exists so an exported bundle can
// be imported by another Hephaestus installation without asking the operator to
// copy a key by hand. Treat the exported file as sensitive regardless.
var migrationEncryptionKey = sha256.Sum256([]byte("hephaestus:config-migration:secret:v1"))

// sensitiveKeys lists the configuration field names whose string values are
// encrypted on export. Keys are compared case-insensitively so both camelCase
// and snake_case payloads are covered.
var sensitiveKeys = map[string]struct{}{
	"accesskey":        {},
	"apikey":           {},
	"clientsecret":     {},
	"credentials":      {},
	"key":              {},
	"password":         {},
	"privatekey":       {},
	"secret":           {},
	"secretkey":        {},
	"smtppassword":     {},
	"token":            {},
	"webhookurl":       {},
	"webhook":          {},
	"authorization":    {},
	"auth":             {},
	"sessiontoken":     {},
	"refreshtoken":     {},
	"accesstoken":      {},
	"connectionstr":    {},
	"connectionstring": {},
}

func isSensitiveKey(key string) bool {
	_, ok := sensitiveKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

// encryptSecretString encrypts a single secret using AES-256-GCM and returns a
// prefixed, base64-encoded payload.
func encryptSecretString(plaintext string) (string, error) {
	block, err := aes.NewCipher(migrationEncryptionKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := make([]byte, 0, len(nonce)+len(sealed))
	payload = append(payload, nonce...)
	payload = append(payload, sealed...)
	return encryptedValuePrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

// decryptSecretString reverses encryptSecretString. Values without the
// encrypted prefix are returned unchanged so hand-edited bundles remain usable.
func decryptSecretString(value string) (string, error) {
	if !strings.HasPrefix(value, encryptedValuePrefix) {
		return value, nil
	}
	encoded := strings.TrimPrefix(value, encryptedValuePrefix)
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("配置密文格式无效: %w", err)
	}
	block, err := aes.NewCipher(migrationEncryptionKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < gcm.NonceSize() {
		return "", errors.New("配置密文长度无效")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("配置密文无法解密，请确认文件来自本系统")
	}
	return string(plaintext), nil
}

// transformSecrets walks an arbitrary JSON value and applies fn to every
// sensitive string leaf. Non-sensitive values are copied recursively.
func transformSecrets(value any, fn func(string) (string, error)) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isSensitiveKey(key) {
				if text, ok := child.(string); ok && strings.TrimSpace(text) != "" {
					transformed, err := fn(text)
					if err != nil {
						return nil, err
					}
					typed[key] = transformed
					continue
				}
			}
			transformed, err := transformSecrets(child, fn)
			if err != nil {
				return nil, err
			}
			typed[key] = transformed
		}
		return typed, nil
	case []any:
		for index, child := range typed {
			transformed, err := transformSecrets(child, fn)
			if err != nil {
				return nil, err
			}
			typed[index] = transformed
		}
		return typed, nil
	default:
		return value, nil
	}
}

func encryptSecrets(value any) (any, error) {
	return transformSecrets(value, func(text string) (string, error) {
		if strings.HasPrefix(text, encryptedValuePrefix) {
			return text, nil
		}
		return encryptSecretString(text)
	})
}

func decryptSecrets(value any) (any, error) {
	return transformSecrets(value, decryptSecretString)
}
