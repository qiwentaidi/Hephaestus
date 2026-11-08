package portscan

import (
	"context"
	gonmap "github.com/qiwentaidi/gonmap"
	"hephaestus-backend/pkg/taskstate"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestSubnetAdmissionConfigValidation(t *testing.T) {
	config, err := parseHostAdmissionConfig(map[string]interface{}{"subnetDiscoveryEnabled": true, "aliveDetection": false}, "80", "", 2)
	if err != nil || config.Subnet.Mode != "full" || config.TCP == nil || config.TCP.Timeout != 3*time.Second || config.Subnet.RandomSamples != 2 {
		t.Fatalf("bad defaults: %+v %v", config, err)
	}
	for key, value := range map[string]interface{}{"subnetDiscoveryMode": "wrong", "subnetDiscoveryHostSuffixes": "0,255", "subnetDiscoveryRandomSamples": 1.5} {
		if _, err := parseHostAdmissionConfig(map[string]interface{}{"subnetDiscoveryEnabled": true, key: value}, "80", "", 2); err == nil {
			t.Fatalf("invalid %s accepted", key)
		}
	}
	if _, err := parseHostAdmissionConfig(map[string]interface{}{"subnetDiscoveryEnabled": true}, "80,U:53", "", 2); err == nil {
		t.Fatal("UDP admitted")
	}
	config, err = parseHostAdmissionConfig(map[string]interface{}{"subnetDiscoveryEnabled": true, "subnetDiscoveryRandomSamples": 0}, "80", "socks5://127.0.0.1:1080", 2)
	if err != nil || config.Ping || config.Subnet.RandomSamples != 0 || config.TCP.ProxyURL == "" {
		t.Fatalf("proxy/explicit zero lost: %+v %v", config, err)
	}
}

func TestSubnetAdmissionFastFullEvidenceAndExplicitTargets(t *testing.T) {
	// Two /24s, one confirmed, one unconfirmed. Additional live devices exist
	// outside the sampled ports; injected Ping represents their responses.
	targets := []string{"192.0.2.0/23", "192.0.2.0/23", "192.0.3.99"}
	snapshot := &subnetAdmission{Signature: "sig", Confirmed: 1, Unconfirmed: 1, Results: []gonmap.SubnetDiscoveryResult{
		{CIDR: "192.0.2.0/24", Status: gonmap.HostUnconfirmed},
		{CIDR: "192.0.3.0/24", Status: gonmap.HostConfirmed, EvidenceHost: "192.0.3.1", Source: "tcp", Port: 80},
	}}
	config := hostAdmissionConfig{Ping: true, Subnet: &subnetAdmissionConfig{Mode: "fast"}}
	var probed []string
	ping := func(_ context.Context, hosts []string, _ bool) []string {
		probed = append(probed, hosts...)
		var live []string
		for _, h := range hosts {
			if h == "192.0.2.42" || h == "192.0.3.42" || h == "192.0.3.99" {
				live = append(live, h)
			}
		}
		return live
	}
	fast, err := discoverSubnetHosts(context.Background(), targets, config, snapshot, ping)
	if err != nil {
		t.Fatal(err)
	}
	// No-response hosts must not be admitted just because their subnet is live.
	if !reflect.DeepEqual(fast.Hosts, []string{"192.0.3.99", "192.0.3.1", "192.0.3.42"}) {
		t.Fatalf("fast admitted wrong hosts: %v", fast.Hosts)
	}
	for _, h := range probed {
		if h == "192.0.2.42" || h == "192.0.3.1" {
			t.Fatalf("fast probed skipped network/evidence host: %s", h)
		}
	}
	config.Subnet.Mode = "full"
	probed = nil
	full, err := discoverSubnetHosts(context.Background(), targets, config, snapshot, ping)
	if err != nil || !reflect.DeepEqual(full.Hosts, append(fast.Hosts, "192.0.2.42")) {
		t.Fatalf("full lost remaining network/priority: %v %v", full, err)
	}
	// Explicit target within an unconfirmed network must survive fast filtering.
	config.Subnet.Mode = "fast"
	explicit, err := discoverSubnetHosts(context.Background(), append(targets, "192.0.2.42"), config, snapshot, ping)
	if err != nil || len(explicit.Hosts) != 4 {
		t.Fatalf("explicit target filtered: %v %v", explicit, err)
	}
	checkpoint := taskstate.ParseCheckpoint(taskstate.MarshalCheckpoint("portscan:scanning", 20, full.Meta(map[string]interface{}{"completed": 1})))
	restored, ok := restoreHostAdmission(checkpoint, "sig")
	if !ok || !reflect.DeepEqual(restored.Hosts, full.Hosts) || restored.Subnets.Confirmed != 1 {
		t.Fatal("resume changed subnet order/evidence")
	}
	addresses := make(chan Address, len(restored.Hosts))
	for _, host := range restored.Hosts {
		addresses <- Address{IP: host, Port: 80}
	}
	close(addresses)
	groups := collectAddressGroups(context.Background(), addresses)
	prioritizeAddressGroups(groups, restored.Hosts)
	remaining := trimAddressGroupsForResume(groups, 1)
	if len(remaining) != 3 || remaining[0].IP != "192.0.3.1" || remaining[2].IP != "192.0.2.42" {
		t.Fatalf("port queue lost priority/resume order: %+v", remaining)
	}
}

func TestSubnetAdmissionRealTCPAndCheckpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	config := hostAdmissionConfig{TCP: &gonmap.HostDiscoveryOptions{Ports: []int{listener.Addr().(*net.TCPAddr).Port}, Timeout: 10 * time.Millisecond, Concurrency: 2}, Subnet: &subnetAdmissionConfig{Mode: "fast", HostSuffixes: []int{1}}}
	snapshot, err := discoverSubnetAdmission(context.Background(), []string{"127.0.0.0/23"}, config, "sig", nil, nil)
	if err != nil || snapshot.Confirmed != 1 || snapshot.Unconfirmed != 1 || snapshot.Results[0].EvidenceHost != "127.0.0.1" {
		t.Fatalf("bad real discovery: %+v %v", snapshot, err)
	}
	checkpoint := taskstate.ParseCheckpoint(taskstate.MarshalCheckpoint("portscan:alive", 0, map[string]interface{}{"subnetAdmission": snapshot}))
	restored := restoreSubnetAdmission(checkpoint, "sig")
	if restored == nil || !reflect.DeepEqual(restored.Results, snapshot.Results) || restoreSubnetAdmission(checkpoint, "changed") != nil {
		t.Fatal("unsafe subnet snapshot restore")
	}
	config.Ping = true
	result, err := discoverSubnetHosts(context.Background(), []string{"127.0.0.0/23"}, config, snapshot, func(_ context.Context, hosts []string, _ bool) []string { return hosts })
	if err != nil || result.Hosts[0] != "127.0.0.1" {
		t.Fatalf("evidence not reused: %+v %v", result, err)
	}
}
