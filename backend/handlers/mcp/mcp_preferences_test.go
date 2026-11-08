package mcphandlers

import (
	"testing"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcptools"
)

func TestPentestNucleiReplacesCLIAndRespectsPreferences(t *testing.T) {
	old := externalMCPReadSetting
	t.Cleanup(func() { externalMCPReadSetting = old })
	externalMCPReadSetting = func(key string) (any, error) {
		if key == aiToolsSettingKey {
			return map[string]bool{"nuclei": false}, nil
		}
		return nil, nil
	}
	catalog, err := pentest.LoadBundledSystemTools()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := catalog.Tool("nuclei"); ok {
		t.Fatal("CLI nuclei is still bundled")
	}
	cfg := pentest.AgentLoopConfig{ToolCatalog: catalog}
	if err := mcptools.RefreshTools(&cfg); err != nil {
		t.Fatal(err)
	}
	if mcptools.AuthorizeTool("nuclei") == nil {
		t.Fatal("existing nuclei preference ignored")
	}
	for _, tool := range pentest.BuildAgentToolSchemasForConfig(cfg) {
		if tool.Function.Name == "nuclei" {
			t.Fatal("disabled nuclei exposed")
		}
	}
	externalMCPReadSetting = func(string) (any, error) { return nil, nil }
	if err := mcptools.RefreshTools(&cfg); err != nil {
		t.Fatal(err)
	}
	names := map[string]int{}
	for _, tool := range pentest.BuildAgentToolSchemasForConfig(cfg) {
		names[tool.Function.Name]++
	}
	for _, name := range []string{"nuclei", "nuclei_list_templates", "nuclei_read_template"} {
		if names[name] != 1 || mcptools.AuthorizeTool(name) != nil {
			t.Fatalf("native tool unavailable/duplicated: %s count=%d", name, names[name])
		}
	}
}
