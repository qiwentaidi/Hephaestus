package portscan

import (
	"context"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"hephaestus-backend/pkg/types"

	gonmap "github.com/qiwentaidi/gonmap/lib"
)

// TestLiveSYNConnectComparison is opt-in because it scans real network targets.
// Example: PORTSCAN_LIVE_TARGETS=192.0.2.1,198.51.100.1 go test ./pkg/portscan -run TestLiveSYNConnectComparison -v
func TestLiveSYNConnectComparison(t *testing.T) {
	targets := strings.Split(strings.TrimSpace(os.Getenv("PORTSCAN_LIVE_TARGETS")), ",")
	if len(targets) == 1 && targets[0] == "" {
		t.Skip("set PORTSCAN_LIVE_TARGETS to authorized comma-separated IPs")
	}
	attempts := defaultSYNDiscoveryAttempts
	if raw := os.Getenv("PORTSCAN_LIVE_SYN_ATTEMPTS"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > defaultSYNDiscoveryAttempts {
			t.Fatalf("PORTSCAN_LIVE_SYN_ATTEMPTS must be 1 or %d", defaultSYNDiscoveryAttempts)
		}
		attempts = parsed
	}

	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		t.Run(target, func(t *testing.T) {
			results := make(map[gonmap.PortDiscoveryMode][]int)
			for _, mode := range []gonmap.PortDiscoveryMode{gonmap.PortDiscoveryModeConnect, gonmap.PortDiscoveryModeSYN} {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				addresses := make(chan Address, 1000)
				for port := 1; port <= 1000; port++ {
					addresses <- Address{IP: target, Port: port, Protocol: gonmap.ScanProtocolTCP}
				}
				close(addresses)

				var mu sync.Mutex
				var openPorts []int
				start := time.Now()
				err := TcpScanWithDiscovery(ctx, "live-comparison", addresses, 200, 1, "", 0, TCPDiscoveryConfig{Mode: mode, Attempts: attempts}, func(result *types.PortInfo) {
					mu.Lock()
					openPorts = append(openPorts, result.Port)
					mu.Unlock()
				}, nil)
				elapsed := time.Since(start)
				cancel()
				if err != nil {
					t.Fatalf("%s scan failed: %v", mode, err)
				}
				if ctx.Err() == context.DeadlineExceeded {
					t.Fatalf("%s scan timed out", mode)
				}
				sort.Ints(openPorts)
				results[mode] = openPorts
				t.Logf("mode=%s elapsed=%s open=%v", mode, elapsed.Round(time.Millisecond), openPorts)
			}
			if !reflect.DeepEqual(results[gonmap.PortDiscoveryModeConnect], results[gonmap.PortDiscoveryModeSYN]) {
				t.Errorf("Connect and SYN open ports differ: connect=%v syn=%v", results[gonmap.PortDiscoveryModeConnect], results[gonmap.PortDiscoveryModeSYN])
			}
		})
	}
}
