package portscan

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"hephaestus-backend/pkg/synpriv"
	"hephaestus-backend/pkg/taskstate"

	gonmap "github.com/qiwentaidi/gonmap/lib"
)

func TestSYNDiscoveryWithoutAuthorizationFailsWithoutConnectFallback(t *testing.T) {
	hosts := []string{"127.0.0.1"}
	ports := []int{80}
	result, err := discoverOpenPortsForBatch(context.Background(), gonmap.New(nil), hosts, ports,
		gonmap.PortDiscoveryOptions{Mode: gonmap.PortDiscoveryModeSYN},
		synDiscoveryRuntime{helperStatus: synpriv.HelperStatus{Path: "/missing/syn-helper"}}, 1, nil, nil)
	if err == nil {
		t.Fatal("expected missing SYN authorization to fail")
	}
	if result != nil {
		t.Fatalf("expected no fallback results, got %#v", result)
	}
	if !strings.Contains(err.Error(), "重新授权 SYN") {
		t.Fatalf("expected actionable authorization error, got %v", err)
	}
}

func TestSYNHelperFailureDoesNotFallBackToConnect(t *testing.T) {
	result, err := discoverOpenPortsForBatch(context.Background(), gonmap.New(nil), []string{"127.0.0.1"}, []int{80},
		gonmap.PortDiscoveryOptions{Mode: gonmap.PortDiscoveryModeSYN},
		synDiscoveryRuntime{useHelper: true, helperStatus: synpriv.HelperStatus{Usable: true, Path: "/missing/syn-helper"}}, 1, nil, nil)
	if err == nil || result != nil {
		t.Fatalf("expected helper failure without Connect results, got result=%#v err=%v", result, err)
	}
	if !strings.Contains(err.Error(), "SYN 授权辅助程序执行失败") {
		t.Fatalf("expected visible helper error, got %v", err)
	}
}

func TestTrimAddressGroupsForResume(t *testing.T) {
	groups := []addressGroup{
		{
			IP:       "1.1.1.1",
			Protocol: gonmap.ScanProtocolTCP,
			Ports:    []int{80, 81, 82},
		},
		{
			IP:       "2.2.2.2",
			Protocol: gonmap.ScanProtocolTCP,
			Ports:    []int{443, 444},
		},
	}

	trimmed := trimAddressGroupsForResume(groups, 4)
	if len(trimmed) != 1 {
		t.Fatalf("expected 1 remaining group, got %#v", trimmed)
	}
	if trimmed[0].IP != "2.2.2.2" {
		t.Fatalf("expected remaining group for second host, got %#v", trimmed[0])
	}
	if !reflect.DeepEqual(trimmed[0].Ports, []int{444}) {
		t.Fatalf("expected remaining ports [444], got %#v", trimmed[0].Ports)
	}
}

func TestBatchPortResultToPortInfoFiltersNoise(t *testing.T) {
	if result := batchPortResultToPortInfo("1.1.1.1", gonmap.ScanProtocolTCP, gonmap.BatchPortResult{
		Port:       20000,
		Status:     gonmap.Open,
		Suspicious: true,
	}, true); result != nil {
		t.Fatalf("expected suspicious port to be dropped on noisy host, got %#v", result)
	}

	if result := batchPortResultToPortInfo("1.1.1.1", gonmap.ScanProtocolTCP, gonmap.BatchPortResult{
		Port:          30000,
		Status:        gonmap.Matched,
		LikelyWrapped: true,
		Response: &gonmap.Response{
			FingerPrint: &gonmap.FingerPrint{Service: "http-proxy"},
		},
	}, true); result != nil {
		t.Fatalf("expected wrapped port to be dropped on noisy host, got %#v", result)
	}

	if result := batchPortResultToPortInfo("1.1.1.1", gonmap.ScanProtocolTCP, gonmap.BatchPortResult{
		Port:          4001,
		Status:        gonmap.Matched,
		LikelyWrapped: true,
		Response: &gonmap.Response{
			FingerPrint: &gonmap.FingerPrint{Service: "ssl"},
		},
	}, false); result == nil {
		t.Fatal("expected wrapped port to be preserved on normal host")
	}

	result := batchPortResultToPortInfo("1.1.1.1", gonmap.ScanProtocolTCP, gonmap.BatchPortResult{
		Port:   443,
		Status: gonmap.Matched,
		Response: &gonmap.Response{
			FingerPrint: &gonmap.FingerPrint{Service: "https", ProductName: "nginx", Version: "1.25.5"},
			Raw:         "HTTP/1.1 200 OK",
		},
	}, false)
	if result == nil {
		t.Fatal("expected stable port to be preserved")
	}
	if result.Port != 443 || result.Protocol != "https" {
		t.Fatalf("unexpected stable result %#v", result)
	}
	if result.Product != "nginx" || result.Version != "1.25.5" {
		t.Fatalf("expected product/version to be preserved, got %#v", result)
	}
}

func TestDefaultTCPDiscoveryConfigUsesConnect(t *testing.T) {
	config := defaultTCPDiscoveryConfig().normalized()
	if config.Mode != gonmap.PortDiscoveryModeConnect {
		t.Fatalf("expected default tcp discovery mode connect, got %q", config.Mode)
	}
}

func TestParseTCPDiscoveryConfigSupportsConnectOverride(t *testing.T) {
	config := parseTCPDiscoveryConfig(map[string]interface{}{
		"scanMode": "connect",
	})
	if config.Mode != gonmap.PortDiscoveryModeConnect {
		t.Fatalf("expected connect discovery mode, got %q", config.Mode)
	}
}

func TestParseTCPDiscoveryConfigSupportsSYNOptions(t *testing.T) {
	config := parseTCPDiscoveryConfig(map[string]interface{}{
		"scanMode":             "syn",
		"synRate":              float64(3000),
		"synDiscoveryAttempts": float64(1),
	})
	if config.Mode != gonmap.PortDiscoveryModeSYN {
		t.Fatalf("expected syn discovery mode, got %q", config.Mode)
	}
	if config.Rate != 3000 {
		t.Fatalf("expected syn rate 3000, got %d", config.Rate)
	}
	if config.Attempts != 1 {
		t.Fatalf("expected one SYN attempt, got %d", config.Attempts)
	}
}

func TestSplitDiscoveryWorkersLimitsSYNGroupConcurrency(t *testing.T) {
	groupWorkers, batchWorkers := splitDiscoveryWorkers(600, 100, TCPDiscoveryConfig{
		Mode: gonmap.PortDiscoveryModeSYN,
	})
	if groupWorkers != maxConcurrentSYNDiscoveryGroups {
		t.Fatalf("expected syn group workers %d, got %d", maxConcurrentSYNDiscoveryGroups, groupWorkers)
	}
	if batchWorkers != 150 {
		t.Fatalf("expected syn batch workers 150, got %d", batchWorkers)
	}

	connectGroupWorkers, _ := splitDiscoveryWorkers(600, 100, TCPDiscoveryConfig{
		Mode: gonmap.PortDiscoveryModeConnect,
	})
	if connectGroupWorkers != 100 {
		t.Fatalf("expected connect group workers to remain 100, got %d", connectGroupWorkers)
	}
}

func TestBuildPortDiscoveryBatchesCoalescesSamePortChunks(t *testing.T) {
	groups := []addressGroup{
		{
			IP:       "10.0.0.1",
			Protocol: gonmap.ScanProtocolTCP,
			Ports:    []int{80, 81, 82, 83},
		},
		{
			IP:       "10.0.0.2",
			Protocol: gonmap.ScanProtocolTCP,
			Ports:    []int{80, 81, 82, 83},
		},
	}

	batches := buildPortDiscoveryBatches(groups, 2)
	if len(batches) != 2 {
		t.Fatalf("expected 2 port batches, got %#v", batches)
	}
	if !reflect.DeepEqual(batches[0].Hosts, []string{"10.0.0.1", "10.0.0.2"}) {
		t.Fatalf("expected first chunk to include both hosts, got %#v", batches[0].Hosts)
	}
	if !reflect.DeepEqual(batches[0].Ports, []int{80, 81}) || batches[0].Total != 4 {
		t.Fatalf("unexpected first chunk %#v", batches[0])
	}
	if !reflect.DeepEqual(batches[1].Hosts, []string{"10.0.0.1", "10.0.0.2"}) {
		t.Fatalf("expected second chunk to include both hosts, got %#v", batches[1].Hosts)
	}
	if !reflect.DeepEqual(batches[1].Ports, []int{82, 83}) || batches[1].Total != 4 {
		t.Fatalf("unexpected second chunk %#v", batches[1])
	}
}

func TestGroupOpenPortsByHostFiltersUnknownHostsAndSortsPorts(t *testing.T) {
	grouped := groupOpenPortsByHost([]string{"10.0.0.1", "10.0.0.2"}, []gonmap.OpenPort{
		{Host: "10.0.0.1", Port: 443},
		{Host: "10.0.0.3", Port: 22},
		{Host: "10.0.0.1", Port: 80},
		{Host: "10.0.0.2", Port: 8080},
	})

	if !reflect.DeepEqual(grouped["10.0.0.1"], []int{80, 443}) {
		t.Fatalf("expected sorted ports for first host, got %#v", grouped["10.0.0.1"])
	}
	if !reflect.DeepEqual(grouped["10.0.0.2"], []int{8080}) {
		t.Fatalf("expected second host port, got %#v", grouped["10.0.0.2"])
	}
	if _, ok := grouped["10.0.0.3"]; ok {
		t.Fatalf("expected unknown host to be filtered, got %#v", grouped)
	}
}

func TestMergeOpenPortsDeduplicatesAndSorts(t *testing.T) {
	merged := mergeOpenPorts(
		[]gonmap.OpenPort{
			{Host: "10.0.0.2", Port: 443},
			{Host: "10.0.0.1", Port: 8080},
		},
		[]gonmap.OpenPort{
			{Host: "10.0.0.1", Port: 80},
			{Host: "10.0.0.2", Port: 443},
		},
	)

	expected := []gonmap.OpenPort{
		{Host: "10.0.0.1", Port: 80},
		{Host: "10.0.0.1", Port: 8080},
		{Host: "10.0.0.2", Port: 443},
	}
	if !reflect.DeepEqual(merged, expected) {
		t.Fatalf("unexpected merged ports: %#v", merged)
	}
}

func TestPortIsOpenForHost(t *testing.T) {
	openPortsByHost := map[string][]int{
		"10.0.0.1": {80, 443},
	}
	if !portIsOpenForHost(openPortsByHost, "10.0.0.1", 443) {
		t.Fatal("expected 443 to be open")
	}
	if portIsOpenForHost(openPortsByHost, "10.0.0.1", 22) {
		t.Fatal("expected 22 to be closed")
	}
	if portIsOpenForHost(openPortsByHost, "10.0.0.2", 443) {
		t.Fatal("expected unknown host to be closed")
	}
}

func TestVerifiedSYNOpenFallsBackToOpenResultWhenFingerprintCloses(t *testing.T) {
	openPortsByHost := map[string][]int{
		"10.0.0.1": {8080},
	}
	item := gonmap.BatchPortResult{Port: 8080, Status: gonmap.Closed}

	result := batchPortResultToPortInfoWithVerifiedOpen("10.0.0.1", gonmap.ScanProtocolTCP, item, openPortsByHost, nil, false)
	if result == nil {
		t.Fatal("expected verified open candidate to be preserved")
	}
	if result.Port != 8080 || result.Protocol == "" {
		t.Fatalf("unexpected fallback result %#v", result)
	}
}

func TestVerifiedSYNOpenDoesNotPreserveSuspiciousResult(t *testing.T) {
	openPortsByHost := map[string][]int{
		"10.0.0.1": {20000},
	}
	result := batchPortResultToPortInfoWithVerifiedOpen("10.0.0.1", gonmap.ScanProtocolTCP, gonmap.BatchPortResult{
		Port:       20000,
		Status:     gonmap.Open,
		Suspicious: true,
	}, openPortsByHost, nil, true)

	if result != nil {
		t.Fatalf("expected suspicious result to remain filtered, got %#v", result)
	}
}

func TestVerifySYNFingerprintCandidatesRetriesOnlyUncertainResults(t *testing.T) {
	openListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer openListener.Close()
	closedListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	openPort := openListener.Addr().(*net.TCPAddr).Port
	closedPort := closedListener.Addr().(*net.TCPAddr).Port
	if err := closedListener.Close(); err != nil {
		t.Fatal(err)
	}

	host := "127.0.0.1"
	results := map[string][]gonmap.BatchPortResult{
		host: {
			{Port: openPort, Status: gonmap.Closed},
			{Port: closedPort, Status: gonmap.Unknown},
		},
	}
	verified, timedOut, filtered, _, fallbackChecks := verifySYNFingerprintCandidates(context.Background(), []string{host}, results, gonmap.PortDiscoveryOptions{
		Timeout:     250 * time.Millisecond,
		Concurrency: 2,
	})
	if fallbackChecks != 2 {
		t.Fatalf("expected two fallback checks, got %d", fallbackChecks)
	}
	if !reflect.DeepEqual(verified[host], []int{openPort}) {
		t.Fatalf("expected only listening port %d to be verified, got %v", openPort, verified[host])
	}
	if len(timedOut[host]) != 0 {
		t.Fatalf("unexpected connection timeouts: %v", timedOut[host])
	}
	if len(filtered[host]) != 1 || filtered[host][0].Port != openPort {
		t.Fatalf("unexpected filtered results: %#v", filtered[host])
	}
	if result := batchPortResultToPortInfoWithVerifiedOpen(host, gonmap.ScanProtocolTCP, filtered[host][0], verified, timedOut, false); result == nil || result.Port != openPort {
		t.Fatalf("expected fallback to preserve open port, got %#v", result)
	}
}

func TestSYNConnectTimeoutPreservesOpenWithUnknownService(t *testing.T) {
	host := "127.0.0.1"
	result := verifyOpenPortsByConnectWithDial(context.Background(), map[string][]int{host: {80, 81, 82}}, 3,
		func(_ context.Context, _, address string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			switch port {
			case "80":
				return nil, &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}
			case "81":
				return nil, errors.New("connection refused")
			default:
				conn, peer := net.Pipe()
				_ = peer.Close()
				return conn, nil
			}
		})
	if !reflect.DeepEqual(result.timedOut[host], []int{80}) || !reflect.DeepEqual(result.open[host], []int{82}) {
		t.Fatalf("unexpected connect verification: %#v", result)
	}
	portInfo := batchPortResultToPortInfoWithVerifiedOpen(host, gonmap.ScanProtocolTCP,
		gonmap.BatchPortResult{Port: 80, Status: gonmap.Closed}, map[string][]int{host: {80, 82}}, result.timedOut, false)
	if portInfo == nil || portInfo.Port != 80 || portInfo.Protocol != "unknown" || portInfo.RawResponse != "" {
		t.Fatalf("expected SYN-open port without fingerprint after timeout, got %#v", portInfo)
	}
	if !isSYNConnectTimeout(context.Background(), &net.OpError{Err: context.DeadlineExceeded}) {
		t.Fatal("expected network timeout to be classified as uncertain")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if isSYNConnectTimeout(canceled, &net.OpError{Err: context.DeadlineExceeded}) {
		t.Fatal("canceled scan must not preserve timed-out port")
	}
}

func TestPortScanStageTrackerKeepsMainStagesOnly(t *testing.T) {
	tracker := newPortScanStageTracker(taskstate.Checkpoint{})
	tracker.Record(portScanStageAlive, "存活探测")
	tracker.Record(portScanStageAlive, "存活 8/10")
	tracker.Record(portScanStageScan, "端口探测 · SYN")
	tracker.Record(StageComplete, "端口扫描完成")

	events := tracker.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 main stage events, got %#v", events)
	}
	if events[0].Stage != portScanStageAlive || events[0].Label != "存活 8/10" {
		t.Fatalf("expected alive stage to be updated in place, got %#v", events[0])
	}

	meta := tracker.Meta(map[string]interface{}{"completed": 24, "total": 24})
	if meta["completed"] != 24 || meta["total"] != 24 {
		t.Fatalf("expected existing meta fields to be preserved, got %#v", meta)
	}
	if _, ok := meta["stageEvents"].([]portScanStageEvent); !ok {
		t.Fatalf("expected stageEvents in meta, got %#v", meta["stageEvents"])
	}
}

func TestCrackTargetTimeoutUsesShortSSHDefault(t *testing.T) {
	if got := crackTargetTimeout("ssh", map[string]interface{}{}); got != defaultSSHCrackTargetTimeout {
		t.Fatalf("expected ssh target timeout to be %s, got %s", defaultSSHCrackTargetTimeout, got)
	}
}

func TestCrackTargetTimeoutAllowsExplicitOverride(t *testing.T) {
	if got := crackTargetTimeout("ssh", map[string]interface{}{"targetTimeout": 5}); got != 5*time.Second {
		t.Fatalf("expected explicit target timeout to win, got %s", got)
	}
}

func TestSSHMaxTimeoutErrorsDefaultAndOverride(t *testing.T) {
	if got := sshMaxTimeoutErrors(map[string]interface{}{}); got != 5 {
		t.Fatalf("expected default ssh timeout error limit 5, got %d", got)
	}
	if got := sshMaxTimeoutErrors(map[string]interface{}{"sshMaxTimeoutErrors": float64(3)}); got != 3 {
		t.Fatalf("expected sshMaxTimeoutErrors override 3, got %d", got)
	}
	if got := sshMaxTimeoutErrors(map[string]interface{}{"sshMaxAuthTimeoutErrors": "0"}); got != 0 {
		t.Fatalf("expected sshMaxAuthTimeoutErrors override 0, got %d", got)
	}
}
