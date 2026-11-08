package admin

import (
	"context"
	"net/http"
	"runtime"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/synpriv"

	"github.com/gin-gonic/gin"
)

type SYNCapabilityStatus struct {
	Platform                 string `json:"platform"`
	RuntimeMode              string `json:"runtimeMode"`
	Supported                bool   `json:"supported"`
	Authorized               bool   `json:"authorized"`
	AuthorizationRequired    bool   `json:"authorizationRequired"`
	CanFallbackToConnect     bool   `json:"canFallbackToConnect"`
	CurrentProcessPrivileged bool   `json:"currentProcessPrivileged"`
	HelperInstalled          bool   `json:"helperInstalled"`
	HelperRunning            bool   `json:"helperRunning"`
	HelperVersion            string `json:"helperVersion,omitempty"`
	HelperPath               string `json:"helperPath,omitempty"`
	Status                   string `json:"status"`
	Message                  string `json:"message"`
}

func GetSYNCapabilityStatusHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    buildSYNCapabilityStatus(),
	})
}

func RequestSYNAuthorizationHandler(c *gin.Context) {
	status := buildSYNCapabilityStatus()
	if !status.Supported {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   status.Message,
			"data":    status,
		})
		return
	}
	if status.Authorized {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    status,
		})
		return
	}

	helperStatus, err := synpriv.Install(c.Request.Context())
	status = buildSYNCapabilityStatusFromHelper(helperStatus)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"data":    status,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}

func buildSYNCapabilityStatus() SYNCapabilityStatus {
	return buildSYNCapabilityStatusFromHelper(synpriv.Status(context.Background()))
}

func buildSYNCapabilityStatusFromHelper(helperStatus synpriv.HelperStatus) SYNCapabilityStatus {
	platform := runtime.GOOS
	runtimeMode := config.GetRuntimeMode()
	// Windows uses the built-in raw-packet path when Npcap is installed.
	// Android shares the Unix raw-socket path and is supported when the process
	// has root/capability access.
	supported := synpriv.SupportsRawSocketPlatform(platform) || (platform == "windows" && synpriv.CurrentProcessHasRawSocketCapability())
	currentProcessPrivileged := false
	if supported {
		currentProcessPrivileged = synpriv.CurrentProcessHasRawSocketCapability()
	}
	helperAuthorized := helperStatus.Usable

	status := SYNCapabilityStatus{
		Platform:                 platform,
		RuntimeMode:              runtimeMode,
		Supported:                supported,
		Authorized:               currentProcessPrivileged || helperAuthorized,
		AuthorizationRequired:    supported && !currentProcessPrivileged && !helperAuthorized,
		CanFallbackToConnect:     false,
		CurrentProcessPrivileged: currentProcessPrivileged,
		HelperInstalled:          helperStatus.Exists,
		HelperRunning:            helperStatus.Usable,
		HelperPath:               helperStatus.Path,
	}

	switch {
	case !supported:
		status.Status = "unsupported"
		if platform == "windows" {
			status.Message = "Windows 需要安装 Npcap 后才能使用 SYN 探测"
		} else {
			status.Message = "当前平台尚未支持本机 SYN 探测授权"
		}
	case platform == "windows" && currentProcessPrivileged:
		status.Status = "authorized"
		status.Message = "已检测到 Npcap，可直接使用 Windows SYN 探测"
	case currentProcessPrivileged:
		status.Status = "authorized"
		status.Message = "当前进程已具备 raw socket 权限，可直接使用 SYN 探测"
	case helperAuthorized:
		status.Status = "authorized"
		status.Message = "SYN privileged helper 已安装，可通过 helper 执行 SYN 端口发现"
	default:
		status.Status = "needs_authorization"
		status.Message = "需要安装或启用 privileged helper 后才能使用 SYN 探测"
		if helperStatus.Error != "" {
			status.Message = "SYN 授权不可用：" + helperStatus.Error
		}
	}

	return status
}
