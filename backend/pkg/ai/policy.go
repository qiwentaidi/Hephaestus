package ai

const (
	WorkbenchRiskReadOnly       = "read_only"
	WorkbenchRiskManualApproval = "manual_approval"
	WorkbenchRiskAggressive     = "aggressive"
)

// WorkbenchActionRequiresApproval 判断指定风险等级在当前策略下是否需要人工审批。
func WorkbenchActionRequiresApproval(riskPolicy string, riskLevel string) bool {
	if riskPolicy == WorkbenchRiskAggressive {
		return false
	}
	if riskPolicy == WorkbenchRiskReadOnly {
		return riskLevel != "low"
	}
	return riskLevel == "medium" || riskLevel == "high"
}
