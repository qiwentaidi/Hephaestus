package ai

// WorkbenchToolDescriptor 描述 AI 智脑可调度能力的治理元数据。
type WorkbenchToolDescriptor struct {
	Name            string                 `json:"name"`
	Title           string                 `json:"title"`
	Category        string                 `json:"category"`
	Description     string                 `json:"description"`
	RiskLevel       string                 `json:"riskLevel"`
	ApprovalDefault bool                   `json:"approvalDefault"`
	Capabilities    []string               `json:"capabilities"`
	Source          string                 `json:"source"`
	MCPName         string                 `json:"mcpName,omitempty"`
	AutoExecutable  bool                   `json:"autoExecutable"`
	ResultMapping   []string               `json:"resultMapping,omitempty"`
	InputSchema     map[string]interface{} `json:"inputSchema,omitempty"`
}
