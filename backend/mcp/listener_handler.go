package mcp

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"

	"hephaestus-backend/pkg/config"

	"github.com/gin-gonic/gin"
)

// resolveMCPMainPort 解析主服务端口，dev 模式下 PORT 环境变量优先
func resolveMCPMainPort() int {
	if value := strings.TrimSpace(os.Getenv("PORT")); value != "" {
		if port, err := strconv.Atoi(value); err == nil && port > 0 && port <= 65535 {
			return port
		}
	}
	return config.GetWebPort()
}

// GetMCPListenerHandler 获取当前 MCP 监听状态
func GetMCPListenerHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    GetMCPListenerState(resolveMCPMainPort()),
	})
}

// UpdateMCPListenerHandler 更新 MCP 监听配置并立即生效，生效后持久化到配置文件
func UpdateMCPListenerHandler(handler http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Host string `json:"host"`
			Port string `json:"port"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
			return
		}

		mainPort := resolveMCPMainPort()
		payload, err := ApplyMCPListener(context.Background(), handler, mainPort, req.Host, req.Port)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}

		host, _ := payload["mcpListenHost"].(string)
		port, _ := payload["mcpListenPort"].(string)
		if err := config.SetMCPListenConfig(host, port); err != nil {
			// 监听已生效，保持运行态不变，仅报告持久化失败
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "监听已生效但配置保存失败: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "data": payload})
	}
}
