package middleware

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
)

const (
	serverJWTTTL  = 24 * time.Hour
	desktopJWTTTL = 30 * 24 * time.Hour
)

var (
	serverJWTKey     []byte
	serverJWTKeyOnce sync.Once
	desktopJWTKey    []byte
	desktopJWTKeyErr error
	desktopJWTKeyMu  sync.Mutex
)

func initServerJWTKey() {
	serverJWTKeyOnce.Do(func() {
		if secret := strings.TrimSpace(os.Getenv("HEPHAESTUS_JWT_SECRET")); secret != "" {
			serverJWTKey = []byte(secret)
			return
		}

		serverJWTKey = make([]byte, 32)
		if _, err := rand.Read(serverJWTKey); err != nil {
			serverJWTKey = []byte("fallback-secret-key-" + time.Now().String())
		}
	})
}

func loadOrCreateDesktopJWTKey() ([]byte, error) {
	value, err := database.GetSetting(database.SettingKeyDesktopJWTSecret)
	if err != nil {
		return nil, fmt.Errorf("读取桌面端 JWT 密钥失败: %w", err)
	}
	if encoded, ok := value.(string); ok {
		encoded = strings.TrimSpace(encoded)
		if encoded != "" {
			key, decodeErr := base64.StdEncoding.DecodeString(encoded)
			if decodeErr == nil && len(key) >= 32 {
				return key, nil
			}
		}
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("生成桌面端 JWT 密钥失败: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(key)
	if err := database.SetSetting(database.SettingKeyDesktopJWTSecret, encoded); err != nil {
		return nil, fmt.Errorf("写入桌面端 JWT 密钥失败: %w", err)
	}

	return key, nil
}

func getDesktopJWTKey() ([]byte, error) {
	desktopJWTKeyMu.Lock()
	defer desktopJWTKeyMu.Unlock()

	if len(desktopJWTKey) > 0 {
		return desktopJWTKey, nil
	}

	desktopJWTKey, desktopJWTKeyErr = loadOrCreateDesktopJWTKey()
	return desktopJWTKey, desktopJWTKeyErr
}

func GetJWTKey() []byte {
	if config.GetRuntimeMode() == config.RuntimeModeDesktop {
		key, err := getDesktopJWTKey()
		if err == nil && len(key) > 0 {
			return key
		}
	}

	initServerJWTKey()
	return serverJWTKey
}

func getJWTKey() []byte {
	return GetJWTKey()
}

func getJWTTTL() time.Duration {
	if config.GetRuntimeMode() == config.RuntimeModeDesktop {
		return desktopJWTTTL
	}
	return serverJWTTTL
}

func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "未提供有效Token"})
			return
		}

		username, ok := authenticateBearerToken(authHeader)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		c.Set("username", username)
		c.Next()
	}
}

func authenticateBearerToken(authHeader string) (string, bool) {
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return "", false
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return getJWTKey(), nil
	})

	if err != nil || !token.Valid {
		return "", false
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", false
	}

	username, ok := claims["username"].(string)
	blmd5, ok2 := claims["blmd5"].(string)
	if !ok || !ok2 {
		return "", false
	}

	h := md5.New()
	h.Write([]byte("bl" + username))
	expect := hex.EncodeToString(h.Sum(nil))
	if blmd5 != expect {
		return "", false
	}

	return username, true
}

func GenerateToken(username string) (string, error) {
	h := md5.New()
	h.Write([]byte("bl" + username))
	blmd5 := hex.EncodeToString(h.Sum(nil))
	now := time.Now()
	claims := jwt.MapClaims{
		"username": username,
		"iat":      now.Unix(),
		"exp":      now.Add(getJWTTTL()).Unix(),
		"blmd5":    blmd5,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getJWTKey())
}
