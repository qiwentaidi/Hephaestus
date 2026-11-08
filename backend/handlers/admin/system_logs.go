package admin

import (
	"errors"
	"fmt"
	"hephaestus-backend/pkg/config"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	defaultLogReadLines  = 200
	maxLogReadLines      = 1000
	largeLogSizeHintByte = 2 * 1024 * 1024
)

type systemLogFileItem struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modifiedAt"`
}

func getRuntimeLogsDir() string {
	return config.GetLogsDir()
}

func resolveLogFilePath(logsDir, fileName string) (string, error) {
	name := strings.TrimSpace(fileName)
	if name == "" {
		return "", errors.New("日志文件名不能为空")
	}
	if name != filepath.Base(name) || name == "." || name == ".." {
		return "", errors.New("非法日志文件名")
	}
	return filepath.Join(logsDir, name), nil
}

func tailTextByLines(path string, lines int) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", false, err
	}
	size := info.Size()
	if size == 0 {
		return "", false, nil
	}

	var (
		offset             = size
		chunkSize    int64 = 4096
		buffer             = make([]byte, 0, minInt64(size, 4096*4))
		newlineCount int
	)

	for offset > 0 && newlineCount <= lines {
		start := offset - chunkSize
		if start < 0 {
			start = 0
		}
		readSize := offset - start
		chunk := make([]byte, readSize)
		if _, err := file.ReadAt(chunk, start); err != nil && err != io.EOF {
			return "", false, err
		}
		buffer = append(chunk, buffer...)
		newlineCount += strings.Count(string(chunk), "\n")
		offset = start
	}

	text := strings.TrimRight(string(buffer), "\n")
	if text == "" {
		return "", false, nil
	}

	allLines := strings.Split(text, "\n")
	truncated := len(allLines) > lines
	if truncated {
		allLines = allLines[len(allLines)-lines:]
	}

	return strings.Join(allLines, "\n"), truncated, nil
}

func readLogTail(path string, lines int, keyword, level string) ([]string, bool, error) {
	if lines <= 0 || lines > maxLogReadLines {
		return nil, false, fmt.Errorf("读取行数必须在 1-%d 之间", maxLogReadLines)
	}

	text, truncated, err := tailTextByLines(path, lines)
	if err != nil {
		return nil, false, err
	}
	if text == "" {
		return []string{}, truncated, nil
	}

	keyword = strings.TrimSpace(keyword)
	level = strings.ToUpper(strings.TrimSpace(level))
	levelNeedle := ""
	if level != "" && level != "ALL" {
		levelNeedle = fmt.Sprintf("[%s]", level)
	}

	result := make([]string, 0)
	for _, line := range strings.Split(text, "\n") {
		if keyword != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
			continue
		}
		if levelNeedle != "" && !strings.Contains(line, levelNeedle) {
			continue
		}
		result = append(result, line)
	}

	return result, truncated, nil
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// ListSystemLogFilesHandler 列出运行时日志目录中的日志文件
func ListSystemLogFilesHandler(c *gin.Context) {
	logsDir := getRuntimeLogsDir()
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "初始化日志目录失败",
		})
		return
	}

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取日志目录失败",
		})
		return
	}

	files := make([]systemLogFileItem, 0)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files = append(files, systemLogFileItem{
			Name:       entry.Name(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().Format(time.RFC3339),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModifiedAt > files[j].ModifiedAt
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"dir":   logsDir,
			"files": files,
		},
	})
}

// ReadSystemLogHandler 读取日志文件尾部有限行数的内容
func ReadSystemLogHandler(c *gin.Context) {
	logsDir := getRuntimeLogsDir()
	fileName := c.Query("file")
	logPath, err := resolveLogFilePath(logsDir, fileName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "日志文件不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取日志文件失败",
		})
		return
	}
	if !info.Mode().IsRegular() {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "仅支持读取常规日志文件",
		})
		return
	}

	lines := defaultLogReadLines
	if rawLines := strings.TrimSpace(c.DefaultQuery("lines", fmt.Sprintf("%d", defaultLogReadLines))); rawLines != "" {
		parsedLines, err := strconv.Atoi(rawLines)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "读取行数格式不正确",
			})
			return
		}
		lines = parsedLines
	}

	logLines, truncated, err := readLogTail(
		logPath,
		lines,
		c.Query("keyword"),
		c.Query("level"),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"file":              info.Name(),
			"path":              logPath,
			"size":              info.Size(),
			"lines":             logLines,
			"truncated":         truncated,
			"maxLines":          maxLogReadLines,
			"suggestDirectRead": info.Size() > largeLogSizeHintByte,
		},
	})
}
