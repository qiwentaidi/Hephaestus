package space

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"hephaestus-backend/handlers/platform"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
)

func ListSpaceExportTasksHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	username := platform.Username(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	engineName := strings.TrimSpace(strings.ToLower(c.Query("engine")))

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}

	query := db.Model(&database.SpaceExportTask{}).Where("created_by = ?", username)
	if engineName != "" {
		query = query.Where("engine = ?", engineName)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询导出任务失败: " + err.Error()})
		return
	}

	var tasks []database.SpaceExportTask
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取导出任务失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"list":     tasks,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

func GetSpaceExportTaskHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	username := platform.Username(c)
	id := c.Param("id")

	var task database.SpaceExportTask
	if err := db.Where("id = ? AND created_by = ?", id, username).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "导出任务不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    task,
	})
}

func DownloadSpaceExportTaskHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	username := platform.Username(c)
	id := c.Param("id")

	var task database.SpaceExportTask
	if err := db.Where("id = ? AND created_by = ?", id, username).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "导出任务不存在"})
		return
	}
	if task.Status != "success" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "导出任务尚未完成"})
		return
	}
	if task.FilePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "导出文件不存在"})
		return
	}
	if _, err := os.Stat(task.FilePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "导出文件不存在或已被清理"})
		return
	}

	format := strings.ToLower(strings.TrimSpace(task.Format))
	if format == "" {
		format = "csv"
	}
	downloadName := ensureFileNameExtension(sanitizeFileName(task.FileName), format)

	contentType := "application/octet-stream"
	if format == "csv" {
		contentType = "text/csv; charset=utf-8"
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", downloadName))
	c.Header("Content-Type", contentType)
	c.File(task.FilePath)
}

func DeleteSpaceExportTaskHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	username := platform.Username(c)
	id := c.Param("id")

	var task database.SpaceExportTask
	if err := db.Where("id = ? AND created_by = ?", id, username).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "导出任务不存在"})
		return
	}
	if task.Status == "pending" || task.Status == "running" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "运行中的导出任务不允许删除"})
		return
	}

	if task.FilePath != "" {
		cleanPath := filepath.Clean(task.FilePath)
		if removeErr := os.Remove(cleanPath); removeErr != nil && !os.IsNotExist(removeErr) {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "删除导出文件失败: " + removeErr.Error()})
			return
		}
	}

	if err := db.Delete(&database.SpaceExportTask{}, "id = ? AND created_by = ?", id, username).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "删除导出任务失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "导出任务已删除",
	})
}
