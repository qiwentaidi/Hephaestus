package scan

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"

	"github.com/gin-gonic/gin"
)

func ListScanAgentsHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	var agents []database.ScanAgentNode
	if err := db.Where("retired = ?", false).Order("last_heartbeat DESC").Find(&agents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询 Agent 列表失败: " + err.Error(),
		})
		return
	}

	staleThreshold := 45 * time.Second
	now := time.Now()
	result := make([]gin.H, 0, len(agents))
	for _, item := range agents {
		isAlive := item.Status == "online" && now.Sub(item.LastHeartbeat) <= staleThreshold
		effectiveEnabled := item.Enabled && isAlive
		effectiveStatus := item.Status
		if !isAlive {
			effectiveStatus = "offline"
		}
		result = append(result, gin.H{
			"aiWorker": item.AIWorker, "scanWorker": item.ScanWorker, "localWorker": item.LocalWorker,
			"aiConcurrency": item.AIConcurrency, "scanConcurrency": item.ScanConcurrency,
			"aiRunning": scantask.AgentRunningCount(db, item.AgentID), "scanRunning": scantask.ScanRunningCount(db, item.AgentID),
			"supportedTypes": item.SupportedTypes, "networkZones": item.NetworkZones,
			"agentId":        item.AgentID,
			"displayName":    item.DisplayName,
			"agentVersion":   item.AgentVersion,
			"updateProtocol": item.UpdateProtocol, "updatePending": item.UpdatePending,
			"updateOS": item.UpdateOS, "updateArch": item.UpdateArch,
			"enabled":            effectiveEnabled,
			"status":             effectiveStatus,
			"isAlive":            isAlive,
			"canDispatch":        effectiveEnabled && !item.UpdatePending,
			"lastHeartbeat":      item.LastHeartbeat,
			"lastSeenHost":       item.LastSeenHost,
			"lastSeenIp":         item.LastSeenIP,
			"cpuUsagePercent":    item.CPUUsagePercent,
			"memoryUsageBytes":   item.MemoryUsageBytes,
			"memoryTotalBytes":   item.MemoryTotalBytes,
			"memoryUsagePercent": item.MemoryUsagePercent,
			"diskUsageBytes":     item.DiskUsageBytes,
			"diskTotalBytes":     item.DiskTotalBytes,
			"diskUsagePercent":   item.DiskUsagePercent,
			"createdAt":          item.CreatedAt,
			"updatedAt":          item.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func SetScanAgentEnabledHandler(c *gin.Context) {
	agentID := c.Param("id")
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "agent id 不能为空",
		})
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效请求参数: " + err.Error(),
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

	now := time.Now()
	update := map[string]interface{}{
		"enabled":    req.Enabled,
		"updated_at": now,
	}
	result := db.Model(&database.ScanAgentNode{}).
		Where("agent_id = ?", agentID).
		Updates(update)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新 Agent 状态失败: " + result.Error.Error(),
		})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Agent 不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"agentId": agentID,
			"enabled": req.Enabled,
		},
	})
}

func UpdateScanAgentHandler(c *gin.Context) {
	agentID := strings.TrimSpace(c.Param("id"))
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "agent id 不能为空",
		})
		return
	}

	var rawReq map[string]json.RawMessage
	if err := c.ShouldBindJSON(&rawReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效请求参数: " + err.Error(),
		})
		return
	}
	if _, exists := rawReq["agentId"]; exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "agentId 不允许修改",
		})
		return
	}
	if _, exists := rawReq["agent_id"]; exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "agentId 不允许修改",
		})
		return
	}

	var req struct {
		DisplayName string `json:"displayName"`
	}
	if payload, exists := rawReq["displayName"]; exists {
		if err := json.Unmarshal(payload, &req.DisplayName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "displayName 参数无效",
			})
			return
		}
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "displayName 不能为空",
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

	result := db.Model(&database.ScanAgentNode{}).
		Where("agent_id = ?", agentID).
		Updates(map[string]interface{}{
			"display_name": displayName,
			"updated_at":   time.Now(),
		})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新 Agent 显示名失败: " + result.Error.Error(),
		})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Agent 不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"agentId":     agentID,
			"displayName": displayName,
		},
	})
}

func DeleteScanAgentHandler(c *gin.Context) {
	agentID := strings.TrimSpace(c.Param("id"))
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "agent id 不能为空",
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

	var agent database.ScanAgentNode
	if err := db.Where("agent_id = ?", agentID).First(&agent).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Agent 不存在",
		})
		return
	}

	staleThreshold := 45 * time.Second
	isAlive := agent.Status == "online" && time.Since(agent.LastHeartbeat) <= staleThreshold
	if isAlive {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "在线 Agent 不允许删除",
		})
		return
	}

	if err := db.Delete(&database.ScanAgentNode{}, "agent_id = ?", agentID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除 Agent 记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"agentId": agentID,
		},
	})
}
