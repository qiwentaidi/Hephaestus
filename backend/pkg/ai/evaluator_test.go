package ai

import (
	"path/filepath"
	"strings"
	"testing"

	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEvaluateWorkbenchGoalPassesWhenEvidenceMeetsCriteria(t *testing.T) {
	result := EvaluateWorkbenchGoal(WorkbenchGoalEvaluationState{
		SuccessCriteria: "覆盖目标范围，沉淀资产上下文，记录已验证证据",
		TotalSteps:      4,
		CompletedSteps:  4,
		Hypotheses:      2,
		Observations:    3,
		AssetBuckets:    1,
		ToolCalls:       2,
	})

	if result.Status != "pass" {
		t.Fatalf("Status = %q, want pass; missing=%#v", result.Status, result.MissingItems)
	}
	if result.Score < 85 {
		t.Fatalf("Score = %d, want at least 85", result.Score)
	}
	if result.Evaluator != WorkbenchGoalEvaluatorRule {
		t.Fatalf("Evaluator = %q, want %q", result.Evaluator, WorkbenchGoalEvaluatorRule)
	}
}

func TestEvaluateWorkbenchGoalTracksRunningAndFailedTools(t *testing.T) {
	result := EvaluateWorkbenchGoal(WorkbenchGoalEvaluationState{
		TotalSteps:       3,
		CompletedSteps:   3,
		Hypotheses:       1,
		Observations:     3,
		AssetBuckets:     1,
		ToolCalls:        2,
		RunningToolCalls: 1,
		FailedToolCalls:  1,
	})

	if result.Status != "fail" {
		t.Fatalf("Status = %q, want fail", result.Status)
	}
	if !containsString(result.MissingItems, "仍有关联工具调用运行中") {
		t.Fatalf("expected running tool missing item, got %#v", result.MissingItems)
	}
	if !containsString(result.MissingItems, "存在失败或取消的工具调用需要复核") {
		t.Fatalf("expected failed tool missing item, got %#v", result.MissingItems)
	}
}

func TestEvaluateWorkbenchGoalHonorsCriteriaKeywords(t *testing.T) {
	result := EvaluateWorkbenchGoal(WorkbenchGoalEvaluationState{
		SuccessCriteria: "输出资产清单并完成扫描验证",
		TotalSteps:      1,
		CompletedSteps:  1,
		Hypotheses:      1,
		Observations:    2,
	})

	if !containsString(result.MissingItems, "尚未形成资产上下文") {
		t.Fatalf("expected asset context missing item, got %#v", result.MissingItems)
	}
	if !containsString(result.MissingItems, "尚未执行工具验证动作") {
		t.Fatalf("expected tool verification missing item, got %#v", result.MissingItems)
	}
}

func TestEvaluateWorkbenchGoalHonorsAPIGraphCriteria(t *testing.T) {
	result := EvaluateWorkbenchGoal(WorkbenchGoalEvaluationState{
		SuccessCriteria: "输出接口链路图，包含登录 token、业务对象 id 和接口依赖关系",
		TotalSteps:      3,
		CompletedSteps:  3,
		Hypotheses:      1,
		Observations:    3,
		ToolCalls:       1,
		APIEndpoints:    2,
	})

	if result.Status != "fail" {
		t.Fatalf("expected fail without API graph relation evidence, got %#v", result)
	}
	for _, want := range []string{"尚未沉淀 HTTP 请求响应证据", "尚未提取接口链路变量", "尚未记录 API 关系", "已发现多个 API 节点但尚未记录接口关系"} {
		if !containsString(result.MissingItems, want) {
			t.Fatalf("missing %q in %#v", want, result.MissingItems)
		}
	}
}

func TestEvaluateWorkbenchGoalPassesAPIGraphCriteriaWhenCovered(t *testing.T) {
	result := EvaluateWorkbenchGoal(WorkbenchGoalEvaluationState{
		SuccessCriteria:     "输出接口链路图，包含登录 token、业务对象 id 和接口依赖关系",
		TotalSteps:          3,
		CompletedSteps:      3,
		Hypotheses:          1,
		Observations:        3,
		ToolCalls:           1,
		APIEndpoints:        2,
		APIRelations:        2,
		ExtractedVariables:  2,
		HTTPRequestEvidence: 2,
	})

	if result.Status != "pass" {
		t.Fatalf("expected pass with API graph relation evidence, got %#v", result)
	}
}

func TestEvaluateWorkbenchEvidenceCompleteness(t *testing.T) {
	result := EvaluateWorkbenchEvidenceCompleteness(WorkbenchGoalEvaluationState{
		SuccessCriteria:  "输出资产清单并完成扫描验证",
		Observations:     1,
		Hypotheses:       1,
		AssetBuckets:     1,
		ToolCalls:        1,
		RunningToolCalls: 0,
		FailedToolCalls:  0,
	})

	if result.Status != "pass" || result.Evaluator != WorkbenchGoalEvaluatorEvidence {
		t.Fatalf("unexpected evidence result: %#v", result)
	}
}

func TestEvaluateWorkbenchRiskPostureNeedsReviewOnHighRiskEvidence(t *testing.T) {
	result := EvaluateWorkbenchRiskPosture(WorkbenchGoalEvaluationState{
		HighRiskEvidence: 1,
		ToolCalls:        2,
	})

	if result.Status != "needs_review" || result.Evaluator != WorkbenchGoalEvaluatorRisk {
		t.Fatalf("unexpected risk posture result: %#v", result)
	}
	if !containsString(result.MissingItems, "存在高风险证据，需要人工复核和报告处置") {
		t.Fatalf("expected high risk missing item, got %#v", result.MissingItems)
	}
}

func TestEvaluateWorkbenchRiskPostureFailsOnUnsettledExecution(t *testing.T) {
	result := EvaluateWorkbenchRiskPosture(WorkbenchGoalEvaluationState{
		RunningToolCalls:  1,
		FailedToolCalls:   1,
		OpenHighRiskSteps: 1,
	})

	if result.Status != "fail" || result.Score >= 85 {
		t.Fatalf("expected failed risk posture, got %#v", result)
	}
	for _, want := range []string{"仍有关联工具调用运行中，风险态势未收敛", "存在失败或取消的工具调用，需复核覆盖缺口", "仍有高风险步骤未完成或未决策"} {
		if !containsString(result.MissingItems, want) {
			t.Fatalf("missing %q in %#v", want, result.MissingItems)
		}
	}
}

func TestCollectWorkbenchGoalEvaluationStateIncludesRiskPosture(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.AIWorkbenchTask{},
		&database.AIWorkbenchTaskStep{},
		&database.AIWorkbenchHypothesis{},
		&database.AIWorkbenchObservation{},
		&database.AIWorkbenchAssetBucket{},
		&database.AIWorkbenchToolCall{},
		&database.AIWorkbenchEvidenceItem{},
		&database.AIWorkbenchAPIEndpoint{},
		&database.AIWorkbenchAPIRelation{},
		&database.AIWorkbenchExtractedVariable{},
		&database.AIWorkbenchHTTPRequestEvidence{},
	); err != nil {
		t.Fatalf("migrate tables: %v", err)
	}
	task := database.AIWorkbenchTask{ID: "task-1", Name: "AI", Target: "https://example.test", Mode: "agent", Status: "running", RiskPolicy: WorkbenchRiskManualApproval}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := db.Create(&database.AIWorkbenchTaskStep{ID: "step-1", TaskID: task.ID, StepType: "verification", Title: "验证", Status: "approved", RiskLevel: "high"}).Error; err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := db.Create(&database.AIWorkbenchEvidenceItem{ID: "evidence-1", TaskID: task.ID, Title: "高危证据", Summary: "RCE", RiskLevel: "high", Status: "observed"}).Error; err != nil {
		t.Fatalf("create evidence: %v", err)
	}
	if err := db.Create(&database.AIWorkbenchToolCall{ID: "call-1", TaskID: task.ID, ToolName: "web.vuln.verify", Status: "failed"}).Error; err != nil {
		t.Fatalf("create tool call: %v", err)
	}

	state, err := CollectWorkbenchGoalEvaluationState(db, task.ID)
	if err != nil {
		t.Fatalf("collect state: %v", err)
	}
	if state.HighRiskEvidence != 1 || state.OpenHighRiskSteps != 1 || state.FailedToolCalls != 1 {
		t.Fatalf("expected risk posture counts, got %#v", state)
	}
}

func TestOpenHighRiskWorkbenchStepsQueryAvoidsAmbiguousNotInParameter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open sqlite dry run: %v", err)
	}

	var count int64
	stmt := queryOpenHighRiskWorkbenchSteps(db, "task-1").Count(&count).Statement
	sql := stmt.SQL.String()

	if strings.Contains(sql, "NOT IN") {
		t.Fatalf("query should avoid NOT IN slice parameters, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "status <> ? AND status <> ?") {
		t.Fatalf("query should use explicit status comparisons, got SQL: %s", sql)
	}
	if len(stmt.Vars) != 4 {
		t.Fatalf("expected four bound parameters, got %d: %#v", len(stmt.Vars), stmt.Vars)
	}
}

func TestEvaluateWorkbenchEvidenceCompletenessRequiresAPIGraphForInterfaceCriteria(t *testing.T) {
	result := EvaluateWorkbenchEvidenceCompleteness(WorkbenchGoalEvaluationState{
		SuccessCriteria: "输出接口链路图，包含登录 token、业务对象 id 和接口依赖关系",
		Observations:    2,
		Hypotheses:      1,
		AssetBuckets:    1,
		ToolCalls:       1,
	})

	if result.Status != "fail" {
		t.Fatalf("expected fail without API graph evidence, got %#v", result)
	}
	for _, want := range []string{"尚未沉淀 API 节点", "尚未沉淀 HTTP 请求响应证据", "尚未提取接口链路变量", "尚未记录 API 关系"} {
		if !containsString(result.MissingItems, want) {
			t.Fatalf("missing %q in %#v", want, result.MissingItems)
		}
	}
}

func TestEvaluateWorkbenchEvidenceCompletenessPassesWithAPIGraphEvidence(t *testing.T) {
	result := EvaluateWorkbenchEvidenceCompleteness(WorkbenchGoalEvaluationState{
		SuccessCriteria:     "输出接口链路图，包含登录 token、业务对象 id 和接口依赖关系",
		Observations:        2,
		Hypotheses:          1,
		AssetBuckets:        1,
		ToolCalls:           1,
		APIEndpoints:        2,
		APIRelations:        1,
		ExtractedVariables:  2,
		HTTPRequestEvidence: 2,
	})

	if result.Status != "pass" {
		t.Fatalf("expected pass with API graph evidence, got %#v", result)
	}
}

func TestVoteWorkbenchGoalEvaluationsPassesOnLocalConsensus(t *testing.T) {
	result := VoteWorkbenchGoalEvaluations(
		WorkbenchGoalEvaluationResult{Status: "pass", Score: 90, Evaluator: WorkbenchGoalEvaluatorRule},
		WorkbenchGoalEvaluationResult{Status: "pass", Score: 95, Evaluator: WorkbenchGoalEvaluatorEvidence},
	)

	if result.Status != "pass" || result.Score < 85 || result.Evaluator != WorkbenchGoalEvaluatorVote {
		t.Fatalf("unexpected vote result: %#v", result)
	}
	if len(result.MissingItems) != 0 {
		t.Fatalf("expected no missing items on pass, got %#v", result.MissingItems)
	}
}

func TestVoteWorkbenchGoalEvaluationsNeedsReviewOnRiskPosture(t *testing.T) {
	result := VoteWorkbenchGoalEvaluations(
		WorkbenchGoalEvaluationResult{Status: "pass", Score: 90, Evaluator: WorkbenchGoalEvaluatorRule},
		WorkbenchGoalEvaluationResult{Status: "pass", Score: 95, Evaluator: WorkbenchGoalEvaluatorEvidence},
		WorkbenchGoalEvaluationResult{Status: "needs_review", Score: 82, MissingItems: []string{"存在高风险证据，需要人工复核和报告处置"}, Evaluator: WorkbenchGoalEvaluatorRisk},
	)

	if result.Status != "needs_review" {
		t.Fatalf("expected needs_review from risk posture disagreement, got %#v", result)
	}
	if !strings.Contains(result.TraceSummary, WorkbenchGoalEvaluatorRisk) {
		t.Fatalf("expected risk evaluator in trace, got %s", result.TraceSummary)
	}
}

func TestVoteWorkbenchGoalEvaluationsNeedsReviewOnConflict(t *testing.T) {
	result := VoteWorkbenchGoalEvaluations(
		WorkbenchGoalEvaluationResult{Status: "pass", Score: 92, Evaluator: WorkbenchGoalEvaluatorRule},
		WorkbenchGoalEvaluationResult{Status: "pass", Score: 90, Evaluator: WorkbenchGoalEvaluatorEvidence},
		WorkbenchGoalEvaluationResult{Status: "fail", Score: 40, MissingItems: []string{"模型认为证据不足"}, Evaluator: WorkbenchGoalEvaluatorModel},
	)

	if result.Status != "needs_review" {
		t.Fatalf("expected needs_review on model conflict, got %#v", result)
	}
	if !containsString(result.MissingItems, "模型认为证据不足") {
		t.Fatalf("expected model missing item to be retained, got %#v", result.MissingItems)
	}
}

func TestVoteWorkbenchGoalEvaluationsFailsOnFailureMajority(t *testing.T) {
	result := VoteWorkbenchGoalEvaluations(
		WorkbenchGoalEvaluationResult{Status: "fail", Score: 60, MissingItems: []string{"仍有计划步骤未完成"}, Evaluator: WorkbenchGoalEvaluatorRule},
		WorkbenchGoalEvaluationResult{Status: "fail", Score: 50, MissingItems: []string{"缺少可引用观察记录"}, Evaluator: WorkbenchGoalEvaluatorEvidence},
		WorkbenchGoalEvaluationResult{Status: "pass", Score: 95, Evaluator: WorkbenchGoalEvaluatorModel},
	)

	if result.Status != "fail" || result.Score > 84 {
		t.Fatalf("expected conservative fail result, got %#v", result)
	}
	if !containsString(result.MissingItems, "仍有计划步骤未完成") || !containsString(result.MissingItems, "缺少可引用观察记录") {
		t.Fatalf("expected missing items from failed evaluators, got %#v", result.MissingItems)
	}
}

func TestVoteWorkbenchGoalEvaluationsWithConfigCanDisableModelVote(t *testing.T) {
	result := VoteWorkbenchGoalEvaluationsWithConfig(
		WorkbenchGoalEvaluationVoteConfig{
			EvaluatorWeights: map[string]int{
				WorkbenchGoalEvaluatorRule:     1,
				WorkbenchGoalEvaluatorEvidence: 1,
				WorkbenchGoalEvaluatorModel:    0,
			},
		},
		WorkbenchGoalEvaluationResult{Status: "pass", Score: 90, Evaluator: WorkbenchGoalEvaluatorRule},
		WorkbenchGoalEvaluationResult{Status: "pass", Score: 94, Evaluator: WorkbenchGoalEvaluatorEvidence},
		WorkbenchGoalEvaluationResult{Status: "fail", Score: 20, MissingItems: []string{"模型冲突"}, Evaluator: WorkbenchGoalEvaluatorModel},
	)

	if result.Status != "pass" {
		t.Fatalf("expected disabled model vote to be ignored, got %#v", result)
	}
}

func TestWorkbenchGoalEvaluationDisagreementAudit(t *testing.T) {
	config := DefaultWorkbenchGoalEvaluationVoteConfig()
	votes := []WorkbenchGoalEvaluationResult{
		{Status: "pass", Score: 90, Evaluator: WorkbenchGoalEvaluatorRule},
		{Status: "fail", Score: 50, MissingItems: []string{"缺少证据"}, Evaluator: WorkbenchGoalEvaluatorEvidence},
	}

	if !HasWorkbenchGoalEvaluationDisagreement(config, votes...) {
		t.Fatal("expected evaluator disagreement")
	}
	audit := BuildWorkbenchGoalEvaluatorVoteAudit(config, votes...)
	if len(audit) != 2 || audit[0].Weight != 1 || audit[1].MissingItems[0] != "缺少证据" {
		t.Fatalf("unexpected audit: %#v", audit)
	}
}

func TestBuildWorkbenchModelEvaluationPromptIncludesEvidence(t *testing.T) {
	prompt := BuildWorkbenchModelEvaluationPrompt(WorkbenchGoalModelEvidence{
		TaskName:        "AI task",
		Target:          "https://example.test",
		Mode:            "deep_test",
		RiskPolicy:      "manual_approval",
		StrategyPacks:   []string{"single_web_deep_test"},
		SuccessCriteria: "输出证据清单",
		RuleStatus:      "fail",
		RuleScore:       70,
		RuleMissing:     []string{"仍有计划步骤未完成"},
		RuleTrace:       "已完成步骤 2/3",
		Observations:    []string{"- 指纹 nginx"},
		Hypotheses:      []string{"- 未授权接口"},
		ToolCalls:       []string{"- [running] web.fingerprint"},
		AssetBuckets:    []string{"- [target/30] https://example.test"},
	})

	for _, want := range []string{"独立 Goal Evaluator", "single_web_deep_test", "仍有计划步骤未完成", "指纹 nginx", "只输出 JSON", "pass|fail|needs_review"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestParseWorkbenchModelEvaluationResult(t *testing.T) {
	fallback := WorkbenchGoalEvaluationResult{
		Status:       "fail",
		Score:        60,
		MissingItems: []string{"尚未执行工具验证动作"},
		Feedback:     "待补齐",
		TraceSummary: "规则轨迹",
		Evaluator:    WorkbenchGoalEvaluatorRule,
	}

	result, ok := ParseWorkbenchModelEvaluationResult(`{
		"status": "needs_review",
		"score": 78,
		"missingItems": ["补充漏洞复测证据", "补充漏洞复测证据"],
		"feedback": "需要补充复测证据",
		"reason": ["已有资产上下文"],
		"suggestions": ["执行低侵入验证"]
	}`, fallback)
	if !ok {
		t.Fatal("expected model result to parse")
	}
	if result.Status != "needs_review" || result.Score != 78 || result.Evaluator != WorkbenchGoalEvaluatorModel {
		t.Fatalf("unexpected parsed result: %#v", result)
	}
	if len(result.MissingItems) != 1 || result.MissingItems[0] != "补充漏洞复测证据" {
		t.Fatalf("unexpected missing items: %#v", result.MissingItems)
	}
	if !strings.Contains(result.TraceSummary, "模型依据") || !strings.Contains(result.TraceSummary, "模型建议") {
		t.Fatalf("expected model trace details, got %q", result.TraceSummary)
	}
}

func TestParseWorkbenchModelEvaluationResultFromCodeFence(t *testing.T) {
	fallback := WorkbenchGoalEvaluationResult{Status: "fail", Score: 50, Feedback: "fallback"}
	result, ok := ParseWorkbenchModelEvaluationResult("```json\n{\"status\":\"pass\",\"score\":100,\"missingItems\":[],\"feedback\":\"已满足\"}\n```", fallback)
	if !ok {
		t.Fatal("expected fenced model result to parse")
	}
	if result.Status != "pass" || result.Score != 100 || result.Feedback != "已满足" {
		t.Fatalf("unexpected fenced result: %#v", result)
	}
}

func TestParseWorkbenchModelEvaluationResultFallsBackOnInvalidJSON(t *testing.T) {
	fallback := WorkbenchGoalEvaluationResult{Status: "fail", Score: 50, Feedback: "fallback"}
	result, ok := ParseWorkbenchModelEvaluationResult("结论: fail", fallback)
	if ok {
		t.Fatal("expected invalid model output to fail parsing")
	}
	if result.Status != fallback.Status || result.Score != fallback.Score || result.Feedback != fallback.Feedback {
		t.Fatalf("expected fallback result, got %#v", result)
	}
}

func TestBuildWorkbenchGoalRemediationPlanPromptIncludesAllowedSteps(t *testing.T) {
	prompt := BuildWorkbenchGoalRemediationPlanPrompt(WorkbenchGoalRemediationPlanEvidence{
		WorkbenchGoalModelEvidence: WorkbenchGoalModelEvidence{
			TaskName:        "AI task",
			Target:          "https://example.test",
			Mode:            "agent",
			RiskPolicy:      WorkbenchRiskManualApproval,
			StrategyPacks:   []string{"single_web_deep_test"},
			SuccessCriteria: "输出证据链",
			RuleStatus:      "fail",
			RuleScore:       70,
			RuleMissing:     []string{"尚未执行工具验证动作"},
			RuleTrace:       "需要补证据",
			Observations:    []string{"- 发现登录入口"},
		},
		AvailableSteps: DefaultWorkbenchGoalRemediationSteps(),
	})

	for _, want := range []string{"补救计划生成器", "只能从可用步骤类型中选择", "tool_orchestration", "goal_evaluation", "尚未执行工具验证动作"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestParseWorkbenchGoalRemediationPlanResultFiltersAndNormalizesSteps(t *testing.T) {
	result, ok := ParseWorkbenchGoalRemediationPlanResult(`{
		"steps": [
			{"stepType": "http_observation", "title": "补充观察", "rationale": "缺少只读证据", "riskLevel": "low"},
			{"stepType": "unknown", "title": "未知", "rationale": "不应接受", "riskLevel": "low"},
			{"stepType": "http_observation", "title": "重复", "rationale": "重复", "riskLevel": "medium"},
			{"stepType": "tool_orchestration", "title": "", "rationale": "", "riskLevel": "invalid"}
		],
		"summary": "先补观察，再补工具验证"
	}`)
	if !ok {
		t.Fatal("expected remediation plan to parse")
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected two accepted steps, got %#v", result.Steps)
	}
	if result.Steps[0].StepType != "http_observation" || result.Steps[0].RiskLevel != "low" {
		t.Fatalf("unexpected first step: %#v", result.Steps[0])
	}
	if result.Steps[1].StepType != "tool_orchestration" || result.Steps[1].RiskLevel != "medium" || result.Steps[1].Title == "" {
		t.Fatalf("expected default title/risk for tool step, got %#v", result.Steps[1])
	}
}

func TestParseWorkbenchGoalRemediationPlanResultRejectsInvalidOutput(t *testing.T) {
	if _, ok := ParseWorkbenchGoalRemediationPlanResult("需要继续"); ok {
		t.Fatal("expected invalid remediation output to be rejected")
	}
}
