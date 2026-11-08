package synpriv

import (
	"testing"
	"time"
)

func TestHelperRunTimeoutIncludesRateLimitedScan(t *testing.T) {
	request := DiscoveryRequest{Hosts: make([]string, 35), Ports: make([]int, 1928), TimeoutMS: 10000, Rate: 3000}
	if got := helperRunTimeout(request); got < 45*time.Second {
		t.Fatalf("helper timeout must allow 67480 probes at rate 3000, got %s", got)
	}
}

func TestSupportsRawSocketPlatform(t *testing.T) {
	tests := []struct {
		platform string
		want     bool
	}{
		{platform: "darwin", want: true},
		{platform: "linux", want: true},
		{platform: "android", want: true},
		{platform: "windows", want: false},
		{platform: "freebsd", want: false},
	}

	for _, test := range tests {
		t.Run(test.platform, func(t *testing.T) {
			if got := SupportsRawSocketPlatform(test.platform); got != test.want {
				t.Fatalf("SupportsRawSocketPlatform(%q) = %v, want %v", test.platform, got, test.want)
			}
		})
	}
}
