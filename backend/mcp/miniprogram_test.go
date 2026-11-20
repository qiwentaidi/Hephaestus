package mcp

import (
	"testing"

	"hephaestus-backend/pkg/config"
)

func TestMiniProgramMCPToolsAreDesktopOnly(t *testing.T) {
	previous := config.GetRuntimeMode()
	t.Cleanup(func() { config.SetRuntimeMode(previous) })

	names := func() map[string]bool {
		result := map[string]bool{}
		for _, tool := range mcpToolsForProfile(mcpHephaestusProfile) {
			result[tool.Name] = true
		}
		return result
	}

	config.SetRuntimeMode(config.RuntimeModeServer)
	serverNames := names()
	for _, name := range []string{"miniprogram_list_programs", "miniprogram_decompile"} {
		if serverNames[name] {
			t.Fatalf("%s must not be exposed by the server MCP endpoint", name)
		}
	}

	config.SetRuntimeMode(config.RuntimeModeDesktop)
	desktopNames := names()
	for _, name := range []string{"miniprogram_list_programs", "miniprogram_decompile"} {
		if !desktopNames[name] {
			t.Fatalf("%s is missing from the desktop MCP endpoint", name)
		}
	}
}
