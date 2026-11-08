package machineidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"runtime"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/config"

	"github.com/shirou/gopsutil/v3/host"
)

// MachineInfo provides a stable local machine code for existing encrypted credentials.
type MachineInfo struct {
	MachineCode     string   `json:"machineCode"`     // Stable machine code (SHA256 of selected stable identifiers)
	CodeSource      string   `json:"codeSource"`      // host_id | mac | hostname | persisted
	Hostname        string   `json:"hostname"`        // Best-effort
	OS              string   `json:"os"`              // runtime.GOOS
	Arch            string   `json:"arch"`            // runtime.GOARCH
	Platform        string   `json:"platform"`        // gopsutil platform (best-effort)
	PlatformVersion string   `json:"platformVersion"` // gopsutil platform version (best-effort)
	HostID          string   `json:"hostId"`          // gopsutil HostID (best-effort)
	MACs            []string `json:"macs"`            // Sorted MAC addresses (best-effort)
	GeneratedAt     string   `json:"generatedAt"`     // RFC3339 timestamp
}

func GetMachineInfo() MachineInfo {
	info := MachineInfo{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	if hostInfo, err := host.Info(); err == nil && hostInfo != nil {
		info.Hostname = hostInfo.Hostname
		info.Platform = hostInfo.Platform
		info.PlatformVersion = hostInfo.PlatformVersion
		info.HostID = hostInfo.HostID
	}

	macs := make([]string, 0, 8)
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if len(iface.HardwareAddr) == 0 {
				continue
			}
			mac := strings.ToLower(iface.HardwareAddr.String())
			if mac == "" || mac == "00:00:00:00:00:00" {
				continue
			}
			macs = append(macs, mac)
		}
	}
	sort.Strings(macs)
	info.MACs = macs

	// Pick the most stable identifier available.
	var binding string
	switch {
	case strings.TrimSpace(info.HostID) != "":
		info.CodeSource = "host_id"
		binding = strings.TrimSpace(info.HostID)
	case len(info.MACs) > 0:
		info.CodeSource = "mac"
		binding = strings.Join(info.MACs, ",")
	default:
		info.CodeSource = "hostname"
		binding = strings.TrimSpace(info.Hostname)
	}

	if binding != "" {
		sum := sha256.Sum256([]byte(binding))
		info.MachineCode = hex.EncodeToString(sum[:])
		stabilizeMachineInfo(&info, config.GetDataDir())
	}

	return info
}
