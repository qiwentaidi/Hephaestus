package ai

import "testing"

func TestWorkbenchActionRequiresApproval(t *testing.T) {
	tests := []struct {
		name      string
		policy    string
		riskLevel string
		want      bool
	}{
		{name: "read only allows low", policy: WorkbenchRiskReadOnly, riskLevel: "low", want: false},
		{name: "read only gates medium", policy: WorkbenchRiskReadOnly, riskLevel: "medium", want: true},
		{name: "manual gates medium", policy: WorkbenchRiskManualApproval, riskLevel: "medium", want: true},
		{name: "manual gates high", policy: WorkbenchRiskManualApproval, riskLevel: "high", want: true},
		{name: "manual allows low", policy: WorkbenchRiskManualApproval, riskLevel: "low", want: false},
		{name: "aggressive allows high", policy: WorkbenchRiskAggressive, riskLevel: "high", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorkbenchActionRequiresApproval(tt.policy, tt.riskLevel); got != tt.want {
				t.Fatalf("WorkbenchActionRequiresApproval(%q, %q) = %v, want %v", tt.policy, tt.riskLevel, got, tt.want)
			}
		})
	}
}
