package crtsh

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestFetchNamesReturnsNamesFromCoveringCertificates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "example.com" {
			t.Fatalf("unexpected q: %s", got)
		}
		if got := r.URL.Query().Get("output"); got != "json" {
			t.Fatalf("unexpected output: %s", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("unexpected accept header: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"name_value":"example.com\nab1.com\nwww.ab1.com"},
			{"name_value":"www.example.com--com.com"},
			{"name_value":"email@example.com\n*.api.example.com"},
			{"name_value":"ab1.com"},
			{"name_value":"www.example.com\nab2.com"}
		]`))
	}))
	defer server.Close()

	names, err := fetchNames(context.Background(), server.URL, "Example.COM.", "")
	if err != nil {
		t.Fatalf("fetchNames returned error: %v", err)
	}

	want := []string{"ab1.com", "ab2.com", "api.example.com", "example.com", "www.ab1.com", "www.example.com"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("expected %v, got %v", want, names)
	}
}

func TestFetchNamesReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	if _, err := fetchNames(context.Background(), server.URL, "example.com", ""); err == nil {
		t.Fatal("expected status error")
	}
}

func TestFetchNamesRejectsNonJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<BR><BR>Unsupported output type: json</BODY></HTML>"))
	}))
	defer server.Close()

	if _, err := fetchNames(context.Background(), server.URL, "example.com", ""); err == nil {
		t.Fatal("expected decode error")
	}
}
