package runtimeenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type BrowserRuntimeInfo struct {
	OS                 string `json:"os"`
	IsLinux            bool   `json:"isLinux"`
	ChromePath         string `json:"chromePath"`
	SnapChromeDetected bool   `json:"snapChromeDetected"`
}

func DetectBrowserRuntime() BrowserRuntimeInfo {
	chromePath, snapChromeDetected := DetectSnapChromeExecutable()
	return BrowserRuntimeInfo{
		OS:                 runtime.GOOS,
		IsLinux:            runtime.GOOS == "linux",
		ChromePath:         chromePath,
		SnapChromeDetected: runtime.GOOS == "linux" && snapChromeDetected,
	}
}

func DetectSnapChromeExecutable() (string, bool) {
	if runtime.GOOS != "linux" {
		return "", false
	}

	candidates := []string{
		os.Getenv("CHROME_PATH"),
		os.Getenv("CHROMEDP_BROWSER_EXECUTABLE"),
	}

	for _, name := range []string{"google-chrome", "chrome", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, path)
		}
	}

	for _, candidate := range candidates {
		path := strings.TrimSpace(candidate)
		if path == "" {
			continue
		}
		if IsSnapChromePath(path) {
			return path, true
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil && IsSnapChromePath(resolved) {
			return resolved, true
		}
	}

	return "", false
}

func IsSnapChromePath(path string) bool {
	cleaned := filepath.Clean(path)
	return cleaned == "/snap" || strings.HasPrefix(cleaned, "/snap/")
}
