package admin

import (
	"net/http"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/dockerupdate"
	"hephaestus-backend/pkg/version"

	"github.com/gin-gonic/gin"
)

func checkDockerUpdate(c *gin.Context, updater *dockerupdate.Client) {
	release, err := updater.Latest(c.Request.Context())
	if err != nil {
		dockerUpdateError(c, http.StatusBadGateway, err.Error())
		return
	}
	stagedVersion := ""
	if status, err := updater.Status(c.Request.Context()); err == nil && status.Phase == dockerupdate.Staged {
		stagedVersion = status.Version
	}
	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"currentVersion": version.Version,
		"latestVersion":  release.Version,
		"hasUpdate":      compareVersions(release.Version, version.Version),
		"stagedVersion":  stagedVersion,
		"updateInfo":     UpdateInfo{Version: release.Version, ReleaseDate: release.ReleaseDate, Delivery: "docker"},
	})
}

func GetDockerUpdateStatus(c *gin.Context) {
	updater := dockerupdate.ConfiguredClient()
	if !config.IsContainerDeployment() || updater == nil {
		dockerUpdateError(c, http.StatusConflict, "此部署未接入 Docker 更新器")
		return
	}
	status, err := updater.Status(c.Request.Context())
	if err != nil {
		dockerUpdateError(c, http.StatusBadGateway, err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": status})
}

func DownloadDockerUpdate(c *gin.Context) {
	startDockerUpdate(c, "pull")
}

func startDockerUpdate(c *gin.Context, operation string) {
	updater := dockerupdate.ConfiguredClient()
	if !config.IsContainerDeployment() || updater == nil {
		dockerUpdateError(c, http.StatusConflict, "此部署未接入 Docker 更新器")
		return
	}
	db, err := database.GetDB()
	if err != nil {
		dockerUpdateError(c, http.StatusServiceUnavailable, "无法确认 Docker 更新权限")
		return
	}
	var user database.User
	if err := db.WithContext(c.Request.Context()).Select("role").
		Where("username = ?", c.GetString("username")).First(&user).Error; err != nil || user.Role != "admin" {
		dockerUpdateError(c, http.StatusForbidden, "仅管理员可以执行 Docker 更新")
		return
	}
	var request struct {
		Version string `json:"version"`
	}
	if c.ShouldBindJSON(&request) != nil || !dockerupdate.ValidVersion(request.Version) ||
		!compareVersions(request.Version, version.Version) {
		dockerUpdateError(c, http.StatusBadRequest, "无效的 Docker 更新版本")
		return
	}
	status, err := updater.Start(c.Request.Context(), operation, request.Version)
	if err != nil {
		dockerUpdateError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": status})
}

func dockerUpdateError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"success": false, "error": message})
}
