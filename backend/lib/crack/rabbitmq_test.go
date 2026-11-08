package crack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRabbitMQUnauth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/overview" {
			t.Fatalf("expected /api/overview, got %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"rabbitmq_version":"3.13.0"}`))
	}))
	defer server.Close()

	flag, response, err := RabbitMQUnauth(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("expected rabbitmq unauthorized check to succeed, got error: %v", err)
	}
	if !flag {
		t.Fatal("expected rabbitmq unauthorized check to return true")
	}
	if !strings.Contains(response, "rabbitmq_version") {
		t.Fatalf("expected rabbitmq overview response, got %q", response)
	}
}

func TestRabbitMQHTTPAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "guest" || pass != "guest" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"management_version":"3.13.0"}`))
	}))
	defer server.Close()

	address := strings.TrimPrefix(server.URL, "http://")
	flag, err := RabbitMQConn(address, "guest", "guest")
	if err != nil {
		t.Fatalf("expected rabbitmq http auth to succeed, got error: %v", err)
	}
	if !flag {
		t.Fatal("expected rabbitmq http auth to return true")
	}

	flag, err = RabbitMQConn(address, "guest", "wrong")
	if err == nil {
		t.Fatal("expected wrong rabbitmq http password to return an error")
	}
	if flag {
		t.Fatal("expected wrong rabbitmq http password to return false")
	}
}

func TestRabbitMQScanReportsHTTPWeakPassword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		user, pass, _ := r.BasicAuth()
		if user != "guest" || pass != "guest" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"rabbitmq_version":"3.13.0"}`))
	}))
	defer server.Close()

	var got *VulnerabilityInfo
	RabbitMQScan(context.Background(), strings.TrimPrefix(server.URL, "http://"), []string{"guest:wrong", "guest:guest"}, func(vuln VulnerabilityInfo) {
		got = &vuln
	})

	if got == nil {
		t.Fatal("expected rabbitmq weak password finding")
	}
	if got.ID != "rabbitmq-weak-password" {
		t.Fatalf("expected rabbitmq weak password finding, got %q", got.ID)
	}
	if got.Extract != "guest/guest" {
		t.Fatalf("expected extracted credential, got %q", got.Extract)
	}
}
