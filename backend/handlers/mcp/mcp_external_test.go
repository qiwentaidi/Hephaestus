package mcphandlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcpclient"
	"hephaestus-backend/pkg/mcptools"
)

func TestExternalMCPManagementAndAIRouting(t *testing.T) {
	oldRead, oldWrite := externalMCPReadSetting, externalMCPWriteSetting
	defer func() { externalMCPReadSetting, externalMCPWriteSetting = oldRead, oldWrite }()
	var stored any
	externalMCPReadSetting = func(string) (any, error) { return stored, nil }
	externalMCPWriteSetting = func(_ string, v any) error {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &stored)
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("credential not preserved")
		}
		var req struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": mcpclient.ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}}
		case "notifications/initialized":
			w.WriteHeader(202)
			return
		case "tools/list":
			result = map[string]any{"tools": []mcpclient.Tool{{Name: "dirscan", Description: "Remote discovery", InputSchema: map[string]any{"type": "object"}}}}
		case "tools/call":
			if req.Params["name"] != "dirscan" {
				t.Error("alias was forwarded instead of original tool name")
			}
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "remote proof"}}}
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer remote.Close()
	router := gin.New()
	router.GET("/servers", ListExternalMCPServersHandler)
	router.POST("/servers", SaveExternalMCPServerHandler)
	router.PUT("/servers/:id", SaveExternalMCPServerHandler)
	router.DELETE("/servers/:id", DeleteExternalMCPServerHandler)
	router.POST("/servers/:id/discover", DiscoverExternalMCPServerHandler)
	request := func(method, path string, body any, status int) map[string]any {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		if out.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, out.Code, out.Body.String())
		}
		if strings.Contains(out.Body.String(), "test-secret") {
			t.Fatal("secret leaked in API response")
		}
		var result map[string]any
		json.Unmarshal(out.Body.Bytes(), &result)
		return result
	}
	created := request("POST", "/servers", map[string]any{"name": "Remote", "url": remote.URL, "headers": map[string]string{"Authorization": "Bearer test-secret"}}, 200)
	id := created["data"].(map[string]any)["id"].(string)
	path := "/servers/" + id
	request("PUT", path, map[string]any{"name": "Remote", "url": remote.URL, "enabled": true}, 400)
	request("POST", path+"/discover", nil, 200)
	request("PUT", path, map[string]any{"name": "Remote", "url": remote.URL, "enabled": true}, 200)
	servers, err := mcptools.LoadServers()
	if err != nil {
		t.Fatal(err)
	}
	schemas := mcptools.ExternalSchemas(servers)
	if len(schemas) != 1 || schemas[0].Function.Name == "dirscan" {
		t.Fatalf("schemas: %+v", schemas)
	}
	alias := schemas[0].Function.Name
	cfg := pentest.AgentLoopConfig{ToolAllowlist: []string{alias}, NativeTools: schemas, NativeToolRunner: mcptools.RunTool}
	if len(pentest.BuildAgentToolSchemasForConfig(cfg)) != 1 {
		t.Fatal("external tool unavailable to model")
	}
	call := pentest.AgentToolCall{Function: pentest.AgentFunctionCall{Name: alias, Arguments: map[string]any{}}}
	result, handled, err := mcptools.RunTool(context.Background(), cfg, call)
	if err != nil || !handled || !strings.Contains(fmt.Sprint(result), "remote proof") {
		t.Fatalf("AI route: %v %v %v", result, handled, err)
	}
	request("GET", "/servers", nil, 200)
	request("PUT", path, map[string]any{"name": "Remote", "url": remote.URL, "enabled": false}, 200)
	if _, handled, err = mcptools.RunTool(context.Background(), cfg, call); !handled || err == nil {
		t.Fatal("disabled tool still executes")
	}
	request("PUT", path, map[string]any{"name": "Remote", "url": remote.URL + "/changed", "enabled": true}, 200)
	servers, _ = mcptools.LoadServers()
	if servers[0].Enabled || len(servers[0].Tools) > 0 || servers[0].CheckedAt != "" {
		t.Fatal("edited connection retained stale enabled tools")
	}
	request("DELETE", path, nil, 200)
	if _, handled, err = mcptools.RunTool(context.Background(), cfg, call); !handled || err == nil {
		t.Fatal("deleted tool still executes")
	}
}

func TestAICapabilitySwitches(t *testing.T) {
	oldRead, oldWrite := externalMCPReadSetting, externalMCPWriteSetting
	defer func() { externalMCPReadSetting, externalMCPWriteSetting = oldRead, oldWrite }()
	stored := map[string]any{}
	externalMCPReadSetting = func(key string) (any, error) { return stored[key], nil }
	externalMCPWriteSetting = func(key string, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		var copy any
		if err = json.Unmarshal(data, &copy); err == nil {
			stored[key] = copy
		}
		return err
	}
	router := gin.New()
	router.GET("/tools", ListSystemToolsHandler)
	router.PUT("/tools/:name", SetAIToolEnabledHandler)
	set := func(name string, enabled bool) {
		t.Helper()
		body, _ := json.Marshal(map[string]bool{"enabled": enabled})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/tools/"+name, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	catalog, err := pentest.LoadSystemTools("")
	if err != nil {
		t.Fatal(err)
	}
	cfg := pentest.AgentLoopConfig{ToolCatalog: catalog}
	set("http-framework-test", false)
	set("exec", false)
	set("dnslog", true)
	if err := mcptools.RefreshTools(&cfg); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range pentest.BuildAgentToolSchemasForConfig(cfg) {
		names[tool.Function.Name] = true
	}
	if names["http-framework-test"] || names["exec"] || names["attack_surface_create_jsscan_task"] || !names["dnslog"] {
		t.Fatalf("incorrect schemas: %v", names)
	}
	if mcptools.AuthorizeTool("exec") == nil || mcptools.AuthorizeTool("http-framework-test") == nil || mcptools.AuthorizeTool("dnslog") != nil {
		t.Fatal("execution policy did not follow switches")
	}
	set("exec", true)
	if err := mcptools.RefreshTools(&cfg); err != nil {
		t.Fatal(err)
	}
	if mcptools.AuthorizeTool("exec") != nil {
		t.Fatal("re-enable failed")
	}
	names = map[string]bool{}
	for _, tool := range pentest.BuildAgentToolSchemasForConfig(cfg) {
		names[tool.Function.Name] = true
	}
	if names["attack_surface_create_jsscan_task"] {
		t.Fatal("JSScan creation should remain hidden")
	}
	for _, name := range pentest.DefaultPentestToolAllowlist() {
		if mcptools.IsJSScanTool(name) {
			continue
		}
		set(name, false)
	}
	set("dnslog", false)
	if err := mcptools.RefreshTools(&cfg); err != nil {
		t.Fatal(err)
	}
	if tools := pentest.BuildAgentToolSchemasForConfig(cfg); len(tools) != 0 {
		t.Fatalf("all disabled exposed tools: %v", tools)
	}
}

func TestExternalMCPIndividualToolDisabled(t *testing.T) {
	server := mcpclient.Server{ID: "test", Name: "test", Enabled: true, Tools: []mcpclient.Tool{{Name: "hidden"}, {Name: "visible"}}, DisabledTools: []string{"hidden"}}
	schemas := mcptools.ExternalSchemas([]mcpclient.Server{server})
	if len(schemas) != 1 || schemas[0].Function.Name != mcpclient.Alias("test", "visible") {
		t.Fatal("disabled tool exposed")
	}
	old := externalMCPReadSetting
	defer func() { externalMCPReadSetting = old }()
	externalMCPReadSetting = func(string) (any, error) { return []mcpclient.Server{server}, nil }
	call := pentest.AgentToolCall{}
	call.Function.Name = mcpclient.Alias("test", "hidden")
	_, handled, err := mcptools.RunTool(context.Background(), pentest.AgentLoopConfig{}, call)
	if !handled || err == nil {
		t.Fatal("disabled tool was not rejected")
	}
}

func TestLocalMCPConfigCredentialsAndInvalidation(t *testing.T) {
	oldRead, oldWrite := externalMCPReadSetting, externalMCPWriteSetting
	defer func() { externalMCPReadSetting, externalMCPWriteSetting = oldRead, oldWrite }()
	stored := []mcpclient.Server{{ID: "local", Name: "local", Transport: "stdio", Command: "server", Args: []string{"argument with spaces"}, Env: map[string]string{"SECRET": "hidden-value"}, CheckedAt: "checked", Enabled: true, Tools: []mcpclient.Tool{{Name: "test"}}}}
	externalMCPReadSetting = func(string) (any, error) { return stored, nil }
	externalMCPWriteSetting = func(_ string, value any) error { stored = value.([]mcpclient.Server); return nil }
	router := gin.New()
	router.PUT("/servers/:id", SaveExternalMCPServerHandler)
	update := func(body string) {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/servers/local", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		if strings.Contains(w.Body.String(), "hidden-value") {
			t.Fatal("environment secret exposed")
		}
	}
	update(`{"name":"local","enabled":false}`)
	if stored[0].Env["SECRET"] != "hidden-value" || stored[0].CheckedAt != "checked" || len(stored[0].Args) != 1 {
		t.Fatal("toggle lost connection state")
	}
	update(`{"name":"local","enabled":true,"command":"new-server"}`)
	if stored[0].Enabled || stored[0].CheckedAt != "" || len(stored[0].Tools) != 0 {
		t.Fatal("changed connection retained cached tools")
	}
	update(`{"name":"local","env":{}}`)
	if len(stored[0].Env) != 0 {
		t.Fatal("explicit environment clear failed")
	}
}

func TestAIToolCallCountsSeparatesExternalServers(t *testing.T) {
	oldCounts, oldRead := loadAIToolCallCounts, externalMCPReadSetting
	defer func() { loadAIToolCallCounts, externalMCPReadSetting = oldCounts, oldRead }()
	loadAIToolCallCounts = func(context.Context) (map[string]int64, error) {
		return map[string]int64{
			"http-framework-test": 7, mcpclient.Alias("one", "echo"): 3, mcpclient.Alias("two", "echo"): 5,
		}, nil
	}
	externalMCPReadSetting = func(string) (any, error) {
		return []mcpclient.Server{
			{ID: "one", Tools: []mcpclient.Tool{{Name: "echo"}, {Name: "unused"}}},
			{ID: "two", Tools: []mcpclient.Tool{{Name: "echo"}}},
		}, nil
	}
	router := gin.New()
	router.GET("/counts", AIToolCallCountsHandler)
	out := httptest.NewRecorder()
	router.ServeHTTP(out, httptest.NewRequest("GET", "/counts", nil))
	if out.Code != 200 {
		t.Fatal(out.Body.String())
	}
	var payload struct {
		Data struct {
			Tools    map[string]int64            `json:"tools"`
			External map[string]map[string]int64 `json:"external"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Tools["http-framework-test"] != 7 || payload.Data.External["one"]["echo"] != 3 || payload.Data.External["two"]["echo"] != 5 || payload.Data.External["one"]["unused"] != 0 {
		t.Fatal(out.Body.String())
	}
	loadAIToolCallCounts = func(context.Context) (map[string]int64, error) { return nil, fmt.Errorf("database unavailable") }
	out = httptest.NewRecorder()
	router.ServeHTTP(out, httptest.NewRequest("GET", "/counts", nil))
	if out.Code != 500 {
		t.Fatal("failed count query reported success")
	}
}
