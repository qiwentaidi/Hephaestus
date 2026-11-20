package mcphandlers

import (
	"context"
	"testing"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/capabilities"
	"hephaestus-backend/pkg/mcpclient"
)

func TestAgentToolVisibilityByRole(t *testing.T) {
	oldRead, oldWrite := agentToolReadSetting, agentToolWriteSetting
	defer func() { agentToolReadSetting, agentToolWriteSetting = oldRead, oldWrite }()
	stored := map[string]any{
		agentToolVisibilityKey: map[string]any{
			"planner":    []string{"nuclei"},
			"worker":     []string{},
			"controller": []string{"external_mcp_demo_search"},
			"pentest":    []string{"Bash", "nuclei"},
		},
	}
	agentToolReadSetting = func(key string) (any, error) { return stored[key], nil }

	cases := []struct {
		role   string
		want   []string
		absent []string
	}{
		{"planner", []string{"nuclei"}, []string{"Bash"}},
		{"worker", nil, []string{"Bash", "add_intent"}},
		{"mainagent", []string{"external_mcp_demo_search"}, []string{"Bash"}},
		{"", []string{"Bash", "nuclei"}, nil},
	}
	for _, tc := range cases {
		disabled, restricted, err := AgentToolDisabled(tc.role)
		if err != nil || !restricted {
			t.Fatalf("role %q restricted=%v err=%v", tc.role, restricted, err)
		}
		for _, name := range tc.want {
			if !disabled[name] {
				t.Fatalf("role %q missing disabled %q: %v", tc.role, name, disabled)
			}
		}
		for _, name := range tc.absent {
			if disabled[name] {
				t.Fatalf("role %q unexpectedly disabled %q: %v", tc.role, name, disabled)
			}
		}
	}
}

func TestAgentToolVisibilityDefaultsApplyLeastPrivilege(t *testing.T) {
	oldRead, oldWrite := agentToolReadSetting, agentToolWriteSetting
	defer func() { agentToolReadSetting, agentToolWriteSetting = oldRead, oldWrite }()
	agentToolReadSetting = func(string) (any, error) { return nil, nil }

	planner, restricted, err := AgentToolDisabled("planner")
	if err != nil || !restricted || !planner["Bash"] {
		t.Fatalf("planner defaults: restricted=%v disabled=%v err=%v", restricted, planner, err)
	}
	worker, _, err := AgentToolDisabled("worker")
	if err != nil || worker["Bash"] {
		t.Fatalf("worker should keep exec: disabled=%v err=%v", worker, err)
	}
	if !worker["add_intent"] {
		t.Fatalf("worker default should hide planning tools: %v", worker)
	}
	pentest, _, err := AgentToolDisabled("")
	if err != nil {
		t.Fatalf("pentest defaults: %v", err)
	}
	// MCP is opt-in for every role, including the standalone pentest role.
	// Native tools remain allowed by default.
	catalog, err := agentToolCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range catalog {
		if tool.Source == "mcp" && !pentest[tool.Name] {
			t.Fatalf("pentest default should disable MCP tool %q", tool.Name)
		}
	}
	for _, name := range []string{"Bash", "nuclei"} {
		if pentest[name] {
			t.Fatalf("pentest default should keep %q: %v", name, pentest)
		}
	}
}

func TestAgentToolCatalogMatchesAICapabilityCatalog(t *testing.T) {
	tools, err := agentToolCatalog()
	if err != nil {
		t.Fatal(err)
	}
	base, err := aiCapabilityCatalog()
	if err != nil {
		t.Fatal(err)
	}
	visible := map[string]bool{}
	for _, tool := range tools {
		if tool.Source == "mcp" {
			continue
		}
		visible[tool.Name] = true
	}
	want := map[string]bool{}
	for _, item := range base {
		name, _ := item["name"].(string)
		want[name] = true
	}
	for name := range want {
		if !visible[name] {
			t.Fatalf("visibility catalog missing %q", name)
		}
	}
	for name := range visible {
		if !want[name] {
			t.Fatalf("visibility catalog has an extra tool %q", name)
		}
	}
}

// The per-role binding is the authorization source: a ticked tool is callable as
// long as it is enabled globally and available, and an unticked one is dropped
// even if the role had it before (ARTEX parity). There is no separate hard-coded
// role allowlist any more.
func TestRoleToolBindingIsTheAuthorizationSource(t *testing.T) {
	oldAgentRead, oldAgentWrite := agentToolReadSetting, agentToolWriteSetting
	oldExternalRead, oldExternalWrite := externalMCPReadSetting, externalMCPWriteSetting
	defer func() {
		agentToolReadSetting, agentToolWriteSetting = oldAgentRead, oldAgentWrite
		externalMCPReadSetting, externalMCPWriteSetting = oldExternalRead, oldExternalWrite
	}()
	server := mcpclient.Server{
		ID: "srv-first", Name: "First", Enabled: true,
		Tools: []mcpclient.Tool{{Name: "miniapp_get_info", Description: "info", InputSchema: map[string]any{"type": "object"}}},
	}
	alias := mcpclient.Alias(server.ID, "miniapp_get_info")
	state := map[string]any{
		externalMCPSettingKey:  []mcpclient.Server{server},
		aiToolsSettingKey:      map[string]bool{"nuclei": false},
		agentToolVisibilityKey: map[string][]string{"planner": {}, "worker": {}, "controller": {}, "pentest": {}},
	}
	read := func(key string) (any, error) { return state[key], nil }
	write := func(key string, value any) error { state[key] = value; return nil }
	agentToolReadSetting, agentToolWriteSetting = read, write
	externalMCPReadSetting, externalMCPWriteSetting = read, write

	worker := pentest.AgentLoopConfig{PromptRole: pentest.AgentRoleWorker}
	if err := RefreshPentestTools(&worker); err != nil {
		t.Fatal(err)
	}
	if !containsName(worker.ToolAllowlist, alias) {
		t.Fatalf("worker allowlist missing visible MCP tool %q: %v", alias, worker.ToolAllowlist)
	}
	if containsName(worker.ToolAllowlist, "nuclei") {
		t.Fatalf("globally disabled tool must not be granted: %v", worker.ToolAllowlist)
	}
	advertised := map[string]bool{}
	for _, schema := range pentest.BuildAgentToolSchemasForConfig(worker) {
		advertised[schema.Function.Name] = true
	}
	if !advertised[alias] {
		t.Fatal("worker model was not offered the visible MCP tool")
	}
	if advertised["nuclei"] {
		t.Fatal("globally disabled tool was still advertised")
	}

	// A planning role can be bound to execution/MCP tools too: the binding, not
	// the role, decides (ARTEX parity). This is what makes "ticked = usable" true
	// for every role.
	planner := pentest.AgentLoopConfig{PromptRole: pentest.AgentRolePlanner}
	if err := RefreshPentestTools(&planner); err != nil {
		t.Fatal(err)
	}
	if !containsName(planner.ToolAllowlist, alias) || !containsName(planner.ToolAllowlist, "Bash") {
		t.Fatalf("planner binding was not honored: %v", planner.ToolAllowlist)
	}

	// Unticking a tool for one role removes it from that role only.
	state[agentToolVisibilityKey] = map[string][]string{
		"planner": {alias, "Bash"}, "worker": {}, "controller": {}, "pentest": {},
	}
	planner = pentest.AgentLoopConfig{PromptRole: pentest.AgentRolePlanner}
	if err := RefreshPentestTools(&planner); err != nil {
		t.Fatal(err)
	}
	if containsName(planner.ToolAllowlist, alias) || containsName(planner.ToolAllowlist, "Bash") {
		t.Fatalf("hidden tools must stay hidden for the planner: %v", planner.ToolAllowlist)
	}
	if !containsName(worker.ToolAllowlist, alias) {
		t.Fatal("hiding a tool for one role changed another role")
	}
}

func TestRoleToolAllowlistStaysRestrictiveWhenEverythingIsHidden(t *testing.T) {
	oldAgentRead, oldAgentWrite := agentToolReadSetting, agentToolWriteSetting
	oldExternalRead := externalMCPReadSetting
	defer func() {
		agentToolReadSetting, agentToolWriteSetting = oldAgentRead, oldAgentWrite
		externalMCPReadSetting = oldExternalRead
	}()
	hidden := append(append([]string{}, pentest.DefaultPentestToolAllowlist()...), "Bash", "nuclei")
	state := map[string]any{
		agentToolVisibilityKey: map[string][]string{"planner": hidden, "worker": hidden, "controller": hidden, "pentest": hidden},
	}
	read := func(key string) (any, error) { return state[key], nil }
	write := func(key string, value any) error { state[key] = value; return nil }
	agentToolReadSetting, agentToolWriteSetting = read, write
	externalMCPReadSetting = read

	names := capabilities.RoleToolAllowlist(context.Background(), pentest.AgentRoleWorker)
	if len(names) != 1 || names[0] != capabilities.NoToolsSentinel {
		t.Fatalf("a fully hidden role must keep only the sentinel: %v", names)
	}
}

func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
