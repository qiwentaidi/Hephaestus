package mcp

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthorizeMCPToolCallPermissionLevels(t *testing.T) {
	if err := authorizeMCPToolCall(nil, mcpTool{Name: "read_tool", Permission: "read"}); err != nil {
		t.Fatalf("read permission should be allowed: %v", err)
	}

	if err := authorizeMCPToolCall(nil, mcpTool{Name: "search_tool", Permission: "search"}); err == nil {
		t.Fatal("search permission should require admin role")
	}

	if err := authorizeMCPToolCall(testMCPContextWithRole("user"), mcpTool{Name: "write_tool", Permission: "write"}); err != nil {
		t.Fatalf("authenticated users should be allowed to use write tools: %v", err)
	}

	if err := authorizeMCPToolCall(nil, mcpTool{Name: "unknown_tool", Permission: "danger"}); err == nil {
		t.Fatal("unknown permission should be denied")
	}
}

func TestAuthorizeMCPToolCallAdminSearch(t *testing.T) {
	ctx := testMCPContextWithRole("admin")

	if err := authorizeMCPToolCall(ctx, mcpTool{Name: "search_tool", Permission: "search"}); err != nil {
		t.Fatalf("admin should be allowed to use search tools: %v", err)
	}
}

func TestAuthorizeMCPToolCallAdminPermission(t *testing.T) {
	if err := authorizeMCPToolCall(testMCPContextWithRole("admin"), mcpTool{Name: "admin_tool", Permission: "admin"}); err != nil {
		t.Fatalf("admin role should be allowed to use admin tools: %v", err)
	}
}

func TestMCPToolCatalogDeclaresPermissionsAndAnnotations(t *testing.T) {
	tools := mcpCatalogTools()
	if len(tools) == 0 {
		t.Fatal("expected MCP tool catalog")
	}

	for _, tool := range tools {
		if strings.TrimSpace(tool.Permission) == "" {
			t.Fatalf("tool %s is missing permission", tool.Name)
		}
		if len(tool.Annotations) == 0 {
			t.Fatalf("tool %s is missing annotations", tool.Name)
		}
		if _, ok := tool.Annotations["readOnlyHint"]; !ok {
			t.Fatalf("tool %s is missing readOnlyHint annotation", tool.Name)
		}
		if _, ok := tool.Annotations["destructiveHint"]; !ok {
			t.Fatalf("tool %s is missing destructiveHint annotation", tool.Name)
		}
	}
}

func TestJWTDictionaryToolsDeclareDiscoveryContract(t *testing.T) {
	list := findMCPToolForTest(t, "jwt_list_dictionaries")
	if list.Permission != "read" {
		t.Fatalf("jwt dictionary listing should be read-only, got %q", list.Permission)
	}
	brute := findMCPToolForTest(t, "jwt_bruteforce")
	if !strings.Contains(brute.Description, ".txt") || !strings.Contains(brute.Description, "jwt_list_dictionaries") {
		t.Fatalf("jwt_bruteforce description must require discovered .txt basename: %s", brute.Description)
	}
}

func TestMCPToolCatalogExposesSupportedModules(t *testing.T) {
	tools := mcpCatalogTools()
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Name] = true
	}

	expected := []string{
		"space_engine_search",
		"space_engine_native_search",
		"space_engine_get_quota",
		"space_engine_get_config_status",
		"jwt_bruteforce",
		"jwt_list_dictionaries",
		"webscan_create_task",
		"attack_surface_create_webscan_task",
		"attack_surface_create_portscan_task",
		"attack_surface_create_dirscan_task",
		"attack_surface_create_crack_task",
		"attack_surface_create_jsscan_task",
		"webscan_get_task",
		"webscan_list_vulnerabilities",
		"webscan_list_results",
		"subdomain_create_task",
		"subdomain_get_task",
		"subdomain_list_results",
		"company_create_task",
		"company_get_task",
		"company_get_results",
	}
	for _, name := range expected {
		if !names[name] {
			t.Fatalf("expected MCP tool %s to be exposed", name)
		}
	}
}

func TestAttackSurfaceCreateTaskToolsExposeSpecificScanTypes(t *testing.T) {
	expected := map[string]string{
		"attack_surface_create_webscan_task":  "webscan",
		"attack_surface_create_portscan_task": "portscan",
		"attack_surface_create_dirscan_task":  "dirscan",
		"attack_surface_create_crack_task":    "crack",
		"attack_surface_create_jsscan_task":   "jsscan",
	}

	for toolName, scanType := range expected {
		tool := findMCPToolForTest(t, toolName)
		if tool.ModuleKey != "webscan" {
			t.Fatalf("%s should belong to webscan module, got %s", toolName, tool.ModuleKey)
		}
		if tool.ModuleNameI18n["zh"] != "攻击面评估" {
			t.Fatalf("%s should be shown as 攻击面评估, got %#v", toolName, tool.ModuleNameI18n)
		}
		properties, ok := tool.InputSchema["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s properties should be an object", toolName)
		}
		if _, exists := properties["scanType"]; exists {
			t.Fatalf("%s should fix scanType=%s in the handler instead of exposing scanType", toolName, scanType)
		}
	}
}

func TestWebScanCreateTaskSchemaSupportsAllScanTaskTypes(t *testing.T) {
	tool := findMCPToolForTest(t, "webscan_create_task")
	properties, ok := tool.InputSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("webscan_create_task properties should be an object")
	}
	scanType, ok := properties["scanType"].(map[string]interface{})
	if !ok {
		t.Fatal("webscan_create_task should declare scanType property")
	}

	for _, expected := range []string{"subdomain", "webscan", "portscan", "dirscan", "crack", "jsscan"} {
		if !enumContains(scanType["enum"], expected) {
			t.Fatalf("scanType enum should include %s, got %#v", expected, scanType["enum"])
		}
	}
}

func TestSubdomainToolsBelongToAttackSurfaceAssessment(t *testing.T) {
	for _, toolName := range []string{"subdomain_create_task", "subdomain_get_task", "subdomain_list_results"} {
		tool := findMCPToolForTest(t, toolName)
		if tool.ModuleKey != "webscan" || tool.ModuleNameI18n["zh"] != "攻击面评估" {
			t.Fatalf("%s should belong to attack surface assessment, got module=%s name=%#v", toolName, tool.ModuleKey, tool.ModuleNameI18n)
		}
	}

	createTool := findMCPToolForTest(t, "subdomain_create_task")
	properties, ok := createTool.InputSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("subdomain_create_task properties should be an object")
	}
	for _, property := range []string{"taskName", "domains", "scanMode", "agentId"} {
		if _, exists := properties[property]; !exists {
			t.Fatalf("subdomain_create_task should expose %s", property)
		}
	}
}

func TestNormalizeMCPScanResultTypeInfersFromScanType(t *testing.T) {
	tests := []struct {
		scanType string
		want     string
	}{
		{scanType: "webscan", want: "vulnerabilities"},
		{scanType: "subdomain", want: "subdomains"},
		{scanType: "crack", want: "vulnerabilities"},
		{scanType: "portscan", want: "ports"},
		{scanType: "dirscan", want: "dir_results"},
		{scanType: "jsscan", want: "js_assets"},
	}

	for _, tt := range tests {
		if got := normalizeMCPScanResultType("", tt.scanType); got != tt.want {
			t.Fatalf("scanType %s should infer %s, got %s", tt.scanType, tt.want, got)
		}
	}

	if got := normalizeMCPScanResultType("ports", "webscan"); got != "ports" {
		t.Fatalf("explicit resultType should win, got %s", got)
	}
	if got := normalizeMCPScanResultType("jsscan", "jsscan"); got != "js_assets" {
		t.Fatalf("jsscan resultType alias should prefer collection assets, got %s", got)
	}
	if got := normalizeMCPScanResultType("js_vulnerabilities", "jsscan"); got != "js_vulnerabilities" {
		t.Fatalf("explicit jsscan vulnerability resultType should remain available, got %s", got)
	}
}

func TestMergeMCPWebScanConfigIncludesNonWebScanFields(t *testing.T) {
	aliveDetection := false
	redirect := true
	defaultDict := true
	config := map[string]interface{}{}

	mergeMCPWebScanConfig(config, mcpWebScanCreateTaskArgs{
		Thread:                 8,
		Workers:                16,
		PortRange:              "80,443,8080",
		AliveDetection:         &aliveDetection,
		TCPDiscoveryMode:       "syn",
		SynRate:                3000,
		SynDiscoveryAttempts:   1,
		Method:                 "POST",
		Paths:                  []string{"/admin", " /login "},
		DictPaths:              []string{"/tmp/dir.txt"},
		Exts:                   []string{"php", "jsp"},
		StatusCodeExclude:      []int{404, 500},
		BodyExclude:            "not found",
		BodyLengthExcludeTimes: 3,
		Redirect:               &redirect,
		Username:               "root",
		Password:               "password.txt",
		DefaultDict:            &defaultDict,
		TargetTimeout:          7,
		SSHMaxTimeoutErrors:    2,
	})

	if config["threads"] != 8 || config["workers"] != 16 {
		t.Fatalf("expected thread/worker config to be set, got %#v", config)
	}
	if config["portRange"] != "80,443,8080" || config["aliveDetection"] != false {
		t.Fatalf("expected portscan config to be set, got %#v", config)
	}
	if config["tcpDiscoveryMode"] != "syn" || config["synRate"] != 3000 || config["synDiscoveryAttempts"] != 1 {
		t.Fatalf("expected SYN discovery config to be set, got %#v", config)
	}
	if config["method"] != "POST" || config["bodyExclude"] != "not found" || config["redirect"] != true {
		t.Fatalf("expected dirscan config to be set, got %#v", config)
	}
	if got := config["paths"].([]string); len(got) != 2 || got[1] != "/login" {
		t.Fatalf("expected normalized dirscan paths, got %#v", config["paths"])
	}
	if got := config["statusCodeExclude"].([]int); len(got) != 2 || got[0] != 404 || got[1] != 500 {
		t.Fatalf("expected status exclusions to be set, got %#v", config["statusCodeExclude"])
	}
	if config["username"] != "root" || config["password"] != "password.txt" || config["defaultDict"] != true {
		t.Fatalf("expected crack config to be set, got %#v", config)
	}
}

func TestMergeMCPWebScanConfigKeepsCustomAICandidatesExplicit(t *testing.T) {
	config := map[string]interface{}{}
	mergeMCPWebScanConfig(config, mcpWebScanCreateTaskArgs{
		ScanType: "dirscan",
		Paths:    []string{"/gongshu-web.zip"},
	})

	if _, configured := config["defaultDict"]; configured {
		t.Fatalf("custom AI paths should not implicitly select the default directory dictionary, got %#v", config["defaultDict"])
	}

	config = map[string]interface{}{"defaultDict": false}
	mergeMCPWebScanConfig(config, mcpWebScanCreateTaskArgs{
		ScanType: "dirscan",
		Paths:    []string{"/only-this"},
	})
	if got, ok := config["defaultDict"].(bool); !ok || got {
		t.Fatalf("explicit defaultDict=false should remain an opt-out, got %#v", config["defaultDict"])
	}
}

func TestMCPProfileToolFiltering(t *testing.T) {
	aggregateTools := mcpToolsForProfile(mcpHephaestusProfile)
	spaceTools := mcpToolsForProfile(mcpSpaceEngineProfile)

	if len(aggregateTools) <= len(spaceTools) {
		t.Fatalf("aggregate endpoint should expose more tools than space-engine endpoint: aggregate=%d space=%d", len(aggregateTools), len(spaceTools))
	}

	aggregateModules := make(map[string]bool)
	for _, tool := range aggregateTools {
		aggregateModules[tool.ModuleKey] = true
	}
	for _, module := range []string{"space_engine", "webscan", "company", "ai_workbench"} {
		if !aggregateModules[module] {
			t.Fatalf("aggregate endpoint missing module %s", module)
		}
	}

	for _, tool := range spaceTools {
		if tool.ModuleKey != "space_engine" {
			t.Fatalf("space-engine endpoint should only expose space_engine tools, got %s from %s", tool.Name, tool.ModuleKey)
		}
	}

}

func TestMCPServerDescriptorsExposeAggregateAndDedicatedEndpoints(t *testing.T) {
	servers := MCPServerDescriptors()
	if len(servers) != 2 {
		t.Fatalf("expected 2 MCP server descriptors, got %d", len(servers))
	}

	var aggregate *MCPServerDescriptor
	var dedicated *MCPServerDescriptor
	for index := range servers {
		switch servers[index].Key {
		case "hephaestus":
			aggregate = &servers[index]
		case "space-engine":
			dedicated = &servers[index]
		}
	}

	if aggregate == nil {
		t.Fatal("missing hephaestus aggregate descriptor")
	}
	if aggregate.Endpoint != "/api/v1/mcp/hephaestus" || !aggregate.Aggregate || aggregate.ToolCount == 0 {
		t.Fatalf("invalid aggregate descriptor: %+v", *aggregate)
	}

	if dedicated == nil {
		t.Fatal("missing space-engine dedicated descriptor")
	}
	if dedicated.Endpoint != "/api/v1/mcp/space-engine" || dedicated.LegacyEndpoint != "/api/v1/mcp/space-engine" {
		t.Fatalf("invalid dedicated descriptor: %+v", *dedicated)
	}
	if aggregate.ToolCount <= dedicated.ToolCount {
		t.Fatalf("aggregate descriptor should expose more tools than dedicated descriptor: aggregate=%d dedicated=%d", aggregate.ToolCount, dedicated.ToolCount)
	}
}

func TestBuildSpaceOptionForEnginesCapsMaxPages(t *testing.T) {
	option := buildSpaceOptionForEngines([]string{"fofa"}, mcpSearchMaxPages+10)

	if option.MaxPages != mcpSearchMaxPages {
		t.Fatalf("expected maxPages to be capped at %d, got %d", mcpSearchMaxPages, option.MaxPages)
	}
	if !option.EnableFofa {
		t.Fatal("expected fofa engine to be enabled")
	}
}

func TestRedactMapSecrets(t *testing.T) {
	redacted := redactMapSecrets(map[string]interface{}{
		"apiKey": "secret-value",
		"plain":  "visible",
		"nested": map[string]interface{}{
			"cookie": "session",
		},
	})

	if redacted["apiKey"] != "***" {
		t.Fatalf("expected apiKey to be redacted, got %#v", redacted["apiKey"])
	}
	if redacted["plain"] != "visible" {
		t.Fatalf("expected non-sensitive value to remain visible")
	}
	nested, ok := redacted["nested"].(map[string]interface{})
	if !ok || nested["cookie"] != "***" {
		t.Fatalf("expected nested cookie to be redacted, got %#v", redacted["nested"])
	}
}

func testMCPContextWithRole(role string) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", role)
	return ctx
}

func findMCPToolForTest(t *testing.T, name string) mcpTool {
	t.Helper()
	for _, tool := range mcpCatalogTools() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %s", name)
	return mcpTool{}
}

func enumContains(enumValue interface{}, expected string) bool {
	items, ok := enumValue.([]interface{})
	if !ok {
		return false
	}
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}
