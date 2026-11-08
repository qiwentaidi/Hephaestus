package scan

import (
	"hephaestus-backend/pkg/scanexec"
	"hephaestus-backend/pkg/scantask"
	"maps"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// 扫描类型支持以及处理函数映射
// @ 介绍一下几个基础参数的内容
// context.Context: 任务级别的上下文对象，用于支持任务取消等功能
// string: 任务ID
// []string: 目标列表
// map[string]interface{}: 配置参数
var scanTypeHandlers = scanexec.Handlers

// CreateScanTaskHandler 创建扫描任务
func CreateScanTaskHandler(c *gin.Context) {
	var rawReq map[string]interface{}
	if err := c.ShouldBindJSON(&rawReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "there is an error in the parms",
		})
		return
	}

	// 判断任务类型
	rawScanType, ok := rawReq["scanType"]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "scanType is required",
		})
		return
	}

	scanType, ok := rawScanType.(string)
	if !ok || scanType == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "scanType must be a non-empty string",
		})
		return
	}

	// 提取 taskName
	var taskName string
	if taskNameVal, ok := rawReq["taskName"].(string); ok && taskNameVal != "" {
		taskName = taskNameVal
	}

	rawScanMode := ""
	if val, ok := rawReq["scanMode"].(string); ok && val != "" {
		rawScanMode = val
	} else if val, ok := rawReq["scan_mode"].(string); ok && val != "" {
		rawScanMode = val
	} else if val, ok := rawReq["mode"].(string); ok && val != "" {
		rawScanMode = val
	}
	assignedAgentID := ""
	if val, ok := rawReq["agentId"].(string); ok && val != "" {
		assignedAgentID = strings.TrimSpace(val)
	} else if val, ok := rawReq["agent_id"].(string); ok && val != "" {
		assignedAgentID = strings.TrimSpace(val)
	}

	configObj, ok := rawReq["config"].(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "config is required",
		})
		return
	}
	if rawScanMode == "" {
		if val, ok := configObj["scanMode"].(string); ok && val != "" {
			rawScanMode = val
		} else if val, ok := configObj["scan_mode"].(string); ok && val != "" {
			rawScanMode = val
		}
	}
	if assignedAgentID == "" {
		if val, ok := configObj["agentId"].(string); ok && val != "" {
			assignedAgentID = strings.TrimSpace(val)
		} else if val, ok := configObj["agent_id"].(string); ok && val != "" {
			assignedAgentID = strings.TrimSpace(val)
		}
	}
	scanMode, err := scantask.NormalizeMode(rawScanMode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	if scanMode == "local" {
		assignedAgentID = ""
	}

	schedule := scantask.ScheduleConfig{}
	if rawSchedule, ok := rawReq["schedule"].(map[string]interface{}); ok {
		parsed, err := scantask.ParseScheduleConfigFromMap(rawSchedule)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		schedule = parsed
	}

	targetsInterface, ok := configObj["targets"]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "config.targets is required",
		})
		return
	}

	var targets []string
	if targetsArray, ok := targetsInterface.([]interface{}); ok {
		for _, t := range targetsArray {
			if str, ok := t.(string); ok {
				targets = append(targets, str)
			}
		}
	}

	if len(targets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "targets 不能为空",
		})
		return
	}

	// 构建配置对象，支持两种格式
	config := make(map[string]interface{})

	// 检查是否有配置参数
	if configObj, ok := rawReq["config"].(map[string]interface{}); ok {
		maps.Copy(config, configObj)
	}

	taskID, err := scantask.CreateAndEnqueue(scanType, taskName, scanMode, assignedAgentID, targets, config, schedule)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"taskId":  taskID,
		"data": gin.H{
			"taskId":   taskID,
			"scanMode": scanMode,
			"agentId":  assignedAgentID,
		},
	})
}
