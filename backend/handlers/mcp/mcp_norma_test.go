package mcphandlers

import (
	"hephaestus-backend/pkg/toolruntime"
	"testing"
)

func TestNormaCatalogAndLegacyPreferences(t *testing.T) {
	oldRead, oldWrite := externalMCPReadSetting, externalMCPWriteSetting
	oldAgentRead, oldAgentWrite := agentToolReadSetting, agentToolWriteSetting
	defer func() {
		externalMCPReadSetting, externalMCPWriteSetting = oldRead, oldWrite
		agentToolReadSetting, agentToolWriteSetting = oldAgentRead, oldAgentWrite
	}()
	state := map[string]any{aiToolsSettingKey: map[string]bool{"exec": false}, agentToolVisibilityKey: map[string][]string{"worker": {"exec", "nuclei"}}}
	read := func(key string) (any, error) { return state[key], nil }
	write := func(key string, value any) error { state[key] = value; return nil }
	externalMCPReadSetting, externalMCPWriteSetting = read, write
	agentToolReadSetting, agentToolWriteSetting = read, write
	preferences, err := loadAIToolPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := preferences["exec"]; ok {
		t.Fatal("old preference not migrated")
	}
	disabled, _, err := AgentToolDisabled("worker")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Bash", "Monitor", "shell_open", "shell_send"} {
		if enabled, ok := preferences[name]; !ok || enabled {
			t.Fatalf("lost disabled preference for %s", name)
		}
		if !disabled[name] {
			t.Fatalf("lost role restriction for %s", name)
		}
	}
	if !disabled["nuclei"] || disabled["TaskStop"] {
		t.Fatalf("unrelated policy changed: %v", disabled)
	}
	catalog, err := aiCapabilityCatalog()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]int{}
	for _, item := range catalog {
		names[item["name"].(string)]++
	}
	for _, name := range toolruntime.Names() {
		if names[name] != 1 {
			t.Fatalf("%s catalog entries: %d", name, names[name])
		}
	}
	if names["exec"] != 0 {
		t.Fatal("exec still advertised")
	}
}
