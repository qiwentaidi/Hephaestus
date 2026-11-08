package ai

import (
	"context"
	"fmt"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcptools"
	"hephaestus-backend/pkg/toolguard"
	"hephaestus-backend/pkg/toolnetwork"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestNativeToolsDoNotUseEgressWhitelist(t *testing.T) {
	// EnforceEgress is retained as an explicit low-level opt-in for callers
	// that provide a runtime. It must not turn native business tools into an
	// implicit allowlist when the normal AI handler leaves it disabled.
	cfg := pentest.AgentLoopConfig{EnforceEgress: true}
	_, handled, err := mcptools.RunTool(context.Background(), cfg, pentest.AgentToolCall{Function: pentest.AgentFunctionCall{
		Name:      "space_engine_search",
		Arguments: map[string]any{"query": "example.com"},
	}})
	if !handled {
		t.Fatal("space engine search was not dispatched")
	}
	if _, ok := toolguard.AsBlocked(err); ok {
		t.Fatalf("space engine search was incorrectly blocked by egress: %v", err)
	}
	_, handled, err = mcptools.RunTool(context.Background(), cfg, pentest.AgentToolCall{Function: pentest.AgentFunctionCall{
		Name:      "dns_lookup",
		Arguments: map[string]any{"host": "example.com", "recordType": "A"},
	}})
	if !handled {
		t.Fatal("dns lookup was not dispatched")
	}
	if _, ok := toolguard.AsBlocked(err); ok {
		t.Fatalf("dns lookup was incorrectly blocked by egress: %v", err)
	}
	// These are business tools with database side effects. Use validation-failing
	// arguments here so this egress-routing test cannot create real scan tasks
	// through the process-global database connection.
	for name, args := range map[string]map[string]any{
		"attack_surface_create_webscan_task":  {"targets": []any{}},
		"attack_surface_create_jsscan_task":   {"targets": []any{}},
		"attack_surface_create_dirscan_task":  {"targets": []any{}},
		"attack_surface_create_portscan_task": {"targets": []any{}},
		"subdomain_create_task":               {"domains": []any{}},
	} {
		_, handled, err := mcptools.RunTool(context.Background(), cfg, pentest.AgentToolCall{Function: pentest.AgentFunctionCall{Name: name, Arguments: args}})
		if !handled {
			t.Fatalf("%s was not dispatched: err=%v", name, err)
		}
		if _, ok := toolguard.AsBlocked(err); ok {
			t.Fatalf("%s was incorrectly blocked before task creation: %v", name, err)
		}
		if err == nil {
			t.Fatalf("%s unexpectedly created a task from empty targets", name)
		}
	}
}

func TestNativeHTTPEgressIntegration(t *testing.T) {
	if os.Getenv("HEPHAESTUS_EGRESS_INTEGRATION") != "1" {
		t.Skip("requires built gateway image and Docker")
	}
	var accepted, blocked atomic.Int32
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gateway-only-deny" {
			blocked.Add(1)
		} else {
			accepted.Add(1)
		}
		w.WriteHeader(200)
	}))
	target.Listener.Close()
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	target.Listener = listener
	target.Start()
	defer target.Close()
	base := fmt.Sprintf("http://host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	// This custom rule exists only in the gateway's live policy, proving the
	// native request was inspected there instead of merely by static toolguard.
	rules := toolguard.Config{Enabled: true, Rules: []toolguard.Rule{{ID: "gateway-only", Name: "test", Enabled: true, Target: "http_method", Pattern: "gateway-only-deny"}}}
	runtime := toolnetwork.NewRuntime(context.Background(), root, "native-test", func() (toolguard.Config, error) { return rules, nil })
	defer runtime.Close()
	cfg := pentest.AgentLoopConfig{EnforceEgress: true, Network: runtime, ToolExecutionID: "native-request"}
	for _, path := range []string{"/ok", "/gateway-only-deny"} {
		_, handled, err := mcptools.RunNative(context.Background(), cfg, pentest.AgentToolCall{Function: pentest.AgentFunctionCall{Name: "http-framework-test", Arguments: map[string]any{"url": base + path, "proxy": "http://127.0.0.1:1"}}})
		if !handled {
			t.Fatal("not handled")
		}
		if path == "/ok" && err != nil {
			t.Fatal(err)
		}
		if path != "/ok" {
			if _, ok := toolguard.AsBlocked(err); !ok {
				t.Fatalf("request not blocked: %v", err)
			}
		}
	}
	if accepted.Load() != 1 || blocked.Load() != 0 {
		t.Fatalf("target accepted=%d blocked=%d", accepted.Load(), blocked.Load())
	}
}
