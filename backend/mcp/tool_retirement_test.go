package mcp

import (
	"encoding/json"
	"fmt"
	"testing"
)

// These names belonged to the retired ai_tasks workbench. Both discovery and
// dispatch must reject them, including clients that cached the old catalog.
func TestRetiredWorkbenchToolsAreUnavailable(t *testing.T) {
	retired := []string{
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
		"api_doc_test",
		"exec_run",
		"python_execute",
		"workbench_mvp_flow_create",
	}
	catalog := make(map[string]bool)
	for _, tool := range mcpCatalogTools() {
		catalog[tool.Name] = true
	}
	for _, name := range retired {
		if catalog[name] {
			t.Errorf("retired tool %s remains in the catalog", name)
		}
	}
	for _, profile := range []mcpServerProfile{mcpHephaestusProfile, mcpSpaceEngineProfile} {
		t.Run(profile.Key, func(t *testing.T) {
			advertised := make(map[string]bool)
			for _, tool := range mcpToolsForProfile(profile) {
				advertised[tool.Name] = true
			}
			for _, name := range retired {
				t.Run(name, func(t *testing.T) {
					if advertised[name] {
						t.Fatalf("retired tool %s is still advertised", name)
					}
					_, err := callMCPTool(nil, name, json.RawMessage(`{}`), profile)
					if want := fmt.Sprintf("不支持的工具: %s", name); err == nil || err.Error() != want {
						t.Fatalf("cached tool call returned %v, want %q", err, want)
					}
				})
			}
		})
	}
}
