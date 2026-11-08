//go:build darwin || linux

package synpriv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverOpenPortsWithStatusCallbackStreamsPortEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake-helper")
	script := "#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' '{\"type\":\"port\",\"port\":{\"host\":\"192.0.2.1\",\"port\":80}}' '{\"type\":\"port\",\"port\":{\"host\":\"192.0.2.1\",\"port\":443}}' '{\"type\":\"done\"}'\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	var streamed []OpenPort
	ports, err := DiscoverOpenPortsWithStatusCallback(context.Background(), HelperStatus{Path: path, Usable: true}, DiscoveryRequest{
		Hosts: []string{"192.0.2.1"}, Ports: []int{80, 443}, TimeoutMS: 1000, Rate: 3000,
	}, func(port OpenPort) {
		streamed = append(streamed, port)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(streamed) != 2 || len(ports) != 2 || streamed[0].Port != 80 || streamed[1].Port != 443 {
		t.Fatalf("unexpected streamed=%#v returned=%#v", streamed, ports)
	}
}
