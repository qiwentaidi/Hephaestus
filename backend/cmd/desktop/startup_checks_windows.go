//go:build windows

package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"syscall"
	"unsafe"

	"github.com/wailsapp/go-webview2/webviewloader"
)

var errMissingWebView2Runtime = errors.New("microsoft edge webview2 runtime is required")

const (
	webView2DownloadURL = "https://developer.microsoft.com/en-us/microsoft-edge/webview2/"

	mbOK        = 0x00000000
	mbYesNo     = 0x00000004
	mbIconError = 0x00000010

	idYes = 6

	swShowNormal = 1
)

func ensureDesktopRuntimePrerequisites() error {
	version, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	if err != nil {
		return fmt.Errorf("failed to detect Microsoft Edge WebView2 Runtime: %w", err)
	}
	if strings.TrimSpace(version) == "" {
		return errMissingWebView2Runtime
	}
	return nil
}

func reportDesktopStartupFailure(err error) {
	if err == nil {
		return
	}

	log.Printf("desktop startup failed: %v", err)

	if isMissingWebView2RuntimeError(err) {
		showMissingWebView2RuntimeDialog()
		return
	}
	if message, ok := desktopPostgresConnectionFailureMessage(err); ok {
		_ = showWindowsMessageBox("Hephaestus", message, mbOK|mbIconError)
		return
	}

	showDesktopStartupErrorDialog(err)
}

func isMissingWebView2RuntimeError(err error) bool {
	if errors.Is(err, errMissingWebView2Runtime) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "webview2")
}

func showMissingWebView2RuntimeDialog() {
	message := "Hephaestus 依赖 Microsoft Edge WebView2 Runtime 才能启动。\n\n当前系统未检测到 WebView2 Runtime。\n\n点击“是”打开官方下载页面，安装完成后请重新启动客户端。"
	if showWindowsMessageBox("Hephaestus 启动失败", message, mbYesNo|mbIconError) == idYes {
		_ = openWindowsURL(webView2DownloadURL)
	}
}

func showDesktopStartupErrorDialog(err error) {
	message := fmt.Sprintf("Hephaestus 启动失败。\n\n错误信息：%v", err)
	_ = showWindowsMessageBox("Hephaestus 启动失败", message, mbOK|mbIconError)
}

func showWindowsMessageBox(title string, message string, flags uintptr) int {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBoxW := user32.NewProc("MessageBoxW")

	titlePtr, _ := syscall.UTF16PtrFromString(title)
	messagePtr, _ := syscall.UTF16PtrFromString(message)
	result, _, _ := messageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		flags,
	)
	return int(result)
}

func openWindowsURL(target string) error {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecuteW := shell32.NewProc("ShellExecuteW")

	verbPtr, _ := syscall.UTF16PtrFromString("open")
	targetPtr, _ := syscall.UTF16PtrFromString(target)
	result, _, _ := shellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(targetPtr)),
		0,
		0,
		swShowNormal,
	)
	if result <= 32 {
		return fmt.Errorf("failed to open url: %s", target)
	}
	return nil
}
