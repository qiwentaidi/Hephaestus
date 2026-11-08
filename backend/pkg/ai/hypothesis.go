package ai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// WorkbenchHypothesisDraft 是 AI 智脑根据证据生成的测试假设草稿。
type WorkbenchHypothesisDraft struct {
	Title        string
	Description  string
	Confidence   string
	EvidenceRefs []string
	Priority     int
}

// WorkbenchHypothesisEvidence 是模型生成发散假设时可见的压缩上下文。
type WorkbenchHypothesisEvidence struct {
	TaskName        string
	Target          string
	Mode            string
	RiskPolicy      string
	StrategyPacks   []string
	Observations    []string
	EvidenceItems   []string
	AssetBuckets    []string
	HistoryFeedback []string
	SuccessCriteria string
}

// WorkbenchHypothesisModelResult 是模型型假设生成器的结构化输出。
type WorkbenchHypothesisModelResult struct {
	Hypotheses []WorkbenchHypothesisModelItem `json:"hypotheses"`
	Summary    string                         `json:"summary"`
}

// WorkbenchHypothesisModelItem 表示一个模型建议的测试假设。
type WorkbenchHypothesisModelItem struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Confidence   string   `json:"confidence"`
	EvidenceRefs []string `json:"evidenceRefs"`
	Priority     int      `json:"priority"`
}

// BuildWorkbenchHypothesisPrompt 构造只允许模型产出候选假设的提示。
func BuildWorkbenchHypothesisPrompt(evidence WorkbenchHypothesisEvidence) string {
	sections := []string{
		"请作为 Hephaestus AI 智脑的测试假设生成器，根据当前证据提出下一步值得验证的候选假设。",
		"只生成假设，不要调用工具，不要生成任意命令，不要生成漏洞利用步骤；必须区分已验证证据和待验证推断。",
		"所有假设必须限制在授权目标、风险策略和成功标准内，高风险或状态变更动作只能描述为需要人工审批的验证方向。",
		workbenchNoCORSDetectionInstruction,
		"只输出 JSON，不要输出 Markdown、代码块或额外解释。JSON schema: {\"hypotheses\":[{\"title\":\"...\",\"description\":\"...\",\"confidence\":\"low|medium|high\",\"priority\":1-100,\"evidenceRefs\":[\"...\"]}],\"summary\":\"...\"}",
		fmt.Sprintf("任务: %s", evidence.TaskName),
		fmt.Sprintf("目标: %s", evidence.Target),
		fmt.Sprintf("模式: %s", evidence.Mode),
		fmt.Sprintf("风险策略: %s", evidence.RiskPolicy),
		fmt.Sprintf("策略包: %s", strings.Join(evidence.StrategyPacks, ", ")),
		fmt.Sprintf("成功标准: %s", evidence.SuccessCriteria),
		"观察摘要:\n" + joinOrDash(evidence.Observations),
		"标准证据:\n" + joinOrDash(evidence.EvidenceItems),
		"资产上下文:\n" + joinOrDash(evidence.AssetBuckets),
		"历史效果反馈:\n" + joinOrDash(evidence.HistoryFeedback),
		"字段要求：hypotheses 最多 6 条；priority 越高越优先；title 简短可读；description 说明判断依据、验证边界和预期证据；evidenceRefs 只能引用上方证据标签或留空。",
	}
	return strings.Join(sections, "\n")
}

// ParseWorkbenchHypothesisDrafts 解析模型输出的候选假设，解析失败时由调用方回退规则结果。
func ParseWorkbenchHypothesisDrafts(reply string) ([]WorkbenchHypothesisDraft, bool) {
	payload := extractWorkbenchJSONPayload(reply)
	if payload == "" {
		return nil, false
	}
	var parsed WorkbenchHypothesisModelResult
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return nil, false
	}
	drafts := make([]WorkbenchHypothesisDraft, 0, len(parsed.Hypotheses))
	seen := make(map[string]bool, len(parsed.Hypotheses))
	for _, item := range parsed.Hypotheses {
		title := strings.TrimSpace(item.Title)
		description := strings.TrimSpace(item.Description)
		key := strings.ToLower(title)
		if title == "" || description == "" || seen[key] {
			continue
		}
		seen[key] = true
		drafts = append(drafts, WorkbenchHypothesisDraft{
			Title:        title,
			Description:  description,
			Confidence:   normalizeWorkbenchHypothesisConfidence(item.Confidence),
			EvidenceRefs: cleanWorkbenchHypothesisEvidenceRefs(item.EvidenceRefs),
			Priority:     normalizeWorkbenchHypothesisPriority(item.Priority, item.Confidence),
		})
		if len(drafts) >= 6 {
			break
		}
	}
	if len(drafts) == 0 {
		return nil, false
	}
	sortWorkbenchHypothesisDrafts(drafts)
	return drafts, true
}

// MergeWorkbenchHypothesisDrafts 合并模型和规则假设，按标题去重并保留优先顺序。
func MergeWorkbenchHypothesisDrafts(primary []WorkbenchHypothesisDraft, fallback []WorkbenchHypothesisDraft, limit int) []WorkbenchHypothesisDraft {
	if limit <= 0 {
		limit = 8
	}
	merged := make([]WorkbenchHypothesisDraft, 0, limit)
	seen := make(map[string]bool, len(primary)+len(fallback))
	add := func(items []WorkbenchHypothesisDraft) {
		for _, item := range items {
			item.Title = strings.TrimSpace(item.Title)
			item.Description = strings.TrimSpace(item.Description)
			item.Confidence = normalizeWorkbenchHypothesisConfidence(item.Confidence)
			item.Priority = normalizeWorkbenchHypothesisPriority(item.Priority, item.Confidence)
			if item.Title == "" || item.Description == "" {
				continue
			}
			key := strings.ToLower(item.Title)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, item)
			if len(merged) >= limit {
				return
			}
		}
	}
	add(primary)
	if len(merged) < limit {
		add(fallback)
	}
	sortWorkbenchHypothesisDrafts(merged)
	return merged
}

// BuildWorkbenchHypothesisDrafts 根据策略包、观察证据和资产上下文生成发散测试假设。
func BuildWorkbenchHypothesisDrafts(target string, strategyPacks []string, observations []string, assetContext WorkbenchTargetContext) []WorkbenchHypothesisDraft {
	text := strings.ToLower(strings.Join(append([]string{target, strings.Join(strategyPacks, " ")}, observations...), "\n"))
	drafts := make([]WorkbenchHypothesisDraft, 0, 6)
	add := func(title string, description string, confidence string) {
		for _, existing := range drafts {
			if existing.Title == title {
				return
			}
		}
		drafts = append(drafts, WorkbenchHypothesisDraft{
			Title:       title,
			Description: description,
			Confidence:  confidence,
			Priority:    normalizeWorkbenchHypothesisPriority(0, confidence),
		})
	}

	if containsAny(text, "external_attack_surface", "子域", "端口", "开放服务", "指纹", "暴露面", "资产") || assetContext.PortCount > 0 || assetContext.FingerprintCount > 0 {
		add(
			"暴露面与指纹关联验证",
			"根据目标资产、开放服务、Web 指纹和历史扫描结果识别可能存在公开漏洞、弱配置或高价值入口的资产。",
			"medium",
		)
	}
	if containsAny(text, "single_web_deep_test", "form", "表单", "login", "登录", "auth", "token", "oauth") {
		add(
			"认证与会话边界验证",
			"目标存在登录、认证、Token 或表单线索，应关注登录态复用、未授权访问、水平/垂直越权和状态变更风险。",
			"medium",
		)
	}
	if containsAny(text, "js_api_analysis", "/api/", "api", "接口", "graphql", "script", "js", "前端脚本") || assetContext.JSAssetCount > 0 {
		add(
			"JS/API 敏感信息与未授权接口",
			"从前端脚本、接口路径、参数、鉴权材料和响应差异中提取值得验证的接口安全假设。",
			"medium",
		)
	}
	if containsAny(text, "/admin", "/actuator", "swagger", "后台", "管理", "敏感路径", "目录") || assetContext.DirResultCount > 0 {
		add(
			"敏感入口与管理面暴露",
			"目录、后台、Swagger、Actuator 或管理入口线索需要判断是否存在访问控制缺陷、信息泄露或误暴露。",
			"medium",
		)
	}
	if containsAny(text, "漏洞", "vulnerability", "cve", "poc", "verify", "验证") || assetContext.VulnerabilityCount > 0 {
		add(
			"候选漏洞最小化复测",
			"对已有漏洞命中、CVE/POC 关联或高风险指纹只做授权范围内的最小化验证，并保留请求响应证据。",
			"high",
		)
	}
	if len(drafts) == 0 {
		add(
			"基础 Web 行为观察",
			"当前证据不足，应先补齐页面结构、响应头、指纹、目录和接口线索，再决定是否进入验证。",
			"low",
		)
	}
	if len(drafts) > 6 {
		return drafts[:6]
	}
	return drafts
}

func normalizeWorkbenchHypothesisConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

func cleanWorkbenchHypothesisEvidenceRefs(values []string) []string {
	cleaned := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		cleaned = append(cleaned, value)
		if len(cleaned) >= 8 {
			break
		}
	}
	return cleaned
}

func normalizeWorkbenchHypothesisPriority(value int, confidence string) int {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	if value > 0 {
		return value
	}
	switch normalizeWorkbenchHypothesisConfidence(confidence) {
	case "high":
		return 80
	case "low":
		return 20
	default:
		return 50
	}
}

func sortWorkbenchHypothesisDrafts(drafts []WorkbenchHypothesisDraft) {
	sort.SliceStable(drafts, func(i, j int) bool {
		return drafts[i].Priority > drafts[j].Priority
	})
}
