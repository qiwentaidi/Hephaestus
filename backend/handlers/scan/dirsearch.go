package scan

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/handlers/toolbox"
	"hephaestus-backend/pkg/config"
)

func readDictPreviewLines(filePath string, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// 允许更长行（默认 64K 可能不够）
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lines := make([]string, 0, maxLines)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) >= maxLines {
			break
		}
	}
	return lines
}

// GetDirsearchDictsHandler 获取可用的字典文件列表
func GetDirsearchDictsHandler(c *gin.Context) {
	entries, err := config.ListManagedDictFiles("dirsearch")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "读取字典目录失败: " + err.Error(),
		})
		return
	}

	var dicts []map[string]interface{}
	for _, entry := range entries {
		filePath := entry.Path
		// 获取文件信息
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}
		// 获取文件大小
		fileSize := info.Size()

		// 读取字典前 3 行预览（非空）
		previewLines := readDictPreviewLines(filePath, 3)
		description := entry.Name
		if len(previewLines) > 0 {
			description = strings.Join(previewLines, "\n")
		}

		dicts = append(dicts, map[string]interface{}{
			"name":        entry.Name,
			"path":        filePath,
			"size":        fileSize,
			"description": description,
			"entries":     previewLines,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"dicts":   dicts,
	})
}

// UploadDirsearchDictHandler 上传字典文件到持久化的 custom-config/dirsearch 目录
func UploadDirsearchDictHandler(c *gin.Context) {
	// 获取上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "未找到上传文件: " + err.Error(),
		})
		return
	}

	// 验证文件扩展名
	filename := file.Filename
	if err := toolbox.ValidateDictFileName(filename); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// 确保字典目录存在
	dictDir := config.GetCustomDictDir("dirsearch")
	if err := os.MkdirAll(dictDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建字典目录失败: " + err.Error(),
		})
		return
	}

	// 构建保存路径
	savePath := filepath.Join(dictDir, filename)

	// 检查文件是否已存在
	if _, err := config.ResolveManagedDictFile("dirsearch", filename); err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "字典文件已存在: " + filename,
		})
		return
	} else if !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "检查字典文件失败: " + err.Error()})
		return
	}

	input, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "读取上传文件失败: " + err.Error()})
		return
	}
	defer input.Close()
	output, err := os.OpenFile(savePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "字典文件已存在: " + filename})
		return
	}
	if err == nil {
		_, err = io.Copy(output, input)
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		_ = os.Remove(savePath)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存字典文件失败: " + err.Error(),
		})
		return
	}

	// 获取文件信息
	fileInfo, err := os.Stat(savePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取文件信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "字典文件上传成功",
		"dict": gin.H{
			"name": filename,
			"path": savePath,
			"size": fileInfo.Size(),
		},
	})
}
