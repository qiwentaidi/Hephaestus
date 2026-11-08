//go:build darwin

package main

import (
	"os/exec"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func handleDesktopTitlebarDoubleClick(window *application.WebviewWindow) {
	if window == nil {
		return
	}

	switch macTitlebarDoubleClickAction() {
	case "minimize":
		window.Minimise()
	case "none":
		return
	default:
		if window.IsMaximised() {
			window.UnMaximise()
			return
		}
		window.Maximise()
	}
}

func macTitlebarDoubleClickAction() string {
	if action, ok := readMacDefaultsString("AppleActionOnDoubleClick"); ok {
		switch strings.ToLower(action) {
		case "minimize":
			return "minimize"
		case "maximize", "fill", "zoom":
			return "maximize"
		case "none":
			return "none"
		}
	}

	if legacyValue, ok := readMacDefaultsString("AppleMiniaturizeOnDoubleClick"); ok {
		switch strings.ToLower(legacyValue) {
		case "1", "true", "yes":
			return "minimize"
		case "0", "false", "no":
			return "maximize"
		}
	}

	return "maximize"
}

func readMacDefaultsString(key string) (string, bool) {
	output, err := exec.Command("defaults", "read", "-g", key).Output()
	if err != nil {
		return "", false
	}

	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", false
	}

	return value, true
}
