package admin

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hephaestus-backend/middleware"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/dockerupdate"
	"hephaestus-backend/pkg/fsutil"
	"hephaestus-backend/pkg/version"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"github.com/minio/selfupdate"
)

const (
	// 官网根地址。更新源是产品内置信任根，不接受运行时配置覆盖。
	DefaultUpdateServer = "http://47.97.59.120:9090"
	// 更新检查API路径
	UpdateCheckPath           = "/api/public/update/check"
	UpdateHistoryPath         = "/api/public/update/history"
	desktopUpdateHelperFlag   = "--desktop-update-helper"
	desktopUpdateHelperWaitMS = 1500
)

func buildUpdateCheckURL(updateServer string) string {
	trimmed := strings.TrimRight(updateServer, "/")
	if strings.HasSuffix(trimmed, UpdateCheckPath) {
		return trimmed
	}
	return trimmed + UpdateCheckPath
}

func buildUpdateHistoryURL(updateServer string) string {
	trimmed := strings.TrimRight(updateServer, "/")
	if strings.HasSuffix(trimmed, UpdateHistoryPath) {
		return trimmed
	}
	return trimmed + UpdateHistoryPath
}

var trustedUpdateDownloadHosts = []string{
	"47.97.59.120",
	"github.com",
	"githubusercontent.com",
	"gitee.com",
	"giteeusercontent.com",
}

func validateUpdateDownloadURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" || parsed.User != nil {
		return "", fmt.Errorf("更新包地址无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("更新包地址必须使用 HTTP 或 HTTPS")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if net.ParseIP(host) != nil {
		if host != "47.97.59.120" {
			return "", fmt.Errorf("更新包地址不在受信任的更新源范围内")
		}
	} else {
		trusted := false
		for _, suffix := range trustedUpdateDownloadHosts[1:] {
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				trusted = true
				break
			}
		}
		if !trusted {
			return "", fmt.Errorf("更新包地址不在受信任的更新源范围内")
		}
	}
	return raw, nil
}

// fetchPublishedDownload only accepts a download currently advertised by the public release API.
func fetchPublishedDownload(ctx context.Context, target, requestedURL, requestedVersion string) (string, string, error) {
	info, err := fetchRemoteUpdateInfo(ctx, target)
	if err != nil {
		return "", "", err
	}
	downloadURL, err := selectPublishedDownload(info, requestedURL, requestedVersion)
	return downloadURL, info.Version, err
}

func selectPublishedDownload(info UpdateInfo, requestedURL, requestedVersion string) (string, error) {
	if requestedVersion != "" && requestedVersion != info.Version {
		return "", fmt.Errorf("发布版本已更新，请重新检查更新")
	}
	requestedURL, err := validateUpdateDownloadURL(requestedURL)
	if err != nil {
		return "", err
	}
	if requestedURL != strings.TrimSpace(info.DownloadURL) && requestedURL != strings.TrimSpace(info.MirrorURL) {
		return "", fmt.Errorf("下载地址已更新，请重新检查更新")
	}
	return requestedURL, nil
}

// UpdateInfo 更新信息
type UpdateInfo struct {
	Delivery    string `json:"delivery,omitempty"`
	Version     string `json:"version"`
	ReleaseDate string `json:"releaseDate"`
	Changelog   string `json:"changelog"`
	DownloadURL string `json:"downloadUrl"`
	MirrorURL   string `json:"mirrorUrl"`
	SHA256      string `json:"sha256"`
}

type UpdateHistoryItem struct {
	Version     string `json:"version"`
	ReleaseDate string `json:"releaseDate"`
	Changelog   string `json:"changelog"`
}

// compareVersions 比较两个版本号，返回 true 表示 remote > local
func compareVersions(remote, local string) bool {
	// 移除 v 前缀
	remote = strings.TrimPrefix(remote, "v")
	local = strings.TrimPrefix(local, "v")

	remoteParts := strings.Split(remote, ".")
	localParts := strings.Split(local, ".")

	// 补齐长度
	maxLen := len(remoteParts)
	if len(localParts) > maxLen {
		maxLen = len(localParts)
	}

	for i := 0; i < maxLen; i++ {
		var r, l int
		if i < len(remoteParts) {
			fmt.Sscanf(remoteParts[i], "%d", &r)
		}
		if i < len(localParts) {
			fmt.Sscanf(localParts[i], "%d", &l)
		}

		if r > l {
			return true
		} else if r < l {
			return false
		}
	}

	return false
}

func resolveUpdateTarget(c *gin.Context) string {
	target := strings.TrimSpace(c.DefaultQuery("target", ""))
	if target != "" {
		return target
	}
	if config.GetRuntimeMode() == config.RuntimeModeDesktop {
		return "desktop"
	}
	if config.GetRuntimeMode() == config.RuntimeModeAgent {
		return "agent"
	}
	return "client"
}

func resolveCurrentVersionForTarget(target string) string {
	switch strings.TrimSpace(target) {
	case "desktop":
		return version.DesktopVersion
	default:
		return version.Version
	}
}

func isDesktopDMGUpdate(target string, downloadURL string) bool {
	if strings.TrimSpace(target) != "desktop" || runtime.GOOS != "darwin" {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(downloadURL))
	if err != nil {
		return strings.HasSuffix(strings.ToLower(strings.TrimSpace(downloadURL)), ".dmg")
	}
	return strings.HasSuffix(strings.ToLower(parsed.Path), ".dmg")
}

func resolveUpdateDownloadPath(execDir string, target string, downloadURL string) string {
	if isDesktopDMGUpdate(target, downloadURL) {
		return filepath.Join(execDir, ".update_temp", "desktop-update.dmg")
	}
	return filepath.Join(execDir, ".update_temp", "update.zip")
}

func isCandidateUpdateBinary(name string, mode os.FileMode) bool {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	return (strings.HasPrefix(lowerName, "hephaestus") || strings.HasPrefix(lowerName, "main")) &&
		(mode&0111 != 0 || strings.HasSuffix(lowerName, ".exe"))
}

func buildSelfUpdateOptions(execPath string) selfupdate.Options {
	return selfupdate.Options{
		TargetPath: execPath,
	}
}

func resolvePreparedBinaryPath(execPath string) string {
	execDir := filepath.Dir(execPath)
	filename := filepath.Base(execPath)
	return filepath.Join(execDir, fmt.Sprintf(".%s.new", filename))
}

func stagePreparedUpdate(execPath, newBinaryPath string) error {
	updateFile, err := os.Open(newBinaryPath)
	if err != nil {
		return err
	}
	defer updateFile.Close()

	return selfupdate.PrepareAndCheckBinary(updateFile, buildSelfUpdateOptions(execPath))
}

func buildDesktopUpdateHelperPath(execPath string) string {
	execDir := filepath.Dir(execPath)
	baseName := filepath.Base(execPath)
	ext := filepath.Ext(baseName)
	name := strings.TrimSuffix(baseName, ext)
	if ext == "" {
		return filepath.Join(execDir, "."+name+".update-helper")
	}
	return filepath.Join(execDir, "."+name+".update-helper"+ext)
}

func launchPreparedDesktopUpdateHelper(execPath string) error {
	helperPath := buildDesktopUpdateHelperPath(execPath)
	_ = os.Remove(helperPath)
	if err := fsutil.CopyFile(execPath, helperPath); err != nil {
		return err
	}
	if err := os.Chmod(helperPath, 0755); err != nil {
		return err
	}

	cmd := exec.Command(
		helperPath,
		desktopUpdateHelperFlag,
		"--target", execPath,
		"--cleanup", helperPath,
		"--wait-ms", fmt.Sprintf("%d", desktopUpdateHelperWaitMS),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	prepareBackgroundCommand(cmd)
	return cmd.Start()
}

// rejectContainerSelfUpdate prevents the binary from being replaced inside a
// container layer. The new process would either be killed with PID 1 or be
// discarded on the next container recreate, so the only supported upgrade path
// is replacing the image and recreating the container.
func rejectContainerSelfUpdate(c *gin.Context) bool {
	if !config.IsContainerDeployment() {
		return false
	}
	c.JSON(http.StatusConflict, gin.H{
		"success": false,
		"error":   "容器部署不支持在线自更新，请拉取新镜像并重建容器",
	})
	return true
}

func rejectContainerSelfUpdateSSE(c *gin.Context) bool {
	if !config.IsContainerDeployment() {
		return false
	}
	c.SSEvent("error", gin.H{"message": "容器部署不支持在线自更新，请拉取新镜像并重建容器"})
	return true
}

// CheckUpdate 检查更新
func CheckUpdate(c *gin.Context) {
	if config.IsContainerDeployment() {
		updater := dockerupdate.ConfiguredClient()
		if updater == nil {
			dockerUpdateError(c, http.StatusConflict, "此部署尚未接入 Docker 更新器")
			return
		}
		checkDockerUpdate(c, updater)
		return
	}

	target := resolveUpdateTarget(c)
	currentVersion := resolveCurrentVersionForTarget(target)
	info, err := fetchRemoteUpdateInfo(c.Request.Context(), target)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// 返回更新信息
	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"currentVersion": currentVersion,
		"latestVersion":  info.Version,
		"hasUpdate":      compareVersions(info.Version, currentVersion),
		"updateInfo":     info,
	})
}

func isPublicUpdateTarget(target string) bool {
	return target == "client" || target == "desktop" || target == updateTargetConfig
}

func GetUpdateHistory(c *gin.Context) {
	apiURL := buildUpdateHistoryURL(DefaultUpdateServer)
	target := strings.TrimSpace(c.DefaultQuery("target", "client"))
	if !isPublicUpdateTarget(target) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的更新目标"})
		return
	}
	url := fmt.Sprintf("%s?target=%s", apiURL, target)

	resp, err := http.Get(url)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "无法连接到更新服务器",
		})
		return
	}
	defer resp.Body.Close()

	var result struct {
		Success bool                `json:"success"`
		Data    []UpdateHistoryItem `json:"data"`
		Error   string              `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "解析历史更新信息失败",
		})
		return
	}

	if !result.Success {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   result.Error,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result.Data,
	})
}

// DownloadAndUpdate 下载并更新
func DownloadAndUpdate(c *gin.Context) {
	if rejectContainerSelfUpdate(c) {
		return
	}

	var req struct {
		DownloadURL string `json:"downloadUrl"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "参数错误"})
		return
	}
	downloadURL, _, err := fetchPublishedDownload(c.Request.Context(), resolveUpdateTarget(c), req.DownloadURL, "")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	if !tryBeginUpdateOperation("client") {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "客户端更新正在进行中，请勿重复触发"})
		return
	}
	defer endUpdateOperation("client")

	// 获取当前执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取执行文件路径失败"})
		return
	}

	execDir := filepath.Dir(execPath)

	// 创建临时目录
	tempDir := filepath.Join(execDir, ".update_temp")
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)

	// 下载文件
	zipPath := filepath.Join(tempDir, "update.zip")
	if err := downloadFile(downloadURL, zipPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "下载更新包失败: " + err.Error()})
		return
	}

	// 解压文件
	extractDir := filepath.Join(tempDir, "extracted")
	if err := unzip(zipPath, extractDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "解压更新包失败: " + err.Error()})
		return
	}

	// 查找二进制文件
	var newBinaryPath string
	filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && isCandidateUpdateBinary(info.Name(), info.Mode()) {
			newBinaryPath = path
			return filepath.SkipDir
		}
		return nil
	})

	if newBinaryPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "未找到可执行文件"})
		return
	}

	if err := stagePreparedUpdate(execPath, newBinaryPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "预置更新失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新包下载完成，请调用重启接口完成更新",
	})
}

// DownloadWithProgress 带进度的下载（SSE）
func DownloadWithProgress(c *gin.Context) {
	if rejectContainerSelfUpdateSSE(c) {
		return
	}

	// 验证 token（从 query 参数）
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

	// 校验 blmd5
	h := md5.New()
	h.Write([]byte("bl" + username))
	expect := hex.EncodeToString(h.Sum(nil))
	if blmd5 != expect {
		c.SSEvent("error", gin.H{"message": "Token验证失败"})
		return
	}

	target := resolveUpdateTarget(c)
	downloadURL, _, err := fetchPublishedDownload(c.Request.Context(), target, c.Query("url"), c.Query("version"))
	if err != nil {
		c.SSEvent("error", gin.H{"message": err.Error()})
		return
	}

	if !tryBeginUpdateOperation("client") {
		c.SSEvent("error", gin.H{"message": "客户端更新正在进行中，请勿重复触发"})
		return
	}
	defer endUpdateOperation("client")

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// 获取文件大小
	resp, err := http.Head(downloadURL)
	if err != nil {
		c.SSEvent("error", gin.H{"message": "无法获取文件信息"})
		return
	}
	totalSize := resp.ContentLength

	// 开始下载
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

	// 获取临时目录
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	tempDir := filepath.Join(execDir, ".update_temp")
	os.MkdirAll(tempDir, 0755)

	downloadPath := resolveUpdateDownloadPath(execDir, target, downloadURL)
	out, err := os.Create(downloadPath)
	if err != nil {
		c.SSEvent("error", gin.H{"message": "创建文件失败"})
		return
	}
	defer out.Close()

	// 边下载边发送进度
	buffer := make([]byte, 32*1024)
	var downloaded int64

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			out.Write(buffer[:n])
			downloaded += int64(n)

			progress := float64(downloaded) / float64(totalSize) * 100
			c.SSEvent("progress", gin.H{
				"progress":   int(progress),
				"downloaded": downloaded,
				"total":      totalSize,
			})
			c.Writer.Flush()
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			c.SSEvent("error", gin.H{"message": "下载中断"})
			return
		}
	}

	out.Close()

	// 下载完成，开始安装
	c.SSEvent("installing", gin.H{"message": "正在安装..."})
	c.Writer.Flush()

	if isDesktopDMGUpdate(target, downloadURL) {
		c.SSEvent("complete", gin.H{
			"message": "安装包下载完成",
			"path":    downloadPath,
		})
		return
	}

	// 解压文件
	extractDir := filepath.Join(tempDir, "extracted")
	if err := unzip(downloadPath, extractDir); err != nil {
		c.SSEvent("error", gin.H{"message": "解压失败: " + err.Error()})
		return
	}

	// 查找二进制文件
	var newBinaryPath string
	filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && isCandidateUpdateBinary(info.Name(), info.Mode()) {
			newBinaryPath = path
			return filepath.SkipDir
		}
		return nil
	})

	if newBinaryPath == "" {
		c.SSEvent("error", gin.H{"message": "未找到可执行文件"})
		return
	}

	if err := stagePreparedUpdate(execPath, newBinaryPath); err != nil {
		c.SSEvent("error", gin.H{"message": "预置更新失败: " + err.Error()})
		return
	}

	c.SSEvent("complete", gin.H{"message": "更新完成"})
}

// RestartService 重启服务
func RestartService(c *gin.Context) {
	if config.IsContainerDeployment() && dockerupdate.ConfiguredClient() != nil {
		startDockerUpdate(c, "restart")
		return
	}
	if rejectContainerSelfUpdate(c) {
		return
	}

	execPath, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取执行文件路径失败"})
		return
	}

	newExecPath := resolvePreparedBinaryPath(execPath)

	// 检查新文件是否存在
	if _, err := os.Stat(newExecPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "未找到新版本文件，请先下载更新"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "服务将在3秒后重启",
	})

	go func() {
		time.Sleep(1200 * time.Millisecond)

		if runtime.GOOS == "windows" && config.GetRuntimeMode() == config.RuntimeModeDesktop {
			if err := launchPreparedDesktopUpdateHelper(execPath); err != nil {
				fmt.Printf("launch desktop update helper failed: %v\n", err)
				return
			}
			os.Exit(0)
			return
		}

		if err := selfupdate.CommitBinary(buildSelfUpdateOptions(execPath)); err != nil {
			fmt.Printf("self update commit failed: %v\n", err)
			if rollbackErr := selfupdate.RollbackError(err); rollbackErr != nil {
				fmt.Printf("self update rollback failed: %v\n", rollbackErr)
			}
			return
		}

		cmd := exec.Command(execPath, os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		prepareBackgroundCommand(cmd)
		if err := cmd.Start(); err != nil {
			fmt.Printf("restart updated binary failed: %v\n", err)
			return
		}

		os.Exit(0)
	}()
}

// GetSystemInfo 获取系统信息
func GetSystemInfo(c *gin.Context) {
	execPath, _ := os.Executable()
	target := resolveUpdateTarget(c)
	currentVersion := resolveCurrentVersionForTarget(target)
	lastModified := ""
	if fileInfo, err := os.Stat(execPath); err == nil && fileInfo != nil {
		lastModified = fileInfo.ModTime().Format("2006-01-02 15:04:05")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version":            currentVersion,
			"os":                 runtime.GOOS,
			"arch":               runtime.GOARCH,
			"execPath":           execPath,
			"lastModified":       lastModified,
			"deployment":         config.GetDeploymentMode(),
			"selfUpdateDisabled": config.IsContainerDeployment() && dockerupdate.ConfiguredClient() == nil,
		},
	})
}

// downloadFile 下载文件
func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// unzip 解压zip文件
func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	root, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}

	for _, f := range r.File {
		name := filepath.Clean(filepath.FromSlash(f.Name))
		if filepath.IsAbs(name) || name == "." || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe archive path %q", f.Name)
		}
		fpath := filepath.Join(root, name)
		rel, err := filepath.Rel(root, fpath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("archive path escapes extraction root: %q", f.Name)
		}
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not allowed in update archives: %q", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, 0700); err != nil {
				return err
			}
			continue
		}

		if !f.FileInfo().Mode().IsRegular() {
			return fmt.Errorf("unsupported archive entry type %q", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(fpath), 0700); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode().Perm())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}
