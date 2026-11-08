//go:build windows

package synpriv

import (
	"os"
	"path/filepath"
)

// hasWindowsNpcap checks the DLLs installed by Npcap. Nmap can be installed
// without Npcap, so checking for nmap.exe would not tell us whether the
// platform's packet-based SYN scanner can work.
func hasWindowsNpcap() bool {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = os.Getenv("windir")
	}
	if systemRoot == "" {
		return false
	}

	candidateDirs := []string{
		filepath.Join(systemRoot, "System32", "Npcap"),
		filepath.Join(systemRoot, "SysWOW64", "Npcap"),
		filepath.Join(systemRoot, "System32"),
		filepath.Join(systemRoot, "SysWOW64"),
	}
	for _, dir := range candidateDirs {
		if fileExists(filepath.Join(dir, "wpcap.dll")) && fileExists(filepath.Join(dir, "Packet.dll")) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
