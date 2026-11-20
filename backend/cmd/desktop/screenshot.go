package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"log"
	"net/http"
	"strings"
	"time"

	"hephaestus-backend/mcp"

	"github.com/gin-gonic/gin"
)

const (
	desktopScreenshotRequestEvent  = "hephaestus:desktop-screenshot-request"
	desktopScreenshotTimeout       = 20 * time.Second
	desktopScreenshotMaxBytes      = 32 << 20
	desktopScreenshotMaxBodyBytes  = 48 << 20
	desktopScreenshotDataURLPrefix = "data:image/png;base64,"
)

type desktopScreenshotOutcome struct {
	screenshot *mcp.DesktopScreenshot
	err        error
}

// CaptureDesktopScreenshot implements mcp.DesktopScreenshotProvider. The
// preferred path is a native window/webview snapshot (pixel-perfect, no screen
// recording permission). Platforms without native support, or a failed native
// capture, fall back to a DOM capture in the live webview.
func (d *DesktopService) CaptureDesktopScreenshot(
	ctx context.Context,
	options mcp.DesktopScreenshotOptions,
) (*mcp.DesktopScreenshot, error) {
	window := d.getWindow()
	if window == nil {
		return nil, errors.New("桌面客户端窗口尚未就绪")
	}

	native, nativeErr := captureNativeScreenshot(window, options.Scale)
	if nativeErr == nil {
		d.notifyDesktopScreenshotTaken()
		return &mcp.DesktopScreenshot{
			PNG:        native.PNG,
			Width:      native.Width,
			Height:     native.Height,
			CapturedAt: time.Now().UTC(),
		}, nil
	}
	if !errors.Is(nativeErr, errNativeScreenshotUnsupported) {
		log.Printf("native desktop screenshot failed, falling back to webview capture: %v", nativeErr)
	}
	return d.captureDesktopScreenshotViaWebview(ctx, options)
}

// captureDesktopScreenshotViaWebview asks the live webview to rasterize its
// current DOM and waits for the PNG to be posted back through the desktop API.
func (d *DesktopService) captureDesktopScreenshotViaWebview(
	ctx context.Context,
	options mcp.DesktopScreenshotOptions,
) (*mcp.DesktopScreenshot, error) {
	window := d.getWindow()
	if window == nil {
		return nil, errors.New("桌面客户端窗口尚未就绪")
	}

	requestID, err := newDesktopScreenshotRequestID()
	if err != nil {
		return nil, fmt.Errorf("生成截图请求失败: %w", err)
	}

	outcomeCh := make(chan desktopScreenshotOutcome, 1)
	d.screenshotMu.Lock()
	if d.screenshotPending == nil {
		d.screenshotPending = make(map[string]chan desktopScreenshotOutcome)
	}
	d.screenshotPending[requestID] = outcomeCh
	d.screenshotMu.Unlock()
	defer func() {
		d.screenshotMu.Lock()
		delete(d.screenshotPending, requestID)
		d.screenshotMu.Unlock()
	}()

	scale := options.Scale
	if scale <= 0 || scale > 2 {
		scale = 1
	}
	detail, err := json.Marshal(map[string]interface{}{
		"requestId": requestID,
		"scale":     scale,
	})
	if err != nil {
		return nil, fmt.Errorf("生成截图请求失败: %w", err)
	}

	script := fmt.Sprintf(
		`window.dispatchEvent(new CustomEvent(%q, { detail: %s }));`,
		desktopScreenshotRequestEvent,
		string(detail),
	)
	window.ExecJS(script)

	timer := time.NewTimer(desktopScreenshotTimeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, errors.New("截图超时，客户端未响应")
	case outcome := <-outcomeCh:
		return outcome.screenshot, outcome.err
	}
}

// completeDesktopScreenshot resolves a pending MCP capture request. It is
// called from the authenticated desktop API used by the webview.
func (d *DesktopService) completeDesktopScreenshot(requestID string, outcome desktopScreenshotOutcome) bool {
	d.screenshotMu.Lock()
	pending, ok := d.screenshotPending[requestID]
	if ok {
		delete(d.screenshotPending, requestID)
	}
	d.screenshotMu.Unlock()
	if !ok {
		return false
	}

	select {
	case pending <- outcome:
	default:
	}
	return true
}

// handleDesktopScreenshotCapture captures the main window natively for the
// titlebar button. A non-2xx response asks the webview to fall back to its DOM
// capture path.
func (d *DesktopService) handleDesktopScreenshotCapture(c *gin.Context) {
	var request struct {
		Scale float64 `json:"scale"`
	}
	// An empty body is valid; the default is a high-resolution button capture.
	_ = c.ShouldBindJSON(&request)

	scale := request.Scale
	if scale <= 0 || scale > 2 {
		scale = 2
	}

	window := d.getWindow()
	if window == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "桌面客户端窗口尚未就绪",
			"data":    gin.H{"fallback": "dom"},
		})
		return
	}

	captured, err := captureNativeScreenshot(window, scale)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errNativeScreenshotUnsupported) {
			status = http.StatusNotImplemented
		}
		log.Printf("desktop screenshot: native capture failed (status %d): %v", status, err)
		c.JSON(status, gin.H{
			"success": false,
			"error":   err.Error(),
			"data":    gin.H{"fallback": "dom"},
		})
		return
	}
	if len(captured.PNG) > desktopScreenshotMaxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"success": false,
			"error":   "screenshot PNG is too large",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"dataUrl":    desktopScreenshotDataURLPrefix + base64.StdEncoding.EncodeToString(captured.PNG),
			"width":      captured.Width,
			"height":     captured.Height,
			"scale":      captured.Scale,
			"capturedAt": time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// notifyDesktopScreenshotTaken tells the webview that an MCP client captured
// the window natively, so the user sees the same notice as the fallback path.
func (d *DesktopService) notifyDesktopScreenshotTaken() {
	if window := d.getWindow(); window != nil {
		window.ExecJS(`window.dispatchEvent(new CustomEvent("hephaestus:desktop-screenshot-taken"));`)
	}
}

func (d *DesktopService) handleDesktopScreenshotComplete(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, desktopScreenshotMaxBodyBytes)

	var req struct {
		RequestID string `json:"requestId"`
		DataURL   string `json:"dataUrl"`
		Error     string `json:"error"`
		Route     string `json:"route"`
		Title     string `json:"title"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid payload"})
		return
	}

	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing requestId"})
		return
	}

	if message := strings.TrimSpace(req.Error); message != "" {
		if !d.completeDesktopScreenshot(requestID, desktopScreenshotOutcome{err: errors.New(message)}) {
			c.JSON(http.StatusGone, gin.H{"success": false, "error": "screenshot request expired"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	pngBytes, pixelWidth, pixelHeight, err := decodeDesktopScreenshotDataURL(req.DataURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	width, height := req.Width, req.Height
	if width <= 0 || height <= 0 {
		width, height = pixelWidth, pixelHeight
	}

	screenshot := &mcp.DesktopScreenshot{
		PNG:        pngBytes,
		Width:      width,
		Height:     height,
		Route:      trimDesktopScreenshotText(req.Route, 512),
		Title:      trimDesktopScreenshotText(req.Title, 256),
		CapturedAt: time.Now().UTC(),
	}
	if !d.completeDesktopScreenshot(requestID, desktopScreenshotOutcome{screenshot: screenshot}) {
		c.JSON(http.StatusGone, gin.H{"success": false, "error": "screenshot request expired"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func decodeDesktopScreenshotDataURL(value string) ([]byte, int, int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, 0, 0, errors.New("missing screenshot data")
	}
	if len(value) > desktopScreenshotMaxBodyBytes {
		return nil, 0, 0, errors.New("screenshot data is too large")
	}
	if !strings.HasPrefix(value, desktopScreenshotDataURLPrefix) {
		return nil, 0, 0, errors.New("screenshot must be a PNG data URL")
	}

	raw := strings.TrimSpace(strings.TrimPrefix(value, desktopScreenshotDataURLPrefix))
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, 0, 0, errors.New("screenshot data is not valid base64")
	}
	if len(data) == 0 {
		return nil, 0, 0, errors.New("screenshot data is empty")
	}
	if len(data) > desktopScreenshotMaxBytes {
		return nil, 0, 0, errors.New("screenshot PNG is too large")
	}

	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return nil, 0, 0, errors.New("screenshot is not a valid PNG")
	}
	return data, config.Width, config.Height, nil
}

func trimDesktopScreenshotText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func newDesktopScreenshotRequestID() (string, error) {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
