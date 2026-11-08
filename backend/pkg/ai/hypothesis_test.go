package ai

import (
	"strings"
	"testing"
)

func TestBuildWorkbenchHypothesisDraftsFromHTTPAndStrategySignals(t *testing.T) {
	drafts := BuildWorkbenchHypothesisDrafts(
		"https://example.test",
		[]string{"single_web_deep_test", "js_api_analysis"},
		[]string{"HTTP 观察发现登录表单 /auth/login、接口 /api/users、后台 /admin"},
		WorkbenchTargetContext{},
	)

	assertHypothesis(t, drafts, "认证与会话边界验证")
	assertHypothesis(t, drafts, "JS/API 敏感信息与未授权接口")
	assertHypothesis(t, drafts, "敏感入口与管理面暴露")
}

func TestBuildWorkbenchHypothesisDraftsFromAssetContext(t *testing.T) {
	drafts := BuildWorkbenchHypothesisDrafts(
		"example.com",
		nil,
		nil,
		WorkbenchTargetContext{
			FingerprintCount:   2,
			VulnerabilityCount: 1,
			PortCount:          3,
		},
	)

	assertHypothesis(t, drafts, "暴露面与指纹关联验证")
	assertHypothesis(t, drafts, "候选漏洞最小化复测")
}

func TestBuildWorkbenchHypothesisDraftsFallback(t *testing.T) {
	drafts := BuildWorkbenchHypothesisDrafts("plain-target", nil, nil, WorkbenchTargetContext{})
	if len(drafts) != 1 {
		t.Fatalf("expected one fallback draft, got %#v", drafts)
	}
	if drafts[0].Title != "基础 Web 行为观察" {
		t.Fatalf("fallback title = %q", drafts[0].Title)
	}
}

func TestBuildWorkbenchHypothesisPromptIncludesEvidenceAndGuardrails(t *testing.T) {
	prompt := BuildWorkbenchHypothesisPrompt(WorkbenchHypothesisEvidence{
		TaskName:        "单站深测",
		Target:          "https://example.test",
		RiskPolicy:      WorkbenchRiskManualApproval,
		StrategyPacks:   []string{"single_web_deep_test"},
		SuccessCriteria: "输出证据链",
		Observations:    []string{"- [http_observer/medium] 发现登录入口"},
		EvidenceItems:   []string{"- [info/medium] evidence: 登录表单"},
		AssetBuckets:    []string{"- [target/20] https://example.test: 有 JS 资产"},
		HistoryFeedback: []string{"- 历史任务 task-1 评估 needs_review/78；缺口 补充认证边界证据"},
	})

	for _, want := range []string{"只生成假设", "不要生成漏洞利用步骤", "CORS", "不要提出、规划、选择或执行 CORS 漏洞检测", "只输出 JSON", "hypotheses", "priority", "登录入口", "历史任务 task-1"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestParseWorkbenchHypothesisDrafts(t *testing.T) {
	drafts, ok := ParseWorkbenchHypothesisDrafts(`{
		"hypotheses": [
			{"title": "认证接口越权候选", "description": "登录接口和用户接口需要在授权账号内验证访问边界。", "confidence": "high", "evidenceRefs": ["ev-1", "ev-1"]},
			{"title": "认证接口越权候选", "description": "重复标题应合并。", "confidence": "low"},
			{"title": "JS 暴露接口候选", "description": "前端脚本暴露 API 路径，需要只读确认鉴权要求。", "confidence": "unknown", "evidenceRefs": ["ev-2"]}
		],
		"summary": "优先认证和 JS/API"
	}`)
	if !ok {
		t.Fatal("expected model hypotheses to parse")
	}
	if len(drafts) != 2 {
		t.Fatalf("expected two drafts, got %#v", drafts)
	}
	if drafts[0].Confidence != "high" || len(drafts[0].EvidenceRefs) != 1 || drafts[0].EvidenceRefs[0] != "ev-1" {
		t.Fatalf("unexpected first draft: %#v", drafts[0])
	}
	if drafts[1].Confidence != "medium" {
		t.Fatalf("invalid confidence should default to medium: %#v", drafts[1])
	}
}

func TestParseWorkbenchHypothesisDraftsSortsByPriority(t *testing.T) {
	drafts, ok := ParseWorkbenchHypothesisDrafts(`{
		"hypotheses": [
			{"title": "低优先级候选", "description": "证据较弱。", "confidence": "low", "priority": 10},
			{"title": "高优先级候选", "description": "历史反馈和当前证据都指向该分支。", "confidence": "medium", "priority": 95}
		]
	}`)
	if !ok {
		t.Fatal("expected model hypotheses to parse")
	}
	if drafts[0].Title != "高优先级候选" || drafts[0].Priority != 95 {
		t.Fatalf("expected high priority draft first, got %#v", drafts)
	}
}

func TestParseWorkbenchHypothesisDraftsRejectsInvalidOutput(t *testing.T) {
	if _, ok := ParseWorkbenchHypothesisDrafts("结论：继续测试"); ok {
		t.Fatal("expected invalid output to be rejected")
	}
}

func TestMergeWorkbenchHypothesisDraftsPrioritizesModelDrafts(t *testing.T) {
	modelDrafts := []WorkbenchHypothesisDraft{
		{Title: "JS 暴露接口候选", Description: "模型建议", Confidence: "high"},
	}
	ruleDrafts := []WorkbenchHypothesisDraft{
		{Title: "JS 暴露接口候选", Description: "规则重复", Confidence: "medium"},
		{Title: "敏感入口与管理面暴露", Description: "规则补充", Confidence: "medium"},
	}

	merged := MergeWorkbenchHypothesisDrafts(modelDrafts, ruleDrafts, 4)
	if len(merged) != 2 {
		t.Fatalf("expected two merged drafts, got %#v", merged)
	}
	if merged[0].Description != "模型建议" || merged[1].Title != "敏感入口与管理面暴露" {
		t.Fatalf("unexpected merge order: %#v", merged)
	}
}

func assertHypothesis(t *testing.T, drafts []WorkbenchHypothesisDraft, title string) {
	t.Helper()
	for _, draft := range drafts {
		if draft.Title == title {
			return
		}
	}
	t.Fatalf("hypothesis %q not found in %#v", title, drafts)
}
