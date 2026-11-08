package shodan

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shodan/host/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("key") != "test-key" || query.Get("query") != "ip:1.2.3.4" || query.Get("page") != "2" || query.Get("minify") != "true" {
			t.Fatalf("unexpected query params: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"matches":[{"ip_str":"1.2.3.4","port":443,"domains":["example.com"],"http":{"title":"Example"},"_shodan":{"module":"https"}}],"total":12,"credits":1}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	result := client.Search("ip:1.2.3.4", 2)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Total != 12 || len(result.Matches) != 1 || result.Matches[0].IP != "1.2.3.4" || result.Matches[0].HTTP.Title != "Example" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestClientMapsMembershipError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"Requires membership or higher to access"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "oss-key")
	result := client.Search("ip:1.2.3.4", 1)
	if !strings.Contains(result.Error, "Membership") {
		t.Fatalf("expected a membership error, got %q", result.Error)
	}
}

func TestClientAPIInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api-info" || r.URL.Query().Get("key") != "test-key" {
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"query_credits":10,"scan_credits":2,"plan":"membership"}`))
	}))
	defer server.Close()

	info := NewClient(server.URL, "test-key").APIInfo()
	if info.Error != "" || info.QueryCredits != 10 || info.ScanCredits != 2 || info.Plan != "membership" {
		t.Fatalf("unexpected api info: %#v", info)
	}
}
