//go:build darwin || linux

package synpriv

import (
	"os"
	"path/filepath"
	"syscall"
)

func helperFileStatus(path string) HelperStatus {
	status := HelperStatus{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		return status
	}
	status.Exists = true
	status.Setuid = info.Mode()&os.ModeSetuid != 0
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		status.RootOwned = stat.Uid == 0
	}
	return status
}

func sameExecutablePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr == nil {
		left = leftAbs
	}
	if rightErr == nil {
		right = rightAbs
	}
	return left == right
}
