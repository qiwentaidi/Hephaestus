package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/notification"
)

// GetConfigHandler 获取所有配置
func GetConfigHandler(c *gin.Context) {
	cfg, err := config.LoadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "加载配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

// GetSpaceEngineConfigHandler 获取空间引擎配置
func GetSpaceEngineConfigHandler(c *gin.Context) {
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "加载配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": spaceConfig})
}

// UpdateSpaceEngineConfigHandler 更新空间引擎配置
func UpdateSpaceEngineConfigHandler(c *gin.Context) {
	var spaceConfig config.SpaceEngineConfig
	if err := c.ShouldBindJSON(&spaceConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	if err := config.UpdateSpaceEngineConfig(spaceConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "配置已保存"})
}

// UpdateConfigHandler 更新完整配置
func UpdateConfigHandler(c *gin.Context) {
	var cfg config.Config
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	if err := config.SaveConfig(&cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "配置已保存"})
}

// GetDatabaseConfigHandler 获取数据库配置
func GetDatabaseConfigHandler(c *gin.Context) {
	dbConfig, err := config.GetDatabaseConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "加载配置失败: " + err.Error()})
		return
	}
	// 不返回密码字段，只返回其他配置信息
	response := gin.H{
		"type":    normalizedDatabaseType(dbConfig.Type),
		"path":    dbConfig.Path,
		"host":    dbConfig.Host,
		"port":    dbConfig.Port,
		"user":    dbConfig.User,
		"dbname":  dbConfig.DBName,
		"sslmode": dbConfig.SSLMode,
		"runtime": gin.H{
			"mode":                 config.GetRuntimeMode(),
			"allowedDatabaseTypes": config.AllowedDatabaseTypes(),
		},
		// password 字段不返回，避免泄露
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

// UpdateDatabaseConfigHandler 更新数据库配置
func UpdateDatabaseConfigHandler(c *gin.Context) {
	var dbConfig config.DatabaseConfig
	if err := c.ShouldBindJSON(&dbConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	dbType := normalizedDatabaseType(dbConfig.Type)
	if !config.IsDatabaseTypeAllowed(dbType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "当前运行模式不允许该数据库类型",
			"data": gin.H{
				"mode":                 config.GetRuntimeMode(),
				"allowedDatabaseTypes": config.AllowedDatabaseTypes(),
			},
		})
		return
	}
	dbConfig.Type = dbType

	if err := config.UpdateDatabaseConfig(dbConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "数据库配置已保存"})
}

func normalizedDatabaseType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "postgres", "postgresql":
		return config.DatabaseTypePostgres
	case "sqlite", "sqlite3":
		return config.DatabaseTypeSQLite
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// GetDingTalkConfigHandler 获取钉钉配置
func GetDingTalkConfigHandler(c *gin.Context) {
	dingTalkConfig, err := config.GetDingTalkConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "加载配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dingTalkConfig})
}

// UpdateDingTalkConfigHandler 更新钉钉配置
func UpdateDingTalkConfigHandler(c *gin.Context) {
	var dingTalkConfig config.DingTalkConfig
	if err := c.ShouldBindJSON(&dingTalkConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	if err := config.UpdateDingTalkConfig(dingTalkConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "钉钉配置已保存"})
}

// TestDingTalkNotificationHandler 测试钉钉通知连通性
func TestDingTalkNotificationHandler(c *gin.Context) {
	var req struct {
		WebhookURL string `json:"webhookUrl"`
		Secret     string `json:"secret"`
	}

	if err := c.ShouldBindJSON(&req); err != nil || req.WebhookURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Webhook URL 不能为空"})
		return
	}

	if err := notification.SendDingTalkTestNotification(req.WebhookURL, req.Secret); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "发送测试通知失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "测试通知已发送"})
}

// GetWeChatConfigHandler 获取企业微信配置
func GetWeChatConfigHandler(c *gin.Context) {
	weChatConfig, err := config.GetWeChatConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "加载配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": weChatConfig})
}

// UpdateWeChatConfigHandler 更新企业微信配置
func UpdateWeChatConfigHandler(c *gin.Context) {
	var weChatConfig config.WeChatConfig
	if err := c.ShouldBindJSON(&weChatConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	if err := config.UpdateWeChatConfig(weChatConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "企业微信配置已保存"})
}

// TestWeChatNotificationHandler 测试企业微信通知
func TestWeChatNotificationHandler(c *gin.Context) {
	var req struct {
		WebhookURL string `json:"webhookUrl"`
	}

	if err := c.ShouldBindJSON(&req); err != nil || req.WebhookURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Webhook URL 不能为空"})
		return
	}

	if err := notification.SendWeChatTestNotification(req.WebhookURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "发送测试通知失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "测试通知已发送"})
}

// GetFeishuConfigHandler 获取飞书配置
func GetFeishuConfigHandler(c *gin.Context) {
	feishuConfig, err := config.GetFeishuConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "加载配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": feishuConfig})
}

// UpdateFeishuConfigHandler 更新飞书配置
func UpdateFeishuConfigHandler(c *gin.Context) {
	var feishuConfig config.FeishuConfig
	if err := c.ShouldBindJSON(&feishuConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	if err := config.UpdateFeishuConfig(feishuConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "飞书配置已保存"})
}

// TestFeishuNotificationHandler 测试飞书通知
func TestFeishuNotificationHandler(c *gin.Context) {
	var req struct {
		WebhookURL string `json:"webhookUrl"`
		Secret     string `json:"secret"`
	}

	if err := c.ShouldBindJSON(&req); err != nil || req.WebhookURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Webhook URL 不能为空"})
		return
	}

	if err := notification.SendFeishuTestNotification(req.WebhookURL, req.Secret); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "发送测试通知失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "测试通知已发送"})
}

// GetNotificationConfigHandler 获取统一通知配置
func GetNotificationConfigHandler(c *gin.Context) {
	notificationConfig, err := config.GetNotificationConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "加载统一通知配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": notificationConfig})
}

// UpdateNotificationConfigHandler 更新统一通知配置
func UpdateNotificationConfigHandler(c *gin.Context) {
	var notificationConfig config.NotificationConfig
	if err := c.ShouldBindJSON(&notificationConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的通知配置: " + err.Error()})
		return
	}
	if err := config.UpdateNotificationConfig(notificationConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存统一通知配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "统一通知配置已保存"})
}

// TestNotificationChannelHandler sends a test event through one configured channel.
func TestNotificationChannelHandler(c *gin.Context) {
	var req struct {
		Channel config.NotificationChannelConfig `json:"channel"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的通知通道配置: " + err.Error()})
		return
	}
	if err := notification.SendTest(req.Channel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "发送测试通知失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "测试通知已发送"})
}

// GetAIConfigHandler 获取AI配置
func GetAIConfigHandler(c *gin.Context) {
	aiConfig, err := config.GetAIConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "加载配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": aiConfig})
}

// UpdateAIConfigHandler 更新AI配置
func UpdateAIConfigHandler(c *gin.Context) {
	var aiConfig config.AIConfig
	if err := c.ShouldBindJSON(&aiConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	if err := config.ValidateAIRequestTimeouts(aiConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := config.UpdateAIConfig(aiConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "AI配置已保存"})
}

// TestAIConfigHandler 测试 AI 服务可用性
func TestAIConfigHandler(c *gin.Context) {
	var aiConfig config.AIConfig
	if err := c.ShouldBindJSON(&aiConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	if err := ai.TestAvailability(ctx, aiConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "AI 服务可用"})
}

// ListAIModelsHandler discovers models using the unsaved provider form.
func ListAIModelsHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var provider config.AIProviderConfig
	if err := c.ShouldBindJSON(&provider); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "invalid_config"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	models, err := ai.ListModels(ctx, provider)
	if err != nil {
		code := "request_failed"
		var listError *ai.ModelListError
		if errors.As(err, &listError) {
			code = listError.Code
		} else if errors.Is(err, context.DeadlineExceeded) {
			code = "timeout"
		}
		// Upstream 401 must not sign the user out of Hephaestus.
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": code})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": models})
}

// GetUpdateServerHandler 获取更新服务器地址
func GetUpdateServerHandler(c *gin.Context) {
	// Keep this endpoint for backwards-compatible clients, but never expose a
	// mutable setting. All update requests use the compiled-in trust root.
	c.JSON(http.StatusOK, gin.H{"success": true, "data": DefaultUpdateServer})
}

// UpdateUpdateServerHandler 更新服务器地址
func UpdateUpdateServerHandler(c *gin.Context) {
	c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "更新服务器地址由系统内置，用户不可修改"})
}
