package scan

import (
	"net/http"
	"strconv"

	"hephaestus-backend/pkg/dirsearch"

	"github.com/gin-gonic/gin"
)

// ListDirsearchPathHistoryHandler 获取路径历史记录列表
func ListDirsearchPathHistoryHandler(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "100")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	histories, err := dirsearch.ListPathHistory(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取路径历史记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    histories,
	})
}

// GetDirsearchPathHistoryHandler 根据路径获取历史记录
func GetDirsearchPathHistoryHandler(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "路径参数不能为空",
		})
		return
	}

	history, err := dirsearch.GetPathHistory(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取路径历史记录失败: " + err.Error(),
		})
		return
	}

	if history == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "未找到该路径的历史记录",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    history,
	})
}

// SearchDirsearchPathHistoryHandler 搜索路径历史记录
func SearchDirsearchPathHistoryHandler(c *gin.Context) {
	keyword := c.Query("keyword")
	limitStr := c.DefaultQuery("limit", "100")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}

	histories, err := dirsearch.SearchPathHistory(keyword, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "搜索路径历史记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    histories,
	})
}

// DeleteDirsearchPathHistoryHandler 删除路径历史记录
func DeleteDirsearchPathHistoryHandler(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "路径参数不能为空",
		})
		return
	}

	if err := dirsearch.DeletePathHistory(path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除路径历史记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}

// ClearDirsearchPathHistoryHandler 清空所有路径历史记录
func ClearDirsearchPathHistoryHandler(c *gin.Context) {
	if err := dirsearch.ClearPathHistory(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "清空路径历史记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "清空成功",
	})
}

// GetDirsearchPathHistoryStatsHandler 获取路径历史统计信息
func GetDirsearchPathHistoryStatsHandler(c *gin.Context) {
	stats, err := dirsearch.GetPathHistoryStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取统计信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}
