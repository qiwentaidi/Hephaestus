package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// WorkbenchToolSelectionEvidence 是模型选择工具时可见的压缩上下文。
type WorkbenchToolSelectionEvidence struct {
	TaskName        string
	Target          string
	Mode            string
	RiskPolicy      string
	StrategyPacks   []string
	AvailableTools  []WorkbenchToolDescriptor
	Observations    []string
	EvidenceItems   []string
	AssetBuckets    []string
	HistoryFeedback []string
	SuccessCriteria string
}

// WorkbenchToolSelectionResult 是模型型工具选择器的结构化输出。
type WorkbenchToolSelectionResult struct {
	Tools   []WorkbenchToolSelectionItem `json:"tools"`
	Summary string                       `json:"summary"`
}

// WorkbenchToolSelectionItem 表示一个候选工具建议。
type WorkbenchToolSelectionItem struct {
	ToolName    string                 `json:"toolName"`
	Reason      string                 `json:"reason"`
	ConfigHints map[string]interface{} `json:"configHints"`
}

// BuildWorkbenchToolSelectionPrompt 构造只允许模型选择白名单工具的提示。
func BuildWorkbenchToolSelectionPrompt(evidence WorkbenchToolSelectionEvidence) string {
	toolLines := make([]string, 0, len(evidence.AvailableTools))
	for _, tool := range evidence.AvailableTools {
		mcpName := tool.MCPName
		if mcpName == "" {
			mcpName = "-"
		}
		toolLines = append(toolLines, fmt.Sprintf(
			"- %s [%s/%s source=%s mcp=%s auto=%t approvalDefault=%t result=%s]: %s",
			tool.Name,
			tool.Category,
			tool.RiskLevel,
			tool.Source,
			mcpName,
			tool.AutoExecutable,
			tool.ApprovalDefault,
			strings.Join(tool.ResultMapping, ","),
			tool.Description,
		))
	}
	sections := []string{
		"请作为 Hephaestus AI 智脑的工具选择器，根据当前证据选择下一步最有价值的工具。",
		"只能从可用工具清单中选择 toolName，不要编造工具，不要生成任意命令，不要生成攻击利用步骤。",
		workbenchNoCORSDetectionInstruction,
		"只输出 JSON，不要输出 Markdown、代码块或额外解释。JSON schema: {\"tools\":[{\"toolName\":\"...\",\"reason\":\"...\",\"configHints\":{\"rateLimit\":1,\"timeoutSeconds\":10,\"maxDepth\":2,\"maxRequests\":200,\"includePassiveOnly\":true}}],\"summary\":\"...\"}",
		fmt.Sprintf("任务: %s", evidence.TaskName),
		fmt.Sprintf("目标: %s", evidence.Target),
		fmt.Sprintf("模式: %s", evidence.Mode),
		fmt.Sprintf("风险策略: %s", evidence.RiskPolicy),
		fmt.Sprintf("策略包: %s", strings.Join(evidence.StrategyPacks, ", ")),
		fmt.Sprintf("成功标准: %s", evidence.SuccessCriteria),
		"可用工具:\n" + joinOrDash(toolLines),
		"观察摘要:\n" + joinOrDash(evidence.Observations),
		"标准证据:\n" + joinOrDash(evidence.EvidenceItems),
		"资产上下文:\n" + joinOrDash(evidence.AssetBuckets),
		"历史效果反馈:\n" + joinOrDash(evidence.HistoryFeedback),
		"选择规则：常规 Web 安全测试的前置资产发现默认选择 web.fingerprint；该工具使用 webscanOption=2 且 deepScan=true，补充 Web 指纹、WAF、主动目录、截图和资产线索，不调用 nuclei 漏洞检测。只有已经有高价值指纹、组件、路径或漏洞线索并进入验证阶段时，才选择 web.vuln.verify。",
		"选择规则：当观察或证据中出现 Swagger/OpenAPI/WSDL/GraphQL、/v2/api-docs、/v3/api-docs、swagger.json、openapi.json、?wsdl、/graphql 等接口文档线索时，优先选择 api.doc.test 做接口文档解析和专项测试用例生成；不要把接口文档发现简单等同于 web.vuln.verify。",
		"字段要求：tools 最多 5 个，按推荐执行顺序排列；优先选择能把结果沉淀为 observation/evidence/api_endpoint/api_relation/extracted_variable 的工具；auto=false 或 approvalDefault=true 的工具只作为待审批建议；reason 只解释为什么当前证据需要该工具；configHints 只能建议 rateLimit、timeoutSeconds、maxDepth、maxRequests、includePassiveOnly，不能包含目标、请求头、Cookie、Payload、命令或漏洞利用参数。",
	}
	return strings.Join(sections, "\n")
}

func ParseWorkbenchToolSelectionResult(reply string) (WorkbenchToolSelectionResult, bool) {
	payload := extractWorkbenchJSONPayload(reply)
	if payload == "" {
		return WorkbenchToolSelectionResult{}, false
	}
	var parsed WorkbenchToolSelectionResult
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return WorkbenchToolSelectionResult{}, false
	}
	parsed.Summary = strings.TrimSpace(parsed.Summary)
	cleaned := make([]WorkbenchToolSelectionItem, 0, len(parsed.Tools))
	seen := make(map[string]bool, len(parsed.Tools))
	for _, item := range parsed.Tools {
		item.ToolName = normalizeWorkbenchToolName(item.ToolName)
		item.Reason = strings.TrimSpace(item.Reason)
		if item.ToolName == "" || seen[item.ToolName] {
			continue
		}
		seen[item.ToolName] = true
		cleaned = append(cleaned, item)
		if len(cleaned) >= 5 {
			break
		}
	}
	if len(cleaned) == 0 {
		return WorkbenchToolSelectionResult{}, false
	}
	parsed.Tools = cleaned
	return parsed, true
}

func ApplyWorkbenchToolSelection(plans []WorkbenchScanExecutionPlan, selection WorkbenchToolSelectionResult) ([]WorkbenchScanExecutionPlan, bool) {
	return ApplyWorkbenchToolSelectionWithFeedback(plans, selection, nil)
}

func ApplyWorkbenchToolSelectionWithFeedback(plans []WorkbenchScanExecutionPlan, selection WorkbenchToolSelectionResult, historyFeedback []string) ([]WorkbenchScanExecutionPlan, bool) {
	if len(plans) == 0 || len(selection.Tools) == 0 {
		return plans, false
	}
	selectionByTool := make(map[string]WorkbenchToolSelectionItem, len(selection.Tools))
	for _, item := range selection.Tools {
		if item.ToolName == "" {
			continue
		}
		selectionByTool[item.ToolName] = item
	}
	if len(selectionByTool) == 0 {
		return plans, false
	}

	selected := make([]WorkbenchScanExecutionPlan, 0, len(plans))
	matchedKnownTool := false
	for _, plan := range plans {
		item, picked := selectionByTool[normalizeWorkbenchToolName(plan.ToolName)]
		if picked {
			matchedKnownTool = true
		}
		if plan.Optional && !picked {
			continue
		}
		if item.Reason != "" {
			plan.Reason = "模型建议: " + item.Reason + " " + plan.Reason
		}
		modelHints := sanitizeWorkbenchToolConfigHints(item.ConfigHints)
		feedbackHints := BuildWorkbenchToolConfigHintsFromFeedback(item.ToolName, historyFeedback)
		hints := mergeWorkbenchToolConfigHints(modelHints, feedbackHints)
		if len(hints) > 0 {
			if plan.Config == nil {
				plan.Config = map[string]interface{}{}
			}
			plan.Config["aiConfigHints"] = hints
			if len(feedbackHints) > 0 {
				plan.Reason = "执行反馈参数建议: " + formatWorkbenchToolConfigHints(feedbackHints) + " " + plan.Reason
			}
			if len(modelHints) > 0 {
				plan.Reason = "模型参数建议: " + formatWorkbenchToolConfigHints(modelHints) + " " + plan.Reason
			}
		}
		selected = append(selected, plan)
	}
	if !matchedKnownTool {
		return plans, false
	}
	if len(selected) == 0 {
		return plans, false
	}
	return selected, true
}

// DefaultWorkbenchExecutablePlans returns the conservative no-model execution
// set. Optional plans remain available to model/tool selection and explicit
// user requests, but are not part of the default Web baseline.
func DefaultWorkbenchExecutablePlans(plans []WorkbenchScanExecutionPlan) []WorkbenchScanExecutionPlan {
	if len(plans) == 0 {
		return plans
	}
	selected := make([]WorkbenchScanExecutionPlan, 0, len(plans))
	for _, plan := range plans {
		if plan.Optional {
			continue
		}
		selected = append(selected, plan)
	}
	if len(selected) == 0 {
		return plans
	}
	return selected
}

// BuildWorkbenchToolConfigHintsFromFeedback 根据同目标历史工具执行效果生成保守参数建议。
func BuildWorkbenchToolConfigHintsFromFeedback(toolName string, historyFeedback []string) map[string]interface{} {
	toolName = normalizeWorkbenchToolName(toolName)
	if toolName == "" || len(historyFeedback) == 0 {
		return nil
	}
	stats := collectWorkbenchToolEffectStats(toolName, historyFeedback)
	if stats.total == 0 {
		return nil
	}
	if stats.failed > 0 && stats.failed >= stats.completed {
		return map[string]interface{}{
			"rateLimit":          1,
			"timeoutSeconds":     30,
			"maxRequests":        200,
			"includePassiveOnly": true,
		}
	}
	if stats.completed > 0 && stats.failed == 0 && stats.running == 0 {
		return map[string]interface{}{
			"timeoutSeconds": 10,
			"maxRequests":    1000,
		}
	}
	if stats.running > 0 && stats.completed == 0 {
		return map[string]interface{}{
			"rateLimit":      2,
			"timeoutSeconds": 45,
			"maxRequests":    500,
		}
	}
	return nil
}

func extractWorkbenchJSONPayload(reply string) string {
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return ""
	}
	if strings.HasPrefix(reply, "```") {
		reply = strings.TrimPrefix(reply, "```json")
		reply = strings.TrimPrefix(reply, "```")
		reply = strings.TrimSuffix(reply, "```")
		reply = strings.TrimSpace(reply)
	}
	start := strings.Index(reply, "{")
	end := strings.LastIndex(reply, "}")
	if start < 0 || end < start {
		return ""
	}
	return reply[start : end+1]
}

func normalizeWorkbenchToolName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func sanitizeWorkbenchToolConfigHints(raw map[string]interface{}) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	safe := map[string]interface{}{}
	if value, ok := boundedWorkbenchIntHint(raw["rateLimit"], 1, 10); ok {
		safe["rateLimit"] = value
	}
	if value, ok := boundedWorkbenchIntHint(raw["timeoutSeconds"], 3, 60); ok {
		safe["timeoutSeconds"] = value
	}
	if value, ok := boundedWorkbenchIntHint(raw["maxDepth"], 1, 5); ok {
		safe["maxDepth"] = value
	}
	if value, ok := boundedWorkbenchIntHint(raw["maxRequests"], 1, 5000); ok {
		safe["maxRequests"] = value
	}
	if value, ok := raw["includePassiveOnly"].(bool); ok {
		safe["includePassiveOnly"] = value
	}
	if len(safe) == 0 {
		return nil
	}
	return safe
}

func mergeWorkbenchToolConfigHints(values ...map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	for _, hints := range values {
		for key, value := range hints {
			merged[key] = value
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func boundedWorkbenchIntHint(value interface{}, minValue int, maxValue int) (int, bool) {
	var parsed int
	switch typed := value.(type) {
	case int:
		parsed = typed
	case int64:
		parsed = int(typed)
	case float64:
		if typed != float64(int(typed)) {
			return 0, false
		}
		parsed = int(typed)
	case json.Number:
		number, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		parsed = int(number)
	case string:
		number, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		parsed = number
	default:
		return 0, false
	}
	if parsed < minValue {
		parsed = minValue
	}
	if parsed > maxValue {
		parsed = maxValue
	}
	return parsed, true
}

func formatWorkbenchToolConfigHints(hints map[string]interface{}) string {
	keys := make([]string, 0, len(hints))
	for key := range hints {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, hints[key]))
	}
	return strings.Join(parts, ", ")
}

type workbenchToolEffectStats struct {
	completed int
	failed    int
	running   int
	total     int
}

func collectWorkbenchToolEffectStats(toolName string, historyFeedback []string) workbenchToolEffectStats {
	stats := workbenchToolEffectStats{}
	re := regexp.MustCompile(`工具明细\s+([^\s；;]+)\s+completed=(\d+)\s+failed=(\d+)\s+running=(\d+)\s+total=(\d+)`)
	for _, line := range historyFeedback {
		for _, match := range re.FindAllStringSubmatch(line, -1) {
			if len(match) != 6 || normalizeWorkbenchToolName(match[1]) != toolName {
				continue
			}
			completed, _ := strconv.Atoi(match[2])
			failed, _ := strconv.Atoi(match[3])
			running, _ := strconv.Atoi(match[4])
			total, _ := strconv.Atoi(match[5])
			stats.completed += completed
			stats.failed += failed
			stats.running += running
			stats.total += total
		}
	}
	return stats
}
