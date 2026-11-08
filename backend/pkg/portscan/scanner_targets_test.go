package portscan

import (
	"reflect"
	"testing"
)

func TestExpandPortScanHostsPreservesDomainsAndExpandsIPRanges(t *testing.T) {
	got := expandPortScanHosts([]string{
		"https://example.com:8443/path",
		"192.0.2.1",
		"192.0.2.4/31",
		"example.com",
	})

	want := []string{"example.com", "192.0.2.1", "192.0.2.4", "192.0.2.5"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected hosts: got %#v want %#v", got, want)
	}
}

func TestNormalizePortScanHost(t *testing.T) {
	cases := map[string]string{
		"https://example.com:8443/path": "example.com",
		"example.org:443":               "example.org",
		"[2001:db8::1]:443":             "2001:db8::1",
		"192.0.2.1":                     "192.0.2.1",
	}

	for input, want := range cases {
		if got := normalizePortScanHost(input); got != want {
			t.Fatalf("normalizePortScanHost(%q) = %q, want %q", input, got, want)
		}
	}
}
