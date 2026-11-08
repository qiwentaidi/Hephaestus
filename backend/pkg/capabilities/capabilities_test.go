package capabilities

import (
	"context"
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
	}}
	provider := &fakeProvider{}
	mcptools.RegisterPentestBridge(bridge)
	mcptools.RegisterServerProvider(provider)
	t.Cleanup(func() {
		mcptools.RegisterPentestBridge(bridge)
		mcptools.RegisterServerProvider(provider)
	})

	worker := toolNames(Tools(context.Background(), RuntimeEngine, "worker"))
	for _, want := range []string{"subdomain_create_task", "attack_surface_create_webscan_task", "nuclei"} {
		if !worker[want] {
			t.Fatalf("worker is missing %s: %v", want, worker)
		}
	}
	if worker["exec"] || worker["Bash"] {
		t.Fatalf("engine must not receive the standalone shell tools: %v", worker)
	}
	mainTools := Tools(context.Background(), RuntimeEngine, "mainagent")
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

	planner := toolNames(Tools(context.Background(), RuntimeEngine, "planner"))
	if !planner["subdomain_create_task"] {
		t.Fatalf("planner is missing discovery: %v", planner)
	}
	if planner["attack_surface_create_webscan_task"] || planner["nuclei"] {
		t.Fatalf("planner must not launch scans: %v", planner)
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
