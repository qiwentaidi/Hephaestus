package mcptools

import (
	"context"
	"errors"
	"testing"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcpclient"
)

type fakeProvider struct {
	authorized string
	ran        string
}

func (f *fakeProvider) LoadServers() ([]mcpclient.Server, error) {
	return []mcpclient.Server{{ID: "srv", Name: "srv", Enabled: true}}, nil
}

func (f *fakeProvider) ExternalSchemas(servers []mcpclient.Server) []pentest.AgentToolSchema {
	return []pentest.AgentToolSchema{{Function: pentest.AgentToolFunction{Name: "external_mcp_x"}}}
}

func (f *fakeProvider) Authorize(tool string) error {
	f.authorized = tool
	return nil
}

func (f *fakeProvider) RefreshTools(cfg *pentest.AgentLoopConfig) error {
	cfg.ToolAllowlist = []string{"refreshed"}
	return nil
}

func (f *fakeProvider) RunExternal(ctx context.Context, cfg pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error) {
	f.ran = "external:" + call.Function.Name
	return "external", true, nil
}

type fakeBridge struct {
	ran string
}

func (fakeBridge) NativeSchemas() []pentest.AgentToolSchema {
	return []pentest.AgentToolSchema{{Function: pentest.AgentToolFunction{Name: "native"}}}
}

func (f *fakeBridge) RunNative(ctx context.Context, cfg pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error) {
	f.ran = "native:" + call.Function.Name
	return "native", true, nil
}

func (fakeBridge) ConversationHasJSScanAsset(id string) bool { return id == "conv-js" }

func TestUnconfiguredRegistryFailsClosed(t *testing.T) {
	provider, bridge = nil, nil
	if _, err := Provider(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
	if _, err := Bridge(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
	if _, handled, err := RunTool(context.Background(), pentest.AgentLoopConfig{}, pentest.AgentToolCall{}); !handled || !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("RunTool should fail closed, got handled=%v err=%v", handled, err)
	}
}

func TestRegistryDispatchesBothSides(t *testing.T) {
	p, b := &fakeProvider{}, &fakeBridge{}
	RegisterServerProvider(p)
	RegisterPentestBridge(b)
	t.Cleanup(func() { provider, bridge = nil, nil })

	servers, err := LoadServers()
	if err != nil || len(servers) != 1 {
		t.Fatalf("LoadServers: %v %#v", err, servers)
	}
	if schemas := ExternalSchemas(servers); len(schemas) != 1 || schemas[0].Function.Name != "external_mcp_x" {
		t.Fatalf("ExternalSchemas: %#v", schemas)
	}
	if err := AuthorizeTool("nuclei"); err != nil || p.authorized != "nuclei" {
		t.Fatalf("Authorize: %v %q", err, p.authorized)
	}
	cfg := &pentest.AgentLoopConfig{}
	if err := RefreshTools(cfg); err != nil || len(cfg.ToolAllowlist) != 1 {
		t.Fatalf("RefreshTools: %v %#v", err, cfg.ToolAllowlist)
	}
	if schemas := NativeSchemas(); len(schemas) != 1 || schemas[0].Function.Name != "native" {
		t.Fatalf("NativeSchemas: %#v", schemas)
	}
	if !ConversationHasJSScanAsset("conv-js") || ConversationHasJSScanAsset("other") {
		t.Fatal("bridge conversation lookup not dispatched")
	}

	if _, handled, err := RunTool(context.Background(), *cfg, pentest.AgentToolCall{Function: pentest.AgentFunctionCall{Name: "external_mcp_x"}}); err != nil || !handled || p.ran != "external:external_mcp_x" {
		t.Fatalf("external dispatch: %v handled=%v ran=%q", err, handled, p.ran)
	}
	if _, handled, err := RunTool(context.Background(), *cfg, pentest.AgentToolCall{Function: pentest.AgentFunctionCall{Name: "nuclei"}}); err != nil || !handled || b.ran != "native:nuclei" {
		t.Fatalf("native dispatch: %v handled=%v ran=%q", err, handled, b.ran)
	}
}

func TestJSScanFiltering(t *testing.T) {
	names := []string{
		"attack_surface_create_jsscan_task",
		"attack_surface_get_jsscan_overview",
		"attack_surface_search_jsscan_operations",
		"attack_surface_search_jsscan_vulnerabilities",
		"attack_surface_search_jsscan_resources",
		"attack_surface_get_jsscan_evidence",
		"attack_surface_export_jsscan_openapi",
		"attack_surface_read_reference",
	}
	for _, name := range names[:len(names)-1] {
		if !IsJSScanTool(name) {
			t.Fatalf("%q should be a JSScan tool", name)
		}
	}
	if IsJSScanTool("attack_surface_read_reference") {
		t.Fatal("read_reference must not be filtered")
	}

	tools := make([]pentest.AgentToolSchema, 0, len(names))
	for _, name := range names {
		tools = append(tools, pentest.AgentToolSchema{Function: pentest.AgentToolFunction{Name: name}})
	}
	all := RemoveJSScanTools(tools)
	if len(all) != 1 || all[0].Function.Name != "attack_surface_read_reference" {
		t.Fatalf("RemoveJSScanTools: %#v", all)
	}
	readers := RemoveJSScanCreateTool(tools)
	if len(readers) != len(tools)-1 {
		t.Fatalf("RemoveJSScanCreateTool kept %d tools", len(readers))
	}
}

func TestFirstLineAndFirstNonEmpty(t *testing.T) {
	if got := FirstLine("  first\nsecond"); got != "first" {
		t.Fatalf("FirstLine=%q", got)
	}
	if got := FirstNonEmpty("", "  ", "x"); got != "x" {
		t.Fatalf("FirstNonEmpty=%q", got)
	}
}
