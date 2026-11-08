package main

import (
	"math"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type desktopWindowMetrics struct {
	width     int
	height    int
	minWidth  int
	minHeight int
}

func computeDesktopWindowMetrics(workAreaWidth int, workAreaHeight int) desktopWindowMetrics {
	const (
		idealWidth    = 1440
		idealHeight   = 920
		baseMinWidth  = 1100
		baseMinHeight = 720
		windowMargin  = 32
	)

	if workAreaWidth <= 0 || workAreaHeight <= 0 {
		return desktopWindowMetrics{
			width:     idealWidth,
			height:    idealHeight,
			minWidth:  baseMinWidth,
			minHeight: baseMinHeight,
		}
	}

	// Keep the entire native window inside the screen work area. The previous
	// lower bounds (900x620) could make the window larger than a low-resolution
	// work area, which caused centered windows to overflow on the left and top.
	maxSafeWidth := maxInt(1, workAreaWidth-windowMargin)
	maxSafeHeight := maxInt(1, workAreaHeight-windowMargin)

	candidateWidth := minInt(idealWidth, int(math.Round(float64(workAreaWidth)*0.92)))
	candidateHeight := minInt(idealHeight, int(math.Round(float64(workAreaHeight)*0.9)))

	if workAreaWidth <= 1280 || workAreaHeight <= 800 {
		candidateWidth = maxSafeWidth
		candidateHeight = maxSafeHeight
	}

	width := minInt(candidateWidth, maxSafeWidth)
	height := minInt(candidateHeight, maxSafeHeight)

	return desktopWindowMetrics{
		width:     width,
		height:    height,
		minWidth:  minInt(baseMinWidth, width),
		minHeight: minInt(baseMinHeight, height),
	}
}

func resolveDesktopWindowBounds(app *application.App) (width int, height int, minWidth int, minHeight int, screen *application.Screen) {
	screen = app.Screen.GetPrimary()
	if screen == nil {
		metrics := computeDesktopWindowMetrics(0, 0)
		return metrics.width, metrics.height, metrics.minWidth, metrics.minHeight, nil
	}

	metrics := computeDesktopWindowMetrics(screen.WorkArea.Width, screen.WorkArea.Height)
	return metrics.width, metrics.height, metrics.minWidth, metrics.minHeight, screen
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
