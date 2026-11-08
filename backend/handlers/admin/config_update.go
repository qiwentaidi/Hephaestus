package admin

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	cfg "hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/webscan"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"

	"hephaestus-backend/middleware"
)

const updateTargetConfig = "config"

func getHomeConfigRoot() string {
	return cfg.GetConfigResourceRoot()
}

// GetConfigUpdateInfo returns local config update metadata.
func GetConfigUpdateInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version": cfg.ReadConfigUpdateVersion(),
		},
	})
}

// CheckConfigUpdate checks the remote config bundle.
func CheckConfigUpdate(c *gin.Context) {
	info, err := fetchRemoteUpdateInfo(c.Request.Context(), updateTargetConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	currentVersion := cfg.ReadConfigUpdateVersion()
	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"currentVersion": currentVersion,
		"latestVersion":  info.Version,
		"hasUpdate":      compareVersions(info.Version, currentVersion),
		"updateInfo":     info,
	})
}

// DownloadConfigWithProgress downloads and applies the config bundle with SSE progress.
func DownloadConfigWithProgress(c *gin.Context) {
	tokenString := c.Query("token")
	if tokenString == "" {
		c.SSEvent("error", gin.H{"message": "未提供有效Token"})
		return
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return middleware.GetJWTKey(), nil
	})

	if err != nil || !token.Valid {
		c.SSEvent("error", gin.H{"message": "Token无效"})
		return
	}

	claims := token.Claims.(jwt.MapClaims)
	username, ok := claims["username"].(string)
	blmd5, ok2 := claims["blmd5"].(string)
	if !ok || !ok2 {
		c.SSEvent("error", gin.H{"message": "Token缺少必要字段"})
		return
	}

	h := md5.New()
	h.Write([]byte("bl" + username))
	expect := hex.EncodeToString(h.Sum(nil))
	if blmd5 != expect {
		c.SSEvent("error", gin.H{"message": "Token验证失败"})
		return
	}

	downloadURL, remoteVersion, err := fetchPublishedDownload(c.Request.Context(), updateTargetConfig, c.Query("url"), c.Query("version"))
	if err != nil {
		c.SSEvent("error", gin.H{"message": err.Error()})
		return
	}

	if !tryBeginUpdateOperation(updateTargetConfig) {
		c.SSEvent("error", gin.H{"message": "配置更新正在进行中，请勿重复触发"})
		return
	}
	defer endUpdateOperation(updateTargetConfig)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	resp, err := http.Head(downloadURL)
	if err != nil {
		c.SSEvent("error", gin.H{"message": "无法获取文件信息"})
		return
	}
	totalSize := resp.ContentLength

	resp, err = http.Get(downloadURL)
	if err != nil {
		c.SSEvent("error", gin.H{"message": "下载失败"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.SSEvent("error", gin.H{"message": fmt.Sprintf("下载失败: HTTP %d", resp.StatusCode)})
		return
	}

	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	tempDir := filepath.Join(execDir, ".config_update_temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		c.SSEvent("error", gin.H{"message": "创建临时目录失败"})
		return
	}
	defer os.RemoveAll(tempDir)

	zipPath := filepath.Join(tempDir, "config-update.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		c.SSEvent("error", gin.H{"message": "创建文件失败"})
		return
	}

	buffer := make([]byte, 32*1024)
	var downloaded int64

	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := out.Write(buffer[:n]); writeErr != nil {
				out.Close()
				c.SSEvent("error", gin.H{"message": "写入文件失败"})
				return
			}
			downloaded += int64(n)

			progress := 0
			if totalSize > 0 {
				progress = int(float64(downloaded) / float64(totalSize) * 100)
			}
			c.SSEvent("progress", gin.H{
				"progress":   progress,
				"downloaded": downloaded,
				"total":      totalSize,
			})
			c.Writer.Flush()
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			out.Close()
			c.SSEvent("error", gin.H{"message": "下载中断"})
			return
		}
	}
	out.Close()

	c.SSEvent("installing", gin.H{"message": "正在应用配置更新..."})
	c.Writer.Flush()

	if err := applyConfigUpdate(zipPath, remoteVersion); err != nil {
		c.SSEvent("error", gin.H{"message": "应用配置更新失败: " + err.Error()})
		return
	}

	c.SSEvent("complete", gin.H{"message": "配置更新完成"})
}

func fetchRemoteUpdateInfo(ctx context.Context, target string) (UpdateInfo, error) {
	if !isPublicUpdateTarget(target) {
		return UpdateInfo{}, fmt.Errorf("无效的更新目标")
	}
	apiURL := buildUpdateCheckURL(DefaultUpdateServer)
	osType := runtime.GOOS
	arch := runtime.GOARCH
	url := fmt.Sprintf("%s?os=%s&arch=%s&target=%s", apiURL, osType, arch, target)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return UpdateInfo{}, err
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("无法连接到更新服务器")
	}
	defer resp.Body.Close()

	var result struct {
		Success bool       `json:"success"`
		Data    UpdateInfo `json:"data"`
		Error   string     `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return UpdateInfo{}, fmt.Errorf("解析更新信息失败")
	}

	if !result.Success {
		if result.Error != "" {
			return UpdateInfo{}, errors.New(result.Error)
		}
		return UpdateInfo{}, fmt.Errorf("获取更新信息失败")
	}

	return result.Data, nil
}

func applyConfigUpdate(zipPath, remoteVersion string) error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	execDir := filepath.Dir(execPath)
	tempDir := filepath.Join(execDir, ".config_update_temp")
	extractDir := filepath.Join(tempDir, "extracted")

	if err := unzip(zipPath, extractDir); err != nil {
		return err
	}

	payloadRoot, err := findConfigPayloadRoot(extractDir)
	if err != nil {
		return err
	}

	targetRoot := getHomeConfigRoot()
	if err := os.MkdirAll(targetRoot, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(payloadRoot)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		if isProtectedConfigUpdateEntry(name) || isMacOSJunkName(name) {
			continue
		}

		srcPath := filepath.Join(payloadRoot, name)
		dstPath := filepath.Join(targetRoot, name)

		if err := os.RemoveAll(dstPath); err != nil {
			return err
		}

		if entry.IsDir() {
			if err := copyConfigDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if err := copyConfigFile(srcPath, dstPath); err != nil {
			return err
		}
	}

	if remoteVersion != "" {
		if err := cfg.WriteConfigUpdateVersion(remoteVersion); err != nil {
			return err
		}
	}

	if err := webscan.SignUpdatedCodeTemplatesForNuclei(filepath.Join(targetRoot, "pocs")); err != nil {
		return fmt.Errorf("签名 code PoC 模板失败: %w", err)
	}

	if err := webscan.RefreshPOCTemplateCache(); err != nil {
		return fmt.Errorf("刷新 PoC 模板缓存失败: %w", err)
	}

	return nil
}

func findConfigPayloadRoot(extractDir string) (string, error) {
	candidates := []string{extractDir, filepath.Join(extractDir, "config")}
	for _, candidate := range candidates {
		if hasConfigPayload(candidate) {
			return candidate, nil
		}
	}

	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		child := filepath.Join(extractDir, entry.Name())
		if hasConfigPayload(child) {
			return child, nil
		}

		nested := filepath.Join(child, "config")
		if hasConfigPayload(nested) {
			return nested, nil
		}
	}

	return "", fmt.Errorf("未找到可应用的配置文件目录")
}

func hasConfigPayload(root string) bool {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return false
	}

	required := []string{"pocs", "crack", "dirsearch"}
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}

	if _, err := os.Stat(filepath.Join(root, "config.yaml")); err == nil {
		return true
	}

	if _, err := os.Stat(filepath.Join(root, "config-desktop.yaml")); err == nil {
		return true
	}

	// 允许仅包含单文件版本信息的包
	if _, err := os.Stat(filepath.Join(root, cfg.ConfigUpdateVersionFileName)); err == nil {
		return true
	}

	return false
}

func copyConfigDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if isMacOSJunkName(info.Name()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyConfigFile(path, dstPath)
	})
}

func isProtectedConfigUpdateEntry(name string) bool {
	switch name {
	case "config.yaml", "config-desktop.yaml", "data", "logs":
		return true
	default:
		return false
	}
}

func isMacOSJunkName(name string) bool {
	return strings.HasPrefix(name, "._") || name == ".DS_Store"
}

func copyConfigFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, srcInfo.Mode())
}
