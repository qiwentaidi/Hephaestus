package capabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcpclient"
	"hephaestus-backend/pkg/mcptools"
)

type fakeBridge struct {
	schemas []pentest.AgentToolSchema
	calls   []string
}

func (f *fakeBridge) NativeSchemas() []pentest.AgentToolSchema { return f.schemas }
func (f *fakeBridge) RunNative(_ context.Context, _ pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error) {
	f.calls = append(f.calls, call.Function.Name)
	return map[string]any{"ok": call.Function.Name}, true, nil
}
func (f *fakeBridge) ConversationHasJSScanAsset(string) bool { return false }

type fakeProvider struct {
	servers []mcpclient.Server
}

func (f *fakeProvider) LoadServers() ([]mcpclient.Server, error) { return f.servers, nil }
func (f *fakeProvider) ExternalSchemas([]mcpclient.Server) []pentest.AgentToolSchema {
	return nil
}
func (f *fakeProvider) Authorize(string) error                      { return nil }
func (f *fakeProvider) RefreshTools(*pentest.AgentLoopConfig) error { return nil }
func (f *fakeProvider) RunExternal(context.Context, pentest.AgentLoopConfig, pentest.AgentToolCall) (any, bool, error) {
	return nil, true, nil
}

func toolSchema(name string) pentest.AgentToolSchema {
	return pentest.AgentToolSchema{
		Type:     "function",
		Function: pentest.AgentToolFunction{Name: name, Description: name, Parameters: map[string]any{"type": "object"}},
	}
}

func TestToolsFiltersNativeByRuntimeAndRole(t *testing.T) {
	bridge := &fakeBridge{schemas: []pentest.AgentToolSchema{
		toolSchema("exec"), toolSchema("Bash"),
		toolSchema("subdomain_create_task"), toolSchema("attack_surface_create_webscan_task"),
		toolSchema("nuclei"),
		toolSchema("attack_surface_read_reference"),
		toolSchema("retest_vulnerability"), toolSchema("record_vulnerability_retest"),
	}}
	provider := &fakeProvider{}
	mcptools.RegisterPentestBridge(bridge)
	mcptools.RegisterServerProvider(provider)
	t.Cleanup(func() {
		mcptools.RegisterPentestBridge(bridge)
		mcptools.RegisterServerProvider(provider)
	})

	worker := toolNames(Tools(context.Background(), RuntimeEngine, "worker"))
	for _, want := range []string{"subdomain_create_task", "attack_surface_create_webscan_task", "nuclei", "retest_vulnerability", "record_vulnerability_retest"} {
		if !worker[want] {
			t.Fatalf("worker is missing %s: %v", want, worker)
		}
	}
	if worker["exec"] || worker["Bash"] {
		t.Fatalf("engine must not receive the standalone shell tools: %v", worker)
	}
	mainTools := Tools(context.Background(), RuntimeEngine, "mainagent")
	for _, name := range []string{"retest_vulnerability", "record_vulnerability_retest"} {
		if !toolNames(mainTools)[name] {
			t.Fatalf("main agent is missing %s", name)
		}
	}
	foundReference := false
	for _, tool := range mainTools {
		if tool.Name == "attack_surface_read_reference" {
			foundReference = true
			if !tool.ReadOnly || !tool.Concurrent {
				t.Fatal("reference reader must be read only")
			}
		}
	}
	if !foundReference {
		t.Fatal("main agent is missing the reference reader")
	}

	// Roles differ only through their visibility binding now: with no binding
	// installed every engine role sees the same engine set, and the tool page
	// decides what a Planner/Controller may actually hold.
	planner := toolNames(Tools(context.Background(), RuntimeEngine, "planner"))
	for _, want := range []string{"subdomain_create_task", "attack_surface_create_webscan_task", "nuclei", "retest_vulnerability", "record_vulnerability_retest"} {
		if !planner[want] {
			t.Fatalf("planner is missing %s in the unbound engine set: %v", want, planner)
		}
	}
}

func TestMCPToolsAreGenericAndRoleFiltered(t *testing.T) {
	provider := &fakeProvider{servers: []mcpclient.Server{{
		ID: "srv-1", Name: "demo", Enabled: true,
		Tools: []mcpclient.Tool{{Name: "search", Description: "search", InputSchema: map[string]any{"type": "object"}}},
	}}}
	mcptools.RegisterServerProvider(provider)
	previousVisibility := ToolDisabled
	ToolDisabled = func(_ context.Context, role string) (map[string]bool, bool, error) {
		if NormalizeRole(role) == "planner" {
			return map[string]bool{mcpclient.Alias("srv-1", "search"): true}, true, nil
		}
		return nil, false, nil
	}
	t.Cleanup(func() { ToolDisabled = previousVisibility })

	worker := Tools(context.Background(), RuntimeEngine, "worker")
	alias := ""
	for _, tool := range worker {
		if tool.Kind == KindMCP {
			alias = tool.Name
		}
	}
	if alias == "" {
		t.Fatal("worker did not receive the MCP tool")
	}
	planner := Tools(context.Background(), RuntimeEngine, "planner")
	for _, tool := range planner {
		if tool.Kind == KindMCP {
			t.Fatalf("planner received MCP tool %s", tool.Name)
		}
	}

	previous := callMCPServer
	callMCPServer = func(_ context.Context, server mcpclient.Server, name string, args map[string]any) (map[string]any, error) {
		if server.ID != "srv-1" || name != "search" || args["q"] != "x" {
			t.Fatalf("unexpected MCP call %s/%s %v", server.ID, name, args)
		}
		return map[string]any{"ok": true}, nil
	}
	t.Cleanup(func() { callMCPServer = previous })

	tool := Tool{Kind: KindMCP, ServerID: "srv-1", ServerName: "demo", RemoteName: "search", Name: alias}
	value, err := Call(context.Background(), Host{}, tool, map[string]any{"q": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if value.(map[string]any)["ok"] != true {
		t.Fatalf("unexpected result %#v", value)
	}
}

func TestOpenToolsSharesOneMCPClientAndUsesCachedTools(t *testing.T) {
	var initialize, calls, lists, deletes atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes.Add(1)
			w.WriteHeader(204)
			return
		}
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			initialize.Add(1)
			w.Header().Set("Mcp-Session-Id", "open-tools-session")
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26","capabilities":{"tools":{}}}}`)
		case "notifications/initialized":
			w.WriteHeader(202)
		case "tools/list":
			lists.Add(1)
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`)
		case "tools/call":
			calls.Add(1)
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":"ok"}]}}`, req.ID)
		default:
			t.Errorf("unexpected method %s", req.Method)
		}
	}))
	defer remote.Close()

	provider := &fakeProvider{servers: []mcpclient.Server{{
		ID: "srv-open", Name: "demo", Enabled: true, URL: remote.URL,
		Tools: []mcpclient.Tool{
			{Name: "echo", Description: "echo", InputSchema: map[string]any{"type": "object"}},
			{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}},
		},
	}}}
	mcptools.RegisterServerProvider(provider)
	previousVisibility := ToolDisabled
	ToolDisabled = func(context.Context, string) (map[string]bool, bool, error) { return nil, false, nil }
	t.Cleanup(func() {
		ToolDisabled = previousVisibility
		mcptools.RegisterServerProvider(provider)
	})

	tools, cleanup, err := OpenTools(context.Background(), RuntimeEngine, "worker")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	var mcpTools []Tool
	for _, tool := range tools {
		if tool.Kind == KindMCP {
			mcpTools = append(mcpTools, tool)
		}
	}
	if len(mcpTools) != 2 {
		t.Fatalf("mcp tools = %#v", mcpTools)
	}
	if mcpTools[0].Client == nil || mcpTools[0].Client != mcpTools[1].Client {
		t.Fatal("tools from one server must share one persistent client")
	}
	for _, tool := range mcpTools {
		if _, err := Call(context.Background(), Host{}, tool, map[string]any{"n": 1}); err != nil {
			t.Fatal(err)
		}
	}
	if initialize.Load() != 1 || calls.Load() != 2 {
		t.Fatalf("initialize=%d calls=%d", initialize.Load(), calls.Load())
	}
	if lists.Load() != 0 {
		t.Fatalf("OpenTools must use the cached tool list, got %d tools/list calls", lists.Load())
	}
}

func TestNativeCallUsesHostContext(t *testing.T) {
	bridge := &fakeBridge{schemas: []pentest.AgentToolSchema{toolSchema("subdomain_create_task")}}
	mcptools.RegisterPentestBridge(bridge)
	tool := Tool{Kind: KindNative, Name: "subdomain_create_task"}
	if _, err := Call(context.Background(), Host{TaskID: 7}, tool, map[string]any{"domain": "example.test"}); err != nil {
		t.Fatal(err)
	}
	if len(bridge.calls) != 1 || bridge.calls[0] != "subdomain_create_task" {
		t.Fatalf("native dispatch = %v", bridge.calls)
	}
}

func toolNames(tools []Tool) map[string]bool {
	out := map[string]bool{}
	for _, tool := range tools {
		out[tool.Name] = true
	}
	return out
}

func TestRoleToolAllowlistFallbackBindsVisibleMCPForEveryRole(t *testing.T) {
	provider := &fakeProvider{servers: []mcpclient.Server{{
		ID: "srv-first", Name: "First", Enabled: true,
		Tools: []mcpclient.Tool{
			{Name: "miniapp_get_info", Description: "info", InputSchema: map[string]any{"type": "object"}},
			{Name: "hidden_tool", Description: "hidden", InputSchema: map[string]any{"type": "object"}},
		},
	}}}
	mcptools.RegisterServerProvider(provider)
	previousVisibility := ToolDisabled
	ToolDisabled = func(_ context.Context, role string) (map[string]bool, bool, error) {
		return map[string]bool{mcpclient.Alias("srv-first", "hidden_tool"): true}, true, nil
	}
	t.Cleanup(func() { ToolDisabled = previousVisibility })

	contains := func(names []string, want string) bool {
		for _, name := range names {
			if name == want {
				return true
			}
		}
		return false
	}
	visible := mcpclient.Alias("srv-first", "miniapp_get_info")
	hidden := mcpclient.Alias("srv-first", "hidden_tool")

	// The fallback (host registry not wired) still honours the per-role binding
	// and keeps the literal name resolution consistent for every role.
	for _, role := range []string{pentest.AgentRoleWorker, pentest.AgentRolePlanner, pentest.AgentRoleController, pentest.AgentRolePentest} {
		names := RoleToolAllowlist(context.Background(), role)
		if !contains(names, NoToolsSentinel) {
			t.Fatalf("%s allowlist must stay restrictive without tools", role)
		}
		if !contains(names, visible) {
			t.Fatalf("%s allowlist missing visible MCP tool: %v", role, names)
		}
		if contains(names, hidden) {
			t.Fatalf("%s allowlist kept hidden MCP tool", role)
		}
	}
}
