package toolnetwork

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"hephaestus-backend/pkg/toolguard"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testGateway(t *testing.T, rules *toolguard.Config) (*Gateway, *http.Client) {
	t.Helper()
	g, err := NewGateway(context.Background(), func() (State, error) {
		return State{Tool: "exec", SessionID: "s", ExecutionID: "e", Token: "secret", Rules: *rules}, nil
	}, func(map[string]any) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(g.Handler(false))
	t.Cleanup(func() { g.Close(); server.Close() })
	proxy, _ := url.Parse(server.URL)
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(g.CA.PEM)
	tr := &http.Transport{Proxy: http.ProxyURL(proxy), TLSClientConfig: &tls.Config{RootCAs: roots}, DisableKeepAlives: true}
	t.Cleanup(tr.CloseIdleConnections)
	return g, &http.Client{Transport: tr, Timeout: 5 * time.Second}
}
func TestGatewayActualRequests(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "HTTP", true: "HTTPS"}[secure], func(t *testing.T) {
			var dangerous, benign atomic.Int32
			target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/redirect") {
					http.Redirect(w, r, "/admin/system/user/remove", http.StatusTemporaryRedirect)
					return
				}
				if r.URL.Path != "/ok" {
					dangerous.Add(1)
				} else {
					benign.Add(1)
				}
				io.WriteString(w, "ok")
			}))
			if secure {
				target.StartTLS()
			} else {
				target.Start()
			}
			defer target.Close()
			rules := toolguard.DefaultConfig()
			g, client := testGateway(t, &rules)
			if secure {
				g.Transport = target.Client().Transport
			}
			for _, path := range []string{"/admin/system/user/remove", "/admin/system/user/%72emove", "/redirect"} {
				response, err := client.Get(target.URL + path)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 403 {
					t.Fatalf("%s: %d", path, response.StatusCode)
				}
			}
			response, err := client.Get(target.URL + "/ok")
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 200 || benign.Load() != 1 || dangerous.Load() != 0 {
				t.Fatalf("status=%d benign=%d dangerous=%d", response.StatusCode, benign.Load(), dangerous.Load())
			}
		})
	}
}
func TestGatewayBodiesAndFailures(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(200) }))
	defer target.Close()
	rules := toolguard.DefaultConfig()
	g, client := testGateway(t, &rules)
	cases := []struct{ kind, encoding, body string }{
		{"application/json", "", `{"action":"remove"}`},
		{"application/json", "", `{"action":"remove","action":"read"}`},
		{"application/json", "", `{"action":"\u0072emove"}`},
		{"application/x-www-form-urlencoded", "", "action=remove&action=read"},
		{"text/plain", "br", "opaque"},
		{"application/octet-stream", "", "opaque"},
		{"application/json", "gzip", `{"operation":"remove"}`},
		{"multipart/form-data; boundary=x", "", "--x\r\nContent-Disposition: form-data; name=\"action\"\r\n\r\nremove\r\n--x--\r\n"},
		{"text/plain", "", strings.Repeat("x", MaxBody+1)},
	}
	for _, tc := range cases {
		t.Run(tc.kind+tc.encoding, func(t *testing.T) {
			body := []byte(tc.body)
			if tc.encoding == "gzip" {
				var buf bytes.Buffer
				z := gzip.NewWriter(&buf)
				z.Write(body)
				z.Close()
				body = buf.Bytes()
			}
			req, _ := http.NewRequest("POST", target.URL+"/ok", bytes.NewReader(body))
			req.Header.Set("Content-Type", tc.kind)
			req.Header.Set("Content-Encoding", tc.encoding)
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != 403 {
				t.Fatalf("status %d", res.StatusCode)
			}
		})
	}
	if hits.Load() != 0 {
		t.Fatalf("blocked requests reached target: %d", hits.Load())
	}
	// Existing disabled/deleted rules remain authoritative, no target whitelist.
	rules.Enabled = false
	res, err := client.Get(target.URL + "/remove")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	g.Audit = func(map[string]any) error { return errors.New("disk unavailable") }
	res, err = client.Get(target.URL + "/ok")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 || hits.Load() != 1 {
		t.Fatal("audit failure forwarded")
	}
	g.State = func() (State, error) { return State{}, errors.New("expired") }
	res, err = client.Get(target.URL + "/ok")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 || hits.Load() != 1 {
		t.Fatal("expired policy forwarded")
	}
}
func TestGatewayCustomScope(t *testing.T) {
	rules := toolguard.Config{Enabled: true, Rules: []toolguard.Rule{{ID: "custom", Name: "custom", Enabled: true, Tools: []string{"execute-python-script"}, Target: "http_method", Pattern: "custom-block", Message: "blocked"}}}
	g, client := testGateway(t, &rules)
	g.State = func() (State, error) { return State{Tool: "execute-python-script", Rules: rules}, nil }
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer target.Close()
	res, err := client.Get(target.URL + "/custom-block")
	if err != nil {
		t.Fatal(err)
	}
	if res.Header.Get("X-Hephaestus-Egress-Rule-ID") != "custom" || res.Header.Get("X-Hephaestus-Egress-Rule-Name") != "custom" || res.Header.Get("X-Hephaestus-Egress-Matched-Text") != "custom-block" {
		t.Fatalf("blocked rule metadata missing: headers=%v", res.Header)
	}
	res.Body.Close()
	if res.StatusCode != 403 || hits.Load() != 0 {
		t.Fatal("original tool scope was lost")
	}
}

func TestGatewayTunnelAndAuthentication(t *testing.T) {
	rules := toolguard.DefaultConfig()
	g, _ := testGateway(t, &rules)
	var hits atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer target.Close()
	g.Transport = target.Client().Transport
	proxy := httptest.NewServer(g.Handler(true))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	plain := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}, Timeout: time.Second}
	res, err := plain.Get("http://example.invalid/ok")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("unauthenticated proxy allowed")
	}
	proxyWithTaskCredential := *proxyURL
	proxyWithTaskCredential.User = url.UserPassword("Bearer", "secret")
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(g.CA.PEM)
	authenticated := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(&proxyWithTaskCredential), TLSClientConfig: &tls.Config{RootCAs: roots}}, Timeout: time.Second}
	res, err = authenticated.Get(target.URL + "/ok")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("task proxy credential rejected: %d", res.StatusCode)
	}
	hits.Store(0)
	for _, payload := range []string{
		"GET /remove HTTP/1.1\r\nHost: %s\r\n\r\n",
		"GET /ok HTTP/1.1\r\nHost: other.invalid\r\n\r\n",
		"GET https://other.invalid/ok HTTP/1.1\r\nHost: %s\r\n\r\n",
		"CONNECT other.invalid:443 HTTP/1.1\r\nHost: other.invalid:443\r\n\r\n",
		"GET /ok HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n",
	} {
		c, err := net.DialTimeout("tcp", proxyURL.Host, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		c.SetDeadline(time.Now().Add(3 * time.Second))
		authority := strings.TrimPrefix(target.URL, "https://")
		fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Bearer secret\r\n\r\n", authority, authority)
		response, err := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: "CONNECT"})
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 200 {
			t.Fatal(response.StatusCode)
		}
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM(g.CA.PEM)
		secure := tls.Client(c, &tls.Config{RootCAs: roots, ServerName: "127.0.0.1"})
		if err = secure.Handshake(); err != nil {
			t.Fatal(err)
		}
		io.WriteString(secure, strings.ReplaceAll(payload, "%s", authority))
		response, err = http.ReadResponse(bufio.NewReader(secure), &http.Request{Method: "GET"})
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 403 {
			t.Fatal(response.StatusCode)
		}
		response.Body.Close()
		secure.Close()
	}
	if hits.Load() != 0 {
		t.Fatal("tunnel bypass reached target")
	}
}
