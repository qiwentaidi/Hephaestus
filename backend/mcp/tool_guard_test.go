package mcp

import (
	"encoding/json"
	"testing"
)

func TestPublicMCPDoesNotExposeCommandExecutionTools(t *testing.T) {
	for toolName := range externallyBlockedMCPTools {
		_, err := callMCPTool(nil, toolName, json.RawMessage(`{}`), mcpHephaestusProfile)
		if err == nil {
			t.Fatalf("public MCP must reject command execution tool %s", toolName)
		}
	}
}
