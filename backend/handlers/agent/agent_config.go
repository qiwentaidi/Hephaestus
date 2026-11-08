package agent

import (
	"net/http"
	"strings"

	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
)

type agentConfigRequest struct {
	Server struct {
		Address string `json:"address"`
	} `json:"server"`
}

func GetAgentConfigHandler(c *gin.Context) {
	cfg, err := database.LoadAgentServerSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取 Agent 配置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    cfg,
	})
}

func UpdateAgentConfigHandler(c *gin.Context) {
	var req agentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	req.Server.Address = strings.TrimSpace(req.Server.Address)
	if req.Server.Address == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Agent 服务端地址不能为空",
		})
		return
	}

	if err := database.SetSetting(database.SettingKeyAgentConfig, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存 Agent 配置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent 配置已保存",
		"data":    req,
	})
}
