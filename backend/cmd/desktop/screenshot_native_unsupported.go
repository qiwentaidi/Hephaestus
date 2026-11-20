//go:build (!darwin && !windows) || ios

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// captureNativeScreenshot is unavailable outside the macOS/Windows desktop
// hosts. Callers fall back to DOM capture.
func captureNativeScreenshot(_ *application.WebviewWindow, _ float64) (*nativeScreenshotResult, error) {
	return nil, errNativeScreenshotUnsupported
}
