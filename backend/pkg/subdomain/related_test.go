package subdomain

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"hephaestus-backend/pkg/types"
)

func TestFilterRelatedDomains(t *testing.T) {
	got := filterRelatedDomains([]string{
		"ab1.com",
		"www.ab1.com",
		"ab1.com",
		"AB2.COM.",
		"a.com",
		"sub.a.com",
		"not-a-domain",
		"localhost",
		"",
	}, []string{"a.com"})

	want := []string{"ab1.com", "ab2.com", "www.ab1.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterRelatedDomains = %v, want %v", got, want)
	}
}

func TestFilterRelatedDomainsCapsResults(t *testing.T) {
	candidates := make([]string, 0, maxRelatedDomains+10)
	for i := 0; i < maxRelatedDomains+10; i++ {
		candidates = append(candidates, "domain"+string(rune('a'+i%26))+"-"+string(rune('a'+(i/26)%26))+".com")
	}
	got := filterRelatedDomains(candidates, nil)
	if len(got) > maxRelatedDomains {
		t.Fatalf("expected at most %d domains, got %d", maxRelatedDomains, len(got))
	}
}

func TestRelatedRootsUseClosestRelatedDomain(t *testing.T) {
	got := relatedRoots([]string{"example.org", "www.example.org", "a.www.example.org", "example.net"})
	want := map[string]string{
		"example.org":       "example.org",
		"www.example.org":   "example.org",
		"a.www.example.org": "www.example.org",
		"example.net":       "example.net",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("relatedRoots = %v, want %v", got, want)
	}
}

func TestRunnerRelatedGroupsHostsUnderRelatedRoots(t *testing.T) {
	original := resolveDNSRecords
	t.Cleanup(func() { resolveDNSRecords = original })
	resolveDNSRecords = func(string, []string, int) ([]string, []string, error) {
		return []string{"203.0.113.9"}, nil, nil
	}

	var mu sync.Mutex
	results := make([]SubdomainResult, 0)
	engine := NewSubdomainEngine(context.Background(), types.SubdomainOption{Thread: 2, Timeout: 1}, func(event string, data interface{}) {
		if event != "subdomainLoading" {
			return
		}
		result, ok := data.(SubdomainResult)
		if !ok {
			return
		}
		mu.Lock()
		results = append(results, result)
		mu.Unlock()
	})

	engine.RunnerRelated(context.Background(), []string{"ab1.com", "www.ab1.com", "ab2.com"}, "crt.sh")

	mu.Lock()
	defer mu.Unlock()
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	bySubdomain := make(map[string]SubdomainResult, len(results))
	for _, result := range results {
		if result.Source != "crt.sh" {
			t.Fatalf("expected crt.sh source, got %q", result.Source)
		}
		if len(result.IPs) != 1 || result.IPs[0] != "203.0.113.9" {
			t.Fatalf("unexpected ips: %v", result.IPs)
		}
		bySubdomain[result.Subdomain] = result
	}
	expected := map[string]string{
		"ab1.com":     "ab1.com",
		"www.ab1.com": "ab1.com",
		"ab2.com":     "ab2.com",
	}
	for subdomain, root := range expected {
		result, ok := bySubdomain[subdomain]
		if !ok {
			t.Fatalf("missing result for %s: %v", subdomain, bySubdomain)
		}
		if result.Domain != root {
			t.Fatalf("expected %s root %q, got %q", subdomain, root, result.Domain)
		}
	}
}

func TestApiPolymerizationSplitsCRTShRelatedDomains(t *testing.T) {
	originalFetch := fetchCRTShNames
	originalResolve := resolveDNSRecords
	t.Cleanup(func() {
		fetchCRTShNames = originalFetch
		resolveDNSRecords = originalResolve
	})

	fetchCRTShNames = func(context.Context, string, string) ([]string, error) {
		return []string{"example.com", "www.example.com", "ab1.com", "www.ab1.com", "ab2.com"}, nil
	}
	resolveDNSRecords = func(string, []string, int) ([]string, []string, error) {
		return []string{"203.0.113.9"}, nil, nil
	}

	var mu sync.Mutex
	results := make([]SubdomainResult, 0)
	engine := NewSubdomainEngine(context.Background(), types.SubdomainOption{
		Domains:       []string{"example.com"},
		Thread:        2,
		Timeout:       1,
		AppendEngines: []string{"crt.sh"},
	}, func(event string, data interface{}) {
		if event != "subdomainLoading" {
			return
		}
		result, ok := data.(SubdomainResult)
		if !ok {
			return
		}
		mu.Lock()
		results = append(results, result)
		mu.Unlock()
	})

	engine.ApiPolymerization(context.Background())

	mu.Lock()
	defer mu.Unlock()
	keys := make(map[string]bool, len(results))
	for _, result := range results {
		keys[result.Domain+"|"+result.Subdomain] = true
	}
	for _, key := range []string{
		"example.com|example.com",
		"example.com|www.example.com",
		"ab1.com|ab1.com",
		"ab1.com|www.ab1.com",
		"ab2.com|ab2.com",
	} {
		if !keys[key] {
			t.Fatalf("expected result %q, got %v", key, keys)
		}
	}
	if keys["example.com|ab1.com"] {
		t.Fatalf("related domain was stored under the target root: %v", keys)
	}
}
