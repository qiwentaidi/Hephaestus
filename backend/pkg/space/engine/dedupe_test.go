package engine

import "testing"

func TestDeduplicateResultsUsesIPPortDomain(t *testing.T) {
	results := []UnifiedResult{
		{URL: "http://192.0.2.60:443", IP: "192.0.2.60", Port: 443, Protocol: "http", Title: "", Source: "hunter"},
		{URL: "https://192.0.2.60", IP: "192.0.2.60", Port: 443, Protocol: "https", Title: "404 Not Found", Source: "fofa"},
		{URL: "", IP: "192.0.2.60", Port: 22, Protocol: "ssh", Components: []string{"OpenSSH"}, Source: "fofa"},
		{URL: "ssh://192.0.2.60:22", IP: "192.0.2.60", Port: 22, Protocol: "ssh", Domain: "ssh", Components: []string{"OpenSSH", "7.4"}, Source: "quake"},
		{URL: "http://192.0.2.60", IP: "192.0.2.60", Port: 80, Protocol: "http", Title: "404", Source: "hunter"},
		{URL: "http://192.0.2.60/", IP: "192.0.2.60", Port: 80, Protocol: "http", Title: "404 Not Found", Source: "fofa"},
	}
	got := DeduplicateResults(results)
	if len(got) != 3 {
		t.Fatalf("expected 3 unique IP:port:domain records, got %d: %#v", len(got), got)
	}
	if got[0].Port != 443 || got[1].Port != 22 || got[2].Port != 80 {
		t.Fatalf("unexpected order or ports: %#v", got)
	}
	if got[0].Source != "hunter" || got[1].Source != "fofa" {
		t.Fatalf("dedupe should keep the first occurrence: %#v", got)
	}
}

func TestDeduplicateResultsKeepsDifferentDomainsOnSameEndpoint(t *testing.T) {
	results := []UnifiedResult{
		{URL: "https://a.example.com", IP: "192.0.2.10", Port: 443, Protocol: "https", Domain: "a.example.com"},
		{URL: "https://b.example.com", IP: "192.0.2.10", Port: 443, Protocol: "https", Domain: "b.example.com"},
		{URL: "https://a.example.com/", IP: "192.0.2.10", Port: 443, Protocol: "https", Domain: "A.Example.com"},
		{IP: "192.0.2.11", Port: 8443, Protocol: "https", Domain: "api.example.com"},
		{IP: "192.0.2.11", Port: 8443, Protocol: "https", Domain: "api.example.com"},
	}
	got := DeduplicateResults(results)
	if len(got) != 3 {
		t.Fatalf("expected 3 unique records, got %d: %#v", len(got), got)
	}
}

func TestDeduplicateResultsKeepsResultsWithoutIdentity(t *testing.T) {
	results := []UnifiedResult{
		{Title: "first"},
		{Title: "second"},
	}
	if got := DeduplicateResults(results); len(got) != 2 {
		t.Fatalf("results without IP or domain must be preserved: %#v", got)
	}
}

func TestDeduplicateAgainstSkipsAlreadyStoredResults(t *testing.T) {
	existing := []UnifiedResult{
		{URL: "https://example.com", IP: "192.0.2.1", Port: 443, Protocol: "https", Domain: "example.com"},
		{IP: "192.0.2.60", Port: 22, Protocol: "ssh"},
	}
	incoming := []UnifiedResult{
		{URL: "https://example.com/", IP: "192.0.2.1", Port: 443, Protocol: "https", Domain: "example.com"},
		{IP: "192.0.2.60", Port: 22, Protocol: "ssh", Source: "fofa"},
		{IP: "192.0.2.60", Port: 8080, Protocol: "http"},
	}
	got := DeduplicateAgainst(existing, incoming)
	if len(got) != 1 || got[0].Port != 8080 {
		t.Fatalf("expected only the new port to be appended: %#v", got)
	}
}

func TestDeduplicateResultsMergesICPFromDuplicate(t *testing.T) {
	results := []UnifiedResult{
		{URL: "https://example.com", IP: "192.0.2.1", Port: 443, Protocol: "https", Domain: "example.com", Source: "fofa"},
		{URL: "https://example.com/", IP: "192.0.2.1", Port: 443, Protocol: "https", Domain: "example.com", ICP: "京ICP备12345678号", ICPName: "示例公司", Source: "quake"},
	}
	got := DeduplicateResults(results)
	if len(got) != 1 {
		t.Fatalf("expected one deduplicated record, got %#v", got)
	}
	if got[0].Source != "fofa" {
		t.Fatalf("dedupe should keep the first occurrence provenance: %#v", got[0])
	}
	if got[0].ICP != "京ICP备12345678号" || got[0].ICPName != "示例公司" {
		t.Fatalf("expected ICP facts to merge into the retained record: %#v", got[0])
	}
}

func TestDeduplicateAgainstMergesICPIntoExisting(t *testing.T) {
	existing := []UnifiedResult{
		{URL: "https://example.com", IP: "192.0.2.1", Port: 443, Protocol: "https", Domain: "example.com"},
	}
	incoming := []UnifiedResult{
		{URL: "https://example.com/", IP: "192.0.2.1", Port: 443, Protocol: "https", Domain: "example.com", ICP: "京ICP备12345678号", ICPName: "示例公司"},
	}
	if got := DeduplicateAgainst(existing, incoming); len(got) != 0 {
		t.Fatalf("expected duplicate to be skipped: %#v", got)
	}
	if existing[0].ICP != "京ICP备12345678号" || existing[0].ICPName != "示例公司" {
		t.Fatalf("expected ICP facts to merge into the stored record: %#v", existing[0])
	}
}

func TestQuakeDomainIgnoresServiceNamesAndIPs(t *testing.T) {
	cases := []struct {
		host, rawURL, want string
	}{
		{"ssh", "ssh://192.0.2.60:22", ""},
		{"", "ssh://192.0.2.60:22", ""},
		{"", "https://api.example.com:443/login", "api.example.com"},
		{"192.0.2.60", "", ""},
		{"Sub.Example.com", "http://other.example.com", "sub.example.com"},
		{"", "api.example.com:8080/x", "api.example.com"},
	}
	for _, tc := range cases {
		if got := quakeDomain(tc.host, tc.rawURL); got != tc.want {
			t.Fatalf("quakeDomain(%q, %q) = %q, want %q", tc.host, tc.rawURL, got, tc.want)
		}
	}
}
