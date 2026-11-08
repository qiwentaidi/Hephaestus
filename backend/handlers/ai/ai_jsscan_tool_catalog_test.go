package ai

import (
	"testing"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcptools"
)

func TestRemovePentestJSScanTools(t *testing.T) {
	tools := []pentest.AgentToolSchema{
		{Function: pentest.AgentToolFunction{Name: "attack_surface_create_jsscan_task"}},
		{Function: pentest.AgentToolFunction{Name: "attack_surface_get_jsscan_overview"}},
		{Function: pentest.AgentToolFunction{Name: "attack_surface_read_reference"}},
	}
	got := mcptools.RemoveJSScanTools(tools)
	if len(got) != 1 || got[0].Function.Name != "attack_surface_read_reference" {
		t.Fatalf("unexpected filtered tools: %#v", got)
	}
}

func TestRemovePentestJSScanCreateToolKeepsReaders(t *testing.T) {
	tools := []pentest.AgentToolSchema{
		{Function: pentest.AgentToolFunction{Name: "attack_surface_create_jsscan_task"}},
		{Function: pentest.AgentToolFunction{Name: "attack_surface_get_jsscan_overview"}},
		{Function: pentest.AgentToolFunction{Name: "attack_surface_get_jsscan_evidence"}},
	}
	got := mcptools.RemoveJSScanCreateTool(tools)
	if len(got) != 2 || got[0].Function.Name != "attack_surface_get_jsscan_overview" || got[1].Function.Name != "attack_surface_get_jsscan_evidence" {
		t.Fatalf("unexpected reader tools: %#v", got)
	}
}
