package admin

import (
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/runtimeenv"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
)

// AboutInfo 关于信息结构
type AboutInfo struct {
	App       AppInfo       `json:"app"`
	Copyright CopyrightInfo `json:"copyright"`
}

// AppInfo 应用信息
type AppInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Deployment string `json:"deployment"`
}

// CopyrightInfo 版权信息
type CopyrightInfo struct {
	Year    string `json:"year"`
	Company string `json:"company"`
	Text    string `json:"text"`
}

// GetSystemInfoHandler 获取应用版本及版权信息
func GetSystemInfoHandler(c *gin.Context) {
	info := AboutInfo{
		App: AppInfo{
			Name:       "Hephaestus",
			Version:    resolveCurrentVersionForTarget(resolveUpdateTarget(c)),
			Deployment: config.GetDeploymentMode(),
		},
		Copyright: CopyrightInfo{
			Year:    "2026",
			Company: "Hephaestus Team",
			Text:    "Copyright © 2026 Hephaestus Team. All rights reserved.",
		},
	}

	c.JSON(200, info)
}

// SystemProxyInfo 系统代理信息
type SystemProxyInfo struct {
	Enabled    bool   `json:"enabled"`    // 是否开启系统代理
	HttpProxy  string `json:"httpProxy"`  // HTTP 代理地址
	HttpsProxy string `json:"httpsProxy"` // HTTPS 代理地址
	NoProxy    string `json:"noProxy"`    // 不代理的地址
}

// GetSystemProxyHandler 获取系统代理配置
func GetSystemProxyHandler(c *gin.Context) {
	proxyInfo := getSystemProxy()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    proxyInfo,
	})
}

// GetBrowserRuntimeHandler 获取浏览器运行环境信息
func GetBrowserRuntimeHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    runtimeenv.DetectBrowserRuntime(),
	})
}

// getSystemProxy 获取系统代理配置
func getSystemProxy() SystemProxyInfo {
	info := SystemProxyInfo{}

	// 根据操作系统获取系统代理设置
	switch runtime.GOOS {
	case "darwin":
		info = getMacOSProxy()
	case "windows":
		info = getWindowsProxy()
	case "linux":
		// Linux 一般使用环境变量，已经在上面检查过了
		info = getLinuxProxy()
	}

	return info
}

// parseProxyEnabled 严格解析 Enabled 字段
func parseProxyEnabled(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Enabled:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Enabled:")) == "Yes"
		}
	}
	return false
}

// parseProxyServerPort 解析代理服务器和端口
func parseProxyServerPort(output string) (server, port string) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Server:") {
			server = strings.TrimSpace(strings.TrimPrefix(line, "Server:"))
		}
		if strings.HasPrefix(line, "Port:") {
			port = strings.TrimSpace(strings.TrimPrefix(line, "Port:"))
		}
	}
	return
}

// getMacOSProxy 获取 macOS 系统代理配置
// 遍历所有网络服务，检查是否有任意服务启用了代理
func getMacOSProxy() SystemProxyInfo {
	info := SystemProxyInfo{}

	// 获取所有网络服务
	services := getAllNetworkServices()
	if len(services) == 0 {
		return info
	}

	// 遍历所有网络服务，查找启用了代理的服务
	for _, service := range services {
		httpOut := runCommand("networksetup", "-getwebproxy", service)
		httpsOut := runCommand("networksetup", "-getsecurewebproxy", service)
		socksOut := runCommand("networksetup", "-getsocksfirewallproxy", service)

		httpEnabled := parseProxyEnabled(httpOut)
		httpsEnabled := parseProxyEnabled(httpsOut)
		socksEnabled := parseProxyEnabled(socksOut)

		// 如果当前服务有任意代理启用
		if httpEnabled || httpsEnabled || socksEnabled {
			info.Enabled = true

			if httpEnabled {
				server, port := parseProxyServerPort(httpOut)
				if server != "" && port != "" && port != "0" {
					info.HttpProxy = "http://" + server + ":" + port
				}
			}

			if httpsEnabled {
				server, port := parseProxyServerPort(httpsOut)
				if server != "" && port != "" && port != "0" {
					info.HttpsProxy = "http://" + server + ":" + port
				}
			}

			if socksEnabled && info.HttpProxy == "" {
				server, port := parseProxyServerPort(socksOut)
				if server != "" && port != "" && port != "0" {
					info.HttpProxy = "socks5://" + server + ":" + port
				}
			}

			// 获取代理绕过列表
			bypassList := runCommand("networksetup", "-getproxybypassdomains", service)
			if bypassList != "" && !strings.Contains(bypassList, "There aren't any bypass domains") {
				domains := strings.Split(strings.TrimSpace(bypassList), "\n")
				info.NoProxy = strings.Join(domains, ",")
			}

			// 找到第一个启用代理的服务就返回
			return info
		}
	}

	return info
}

// getAllNetworkServices 获取所有网络服务列表
func getAllNetworkServices() []string {
	output := runCommand("networksetup", "-listallnetworkservices")
	lines := strings.Split(output, "\n")

	var services []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 跳过空行、禁用的服务（*开头）、标题行
		if line == "" || strings.HasPrefix(line, "*") || strings.Contains(line, "denotes") {
			continue
		}
		services = append(services, line)
	}
	return services
}

// getWindowsProxy 获取 Windows 系统代理配置
func getWindowsProxy() SystemProxyInfo {
	info := SystemProxyInfo{}

	// 使用 reg query 获取 IE/系统代理设置
	output := runCommand("reg", "query", "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings", "/v", "ProxyEnable")
	if strings.Contains(output, "0x1") {
		info.Enabled = true
		// 获取代理服务器地址
		proxyOutput := runCommand("reg", "query", "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings", "/v", "ProxyServer")
		lines := strings.Split(proxyOutput, "\n")
		for _, line := range lines {
			if strings.Contains(line, "ProxyServer") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					proxy := parts[len(parts)-1]
					if !strings.HasPrefix(proxy, "http://") && !strings.HasPrefix(proxy, "https://") && !strings.HasPrefix(proxy, "socks") {
						proxy = "http://" + proxy
					}
					info.HttpProxy = proxy
					info.HttpsProxy = proxy
				}
			}
		}
		// 获取代理绕过列表
		bypassOutput := runCommand("reg", "query", "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings", "/v", "ProxyOverride")
		lines = strings.Split(bypassOutput, "\n")
		for _, line := range lines {
			if strings.Contains(line, "ProxyOverride") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					info.NoProxy = strings.Replace(parts[len(parts)-1], ";", ",", -1)
				}
			}
		}
	}

	return info
}

// getLinuxProxy 获取 Linux 系统代理配置
func getLinuxProxy() SystemProxyInfo {
	info := SystemProxyInfo{}

	// 尝试从 gsettings 获取 GNOME 代理设置
	mode := runCommand("gsettings", "get", "org.gnome.system.proxy", "mode")
	if strings.Contains(mode, "manual") {
		info.Enabled = true

		// HTTP 代理
		httpHost := strings.Trim(runCommand("gsettings", "get", "org.gnome.system.proxy.http", "host"), "'\" \n")
		httpPort := strings.Trim(runCommand("gsettings", "get", "org.gnome.system.proxy.http", "port"), "'\" \n")
		if httpHost != "" && httpPort != "" && httpPort != "0" {
			info.HttpProxy = "http://" + httpHost + ":" + httpPort
		}

		// HTTPS 代理
		httpsHost := strings.Trim(runCommand("gsettings", "get", "org.gnome.system.proxy.https", "host"), "'\" \n")
		httpsPort := strings.Trim(runCommand("gsettings", "get", "org.gnome.system.proxy.https", "port"), "'\" \n")
		if httpsHost != "" && httpsPort != "" && httpsPort != "0" {
			info.HttpsProxy = "http://" + httpsHost + ":" + httpsPort
		}

		// 忽略列表
		ignoreList := runCommand("gsettings", "get", "org.gnome.system.proxy", "ignore-hosts")
		if ignoreList != "" {
			// 清理格式：['localhost', '127.0.0.0/8'] -> localhost,127.0.0.0/8
			ignoreList = strings.Trim(ignoreList, "[]'\" \n")
			ignoreList = strings.Replace(ignoreList, "', '", ",", -1)
			ignoreList = strings.Replace(ignoreList, "'", "", -1)
			info.NoProxy = ignoreList
		}
	}

	return info
}

// runCommand 执行命令并返回输出
func runCommand(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	prepareBackgroundCommand(cmd)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(output)
}

// GetAppCPUHandler 返回系统总核数和当前应用可用的 GOMAXPROCS 值
func GetAppCPUHandler(c *gin.Context) {
	numCPU := runtime.NumCPU()
	gomax := runtime.GOMAXPROCS(0)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"numCPU":     numCPU,
			"gomaxprocs": gomax,
		},
	})
}

// SetAppCPUHandler 设置应用可用的 CPU 核心数（GOMAXPROCS）
// 请求 JSON 示例: { "cores": 4 }
func SetAppCPUHandler(c *gin.Context) {
	var req struct {
		Cores int `json:"cores"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求体: " + err.Error()})
		return
	}

	if req.Cores <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "cores 必须是正整数"})
		return
	}

	// 限制最大值为系统可见的 CPU 数
	max := runtime.NumCPU()
	if req.Cores > max {
		req.Cores = max
	}

	// 设置并返回之前的值
	prev := runtime.GOMAXPROCS(req.Cores)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"requested": req.Cores,
			"applied":   req.Cores,
			"previous":  prev,
			"numCPU":    max,
		},
	})
}

// NetworkConfig 网卡配置结构
type NetworkConfig struct {
	DefaultNetwork string `json:"defaultNetwork"` // 默认网卡名称或 "auto"
}

// GetNetworkConfigHandler 获取网卡配置
func GetNetworkConfigHandler(c *gin.Context) {
	// 从配置文件读取网卡配置
	cfg, err := getNetworkConfigFromEnv()
	if err != nil {
		// 如果读取失败，返回默认配置
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": NetworkConfig{
				DefaultNetwork: "auto",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    cfg,
	})
}

// SaveNetworkConfigHandler 保存网卡配置
func SaveNetworkConfigHandler(c *gin.Context) {
	var req NetworkConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求体: " + err.Error()})
		return
	}

	// 验证网卡名称
	if req.DefaultNetwork == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "网卡名称不能为空"})
		return
	}

	// 保存到配置文件
	if err := saveNetworkConfigToEnv(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "网卡配置已保存",
		"data":    req,
	})
}

// getNetworkConfigFromEnv 从环境变量读取网卡配置
func getNetworkConfigFromEnv() (NetworkConfig, error) {
	cfg := NetworkConfig{DefaultNetwork: "auto"}

	// 从环境变量读取
	if networkName := os.Getenv("DEFAULT_NETWORK"); networkName != "" {
		cfg.DefaultNetwork = networkName
	}

	return cfg, nil
}

// saveNetworkConfigToFile 保存网卡配置到环境变量
func saveNetworkConfigToEnv(cfg NetworkConfig) error {
	// 设置环境变量
	os.Setenv("DEFAULT_NETWORK", cfg.DefaultNetwork)
	return nil
}
