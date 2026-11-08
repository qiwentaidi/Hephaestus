package mcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/logger"

	"github.com/gin-gonic/gin"
)

const (
	mcpDefaultListenHost       = "127.0.0.1"
	mcpAllInterfacesListenHost = "0.0.0.0"
)

// mcpListenerRuntime 记录 server 模式下 MCP 专用监听的运行时状态
type mcpListenerRuntime struct {
	server   *http.Server
	listener net.Listener
	host     string
	port     string
	url      string
}

var (
	mcpListenerMu sync.Mutex
	mcpListenerRt mcpListenerRuntime
)

// NormalizeMCPListenHost 校验并规范化 MCP 监听地址（与桌面端逻辑一致）
func NormalizeMCPListenHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" || host == "localhost" {
		return mcpDefaultListenHost, nil
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("仅支持 IPv4 监听地址")
	}
	if ip.IsUnspecified() {
		return mcpAllInterfacesListenHost, nil
	}
	if ip.IsLoopback() {
		return mcpDefaultListenHost, nil
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			if ipNet.IP.Equal(ip) {
				return ip.String(), nil
			}
		}
	}

	return "", fmt.Errorf("监听地址必须来自当前系统网卡")
}

// NormalizeMCPListenPort 校验并规范化 MCP 监听端口（server 模式下端口必填）
func NormalizeMCPListenPort(port string) (string, error) {
	port = strings.TrimSpace(port)
	if port == "" {
		return "", fmt.Errorf("监听端口不能为空")
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return "", fmt.Errorf("监听端口必须是 1-65535 之间的数字")
	}
	return strconv.Itoa(value), nil
}

// mcpAdvertiseHost 计算对外展示的 MCP 访问地址主机部分
func mcpAdvertiseHost(listenHost string) string {
	if listenHost != mcpAllInterfacesListenHost {
		return listenHost
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return mcpDefaultListenHost
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
				return ip.String()
			}
		}
	}
	return mcpDefaultListenHost
}

// GetMCPListenerState 返回当前 MCP 监听状态，未配置时返回基于主端口的默认值
func GetMCPListenerState(mainPort int) gin.H {
	mcpListenerMu.Lock()
	defer mcpListenerMu.Unlock()
	return mcpListenerStateLocked(mainPort)
}

func mcpListenerStateLocked(mainPort int) gin.H {
	host := mcpListenerRt.host
	port := mcpListenerRt.port
	serverURL := mcpListenerRt.url
	if host == "" {
		host = mcpDefaultListenHost
	}
	if port == "" {
		port = strconv.Itoa(mainPort)
	}

	scope := "local"
	if host != "" && host != mcpDefaultListenHost {
		scope = "intranet"
	}

	return gin.H{
		"mcpListenHost":  host,
		"mcpListenPort":  port,
		"mcpServerURL":   serverURL,
		"mcpAccessScope": scope,
		"dedicated":      mcpListenerRt.listener != nil,
	}
}

// ApplyMCPListener 应用 MCP 监听配置：端口与主服务一致时复用主服务，否则启动专用监听
func ApplyMCPListener(ctx context.Context, handler http.Handler, mainPort int, host, port string) (gin.H, error) {
	if config.GetRuntimeMode() == config.RuntimeModeDesktop {
		return nil, fmt.Errorf("桌面模式请使用 /api/v1/desktop/mcp/listener")
	}

	host, err := NormalizeMCPListenHost(host)
	if err != nil {
		return nil, err
	}
	port, err = NormalizeMCPListenPort(port)
	if err != nil {
		return nil, err
	}

	mcpListenerMu.Lock()
	defer mcpListenerMu.Unlock()

	if port == strconv.Itoa(mainPort) {
		// 主服务已监听 :mainPort 的全部网卡，无需专用监听
		shutdownMCPListenerLocked(ctx)
		mcpListenerRt.host = host
		mcpListenerRt.port = port
		mcpListenerRt.url = "http://" + net.JoinHostPort(mcpAdvertiseHost(host), port)
		return mcpListenerStateLocked(mainPort), nil
	}

	// 幂等：相同 host:port 且专用监听仍在运行时直接返回当前状态
	if mcpListenerRt.listener != nil && mcpListenerRt.host == host && mcpListenerRt.port == port {
		return mcpListenerStateLocked(mainPort), nil
	}

	listenAddr := net.JoinHostPort(host, port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("监听 %s 失败，端口可能已被占用: %w", listenAddr, err)
	}

	server := &http.Server{
		Addr:         listener.Addr().String(),
		Handler:      handler,
		ReadTimeout:  300 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  300 * time.Second,
	}

	oldServer := mcpListenerRt.server
	oldListener := mcpListenerRt.listener
	mcpListenerRt.server = server
	mcpListenerRt.listener = listener
	mcpListenerRt.host = host
	mcpListenerRt.port = port
	mcpListenerRt.url = "http://" + net.JoinHostPort(mcpAdvertiseHost(host), port)

	go func() {
		_ = server.Serve(listener)
	}()

	// 新监听成功启动后再关闭旧监听
	if oldServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = oldServer.Shutdown(shutdownCtx)
		cancel()
	}
	if oldListener != nil {
		_ = oldListener.Close()
	}

	return mcpListenerStateLocked(mainPort), nil
}

// RestoreMCPListener 启动时恢复持久化的 MCP 监听配置，失败仅告警不中断启动
func RestoreMCPListener(handler http.Handler, mainPort int) error {
	host, port := config.GetMCPListenConfig()
	if host == "" && port == "" {
		mcpListenerMu.Lock()
		mcpListenerRt = mcpListenerRuntime{}
		mcpListenerMu.Unlock()
		return nil
	}

	if _, err := ApplyMCPListener(context.Background(), handler, mainPort, host, port); err != nil {
		logger.Warning("恢复 MCP 监听配置失败: %v", err)
		return err
	}
	logger.Info("MCP 监听配置已恢复")
	return nil
}

// ShutdownMCPListener 优雅关闭 MCP 专用监听
func ShutdownMCPListener(ctx context.Context) {
	mcpListenerMu.Lock()
	defer mcpListenerMu.Unlock()
	shutdownMCPListenerLocked(ctx)
	mcpListenerRt.host = ""
	mcpListenerRt.port = ""
	mcpListenerRt.url = ""
}

func shutdownMCPListenerLocked(ctx context.Context) {
	if mcpListenerRt.server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = mcpListenerRt.server.Shutdown(shutdownCtx)
		cancel()
		mcpListenerRt.server = nil
	}
	if mcpListenerRt.listener != nil {
		_ = mcpListenerRt.listener.Close()
		mcpListenerRt.listener = nil
	}
}
