package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"

	"golang.org/x/image/draw"
)

// errNativeScreenshotUnsupported is returned on platforms where the window's
// webview cannot be captured natively. Callers fall back to DOM capture.
var errNativeScreenshotUnsupported = errors.New("native screenshot is not supported on this platform")

type nativeScreenshotResult struct {
	PNG    []byte
	Width  int     // logical (CSS pixel) width
	Height int     // logical (CSS pixel) height
	Scale  float64 // physical pixels per logical pixel
}

// prepareNativeScreenshot normalizes a freshly captured PNG for the requested
// scale. A scale < 1.5 is treated as 1x, which keeps MCP responses small.
func prepareNativeScreenshot(pngData []byte, logicalWidth, logicalHeight int, scale float64) (*nativeScreenshotResult, error) {
	if len(pngData) == 0 {
		return nil, errors.New("native screenshot is empty")
	}

	config, err := png.DecodeConfig(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("native screenshot is not a valid PNG: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return nil, errors.New("native screenshot has invalid dimensions")
	}

	width, height := config.Width, config.Height
	if logicalWidth <= 0 || logicalHeight <= 0 {
		logicalWidth, logicalHeight = width, height
	}

	actualScale := float64(width) / float64(logicalWidth)
	if scale < 1.5 && width > logicalWidth {
		resized, err := resizeScreenshotPNG(pngData, logicalWidth)
		if err != nil {
			return nil, err
		}
		height = int(math.Round(float64(height) * float64(logicalWidth) / float64(width)))
		width = logicalWidth
		actualScale = 1
		pngData = resized
	}

	return &nativeScreenshotResult{
		PNG:    pngData,
		Width:  logicalWidth,
		Height: logicalHeight,
		Scale:  actualScale,
	}, nil
}

func resizeScreenshotPNG(data []byte, width int) ([]byte, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}
	bounds := source.Bounds()
	if width <= 0 || width >= bounds.Dx() {
		return data, nil
	}

	height := int(math.Round(float64(bounds.Dy()) * float64(width) / float64(bounds.Dx())))
	if height <= 0 {
		return nil, errors.New("invalid screenshot resize dimensions")
	}

	target := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Over, nil)

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, target); err != nil {
		return nil, fmt.Errorf("encode resized screenshot: %w", err)
	}
	return buffer.Bytes(), nil
}
