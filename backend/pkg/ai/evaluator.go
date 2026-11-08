package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

const (
	WorkbenchGoalEvaluatorRule     = "rule_evaluator"
	WorkbenchGoalEvaluatorModel    = "model_evaluator"
	WorkbenchGoalEvaluatorEvidence = "evidence_evaluator"
	WorkbenchGoalEvaluatorRisk     = "risk_posture_evaluator"
	WorkbenchGoalEvaluatorVote     = "vote_evaluator"
)

// WorkbenchGoalEvaluationState 是 Evaluator 判断任务完成度所需的最小状态快照。
type WorkbenchGoalEvaluationState struct {
	TaskID              string
	SuccessCriteria     string
	TotalSteps          int64
	CompletedSteps      int64
	Hypotheses          int64
	Observations        int64
	AssetBuckets        int64
	ToolCalls           int64
	RunningToolCalls    int64
	FailedToolCalls     int64
	HighRiskEvidence    int64
	MediumRiskEvidence  int64
	OpenHighRiskSteps   int64
	APIEndpoints        int64
	APIRelations        int64
	ExtractedVariables  int64
	HTTPRequestEvidence int64
}

// WorkbenchGoalEvaluationResult 是独立 Evaluator 输出，handler 负责把它持久化。
type WorkbenchGoalEvaluationResult struct {
	Status       string
	Score        int
	MissingItems []string
	Feedback     string
	TraceSummary string
	Evaluator    string
}

// WorkbenchGoalEvaluationVoteConfig 控制多评估器投票权重。
type WorkbenchGoalEvaluationVoteConfig struct {
	EvaluatorWeights map[string]int
}

// WorkbenchGoalEvaluatorVoteItem 是评估器分歧审计的单项快照。
type WorkbenchGoalEvaluatorVoteItem struct {
	Evaluator    string   `json:"evaluator"`
	Status       string   `json:"status"`
	Score        int      `json:"score"`
	MissingItems []string `json:"missingItems"`
	Weight       int      `json:"weight"`
}

// WorkbenchGoalModelEvaluationResult 是模型型 Evaluator 的结构化输出。
type WorkbenchGoalModelEvaluationResult struct {
	Status       string   `json:"status"`
	Score        int      `json:"score"`
	MissingItems []string `json:"missingItems"`
	Feedback     string   `json:"feedback"`
	Reason       []string `json:"reason"`
	Suggestions  []string `json:"suggestions"`
}

// WorkbenchGoalModelEvidence 是提供给模型型 Evaluator 的压缩证据。
type WorkbenchGoalModelEvidence struct {
	TaskName        string
	Target          string
	Mode            string
	RiskPolicy      string
	StrategyPacks   []string
	Observations    []string
	Hypotheses      []string
	ToolCalls       []string
	AssetBuckets    []string
	RuleStatus      string
	RuleScore       int
	RuleMissing     []string
	RuleTrace       string
	SuccessCriteria string
}

// WorkbenchGoalRemediationPlanEvidence 是模型生成补救计划时可见的压缩证据。
type WorkbenchGoalRemediationPlanEvidence struct {
	WorkbenchGoalModelEvidence
	AvailableSteps []WorkbenchGoalRemediationStep
}

// WorkbenchGoalRemediationPlanResult 是模型型补救计划生成器的结构化输出。
type WorkbenchGoalRemediationPlanResult struct {
	Steps   []WorkbenchGoalRemediationStep `json:"steps"`
	Summary string                         `json:"summary"`
}

// WorkbenchGoalRemediationStep 表示一个允许注入下一轮计划的候选步骤。
type WorkbenchGoalRemediationStep struct {
	StepType  string `json:"stepType"`
	Title     string `json:"title"`
	Rationale string `json:"rationale"`
	RiskLevel string `json:"riskLevel"`
}

// CollectWorkbenchGoalEvaluationState 从 AI 工作台数据库表收集评估快照。
func CollectWorkbenchGoalEvaluationState(db *gorm.DB, taskID string) (WorkbenchGoalEvaluationState, error) {
	state := WorkbenchGoalEvaluationState{TaskID: strings.TrimSpace(taskID)}
	if state.TaskID == "" {
		return state, fmt.Errorf("taskID 不能为空")
	}

	var task database.AIWorkbenchTask
	if err := db.Select("id", "success_criteria").Where("id = ?", state.TaskID).First(&task).Error; err != nil {
		return state, err
	}
	state.SuccessCriteria = task.SuccessCriteria

	if err := db.Model(&database.AIWorkbenchTaskStep{}).Where("task_id = ?", state.TaskID).Count(&state.TotalSteps).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchTaskStep{}).Where("task_id = ? AND status = ?", state.TaskID, "completed").Count(&state.CompletedSteps).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchHypothesis{}).Where("task_id = ?", state.TaskID).Count(&state.Hypotheses).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchObservation{}).Where("task_id = ?", state.TaskID).Count(&state.Observations).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchAssetBucket{}).Where("task_id = ?", state.TaskID).Count(&state.AssetBuckets).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchToolCall{}).Where("task_id = ?", state.TaskID).Count(&state.ToolCalls).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchToolCall{}).Where("task_id = ? AND status = ?", state.TaskID, "running").Count(&state.RunningToolCalls).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchToolCall{}).Where("task_id = ? AND status IN ?", state.TaskID, []string{"failed", "cancelled"}).Count(&state.FailedToolCalls).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchEvidenceItem{}).Where("task_id = ? AND risk_level IN ?", state.TaskID, []string{"high", "critical"}).Count(&state.HighRiskEvidence).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchEvidenceItem{}).Where("task_id = ? AND risk_level = ?", state.TaskID, "medium").Count(&state.MediumRiskEvidence).Error; err != nil {
		return state, err
	}
	if err := queryOpenHighRiskWorkbenchSteps(db, state.TaskID).Count(&state.OpenHighRiskSteps).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchAPIEndpoint{}).Where("task_id = ?", state.TaskID).Count(&state.APIEndpoints).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchAPIRelation{}).Where("task_id = ?", state.TaskID).Count(&state.APIRelations).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchExtractedVariable{}).Where("task_id = ?", state.TaskID).Count(&state.ExtractedVariables).Error; err != nil {
		return state, err
	}
	if err := db.Model(&database.AIWorkbenchHTTPRequestEvidence{}).Where("task_id = ?", state.TaskID).Count(&state.HTTPRequestEvidence).Error; err != nil {
		return state, err
	}
	return state, nil
}

func queryOpenHighRiskWorkbenchSteps(db *gorm.DB, taskID string) *gorm.DB {
	return db.Model(&database.AIWorkbenchTaskStep{}).
		Where("task_id = ? AND risk_level = ? AND status <> ? AND status <> ?", taskID, "high", "completed", "rejected")
}

// EvaluateWorkbenchGoal 根据用户成功标准和当前证据状态判断目标是否达成。
func EvaluateWorkbenchGoal(state WorkbenchGoalEvaluationState) WorkbenchGoalEvaluationResult {
	score := 20
	if state.TotalSteps > 0 {
		score += int(float64(state.CompletedSteps) / float64(state.TotalSteps) * 45)
	}
	if state.Hypotheses > 0 {
		score += 15
	}
	if state.Observations > 1 {
		score += 10
	}
	if state.AssetBuckets > 0 {
		score += 5
	}
	if state.ToolCalls > 0 {
		score += 5
	}
	if state.APIEndpoints > 0 {
		score += 5
	}
	if state.ExtractedVariables > 0 || state.APIRelations > 0 {
		score += 5
	}
	if score > 100 {
		score = 100
	}

	missing := evaluateWorkbenchGoalMissingItems(state)
	status := "fail"
	if score >= 85 && len(missing) == 0 {
		status = "pass"
	}

	return WorkbenchGoalEvaluationResult{
		Status:       status,
		Score:        score,
		MissingItems: missing,
		Feedback:     buildWorkbenchGoalEvaluationFeedback(status, missing),
		TraceSummary: fmt.Sprintf(
			"已完成步骤 %d/%d，观察 %d 条，假设 %d 条，资产上下文 %d 个，工具调用 %d 次，API 节点 %d 个，API 关系 %d 条，变量 %d 个。",
			state.CompletedSteps,
			state.TotalSteps,
			state.Observations,
			state.Hypotheses,
			state.AssetBuckets,
			state.ToolCalls,
			state.APIEndpoints,
			state.APIRelations,
			state.ExtractedVariables,
		),
		Evaluator: WorkbenchGoalEvaluatorRule,
	}
}

// EvaluateWorkbenchRiskPosture 从风险态势角度复核是否需要人工处理或补证据。
func EvaluateWorkbenchRiskPosture(state WorkbenchGoalEvaluationState) WorkbenchGoalEvaluationResult {
	score := 95
	status := "pass"
	missing := make([]string, 0, 5)
	if state.RunningToolCalls > 0 {
		score -= 25
		status = "fail"
		missing = append(missing, "仍有关联工具调用运行中，风险态势未收敛")
	}
	if state.FailedToolCalls > 0 {
		score -= 20
		status = "fail"
		missing = append(missing, "存在失败或取消的工具调用，需复核覆盖缺口")
	}
	if state.OpenHighRiskSteps > 0 {
		score -= 25
		status = "fail"
		missing = append(missing, "仍有高风险步骤未完成或未决策")
	}
	if state.HighRiskEvidence > 0 && status != "fail" {
		score -= 10
		status = "needs_review"
		missing = append(missing, "存在高风险证据，需要人工复核和报告处置")
	}
	if state.HighRiskEvidence == 0 && state.MediumRiskEvidence > 0 && status == "pass" {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	return WorkbenchGoalEvaluationResult{
		Status:       status,
		Score:        score,
		MissingItems: missing,
		Feedback:     buildWorkbenchGoalEvaluationFeedback(status, missing),
		TraceSummary: fmt.Sprintf(
			"风险态势评估: 高风险证据 %d 条，中风险证据 %d 条，高风险未完成步骤 %d 个，运行中工具 %d 次，失败工具 %d 次。",
			state.HighRiskEvidence,
			state.MediumRiskEvidence,
			state.OpenHighRiskSteps,
			state.RunningToolCalls,
			state.FailedToolCalls,
		),
		Evaluator: WorkbenchGoalEvaluatorRisk,
	}
}

// EvaluateWorkbenchEvidenceCompleteness 从证据完整性角度独立评估任务完成度。
func EvaluateWorkbenchEvidenceCompleteness(state WorkbenchGoalEvaluationState) WorkbenchGoalEvaluationResult {
	score := 100
	missing := make([]string, 0, 6)
	if state.Observations == 0 {
		score -= 30
		missing = append(missing, "缺少可引用观察记录")
	}
	if state.Hypotheses == 0 {
		score -= 20
		missing = append(missing, "缺少可追踪测试假设")
	}
	if state.RunningToolCalls > 0 {
		score -= 25
		missing = append(missing, "仍有关联工具调用运行中")
	}
	if state.FailedToolCalls > 0 {
		score -= 20
		missing = append(missing, "存在失败或取消的工具调用需要复核")
	}
	criteria := strings.ToLower(state.SuccessCriteria)
	if state.AssetBuckets == 0 && containsAny(criteria, "资产", "asset", "清单") {
		score -= 15
		missing = append(missing, "尚未形成资产上下文")
	}
	if state.ToolCalls == 0 && containsAny(criteria, "验证", "扫描", "工具", "test", "scan", "verify") {
		score -= 20
		missing = append(missing, "尚未执行工具验证动作")
	}
	apiMissing := evaluateWorkbenchGoalAPIMissingItems(state)
	if len(apiMissing) > 0 {
		score -= 15 * len(apiMissing)
		if score < 0 {
			score = 0
		}
		missing = append(missing, apiMissing...)
	}
	if score < 0 {
		score = 0
	}
	status := "pass"
	if len(missing) > 0 || score < 85 {
		status = "fail"
	}
	return WorkbenchGoalEvaluationResult{
		Status:       status,
		Score:        score,
		MissingItems: missing,
		Feedback:     buildWorkbenchGoalEvaluationFeedback(status, missing),
		TraceSummary: fmt.Sprintf(
			"证据完整性评估: 观察 %d 条，假设 %d 条，资产上下文 %d 个，工具调用 %d 次，HTTP 证据 %d 条，API 节点 %d 个，API 关系 %d 条，变量 %d 个，运行中 %d 次，失败 %d 次。",
			state.Observations,
			state.Hypotheses,
			state.AssetBuckets,
			state.ToolCalls,
			state.HTTPRequestEvidence,
			state.APIEndpoints,
			state.APIRelations,
			state.ExtractedVariables,
			state.RunningToolCalls,
			state.FailedToolCalls,
		),
		Evaluator: WorkbenchGoalEvaluatorEvidence,
	}
}

// VoteWorkbenchGoalEvaluations 聚合多个独立 Evaluator，避免单一评估器直接决定终态。
func VoteWorkbenchGoalEvaluations(results ...WorkbenchGoalEvaluationResult) WorkbenchGoalEvaluationResult {
	return VoteWorkbenchGoalEvaluationsWithConfig(DefaultWorkbenchGoalEvaluationVoteConfig(), results...)
}

func DefaultWorkbenchGoalEvaluationVoteConfig() WorkbenchGoalEvaluationVoteConfig {
	return WorkbenchGoalEvaluationVoteConfig{
		EvaluatorWeights: map[string]int{
			WorkbenchGoalEvaluatorRule:     1,
			WorkbenchGoalEvaluatorEvidence: 1,
			WorkbenchGoalEvaluatorRisk:     1,
			WorkbenchGoalEvaluatorModel:    1,
		},
	}
}

// VoteWorkbenchGoalEvaluationsWithConfig 聚合多个独立 Evaluator，并允许上层显式配置权重。
func VoteWorkbenchGoalEvaluationsWithConfig(config WorkbenchGoalEvaluationVoteConfig, results ...WorkbenchGoalEvaluationResult) WorkbenchGoalEvaluationResult {
	candidates := make([]WorkbenchGoalEvaluationResult, 0, len(results))
	for _, result := range results {
		result.Status = normalizeWorkbenchGoalVoteStatus(result.Status)
		if result.Status == "" {
			continue
		}
		if workbenchGoalEvaluatorWeight(config, result.Evaluator) <= 0 {
			continue
		}
		if result.Score < 0 {
			result.Score = 0
		}
		if result.Score > 100 {
			result.Score = 100
		}
		candidates = append(candidates, result)
	}
	if len(candidates) == 0 {
		return WorkbenchGoalEvaluationResult{
			Status:       "fail",
			Score:        0,
			MissingItems: []string{"没有可用评估器结果"},
			Feedback:     "待补齐: 没有可用评估器结果",
			Evaluator:    WorkbenchGoalEvaluatorVote,
		}
	}

	passVotes := 0
	failVotes := 0
	reviewVotes := 0
	scoreSum := 0
	scoreWeight := 0
	traceLines := make([]string, 0, len(candidates)+1)
	missing := make([]string, 0)
	for _, result := range candidates {
		weight := workbenchGoalEvaluatorWeight(config, result.Evaluator)
		switch result.Status {
		case "pass":
			passVotes += weight
		case "fail":
			failVotes += weight
		case "needs_review":
			reviewVotes += weight
		}
		scoreSum += result.Score * weight
		scoreWeight += weight
		traceLines = append(traceLines, fmt.Sprintf("%s(weight=%d): %s/%d %s", emptyStringAs(result.Evaluator, "unknown_evaluator"), weight, result.Status, result.Score, singleLineText(result.TraceSummary)))
		if result.Status != "pass" {
			missing = appendUniqueWorkbenchGoalStrings(missing, result.MissingItems...)
		}
	}

	status := "needs_review"
	if failVotes >= 2 || (failVotes == 1 && passVotes == 0) {
		status = "fail"
	} else if passVotes >= 2 && failVotes == 0 && reviewVotes == 0 {
		status = "pass"
	}
	score := 0
	if scoreWeight > 0 {
		score = scoreSum / scoreWeight
	}
	if status == "pass" && score < 85 {
		score = 85
	}
	if status == "fail" && score > 84 {
		score = 84
	}
	if status == "pass" {
		missing = nil
	}
	feedback := buildWorkbenchGoalEvaluationFeedback(status, missing)
	if status == "needs_review" && len(missing) == 0 {
		feedback = "评估器结论不一致，需要人工复核或补充证据。"
	}
	traceLines = append([]string{fmt.Sprintf("多评估器投票: pass=%d fail=%d needs_review=%d。", passVotes, failVotes, reviewVotes)}, traceLines...)
	return WorkbenchGoalEvaluationResult{
		Status:       status,
		Score:        score,
		MissingItems: missing,
		Feedback:     feedback,
		TraceSummary: strings.Join(traceLines, "\n"),
		Evaluator:    WorkbenchGoalEvaluatorVote,
	}
}

// HasWorkbenchGoalEvaluationDisagreement 判断多个评估器是否给出不同状态。
func HasWorkbenchGoalEvaluationDisagreement(config WorkbenchGoalEvaluationVoteConfig, results ...WorkbenchGoalEvaluationResult) bool {
	seen := map[string]bool{}
	for _, result := range results {
		status := normalizeWorkbenchGoalVoteStatus(result.Status)
		if status == "" || workbenchGoalEvaluatorWeight(config, result.Evaluator) <= 0 {
			continue
		}
		seen[status] = true
		if len(seen) > 1 {
			return true
		}
	}
	return false
}

func BuildWorkbenchGoalEvaluatorVoteAudit(config WorkbenchGoalEvaluationVoteConfig, results ...WorkbenchGoalEvaluationResult) []WorkbenchGoalEvaluatorVoteItem {
	items := make([]WorkbenchGoalEvaluatorVoteItem, 0, len(results))
	for _, result := range results {
		status := normalizeWorkbenchGoalVoteStatus(result.Status)
		if status == "" {
			continue
		}
		weight := workbenchGoalEvaluatorWeight(config, result.Evaluator)
		if weight <= 0 {
			continue
		}
		items = append(items, WorkbenchGoalEvaluatorVoteItem{
			Evaluator:    emptyStringAs(result.Evaluator, "unknown_evaluator"),
			Status:       status,
			Score:        result.Score,
			MissingItems: result.MissingItems,
			Weight:       weight,
		})
	}
	return items
}

// BuildWorkbenchModelEvaluationPrompt 构造模型型 Evaluator 的输入，要求模型只做完成度评审和缺口反馈。
func BuildWorkbenchModelEvaluationPrompt(evidence WorkbenchGoalModelEvidence) string {
	sections := []string{
		"请作为 Hephaestus AI 智脑的独立 Goal Evaluator，评审当前任务是否达到成功标准。",
		"只做完成度评估，不要生成攻击利用步骤；请明确区分已验证证据、未验证线索和下一步缺口。",
		"只输出 JSON，不要输出 Markdown、代码块或额外解释。JSON schema: {\"status\":\"pass|fail|needs_review\",\"score\":0-100,\"missingItems\":[\"...\"],\"feedback\":\"...\",\"reason\":[\"...\"],\"suggestions\":[\"...\"]}",
		fmt.Sprintf("任务: %s", evidence.TaskName),
		fmt.Sprintf("目标: %s", evidence.Target),
		fmt.Sprintf("模式: %s", evidence.Mode),
		fmt.Sprintf("风险策略: %s", evidence.RiskPolicy),
		fmt.Sprintf("策略包: %s", strings.Join(evidence.StrategyPacks, ", ")),
		fmt.Sprintf("成功标准: %s", evidence.SuccessCriteria),
		fmt.Sprintf("规则评估: %s / %d", evidence.RuleStatus, evidence.RuleScore),
		fmt.Sprintf("规则缺口: %s", strings.Join(evidence.RuleMissing, "；")),
		fmt.Sprintf("规则轨迹: %s", evidence.RuleTrace),
		"观察摘要:\n" + joinOrDash(evidence.Observations),
		"假设摘要:\n" + joinOrDash(evidence.Hypotheses),
		"工具调用摘要:\n" + joinOrDash(evidence.ToolCalls),
		"资产上下文:\n" + joinOrDash(evidence.AssetBuckets),
		"字段要求：status 只能是 pass、fail 或 needs_review；score 必须是 0 到 100 的整数；missingItems、reason、suggestions 每项不超过 4 条。",
	}
	return strings.Join(sections, "\n")
}

// DefaultWorkbenchGoalRemediationSteps 返回模型可选择的补救步骤白名单。
func DefaultWorkbenchGoalRemediationSteps() []WorkbenchGoalRemediationStep {
	return []WorkbenchGoalRemediationStep{
		{StepType: "asset_context", Title: "补齐资产上下文", RiskLevel: "low"},
		{StepType: "http_observation", Title: "补充只读观察证据", RiskLevel: "low"},
		{StepType: "hypothesis", Title: "补充测试假设", RiskLevel: "low"},
		{StepType: "tool_orchestration", Title: "补充低侵入工具验证", RiskLevel: "medium"},
		{StepType: "goal_evaluation", Title: "复核目标完成度", RiskLevel: "low"},
	}
}

// BuildWorkbenchGoalRemediationPlanPrompt 构造模型型补救计划提示。
func BuildWorkbenchGoalRemediationPlanPrompt(evidence WorkbenchGoalRemediationPlanEvidence) string {
	stepLines := make([]string, 0, len(evidence.AvailableSteps))
	for _, step := range evidence.AvailableSteps {
		stepLines = append(stepLines, fmt.Sprintf("- %s [%s]: %s", step.StepType, step.RiskLevel, step.Title))
	}
	sections := []string{
		"请作为 Hephaestus AI 智脑的补救计划生成器，根据 Goal Evaluation 缺口生成下一轮最小必要计划。",
		"只能从可用步骤类型中选择 stepType，不要编造步骤，不要生成工具命令或漏洞利用步骤；高风险或状态变更方向必须保持为需审批的计划项。",
		"只输出 JSON，不要输出 Markdown、代码块或额外解释。JSON schema: {\"steps\":[{\"stepType\":\"...\",\"title\":\"...\",\"rationale\":\"...\",\"riskLevel\":\"low|medium|high\"}],\"summary\":\"...\"}",
		fmt.Sprintf("任务: %s", evidence.TaskName),
		fmt.Sprintf("目标: %s", evidence.Target),
		fmt.Sprintf("模式: %s", evidence.Mode),
		fmt.Sprintf("风险策略: %s", evidence.RiskPolicy),
		fmt.Sprintf("策略包: %s", strings.Join(evidence.StrategyPacks, ", ")),
		fmt.Sprintf("成功标准: %s", evidence.SuccessCriteria),
		fmt.Sprintf("当前评估: %s / %d", evidence.RuleStatus, evidence.RuleScore),
		fmt.Sprintf("缺口: %s", strings.Join(evidence.RuleMissing, "；")),
		fmt.Sprintf("反馈: %s", evidence.RuleTrace),
		"可用步骤:\n" + joinOrDash(stepLines),
		"观察摘要:\n" + joinOrDash(evidence.Observations),
		"假设摘要:\n" + joinOrDash(evidence.Hypotheses),
		"工具调用摘要:\n" + joinOrDash(evidence.ToolCalls),
		"资产上下文:\n" + joinOrDash(evidence.AssetBuckets),
		"字段要求：steps 最多 4 条，按执行顺序排列；优先补证据缺口，最后可选择 goal_evaluation 复核；title 和 rationale 必须说明证据缺口而不是攻击步骤。",
	}
	return strings.Join(sections, "\n")
}

// ParseWorkbenchGoalRemediationPlanResult 解析模型补救计划，并按步骤白名单过滤。
func ParseWorkbenchGoalRemediationPlanResult(reply string) (WorkbenchGoalRemediationPlanResult, bool) {
	payload := extractWorkbenchModelEvaluationJSON(reply)
	if payload == "" {
		return WorkbenchGoalRemediationPlanResult{}, false
	}
	var parsed WorkbenchGoalRemediationPlanResult
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return WorkbenchGoalRemediationPlanResult{}, false
	}
	allowed := workbenchGoalRemediationStepSet()
	cleaned := make([]WorkbenchGoalRemediationStep, 0, len(parsed.Steps))
	seen := make(map[string]bool, len(parsed.Steps))
	for _, step := range parsed.Steps {
		step.StepType = normalizeWorkbenchGoalRemediationStepType(step.StepType)
		step.Title = strings.TrimSpace(step.Title)
		step.Rationale = strings.TrimSpace(step.Rationale)
		step.RiskLevel = normalizeWorkbenchGoalRemediationRisk(step.RiskLevel)
		defaultStep, ok := allowed[step.StepType]
		if step.StepType == "" || !ok || seen[step.StepType] {
			continue
		}
		if step.Title == "" {
			step.Title = defaultStep.Title
		}
		if step.Rationale == "" {
			step.Rationale = step.Title
		}
		if step.RiskLevel == "" {
			step.RiskLevel = defaultStep.RiskLevel
		}
		seen[step.StepType] = true
		cleaned = append(cleaned, step)
		if len(cleaned) >= 4 {
			break
		}
	}
	if len(cleaned) == 0 {
		return WorkbenchGoalRemediationPlanResult{}, false
	}
	parsed.Steps = cleaned
	parsed.Summary = strings.TrimSpace(parsed.Summary)
	return parsed, true
}

func ParseWorkbenchModelEvaluationResult(reply string, fallback WorkbenchGoalEvaluationResult) (WorkbenchGoalEvaluationResult, bool) {
	payload := extractWorkbenchModelEvaluationJSON(reply)
	if payload == "" {
		return fallback, false
	}

	var parsed WorkbenchGoalModelEvaluationResult
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return fallback, false
	}

	status := normalizeWorkbenchGoalModelStatus(parsed.Status)
	if status == "" {
		return fallback, false
	}
	score := parsed.Score
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	if score == 0 && fallback.Score > 0 {
		score = fallback.Score
	}

	result := fallback
	result.Status = status
	result.Score = score
	result.Evaluator = WorkbenchGoalEvaluatorModel
	result.MissingItems = normalizeWorkbenchGoalStringList(parsed.MissingItems, fallback.MissingItems)
	result.Feedback = strings.TrimSpace(parsed.Feedback)
	if result.Feedback == "" {
		result.Feedback = fallback.Feedback
	}

	summaryParts := make([]string, 0, 3)
	if len(parsed.Reason) > 0 {
		summaryParts = append(summaryParts, "模型依据: "+strings.Join(normalizeWorkbenchGoalStringList(parsed.Reason, nil), "；"))
	}
	if len(parsed.Suggestions) > 0 {
		summaryParts = append(summaryParts, "模型建议: "+strings.Join(normalizeWorkbenchGoalStringList(parsed.Suggestions, nil), "；"))
	}
	if len(summaryParts) > 0 {
		result.TraceSummary = strings.TrimSpace(fallback.TraceSummary + "\n" + strings.Join(summaryParts, "\n"))
	}
	return result, true
}

func extractWorkbenchModelEvaluationJSON(reply string) string {
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

func normalizeWorkbenchGoalModelStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed", "success":
		return "pass"
	case "fail", "failed":
		return "fail"
	case "needs_review", "needs-review", "review", "needs review":
		return "needs_review"
	default:
		return ""
	}
}

func normalizeWorkbenchGoalVoteStatus(status string) string {
	return normalizeWorkbenchGoalModelStatus(status)
}

func workbenchGoalEvaluatorWeight(config WorkbenchGoalEvaluationVoteConfig, evaluator string) int {
	evaluator = strings.TrimSpace(evaluator)
	if evaluator == "" {
		evaluator = "unknown_evaluator"
	}
	if config.EvaluatorWeights != nil {
		if weight, ok := config.EvaluatorWeights[evaluator]; ok {
			return weight
		}
	}
	return 1
}

func normalizeWorkbenchGoalStringList(values []string, fallback []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
		if len(result) >= 4 {
			break
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func appendUniqueWorkbenchGoalStrings(base []string, values ...string) []string {
	seen := make(map[string]bool, len(base)+len(values))
	for _, value := range base {
		seen[value] = true
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		base = append(base, value)
		if len(base) >= 8 {
			break
		}
	}
	return base
}

func singleLineText(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) > 180 {
		return value[:180] + "..."
	}
	return value
}

func evaluateWorkbenchGoalMissingItems(state WorkbenchGoalEvaluationState) []string {
	missing := make([]string, 0, 6)
	if state.CompletedSteps < state.TotalSteps {
		missing = append(missing, "仍有计划步骤未完成")
	}
	if state.Hypotheses == 0 {
		missing = append(missing, "尚未形成测试假设")
	}
	if state.Observations <= 1 {
		missing = append(missing, "证据和观察记录不足")
	}
	if state.RunningToolCalls > 0 {
		missing = append(missing, "仍有关联工具调用运行中")
	}
	if state.FailedToolCalls > 0 {
		missing = append(missing, "存在失败或取消的工具调用需要复核")
	}

	criteria := strings.ToLower(state.SuccessCriteria)
	if state.AssetBuckets == 0 && containsAny(criteria, "资产", "asset", "清单") {
		missing = append(missing, "尚未形成资产上下文")
	}
	if state.ToolCalls == 0 && containsAny(criteria, "验证", "扫描", "工具", "test", "scan", "verify") {
		missing = append(missing, "尚未执行工具验证动作")
	}
	missing = append(missing, evaluateWorkbenchGoalAPIMissingItems(state)...)
	return missing
}

func evaluateWorkbenchGoalAPIMissingItems(state WorkbenchGoalEvaluationState) []string {
	criteria := strings.ToLower(state.SuccessCriteria)
	apiRequired := containsAny(criteria, "接口", "api", "链路", "token", "登录", "变量", "关系", "依赖")
	relationsRequired := apiRequired && containsAny(criteria, "链路", "关系", "依赖", "token", "登录", "变量")
	missing := make([]string, 0, 4)
	if apiRequired {
		if state.APIEndpoints == 0 {
			missing = append(missing, "尚未沉淀 API 节点")
		}
		if state.HTTPRequestEvidence == 0 {
			missing = append(missing, "尚未沉淀 HTTP 请求响应证据")
		}
		if state.ExtractedVariables == 0 && containsAny(criteria, "变量", "token", "登录", "链路") {
			missing = append(missing, "尚未提取接口链路变量")
		}
		if state.APIRelations == 0 && relationsRequired {
			missing = append(missing, "尚未记录 API 关系")
		}
	}
	if state.APIEndpoints > 1 && state.APIRelations == 0 {
		missing = appendUniqueWorkbenchGoalStrings(missing, "已发现多个 API 节点但尚未记录接口关系")
	}
	return missing
}

func buildWorkbenchGoalEvaluationFeedback(status string, missing []string) string {
	if status == "pass" {
		return "任务已满足当前成功标准。"
	}
	if len(missing) == 0 {
		return "任务仍需更多证据或人工复核。"
	}
	return "待补齐: " + strings.Join(missing, "；")
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, "\n")
}

func workbenchGoalRemediationStepSet() map[string]WorkbenchGoalRemediationStep {
	steps := DefaultWorkbenchGoalRemediationSteps()
	allowed := make(map[string]WorkbenchGoalRemediationStep, len(steps))
	for _, step := range steps {
		allowed[step.StepType] = step
	}
	return allowed
}

func normalizeWorkbenchGoalRemediationStepType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeWorkbenchGoalRemediationRisk(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}
