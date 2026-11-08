package scan

import (
	"errors"
	"hephaestus-backend/handlers/agent"
	"hephaestus-backend/pkg/agentasset"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// FaviconHandler 提供favicon文件访问接口
// 使用接口方式提供favicon访问，便于后续扩展支持对象存储（OSS、S3等）
func FaviconHandler(c *gin.Context) {
	// 获取路径参数（支持 *path 通配符）
	faviconPath := c.Param("path")

	// 移除前导斜杠（Gin的*参数会包含前导斜杠）
	faviconPath = strings.TrimPrefix(faviconPath, "/")

	// 如果路径已经以 favicon/ 开头，则移除它（避免重复）
	faviconPath = strings.TrimPrefix(faviconPath, "favicon/")

	if faviconPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "favicon路径不能为空"})
		return
	}

	if strings.Contains(faviconPath, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的favicon路径"})
		return
	}
	faviconPath = agentasset.NormalizeScanAssetFilename(strings.ReplaceAll(faviconPath, "\\", "/"))
	if faviconPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的favicon路径"})
		return
	}

	// 构建文件路径（自动添加 data/favicon/ 前缀）
	faviconDir := config.GetScanAssetDir("favicon")
	filePath := filepath.Join(faviconDir, faviconPath)

	// 优先读取本地文件；不存在时回退到数据库二进制存储
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		db, dbErr := database.GetDB()
		if dbErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取数据库连接失败: " + dbErr.Error()})
			return
		}
		var asset database.ScanAssetFile
		if err := db.Select("content_type", "data").
			Where("asset_type = ? AND file_name = ?", "favicon", faviconPath).
			First(&asset).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "favicon文件不存在"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取favicon数据失败: " + err.Error()})
			return
		}

		contentType := strings.TrimSpace(asset.ContentType)
		if contentType == "" {
			contentType = agent.DetectAgentScanAssetContentType("favicon", faviconPath)
		}
		c.Header("Content-Type", contentType)
		c.Header("Cache-Control", "public, max-age=86400")
		c.Data(http.StatusOK, contentType, asset.Data)
		return
	}

	// 检查文件是否在 favicon 目录内（防止路径遍历）
	absFaviconDir, err := filepath.Abs(faviconDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取favicon目录路径失败: " + err.Error()})
		return
	}

	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取文件路径失败: " + err.Error()})
		return
	}

	if !strings.HasPrefix(absFilePath, absFaviconDir) {
		c.JSON(http.StatusForbidden, gin.H{"error": "访问被拒绝"})
		return
	}

	// 根据文件扩展名设置Content-Type
	ext := strings.ToLower(filepath.Ext(filePath))
	contentType := "image/x-icon" // 默认为ico
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".gif":
		contentType = "image/gif"
	case ".svg":
		contentType = "image/svg+xml"
	case ".webp":
		contentType = "image/webp"
	}

	c.Header("Content-Type", contentType)

	// 设置缓存头（可选，提高性能）
	c.Header("Cache-Control", "public, max-age=86400") // 缓存1天

	// 返回文件
	c.File(filePath)
}

// ScreenshotHandler 提供截图文件访问接口
// 使用接口方式提供截图访问，便于后续扩展支持对象存储（OSS、S3等）
func ScreenshotHandler(c *gin.Context) {
	// 获取路径参数（支持 *path 通配符）
	screenshotPath := c.Param("path")

	// 移除前导斜杠（Gin的*参数会包含前导斜杠）
	screenshotPath = strings.TrimPrefix(screenshotPath, "/")

	// 如果路径已经以 screenshot/ 开头，则移除它（避免重复）
	screenshotPath = strings.TrimPrefix(screenshotPath, "screenshot/")

	if screenshotPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "截图路径不能为空"})
		return
	}

	if strings.Contains(screenshotPath, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的截图路径"})
		return
	}
	screenshotPath = agentasset.NormalizeScanAssetFilename(strings.ReplaceAll(screenshotPath, "\\", "/"))
	if screenshotPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的截图路径"})
		return
	}

	// 构建文件路径（自动添加 data/screenshot/ 前缀）
	screenshotDir := config.GetScanAssetDir("screenshot")
	filePath := filepath.Join(screenshotDir, screenshotPath)

	// 优先读取本地文件；不存在时回退到数据库二进制存储
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		db, dbErr := database.GetDB()
		if dbErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取数据库连接失败: " + dbErr.Error()})
			return
		}
		var asset database.ScanAssetFile
		if err := db.Select("content_type", "data").
			Where("asset_type = ? AND file_name = ?", "screenshot", screenshotPath).
			First(&asset).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "截图文件不存在"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取截图数据失败: " + err.Error()})
			return
		}

		contentType := strings.TrimSpace(asset.ContentType)
		if contentType == "" {
			contentType = screenshotContentTypeByName(screenshotPath)
		}
		c.Header("Content-Type", contentType)
		c.Header("Cache-Control", "public, max-age=86400")
		c.Data(http.StatusOK, contentType, asset.Data)
		return
	}

	// 检查文件是否在 screenshot 目录内（防止路径遍历）
	absScreenshotDir, err := filepath.Abs(screenshotDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取截图目录路径失败: " + err.Error()})
		return
	}

	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取文件路径失败: " + err.Error()})
		return
	}

	if !strings.HasPrefix(absFilePath, absScreenshotDir) {
		c.JSON(http.StatusForbidden, gin.H{"error": "访问被拒绝"})
		return
	}

	// 设置Content-Type
	c.Header("Content-Type", screenshotContentTypeByName(screenshotPath))

	// 设置缓存头（可选，提高性能）
	c.Header("Cache-Control", "public, max-age=86400") // 缓存1天

	// 返回文件
	c.File(filePath)
}

func screenshotContentTypeByName(name string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(name))) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	default:
		return "image/png"
	}
}
