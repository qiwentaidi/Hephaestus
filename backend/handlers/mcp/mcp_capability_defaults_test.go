package mcphandlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestListSystemToolsEnablesDefaultNativeCapabilities(t *testing.T) {
	t.Setenv("HEPHAESTUS_TOOLS_ROOT", "")
	t.Chdir(t.TempDir()) // A packaged desktop app starts outside the source tree.
	oldRead := externalMCPReadSetting
	defer func() { externalMCPReadSetting = oldRead }()
	externalMCPReadSetting = func(string) (any, error) { return nil, nil }

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/tools", nil)
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	ListSystemToolsHandler(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ListSystemToolsHandler status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success {
		t.Fatal("system tools response was unsuccessful")
	}
	enabled := map[string]bool{}
	for _, tool := range response.Data {
		enabled[tool.Name] = tool.Enabled
	}
	for _, name := range []string{"space_engine_search"} {
		if !enabled[name] {
			t.Fatalf("default native capability %q is not enabled: %#v", name, enabled)
		}
	}
	for _, name := range []string{"attack_surface_search_jsscan_resources", "attack_surface_create_jsscan_task"} {
		if _, ok := enabled[name]; ok {
			t.Fatalf("internal JSScan capability should be hidden: %#v", enabled)
		}
	}
	if _, ok := enabled["execute-python-script"]; !ok {
		t.Fatal("bundled system tool is missing from the response")
	}
}
