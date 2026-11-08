//go:build !darwin

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func handleDesktopTitlebarDoubleClick(window *application.WebviewWindow) {
	if window == nil {
		return
	}

	if window.IsMaximised() {
		window.UnMaximise()
		return
	}

	window.Maximise()
}
