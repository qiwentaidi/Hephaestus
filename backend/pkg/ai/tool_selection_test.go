package ai

import (
	"strings"
	"testing"
)

func TestBuildWorkbenchToolSelectionPromptUsesWhitelistSchema(t *testing.T) {
	prompt := BuildWorkbenchToolSelectionPrompt(WorkbenchToolSelectionEvidence{
		TaskName:        "AI task",
		Target:          "https://example.test",
		RiskPolicy:      WorkbenchRiskManualApproval,
		StrategyPacks:   []string{"single_web_deep_test"},
		SuccessCriteria: "输出证据",
		AvailableTools: []WorkbenchToolDescriptor{
			{Name: "web.fingerprint", Category: "scan", RiskLevel: "low", Description: "指纹识别"},
			{Name: "web.dirscan", Category: "scan", RiskLevel: "medium", Description: "目录探测"},
		},
		Observations:  []string{"- 发现登录入口"},
		EvidenceItems: []string{"- HTTP 只读观察"},
		HistoryFeedback: []string{
			"- 历史任务 task-1 评估 pass/92；工具调用 completed=3 failed=0 total=3",
		},
	})

	for _, want := range []string{"只能从可用工具清单中选择", "CORS", "不要提出、规划、选择或执行 CORS 漏洞检测", "web.fingerprint", "web.dirscan", "只输出 JSON", "tools", "configHints", "不能包含目标", "历史任务 task-1"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestParseWorkbenchToolSelectionResult(t *testing.T) {
	selection, ok := ParseWorkbenchToolSelectionResult(`{
		"tools": [
			{"toolName": "web.jsscan", "reason": "观察到多个 JS 入口", "configHints": {"timeoutSeconds": 12}},
			{"toolName": "web.jsscan", "reason": "重复"},
			{"toolName": "web.dirscan", "reason": "发现登录入口"}
		],
		"summary": "优先 JS/API 再目录验证"
	}`)
	if !ok {
		t.Fatal("expected selection to parse")
	}
	if len(selection.Tools) != 2 {
		t.Fatalf("expected duplicate tools to be collapsed, got %#v", selection.Tools)
	}
	if selection.Tools[0].ToolName != "web.jsscan" || selection.Tools[1].ToolName != "web.dirscan" {
		t.Fatalf("unexpected tool order: %#v", selection.Tools)
	}
	if selection.Tools[0].ConfigHints["timeoutSeconds"].(float64) != 12 {
		t.Fatalf("expected config hints to be parsed, got %#v", selection.Tools[0].ConfigHints)
	}
}

func TestApplyWorkbenchToolSelectionPreservesBaselineAndAddsSelectedOptionalPlans(t *testing.T) {
	plans := []WorkbenchScanExecutionPlan{
		{ToolName: "web.fingerprint", Reason: "默认指纹"},
		{ToolName: "web.dirscan", Optional: true, Reason: "默认目录"},
		{ToolName: "web.jsscan", Reason: "默认 JS"},
	}
	selection := WorkbenchToolSelectionResult{
		Tools: []WorkbenchToolSelectionItem{
			{ToolName: "unknown.tool", Reason: "未知工具应忽略"},
			{ToolName: "web.jsscan", Reason: "JS/API 线索更多"},
			{ToolName: "web.dirscan", Reason: "补充入口"},
		},
	}

	selected, applied := ApplyWorkbenchToolSelection(plans, selection)
	if !applied {
		t.Fatal("expected selection to apply")
	}
	if len(selected) != 3 {
		t.Fatalf("expected baseline plus selected optional plans, got %#v", selected)
	}
	if selected[0].ToolName != "web.fingerprint" || selected[1].ToolName != "web.dirscan" || selected[2].ToolName != "web.jsscan" {
		t.Fatalf("unexpected selected order: %#v", selected)
	}
	if !strings.Contains(selected[1].Reason, "模型建议") || !strings.Contains(selected[2].Reason, "模型建议") {
		t.Fatalf("expected model rationale in selected plan reasons: %#v", selected)
	}
}

func TestApplyWorkbenchToolSelectionAddsOnlySafeConfigHints(t *testing.T) {
	plans := []WorkbenchScanExecutionPlan{
		{ToolName: "web.dirscan", Reason: "默认目录"},
	}
	selection := WorkbenchToolSelectionResult{
		Tools: []WorkbenchToolSelectionItem{
			{
				ToolName: "web.dirscan",
				Reason:   "登录入口较多，降低速率并限制深度",
				ConfigHints: map[string]interface{}{
					"rateLimit":          float64(99),
					"timeoutSeconds":     float64(1),
					"maxDepth":           float64(3),
					"maxRequests":        "250",
					"includePassiveOnly": true,
					"headers":            map[string]interface{}{"Cookie": "secret"},
					"payload":            "' OR 1=1 --",
					"target":             "https://outside.example",
				},
			},
		},
	}

	selected, applied := ApplyWorkbenchToolSelection(plans, selection)
	if !applied {
		t.Fatal("expected selection to apply")
	}
	hints, ok := selected[0].Config["aiConfigHints"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected aiConfigHints to be present, got %#v", selected[0].Config)
	}
	if hints["rateLimit"] != 10 || hints["timeoutSeconds"] != 3 || hints["maxDepth"] != 3 || hints["maxRequests"] != 250 || hints["includePassiveOnly"] != true {
		t.Fatalf("unexpected sanitized hints: %#v", hints)
	}
	if _, ok := hints["headers"]; ok {
		t.Fatalf("expected headers to be ignored: %#v", hints)
	}
	if _, ok := hints["payload"]; ok {
		t.Fatalf("expected payload to be ignored: %#v", hints)
	}
	if _, ok := hints["target"]; ok {
		t.Fatalf("expected target to be ignored: %#v", hints)
	}
	if !strings.Contains(selected[0].Reason, "模型参数建议") {
		t.Fatalf("expected config hint summary in reason: %q", selected[0].Reason)
	}
}

func TestApplyWorkbenchToolSelectionMergesHistoryFeedbackHints(t *testing.T) {
	plans := []WorkbenchScanExecutionPlan{
		{ToolName: "web.dirscan", Reason: "默认目录"},
	}
	selection := WorkbenchToolSelectionResult{
		Tools: []WorkbenchToolSelectionItem{
			{
				ToolName: "web.dirscan",
				Reason:   "目录入口较多",
				ConfigHints: map[string]interface{}{
					"rateLimit":      8,
					"timeoutSeconds": 6,
				},
			},
		},
	}

	selected, applied := ApplyWorkbenchToolSelectionWithFeedback(plans, selection, []string{
		"- 历史任务 task-1 [failed]；工具明细 web.dirscan completed=0 failed=2 running=0 total=2",
	})
	if !applied {
		t.Fatal("expected selection to apply")
	}
	hints, ok := selected[0].Config["aiConfigHints"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected aiConfigHints to be present, got %#v", selected[0].Config)
	}
	if hints["rateLimit"] != 1 || hints["timeoutSeconds"] != 30 || hints["maxRequests"] != 200 || hints["includePassiveOnly"] != true {
		t.Fatalf("expected conservative feedback hints to override model hints, got %#v", hints)
	}
	if !strings.Contains(selected[0].Reason, "执行反馈参数建议") {
		t.Fatalf("expected feedback hint summary in reason: %q", selected[0].Reason)
	}
}

func TestBuildWorkbenchToolConfigHintsFromFeedbackUsesSuccessfulHistory(t *testing.T) {
	hints := BuildWorkbenchToolConfigHintsFromFeedback("web.jsscan", []string{
		"- 历史任务 task-1 [completed]；工具明细 web.jsscan completed=3 failed=0 running=0 total=3",
	})

	if hints["timeoutSeconds"] != 10 || hints["maxRequests"] != 1000 {
		t.Fatalf("unexpected success hints: %#v", hints)
	}
}

func TestApplyWorkbenchToolSelectionFallsBackWhenNoPlanMatches(t *testing.T) {
	plans := []WorkbenchScanExecutionPlan{{ToolName: "web.fingerprint"}}
	selected, applied := ApplyWorkbenchToolSelection(plans, WorkbenchToolSelectionResult{
		Tools: []WorkbenchToolSelectionItem{{ToolName: "unknown.tool"}},
	})
	if applied {
		t.Fatal("expected selection not to apply")
	}
	if len(selected) != len(plans) || selected[0].ToolName != plans[0].ToolName {
		t.Fatalf("expected original plans, got %#v", selected)
	}
}
