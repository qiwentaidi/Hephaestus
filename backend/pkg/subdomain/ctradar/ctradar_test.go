package ctradar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchHostsWithURL(t *testing.T) {
	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-Key")
		if r.URL.Query().Get("q") != "example.com" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("exclude_wildcard") != "true" {
			t.Fatalf("expected wildcard exclusion")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"domain":"www.example.com"},{"domain":"*.api.example.com"},{"domain":"www.example.com"},{"domain":"outside.test"}]}`))
	}))
	defer server.Close()

	hosts, err := fetchHosts(context.Background(), server.URL, "example.com", "ctr_test", 100)
	if err != nil {
		t.Fatalf("fetchHosts returned error: %v", err)
	}
	if gotKey != "ctr_test" {
		t.Fatalf("expected API key header, got %q", gotKey)
	}

	want := []string{"api.example.com", "www.example.com"}
	if len(hosts) != len(want) {
		t.Fatalf("expected %v, got %v", want, hosts)
	}
	for i := range want {
		if hosts[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, hosts)
		}
	}
}
