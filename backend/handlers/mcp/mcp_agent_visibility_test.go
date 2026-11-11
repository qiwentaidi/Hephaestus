package mcphandlers

import (
	"testing"
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
	if err != nil || len(pentest) != 0 {
		t.Fatalf("pentest default should keep everything: %v err=%v", pentest, err)
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
