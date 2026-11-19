package crtsh

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
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

func TestFetchNamesAugmentsMatchingIdentitiesWithCertificateSANs(t *testing.T) {
	certPEM := testCertificatePEM(t, "www.example.org", []string{
		"www.example.org",
		"example.net",
		"example.edu",
		"example.com",
		"example.org",
		"www.example.com",
		"www.example.edu",
		"www.example.net",
	})
	var certFetches int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("d") != "" {
			atomic.AddInt32(&certFetches, 1)
			w.Header().Set("Content-Type", "application/x-pem-file")
			_, _ = w.Write(certPEM)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":1,"common_name":"www.example.org","name_value":"example.com\nwww.example.com"},
			{"id":2,"common_name":"www.example.org","name_value":"example.com\nwww.example.com"}
		]`))
	}))
	defer server.Close()

	names, err := fetchNames(context.Background(), server.URL, "example.com", "")
	if err != nil {
		t.Fatalf("fetchNames returned error: %v", err)
	}

	want := []string{
		"example.com",
		"example.edu",
		"example.net",
		"example.org",
		"www.example.com",
		"www.example.edu",
		"www.example.net",
		"www.example.org",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("expected %v, got %v", want, names)
	}
	if got := atomic.LoadInt32(&certFetches); got != 1 {
		t.Fatalf("expected one certificate fetch for the deduplicated group, got %d", got)
	}
}

func TestFetchNamesFallsBackToMatchingIdentitiesWhenCertificateFetchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("d") != "" {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"common_name":"example.com","name_value":"example.com\nwww.example.com"}]`))
	}))
	defer server.Close()

	names, err := fetchNames(context.Background(), server.URL, "example.com", "")
	if err != nil {
		t.Fatalf("fetchNames returned error: %v", err)
	}

	want := []string{"example.com", "www.example.com"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("expected %v, got %v", want, names)
	}
}

func TestFetchNamesIncludesCertificateCommonName(t *testing.T) {
	certPEM := testCertificatePEM(t, "cn-only.example", []string{"example.com"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("d") != "" {
			_, _ = w.Write(certPEM)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":2,"common_name":"cn-only.example","name_value":"example.com"}]`))
	}))
	defer server.Close()

	names, err := fetchNames(context.Background(), server.URL, "example.com", "")
	if err != nil {
		t.Fatalf("fetchNames returned error: %v", err)
	}

	want := []string{"cn-only.example", "example.com"}
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

func testCertificatePEM(t *testing.T, commonName string, dnsNames []string) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
