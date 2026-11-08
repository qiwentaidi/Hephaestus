package space

import (
	"net/http"
	"strconv"
	"strings"

	"hephaestus-backend/handlers/platform"
	dbspace "hephaestus-backend/pkg/database/space"

	"github.com/gin-gonic/gin"
)

func listSpaceEngineSearchHistory(c *gin.Context, source string) {
	username := strings.TrimSpace(platform.Username(c))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	records, err := dbspace.ListSpaceEngineSearchHistory(source, username, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取搜索历史失败: " + err.Error(),
		})
		return
	}

	items := make([]string, 0, len(records))
	for _, record := range records {
		items = append(items, record.Query)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

func saveSpaceEngineSearchHistory(c *gin.Context, source string) {
	username := strings.TrimSpace(platform.Username(c))
	var req struct {
		Query string `json:"query" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	if err := dbspace.SaveSpaceEngineSearchHistory(source, req.Query, username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存搜索历史失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "搜索历史已保存",
	})
}

func ListFofaSearchHistoryHandler(c *gin.Context) {
	listSpaceEngineSearchHistory(c, "fofa")
}

func SaveFofaSearchHistoryHandler(c *gin.Context) {
	saveSpaceEngineSearchHistory(c, "fofa")
}

func ListHunterSearchHistoryHandler(c *gin.Context) {
	listSpaceEngineSearchHistory(c, "hunter")
}

func SaveHunterSearchHistoryHandler(c *gin.Context) {
	saveSpaceEngineSearchHistory(c, "hunter")
}

func ListQuakeSearchHistoryHandler(c *gin.Context) {
	listSpaceEngineSearchHistory(c, "quake")
}

func SaveQuakeSearchHistoryHandler(c *gin.Context) {
	saveSpaceEngineSearchHistory(c, "quake")
}
