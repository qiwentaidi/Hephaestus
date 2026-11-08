package portscan

import (
	"context"
	gonmap "github.com/qiwentaidi/gonmap/lib"
	"hephaestus-backend/pkg/taskstate"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestSYNRoundsProduceMonotonicDisplayProgress(t *testing.T) {
	var progress []int32
	opts := gonmap.PortDiscoveryOptions{Mode: gonmap.PortDiscoveryModeSYN, OnProgress: func(n, total int) { progress = append(progress, synDisplayWork(320, n, total)) }}
	calls := 0
	discover := func(ctx context.Context, hosts []string, ports []int, options *gonmap.PortDiscoveryOptions, onOpen func(gonmap.OpenPort)) []gonmap.OpenPort {
		calls++
		options.OnProgress(160, 320)
		options.OnProgress(320, 320)
		return []gonmap.OpenPort{{Host: "192.0.2.1", Port: 80}}
	}
	result := runNativeSYNAttempts(context.Background(), []string{"192.0.2.1"}, []int{80}, opts, 2, nil, nil, discover)
	if calls != 2 || len(result) != 1 || !reflect.DeepEqual(progress, []int32{64, 128, 192, 256}) {
		t.Fatalf("calls=%d result=%v progress=%v", calls, result, progress)
	}
}
func TestSYNCancellationStopsRoundsWithoutCompletingProgress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	var n, total int
	opts := gonmap.PortDiscoveryOptions{Mode: gonmap.PortDiscoveryModeSYN, OnProgress: func(a, b int) { n, total = a, b }}
	discover := func(context.Context, []string, []int, *gonmap.PortDiscoveryOptions, func(gonmap.OpenPort)) []gonmap.OpenPort {
		return nil
	}
	discover = func(ctx context.Context, hosts []string, ports []int, o *gonmap.PortDiscoveryOptions, cb func(gonmap.OpenPort)) []gonmap.OpenPort {
		calls++
		o.OnProgress(7, 320)
		cancel()
		return nil
	}
	runNativeSYNAttempts(ctx, []string{"192.0.2.1"}, []int{80}, opts, 2, nil, nil, discover)
	if calls != 1 || n != 7 || total != 640 {
		t.Fatalf("calls=%d progress=%d/%d", calls, n, total)
	}
}
func TestSYNCheckpointRoundTripReplaysAllWork(t *testing.T) {
	config := TCPDiscoveryConfig{Mode: gonmap.PortDiscoveryModeSYN}
	groups := []addressGroup{{IP: "192.0.2.1", Ports: []int{80, 443}}, {IP: "192.0.2.2", Ports: []int{22, 8080}}}
	for _, display := range []int32{1, 2, 4} {
		// Use the exact metadata constructor used by the live scanner, then simulate
		// durable serialization and process restart before completing fingerprints.
		meta := portScanCheckpointMeta(config, display, 4)
		checkpoint := taskstate.ParseCheckpoint(taskstate.MarshalCheckpoint("portscan:scanning", portScanDisplayPercent(config, display, 4, 0), meta))
		resume := portScanResumeCompleted(config, int32(checkpoint.Meta["completed"].(float64)))
		got := trimAddressGroupsForResume(groups, resume)
		if !reflect.DeepEqual(got, groups) || checkpoint.Progress >= 100 || checkpoint.Meta["displayCompleted"] != float64(display) {
			t.Fatalf("checkpoint=%+v remaining=%v", checkpoint, got)
		}
	}
	// Old checkpoints may contain unsafe attempted counts; SYN ignores them.
	if got := trimAddressGroupsForResume(groups, portScanResumeCompleted(config, 4)); !reflect.DeepEqual(got, groups) {
		t.Fatal(got)
	}
	// Connect retains its existing durable cursor and percentage behavior.
	connect := TCPDiscoveryConfig{Mode: gonmap.PortDiscoveryModeConnect}
	if got := trimAddressGroupsForResume(groups, portScanResumeCompleted(connect, 2)); len(got) != 1 || got[0].IP != "192.0.2.2" {
		t.Fatal(got)
	}
	if portScanDisplayPercent(connect, 4, 4, 0) != 100 || portScanDisplayPercent(config, 1, 4, 100) != 99 {
		t.Fatal("incorrect display cap")
	}
}
func TestSYNRetryBudgetIsDiagnosticThreshold(t *testing.T) {
	if got := synRetryBudget(81920, 3000, 3*time.Second); got < 105919*time.Millisecond || got > 105921*time.Millisecond {
		t.Fatal(got)
	}
	if synRetryBudget(0, 3000, time.Second) != 0 || synRetryBudget(1, 0, 0) != time.Minute {
		t.Fatal("budget bounds")
	}
	// A rate high enough to be faster than the driver does not change the number
	// of configured attempts. The loop contains no budget-based break.
}

func TestSYNDiscoveryPropagatesCancellationAndSetupFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := synDiscoveryRuntime{hasRawSocket: true}
	opts := gonmap.PortDiscoveryOptions{Mode: gonmap.PortDiscoveryModeSYN}
	if _, err := discoverOpenPortsForBatch(ctx, gonmap.New(nil), []string{"192.0.2.1"}, []int{80}, opts, runtime, 2, nil, nil); err != context.Canceled {
		t.Fatalf("cancellation swallowed: %v", err)
	}
	// IPv6 produces a setup failure before raw sockets are opened.
	if _, err := discoverOpenPortsForBatch(context.Background(), gonmap.New(nil), []string{"::1"}, []int{80}, opts, runtime, 1, nil, nil); err == nil {
		t.Fatal("setup failure presented as an empty successful scan")
	}
}

func TestSYNControlWatcherAcknowledgesInflightPollBeforeExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, sawCancel, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	wait := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("control watcher timeout")
		}
	}
	stop := startPortScanControlWatcher(ctx, cancel, func(pollCtx context.Context) bool {
		close(entered)
		<-pollCtx.Done()
		close(sawCancel)
		<-release
		return false
	})
	defer func() { unblock(); stop() }()
	wait(entered)
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	wait(sawCancel)
	select {
	case <-stopped:
		t.Fatal("control watcher acknowledged before query exited")
	default:
	}
	unblock()
	wait(stopped)
}
