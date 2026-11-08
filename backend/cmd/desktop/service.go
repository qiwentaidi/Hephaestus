package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	appcore "hephaestus-backend/internal/app"
	"hephaestus-backend/middleware"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type DesktopService struct {
	listener                 net.Listener
	mcpListener              net.Listener
	serverURL                string
	mcpServerURL             string
	mcpListenHost            string
	mcpListenPort            string
	apiToken                 string
	preferredBackendPort     string
	temporaryBackendPort     bool
	backendPortFallbackError string
	coreApp                  *appcore.App
	httpServer               *http.Server
	mcpHTTPServer            *http.Server
	mcpMu                    sync.Mutex
	windowMu                 sync.RWMutex
	window                   *application.WebviewWindow
	settingsWindowMu         sync.RWMutex
	settingsWindow           *application.WebviewWindow
	launcherWindowMu         sync.RWMutex
	launcherWindow           *application.WebviewWindow
	appMu                    sync.RWMutex
	app                      *application.App
	bootstrapMu              sync.Mutex
	bootstrapNonces          map[string]time.Time
}

var desktopStorageMu sync.Mutex

const (
	desktopAPITokenCookieName = "hephaestus_desktop_api_token"
	desktopBackendHost        = "127.0.0.1"
	desktopAllInterfacesHost  = "0.0.0.0"
	desktopDefaultBackendPort = "38456"
	// desktopBootstrapNonceTTL bounds how long a native-issued window nonce may
	// be exchanged for the desktop API credential.
	desktopBootstrapNonceTTL = 2 * time.Minute
)

func NewDesktopService() (*DesktopService, error) {
	listener, preferredPort, temporaryPort, fallbackError, err := reserveDesktopBackendListener()
	if err != nil {
		return nil, fmt.Errorf("failed to reserve local desktop port: %w", err)
	}
	apiToken, err := newDesktopAPIToken()
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("failed to create desktop api token: %w", err)
	}

	return &DesktopService{
		listener:                 listener,
		serverURL:                "http://" + listener.Addr().String(),
		mcpServerURL:             "http://" + listener.Addr().String(),
		mcpListenHost:            desktopBackendHost,
		mcpListenPort:            listenerPort(listener),
		apiToken:                 apiToken,
		preferredBackendPort:     preferredPort,
		temporaryBackendPort:     temporaryPort,
		backendPortFallbackError: fallbackError,
		coreApp:                  appcore.New(),
	}, nil
}

func reserveDesktopBackendListener() (net.Listener, string, bool, string, error) {
	preferredPort := strings.TrimSpace(os.Getenv("HEPHAESTUS_DESKTOP_BACKEND_PORT"))
	if preferredPort == "" {
		preferredPort = desktopDefaultBackendPort
	}

	preferredAddr := net.JoinHostPort(desktopBackendHost, preferredPort)
	listener, err := net.Listen("tcp", preferredAddr)
	if err == nil {
		return listener, preferredPort, false, "", nil
	}

	fallbackListener, fallbackErr := net.Listen("tcp", net.JoinHostPort(desktopBackendHost, "0"))
	if fallbackErr != nil {
		return nil, preferredPort, false, "", fmt.Errorf("%s unavailable: %w; temporary port unavailable: %w", preferredAddr, err, fallbackErr)
	}

	return fallbackListener, preferredPort, true, err.Error(), nil
}

func (d *DesktopService) URL() string {
	return d.serverURL
}

func (d *DesktopService) APIToken() string {
	return d.apiToken
}

// windowURL builds the URL for a desktop webview window. Packaged builds never
// put the long-lived desktop API token in that URL: the native host injects a
// single-use nonce instead, and the desktop HTTP server exchanges it for a
// session cookie (see attachDesktopBootstrapCookie). Local development keeps
// the token in the URL because the document is served by the Next.js dev
// server, so the backend never observes the nonce-bearing navigation.
func (d *DesktopService) windowURL(routePath string) string {
	if os.Getenv("FRONTEND_DEV_URL") != "" {
		return resolveDesktopWindowURL(d.URL(), routePath, map[string]string{
			desktopAPITokenQueryParam: d.apiToken,
		})
	}

	nonce, err := d.newBootstrapNonce()
	if err != nil {
		log.Printf("failed to issue desktop bootstrap nonce: %v", err)
		return resolveDesktopWindowURL(d.URL(), routePath, nil)
	}
	return resolveDesktopWindowURL(d.URL(), routePath, map[string]string{
		desktopBootstrapQueryParam: nonce,
	})
}

// newBootstrapNonce mints a single-use, short-lived credential that only the
// native host can hand to a webview.
func (d *DesktopService) newBootstrapNonce() (string, error) {
	nonce, err := newDesktopAPIToken()
	if err != nil {
		return "", err
	}

	now := time.Now()
	d.bootstrapMu.Lock()
	defer d.bootstrapMu.Unlock()
	if d.bootstrapNonces == nil {
		d.bootstrapNonces = make(map[string]time.Time)
	}
	for value, expiresAt := range d.bootstrapNonces {
		if !now.Before(expiresAt) {
			delete(d.bootstrapNonces, value)
		}
	}
	d.bootstrapNonces[nonce] = now.Add(desktopBootstrapNonceTTL)
	return nonce, nil
}

// consumeBootstrapNonce validates and invalidates a bootstrap nonce. A nonce
// can be exchanged at most once, so a value captured from a URL, a log entry or
// the webview history cannot be replayed.
func (d *DesktopService) consumeBootstrapNonce(nonce string) bool {
	nonce = strings.TrimSpace(nonce)
	if nonce == "" {
		return false
	}

	d.bootstrapMu.Lock()
	defer d.bootstrapMu.Unlock()
	expiresAt, ok := d.bootstrapNonces[nonce]
	if !ok {
		return false
	}
	delete(d.bootstrapNonces, nonce)
	return time.Now().Before(expiresAt)
}

func (d *DesktopService) backendPort() string {
	return listenerPort(d.listener)
}

func listenerPort(listener net.Listener) string {
	if listener == nil {
		return ""
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return ""
	}
	return port
}

func (d *DesktopService) currentMCPServerURL() string {
	d.mcpMu.Lock()
	defer d.mcpMu.Unlock()
	if strings.TrimSpace(d.mcpServerURL) != "" {
		return d.mcpServerURL
	}
	return d.serverURL
}

func (d *DesktopService) currentMCPListenHost() string {
	d.mcpMu.Lock()
	defer d.mcpMu.Unlock()
	if strings.TrimSpace(d.mcpListenHost) != "" {
		return d.mcpListenHost
	}
	return desktopBackendHost
}

func (d *DesktopService) currentMCPListenPort() string {
	d.mcpMu.Lock()
	defer d.mcpMu.Unlock()
	if strings.TrimSpace(d.mcpListenPort) != "" {
		return d.mcpListenPort
	}
	return d.backendPort()
}

func normalizeDesktopMCPListenHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" || host == "localhost" {
		return desktopBackendHost, nil
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("仅支持 IPv4 监听地址")
	}
	if ip.IsUnspecified() {
		return desktopAllInterfacesHost, nil
	}
	if ip.IsLoopback() {
		return desktopBackendHost, nil
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

func normalizeDesktopMCPListenPort(port string) (string, error) {
	port = strings.TrimSpace(port)
	if port == "" {
		return "", nil
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return "", fmt.Errorf("监听端口必须是 1-65535 之间的数字")
	}
	return strconv.Itoa(value), nil
}

func desktopMCPAdvertiseHost(listenHost string) string {
	if listenHost != desktopAllInterfacesHost {
		return listenHost
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return desktopBackendHost
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
	return desktopBackendHost
}

func (d *DesktopService) configureMCPListener(ctx context.Context, router *gin.Engine, host string, port string) (gin.H, error) {
	host, err := normalizeDesktopMCPListenHost(host)
	if err != nil {
		return nil, err
	}
	port, err = normalizeDesktopMCPListenPort(port)
	if err != nil {
		return nil, err
	}
	if port == "" {
		port = d.backendPort()
	}
	if port == "" {
		port = d.preferredBackendPort
	}

	d.mcpMu.Lock()
	defer d.mcpMu.Unlock()

	backendPort := d.backendPort()
	if host == desktopBackendHost && port == backendPort {
		if d.mcpHTTPServer != nil {
			shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_ = d.mcpHTTPServer.Shutdown(shutdownCtx)
			cancel()
			d.mcpHTTPServer = nil
		}
		if d.mcpListener != nil {
			_ = d.mcpListener.Close()
			d.mcpListener = nil
		}
		d.mcpListenHost = desktopBackendHost
		d.mcpListenPort = backendPort
		d.mcpServerURL = d.serverURL
		return d.mcpListenerPayloadLocked(), nil
	}

	if d.mcpListener != nil && d.mcpListenHost == host && d.mcpListenPort == port {
		return d.mcpListenerPayloadLocked(), nil
	}

	if d.mcpListener != nil && d.mcpListenPort == port {
		if d.mcpHTTPServer != nil {
			shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_ = d.mcpHTTPServer.Shutdown(shutdownCtx)
			cancel()
			d.mcpHTTPServer = nil
		}
		_ = d.mcpListener.Close()
		d.mcpListener = nil
	}

	listenAddr := net.JoinHostPort(host, port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("监听 %s 失败，端口可能已被占用: %w", listenAddr, err)
	}

	server := d.coreApp.NewHTTPServer(listener.Addr().String(), router)
	oldServer := d.mcpHTTPServer
	oldListener := d.mcpListener
	d.mcpHTTPServer = server
	d.mcpListener = listener
	d.mcpListenHost = host
	d.mcpListenPort = port
	d.mcpServerURL = "http://" + net.JoinHostPort(desktopMCPAdvertiseHost(host), port)

	go func() {
		_ = server.Serve(listener)
	}()

	if oldServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = oldServer.Shutdown(shutdownCtx)
		cancel()
	}
	if oldListener != nil {
		_ = oldListener.Close()
	}

	return d.mcpListenerPayloadLocked(), nil
}

func (d *DesktopService) mcpListenerPayloadLocked() gin.H {
	scope := "local"
	if d.mcpListenHost != "" && d.mcpListenHost != desktopBackendHost {
		scope = "intranet"
	}
	return gin.H{
		"mcpServerURL":   d.mcpServerURL,
		"mcpListenHost":  d.mcpListenHost,
		"mcpListenPort":  d.mcpListenPort,
		"mcpAccessScope": scope,
	}
}

func (d *DesktopService) AttachWindow(window *application.WebviewWindow) {
	d.windowMu.Lock()
	defer d.windowMu.Unlock()
	d.window = window
}

func (d *DesktopService) AttachApp(app *application.App) {
	d.appMu.Lock()
	defer d.appMu.Unlock()
	d.app = app
}

func (d *DesktopService) getWindow() *application.WebviewWindow {
	d.windowMu.RLock()
	defer d.windowMu.RUnlock()
	return d.window
}

func (d *DesktopService) getSettingsWindow() *application.WebviewWindow {
	d.settingsWindowMu.RLock()
	defer d.settingsWindowMu.RUnlock()
	return d.settingsWindow
}

func (d *DesktopService) getLauncherWindow() *application.WebviewWindow {
	d.launcherWindowMu.RLock()
	defer d.launcherWindowMu.RUnlock()
	return d.launcherWindow
}

func (d *DesktopService) setSettingsWindow(window *application.WebviewWindow) {
	d.settingsWindowMu.Lock()
	defer d.settingsWindowMu.Unlock()
	d.settingsWindow = window
}

func (d *DesktopService) setLauncherWindow(window *application.WebviewWindow) {
	d.launcherWindowMu.Lock()
	defer d.launcherWindowMu.Unlock()
	d.launcherWindow = window
}

func (d *DesktopService) broadcastDesktopStorageItems(items map[string]string) {
	if len(items) == 0 {
		return
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return
	}
	script := fmt.Sprintf(`(() => {
const items = %s;
for (const [key, value] of Object.entries(items)) {
	window.localStorage.setItem(key, String(value));
	window.dispatchEvent(new StorageEvent("storage", { key, newValue: String(value) }));
}
window.dispatchEvent(new Event("storage"));
window.dispatchEvent(new CustomEvent("hephaestus:desktop-storage-updated", { detail: { items } }));
})()`, string(payload))
	d.execJSInDesktopWindows(script)
}

func (d *DesktopService) broadcastDesktopStorageRemoval(keys []string) {
	if len(keys) == 0 {
		return
	}
	payload, err := json.Marshal(keys)
	if err != nil {
		return
	}
	script := fmt.Sprintf(`(() => {
const keys = %s;
for (const key of keys) {
	window.localStorage.removeItem(key);
	window.dispatchEvent(new StorageEvent("storage", { key, newValue: null }));
}
window.dispatchEvent(new Event("storage"));
window.dispatchEvent(new CustomEvent("hephaestus:desktop-storage-updated", { detail: { keys } }));
})()`, string(payload))
	d.execJSInDesktopWindows(script)
}

func (d *DesktopService) execJSInDesktopWindows(script string) {
	if window := d.getWindow(); window != nil {
		window.ExecJS(script)
	}
	if window := d.getSettingsWindow(); window != nil {
		window.ExecJS(script)
	}
	if window := d.getLauncherWindow(); window != nil {
		window.ExecJS(script)
	}
}

func (d *DesktopService) getApp() *application.App {
	d.appMu.RLock()
	defer d.appMu.RUnlock()
	return d.app
}

func (d *DesktopService) RevealMainWindow() {
	if app := d.getApp(); app != nil {
		application.InvokeSync(app.Show)
	}
	if window := d.getWindow(); window != nil {
		window.Show()
		if window.IsMinimised() {
			window.UnMinimise()
		}
		window.Focus()
	}
}

func (d *DesktopService) CloseChildWindows() {
	if window := d.getSettingsWindow(); window != nil {
		window.Close()
		d.setSettingsWindow(nil)
	}
	if window := d.getLauncherWindow(); window != nil {
		window.Close()
		d.setLauncherWindow(nil)
	}
}

func (d *DesktopService) openSettingsWindow() error {
	if window := d.getSettingsWindow(); window != nil {
		window.Show()
		window.Focus()
		return nil
	}

	app := d.getApp()
	if app == nil {
		return fmt.Errorf("desktop app unavailable")
	}

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                       "settings",
		Title:                      "Setting",
		Width:                      1180,
		Height:                     780,
		MinWidth:                   900,
		MinHeight:                  620,
		InitialPosition:            application.WindowCentered,
		URL:                        d.windowURL("/settings-window"),
		Frameless:                  runtime.GOOS != "darwin",
		BackgroundColour:           application.NewRGB(16, 19, 24),
		DefaultContextMenuDisabled: true,
		UseApplicationMenu:         true,
		Windows: application.WindowsWindow{
			DisableFramelessWindowDecorations: false,
		},
		Mac: application.MacWindow{
			Appearance:              application.NSAppearanceNameDarkAqua,
			TitleBar:                application.MacTitleBarHidden,
			InvisibleTitleBarHeight: 48,
		},
	})
	d.setSettingsWindow(window)
	window.OnWindowEvent(events.Common.WindowClosing, func(event *application.WindowEvent) {
		d.setSettingsWindow(nil)
	})
	window.Show()
	window.Focus()
	return nil
}

func (d *DesktopService) openLauncherWindow() error {
	if window := d.getLauncherWindow(); window != nil {
		window.Show()
		window.Focus()
		return nil
	}

	app := d.getApp()
	if app == nil {
		return fmt.Errorf("desktop app unavailable")
	}

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                       "launcher",
		Title:                      "Launcher",
		Width:                      1180,
		Height:                     780,
		MinWidth:                   900,
		MinHeight:                  620,
		InitialPosition:            application.WindowCentered,
		URL:                        d.windowURL("/launcher-window"),
		Frameless:                  runtime.GOOS != "darwin",
		EnableFileDrop:             true,
		BackgroundColour:           application.NewRGB(16, 19, 24),
		DefaultContextMenuDisabled: true,
		UseApplicationMenu:         true,
		Windows: application.WindowsWindow{
			DisableFramelessWindowDecorations: false,
		},
		Mac: application.MacWindow{
			Appearance:              application.NSAppearanceNameDarkAqua,
			TitleBar:                application.MacTitleBarHidden,
			InvisibleTitleBarHeight: 48,
		},
	})
	d.setLauncherWindow(window)
	window.OnWindowEvent(events.Common.WindowClosing, func(event *application.WindowEvent) {
		d.setLauncherWindow(nil)
	})
	attachLauncherFileDropHandler(window)
	window.Show()
	window.Focus()
	return nil
}

func (d *DesktopService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	config.SetRuntimeMode(config.RuntimeModeDesktop)

	if err := d.coreApp.Bootstrap(); err != nil {
		return err
	}
	if err := ensureLegacySlackLauncherMigrated(); err != nil {
		return err
	}

	router := d.coreApp.BuildRouter(true)
	router.Use(d.attachDesktopBootstrapCookie())
	d.registerDesktopRoutes(router)
	d.httpServer = d.coreApp.NewHTTPServer(d.listener.Addr().String(), router)

	go func() {
		_ = d.httpServer.Serve(d.listener)
	}()

	return nil
}

// attachDesktopBootstrapCookie hands the desktop API token to a webview that
// presents a valid, unused bootstrap nonce. The nonce is minted by the native
// host for a single window load, so unauthenticated HTTP clients - including
// peers that can reach the intranet MCP listener - never receive the token.
func (d *DesktopService) attachDesktopBootstrapCookie() gin.HandlerFunc {
	return func(c *gin.Context) {
		nonce := c.Query(desktopBootstrapQueryParam)
		if nonce != "" {
			if d.consumeBootstrapNonce(nonce) {
				c.SetCookie(
					desktopAPITokenCookieName,
					d.apiToken,
					0,
					"/",
					"",
					false,
					false,
				)
			} else {
				log.Printf("rejected desktop bootstrap nonce from %s", c.ClientIP())
			}
		}
		c.Next()
	}
}

func (d *DesktopService) registerDesktopRoutes(router *gin.Engine) {
	router.GET("/api/v1/desktop/meta", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"isDesktop":                true,
				"platform":                 runtime.GOOS,
				"isMaximised":              d.windowState(),
				"isFullscreen":             d.windowFullscreenState(),
				"serverURL":                d.serverURL,
				"mcpServerURL":             d.currentMCPServerURL(),
				"mcpListenHost":            d.currentMCPListenHost(),
				"mcpListenPort":            d.currentMCPListenPort(),
				"backendPort":              d.backendPort(),
				"preferredBackendPort":     d.preferredBackendPort,
				"isTemporaryBackendPort":   d.temporaryBackendPort,
				"backendPortFallbackError": d.backendPortFallbackError,
				"mcpAccessScope": func() string {
					if d.currentMCPListenHost() == desktopBackendHost {
						return "local"
					}
					return "intranet"
				}(),
			},
		})
	})

	api := router.Group("/api/v1/desktop")
	api.Use(d.requireDesktopAPIToken())
	{
		api.GET("/auth/session", func(c *gin.Context) {
			token, err := readDesktopAuthToken()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"token": token}})
		})

		api.PUT("/mcp/listener", func(c *gin.Context) {
			var req struct {
				Host string `json:"host"`
				Port string `json:"port"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
				return
			}
			payload, err := d.configureMCPListener(context.Background(), router, req.Host, req.Port)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": payload})
		})

		api.PUT("/auth/session", func(c *gin.Context) {
			var req struct {
				Token string `json:"token"`
			}
			if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Token) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid token"})
				return
			}
			if err := writeDesktopAuthToken(req.Token); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.DELETE("/auth/session", func(c *gin.Context) {
			if err := clearDesktopAuthToken(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.GET("/storage", func(c *gin.Context) {
			items, err := readDesktopStorage()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items}})
		})

		api.PUT("/storage", func(c *gin.Context) {
			var req struct {
				Items map[string]string `json:"items"`
			}
			if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid items"})
				return
			}
			if err := mergeDesktopStorage(req.Items); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			d.broadcastDesktopStorageItems(req.Items)
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.DELETE("/storage", func(c *gin.Context) {
			var req struct {
				Keys []string `json:"keys"`
			}
			if err := c.ShouldBindJSON(&req); err != nil || len(req.Keys) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid keys"})
				return
			}
			if err := deleteDesktopStorageKeys(req.Keys); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			d.broadcastDesktopStorageRemoval(req.Keys)
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.GET("/window/state", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"isMaximised":  d.windowState(),
					"isFullscreen": d.windowFullscreenState(),
				},
			})
		})

		api.POST("/window/minimise", func(c *gin.Context) {
			if window := d.getWindow(); window != nil {
				window.Minimise()
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/window/start-resize", func(c *gin.Context) {
			edge, ok := desktopResizeEdge(c)
			if !ok {
				return
			}
			if window := d.getWindow(); window != nil {
				window.HandleMessage("wails:resize:" + edge)
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/window/toggle-maximise", func(c *gin.Context) {
			if window := d.getWindow(); window != nil {
				if window.IsMaximised() {
					window.UnMaximise()
				} else {
					window.Maximise()
				}
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"isMaximised":  d.windowState(),
					"isFullscreen": d.windowFullscreenState(),
				},
			})
		})

		api.POST("/window/toggle-fullscreen", func(c *gin.Context) {
			if window := d.getWindow(); window != nil {
				window.ToggleFullscreen()
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"isMaximised":  d.windowState(),
					"isFullscreen": d.windowFullscreenState(),
				},
			})
		})

		api.POST("/window/fit", func(c *gin.Context) {
			var req struct {
				AvailWidth  int `json:"availWidth"`
				AvailHeight int `json:"availHeight"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
				return
			}

			if window := d.getWindow(); window != nil && req.AvailWidth > 0 && req.AvailHeight > 0 && !window.IsMaximised() && !window.IsFullscreen() {
				metrics := computeDesktopWindowMetrics(req.AvailWidth, req.AvailHeight)
				window.SetMinSize(metrics.minWidth, metrics.minHeight)
				window.SetSize(metrics.width, metrics.height)
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"isMaximised":  d.windowState(),
					"isFullscreen": d.windowFullscreenState(),
				},
			})
		})

		api.POST("/window/titlebar-double-click", func(c *gin.Context) {
			if window := d.getWindow(); window != nil {
				handleDesktopTitlebarDoubleClick(window)
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"isMaximised":  d.windowState(),
					"isFullscreen": d.windowFullscreenState(),
				},
			})
		})

		api.POST("/window/start-drag", func(c *gin.Context) {
			if window := d.getWindow(); window != nil {
				window.HandleMessage("wails:drag")
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/window/close", func(c *gin.Context) {
			if window := d.getWindow(); window != nil {
				window.Close()
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/settings/open", func(c *gin.Context) {
			if err := d.openSettingsWindow(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/settings/window/minimise", func(c *gin.Context) {
			if window := d.getSettingsWindow(); window != nil {
				window.Minimise()
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/settings/window/start-resize", func(c *gin.Context) {
			edge, ok := desktopResizeEdge(c)
			if !ok {
				return
			}
			if window := d.getSettingsWindow(); window != nil {
				window.HandleMessage("wails:resize:" + edge)
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.GET("/settings/window/state", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"isMaximised": d.settingsWindowState(),
				},
			})
		})

		api.POST("/settings/window/toggle-maximise", func(c *gin.Context) {
			if window := d.getSettingsWindow(); window != nil {
				if window.IsMaximised() {
					window.UnMaximise()
				} else {
					window.Maximise()
				}
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"isMaximised": d.settingsWindowState(),
				},
			})
		})

		api.POST("/settings/window/titlebar-double-click", func(c *gin.Context) {
			if window := d.getSettingsWindow(); window != nil {
				handleDesktopTitlebarDoubleClick(window)
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"isMaximised": d.settingsWindowState(),
				},
			})
		})

		api.POST("/settings/window/start-drag", func(c *gin.Context) {
			if window := d.getSettingsWindow(); window != nil {
				window.HandleMessage("wails:drag")
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/settings/window/close", func(c *gin.Context) {
			if window := d.getSettingsWindow(); window != nil {
				window.Close()
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/launcher/open", func(c *gin.Context) {
			if err := d.openLauncherWindow(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/launcher/window/minimise", func(c *gin.Context) {
			if window := d.getLauncherWindow(); window != nil {
				window.Minimise()
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/launcher/window/start-resize", func(c *gin.Context) {
			edge, ok := desktopResizeEdge(c)
			if !ok {
				return
			}
			if window := d.getLauncherWindow(); window != nil {
				window.HandleMessage("wails:resize:" + edge)
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.GET("/launcher/window/state", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"isMaximised": d.launcherWindowState(),
				},
			})
		})

		api.POST("/launcher/window/toggle-maximise", func(c *gin.Context) {
			if window := d.getLauncherWindow(); window != nil {
				if window.IsMaximised() {
					window.UnMaximise()
				} else {
					window.Maximise()
				}
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"isMaximised": d.launcherWindowState(),
				},
			})
		})

		api.POST("/launcher/window/titlebar-double-click", func(c *gin.Context) {
			if window := d.getLauncherWindow(); window != nil {
				handleDesktopTitlebarDoubleClick(window)
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"isMaximised": d.launcherWindowState(),
				},
			})
		})

		api.POST("/launcher/window/start-drag", func(c *gin.Context) {
			if window := d.getLauncherWindow(); window != nil {
				window.HandleMessage("wails:drag")
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/launcher/window/close", func(c *gin.Context) {
			if window := d.getLauncherWindow(); window != nil {
				window.Close()
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/save-download", func(c *gin.Context) {
			filename := strings.TrimSpace(c.PostForm("filename"))
			if filename == "" {
				filename = "download.bin"
			}

			fileHeader, err := c.FormFile("file")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing file"})
				return
			}
			file, err := fileHeader.Open()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			defer file.Close()

			content, err := io.ReadAll(file)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}

			app := d.getApp()
			window := d.getWindow()
			if app == nil || window == nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "desktop dialogs unavailable"})
				return
			}

			dialog := app.Dialog.SaveFile().
				AttachToWindow(window).
				SetMessage("保存文件").
				SetButtonText("保存").
				SetFilename(filename).
				CanCreateDirectories(true)

			if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), "."); ext != "" {
				dialog.AddFilter(strings.ToUpper(ext)+" files", "*."+ext)
			}

			selected, err := dialog.PromptForSingleSelection()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			if strings.TrimSpace(selected) == "" {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"cancelled": true}})
				return
			}

			if err := os.WriteFile(selected, content, 0o644); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"path": selected}})
		})

		api.POST("/open-external", func(c *gin.Context) {
			var req struct {
				URL string `json:"url"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
				return
			}

			target, err := normalizeDesktopExternalURL(req.URL)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
				return
			}

			app := d.getApp()
			if app == nil {
				if err := openURL(target); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
					return
				}
			} else if err := app.Browser.OpenURL(target); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		api.POST("/update/install-latest", func(c *gin.Context) {
			if runtime.GOOS != "darwin" {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "当前平台不支持 dmg 自动安装"})
				return
			}
			if _, err := latestDesktopUpdateDMGPath(); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "正在打开安装器"})
			d.launchInstalledUpdateAndQuit()
		})
	}

	launcher := api.Group("/launcher")
	launcher.Use(middleware.JWTAuthMiddleware())
	{
		launcher.GET("/config", func(c *gin.Context) {
			groups, err := readLauncherGroups()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			jdks, err := readLauncherJDKs()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"groups": groups, "jdks": jdks}})
		})

		launcher.PUT("/navigation", func(c *gin.Context) {
			var groups []LauncherGroup
			if err := c.ShouldBindJSON(&groups); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
				return
			}
			if err := writeLauncherGroups(groups); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		launcher.PUT("/jdks", func(c *gin.Context) {
			var jdks []LauncherJDK
			if err := c.ShouldBindJSON(&jdks); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
				return
			}
			if err := writeLauncherJDKs(jdks); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		launcher.POST("/run", func(c *gin.Context) {
			var item LauncherItem
			if err := c.ShouldBindJSON(&item); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
				return
			}
			if err := runLauncherItem(item); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		launcher.POST("/open-folder", func(c *gin.Context) {
			var req struct {
				Path string `json:"path"`
			}
			if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Path) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid path"})
				return
			}
			if err := openFolder(req.Path); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		launcher.POST("/open-terminal", func(c *gin.Context) {
			var req struct {
				Path string `json:"path"`
			}
			if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Path) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid path"})
				return
			}
			if err := openTerminalForPath(req.Path); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true})
		})

		launcher.POST("/generate-icon", func(c *gin.Context) {
			var req struct {
				Kind  string `json:"kind"`
				Value string `json:"value"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
				return
			}
			var icon string
			switch strings.ToLower(strings.TrimSpace(req.Kind)) {
			case "url":
				icon = generateLauncherIconFromURL(req.Value)
			default:
				icon = generateLauncherIconFromFile(req.Value)
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"icon": icon}})
		})

		launcher.POST("/select-path", func(c *gin.Context) {
			var req struct {
				Directory bool `json:"directory"`
				Icon      bool `json:"icon"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
				return
			}

			app := d.getApp()
			window := d.getWindow()
			if app == nil || window == nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "desktop dialogs unavailable"})
				return
			}

			dialog := app.Dialog.OpenFile().AttachToWindow(window)
			if req.Directory {
				dialog.CanChooseDirectories(true).CanChooseFiles(false).SetTitle("选择目录")
			} else {
				dialog.CanChooseFiles(true).CanChooseDirectories(false).SetTitle("选择文件")
			}
			if req.Icon {
				dialog.AddFilter("Images", "*.png;*.jpg;*.jpeg;*.svg;*.ico;*.icns;*.webp")
			}
			selected, err := dialog.PromptForSingleSelection()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"path": selected}})
		})
	}
}

func desktopResizeEdge(c *gin.Context) (string, bool) {
	edge := c.Query("edge")
	switch edge {
	case "n-resize", "ne-resize", "e-resize", "se-resize", "s-resize", "sw-resize", "w-resize", "nw-resize":
		return edge, true
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid resize edge"})
		return "", false
	}
}

func (d *DesktopService) requireDesktopAPIToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		// Only the explicit request header authorizes desktop routes. The
		// bootstrap cookie is a transport for the webview, never an
		// authentication factor on its own.
		token := strings.TrimSpace(c.GetHeader("X-Hephaestus-Desktop-Token"))
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(d.apiToken)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "desktop api forbidden",
			})
			return
		}
		c.Next()
	}
}

func newDesktopAPIToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func readDesktopAuthToken() (string, error) {
	items, err := readDesktopStorage()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(items["auth_token"]), nil
}

func writeDesktopAuthToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return clearDesktopAuthToken()
	}
	return mergeDesktopStorage(map[string]string{"auth_token": token})
}

func clearDesktopAuthToken() error {
	return deleteDesktopStorageKeys([]string{"auth_token"})
}

func readDesktopStorage() (map[string]string, error) {
	desktopStorageMu.Lock()
	defer desktopStorageMu.Unlock()
	return readDesktopStorageLocked()
}

func readDesktopStorageLocked() (map[string]string, error) {
	value, err := database.GetSetting(database.SettingKeyDesktopConfig)
	if err != nil {
		return nil, fmt.Errorf("读取桌面端配置失败: %w", err)
	}
	items := map[string]string{}
	if value == nil {
		return items, nil
	}

	rawMap, ok := value.(map[string]interface{})
	if !ok {
		return items, nil
	}
	for key, raw := range rawMap {
		if key = strings.TrimSpace(key); key == "" {
			continue
		}
		if text, ok := raw.(string); ok {
			items[key] = text
		}
	}
	return items, nil
}

func writeDesktopStorageLocked(items map[string]string) error {
	if err := database.SetSetting(database.SettingKeyDesktopConfig, items); err != nil {
		return fmt.Errorf("写入桌面端配置失败: %w", err)
	}
	return nil
}

func mergeDesktopStorage(next map[string]string) error {
	desktopStorageMu.Lock()
	defer desktopStorageMu.Unlock()

	items, err := readDesktopStorageLocked()
	if err != nil {
		return err
	}
	for key, value := range next {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		items[key] = value
	}
	return writeDesktopStorageLocked(items)
}

func deleteDesktopStorageKeys(keys []string) error {
	desktopStorageMu.Lock()
	defer desktopStorageMu.Unlock()

	items, err := readDesktopStorageLocked()
	if err != nil {
		return err
	}
	for _, key := range keys {
		delete(items, strings.TrimSpace(key))
	}
	return writeDesktopStorageLocked(items)
}

func normalizeDesktopExternalURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("invalid url")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid url")
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	switch scheme {
	case "http", "https":
		if strings.TrimSpace(parsed.Host) == "" {
			return "", fmt.Errorf("invalid url")
		}
	case "mailto", "tel":
	default:
		return "", fmt.Errorf("unsupported url scheme")
	}

	return parsed.String(), nil
}

func (d *DesktopService) windowState() bool {
	if window := d.getWindow(); window != nil {
		return window.IsMaximised()
	}
	return false
}

func (d *DesktopService) windowFullscreenState() bool {
	if window := d.getWindow(); window != nil {
		return window.IsFullscreen()
	}
	return false
}

func (d *DesktopService) settingsWindowState() bool {
	if window := d.getSettingsWindow(); window != nil {
		return window.IsMaximised()
	}
	return false
}

func (d *DesktopService) launcherWindowState() bool {
	if window := d.getLauncherWindow(); window != nil {
		return window.IsMaximised()
	}
	return false
}

func (d *DesktopService) ServiceShutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = d.coreApp.Shutdown(shutdownCtx)
	if d.httpServer != nil {
		_ = d.httpServer.Shutdown(shutdownCtx)
	}
	if d.mcpHTTPServer != nil {
		_ = d.mcpHTTPServer.Shutdown(shutdownCtx)
	}
	if d.listener != nil {
		_ = d.listener.Close()
	}
	if d.mcpListener != nil {
		_ = d.mcpListener.Close()
	}

	return nil
}
