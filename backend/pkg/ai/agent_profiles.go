package ai

import "strings"

const (
	WorkbenchAgentProfileDeepAgent = "deep_agent"
	WorkbenchAgentProfileAnalyst   = "analyst"
)

// WorkbenchAgentProfile describes the product-level agent persona selected for
// an AI workbench task. This is separate from the provider model name: the same
// DeepAgent profile can run on different configured LLM models.
type WorkbenchAgentProfile struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	DefaultMode           string   `json:"defaultMode"`
	DefaultRiskPolicy     string   `json:"defaultRiskPolicy"`
	DefaultDelivery       []string `json:"defaultDelivery"`
	PreferredStrategies   []string `json:"preferredStrategies"`
	PreferredCapabilities []string `json:"preferredCapabilities"`
	SystemAddendum        string   `json:"systemAddendum"`
}

func ListWorkbenchAgentProfiles() []WorkbenchAgentProfile {
	return []WorkbenchAgentProfile{
		{
			ID:                WorkbenchAgentProfileDeepAgent,
			Name:              "DeepAgent",
			Description:       "面向单目标 Web 深度测试的主代理，负责从对话意图出发持续规划、调度、验证和沉淀证据。",
			DefaultMode:       "deep_test",
			DefaultRiskPolicy: WorkbenchRiskManualApproval,
			DefaultDelivery: []string{
				"漏洞发现总结",
				"证据清单",
				"复测建议",
			},
			PreferredStrategies: []string{
				"single_web_deep_test",
				"js_api_analysis",
				"verification_report",
			},
			PreferredCapabilities: []string{
				"web.http.observe",
				"web.fingerprint",
				"web.dirscan",
				"web.jsscan",
				"web.vuln.verify",
				"report.markdown",
			},
			SystemAddendum: strings.Join([]string{
				"当前任务选择了 DeepAgent 工作模式。",
				"你不是普通聊天助手，而是授权渗透测试主代理；对话只是入口，AgentRun、工具执行、资产变化、发现和证据沉淀才是主体。",
				"默认围绕单个目标 URL 做渐进式深度测试：先校验授权范围和目标可达性，再低侵入观察页面/API/JS 线索，之后基于证据提出候选风险并在需要时请求审批。",
				"不要把过程输出堆成日志；每轮回复应说明当前阶段、已沉淀的事实、下一步动作和需要用户确认的风险点。",
			}, "\n"),
		},
		{
			ID:                WorkbenchAgentProfileAnalyst,
			Name:              "Analyst",
			Description:       "偏分析与报告整理的安全分析代理，适合复核已有证据、归并漏洞和生成交付物。",
			DefaultMode:       "review",
			DefaultRiskPolicy: WorkbenchRiskReadOnly,
			DefaultDelivery: []string{
				"证据清单",
				"漏洞发现总结",
			},
			PreferredStrategies: []string{
				"verification_report",
				"js_api_analysis",
			},
			PreferredCapabilities: []string{
				"report.markdown",
				"web.http.observe",
			},
			SystemAddendum: strings.Join([]string{
				"当前任务选择了 Analyst 工作模式。",
				"优先复核事实、归并证据、标注缺口和生成报告，不主动推进中高风险验证。",
			}, "\n"),
		},
	}
}

func DefaultWorkbenchAgentProfile() WorkbenchAgentProfile {
	for _, profile := range ListWorkbenchAgentProfiles() {
		if profile.ID == WorkbenchAgentProfileDeepAgent {
			return profile
		}
	}
	return ListWorkbenchAgentProfiles()[0]
}

func ResolveWorkbenchAgentProfile(id string) WorkbenchAgentProfile {
	id = NormalizeWorkbenchAgentProfileID(id)
	for _, profile := range ListWorkbenchAgentProfiles() {
		if profile.ID == id {
			return profile
		}
	}
	return DefaultWorkbenchAgentProfile()
}

func NormalizeWorkbenchAgentProfileID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.ReplaceAll(id, "-", "_")
	switch id {
	case "", "deep", "deepagent", "deep_agent":
		return WorkbenchAgentProfileDeepAgent
	case "analyst", "review", "report":
		return WorkbenchAgentProfileAnalyst
	default:
		return id
	}
}
