package scan

import (
	"net/http"
	"runtime/debug"
	"strconv"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v3/mem"
)

const (
	defaultMaxConcurrentScanTasks = 3
	maxAllowedConcurrentScanTasks = 32
	defaultMemoryLimitMB          = 0
)

type ScanRuntimeConfig struct {
	MaxConcurrentTasks int `json:"maxConcurrentTasks"`
	MemoryLimitMB      int `json:"memoryLimitMB"`
}

func getMaxMemoryLimitMB() int {
	memStat, err := mem.VirtualMemory()
	if err != nil || memStat == nil || memStat.Total == 0 {
		return 8192
	}
	totalMB := int(memStat.Total / 1024 / 1024)
	if totalMB <= 0 {
		return 8192
	}
	return totalMB
}

func normalizeMaxConcurrentTasks(value int) int {
	if value <= 0 {
		return defaultMaxConcurrentScanTasks
	}
	if value > maxAllowedConcurrentScanTasks {
		return maxAllowedConcurrentScanTasks
	}
	return value
}

func parseMaxConcurrentTasks(raw interface{}) int {
	switch value := raw.(type) {
	case float64:
		return normalizeMaxConcurrentTasks(int(value))
	case float32:
		return normalizeMaxConcurrentTasks(int(value))
	case int:
		return normalizeMaxConcurrentTasks(value)
	case int64:
		return normalizeMaxConcurrentTasks(int(value))
	case string:
		if parsed, err := strconv.Atoi(value); err == nil {
			return normalizeMaxConcurrentTasks(parsed)
		}
	}
	return defaultMaxConcurrentScanTasks
}

func normalizeMemoryLimitMB(value int, maxLimitMB int) int {
	if value <= 0 {
		return defaultMemoryLimitMB
	}
	if value > maxLimitMB {
		return maxLimitMB
	}
	return value
}

func parseMemoryLimitMB(raw interface{}, maxLimitMB int) int {
	switch value := raw.(type) {
	case float64:
		return normalizeMemoryLimitMB(int(value), maxLimitMB)
	case float32:
		return normalizeMemoryLimitMB(int(value), maxLimitMB)
	case int:
		return normalizeMemoryLimitMB(value, maxLimitMB)
	case int64:
		return normalizeMemoryLimitMB(int(value), maxLimitMB)
	case string:
		if parsed, err := strconv.Atoi(value); err == nil {
			return normalizeMemoryLimitMB(parsed, maxLimitMB)
		}
	}
	return defaultMemoryLimitMB
}

func LoadScanRuntimeConfig() ScanRuntimeConfig {
	maxMemoryLimitMB := getMaxMemoryLimitMB()
	cfg := ScanRuntimeConfig{
		MaxConcurrentTasks: defaultMaxConcurrentScanTasks,
		MemoryLimitMB:      defaultMemoryLimitMB,
	}
	raw, err := database.GetSetting(database.SettingKeyScanRuntime)
	if err != nil || raw == nil {
		return cfg
	}

	if settings, ok := raw.(map[string]interface{}); ok {
		if value, exists := settings["maxConcurrentTasks"]; exists {
			cfg.MaxConcurrentTasks = parseMaxConcurrentTasks(value)
		}
		if value, exists := settings["memoryLimitMB"]; exists {
			cfg.MemoryLimitMB = parseMemoryLimitMB(value, maxMemoryLimitMB)
		}
	}

	return cfg
}

func ApplyScanRuntimeConfig(cfg ScanRuntimeConfig) {
	maxMemoryLimitMB := getMaxMemoryLimitMB()
	cfg.MemoryLimitMB = normalizeMemoryLimitMB(cfg.MemoryLimitMB, maxMemoryLimitMB)
	if cfg.MemoryLimitMB <= 0 {
		return
	}

	limitBytes := int64(cfg.MemoryLimitMB) * 1024 * 1024
	debug.SetMemoryLimit(limitBytes)
	logger.Info("Go 运行时内存预算已设置: %d MB", cfg.MemoryLimitMB)
}

func GetScanRuntimeConfigHandler(c *gin.Context) {
	config := LoadScanRuntimeConfig()
	maxMemoryLimitMB := getMaxMemoryLimitMB()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"maxConcurrentTasks": config.MaxConcurrentTasks,
			"memoryLimitMB":      config.MemoryLimitMB,
			"maxMemoryLimitMB":   maxMemoryLimitMB,
		},
	})
}

func UpdateScanRuntimeConfigHandler(c *gin.Context) {
	var req ScanRuntimeConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求体: " + err.Error(),
		})
		return
	}

	req.MaxConcurrentTasks = normalizeMaxConcurrentTasks(req.MaxConcurrentTasks)
	maxMemoryLimitMB := getMaxMemoryLimitMB()
	req.MemoryLimitMB = normalizeMemoryLimitMB(req.MemoryLimitMB, maxMemoryLimitMB)
	if err := database.SetSetting(database.SettingKeyScanRuntime, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存扫描运行时配置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "扫描运行时配置已保存，重启后生效",
		"data": gin.H{
			"config":           req,
			"maxMemoryLimitMB": maxMemoryLimitMB,
			"restartRequired":  true,
		},
	})
}
