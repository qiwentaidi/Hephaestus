package ai

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// WorkbenchTargetProfile is the normalized target-shape summary used before
// the workbench creates executable steps.
type WorkbenchTargetProfile struct {
	Kind      string   `json:"kind"`
	HasWeb    bool     `json:"hasWeb"`
	HasCIDR   bool     `json:"hasCidr"`
	HasHost   bool     `json:"hasHost"`
	HasDomain bool     `json:"hasDomain"`
	Targets   []string `json:"targets"`
	Summary   string   `json:"summary"`
}

// WorkbenchTaskPlanStep is the model/rule-neutral representation of an AI
// workbench step before the handler assigns database IDs.
type WorkbenchTaskPlanStep struct {
	StepType  string `json:"stepType"`
	Title     string `json:"title"`
	Rationale string `json:"rationale"`
	RiskLevel string `json:"riskLevel"`
}

// WorkbenchTaskPlanningEvidence is the compact planning context sent to the
// model when initial task orchestration is enabled.
type WorkbenchTaskPlanningEvidence struct {
	TaskName        string
	Target          string
	Scope           []string
	Mode            string
	RiskPolicy      string
	Objective       string
	SuccessCriteria string
	StrategyPacks   []string
	TargetProfile   WorkbenchTargetProfile
	RulePlan        []WorkbenchTaskPlanStep
	HistoryFeedback []string
}

// WorkbenchTaskPlanningResult is the structured model output for initial task
// orchestration.
type WorkbenchTaskPlanningResult struct {
	Steps   []WorkbenchTaskPlanStep `json:"steps"`
	Summary string                  `json:"summary"`
}

func DetectWorkbenchTargetProfile(target string, scope []string) WorkbenchTargetProfile {
	targets := normalizeWorkbenchScanTargets(target, scope)
	profile := WorkbenchTargetProfile{
		Kind:    "unknown",
		Targets: targets,
	}
	for _, item := range targets {
		host := hostFromWorkbenchTarget(item)
		switch {
		case isWorkbenchWebTarget(item):
			profile.HasWeb = true
		case isWorkbenchCIDR(item):
			profile.HasCIDR = true
		case net.ParseIP(host) != nil:
			profile.HasHost = true
		case host != "":
			profile.HasDomain = true
		}
	}
	switch {
	case profile.HasCIDR:
		profile.Kind = "cidr"
	case profile.HasWeb && (profile.HasHost || profile.HasDomain):
		profile.Kind = "mixed"
	case profile.HasWeb:
		profile.Kind = "web_url"
	case profile.HasHost:
		profile.Kind = "host"
	case profile.HasDomain:
		profile.Kind = "domain"
	}
	profile.Summary = buildWorkbenchTargetProfileSummary(profile)
	return profile
}

func buildWorkbenchTargetProfileSummary(profile WorkbenchTargetProfile) string {
	parts := []string{fmt.Sprintf("目标类型: %s", profile.Kind)}
	if profile.HasCIDR {
		parts = append(parts, "包含 CIDR 网段")
	}
	if profile.HasHost {
		parts = append(parts, "包含 IP/主机")
	}
	if profile.HasDomain {
		parts = append(parts, "包含域名")
	}
	if profile.HasWeb {
		parts = append(parts, "包含 HTTP/HTTPS URL")
	}
	if len(profile.Targets) > 0 {
		parts = append(parts, "规范化目标: "+strings.Join(profile.Targets, ", "))
	}
	return strings.Join(parts, "；")
}

func BuildWorkbenchRuleInitialPlan(mode string, target string, scope []string, riskPolicy string, strategyPacks []string) []WorkbenchTaskPlanStep {
	profile := DetectWorkbenchTargetProfile(target, scope)
	discoveryFirst := WorkbenchTargetNeedsDiscoveryFirst(profile)
	plan := make([]WorkbenchTaskPlanStep, 0, 7)
	add := func(stepType string, title string, rationale string, riskLevel string) {
		plan = append(plan, WorkbenchTaskPlanStep{
			StepType:  stepType,
			Title:     title,
			Rationale: rationale,
			RiskLevel: riskLevel,
		})
	}

	add("scope_review", "建立授权范围和任务上下文", "先明确目标、白名单、限制条件和成功标准，避免后续动作越界。", "low")
	add("asset_context", "汇总资产与已有扫描线索", "把现有模块输出收敛为 AI 可读的统一资产上下文。", "low")
	if discoveryFirst {
		add("tool_orchestration", "识别资产与开放服务", "当前目标不是明确的 Web URL，先执行资产发现、端口扫描或服务识别，再派生可观察的 Web 入口。", "medium")
		add("http_observation", "观察已识别 Web 入口", "仅对已授权且可确认为 HTTP/HTTPS 的入口进行只读观察，避免把网段或裸 IP 当成站点。", "low")
	} else {
		add("http_observation", "执行 HTTP 只读观察", "对授权 URL 进行低侵入页面观察，提取标题、响应头、表单、脚本和接口线索。", "low")
		add("tool_orchestration", "调度常规网站扫描能力", "默认先执行 webscanOption=2 且 deepScan=true 的全指纹资产发现，并按证据需要补充目录、JS/API 或后续漏洞验证。", "medium")
	}
	add("hypothesis", "生成发散性测试假设", "基于站点类型、路径、指纹、接口、开放服务和历史证据提出值得验证的安全假设。", "low")
	add("verification", "验证高价值候选问题", "只对已授权范围内的候选风险做最小化验证，并保留复现证据。", "high")
	add("goal_evaluation", "执行目标完成度评估并生成交付物", "根据成功标准检查缺口，输出报告、证据清单、未验证线索和下一步建议。", "low")

	_ = mode
	_ = riskPolicy
	_ = strategyPacks
	return plan
}

func WorkbenchTargetNeedsDiscoveryFirst(profile WorkbenchTargetProfile) bool {
	if profile.HasCIDR || profile.HasHost || profile.HasDomain {
		return true
	}
	return !profile.HasWeb
}

func BuildWorkbenchTaskPlanningPrompt(evidence WorkbenchTaskPlanningEvidence) string {
	ruleLines := make([]string, 0, len(evidence.RulePlan))
	for index, step := range evidence.RulePlan {
		ruleLines = append(ruleLines, fmt.Sprintf("%d. %s [%s/%s] %s", index+1, step.StepType, step.RiskLevel, step.Title, step.Rationale))
	}
	sections := []string{
		"请作为 Hephaestus AI 智脑的任务级规划器，根据目标形态、策略包和成功标准编排初始步骤。",
		"只能使用这些 stepType: scope_review, asset_context, tool_orchestration, http_observation, hypothesis, verification, goal_evaluation。",
		"不要编造未实现的 stepType，不要生成命令、Payload 或漏洞利用步骤；中高风险步骤必须保留对应风险等级。",
		"如果目标是 CIDR、裸 IP、根域名或混合资产，必须先 tool_orchestration 做资产/服务发现，再 http_observation；只有明确 HTTP/HTTPS URL 才可以先 http_observation。",
		"必须包含 scope_review、asset_context、goal_evaluation；goal_evaluation 必须在最后。",
		"只输出 JSON，不要输出 Markdown、代码块或额外解释。JSON schema: {\"steps\":[{\"stepType\":\"...\",\"title\":\"...\",\"rationale\":\"...\",\"riskLevel\":\"low|medium|high\"}],\"summary\":\"...\"}",
		fmt.Sprintf("任务: %s", evidence.TaskName),
		fmt.Sprintf("目标: %s", evidence.Target),
		fmt.Sprintf("范围: %s", strings.Join(evidence.Scope, ", ")),
		fmt.Sprintf("模式: %s", evidence.Mode),
		fmt.Sprintf("风险策略: %s", evidence.RiskPolicy),
		fmt.Sprintf("目标说明: %s", evidence.Objective),
		fmt.Sprintf("成功标准: %s", evidence.SuccessCriteria),
		fmt.Sprintf("策略包: %s", strings.Join(evidence.StrategyPacks, ", ")),
		fmt.Sprintf("目标识别: %s", evidence.TargetProfile.Summary),
		"规则兜底计划:\n" + joinOrDash(ruleLines),
		"历史效果反馈:\n" + joinOrDash(evidence.HistoryFeedback),
	}
	return strings.Join(sections, "\n")
}

func ParseWorkbenchTaskPlanningResult(reply string) (WorkbenchTaskPlanningResult, bool) {
	payload := extractWorkbenchJSONPayload(reply)
	if payload == "" {
		return WorkbenchTaskPlanningResult{}, false
	}
	var parsed WorkbenchTaskPlanningResult
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return WorkbenchTaskPlanningResult{}, false
	}
	parsed.Summary = strings.TrimSpace(parsed.Summary)
	cleaned := make([]WorkbenchTaskPlanStep, 0, len(parsed.Steps))
	for _, step := range parsed.Steps {
		step.StepType = normalizeWorkbenchStepType(step.StepType)
		step.Title = strings.TrimSpace(step.Title)
		step.Rationale = strings.TrimSpace(step.Rationale)
		step.RiskLevel = normalizeWorkbenchRiskLevel(step.RiskLevel)
		if step.StepType == "" {
			continue
		}
		if step.Title == "" {
			step.Title = defaultWorkbenchStepTitle(step.StepType)
		}
		if step.Rationale == "" {
			step.Rationale = step.Title
		}
		cleaned = append(cleaned, step)
		if len(cleaned) >= 8 {
			break
		}
	}
	if len(cleaned) == 0 {
		return WorkbenchTaskPlanningResult{}, false
	}
	parsed.Steps = cleaned
	return parsed, true
}

func MergeWorkbenchTaskPlanSteps(profile WorkbenchTargetProfile, modelSteps []WorkbenchTaskPlanStep, ruleSteps []WorkbenchTaskPlanStep) ([]WorkbenchTaskPlanStep, bool) {
	if len(modelSteps) == 0 {
		return normalizeWorkbenchTaskPlan(ruleSteps, profile), false
	}
	merged := make([]WorkbenchTaskPlanStep, 0, len(modelSteps)+len(ruleSteps))
	seen := make(map[string]bool, len(modelSteps)+len(ruleSteps))
	add := func(step WorkbenchTaskPlanStep) {
		step.StepType = normalizeWorkbenchStepType(step.StepType)
		if step.StepType == "" || seen[step.StepType] {
			return
		}
		step.Title = strings.TrimSpace(step.Title)
		if step.Title == "" {
			step.Title = defaultWorkbenchStepTitle(step.StepType)
		}
		step.Rationale = strings.TrimSpace(step.Rationale)
		if step.Rationale == "" {
			step.Rationale = step.Title
		}
		step.RiskLevel = normalizeWorkbenchRiskLevel(step.RiskLevel)
		seen[step.StepType] = true
		merged = append(merged, step)
	}
	for _, step := range modelSteps {
		add(step)
	}
	for _, step := range ruleSteps {
		add(step)
	}
	normalized := normalizeWorkbenchTaskPlan(merged, profile)
	return normalized, len(normalized) > 0
}

func normalizeWorkbenchTaskPlan(plan []WorkbenchTaskPlanStep, profile WorkbenchTargetProfile) []WorkbenchTaskPlanStep {
	byType := make(map[string]WorkbenchTaskPlanStep, len(plan))
	for _, step := range plan {
		step.StepType = normalizeWorkbenchStepType(step.StepType)
		if step.StepType == "" {
			continue
		}
		if step.Title == "" {
			step.Title = defaultWorkbenchStepTitle(step.StepType)
		}
		if step.Rationale == "" {
			step.Rationale = step.Title
		}
		step.RiskLevel = normalizeWorkbenchRiskLevel(step.RiskLevel)
		byType[step.StepType] = step
	}
	required := []string{"scope_review", "asset_context"}
	if WorkbenchTargetNeedsDiscoveryFirst(profile) {
		required = append(required, "tool_orchestration", "http_observation")
	} else {
		required = append(required, "http_observation", "tool_orchestration")
	}
	required = append(required, "hypothesis", "verification", "goal_evaluation")
	result := make([]WorkbenchTaskPlanStep, 0, len(required))
	for _, stepType := range required {
		step, ok := byType[stepType]
		if !ok {
			step = defaultWorkbenchPlanStep(stepType, WorkbenchTargetNeedsDiscoveryFirst(profile))
		}
		result = append(result, step)
	}
	return result
}

func normalizeWorkbenchStepType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "scope_review", "asset_context", "tool_orchestration", "http_observation", "hypothesis", "verification", "goal_evaluation":
		return value
	default:
		return ""
	}
}

func normalizeWorkbenchRiskLevel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "low", "medium", "high":
		return value
	default:
		return "low"
	}
}

func defaultWorkbenchPlanStep(stepType string, discoveryFirst bool) WorkbenchTaskPlanStep {
	switch stepType {
	case "scope_review":
		return WorkbenchTaskPlanStep{StepType: stepType, Title: "建立授权范围和任务上下文", Rationale: "先明确目标、白名单、限制条件和成功标准，避免后续动作越界。", RiskLevel: "low"}
	case "asset_context":
		return WorkbenchTaskPlanStep{StepType: stepType, Title: "汇总资产与已有扫描线索", Rationale: "把现有模块输出收敛为 AI 可读的统一资产上下文。", RiskLevel: "low"}
	case "tool_orchestration":
		if discoveryFirst {
			return WorkbenchTaskPlanStep{StepType: stepType, Title: "识别资产与开放服务", Rationale: "当前目标不是明确的 Web URL，先执行资产发现、端口扫描或服务识别。", RiskLevel: "medium"}
		}
		return WorkbenchTaskPlanStep{StepType: stepType, Title: "调度常规网站扫描能力", Rationale: "默认先执行 webscanOption=2 且 deepScan=true 的全指纹资产发现，并按证据需要补充目录、JS/API 或后续漏洞验证。", RiskLevel: "medium"}
	case "http_observation":
		if discoveryFirst {
			return WorkbenchTaskPlanStep{StepType: stepType, Title: "观察已识别 Web 入口", Rationale: "仅对已授权且可确认为 HTTP/HTTPS 的入口进行只读观察。", RiskLevel: "low"}
		}
		return WorkbenchTaskPlanStep{StepType: stepType, Title: "执行 HTTP 只读观察", Rationale: "对授权 URL 进行低侵入页面观察。", RiskLevel: "low"}
	case "hypothesis":
		return WorkbenchTaskPlanStep{StepType: stepType, Title: "生成发散性测试假设", Rationale: "基于上下文提出值得验证的安全假设。", RiskLevel: "low"}
	case "verification":
		return WorkbenchTaskPlanStep{StepType: stepType, Title: "验证高价值候选问题", Rationale: "只对已授权范围内的候选风险做最小化验证。", RiskLevel: "high"}
	default:
		return WorkbenchTaskPlanStep{StepType: "goal_evaluation", Title: "执行目标完成度评估并生成交付物", Rationale: "根据成功标准检查缺口并生成交付物。", RiskLevel: "low"}
	}
}

func defaultWorkbenchStepTitle(stepType string) string {
	return defaultWorkbenchPlanStep(stepType, false).Title
}

func WorkbenchWebURLsFromPortResults(ports []int, host string, protocol string, product string) []string {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	result := make([]string, 0, len(ports))
	for _, port := range ports {
		if !isLikelyWorkbenchHTTPPort(port, protocol, product) {
			continue
		}
		scheme := "http"
		text := strings.ToLower(protocol + " " + product)
		if strings.Contains(text, "https") || port == 443 || port == 8443 {
			scheme = "https"
		}
		targetHost := host
		if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
			targetHost = "[" + host + "]"
		}
		if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
			result = append(result, fmt.Sprintf("%s://%s", scheme, targetHost))
		} else {
			result = append(result, fmt.Sprintf("%s://%s:%d", scheme, targetHost, port))
		}
	}
	return result
}

func isLikelyWorkbenchHTTPPort(port int, protocol string, product string) bool {
	switch port {
	case 80, 443, 8080, 8443, 8000, 8888, 3000, 5000, 9000:
		return true
	}
	text := strings.ToLower(protocol + " " + product)
	return strings.Contains(text, "http") || strings.Contains(text, "nginx") || strings.Contains(text, "apache") || strings.Contains(text, "tomcat")
}

func NormalizeWorkbenchURLCandidate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return strings.TrimRight(parsed.String(), "/")
}
