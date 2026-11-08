package agentruntime

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"hephaestus-backend/pkg/config"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// PopulateNodeMetrics samples the host shared by embedded and standalone Workers.
func PopulateNodeMetrics(opt *NodeUpsertOption) {
	if opt == nil {
		return
	}

	opt.LastSeenIP = DetectLocalIPv4()

	if values, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(values) > 0 {
		opt.CPUUsagePercent = values[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
		opt.MemoryUsageBytes = vm.Used
		opt.MemoryTotalBytes = vm.Total
		opt.MemoryUsagePercent = vm.UsedPercent
	}
	if usage := getAgentDiskUsage(); usage != nil {
		opt.DiskUsageBytes = usage.Used
		opt.DiskTotalBytes = usage.Total
		opt.DiskUsagePercent = usage.UsedPercent
	}
}

func getAgentDiskUsage() *disk.UsageStat {
	candidatePaths := []string{
		config.GetConfigResourceRoot(),
		".",
		"/",
	}
	if runtime.GOOS == "windows" {
		candidatePaths = candidatePaths[:0]
		if wd, err := os.Getwd(); err == nil {
			if volume := filepath.VolumeName(wd); volume != "" {
				candidatePaths = append(candidatePaths, volume+"\\")
			}
		}
		candidatePaths = append(candidatePaths, "C:\\")
	}

	seen := make(map[string]struct{}, len(candidatePaths))
	for _, path := range candidatePaths {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		usage, err := disk.Usage(path)
		if err == nil && usage != nil {
			return usage
		}
	}

	parts, err := disk.Partitions(false)
	if err != nil {
		return nil
	}
	for _, part := range parts {
		usage, usageErr := disk.Usage(part.Mountpoint)
		if usageErr == nil && usage != nil {
			return usage
		}
	}
	return nil
}

// DetectLocalIPv4 reports an address of an active, non-loopback host interface.
func DetectLocalIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			return ip.String()
		}
	}
	return ""
}
