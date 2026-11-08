package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"

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

func TestMCPToolCatalogExposesPlannedExternalModules(t *testing.T) {
	tools := mcpCatalogTools()
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Name] = true
	}

	expected := []string{
		"space_engine_search",
		"space_engine_native_search",
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
		"workbench_task_update",
		"workbench_step_add",
		"workbench_fact_upsert",
		"workbench_fact_search",
		"workbench_evidence_add",
		"workbench_hypothesis_update",
		"workbench_scope_check",
		"workbench_execution_list",
		"workbench_execution_get",
		"workbench_execution_wait",
		"workbench_execution_cancel",
		"http_request",
		"http_sequence_run",
		"http_extract_variable",
		"http_compare_response",
		"api_endpoint_record",
		"api_relation_record",
		"api_schema_analyze",
		"exec_run",
		"python_execute",
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

	for _, tool := range aggregateTools {
		if _, blocked := externallyBlockedMCPTools[tool.Name]; blocked {
			t.Fatalf("public Hephaestus endpoint must not expose command execution tool %s", tool.Name)
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

func TestWorkbenchMCPToolsManageTaskBlackboard(t *testing.T) {
	setupMCPWorkbenchTestDatabase(t)

	db, err := database.GetDB()
	if err != nil {
		t.Fatalf("GetDB() error = %v", err)
	}
	task := database.AIWorkbenchTask{
		ID:              "ai-task-mcp",
		Name:            "MCP workbench task",
		Target:          "https://example.test",
		Scope:           []string{"https://example.test", "10.0.0.0/24"},
		Mode:            "agent",
		Status:          "running",
		RiskPolicy:      "manual_approval",
		Objective:       "验证工作台 MCP 控制面",
		SuccessCriteria: "事实、证据和假设可被写入并检索",
		DeliveryFormat:  []string{"证据清单"},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := mcpCallWorkbenchFactUpsert(mcpWorkbenchFactUpsertArgs{
		TaskID:     task.ID,
		FactKey:    "api/login-flow",
		Summary:    "登录接口返回 bearer token，后续接口依赖该 token。",
		Confidence: "high",
	}); err != nil {
		t.Fatalf("fact upsert: %v", err)
	}
	if _, err := mcpCallWorkbenchFactUpsert(mcpWorkbenchFactUpsertArgs{
		TaskID:     task.ID,
		FactKey:    "api/login-flow",
		Summary:    "登录接口返回 bearer token 和 userId，后续接口依赖该 token。",
		Confidence: "high",
	}); err != nil {
		t.Fatalf("fact upsert update: %v", err)
	}

	searchResult, err := mcpCallWorkbenchFactSearch(mcpWorkbenchFactSearchArgs{TaskID: task.ID, Query: "userId"})
	if err != nil {
		t.Fatalf("fact search: %v", err)
	}
	searchPayload, ok := searchResult.StructuredContent.(gin.H)
	if !ok || searchPayload["total"].(int) != 1 {
		t.Fatalf("expected one searched fact, got %#v", searchResult.StructuredContent)
	}

	if _, err := mcpCallWorkbenchEvidenceAdd(mcpWorkbenchEvidenceAddArgs{
		TaskID:     task.ID,
		SourceType: "http_sequence",
		TargetRef:  "https://example.test/api/login",
		Title:      "登录接口响应",
		Summary:    "响应 JSON 中包含 token 和 userId 字段。",
		RiskLevel:  "info",
		Confidence: "high",
	}); err != nil {
		t.Fatalf("evidence add: %v", err)
	}

	hypothesisResult, err := mcpCallWorkbenchHypothesisUpdate(mcpWorkbenchHypothesisUpdateArgs{
		TaskID:      task.ID,
		Title:       "用户对象存在越权风险",
		Description: "列表接口返回 userId，详情接口可能接受任意 userId。",
		Status:      "testing",
		Confidence:  "medium",
	})
	if err != nil {
		t.Fatalf("hypothesis create: %v", err)
	}
	hypothesisPayload := hypothesisResult.StructuredContent.(gin.H)
	hypothesis := hypothesisPayload["hypothesis"].(database.AIWorkbenchHypothesis)
	if _, err := mcpCallWorkbenchHypothesisUpdate(mcpWorkbenchHypothesisUpdateArgs{
		TaskID:       task.ID,
		HypothesisID: hypothesis.ID,
		Status:       "supported",
		Confidence:   "high",
	}); err != nil {
		t.Fatalf("hypothesis update: %v", err)
	}

	if _, err := mcpCallWorkbenchStepAdd(mcpWorkbenchStepAddArgs{
		TaskID:    task.ID,
		StepType:  "verification",
		Title:     "验证用户详情接口越权",
		RiskLevel: "high",
	}); err != nil {
		t.Fatalf("step add: %v", err)
	}

	scopeResult, err := mcpCallWorkbenchScopeCheck(mcpWorkbenchScopeCheckArgs{TaskID: task.ID, Target: "https://api.example.test/v1/users"})
	if err != nil {
		t.Fatalf("scope check: %v", err)
	}
	scopePayload := scopeResult.StructuredContent.(gin.H)
	if inScope, _ := scopePayload["inScope"].(bool); !inScope {
		t.Fatalf("expected subdomain to be in scope, got %#v", scopePayload)
	}

	var factCount int64
	if err := db.Model(&database.AIWorkbenchObservation{}).Where("task_id = ? AND source = ?", task.ID, mcpWorkbenchFactSource).Count(&factCount).Error; err != nil {
		t.Fatalf("count facts: %v", err)
	}
	if factCount != 1 {
		t.Fatalf("expected fact upsert to deduplicate by key, got %d", factCount)
	}

	var storedTask database.AIWorkbenchTask
	if err := db.Where("id = ?", task.ID).First(&storedTask).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if len(storedTask.CurrentPlan) != 1 || storedTask.CurrentPlan[0].StepType != "verification" {
		encoded, _ := json.Marshal(storedTask.CurrentPlan)
		t.Fatalf("expected appended verification plan item, got %s", string(encoded))
	}
	var planTaskCount int64
	if err := db.Model(&database.AIWorkbenchPlanTask{}).Where("task_id = ? AND step_id = ?", task.ID, storedTask.CurrentPlan[0].ID).Count(&planTaskCount).Error; err != nil {
		t.Fatalf("count plan tasks: %v", err)
	}
	if planTaskCount != 1 {
		t.Fatalf("expected appended step to sync one plan task, got %d", planTaskCount)
	}
}

func TestWorkbenchHTTPMCPToolsRecordAPIGraph(t *testing.T) {
	setupMCPWorkbenchTestDatabase(t)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login":
			_, _ = w.Write([]byte(`{"token":"t-123","user":{"id":42}}`))
		case "/profile":
			if r.Header.Get("Authorization") != "Bearer t-123" || r.URL.Query().Get("id") != "42" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			_, _ = w.Write([]byte(`{"userId":42,"name":"admin"}`))
		case "/profile-alt":
			_, _ = w.Write([]byte(`{"userId":43,"name":"guest"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer api.Close()

	db, err := database.GetDB()
	if err != nil {
		t.Fatalf("GetDB() error = %v", err)
	}
	task := database.AIWorkbenchTask{
		ID:              "ai-task-http-mcp",
		Name:            "MCP HTTP workbench task",
		Target:          api.URL,
		Scope:           []string{api.URL},
		Mode:            "agent",
		Status:          "running",
		RiskPolicy:      "manual_approval",
		Objective:       "验证接口链路和 API Graph",
		SuccessCriteria: "HTTP 请求、变量和接口关系可被沉淀",
		DeliveryFormat:  []string{"接口链路图"},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	sequenceResult, err := mcpCallWorkbenchHTTPSequenceRun(mcpWorkbenchHTTPSequenceRunArgs{
		TaskID: task.ID,
		Requests: []mcpWorkbenchHTTPRequestArgs{
			{
				Method: "POST",
				URL:    api.URL + "/login",
				Extract: []mcpWorkbenchHTTPExtractorArg{
					{Name: "token", JSONPath: "$.token", ValueType: "token"},
					{Name: "userId", JSONPath: "$.user.id", ValueType: "id"},
				},
			},
			{
				Method:  "GET",
				URL:     api.URL + "/profile?id={{userId}}",
				Headers: map[string]string{"Authorization": "Bearer {{token}}"},
				Extract: []mcpWorkbenchHTTPExtractorArg{
					{Name: "profileUserId", JSONPath: "$.userId", ValueType: "id"},
				},
			},
		},
		RecordRelations: true,
	})
	if err != nil {
		t.Fatalf("http sequence run: %v", err)
	}
	sequencePayload := sequenceResult.StructuredContent.(gin.H)
	variables := sequencePayload["variables"].(map[string]string)
	if variables["token"] != "t-123" || variables["userId"] != "42" || variables["profileUserId"] != "42" {
		t.Fatalf("unexpected extracted variables: %#v", variables)
	}

	var endpoints []database.AIWorkbenchAPIEndpoint
	if err := db.Where("task_id = ?", task.ID).Order("created_at ASC").Find(&endpoints).Error; err != nil {
		t.Fatalf("load endpoints: %v", err)
	}
	if len(endpoints) != 2 {
		t.Fatalf("expected two API endpoints, got %d", len(endpoints))
	}
	if !endpoints[1].AuthRequired {
		t.Fatalf("expected profile endpoint to be marked auth required, got %#v", endpoints[1])
	}

	if _, err := mcpCallWorkbenchAPIRelationRecord(mcpWorkbenchAPIRelationRecordArgs{
		TaskID:           task.ID,
		SourceEndpointID: endpoints[0].ID,
		TargetEndpointID: endpoints[1].ID,
		RelationType:     "variable_feeds",
		VariableName:     "token",
		Confidence:       "high",
	}); err != nil {
		t.Fatalf("api relation record: %v", err)
	}

	var relationCount int64
	if err := db.Model(&database.AIWorkbenchAPIRelation{}).Where("task_id = ?", task.ID).Count(&relationCount).Error; err != nil {
		t.Fatalf("count relations: %v", err)
	}
	if relationCount != 4 {
		t.Fatalf("expected sequence, auth, variable, and manual relations, got %d", relationCount)
	}
	relationTypeCounts := map[string]int64{}
	for _, relationType := range []string{"sequence_next", "auth_flow", "variable_flow", "variable_feeds"} {
		var count int64
		if err := db.Model(&database.AIWorkbenchAPIRelation{}).Where("task_id = ? AND relation_type = ?", task.ID, relationType).Count(&count).Error; err != nil {
			t.Fatalf("count relation type %s: %v", relationType, err)
		}
		relationTypeCounts[relationType] = count
		if relationTypeCounts[relationType] != 1 {
			t.Fatalf("expected one %s relation, got %#v", relationType, relationTypeCounts)
		}
	}

	var httpEvidence []database.AIWorkbenchHTTPRequestEvidence
	if err := db.Where("task_id = ?", task.ID).Order("created_at ASC").Find(&httpEvidence).Error; err != nil {
		t.Fatalf("load http evidence: %v", err)
	}
	if len(httpEvidence) != 2 || httpEvidence[1].StatusCode != http.StatusOK {
		t.Fatalf("expected two successful HTTP evidence records, got %#v", httpEvidence)
	}
	if httpEvidence[1].RequestHeaders["Authorization"] != "***" {
		t.Fatalf("expected stored authorization header to be redacted, got %#v", httpEvidence[1].RequestHeaders)
	}

	if _, err := mcpCallWorkbenchHTTPExtractVariable(mcpWorkbenchHTTPExtractVariableArgs{
		TaskID:     task.ID,
		Name:       "manualName",
		SourceID:   httpEvidence[1].ID,
		JSONPath:   "$.name",
		Confidence: "high",
	}); err != nil {
		t.Fatalf("extract variable from evidence: %v", err)
	}

	extraResult, err := mcpCallWorkbenchHTTPRequest(mcpWorkbenchHTTPRequestArgs{TaskID: task.ID, Method: "GET", URL: api.URL + "/profile-alt"})
	if err != nil {
		t.Fatalf("extra http request: %v", err)
	}
	extraPayload := extraResult.StructuredContent.(mcpWorkbenchHTTPRequestResult)
	if extraPayload.ExecutionID == "" {
		t.Fatal("expected http_request to return executionId")
	}
	if _, err := mcpCallWorkbenchHTTPCompareResponse(mcpWorkbenchHTTPCompareResponseArgs{
		TaskID:              task.ID,
		BaselineEvidenceID:  httpEvidence[1].ID,
		CandidateEvidenceID: extraPayload.HTTPEvidenceID,
		Checks:              []string{"body"},
	}); err != nil {
		t.Fatalf("compare response: %v", err)
	}

	if _, err := mcpCallWorkbenchHTTPRequest(mcpWorkbenchHTTPRequestArgs{TaskID: task.ID, Method: "GET", URL: "https://example.invalid/out-of-scope"}); err == nil {
		t.Fatal("out-of-scope HTTP request should be rejected")
	}

	var httpExecutionCount int64
	if err := db.Model(&database.AIWorkbenchToolExecution{}).Where("task_id = ? AND tool_name = ? AND status = ?", task.ID, "http.request", "completed").Count(&httpExecutionCount).Error; err != nil {
		t.Fatalf("count http executions: %v", err)
	}
	if httpExecutionCount != 3 {
		t.Fatalf("expected three completed HTTP executions, got %d", httpExecutionCount)
	}
}

func TestWorkbenchPythonExecuteRunsRestrictedAnalysis(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not available")
	}
	setupMCPWorkbenchTestDatabase(t)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"items":[1,2,3]}`))
	}))
	defer api.Close()

	db, err := database.GetDB()
	if err != nil {
		t.Fatalf("GetDB() error = %v", err)
	}
	task := database.AIWorkbenchTask{
		ID:              "ai-task-python-mcp",
		Name:            "MCP Python workbench task",
		Target:          api.URL,
		Scope:           []string{api.URL},
		Mode:            "agent",
		Status:          "running",
		RiskPolicy:      "manual_approval",
		Objective:       "验证受控 Python 分析",
		SuccessCriteria: "Python 分析输出可审计",
		DeliveryFormat:  []string{"分析结果"},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	result, err := mcpCallWorkbenchPythonExecute(mcpWorkbenchPythonExecuteArgs{
		TaskID: task.ID,
		Code: strings.Join([]string{
			"import json",
			"resp = scoped_http_request('GET', inputs['url'])",
			"payload = json.loads(resp['body'])",
			"_result = {'count': len(payload['items']), 'ok': payload['ok']}",
			"write_text('summary.json', json.dumps(_result))",
		}, "\n"),
		Inputs: map[string]interface{}{"url": api.URL},
	})
	if err != nil {
		t.Fatalf("python execute: %v", err)
	}
	payload := result.StructuredContent.(gin.H)
	if payload["executionId"] == "" {
		t.Fatal("expected python_execute to return executionId")
	}
	if !strings.Contains(payload["result"].(string), `"count": 3`) {
		t.Fatalf("unexpected python result: %#v", payload)
	}
	files := payload["files"].([]string)
	if len(files) != 1 || files[0] != "summary.json" {
		t.Fatalf("expected summary file, got %#v", files)
	}

	if _, err := mcpCallWorkbenchPythonExecute(mcpWorkbenchPythonExecuteArgs{
		TaskID: task.ID,
		Code:   "import os\n_result = os.getcwd()",
	}); err == nil {
		t.Fatal("python_execute should reject denied imports")
	}

	if _, err := mcpCallWorkbenchPythonExecute(mcpWorkbenchPythonExecuteArgs{
		TaskID:    task.ID,
		Code:      "_result = 'high risk'",
		RiskLevel: "high",
	}); err == nil {
		t.Fatal("high-risk python_execute should require approved approvalId")
	}

	var callCount int64
	if err := db.Model(&database.AIWorkbenchToolCall{}).Where("task_id = ? AND tool_name = ?", task.ID, "python_execute").Count(&callCount).Error; err != nil {
		t.Fatalf("count python tool calls: %v", err)
	}
	if callCount < 2 {
		t.Fatalf("expected completed and failed python tool calls, got %d", callCount)
	}

	var executionCount int64
	if err := db.Model(&database.AIWorkbenchToolExecution{}).Where("task_id = ? AND tool_name = ?", task.ID, "python.execute").Count(&executionCount).Error; err != nil {
		t.Fatalf("count python executions: %v", err)
	}
	if executionCount < 2 {
		t.Fatalf("expected completed and failed python executions, got %d", executionCount)
	}
}

func TestWorkbenchExecutionMCPToolsControlLifecycle(t *testing.T) {
	setupMCPWorkbenchTestDatabase(t)

	db, err := database.GetDB()
	if err != nil {
		t.Fatalf("GetDB() error = %v", err)
	}
	task := database.AIWorkbenchTask{
		ID:              "ai-task-execution-mcp",
		Name:            "MCP execution control task",
		Target:          "https://example.test",
		Scope:           []string{"https://example.test"},
		Mode:            "agent",
		Status:          "running",
		RiskPolicy:      "manual_approval",
		Objective:       "验证工具 execution 控制",
		SuccessCriteria: "execution 可查询、等待和取消",
		DeliveryFormat:  []string{"控制状态"},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	startedAt := time.Now().Add(-time.Second)
	toolCall := database.AIWorkbenchToolCall{
		ID:          "tool-call-execution-mcp",
		TaskID:      task.ID,
		ExecutionID: "execution-mcp-running",
		ToolName:    "http_request",
		Status:      "running",
	}
	execution := database.AIWorkbenchToolExecution{
		ID:           "execution-mcp-running",
		TaskID:       task.ID,
		ToolCallID:   toolCall.ID,
		ToolName:     "http.request",
		Runtime:      aipkg.WorkbenchToolRuntimeLocalMCP,
		Status:       aipkg.WorkbenchToolExecutionStatusRunning,
		StartedAt:    &startedAt,
		ResultRef:    "ai_http_request:pending",
		Arguments:    map[string]any{"url": "https://example.test"},
		InputSummary: "GET https://example.test",
	}
	if err := db.Create(&toolCall).Error; err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if err := db.Create(&execution).Error; err != nil {
		t.Fatalf("create execution: %v", err)
	}

	listResult, err := mcpCallWorkbenchExecutionList(mcpWorkbenchExecutionListArgs{TaskID: task.ID, Status: aipkg.WorkbenchToolExecutionStatusRunning})
	if err != nil {
		t.Fatalf("list executions: %v", err)
	}
	listPayload := listResult.StructuredContent.(gin.H)
	if listPayload["count"].(int) != 1 {
		t.Fatalf("expected one running execution, got %#v", listPayload)
	}

	getResult, err := mcpCallWorkbenchExecutionGet(mcpWorkbenchExecutionGetArgs{TaskID: task.ID, ExecutionID: execution.ID})
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	getPayload := getResult.StructuredContent.(gin.H)
	gotExecution := getPayload["execution"].(database.AIWorkbenchToolExecution)
	if gotExecution.ID != execution.ID || gotExecution.Status != aipkg.WorkbenchToolExecutionStatusRunning {
		t.Fatalf("unexpected execution payload: %#v", gotExecution)
	}

	waitResult, err := mcpCallWorkbenchExecutionWait(mcpWorkbenchExecutionWaitArgs{TaskID: task.ID, ExecutionID: execution.ID, TimeoutMS: 1})
	if err != nil {
		t.Fatalf("wait execution: %v", err)
	}
	waitPayload := waitResult.StructuredContent.(gin.H)
	if waitPayload["timedOut"].(bool) != true {
		t.Fatalf("expected wait to time out for running execution, got %#v", waitPayload)
	}

	cancelResult, err := mcpCallWorkbenchExecutionCancel(mcpWorkbenchExecutionCancelArgs{TaskID: task.ID, ExecutionID: execution.ID})
	if err != nil {
		t.Fatalf("cancel execution: %v", err)
	}
	cancelPayload := cancelResult.StructuredContent.(gin.H)
	cancelled := cancelPayload["execution"].(database.AIWorkbenchToolExecution)
	if cancelled.Status != aipkg.WorkbenchToolExecutionStatusCancelled || !cancelPayload["cancelled"].(bool) {
		t.Fatalf("expected cancelled execution, got %#v", cancelPayload)
	}

	var updatedCall database.AIWorkbenchToolCall
	if err := db.Where("id = ?", toolCall.ID).First(&updatedCall).Error; err != nil {
		t.Fatalf("load updated tool call: %v", err)
	}
	if updatedCall.Status != "cancelled" {
		t.Fatalf("expected linked tool call cancelled, got %s", updatedCall.Status)
	}

	var eventCount int64
	if err := db.Model(&database.AIWorkbenchEvent{}).Where("task_id = ? AND type = ?", task.ID, "tool.execution.cancelled").Count(&eventCount).Error; err != nil {
		t.Fatalf("count cancel events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("expected one cancel event, got %d", eventCount)
	}
}

func TestWorkbenchAPISchemaAnalyzeRecordsCandidateGraph(t *testing.T) {
	setupMCPWorkbenchTestDatabase(t)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	db, err := database.GetDB()
	if err != nil {
		t.Fatalf("GetDB() error = %v", err)
	}
	task := database.AIWorkbenchTask{
		ID:              "ai-task-api-analyze-mcp",
		Name:            "MCP API schema analyze task",
		Target:          api.URL,
		Scope:           []string{api.URL},
		Mode:            "agent",
		Status:          "running",
		RiskPolicy:      "manual_approval",
		Objective:       "验证 API 图谱候选提取",
		SuccessCriteria: "接口节点和候选关系可落库",
		DeliveryFormat:  []string{"API 图谱"},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	result, err := mcpCallWorkbenchAPISchemaAnalyze(mcpWorkbenchAPISchemaAnalyzeArgs{
		TaskID:     task.ID,
		BaseURL:    api.URL,
		SourceType: "js",
		SourceID:   "asset-js-1",
		Input: strings.Join([]string{
			`axios.post('/api/login', payload)`,
			`fetch('/api/users/' + userId, { method: 'GET' })`,
			`GET /api/orders?userId=42`,
			`const ignored = "/static/app.js"`,
			`const outOfScope = "https://evil.example/api/skip"`,
		}, "\n"),
	})
	if err != nil {
		t.Fatalf("api schema analyze: %v", err)
	}
	payload := result.StructuredContent.(gin.H)
	if payload["executionId"] == "" {
		t.Fatal("expected api_schema_analyze to return executionId")
	}
	endpoints := payload["endpoints"].([]database.AIWorkbenchAPIEndpoint)
	if len(endpoints) != 3 {
		t.Fatalf("expected three scoped endpoints, got %#v", endpoints)
	}
	skipped := payload["skipped"].([]mcpWorkbenchAPIAnalyzeSkipped)
	if len(skipped) != 1 || !strings.Contains(skipped[0].URL, "evil.example") {
		t.Fatalf("expected one out-of-scope skipped candidate, got %#v", skipped)
	}
	relations := payload["relations"].([]database.AIWorkbenchAPIRelation)
	if len(relations) != 3 {
		t.Fatalf("expected two candidate sequence relations and one variable relation, got %#v", relations)
	}
	var variableRelationCount int
	for _, relation := range relations {
		if relation.RelationType == "candidate_variable_flow" && relation.VariableName == "userid" {
			variableRelationCount++
		}
	}
	if variableRelationCount != 1 {
		t.Fatalf("expected one candidate variable relation for userId, got %#v", relations)
	}

	var execution database.AIWorkbenchToolExecution
	if err := db.Where("task_id = ? AND tool_name = ? AND status = ?", task.ID, "api.schema.analyze", aipkg.WorkbenchToolExecutionStatusCompleted).First(&execution).Error; err != nil {
		t.Fatalf("expected completed api schema execution: %v", err)
	}
	if execution.ToolCallID == "" || execution.DurationMS < 0 {
		t.Fatalf("unexpected execution record: %#v", execution)
	}
}

func TestWorkbenchExecRunUsesControlledExecution(t *testing.T) {
	setupMCPWorkbenchTestDatabase(t)

	db, err := database.GetDB()
	if err != nil {
		t.Fatalf("GetDB() error = %v", err)
	}
	task := database.AIWorkbenchTask{
		ID:              "ai-task-exec-mcp",
		Name:            "MCP exec control task",
		Target:          "https://example.test",
		Scope:           []string{"https://example.test"},
		Mode:            "agent",
		Status:          "running",
		RiskPolicy:      "manual_approval",
		Objective:       "验证受控 exec",
		SuccessCriteria: "命令执行受范围和白名单约束",
		DeliveryFormat:  []string{"exec 结果"},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	result, err := mcpCallWorkbenchExecRun(mcpWorkbenchExecRunArgs{
		TaskID:  task.ID,
		Command: "printf",
		Args:    []string{"hello"},
	})
	if err != nil {
		t.Fatalf("exec run: %v", err)
	}
	payload := result.StructuredContent.(gin.H)
	if payload["executionId"] == "" || payload["stdout"].(string) != "hello" {
		t.Fatalf("unexpected exec payload: %#v", payload)
	}

	if _, err := mcpCallWorkbenchExecRun(mcpWorkbenchExecRunArgs{
		TaskID:  task.ID,
		Command: "sh",
		Args:    []string{"-c", "echo unsafe"},
	}); err == nil {
		t.Fatal("exec_run should reject commands outside the allowlist")
	}

	if _, err := mcpCallWorkbenchExecRun(mcpWorkbenchExecRunArgs{
		TaskID:  task.ID,
		Command: "curl",
		Args:    []string{"https://evil.example/api"},
	}); err == nil {
		t.Fatal("exec_run should reject out-of-scope URL arguments")
	}

	var execution database.AIWorkbenchToolExecution
	if err := db.Where("task_id = ? AND tool_name = ? AND status = ?", task.ID, "exec.run", aipkg.WorkbenchToolExecutionStatusCompleted).First(&execution).Error; err != nil {
		t.Fatalf("expected completed exec execution: %v", err)
	}
	if execution.ToolCallID == "" || execution.ResultRef == "" {
		t.Fatalf("unexpected exec execution: %#v", execution)
	}
}

func setupMCPWorkbenchTestDatabase(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	config.ForceSetConfigBaseDir(tempDir)
	config.SetRuntimeMode(config.RuntimeModeServer)
	configPath := filepath.Join(tempDir, "config.yaml")
	dbPath := filepath.Join(tempDir, "hephaestus-test.db")
	content := "database:\n  type: sqlite\n  path: " + dbPath + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
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
