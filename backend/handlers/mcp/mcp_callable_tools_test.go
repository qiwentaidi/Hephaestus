package mcphandlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcpclient"
	"hephaestus-backend/pkg/mcptools"
)

func TestCallableAIToolsMatchDialogueRuntime(t *testing.T) {
	oldRead := externalMCPReadSetting
	defer func() { externalMCPReadSetting = oldRead }()
	preferences := map[string]bool{"Bash": false, "dnslog": true}
	servers := []mcpclient.Server{
		{ID: "one", Name: "First", Enabled: true, Headers: map[string]string{"Authorization": "secret-header"}, Env: map[string]string{"KEY": "secret-env"}, Tools: []mcpclient.Tool{{Name: "search"}, {Name: "hidden"}}, DisabledTools: []string{"hidden"}},
		{ID: "two", Name: "Second", Enabled: true, Tools: []mcpclient.Tool{{Name: "search"}}},
		{ID: "off", Name: "Disabled", Enabled: false, Tools: []mcpclient.Tool{{Name: "search"}}},
	}
	externalMCPReadSetting = func(key string) (any, error) {
		if key == aiToolsSettingKey {
			return preferences, nil
		}
		return servers, nil
	}
	router := gin.New()
	router.GET("/tools", ListCallableAIToolsHandler)
	read := func() []callableAITool {
		t.Helper()
		out := httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequest("GET", "/tools", nil))
		if out.Code != 200 {
			t.Fatalf("catalog: %d %s", out.Code, out.Body.String())
		}
		if strings.Contains(out.Body.String(), "secret-") {
			t.Fatal("catalog exposed connection credentials")
		}
		var result struct {
			Data []callableAITool `json:"data"`
		}
		if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Data
	}
	items := read()
	catalog, err := pentest.LoadSystemTools("")
	if err != nil {
		t.Fatal(err)
	}
	cfg := pentest.AgentLoopConfig{ToolCatalog: catalog}
	cfg.Conversation.Mode = pentest.SoloPentestMode
	if err := mcptools.RefreshTools(&cfg); err != nil {
		t.Fatal(err)
	}
	available := map[string]bool{}
	for _, schema := range pentest.BuildAgentToolSchemasForConfig(cfg) {
		available[schema.Function.Name] = true
	}
	listed := map[string]callableAITool{}
	for _, item := range items {
		if !available[item.Name] {
			t.Fatalf("listed tool is not exposed to the dialogue: %s", item.Name)
		}
		listed[item.Name] = item
	}
	for name := range available {
		if mcptools.IsJSScanTool(name) {
			continue
		}
		if listed[name].Name == "" {
			t.Fatalf("dialogue tool missing from catalog: %s", name)
		}
	}
	if listed["Bash"].Name != "" || listed["attack_surface_search_jsscan_vulnerabilities"].Name != "" || listed["dnslog"].Name == "" {
		t.Fatalf("catalog ignored tool preferences: %+v", items)
	}
	for _, server := range servers[:2] {
		item := listed[mcpclient.Alias(server.ID, "search")]
		if item.DisplayName != "search" || item.SourceName != server.Name || item.Source != "external" {
			t.Fatalf("external identity mismatch: %+v", item)
		}
	}
	if listed[mcpclient.Alias("one", "hidden")].Name != "" || listed[mcpclient.Alias("off", "search")].Name != "" {
		t.Fatal("disabled external tools were listed")
	}
	// Opening the menu again must reflect updated switches in existing chats.
	servers[0].Enabled = false
	for _, item := range read() {
		if item.Name == mcpclient.Alias("one", "search") {
			t.Fatalf("stale enabled tool: %+v", item)
		}
	}
}
