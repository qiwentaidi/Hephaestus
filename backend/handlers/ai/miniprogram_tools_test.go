package ai

import (
	"testing"

	"hephaestus-backend/pkg/config"
)

func TestMiniProgramNativeToolsAreDesktopOnly(t *testing.T) {
	previous := config.GetRuntimeMode()
	t.Cleanup(func() { config.SetRuntimeMode(previous) })

	names := func() map[string]bool {
		result := map[string]bool{}
		for _, schema := range PentestNativeMCPToolSchemas() {
			result[schema.Function.Name] = true
		}
		return result
	}

	config.SetRuntimeMode(config.RuntimeModeServer)
	serverNames := names()
	for _, name := range []string{"miniprogram_list_programs", "miniprogram_decompile"} {
		if serverNames[name] {
			t.Fatalf("%s must not be exposed in server mode", name)
		}
	}

	config.SetRuntimeMode(config.RuntimeModeDesktop)
	desktopNames := names()
	for _, name := range []string{"miniprogram_list_programs", "miniprogram_decompile"} {
		if !desktopNames[name] {
			t.Fatalf("%s is missing from desktop AI capabilities", name)
		}
	}
}
