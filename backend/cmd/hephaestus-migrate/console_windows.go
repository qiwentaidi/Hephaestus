//go:build windows

package main

import "syscall"

// setupConsole switches the Windows console to UTF-8 so the Chinese status
// messages render correctly in cmd.exe/PowerShell.
func setupConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	_, _, _ = kernel32.NewProc("SetConsoleOutputCP").Call(65001)
	_, _, _ = kernel32.NewProc("SetConsoleCP").Call(65001)
}
