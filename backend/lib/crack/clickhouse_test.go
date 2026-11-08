package crack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClickHouseUnauth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := strings.TrimSpace(r.URL.Query().Get("query")); got != "SELECT 1" {
			t.Fatalf("expected SELECT 1 query, got %q", got)
		}
		_, _ = w.Write([]byte("1\n"))
	}))
	defer server.Close()

	flag, err := ClickHouseUnauth(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("expected unauth query to succeed, got error: %v", err)
	}
	if !flag {
		t.Fatal("expected unauth query to return true")
	}
}

func TestClickHouseConn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "default" || pass != "clickhouse" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("1\n"))
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	flag, err := ClickHouseConn(host, "default", "clickhouse")
	if err != nil {
		t.Fatalf("expected authenticated query to succeed, got error: %v", err)
	}
	if !flag {
		t.Fatal("expected authenticated query to return true")
	}

	flag, err = ClickHouseConn(host, "default", "wrong")
	if err == nil {
		t.Fatal("expected wrong password to return an error")
	}
	if flag {
		t.Fatal("expected wrong password to return false")
	}
}

func TestClickHouseScanReportsWeakPassword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "default" || pass != "clickhouse" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("1\n"))
	}))
	defer server.Close()

	var got *VulnerabilityInfo
	ClickHouseScan(context.Background(), strings.TrimPrefix(server.URL, "http://"), []string{"default:wrong", "default:clickhouse"}, func(vuln VulnerabilityInfo) {
		got = &vuln
	})

	if got == nil {
		t.Fatal("expected weak password finding")
	}
	if got.ID != "clickhouse-weak-password" {
		t.Fatalf("expected clickhouse weak password finding, got %q", got.ID)
	}
	if got.Extract != "default/clickhouse" {
		t.Fatalf("expected extracted credential, got %q", got.Extract)
	}
}
