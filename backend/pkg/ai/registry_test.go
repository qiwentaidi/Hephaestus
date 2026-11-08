package ai

import "testing"

func TestListWorkbenchToolsExposeDecisionMetadata(t *testing.T) {
	tools := ListWorkbenchTools()
	if len(tools) == 0 {
		t.Fatal("expected workbench tool registry")
	}
	for _, tool := range tools {
		if tool.Name == "" || tool.Category == "" || tool.RiskLevel == "" {
			t.Fatalf("tool missing identity metadata: %+v", tool)
		}
		if tool.Source == "" {
			t.Fatalf("tool %s missing source", tool.Name)
		}
		if len(tool.Capabilities) == 0 {
			t.Fatalf("tool %s missing capabilities", tool.Name)
		}
		if len(tool.ResultMapping) == 0 {
			t.Fatalf("tool %s missing result mapping", tool.Name)
		}
		if len(tool.InputSchema) == 0 {
			t.Fatalf("tool %s missing input schema", tool.Name)
		}
		if tool.Source == "mcp" && tool.MCPName == "" {
			t.Fatalf("mcp tool %s missing mcp name", tool.Name)
		}
	}
}
