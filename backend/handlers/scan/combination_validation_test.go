package scan

import "testing"

func TestDetectCombinationTargetInputType(t *testing.T) {
	t.Run("portscan first stage uses network input", func(t *testing.T) {
		inputType, err := detectCombinationTargetInputType([]combinationPipelineNode{
			{ModuleID: "portscan", StageIndex: 0},
			{ModuleID: "webscan", StageIndex: 1},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inputType != combinationTargetInputTypeNetwork {
			t.Fatalf("expected network input type, got %q", inputType)
		}
	})

	t.Run("web first stage uses web input", func(t *testing.T) {
		inputType, err := detectCombinationTargetInputType([]combinationPipelineNode{
			{ModuleID: "webscan", StageIndex: 0},
			{ModuleID: "jsscan", StageIndex: 1},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inputType != combinationTargetInputTypeWeb {
			t.Fatalf("expected web input type, got %q", inputType)
		}
	})

	t.Run("mixed first stage is rejected", func(t *testing.T) {
		_, err := detectCombinationTargetInputType([]combinationPipelineNode{
			{ModuleID: "webscan", StageIndex: 0},
			{ModuleID: "crack", StageIndex: 0},
		})
		if err == nil {
			t.Fatalf("expected error for mixed first-stage input types")
		}
	})
}

func TestValidateCombinationTargetsByInputType(t *testing.T) {
	t.Run("network input rejects urls", func(t *testing.T) {
		invalid := validateCombinationTargetsByInputType(
			[]string{"https://demo.com", "1.1.1.1", "example.com", "1.1.1.0/24"},
			combinationTargetInputTypeNetwork,
		)
		if len(invalid) != 1 || invalid[0] != "https://demo.com" {
			t.Fatalf("expected only https url to be invalid, got %#v", invalid)
		}
	})

	t.Run("web input requires http urls", func(t *testing.T) {
		invalid := validateCombinationTargetsByInputType(
			[]string{"https://demo.com", "demo.com", "1.1.1.1/24"},
			combinationTargetInputTypeWeb,
		)
		if len(invalid) != 2 {
			t.Fatalf("expected 2 invalid targets, got %#v", invalid)
		}
	})

	t.Run("service input requires crack protocol and port", func(t *testing.T) {
		invalid := validateCombinationTargetsByInputType(
			[]string{"ssh://1.1.1.1:22", "mysql://db.example.com:3306", "https://demo.com"},
			combinationTargetInputTypeService,
		)
		if len(invalid) != 1 || invalid[0] != "https://demo.com" {
			t.Fatalf("expected https target to be invalid, got %#v", invalid)
		}
	})
}
