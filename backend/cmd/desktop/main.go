package main

import (
	_ "embed"
	"log"
	"net/url"
	"os"
	"runtime"

	"hephaestus-backend/pkg/synpriv"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed build/appicon.png
var appIcon []byte

// desktopBootstrapQueryParam carries a single-use bootstrap nonce from the
// native host into the desktop webview. The desktop HTTP server exchanges the
// nonce for a session cookie holding the API token, so the long-lived token
// never appears in a window URL or in an HTTP access log.
const desktopBootstrapQueryParam = "desktop_bootstrap"

// desktopAPITokenQueryParam is only used by local development, where pages are
// served by the Next.js dev server and the desktop backend never observes the
// document request that would otherwise carry a bootstrap nonce.
const desktopAPITokenQueryParam = "desktop_api_token"

func resolveDesktopWindowURL(serverURL string, routePath string, queryParams map[string]string) string {
	devURL := os.Getenv("FRONTEND_DEV_URL")
	if devURL == "" {
		target, err := url.Parse(serverURL)
		if err != nil {
			return serverURL
		}
		target.Path = routePath
		query := target.Query()
		for key, value := range queryParams {
			if value == "" {
				continue
			}
			query.Set(key, value)
		}
		target.RawQuery = query.Encode()
		return target.String()
	}

	frontendURL, err := url.Parse(devURL)
	if err != nil {
		return serverURL
	}
	frontendURL.Path = routePath
	backendURL, err := url.Parse(serverURL)
	if err != nil {
		return devURL
	}

	query := frontendURL.Query()
	query.Set("backend_port", backendURL.Port())
	for key, value := range queryParams {
		if value == "" {
			continue
		}
		query.Set(key, value)
	}
	frontendURL.RawQuery = query.Encode()
	return frontendURL.String()
}

func attachLauncherFileDropHandler(window *application.WebviewWindow) {
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		ctx := event.Context()
		if ctx == nil {
			return
		}
		details := ctx.DropTargetDetails()
		if details == nil || details.Attributes == nil {
			return
		}
		groupName := details.Attributes["data-group"]
		if groupName == "" {
			return
		}
		files := ctx.DroppedFiles()
		if len(files) == 0 {
			return
		}
		if err := addDroppedFilesToGroup(groupName, files); err != nil {
			log.Printf("launcher file drop failed: %v", err)
			return
		}
		window.ExecJS(`window.dispatchEvent(new CustomEvent("hephaestus:launcher-updated"));`)
	})
}

func main() {
	if synpriv.MaybeRunHelper() {
		return
	}
	if synpriv.DenyRegularHelperStartup() {
		log.Println("SYN helper can only be used through the discovery helper interface")
		return
	}

	if maybeRunDesktopUpdateHelper() {
		return
	}

	execPath, err := os.Executable()
	if err == nil {
		cleanupDesktopUpdateArtifacts(execPath)
	}

	if err := ensureDesktopRuntimePrerequisites(); err != nil {
		reportDesktopStartupFailure(err)
		return
	}

	desktopService, err := NewDesktopService()
	if err != nil {
		reportDesktopStartupFailure(err)
		return
	}

	app := application.New(application.Options{
		Name:        "Hephaestus",
		Description: "Hephaestus desktop client",
		Icon:        appIcon,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.hephaestus.desktop",
			OnSecondInstanceLaunch: func(_ application.SecondInstanceData) {
				desktopService.RevealMainWindow()
			},
		},
		Services: []application.Service{
			application.NewService(desktopService),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	desktopService.AttachApp(app)

	windowWidth, windowHeight, minWindowWidth, minWindowHeight, primaryScreen := resolveDesktopWindowBounds(app)

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                       "main",
		Title:                      "Hephaestus",
		Width:                      windowWidth,
		Height:                     windowHeight,
		MinWidth:                   minWindowWidth,
		MinHeight:                  minWindowHeight,
		InitialPosition:            application.WindowCentered,
		Screen:                     primaryScreen,
		URL:                        desktopService.windowURL(""),
		Frameless:                  runtime.GOOS != "darwin",
		EnableFileDrop:             true,
		DefaultContextMenuDisabled: true,
		BackgroundColour:           application.NewRGB(16, 19, 24),
		UseApplicationMenu:         true,
		Windows: application.WindowsWindow{
			DisableFramelessWindowDecorations: false,
		},
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHidden,
			InvisibleTitleBarHeight: 48,
		},
	})
	desktopService.AttachWindow(window)
	attachLauncherFileDropHandler(window)
	window.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		desktopService.CloseChildWindows()
	})
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(_ *application.ApplicationEvent) {
		desktopService.RevealMainWindow()
	})

	if err := app.Run(); err != nil {
		reportDesktopStartupFailure(err)
	}
}
