package portscan

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestBuildAliveHostAliasesMapsResolvedIPToOriginalHost(t *testing.T) {
	aliases := buildAliveHostAliases([]string{"localhost"})

	if aliases["localhost"] != "localhost" {
		t.Fatalf("expected direct localhost alias, got %q", aliases["localhost"])
	}
	if aliases["127.0.0.1"] != "localhost" {
		t.Fatalf("expected 127.0.0.1 to map to localhost, got %q", aliases["127.0.0.1"])
	}
}

func TestCheckLiveWithContextStopsBeforeDispatchWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	got := CheckLiveWithContext(ctx, []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"}, true)
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("expected canceled live check to return quickly, took %s", elapsed)
	}
	if len(got) != 0 {
		t.Fatalf("expected no alive hosts after pre-canceled context, got %#v", got)
	}
}

func TestCollectAliveHostsIsolatedAcrossConcurrentChecks(t *testing.T) {
	type probeResult struct {
		hosts []string
	}

	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan probeResult, 2)

	run := func(host string) {
		results <- probeResult{hosts: collectAliveHosts([]string{host}, func(chanHosts chan string) {
			ready <- struct{}{}
			<-start
			chanHosts <- host
		})}
	}

	go run("192.0.2.10")
	go run("198.51.100.20")
	<-ready
	<-ready
	close(start)

	seen := make(map[string][]string, 2)
	for i := 0; i < 2; i++ {
		result := <-results
		if len(result.hosts) != 1 {
			t.Fatalf("expected one isolated live host, got %#v", result.hosts)
		}
		seen[result.hosts[0]] = result.hosts
	}

	for _, host := range []string{"192.0.2.10", "198.51.100.20"} {
		if got := seen[host]; !reflect.DeepEqual(got, []string{host}) {
			t.Fatalf("live check for %s was contaminated: %#v", host, got)
		}
	}
}
