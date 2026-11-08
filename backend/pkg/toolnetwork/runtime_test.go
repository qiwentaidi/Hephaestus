package toolnetwork

import (
	"context"
	"errors"
	"fmt"
	"hephaestus-backend/pkg/toolguard"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRuntimeIsolationLifecycleIntegration(t *testing.T) {
	if os.Getenv("HEPHAESTUS_EGRESS_INTEGRATION") != "1" {
		t.Skip("requires Docker")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	var policyFailed atomic.Bool
	runtime := NewRuntime(ctx, root, "lifecycle", func() (toolguard.Config, error) {
		if policyFailed.Load() {
			return toolguard.Config{}, errors.New("policy offline")
		}
		return toolguard.DefaultConfig(), nil
	})
	defer runtime.Close()
	e, err := runtime.Begin(ctx, "exec", "test")
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := e.Command(ctx, "sh", []string{"-c", "sleep 60"}, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err = docker(ctx, "inspect", e.worker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker startup timed out")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Rule storage failure revokes the listener even while the process is alive.
	policyFailed.Store(true)
	deadline = time.Now().Add(8 * time.Second)
	for {
		_, err = docker(ctx, "inspect", e.gateway)
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stale gateway did not stop")
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(12 * time.Second):
		t.Fatal("worker cancellation not acknowledged")
	}
	if err = runtime.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{e.gateway, e.worker} {
		out, err := exec.Command("docker", "inspect", name).CombinedOutput()
		if err == nil || !strings.Contains(strings.ToLower(string(out)), "no such") {
			t.Fatalf("container exit unconfirmed: %s %v", out, err)
		}
	}
	if _, err = runtime.Begin(context.Background(), "exec", "late"); err == nil {
		t.Fatal("closed runtime reopened")
	}
}

func TestTaskNetworkLeaseAuditAndCancellationIntegration(t *testing.T) {
	if os.Getenv("HEPHAESTUS_EGRESS_INTEGRATION") != "1" {
		t.Skip("requires built egress image and Docker")
	}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "task-ok") })}
	go server.Serve(listener)
	defer server.Close()

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	network, err := BeginTask(ctx, root, "scan-session", "task-1", "jsscan", func() (toolguard.Config, error) {
		return toolguard.DefaultConfig(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := url.Parse(network.ProxyURL())
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}, Timeout: 10 * time.Second}
	response, err := client.Get("http://host.docker.internal:" + fmt.Sprint(listener.Addr().(*net.TCPAddr).Port) + "/ok")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", response.StatusCode)
	}
	cancel()
	if err := network.Close(); err != nil {
		t.Fatal(err)
	}
	audit := filepath.Join(root, "cmd-output", "egress-task-1.jsonl")
	if data, err := os.ReadFile(audit); err != nil || !strings.Contains(string(data), `"executionId":"task-1"`) {
		t.Fatalf("task audit missing: %v %s", err, data)
	}
}
func TestReadStateRejectsExpiryAndInvalidRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := State{Version: ProtocolVersion, Tool: "exec", Token: "token", SessionID: "s", ExecutionID: "e", Expires: time.Now().Add(time.Minute), Rules: toolguard.DefaultConfig()}
	if err := WriteState(path, state); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadState(path); err != nil {
		t.Fatal(err)
	}
	state.Expires = time.Now().Add(-time.Second)
	WriteState(path, state)
	if _, err := ReadState(path); err == nil {
		t.Fatal("expired policy accepted")
	}
	state.Expires = time.Now().Add(time.Minute)
	state.Rules.Rules[0].Pattern = "["
	WriteState(path, state)
	if _, err := ReadState(path); err == nil {
		t.Fatal("invalid policy accepted")
	}
}

func TestTaskNetworkContextAndCreationCapabilities(t *testing.T) {
	ctx := WithTaskNetwork(context.Background(), nil)
	if TaskNetworkFromContext(ctx) != nil || ProxyURLFromContext(ctx) != "" {
		t.Fatal("nil task network should not expose a proxy")
	}
	if err := (*TaskNetwork)(nil).Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeStartupFailureRetainsUnconfirmedCleanup(t *testing.T) {
	// Simulate a daemon disappearing between image inspection and resource
	// creation. The parent must retain the failure, not acknowledge stopped.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nif [ \"$1\" = image ]; then exit 0; fi\necho 'daemon unavailable' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	runtime := NewRuntime(context.Background(), t.TempDir(), "failure", func() (toolguard.Config, error) { return toolguard.DefaultConfig(), nil })
	if _, err := runtime.Begin(context.Background(), "exec", "failed-start"); !errors.Is(err, ErrStopUnconfirmed) {
		t.Fatalf("missing cleanup error: %v", err)
	}
	if err := runtime.Close(); !errors.Is(err, ErrStopUnconfirmed) {
		t.Fatalf("parent falsely acknowledged stop: %v", err)
	}
	for e := range runtime.executions {
		if _, err := os.Stat(filepath.Join(e.dir, "policy", "state.json")); !os.IsNotExist(err) {
			t.Fatal("failed execution still authorized")
		}
		os.RemoveAll(e.dir)
	}
}
