//go:build windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	printWindowRenderFullContent = 2
	biRGB                        = 0
	nativeScreenshotTimeout      = 8 * time.Second
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	procGetWindowRect          = user32.NewProc("GetWindowRect")
	procGetDC                  = user32.NewProc("GetDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procPrintWindow            = user32.NewProc("PrintWindow")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
)

type winRect struct {
	left   int32
	top    int32
	right  int32
	bottom int32
}

type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

type bitmapInfo struct {
	header bitmapInfoHeader
	colors [1]uint32
}

// captureNativeScreenshot renders the main window (including the WebView2
// child) with PrintWindow. Unlike a screen grab, it also works when the window
// is covered by other windows. A blank result is reported as an error so the
// caller can fall back to DOM capture.
func captureNativeScreenshot(window *application.WebviewWindow, scale float64) (*nativeScreenshotResult, error) {
	if window == nil {
		return nil, errors.New("desktop window is unavailable")
	}
	handle := window.NativeWindow()
	if handle == nil {
		return nil, errors.New("native window handle is unavailable")
	}

	type outcome struct {
		png []byte
		err error
	}
	result := make(chan outcome, 1)
	go func() {
		data, err := captureWindowPNG(uintptr(handle))
		result <- outcome{png: data, err: err}
	}()

	var captured outcome
	select {
	case captured = <-result:
	case <-time.After(nativeScreenshotTimeout):
		return nil, errors.New("native screenshot timed out")
	}
	if captured.err != nil {
		return nil, captured.err
	}

	logicalWidth, logicalHeight := window.Size()
	prepared, err := prepareNativeScreenshot(captured.png, logicalWidth, logicalHeight, scale)
	if err != nil {
		return nil, fmt.Errorf("prepare native screenshot: %w", err)
	}
	return prepared, nil
}

func captureWindowPNG(hwnd uintptr) ([]byte, error) {
	var rect winRect
	if ok, _, err := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok == 0 {
		return nil, fmt.Errorf("GetWindowRect failed: %w", err)
	}
	width := int(rect.right - rect.left)
	height := int(rect.bottom - rect.top)
	if width <= 0 || height <= 0 {
		return nil, errors.New("window has no drawable area")
	}
	if width*height > 80_000_000 {
		return nil, errors.New("window is too large to capture")
	}

	windowDC, _, err := procGetDC.Call(hwnd)
	if windowDC == 0 {
		return nil, fmt.Errorf("GetDC failed: %w", err)
	}
	defer procReleaseDC.Call(hwnd, windowDC)

	memoryDC, _, err := procCreateCompatibleDC.Call(windowDC)
	if memoryDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed: %w", err)
	}
	defer procDeleteDC.Call(memoryDC)

	bitmap, _, err := procCreateCompatibleBitmap.Call(windowDC, uintptr(width), uintptr(height))
	if bitmap == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed: %w", err)
	}
	defer procDeleteObject.Call(bitmap)

	previous, _, _ := procSelectObject.Call(memoryDC, bitmap)
	if previous == 0 {
		return nil, errors.New("SelectObject failed")
	}

	// PW_RENDERFULLCONTENT is required for DirectComposition/WebView2 content.
	printed, _, _ := procPrintWindow.Call(hwnd, memoryDC, printWindowRenderFullContent)
	if printed == 0 {
		printed, _, _ = procPrintWindow.Call(hwnd, memoryDC, 0)
	}
	procSelectObject.Call(memoryDC, previous)
	if printed == 0 {
		return nil, errors.New("PrintWindow failed")
	}

	var info bitmapInfo
	info.header.size = uint32(unsafe.Sizeof(info.header))
	info.header.width = int32(width)
	info.header.height = -int32(height) // top-down
	info.header.planes = 1
	info.header.bitCount = 32
	info.header.compression = biRGB

	pixels := make([]byte, width*height*4)
	lines, _, err := procGetDIBits.Call(
		memoryDC,
		bitmap,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&info)),
		0,
	)
	if lines == 0 {
		return nil, fmt.Errorf("GetDIBits failed: %w", err)
	}

	imageData := image.NewNRGBA(image.Rect(0, 0, width, height))
	nonBlack := 0
	for y := 0; y < height; y++ {
		sourceRow := y * width * 4
		targetRow := y * imageData.Stride
		for x := 0; x < width; x++ {
			source := sourceRow + x*4
			target := targetRow + x*4
			blue, green, red := pixels[source], pixels[source+1], pixels[source+2]
			imageData.Pix[target] = red
			imageData.Pix[target+1] = green
			imageData.Pix[target+2] = blue
			imageData.Pix[target+3] = 0xff
			if red > 8 || green > 8 || blue > 8 {
				nonBlack++
			}
		}
	}
	if nonBlack*100 < width*height {
		return nil, errors.New("PrintWindow returned a blank image")
	}

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, imageData); err != nil {
		return nil, fmt.Errorf("encode window capture: %w", err)
	}
	return buffer.Bytes(), nil
}
