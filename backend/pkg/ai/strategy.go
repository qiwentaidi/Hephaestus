package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// WorkbenchStrategyPack 描述 AI 智脑在不同测试场景下采用的策略画像。
type WorkbenchStrategyPack struct {
	ID                    string   `json:"id"`
	Title                 string   `json:"title"`
	Source                string   `json:"source"`
	Enabled               bool     `json:"enabled"`
	Scenario              string   `json:"scenario"`
	Description           string   `json:"description"`
	Triggers              []string `json:"triggers"`
	PreferredCapabilities []string `json:"preferredCapabilities"`
	EvidenceRequirements  []string `json:"evidenceRequirements"`
	SafetyRules           []string `json:"safetyRules"`
}

// WorkbenchStrategySelectionEvidence 是模型选择策略包时可见的任务上下文。
type WorkbenchStrategySelectionEvidence struct {
	Mode                 string
	Target               string
	Scope                []string
	Objective            string
	SuccessCriteria      string
	CurrentStrategyPacks []string
	Signals              []string
	HistoryFeedback      []string
	AvailablePacks       []WorkbenchStrategyPack
}

// WorkbenchStrategySelectionResult 是模型型策略选择器的结构化输出。
type WorkbenchStrategySelectionResult struct {
	StrategyPacks []WorkbenchStrategySelectionItem `json:"strategyPacks"`
	Summary       string                           `json:"summary"`
}

// WorkbenchStrategySelectionItem 表示模型建议加载的策略包。
type WorkbenchStrategySelectionItem struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// WorkbenchStrategyAdjustment 记录策略包因为效果反馈产生的运行时调整。
type WorkbenchStrategyAdjustment struct {
	StrategyID string `json:"strategyId"`
	Action     string `json:"action"`
	Reason     string `json:"reason"`
}

// WorkbenchStrategyEffectScore 是同目标历史反馈沉淀出的策略包效果评分。
type WorkbenchStrategyEffectScore struct {
	StrategyID string `json:"strategyId"`
	Score      int    `json:"score"`
	Positive   int    `json:"positive"`
	Negative   int    `json:"negative"`
	Reason     string `json:"reason"`
}

// WorkbenchStrategyExperimentPlan 描述策略包 A/B 对照候选，不直接改变执行计划。
type WorkbenchStrategyExperimentPlan struct {
	ChampionID      string   `json:"championId"`
	ChallengerID    string   `json:"challengerId"`
	ChampionScore   int      `json:"championScore"`
	ChallengerScore int      `json:"challengerScore"`
	Basis           []string `json:"basis"`
	Reason          string   `json:"reason"`
}

// ListWorkbenchStrategyPacks 返回内置策略包目录。
func ListWorkbenchStrategyPacks() []WorkbenchStrategyPack {
	return []WorkbenchStrategyPack{
		{
			ID:          "external_attack_surface",
			Title:       "攻击面资产评估",
			Source:      "builtin",
			Enabled:     true,
			Scenario:    "域名、IP、CIDR、空间测绘或资产扩展",
			Description: "先识别授权资产边界，再把子域名、开放服务、Web 指纹和历史扫描结果聚合为可验证上下文。",
			Triggers:    []string{"root_domain", "ip", "cidr", "scan", "ai_scan"},
			PreferredCapabilities: []string{
				"asset.subdomain",
				"asset.portscan",
				"space.search",
				"web.fingerprint",
			},
			EvidenceRequirements: []string{
				"授权范围与目标归属说明",
				"新增资产来源和去重结果",
				"开放服务、指纹和历史风险关联",
			},
			SafetyRules: []string{
				"仅在授权范围内扩展资产",
				"服务识别使用保守端口与低侵入参数",
			},
		},
		{
			ID:          "single_web_deep_test",
			Title:       "单站深度测试",
			Source:      "builtin",
			Enabled:     true,
			Scenario:    "HTTP/HTTPS 站点、业务入口、登录态测试",
			Description: "围绕单个 Web 目标先执行全指纹资产发现，再进行 JS/API 采集、目录入口补充和候选漏洞证据整理；漏洞验证按证据进入后续阶段。",
			Triggers:    []string{"http", "https", "deep_test", "web"},
			PreferredCapabilities: []string{
				"web.http.observe",
				"web.fingerprint",
				"web.dirscan",
				"web.jsscan",
				"web.vuln.verify",
			},
			EvidenceRequirements: []string{
				"目标 URL、入口路径和状态码",
				"关键目录、脚本、接口和框架指纹",
				"前置全指纹资产发现结果和后续验证触发依据",
			},
			SafetyRules: []string{
				"默认避免破坏性状态变更",
				"中高风险验证动作需要审批或明确授权策略",
			},
		},
		{
			ID:          "js_api_analysis",
			Title:       "JS/API 线索分析",
			Source:      "builtin",
			Enabled:     true,
			Scenario:    "前端脚本、接口枚举、Token、鉴权与越权线索",
			Description: "优先从 JS、接口路径、参数、鉴权材料和响应差异中生成测试假设，并保留未验证线索。",
			Triggers:    []string{"js", "api", "接口", "token", "鉴权", "认证", "登录"},
			PreferredCapabilities: []string{
				"web.http.observe",
				"web.jsscan",
				"web.repeater",
				"web.vuln.verify",
			},
			EvidenceRequirements: []string{
				"脚本来源和接口路径",
				"参数、鉴权上下文与角色边界",
				"未授权、越权或敏感信息线索",
			},
			SafetyRules: []string{
				"不自动复用或导出敏感凭据",
				"涉及账号、权限或状态变更时必须标注审批需求",
			},
		},
		{
			ID:          "verification_report",
			Title:       "复测与报告交付",
			Source:      "builtin",
			Enabled:     true,
			Scenario:    "复测分析、证据整理、报告生成",
			Description: "把已完成步骤、工具输出、观察和假设收敛成目标完成度评估、风险证据链和剩余缺口。",
			Triggers:    []string{"review", "report", "报告", "复测", "验证", "evidence"},
			PreferredCapabilities: []string{
				"web.vuln.verify",
				"report.markdown",
			},
			EvidenceRequirements: []string{
				"验证状态、证据引用和影响说明",
				"未验证线索和复测建议",
				"面向交付物的结论与边界声明",
			},
			SafetyRules: []string{
				"报告中区分已验证事实和推测性判断",
				"避免输出破坏性利用步骤",
			},
		},
	}
}

// BuildWorkbenchStrategySelectionPrompt 构造只允许模型选择内置策略包的提示。
func BuildWorkbenchStrategySelectionPrompt(evidence WorkbenchStrategySelectionEvidence) string {
	packLines := make([]string, 0, len(evidence.AvailablePacks))
	for _, pack := range evidence.AvailablePacks {
		if !workbenchStrategyPackEnabled(pack) {
			continue
		}
		packLines = append(packLines, fmt.Sprintf("- %s: %s；场景: %s；触发: %s；优先能力: %s",
			pack.ID,
			pack.Description,
			pack.Scenario,
			strings.Join(pack.Triggers, ", "),
			strings.Join(pack.PreferredCapabilities, ", "),
		))
	}
	sections := []string{
		"请作为 Hephaestus AI 智脑的策略包选择器，根据任务目标和新证据选择应加载的内置策略包。",
		"只能从可用策略包清单中选择 id，不要编造策略包，不要生成工具调用、命令或漏洞利用步骤。",
		"只输出 JSON，不要输出 Markdown、代码块或额外解释。JSON schema: {\"strategyPacks\":[{\"id\":\"...\",\"reason\":\"...\"}],\"summary\":\"...\"}",
		fmt.Sprintf("模式: %s", evidence.Mode),
		fmt.Sprintf("目标: %s", evidence.Target),
		fmt.Sprintf("范围: %s", strings.Join(evidence.Scope, ", ")),
		fmt.Sprintf("目标说明: %s", evidence.Objective),
		fmt.Sprintf("成功标准: %s", evidence.SuccessCriteria),
		fmt.Sprintf("当前策略包: %s", strings.Join(evidence.CurrentStrategyPacks, ", ")),
		"新证据信号:\n" + joinOrDash(evidence.Signals),
		"历史效果反馈:\n" + joinOrDash(evidence.HistoryFeedback),
		"可用策略包:\n" + joinOrDash(packLines),
		"字段要求：strategyPacks 最多 4 个，按推荐优先级排列；reason 只解释为什么当前目标或证据需要该策略；如果历史反馈显示某策略在同目标上低分、失败或产出不足，可以用当前证据更匹配的策略替换它。",
	}
	return strings.Join(sections, "\n")
}

// ParseWorkbenchStrategySelectionResult 解析模型输出并按内置策略包白名单过滤。
func ParseWorkbenchStrategySelectionResult(reply string) (WorkbenchStrategySelectionResult, bool) {
	return ParseWorkbenchStrategySelectionResultWithCatalog(reply, ListWorkbenchStrategyPacks())
}

// ParseWorkbenchStrategySelectionResultWithCatalog 解析模型输出并按给定策略目录白名单过滤。
func ParseWorkbenchStrategySelectionResultWithCatalog(reply string, catalog []WorkbenchStrategyPack) (WorkbenchStrategySelectionResult, bool) {
	payload := extractWorkbenchJSONPayload(reply)
	if payload == "" {
		return WorkbenchStrategySelectionResult{}, false
	}
	var parsed WorkbenchStrategySelectionResult
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return WorkbenchStrategySelectionResult{}, false
	}
	allowed := workbenchStrategyPackIDSet(catalog)
	cleaned := make([]WorkbenchStrategySelectionItem, 0, len(parsed.StrategyPacks))
	seen := make(map[string]bool, len(parsed.StrategyPacks))
	for _, item := range parsed.StrategyPacks {
		item.ID = normalizeWorkbenchStrategyID(item.ID)
		item.Reason = strings.TrimSpace(item.Reason)
		if item.ID == "" || !allowed[item.ID] || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		cleaned = append(cleaned, item)
		if len(cleaned) >= 4 {
			break
		}
	}
	if len(cleaned) == 0 {
		return WorkbenchStrategySelectionResult{}, false
	}
	parsed.StrategyPacks = cleaned
	parsed.Summary = strings.TrimSpace(parsed.Summary)
	return parsed, true
}

// ApplyWorkbenchStrategySelection 将模型策略选择和规则策略合并，保留规则回退。
func ApplyWorkbenchStrategySelection(ruleStrategyIDs []string, selection WorkbenchStrategySelectionResult, limit int) ([]string, bool) {
	return ApplyWorkbenchStrategySelectionWithCatalog(ruleStrategyIDs, selection, ListWorkbenchStrategyPacks(), limit)
}

// ApplyWorkbenchStrategySelectionWithCatalog 将模型策略选择和规则策略按给定目录合并。
func ApplyWorkbenchStrategySelectionWithCatalog(ruleStrategyIDs []string, selection WorkbenchStrategySelectionResult, catalog []WorkbenchStrategyPack, limit int) ([]string, bool) {
	if limit <= 0 {
		limit = 4
	}
	allowed := workbenchStrategyPackIDSet(catalog)
	selected := make([]string, 0, limit)
	seen := make(map[string]bool, len(ruleStrategyIDs)+len(selection.StrategyPacks))
	add := func(id string) {
		id = normalizeWorkbenchStrategyID(id)
		if id == "" || !allowed[id] || seen[id] || len(selected) >= limit {
			return
		}
		seen[id] = true
		selected = append(selected, id)
	}
	for _, item := range selection.StrategyPacks {
		add(item.ID)
	}
	if len(selected) == 0 {
		return ruleStrategyIDs, false
	}
	for _, id := range ruleStrategyIDs {
		add(id)
	}
	return selected, true
}

// SelectWorkbenchStrategyPacks 根据任务输入选择内置策略包。
func SelectWorkbenchStrategyPacks(mode string, target string, scope []string, objective string, successCriteria string) []string {
	return SelectWorkbenchStrategyPacksFromCatalog(ListWorkbenchStrategyPacks(), mode, target, scope, objective, successCriteria)
}

// SelectWorkbenchStrategyPacksFromCatalog 根据任务输入和策略目录选择策略包。
func SelectWorkbenchStrategyPacksFromCatalog(catalog []WorkbenchStrategyPack, mode string, target string, scope []string, objective string, successCriteria string) []string {
	targets := normalizeWorkbenchScanTargets(target, scope)
	text := strings.ToLower(strings.Join([]string{mode, target, objective, successCriteria, strings.Join(scope, " ")}, " "))
	selected := make([]string, 0, 4)
	allowed := workbenchStrategyPackIDSet(catalog)
	add := func(id string) {
		id = normalizeWorkbenchStrategyID(id)
		if id == "" || !allowed[id] {
			return
		}
		for _, existing := range selected {
			if existing == id {
				return
			}
		}
		selected = append(selected, id)
	}

	for _, item := range targets {
		host := hostFromWorkbenchTarget(item)
		if isWorkbenchCIDR(item) || isLikelyRootDomain(host) {
			add("external_attack_surface")
		}
		if isWorkbenchWebTarget(item) {
			add("single_web_deep_test")
		}
	}
	if strings.Contains(text, "scan") || strings.Contains(text, "ai_scan") || strings.Contains(text, "暴露面") || strings.Contains(text, "资产") {
		add("external_attack_surface")
	}
	if strings.Contains(text, "deep_test") || strings.Contains(text, "web") || strings.Contains(text, "站点") || strings.Contains(text, "业务") {
		add("single_web_deep_test")
	}
	for _, keyword := range []string{"js", "api", "接口", "token", "鉴权", "认证", "登录", "越权"} {
		if strings.Contains(text, keyword) {
			add("js_api_analysis")
			break
		}
	}
	for _, keyword := range []string{"review", "report", "报告", "复测", "验证", "evidence", "证据"} {
		if strings.Contains(text, keyword) {
			add("verification_report")
			break
		}
	}
	for _, pack := range catalog {
		if !workbenchStrategyPackEnabled(pack) {
			continue
		}
		for _, trigger := range pack.Triggers {
			trigger = strings.ToLower(strings.TrimSpace(trigger))
			if trigger != "" && strings.Contains(text, trigger) {
				add(pack.ID)
				break
			}
		}
	}
	if len(selected) == 0 {
		add("single_web_deep_test")
	}
	return selected
}

// RefineWorkbenchStrategyPacks 根据新证据增补策略包，不自动删除既有策略。
func RefineWorkbenchStrategyPacks(strategyIDs []string, signals ...string) []string {
	return RefineWorkbenchStrategyPacksFromCatalog(ListWorkbenchStrategyPacks(), strategyIDs, signals...)
}

// RefineWorkbenchStrategyPacksFromCatalog 根据新证据和给定策略目录增补策略包，不自动删除既有策略。
func RefineWorkbenchStrategyPacksFromCatalog(catalog []WorkbenchStrategyPack, strategyIDs []string, signals ...string) []string {
	selected := append([]string{}, strategyIDs...)
	allowed := workbenchStrategyPackIDSet(catalog)
	add := func(id string) {
		id = normalizeWorkbenchStrategyID(id)
		if id == "" || !allowed[id] {
			return
		}
		for _, existing := range selected {
			if existing == id {
				return
			}
		}
		selected = append(selected, id)
	}
	text := strings.ToLower(strings.Join(signals, "\n"))
	if containsAny(text, "api", "/api/", "graphql", "js", "script", "接口", "token", "鉴权", "认证", "登录", "auth", "oauth") {
		add("js_api_analysis")
	}
	if containsAny(text, "http", "https", "form", "表单", "页面", "站点", "web") {
		add("single_web_deep_test")
	}
	if containsAny(text, "vulnerability", "漏洞", "cve", "poc", "verify", "验证", "高危") {
		add("verification_report")
	}
	if containsAny(text, "subdomain", "子域", "port", "端口", "service", "开放服务", "fingerprint", "指纹", "暴露面") {
		add("external_attack_surface")
	}
	for _, pack := range catalog {
		if !workbenchStrategyPackEnabled(pack) {
			continue
		}
		for _, trigger := range pack.Triggers {
			trigger = strings.ToLower(strings.TrimSpace(trigger))
			if trigger != "" && strings.Contains(text, trigger) {
				add(pack.ID)
				break
			}
		}
	}
	return selected
}

// AdjustWorkbenchStrategyPacksWithFeedback 根据同目标历史效果反馈保守地降级或替换策略包。
func AdjustWorkbenchStrategyPacksWithFeedback(catalog []WorkbenchStrategyPack, selected []string, fallbackCandidates []string, historyFeedback []string, signals []string, limit int) ([]string, []WorkbenchStrategyAdjustment) {
	if limit <= 0 {
		limit = 4
	}
	allowed := workbenchStrategyPackIDSet(catalog)
	packByID := make(map[string]WorkbenchStrategyPack, len(catalog))
	for _, pack := range catalog {
		id := normalizeWorkbenchStrategyID(pack.ID)
		if allowed[id] {
			packByID[id] = pack
		}
	}
	effects := summarizeWorkbenchStrategyEffects(catalog, historyFeedback)
	current := normalizeWorkbenchStrategyList(selected, allowed, limit)
	if len(current) == 0 {
		current = normalizeWorkbenchStrategyList(fallbackCandidates, allowed, limit)
	}
	if len(current) == 0 {
		return selected, nil
	}

	next := make([]string, 0, limit)
	seen := make(map[string]bool, len(current))
	degraded := make(map[string]bool, len(current))
	adjustments := make([]WorkbenchStrategyAdjustment, 0)
	for _, id := range current {
		effect := effects[id]
		if effect.negative > effect.positive && len(current)-len(degraded) > 1 && !workbenchStrategySignalsSupportPack(packByID[id], signals) {
			degraded[id] = true
			adjustments = append(adjustments, WorkbenchStrategyAdjustment{
				StrategyID: id,
				Action:     "downgrade",
				Reason:     effect.reason(),
			})
			continue
		}
		seen[id] = true
		next = append(next, id)
	}

	if len(degraded) > 0 {
		replacements := append([]string{}, fallbackCandidates...)
		for id, effect := range effects {
			if effect.positive > effect.negative {
				replacements = append(replacements, id)
			}
		}
		replacements = append(replacements, RefineWorkbenchStrategyPacksFromCatalog(catalog, nil, signals...)...)
		for _, id := range replacements {
			id = normalizeWorkbenchStrategyID(id)
			if id == "" || !allowed[id] || seen[id] || degraded[id] || len(next) >= limit {
				continue
			}
			seen[id] = true
			next = append(next, id)
			adjustments = append(adjustments, WorkbenchStrategyAdjustment{
				StrategyID: id,
				Action:     "replace",
				Reason:     "根据当前证据信号或同目标正反馈补入替代策略。",
			})
		}
	}

	if len(next) == 0 {
		next = current[:1]
	}
	if stringSlicesEqualNormalized(current, next) {
		return current, nil
	}
	return next, adjustments
}

// ScoreWorkbenchStrategyEffects 将同目标历史效果反馈转成策略包级评分，供审计和后续选择使用。
func ScoreWorkbenchStrategyEffects(catalog []WorkbenchStrategyPack, historyFeedback []string) []WorkbenchStrategyEffectScore {
	effects := summarizeWorkbenchStrategyEffects(catalog, historyFeedback)
	scores := make([]WorkbenchStrategyEffectScore, 0, len(effects))
	for id, effect := range effects {
		score := 50 + effect.positive*20 - effect.negative*25
		if score < 0 {
			score = 0
		}
		if score > 100 {
			score = 100
		}
		scores = append(scores, WorkbenchStrategyEffectScore{
			StrategyID: id,
			Score:      score,
			Positive:   effect.positive,
			Negative:   effect.negative,
			Reason:     effect.reason(),
		})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score == scores[j].Score {
			return scores[i].StrategyID < scores[j].StrategyID
		}
		return scores[i].Score < scores[j].Score
	})
	return scores
}

// BuildWorkbenchStrategyExperiments 根据当前策略、候选策略和历史评分生成 A/B 对照计划。
func BuildWorkbenchStrategyExperiments(catalog []WorkbenchStrategyPack, current []string, candidates []string, scores []WorkbenchStrategyEffectScore, signals []string, limit int) []WorkbenchStrategyExperimentPlan {
	if limit <= 0 {
		limit = 2
	}
	allowed := workbenchStrategyPackIDSet(catalog)
	packByID := make(map[string]WorkbenchStrategyPack, len(catalog))
	for _, pack := range catalog {
		id := normalizeWorkbenchStrategyID(pack.ID)
		if allowed[id] {
			packByID[id] = pack
		}
	}
	current = normalizeWorkbenchStrategyList(current, allowed, 8)
	if len(current) == 0 {
		return nil
	}
	scoreByID := make(map[string]WorkbenchStrategyEffectScore, len(scores))
	for _, score := range scores {
		id := normalizeWorkbenchStrategyID(score.StrategyID)
		if allowed[id] {
			scoreByID[id] = score
		}
	}
	challengers := normalizeWorkbenchStrategyList(candidates, allowed, 8)
	for _, score := range scores {
		if score.Score >= 50 {
			challengers = append(challengers, score.StrategyID)
		}
	}
	challengers = append(challengers, RefineWorkbenchStrategyPacksFromCatalog(catalog, nil, signals...)...)
	challengers = normalizeWorkbenchStrategyList(challengers, allowed, 8)
	if len(challengers) == 0 {
		return nil
	}

	currentSet := make(map[string]bool, len(current))
	for _, id := range current {
		currentSet[id] = true
	}
	experiments := make([]WorkbenchStrategyExperimentPlan, 0, limit)
	usedChallengers := make(map[string]bool, len(challengers))
	for _, championID := range current {
		championScore := workbenchStrategyScore(scoreByID, championID)
		for _, challengerID := range challengers {
			if challengerID == championID || currentSet[challengerID] || usedChallengers[challengerID] {
				continue
			}
			challengerScore := workbenchStrategyScore(scoreByID, challengerID)
			supported := workbenchStrategySignalsSupportPack(packByID[challengerID], signals)
			if challengerScore < championScore && !supported {
				continue
			}
			basis := []string{
				fmt.Sprintf("championScore=%d", championScore),
				fmt.Sprintf("challengerScore=%d", challengerScore),
			}
			if supported {
				basis = append(basis, "currentSignalsSupportChallenger=true")
			}
			if score, ok := scoreByID[challengerID]; ok && score.Reason != "" {
				basis = append(basis, "challengerHistory="+score.Reason)
			}
			experiments = append(experiments, WorkbenchStrategyExperimentPlan{
				ChampionID:      championID,
				ChallengerID:    challengerID,
				ChampionScore:   championScore,
				ChallengerScore: challengerScore,
				Basis:           basis,
				Reason:          fmt.Sprintf("保留 %s 作为当前策略，同时观察 %s 是否在同类信号下带来更高收益。", championID, challengerID),
			})
			usedChallengers[challengerID] = true
			break
		}
		if len(experiments) >= limit {
			break
		}
	}
	return experiments
}

// StrategyPackPrompt 将策略包转成可注入模型上下文的提示片段。
func StrategyPackPrompt(strategyIDs []string) string {
	return StrategyPackPromptFromCatalog(strategyIDs, ListWorkbenchStrategyPacks())
}

// StrategyPackPromptFromCatalog 将给定策略目录转成可注入模型上下文的提示片段。
func StrategyPackPromptFromCatalog(strategyIDs []string, packs []WorkbenchStrategyPack) string {
	if len(strategyIDs) == 0 {
		return ""
	}
	byID := make(map[string]WorkbenchStrategyPack, len(packs))
	for _, pack := range packs {
		if !workbenchStrategyPackEnabled(pack) {
			continue
		}
		byID[pack.ID] = pack
	}

	lines := []string{"当前任务已加载以下策略包，请在分析、追问、调度建议和报告中遵循："}
	for _, id := range strategyIDs {
		pack, ok := byID[id]
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s（%s）: %s", pack.Title, pack.Scenario, pack.Description))
		if len(pack.PreferredCapabilities) > 0 {
			lines = append(lines, "  优先能力: "+strings.Join(pack.PreferredCapabilities, ", "))
		}
		if len(pack.EvidenceRequirements) > 0 {
			lines = append(lines, "  证据要求: "+strings.Join(pack.EvidenceRequirements, "；"))
		}
		if len(pack.SafetyRules) > 0 {
			lines = append(lines, "  安全规则: "+strings.Join(pack.SafetyRules, "；"))
		}
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// WorkbenchSystemPromptWithStrategies 返回包含策略画像的系统提示。
func WorkbenchSystemPromptWithStrategies(strategyIDs []string) string {
	return WorkbenchSystemPromptWithStrategyCatalog(strategyIDs, ListWorkbenchStrategyPacks())
}

// WorkbenchSystemPromptWithStrategyCatalog 返回包含给定策略目录画像的系统提示。
func WorkbenchSystemPromptWithStrategyCatalog(strategyIDs []string, catalog []WorkbenchStrategyPack) string {
	prompt := WorkbenchSystemPrompt()
	if strategyPrompt := StrategyPackPromptFromCatalog(strategyIDs, catalog); strategyPrompt != "" {
		prompt += "\n\n" + strategyPrompt
	}
	return prompt
}

func workbenchStrategyPackIDSet(catalog []WorkbenchStrategyPack) map[string]bool {
	allowed := make(map[string]bool, len(catalog))
	for _, pack := range catalog {
		if !workbenchStrategyPackEnabled(pack) {
			continue
		}
		id := normalizeWorkbenchStrategyID(pack.ID)
		if id != "" {
			allowed[id] = true
		}
	}
	return allowed
}

func normalizeWorkbenchStrategyID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func workbenchStrategyPackEnabled(pack WorkbenchStrategyPack) bool {
	return pack.Source != "custom" || pack.Enabled
}

type workbenchStrategyEffectSummary struct {
	positive int
	negative int
	reasons  []string
}

func (summary workbenchStrategyEffectSummary) reason() string {
	if len(summary.reasons) == 0 {
		return "同目标历史反馈显示该策略效果较弱。"
	}
	return strings.Join(summary.reasons, "；")
}

func summarizeWorkbenchStrategyEffects(catalog []WorkbenchStrategyPack, historyFeedback []string) map[string]workbenchStrategyEffectSummary {
	effects := map[string]workbenchStrategyEffectSummary{}
	allowed := workbenchStrategyPackIDSet(catalog)
	for _, line := range historyFeedback {
		normalizedLine := strings.ToLower(line)
		status, score, hasEvaluation := parseWorkbenchStrategyHistoryEvaluation(normalizedLine)
		completedCalls, failedCalls, hasToolCalls := parseWorkbenchStrategyHistoryToolCalls(normalizedLine)
		if !hasEvaluation && !hasToolCalls {
			continue
		}
		isNegative := status == "fail" || status == "failed" || (hasEvaluation && score > 0 && score < 60) || (hasToolCalls && failedCalls > 0 && failedCalls >= completedCalls)
		isPositive := (status == "pass" || status == "passed") && (!hasEvaluation || score >= 80) && (!hasToolCalls || failedCalls == 0)
		if !isNegative && !isPositive {
			continue
		}
		for id := range allowed {
			if !strings.Contains(normalizedLine, id) {
				continue
			}
			effect := effects[id]
			if isNegative {
				effect.negative++
				effect.reasons = append(effect.reasons, fmt.Sprintf("历史反馈 %s/%d，工具失败 %d/%d", emptyStringAs(status, "unknown"), score, failedCalls, completedCalls+failedCalls))
			}
			if isPositive {
				effect.positive++
			}
			effects[id] = effect
		}
	}
	return effects
}

func parseWorkbenchStrategyHistoryEvaluation(line string) (string, int, bool) {
	matches := regexp.MustCompile(`评估\s+([a-z_]+)\s*/\s*(\d+)`).FindStringSubmatch(line)
	if len(matches) != 3 {
		return "", 0, false
	}
	score, err := strconv.Atoi(matches[2])
	if err != nil {
		return matches[1], 0, true
	}
	return matches[1], score, true
}

func parseWorkbenchStrategyHistoryToolCalls(line string) (int, int, bool) {
	matches := regexp.MustCompile(`completed=(\d+)\s+failed=(\d+)\s+total=(\d+)`).FindStringSubmatch(line)
	if len(matches) != 4 {
		return 0, 0, false
	}
	completed, errCompleted := strconv.Atoi(matches[1])
	failed, errFailed := strconv.Atoi(matches[2])
	if errCompleted != nil || errFailed != nil {
		return 0, 0, false
	}
	return completed, failed, true
}

func normalizeWorkbenchStrategyList(values []string, allowed map[string]bool, limit int) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		id := normalizeWorkbenchStrategyID(value)
		if id == "" || !allowed[id] || seen[id] {
			continue
		}
		seen[id] = true
		normalized = append(normalized, id)
		if len(normalized) >= limit {
			break
		}
	}
	return normalized
}

func workbenchStrategyScore(scores map[string]WorkbenchStrategyEffectScore, id string) int {
	if score, ok := scores[normalizeWorkbenchStrategyID(id)]; ok {
		return score.Score
	}
	return 50
}

func workbenchStrategySignalsSupportPack(pack WorkbenchStrategyPack, signals []string) bool {
	text := strings.ToLower(strings.Join(signals, "\n"))
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, trigger := range pack.Triggers {
		trigger = strings.ToLower(strings.TrimSpace(trigger))
		if trigger != "" && strings.Contains(text, trigger) {
			return true
		}
	}
	for _, capability := range pack.PreferredCapabilities {
		capability = strings.ToLower(strings.TrimSpace(capability))
		if capability != "" && strings.Contains(text, capability) {
			return true
		}
	}
	return false
}

func stringSlicesEqualNormalized(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if normalizeWorkbenchStrategyID(left[i]) != normalizeWorkbenchStrategyID(right[i]) {
			return false
		}
	}
	return true
}

func emptyStringAs(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
