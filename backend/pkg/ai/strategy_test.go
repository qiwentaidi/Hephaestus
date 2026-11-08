package ai

import (
	"strings"
	"testing"
)

func TestSelectWorkbenchStrategyPacksForDomainAndWebScope(t *testing.T) {
	got := SelectWorkbenchStrategyPacks(
		"scan",
		"example.com",
		[]string{"https://example.com/login"},
		"攻击面资产和登录接口测试",
		"输出证据清单",
	)

	assertContainsStrategy(t, got, "external_attack_surface")
	assertContainsStrategy(t, got, "single_web_deep_test")
	assertContainsStrategy(t, got, "js_api_analysis")
	assertContainsStrategy(t, got, "verification_report")
}

func TestSelectWorkbenchStrategyPacksForCIDR(t *testing.T) {
	got := SelectWorkbenchStrategyPacks(
		"agent",
		"10.0.0.0/24",
		nil,
		"识别授权网段中的 Web 服务",
		"",
	)

	assertContainsStrategy(t, got, "external_attack_surface")
}

func TestStrategyPackPromptIncludesGuidance(t *testing.T) {
	prompt := StrategyPackPrompt([]string{"single_web_deep_test", "unknown"})
	if !strings.Contains(prompt, "单站深度测试") {
		t.Fatalf("expected prompt to include strategy title, got %q", prompt)
	}
	if !strings.Contains(prompt, "web.vuln.verify") {
		t.Fatalf("expected prompt to include preferred capabilities, got %q", prompt)
	}
}

func TestRefineWorkbenchStrategyPacksAddsEvidenceDrivenStrategies(t *testing.T) {
	got := RefineWorkbenchStrategyPacks(
		[]string{"external_attack_surface"},
		"HTTP 观察发现 /api/users、/auth/login 和高危漏洞候选",
	)

	assertContainsStrategy(t, got, "external_attack_surface")
	assertContainsStrategy(t, got, "js_api_analysis")
	assertContainsStrategy(t, got, "single_web_deep_test")
	assertContainsStrategy(t, got, "verification_report")
}

func TestBuildWorkbenchStrategySelectionPromptUsesBuiltinCatalog(t *testing.T) {
	prompt := BuildWorkbenchStrategySelectionPrompt(WorkbenchStrategySelectionEvidence{
		Mode:                 "agent",
		Target:               "https://example.test",
		Scope:                []string{"https://example.test/login"},
		Objective:            "登录接口测试",
		SuccessCriteria:      "输出证据链",
		CurrentStrategyPacks: []string{"single_web_deep_test"},
		Signals:              []string{"发现 /api/users"},
		HistoryFeedback:      []string{"- 历史任务 task-1 策略包 js_api_analysis；评估 pass/90"},
		AvailablePacks:       ListWorkbenchStrategyPacks(),
	})

	for _, want := range []string{"只能从可用策略包清单中选择", "strategyPacks", "single_web_deep_test", "js_api_analysis", "/api/users", "历史任务 task-1"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestParseWorkbenchStrategySelectionResultFiltersUnknownPacks(t *testing.T) {
	selection, ok := ParseWorkbenchStrategySelectionResult(`{
		"strategyPacks": [
			{"id": "js_api_analysis", "reason": "API 线索"},
			{"id": "unknown", "reason": "未知策略"},
			{"id": "js_api_analysis", "reason": "重复"},
			{"id": "verification_report", "reason": "需要证据交付"}
		],
		"summary": "优先 JS/API"
	}`)
	if !ok {
		t.Fatal("expected selection to parse")
	}
	if len(selection.StrategyPacks) != 2 {
		t.Fatalf("expected two known strategies, got %#v", selection.StrategyPacks)
	}
	if selection.StrategyPacks[0].ID != "js_api_analysis" || selection.StrategyPacks[1].ID != "verification_report" {
		t.Fatalf("unexpected strategy order: %#v", selection.StrategyPacks)
	}
}

func TestApplyWorkbenchStrategySelectionMergesModelAndRuleStrategies(t *testing.T) {
	selected, applied := ApplyWorkbenchStrategySelection(
		[]string{"external_attack_surface", "single_web_deep_test"},
		WorkbenchStrategySelectionResult{
			StrategyPacks: []WorkbenchStrategySelectionItem{
				{ID: "js_api_analysis"},
				{ID: "unknown"},
				{ID: "single_web_deep_test"},
			},
		},
		4,
	)
	if !applied {
		t.Fatal("expected strategy selection to apply")
	}
	want := []string{"js_api_analysis", "single_web_deep_test", "external_attack_surface"}
	if strings.Join(selected, ",") != strings.Join(want, ",") {
		t.Fatalf("selected strategies = %#v, want %#v", selected, want)
	}
}

func TestApplyWorkbenchStrategySelectionFallsBackWhenNoModelPackMatches(t *testing.T) {
	rule := []string{"single_web_deep_test"}
	selected, applied := ApplyWorkbenchStrategySelection(rule, WorkbenchStrategySelectionResult{
		StrategyPacks: []WorkbenchStrategySelectionItem{{ID: "unknown"}},
	}, 4)
	if applied {
		t.Fatal("expected selection not to apply")
	}
	if len(selected) != len(rule) || selected[0] != rule[0] {
		t.Fatalf("expected rule fallback, got %#v", selected)
	}
}

func TestSelectWorkbenchStrategyPacksFromCatalogUsesCustomTriggers(t *testing.T) {
	catalog := append(ListWorkbenchStrategyPacks(), WorkbenchStrategyPack{
		ID:          "custom_business_auth",
		Title:       "业务鉴权专项",
		Source:      "custom",
		Enabled:     true,
		Description: "聚焦业务鉴权边界。",
		Triggers:    []string{"业务鉴权"},
	})

	got := SelectWorkbenchStrategyPacksFromCatalog(
		catalog,
		"agent",
		"https://example.test",
		nil,
		"验证业务鉴权边界",
		"输出证据链",
	)

	assertContainsStrategy(t, got, "custom_business_auth")
}

func TestStrategyPackPromptFromCatalogIncludesCustomPack(t *testing.T) {
	prompt := StrategyPackPromptFromCatalog([]string{"custom_business_auth"}, []WorkbenchStrategyPack{
		{
			ID:                    "custom_business_auth",
			Title:                 "业务鉴权专项",
			Source:                "custom",
			Enabled:               true,
			Scenario:              "登录态业务测试",
			Description:           "聚焦业务鉴权边界。",
			PreferredCapabilities: []string{"web.repeater"},
		},
	})
	if !strings.Contains(prompt, "业务鉴权专项") || !strings.Contains(prompt, "web.repeater") {
		t.Fatalf("expected custom strategy in prompt, got %q", prompt)
	}
}

func TestParseWorkbenchStrategySelectionResultWithCatalogSkipsDisabledCustomPack(t *testing.T) {
	catalog := []WorkbenchStrategyPack{
		{ID: "custom_disabled", Source: "custom", Enabled: false},
		{ID: "custom_enabled", Source: "custom", Enabled: true},
	}
	selection, ok := ParseWorkbenchStrategySelectionResultWithCatalog(`{
		"strategyPacks": [
			{"id": "custom_disabled", "reason": "禁用"},
			{"id": "custom_enabled", "reason": "启用"}
		]
	}`, catalog)
	if !ok {
		t.Fatal("expected enabled custom pack to parse")
	}
	if len(selection.StrategyPacks) != 1 || selection.StrategyPacks[0].ID != "custom_enabled" {
		t.Fatalf("unexpected selection: %#v", selection.StrategyPacks)
	}
}

func TestAdjustWorkbenchStrategyPacksWithFeedbackReplacesWeakStrategy(t *testing.T) {
	next, adjustments := AdjustWorkbenchStrategyPacksWithFeedback(
		ListWorkbenchStrategyPacks(),
		[]string{"single_web_deep_test", "verification_report"},
		[]string{"single_web_deep_test", "js_api_analysis", "verification_report"},
		[]string{"- 历史任务 task-1 [failed]；策略包 single_web_deep_test；评估 fail/42；工具调用 completed=0 failed=2 total=2"},
		[]string{"发现 /api/users、token 和 auth 接口"},
		4,
	)

	if testContainsStrategy(next, "single_web_deep_test") {
		t.Fatalf("expected weak strategy to be downgraded, got %#v", next)
	}
	assertContainsStrategy(t, next, "js_api_analysis")
	assertContainsStrategy(t, next, "verification_report")
	if len(adjustments) == 0 {
		t.Fatalf("expected strategy adjustments")
	}
	if adjustments[0].StrategyID != "single_web_deep_test" || adjustments[0].Action != "downgrade" {
		t.Fatalf("unexpected adjustment: %#v", adjustments)
	}
}

func TestAdjustWorkbenchStrategyPacksWithFeedbackKeepsSignalSupportedStrategy(t *testing.T) {
	next, adjustments := AdjustWorkbenchStrategyPacksWithFeedback(
		ListWorkbenchStrategyPacks(),
		[]string{"single_web_deep_test", "verification_report"},
		[]string{"single_web_deep_test", "verification_report"},
		[]string{"- 历史任务 task-1 [failed]；策略包 single_web_deep_test；评估 fail/42；工具调用 completed=0 failed=2 total=2"},
		[]string{"HTTP 表单页面和 web 登录入口仍是当前关键证据"},
		4,
	)

	assertContainsStrategy(t, next, "single_web_deep_test")
	if len(adjustments) != 0 {
		t.Fatalf("expected signal-supported strategy to be retained without adjustment, got %#v", adjustments)
	}
}

func TestAdjustWorkbenchStrategyPacksWithFeedbackNeverEmptiesSelection(t *testing.T) {
	next, _ := AdjustWorkbenchStrategyPacksWithFeedback(
		ListWorkbenchStrategyPacks(),
		[]string{"verification_report"},
		[]string{"verification_report"},
		[]string{"- 历史任务 task-1 [failed]；策略包 verification_report；评估 fail/30；工具调用 completed=0 failed=1 total=1"},
		nil,
		4,
	)

	if len(next) == 0 {
		t.Fatal("expected at least one strategy to remain")
	}
	assertContainsStrategy(t, next, "verification_report")
}

func TestScoreWorkbenchStrategyEffectsRanksWeakStrategiesFirst(t *testing.T) {
	scores := ScoreWorkbenchStrategyEffects(ListWorkbenchStrategyPacks(), []string{
		"- 历史任务 task-1 [failed]；策略包 single_web_deep_test；评估 fail/42；工具调用 completed=0 failed=2 total=2",
		"- 历史任务 task-2 [completed]；策略包 js_api_analysis；评估 pass/92；工具调用 completed=3 failed=0 total=3",
	})

	if len(scores) != 2 {
		t.Fatalf("expected two strategy scores, got %#v", scores)
	}
	if scores[0].StrategyID != "single_web_deep_test" || scores[0].Score >= 50 || scores[0].Negative != 1 {
		t.Fatalf("expected weak strategy first, got %#v", scores)
	}
	if scores[1].StrategyID != "js_api_analysis" || scores[1].Score <= 50 || scores[1].Positive != 1 {
		t.Fatalf("expected positive strategy score, got %#v", scores)
	}
}

func TestBuildWorkbenchStrategyExperimentsUsesHistoricalChallenger(t *testing.T) {
	experiments := BuildWorkbenchStrategyExperiments(
		ListWorkbenchStrategyPacks(),
		[]string{"single_web_deep_test"},
		[]string{"js_api_analysis"},
		[]WorkbenchStrategyEffectScore{
			{StrategyID: "single_web_deep_test", Score: 35, Negative: 1, Reason: "历史失败较多。"},
			{StrategyID: "js_api_analysis", Score: 90, Positive: 2, Reason: "历史通过率高。"},
		},
		nil,
		2,
	)

	if len(experiments) != 1 {
		t.Fatalf("expected one strategy experiment, got %#v", experiments)
	}
	if experiments[0].ChampionID != "single_web_deep_test" || experiments[0].ChallengerID != "js_api_analysis" {
		t.Fatalf("unexpected experiment: %#v", experiments[0])
	}
	if experiments[0].ChallengerScore != 90 || !strings.Contains(strings.Join(experiments[0].Basis, "\n"), "challengerHistory") {
		t.Fatalf("expected challenger score and basis, got %#v", experiments[0])
	}
}

func TestBuildWorkbenchStrategyExperimentsUsesSignalSupportedChallenger(t *testing.T) {
	experiments := BuildWorkbenchStrategyExperiments(
		ListWorkbenchStrategyPacks(),
		[]string{"external_attack_surface"},
		nil,
		nil,
		[]string{"HTTP 观察发现 /api/users、auth token 和登录接口"},
		2,
	)

	if len(experiments) == 0 {
		t.Fatalf("expected signal-driven strategy experiment")
	}
	if experiments[0].ChallengerID != "single_web_deep_test" && experiments[0].ChallengerID != "js_api_analysis" {
		t.Fatalf("expected web or js challenger, got %#v", experiments[0])
	}
	if !strings.Contains(strings.Join(experiments[0].Basis, "\n"), "currentSignalsSupportChallenger=true") {
		t.Fatalf("expected signal support basis, got %#v", experiments[0])
	}
}

func assertContainsStrategy(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("expected strategies %#v to contain %q", values, want)
}

func testContainsStrategy(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
