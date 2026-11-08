//go:build windows

package synpriv

import "strings"

func helperFileStatus(path string) HelperStatus {
	return HelperStatus{Path: path}
}

func sameExecutablePath(left, right string) bool {
	return strings.EqualFold(left, right)
}
