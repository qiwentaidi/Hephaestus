package crtname

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchHostsWithURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apex") != "example.com" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("format") != "json" {
			t.Fatalf("expected json format")
		}
		if r.URL.Query().Get("dates") != "1" {
			t.Fatalf("expected dates flag")
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("expected json accept header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"first_seen":"2024-01-01T00:00:00Z","sub":"www.example.com"},
			{"first_seen":null,"sub":"*.api.example.com"},
			{"first_seen":"2024-01-01T00:00:00Z","sub":"www.example.com"},
			{"first_seen":"2024-01-01T00:00:00Z","sub":"outside.test"}
		]`))
	}))
	defer server.Close()

	hosts, err := fetchHosts(context.Background(), server.URL, "Example.COM.")
	if err != nil {
		t.Fatalf("fetchHosts returned error: %v", err)
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

func TestFetchHostsReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid apex", http.StatusBadRequest)
	}))
	defer server.Close()

	if _, err := fetchHosts(context.Background(), server.URL, "example.com"); err == nil {
		t.Fatal("expected status error")
	}
}
