package subdomain

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"hephaestus-backend/pkg/types"
)

func TestWildcardBaselineMatchesObservedAnswersOnly(t *testing.T) {
	original := resolveDNSRecords
	t.Cleanup(func() { resolveDNSRecords = original })
	calls := 0
	resolveDNSRecords = func(string, []string, int) ([]string, []string, error) {
		calls++
		switch calls {
		case 1:
			return []string{"203.0.113.10"}, []string{" CatchAll.EXAMPLE.NET. "}, nil
		case 2:
			return nil, nil, errors.New("temporary DNS failure")
		default:
			return []string{"203.0.113.11", "2001:db8::1"}, []string{"catchall.example.net"}, nil
		}
	}
	baseline := probeWildcardDNS("example.com", []string{"127.0.0.1:53"}, 1)
	if calls != 3 || !baseline.detected() {
		t.Fatalf("expected three probes and a detected wildcard, got %d / %#v", calls, baseline)
	}
	tests := []struct {
		name        string
		ips, cnames []string
		want        bool
	}{
		{"matching address", []string{"203.0.113.10"}, nil, true},
		{"rotating addresses", []string{"203.0.113.11", "203.0.113.10"}, nil, true},
		{"IPv6 normalization", []string{"2001:0db8:0:0:0:0:0:1"}, nil, true},
		{"independent address", []string{"203.0.113.20"}, nil, false},
		{"mixed addresses", []string{"203.0.113.10", "203.0.113.20"}, nil, false},
		{"unresolved", nil, nil, false},
		{"matching CNAME with rotating address", []string{"203.0.113.12"}, []string{"CATCHALL.example.net."}, true},
		{"independent CNAME sharing address", []string{"203.0.113.10"}, []string{"app.example.net"}, false},
		{"CNAME only", nil, []string{"catchall.example.net"}, true},
		{"invalid address", []string{"invalid"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := baseline.matches(tt.ips, tt.cnames); got != tt.want {
				t.Fatalf("matches = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunnerPreservesResultsAndMarksOnlyMatchingWildcardChildren(t *testing.T) {
	original := resolveDNSRecords
	t.Cleanup(func() { resolveDNSRecords = original })
	resolveDNSRecords = func(host string, _ []string, _ int) ([]string, []string, error) {
		switch host {
		case "real.example.com":
			return []string{"203.0.113.20"}, nil, nil
		case "alias.example.com":
			return []string{"203.0.113.10"}, []string{"independent.example.net"}, nil
		case "unresolved.example.com":
			return nil, nil, nil
		default:
			return []string{"203.0.113.10"}, nil, nil
		}
	}
	results := make(map[string]bool)
	engine := NewSubdomainEngine(context.Background(), types.SubdomainOption{
		DnsServers: []string{"127.0.0.1:53"}, Timeout: 1, Thread: 2,
	}, func(event string, data interface{}) {
		if event == "subdomainLoading" {
			result := data.(SubdomainResult)
			results[result.Subdomain] = result.IsWildcard
		}
	})
	engine.RunnerWithSources(context.Background(), "example.com", []string{
		"example.com", "random.example.com", "real.example.com", "alias.example.com", "unresolved.example.com",
	}, "test", nil)
	want := map[string]bool{
		"example.com": false, "random.example.com": true, "real.example.com": false,
		"alias.example.com": false, "unresolved.example.com": false,
	}
	if !reflect.DeepEqual(results, want) {
		t.Fatalf("results = %#v, want %#v", results, want)
	}
}
