//go:build !windows

package synpriv

func hasWindowsNpcap() bool {
	return false
}
