package portscan

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"hephaestus-backend/pkg/taskstate"

	gonmap "github.com/qiwentaidi/gonmap/lib"
)

func TestHostAdmissionConfigDefaultsAndValidation(t *testing.T) {
	legacy, err := parseHostAdmissionConfig(nil, "1-65535", "", 600)
	if err != nil || !legacy.Ping || legacy.TCP != nil {
		t.Fatalf("legacy defaults changed: %+v %v", legacy, err)
	}
	defaults, err := parseHostAdmissionConfig(map[string]interface{}{"tcpHeuristicEnabled": true, "aliveDetection": false}, "1-65535", "", 600)
	if err != nil || defaults.Ping || defaults.TCP.Timeout != 3*time.Second || defaults.TCP.Concurrency != 256 || !reflect.DeepEqual(defaults.TCP.Ports, []int{80, 443, 22, 3389, 445}) {
		t.Fatalf("incorrect TCP defaults: %+v %v", defaults.TCP, err)
	}
	custom, err := parseHostAdmissionConfig(map[string]interface{}{"tcpHeuristicEnabled": true, "tcpHeuristicTimeoutMs": 1500}, "80", "", 20)
	if err != nil || custom.TCP.Timeout != 1500*time.Millisecond {
		t.Fatalf("explicit timeout duration changed: %+v %v", custom.TCP, err)
	}
	for _, test := range []struct {
		key       string
		value     interface{}
		portRange string
	}{
		{"tcpHeuristicPorts", "0,80", "80"},
		{"tcpHeuristicPorts", "80-443", "80"},
		{"tcpHeuristicPorts", "65536", "80"},
		{"tcpHeuristicTimeoutMs", 0, "80"},
		{"tcpHeuristicTimeoutMs", 1000.5, "80"},
		{"tcpHeuristicTimeoutMs", 30001, "80"},
		{"tcpHeuristicPorts", "80", "U:53"},
		{"tcpHeuristicPorts", "80", "80,U:53"},
	} {
		config := map[string]interface{}{"tcpHeuristicEnabled": true, test.key: test.value}
		if _, err := parseHostAdmissionConfig(config, test.portRange, "", 20); err == nil {
			t.Errorf("accepted invalid config: %+v", test)
		}
	}
	proxied, err := parseHostAdmissionConfig(map[string]interface{}{"tcpHeuristicEnabled": true}, "80", "socks5://127.0.0.1:1080", 20)
	if err != nil || proxied.Ping || proxied.TCP.ProxyURL == "" {
		t.Fatalf("proxy admission must not use local ICMP: %+v %v", proxied, err)
	}
}

func TestHostAdmissionCombinesPingAndRealTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	hosts := []string{"127.0.0.3", "127.0.0.1", "127.0.0.2"}
	config := hostAdmissionConfig{Ping: true, TCP: &gonmap.HostDiscoveryOptions{Ports: []int{port}, Timeout: 100 * time.Millisecond, Concurrency: 2}}
	ping := func(context.Context, []string, bool) []string { return []string{"127.0.0.3"} }
	result, err := discoverAdmittedHosts(context.Background(), hosts, config, "signature", ping)
	if err != nil || !reflect.DeepEqual(result.Hosts, []string{"127.0.0.1", "127.0.0.3"}) || result.PingAlive != 1 || result.TCPAlive != 1 || result.Unconfirmed != 1 {
		t.Fatalf("ICMP/TCP must be a union: %+v %v", result, err)
	}
	config.Ping = false
	result, err = discoverAdmittedHosts(context.Background(), hosts, config, "signature", func(context.Context, []string, bool) []string { t.Fatal("Ping disabled"); return nil })
	if err != nil || !reflect.DeepEqual(result.Hosts, []string{"127.0.0.1"}) {
		t.Fatalf("TCP-only discovery failed: %+v %v", result, err)
	}
	config.TCP = nil
	result, err = discoverAdmittedHosts(context.Background(), hosts, config, "signature", nil)
	if err != nil || len(result.Hosts) != len(hosts) || result.Unconfirmed != 0 {
		t.Fatalf("disabled discovery must admit every host: %+v %v", result, err)
	}
}

func TestHostAdmissionSnapshotSurvivesRestartWithoutReprobe(t *testing.T) {
	hosts := []string{"192.0.2.2", "192.0.2.1", "192.0.2.3"}
	config, _ := parseHostAdmissionConfig(map[string]interface{}{"tcpHeuristicEnabled": true}, "80,443", "", 30)
	signature := hostAdmissionSignature(hosts, config, "80,443")
	admission := &hostAdmission{Signature: signature, Hosts: []string{"192.0.2.1", "192.0.2.3"}, Total: 3, TCPAlive: 2, Unconfirmed: 1}
	meta := admission.Meta(portScanCheckpointMeta(TCPDiscoveryConfig{}, 2, 4))
	checkpoint := taskstate.ParseCheckpoint(taskstate.MarshalCheckpoint("portscan:scanning", 50, meta))
	// The persisted host set is authoritative on restart, including hosts that
	// have since stopped responding. Re-filtering would shift the port cursor.
	got, ok := restoreHostAdmission(checkpoint, signature)
	if !ok || !reflect.DeepEqual(got.Hosts, admission.Hosts) || checkpoint.Meta["completed"] != float64(2) {
		t.Fatalf("unsafe resume snapshot: %+v", checkpoint)
	}
	groups := []addressGroup{{IP: got.Hosts[0], Ports: []int{80, 443}}, {IP: got.Hosts[1], Ports: []int{80, 443}}}
	remaining := trimAddressGroupsForResume(groups, 2)
	if len(remaining) != 1 || remaining[0].IP != "192.0.2.3" {
		t.Fatalf("resume cursor changed host: %+v", remaining)
	}
	if _, ok := restoreHostAdmission(checkpoint, hostAdmissionSignature(hosts, config, "1-65535")); ok {
		t.Fatal("changed port range reused an incompatible snapshot")
	}
	empty := &hostAdmission{Signature: signature, Hosts: []string{}, Total: 3, Unconfirmed: 3}
	checkpoint = taskstate.ParseCheckpoint(taskstate.MarshalCheckpoint("portscan:alive_done", 0, empty.Meta(nil)))
	if result, ok := restoreHostAdmission(checkpoint, signature); !ok || len(result.Hosts) != 0 {
		t.Fatalf("empty completed admission must not trigger a rescan: %+v", result)
	}
}

func TestHostAdmissionCancellationDoesNotProduceCompletedSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	config := hostAdmissionConfig{Ping: true}
	result, err := discoverAdmittedHosts(ctx, []string{"127.0.0.1"}, config, "", func(context.Context, []string, bool) []string { cancel(); return []string{"127.0.0.1"} })
	if err != context.Canceled || result != nil {
		t.Fatal(fmt.Sprintf("cancellation treated as completed discovery: %+v %v", result, err))
	}
}
