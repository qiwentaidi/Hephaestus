//go:build darwin && !ios

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
#include "screenshot_native_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const nativeScreenshotTimeout = 8 * time.Second

// captureNativeScreenshot snapshots the live WKWebView without requiring the
// macOS Screen Recording permission. It must be called from a non-main
// goroutine: the snapshot is started on the main thread and waits on a pipe.
func captureNativeScreenshot(window *application.WebviewWindow, scale float64) (*nativeScreenshotResult, error) {
	if window == nil {
		return nil, errors.New("desktop window is unavailable")
	}
	nativeWindow := window.NativeWindow()
	if nativeWindow == nil {
		return nil, errors.New("native window handle is unavailable")
	}

	snapshot := C.hephaestusSnapshotCreate()
	if snapshot == nil {
		return nil, errors.New("failed to allocate native screenshot request")
	}
	defer C.hephaestusSnapshotRelease(snapshot)

	// WKWebView snapshotting is a main-thread API. Start the async request from
	// the main thread; the completion handler signals the pipe below.
	application.InvokeSync(func() {
		C.hephaestusSnapshotStart(nativeWindow, snapshot)
	})

	if C.hephaestusSnapshotWait(snapshot, C.int(nativeScreenshotTimeout/time.Millisecond)) == 0 {
		return nil, errors.New("native screenshot timed out")
	}

	length := C.hephaestusSnapshotLength(snapshot)
	if length == 0 {
		return nil, errors.New("native screenshot returned no image")
	}
	data := C.GoBytes(C.hephaestusSnapshotData(snapshot), C.int(length))

	logicalWidth := int(C.hephaestusSnapshotWidth(snapshot))
	logicalHeight := int(C.hephaestusSnapshotHeight(snapshot))
	if logicalWidth <= 0 || logicalHeight <= 0 {
		logicalWidth, logicalHeight = window.Size()
	}
	result, err := prepareNativeScreenshot(data, logicalWidth, logicalHeight, scale)
	if err != nil {
		return nil, fmt.Errorf("prepare native screenshot: %w", err)
	}
	return result, nil
}
