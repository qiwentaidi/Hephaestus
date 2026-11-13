package scan

import (
	"net/http"

	"hephaestus-backend/pkg/webscan"

	"github.com/gin-gonic/gin"
)

// GetPOCTemplateCacheStatusHandler 返回 PoC 模板缓存预热状态，供桌面端启动遮罩轮询。
func GetPOCTemplateCacheStatusHandler(c *gin.Context) {
	state, errMsg, elapsedSeconds := webscan.POCTemplateCacheStatus()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"state":          state,
			"error":          errMsg,
			"elapsedSeconds": elapsedSeconds,
		},
	})
}

// WarmupPOCTemplateCacheHandler 在预热失败后触发重试。
func WarmupPOCTemplateCacheHandler(c *gin.Context) {
	webscan.RetryPOCTemplateCacheWarmup()
	c.JSON(http.StatusAccepted, gin.H{"success": true})
}
