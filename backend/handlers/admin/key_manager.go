package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/dgwork"

	"github.com/gin-gonic/gin"
	"github.com/qiwentaidi/clients"
)

type keyManagerRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    interface{}       `json:"body"`
	Timeout int               `json:"timeout"`
	Proxy   string            `json:"proxy"`
}

type keyManagerEntryPayload struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	BaseURL     string `json:"baseUrl"`
	Credentials string `json:"credentials"`
	Headers     string `json:"headers"`
	Notes       string `json:"notes"`
}

type dgworkTokenRequest struct {
	BaseURL   string            `json:"baseURL"`
	AppKey    string            `json:"appKey"`
	AppSecret string            `json:"appSecret"`
	Headers   map[string]string `json:"headers"`
	Proxy     string            `json:"proxy"`
}

// KeyManagerRequestHandler 统一代理密钥相关请求，避免浏览器跨域限制
func KeyManagerRequestHandler(c *gin.Context) {
	var req keyManagerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	if strings.TrimSpace(req.URL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求 URL 不能为空",
		})
		return
	}

	var bodyReader io.Reader
	switch val := req.Body.(type) {
	case nil:
		bodyReader = nil
	case string:
		if strings.TrimSpace(val) != "" {
			bodyReader = strings.NewReader(val)
		}
	case map[string]interface{}, []interface{}:
		payload, err := json.Marshal(val)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "请求体序列化失败: " + err.Error(),
			})
			return
		}
		bodyReader = bytes.NewReader(payload)
	default:
		payload, err := json.Marshal(val)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "请求体序列化失败: " + err.Error(),
			})
			return
		}
		bodyReader = bytes.NewReader(payload)
	}

	headers := map[string]string{}
	for k, v := range req.Headers {
		headers[k] = v
	}
	if bodyReader != nil {
		if _, ok := headers["Content-Type"]; !ok {
			headers["Content-Type"] = "application/json"
		}
	}

	client := clients.NewRestyClientWithProxy(nil, true, req.Proxy)
	resp, err := clients.DoRequest(method, req.URL, headers, bodyReader, req.Timeout, client)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"status":  resp.StatusCode(),
		"body":    string(resp.Body()),
		"headers": resp.Header(),
	})
}

// KeyManagerDGWorkTokenHandler 获取专有钉钉 AccessToken
func KeyManagerDGWorkTokenHandler(c *gin.Context) {
	var req dgworkTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" || req.AppKey == "" || req.AppSecret == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "baseURL、appKey、appSecret 不能为空",
		})
		return
	}

	xForwardedFor := ""
	if req.Headers != nil {
		if value, ok := req.Headers["X-Forwarded-For"]; ok {
			xForwardedFor = value
		} else if value, ok := req.Headers["x-forwarded-for"]; ok {
			xForwardedFor = value
		}
	}
	dgworkClient := dgwork.NewDGworkClient(baseURL, req.AppKey, req.AppSecret, xForwardedFor)
	token := dgworkClient.GetToken(dgworkClient.GETURL("/gettoken.json"), req.Proxy)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"body":    token,
		"status":  200,
		"headers": nil,
	})
}

// ListKeyManagerEntriesHandler 获取密钥条目列表
func ListKeyManagerEntriesHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	var entries []database.KeyManagerEntry
	if err := db.Order("updated_at DESC").Find(&entries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取密钥列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    entries,
	})
}

// CreateKeyManagerEntryHandler 创建密钥条目
func CreateKeyManagerEntryHandler(c *gin.Context) {
	var payload keyManagerEntryPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Type) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "name 和 type 不能为空",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	entry := database.KeyManagerEntry{
		ID:          database.GenerateNanoid(),
		Name:        payload.Name,
		Type:        payload.Type,
		BaseURL:     payload.BaseURL,
		Credentials: payload.Credentials,
		Headers:     payload.Headers,
		Notes:       payload.Notes,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.Create(&entry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建密钥条目失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    entry,
	})
}

// GetKeyManagerEntryHandler 获取密钥条目
func GetKeyManagerEntryHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "id 不能为空",
		})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}
	var entry database.KeyManagerEntry
	if err := db.Where("id = ?", id).First(&entry).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "密钥条目不存在",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    entry,
	})
}

// UpdateKeyManagerEntryHandler 更新密钥条目
func UpdateKeyManagerEntryHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "id 不能为空",
		})
		return
	}

	var payload keyManagerEntryPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	updates := map[string]interface{}{
		"name":        payload.Name,
		"type":        payload.Type,
		"base_url":    payload.BaseURL,
		"credentials": payload.Credentials,
		"headers":     payload.Headers,
		"notes":       payload.Notes,
		"updated_at":  time.Now(),
	}

	if err := db.Model(&database.KeyManagerEntry{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新密钥条目失败: " + err.Error(),
		})
		return
	}

	var entry database.KeyManagerEntry
	_ = db.Where("id = ?", id).First(&entry).Error

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    entry,
	})
}

// DeleteKeyManagerEntryHandler 删除密钥条目
func DeleteKeyManagerEntryHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "id 不能为空",
		})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}
	if err := db.Where("id = ?", id).Delete(&database.KeyManagerEntry{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除密钥条目失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}
