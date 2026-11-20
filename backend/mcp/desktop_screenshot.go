package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"sync"
	"time"
)

const (
	desktopScreenshotToolName = "desktop_screenshot"
	desktopScreenshotTimeout  = 20 * time.Second
	maxDesktopScreenshotBytes = 32 << 20
)

// DesktopScreenshotOptions carries capture settings requested by an MCP caller.
type DesktopScreenshotOptions struct {
	// Scale is the raster scale (1 = one PNG pixel per CSS pixel).
	Scale float64
}

// DesktopScreenshot is the PNG captured from the desktop client window.
type DesktopScreenshot struct {
	PNG        []byte
	Width      int
	Height     int
	Route      string
	Title      string
	CapturedAt time.Time
}

// DesktopScreenshotProvider is implemented by the Wails desktop host. It must
// capture the main window in the live webview and return a PNG.
type DesktopScreenshotProvider interface {
	CaptureDesktopScreenshot(ctx context.Context, options DesktopScreenshotOptions) (*DesktopScreenshot, error)
}

var (
	desktopScreenshotProviderMu sync.RWMutex
	desktopScreenshotProvider   DesktopScreenshotProvider
)

// RegisterDesktopScreenshotProvider wires the desktop host into the MCP tool.
// Server-only deployments leave it unset and the tool is not advertised.
func RegisterDesktopScreenshotProvider(provider DesktopScreenshotProvider) {
	desktopScreenshotProviderMu.Lock()
	defer desktopScreenshotProviderMu.Unlock()
	desktopScreenshotProvider = provider
}

func currentDesktopScreenshotProvider() DesktopScreenshotProvider {
	desktopScreenshotProviderMu.RLock()
	defer desktopScreenshotProviderMu.RUnlock()
	return desktopScreenshotProvider
}

func hasDesktopScreenshotProvider() bool {
	return currentDesktopScreenshotProvider() != nil
}

func mcpCallDesktopScreenshot(ctx context.Context) (mcpToolCallResult, error) {
	provider := currentDesktopScreenshotProvider()
	if provider == nil {
		return mcpToolCallResult{}, errors.New("desktop_screenshot 仅在 Hephaestus 桌面客户端中可用")
	}

	callCtx, cancel := context.WithTimeout(ctx, desktopScreenshotTimeout)
	defer cancel()

	captured, err := provider.CaptureDesktopScreenshot(callCtx, DesktopScreenshotOptions{Scale: 1})
	if err != nil {
		return mcpToolCallResult{}, fmt.Errorf("截取桌面客户端窗口失败: %w", err)
	}
	if captured == nil || len(captured.PNG) == 0 {
		return mcpToolCallResult{}, errors.New("截取桌面客户端窗口失败: 未返回图像")
	}
	if len(captured.PNG) > maxDesktopScreenshotBytes {
		return mcpToolCallResult{}, fmt.Errorf("截取桌面客户端窗口失败: 图像超过 %d MB", maxDesktopScreenshotBytes>>20)
	}

	config, err := png.DecodeConfig(bytes.NewReader(captured.PNG))
	if err != nil {
		return mcpToolCallResult{}, fmt.Errorf("截取桌面客户端窗口失败: 无效的 PNG 图像: %w", err)
	}

	capturedAt := captured.CapturedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}
	width, height := captured.Width, captured.Height
	if width <= 0 || height <= 0 {
		width, height = config.Width, config.Height
	}

	encoded := base64.StdEncoding.EncodeToString(captured.PNG)
	structured := map[string]interface{}{
		"mimeType":    "image/png",
		"width":       width,
		"height":      height,
		"pixelWidth":  config.Width,
		"pixelHeight": config.Height,
		"capturedAt":  capturedAt.UTC().Format(time.RFC3339),
		"route":       captured.Route,
		"title":       captured.Title,
	}

	return mcpToolCallResult{
		Content: []mcpToolContent{
			{Type: "image", Data: encoded, MimeType: "image/png"},
			{
				Type: "text",
				Text: fmt.Sprintf(
					"已截取 Hephaestus 桌面客户端主窗口（%d×%d，%s）。截图反映操作者当前界面，可能包含敏感信息。",
					width,
					height,
					capturedAt.Local().Format("2006-01-02 15:04:05"),
				),
			},
		},
		StructuredContent: structured,
	}, nil
}
