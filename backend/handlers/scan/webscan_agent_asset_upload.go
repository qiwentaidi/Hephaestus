package scan

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/handlers/agent"
	"hephaestus-backend/pkg/agentruntime"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"

	"github.com/gin-gonic/gin"
)

const (
	maxAgentImageUploadSize = 10 * 1024 * 1024 // 10MB
)

var agentImageUploadSuffixBlacklist = map[string]struct{}{
	".php":   {},
	".phtml": {},
	".jsp":   {},
	".jspx":  {},
	".asp":   {},
	".aspx":  {},
	".js":    {},
	".sh":    {},
	".bat":   {},
	".cmd":   {},
	".exe":   {},
	".dll":   {},
	".so":    {},
	".py":    {},
	".rb":    {},
	".pl":    {},
	".go":    {},
	".jar":   {},
	".war":   {},
	".html":  {},
	".htm":   {},
	".svg":   {}, // svg 可携带脚本，默认禁用
}

var allowedImageExtSet = map[string]struct{}{
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".gif":  {},
	".webp": {},
	".ico":  {},
	".bmp":  {},
}

func isAllowedImageContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if !strings.HasPrefix(contentType, "image/") {
		return false
	}
	// svg 虽然是 image/*，但存在脚本风险，默认禁用
	return contentType != "image/svg+xml"
}

func extFromImageContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/x-icon", "image/vnd.microsoft.icon":
		return ".ico"
	case "image/bmp":
		return ".bmp"
	default:
		return ".png"
	}
}

func verifyAgentResourceAuth(c *gin.Context, scanOnly bool) error {
	agentID := c.GetHeader(agentruntime.NodeCredentialHeader)
	token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !ok {
		return agentruntime.ErrNodeCredential
	}
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	if scanOnly {
		return agentruntime.VerifyScanNodeCredential(c.Request.Context(), db, agentID, token)
	}
	return agentruntime.VerifyResourceNodeCredential(c.Request.Context(), db, agentID, token)
}

// AgentUploadImageHandler Agent 上传截图/图标接口
// 认证: 当前扫描节点会话
func AgentUploadImageHandler(c *gin.Context) {
	if err := verifyAgentResourceAuth(c, true); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	assetType := agent.NormalizeAgentScanAssetType(c.PostForm("assetType"))
	if assetType == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "assetType 必须是 screenshot 或 favicon",
		})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "缺少文件: " + err.Error(),
		})
		return
	}

	originName := strings.TrimSpace(fileHeader.Filename)
	originExt := strings.ToLower(filepath.Ext(originName))
	if _, blocked := agentImageUploadSuffixBlacklist[originExt]; blocked {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "文件后缀被禁止上传: " + originExt,
		})
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "读取上传文件失败: " + err.Error(),
		})
		return
	}
	defer src.Close()

	data, err := io.ReadAll(io.LimitReader(src, maxAgentImageUploadSize+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "读取上传文件失败: " + err.Error(),
		})
		return
	}
	if len(data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "上传文件为空",
		})
		return
	}
	if len(data) > maxAgentImageUploadSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "上传文件过大，最大支持 10MB",
		})
		return
	}

	sniffLen := len(data)
	if sniffLen > 512 {
		sniffLen = 512
	}
	contentType := http.DetectContentType(data[:sniffLen])
	if !isAllowedImageContentType(contentType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "仅允许上传图片内容（png/jpg/gif/webp/ico/bmp）",
		})
		return
	}

	ext := originExt
	if _, ok := allowedImageExtSet[ext]; !ok {
		ext = extFromImageContentType(contentType)
	}

	filename := fmt.Sprintf("%d_%s%s", time.Now().Unix(), database.GenerateNanoid(), ext)
	assetDir := config.GetScanAssetDir(assetType)
	if err := os.MkdirAll(assetDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建目录失败: " + err.Error(),
		})
		return
	}
	localPath := filepath.Join(assetDir, filename)
	if err := os.WriteFile(localPath, data, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存文件失败: " + err.Error(),
		})
		return
	}

	taskID := strings.TrimSpace(c.PostForm("taskId"))
	version := 1
	if versionRaw := strings.TrimSpace(c.PostForm("version")); versionRaw != "" {
		if parsed, parseErr := strconv.Atoi(versionRaw); parseErr == nil && parsed > 0 {
			version = parsed
		}
	}

	db, dbErr := database.GetDB()
	if dbErr == nil {
		now := time.Now()
		record := database.ScanAssetFile{
			AssetType:     assetType,
			FileName:      filename,
			ContentType:   contentType,
			Data:          data,
			Size:          int64(len(data)),
			SourceTaskID:  taskID,
			SourceVersion: version,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := agent.UpsertAgentScanAssetRecord(db, &record); err != nil {
			logger.Warning("[agent-upload] 写入 scan_asset_file 失败: %v", err)
		}
	} else {
		logger.Warning("[agent-upload] 获取数据库失败，跳过资产二进制入库: %v", dbErr)
	}

	relativePath := filepath.ToSlash(filepath.Join("data", assetType, filename))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"path":        relativePath,
			"fileName":    filename,
			"assetType":   assetType,
			"contentType": contentType,
			"size":        len(data),
		},
	})
}
