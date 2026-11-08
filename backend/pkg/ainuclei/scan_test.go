package ainuclei

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	nuclei "github.com/projectdiscovery/nuclei/v3/lib"
)

const testTemplate = `id: ai-sdk-test
info:
  name: AI SDK local integration test
  author: hephaestus
  severity: info
  tags: ai-sdk,local-test
http:
  - method: GET
    path:
      - "{{BaseURL}}/{{probe_path}}"
    matchers:
      - type: word
        words:
          - "sdk-evidence-marker"
`

func scanFixture(t *testing.T, target string) (string, Options) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(testTemplate), 0600); err != nil {
		t.Fatal(err)
	}
	oob, retries := false, 0
	return dir, Options{Target: target, Templates: []string{path}, Variables: []string{"probe_path=probe"}, OOB: &oob, Retries: &retries, Concurrency: 1, RateLimit: 100, Timeout: 2}
}

func TestRunSDKCapturesEvidenceAndFilters(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/probe" || r.Header.Get("X-AI-Test") != "session-value" {
			t.Errorf("request parameters lost: %s %s", r.URL.Path, r.Header.Get("X-AI-Test"))
		}
		fmt.Fprint(w, "sdk-evidence-marker")
	}))
	defer server.Close()
	dir, args := scanFixture(t, server.URL)
	args.Headers = []string{"X-AI-Test: session-value"}
	args.Tags = "ai-sdk"
	result, err := Run(context.Background(), dir, args)
	if err != nil {
		t.Fatal(err)
	}
	if result.LoadedTemplates != 1 || result.Matches != 1 || result.Requests < 1 || requests.Load() != 1 || result.RequestErrors != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	data, err := os.ReadFile(result.OutputFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sdk-evidence-marker", "session-value", "sha256", "ai-sdk-test", `"kind":"summary"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s from evidence: %s", want, data)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if !json.Valid([]byte(line)) {
			t.Fatalf("invalid evidence JSONL: %s", line)
		}
	}
	args.Severity = "critical"
	filtered, err := Run(context.Background(), dir, args)
	if !errors.Is(err, nuclei.ErrNoTemplatesAvailable) || filtered.Status != "no_templates" || requests.Load() != 1 {
		t.Fatalf("filtered templates must not scan: %+v %v requests=%d", filtered, err, requests.Load())
	}
}

func TestRunSDKFlowPreservesExtractedValuesAndEvidence(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/token":
			fmt.Fprint(w, "token=flow-session-value")
		case "/probe/flow-session-value":
			fmt.Fprint(w, "sdk-evidence-marker")
		default:
			t.Errorf("flow lost extracted value: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	dir, args := scanFixture(t, server.URL)
	template := `id: ai-sdk-flow-test
info:
  name: AI SDK flow integration test
  author: hephaestus
  severity: info
flow: http(1) && http(2)
http:
  - method: GET
    path:
      - "{{BaseURL}}/token"
    matchers:
      - type: word
        words: ["token="]
        internal: true
    extractors:
      - type: regex
        name: session_token
        group: 1
        regex: ['token=([a-z-]+)']
        internal: true
  - method: GET
    path:
      - "{{BaseURL}}/probe/{{session_token}}"
    matchers:
      - type: dsl
        dsl: ["status_code == 200 && contains(body, 'sdk-evidence-marker')"]
`
	if err := os.WriteFile(args.Templates[0], []byte(template), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), dir, args)
	if err != nil {
		t.Fatal(err)
	}
	if result.LoadedTemplates != 1 || result.Matches != 1 || requests.Load() != 2 || result.RequestErrors != 0 {
		t.Fatalf("unexpected flow result: %+v requests=%d", result, requests.Load())
	}
	data, err := os.ReadFile(result.OutputFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ai-sdk-flow-test", "sdk-evidence-marker", "/probe/flow-session-value"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s from flow evidence: %s", want, data)
		}
	}
}

func TestRunSDKNativeJavascriptNetworkProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		request := make([]byte, 4)
		_, err = io.ReadFull(conn, request)
		if err == nil && string(request) != "ping" {
			err = fmt.Errorf("unexpected probe: %q", request)
		}
		if err == nil {
			_, err = io.WriteString(conn, "sdk-evidence-marker")
		}
		done <- err
	}()
	dir, args := scanFixture(t, "http://"+listener.Addr().String())
	template := `id: ai-sdk-native-js-test
info:
  name: AI SDK native JS local integration test
  author: hephaestus
  severity: info
javascript:
  - args:
      address: "{{Hostname}}"
    code: |
      const net = require('nuclei/net');
      const conn = net.Open('tcp', address);
      conn.Send('ping');
      Export(conn.RecvFullString(19));
      conn.Close();
      true;
    matchers:
      - type: word
        words: ["sdk-evidence-marker"]
`
	if err := os.WriteFile(args.Templates[0], []byte(template), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), dir, args)
	if err != nil || result.Matches != 1 || result.RequestErrors != 0 {
		t.Fatalf("native JS network scan failed: %+v error=%v", result, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("native module did not complete the local TCP exchange")
	}
	data, err := os.ReadFile(result.OutputFile)
	if err != nil || !strings.Contains(string(data), "sdk-evidence-marker") || !strings.Contains(string(data), "ai-sdk-native-js-test") {
		t.Fatalf("native JS evidence lost: %s error=%v", data, err)
	}
}

func TestRunSDKCancellationWaitsForWorkers(t *testing.T) {
	started := make(chan struct{}, 1)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	dir, args := scanFixture(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type completion struct {
		result Result
		err    error
	}
	done := make(chan completion, 1)
	go func() { result, err := Run(ctx, dir, args); done <- completion{result, err} }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("SDK did not start request")
	}
	cancel()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) || got.result.Status != "cancelled" {
			t.Fatalf("cancel not propagated: %+v %v", got.result, got.err)
		}
		before, _ := os.ReadFile(got.result.OutputFile)
		count := requests.Load()
		time.Sleep(150 * time.Millisecond)
		after, _ := os.ReadFile(got.result.OutputFile)
		if string(before) != string(after) || count != requests.Load() {
			t.Fatal("SDK activity continued after cancellation acknowledged")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SDK did not acknowledge cancellation")
	}
}

func TestRunSDKRequestErrorsAreNotCleanResults(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	target := "http://" + listener.Addr().String()
	listener.Close()
	dir, args := scanFixture(t, target)
	result, err := Run(context.Background(), dir, args)
	if err != nil {
		t.Fatal(err)
	}
	if result.RequestErrors == 0 || result.Status != "completed_with_errors" || result.Matches != 0 {
		t.Fatalf("network failure appeared clean: %+v", result)
	}
}

func TestRunSDKInvalidAndMissingTemplates(t *testing.T) {
	dir, args := scanFixture(t, "http://127.0.0.1:1")
	if err := os.WriteFile(args.Templates[0], []byte("invalid: ["), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), dir, args)
	if err == nil || result.Status != "no_templates" || result.Requests != 0 {
		t.Fatalf("invalid template appeared successful: %+v %v", result, err)
	}
	args.Templates = []string{filepath.Join(dir, "missing.yaml")}
	result, err = Run(context.Background(), dir, args)
	if err == nil || result.Status != "failed" {
		t.Fatalf("missing path ignored: %+v %v", result, err)
	}
}

func TestRunSDKConcurrentSessionsKeepEvidenceIsolated(t *testing.T) {
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Session") != strings.TrimPrefix(r.URL.Path, "/") {
			t.Errorf("cross-session request: %s %s", r.URL.Path, r.Header.Get("X-Session"))
		}
		arrived <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		fmt.Fprint(w, "sdk-evidence-marker")
	}))
	defer server.Close()
	type completion struct {
		name   string
		result Result
		err    error
	}
	done := make(chan completion, 2)
	for _, name := range []string{"session-alpha", "session-beta"} {
		dir, args := scanFixture(t, server.URL)
		args.Headers = []string{"X-Session: " + name}
		args.Variables = []string{"probe_path=" + name}
		args.Timeout = 10
		go func() { result, err := Run(context.Background(), dir, args); done <- completion{name, result, err} }()
	}
	for range 2 {
		select {
		case <-arrived:
		case <-time.After(10 * time.Second):
			close(release)
			t.Fatal("concurrent SDK requests did not start")
		}
	}
	close(release)
	for range 2 {
		got := <-done
		if got.err != nil || got.result.Matches != 1 {
			t.Fatalf("concurrent SDK failure: %+v %v", got.result, got.err)
		}
		data, err := os.ReadFile(got.result.OutputFile)
		if err != nil {
			t.Fatal(err)
		}
		other := "session-alpha"
		if got.name == other {
			other = "session-beta"
		}
		if !strings.Contains(string(data), got.name) || strings.Contains(string(data), other) {
			t.Fatal("session evidence crossed scans")
		}
	}
}
