package ai

import "testing"

func TestBuildWorkbenchScanExecutionPlansDiscovery(t *testing.T) {
	plans := BuildWorkbenchScanExecutionPlans(
		"站点评估",
		"example.com",
		[]string{"https://example.com/app"},
		"tool_orchestration",
		WorkbenchRiskManualApproval,
	)

	byType := make(map[string]WorkbenchScanExecutionPlan, len(plans))
	for _, plan := range plans {
		byType[plan.ScanType] = plan
	}
	for _, scanType := range []string{"subdomain", "portscan", "webscan", "dirscan", "jsscan"} {
		plan, ok := byType[scanType]
		if !ok {
			t.Fatalf("expected %s plan in %#v", scanType, plans)
		}
		if !plan.Executable {
			t.Fatalf("expected %s plan to be executable: %s", scanType, plan.Reason)
		}
	}

	if got := byType["webscan"].Config["webscanOption"]; got != 2 {
		t.Fatalf("webscanOption = %#v, want 2", got)
	}
	if got := byType["webscan"].Config["vulnerability"]; got != false {
		t.Fatalf("vulnerability = %#v, want false", got)
	}
	if got := byType["webscan"].Config["deepScan"]; got != true {
		t.Fatalf("deepScan = %#v, want true", got)
	}
	if byType["webscan"].ToolName != "web.fingerprint" {
		t.Fatalf("webscan ToolName = %q, want web.fingerprint", byType["webscan"].ToolName)
	}
	if byType["webscan"].Optional {
		t.Fatalf("webscan should be a baseline plan")
	}
	if byType["jsscan"].Optional {
		t.Fatalf("jsscan should be a baseline plan")
	}
	if !byType["dirscan"].Optional {
		t.Fatalf("dirscan should be conditional optional plan")
	}
	defaultPlans := DefaultWorkbenchExecutablePlans(plans)
	defaultByType := make(map[string]WorkbenchScanExecutionPlan, len(defaultPlans))
	for _, plan := range defaultPlans {
		defaultByType[plan.ScanType] = plan
	}
	if _, ok := defaultByType["dirscan"]; ok {
		t.Fatalf("dirscan should not be in default execution plans: %#v", defaultPlans)
	}
	for _, scanType := range []string{"subdomain", "portscan", "webscan", "jsscan"} {
		if _, ok := defaultByType[scanType]; !ok {
			t.Fatalf("expected %s in default execution plans: %#v", scanType, defaultPlans)
		}
	}
	if got, ok := byType["portscan"].Config["portRange"]; ok {
		t.Fatalf("portscan planner should defer to the server default port group, got portRange %#v", got)
	}
}

func TestWorkbenchScanExecutionPlansFingerprintOnlyRequest(t *testing.T) {
	plans := BuildWorkbenchScanExecutionPlans(
		"只做指纹识别",
		"https://example.com",
		nil,
		"tool_orchestration",
		WorkbenchRiskManualApproval,
	)

	var webscan WorkbenchScanExecutionPlan
	for _, plan := range plans {
		if plan.ScanType == "webscan" {
			webscan = plan
			break
		}
	}
	if webscan.ScanType == "" {
		t.Fatalf("expected webscan plan in %#v", plans)
	}
	if got := webscan.Config["webscanOption"]; got != 2 {
		t.Fatalf("fingerprint-only webscanOption = %#v, want 2", got)
	}
	if got := webscan.Config["vulnerability"]; got != false {
		t.Fatalf("fingerprint-only vulnerability = %#v, want false", got)
	}
	if webscan.ToolName != "web.fingerprint" {
		t.Fatalf("fingerprint-only ToolName = %q, want web.fingerprint", webscan.ToolName)
	}
}

func TestWorkbenchScanExecutionPlansPreserveCIDRForPortscan(t *testing.T) {
	plans := BuildWorkbenchScanExecutionPlans(
		"网段评估",
		"10.0.0.0/24",
		nil,
		"tool_orchestration",
		WorkbenchRiskManualApproval,
	)

	for _, plan := range plans {
		if plan.ScanType == "portscan" {
			if len(plan.Targets) != 1 || plan.Targets[0] != "10.0.0.0/24" {
				t.Fatalf("portscan targets = %#v, want CIDR preserved", plan.Targets)
			}
			if got, ok := plan.Config["portRange"]; ok {
				t.Fatalf("portscan planner should defer to the server default port group, got portRange %#v", got)
			}
			return
		}
	}
	t.Fatalf("expected portscan plan in %#v", plans)
}

func TestWorkbenchScanExecutionPlansDoNotFallbackToWebscanForUnknownTarget(t *testing.T) {
	plans := BuildWorkbenchScanExecutionPlans(
		"未知目标评估",
		"not a target",
		nil,
		"tool_orchestration",
		WorkbenchRiskManualApproval,
	)

	if len(plans) != 1 {
		t.Fatalf("plans = %#v, want one skipped plan", plans)
	}
	if plans[0].Executable {
		t.Fatalf("unknown target should not create executable scan plan: %#v", plans[0])
	}
	if plans[0].ScanType == "webscan" {
		t.Fatalf("unknown target should not fallback to webscan: %#v", plans[0])
	}
}

func TestBuildWorkbenchScanExecutionPlanReadOnlySkipsVerification(t *testing.T) {
	plan := BuildWorkbenchScanExecutionPlan(
		"",
		"https://example.com",
		nil,
		"verification",
		WorkbenchRiskReadOnly,
	)

	if plan.Executable {
		t.Fatalf("expected read-only verification to be skipped")
	}
	if plan.ToolName != "web.vuln.verify" {
		t.Fatalf("ToolName = %q, want web.vuln.verify", plan.ToolName)
	}
}

func TestBuildWorkbenchScanExecutionPlanManualVerification(t *testing.T) {
	plan := BuildWorkbenchScanExecutionPlan(
		"",
		"https://example.com",
		nil,
		"verification",
		WorkbenchRiskManualApproval,
	)

	if !plan.Executable {
		t.Fatalf("expected manual approval verification plan to be executable after approval")
	}
	if got := plan.Config["webscanOption"]; got != 3 {
		t.Fatalf("webscanOption = %#v, want 3", got)
	}
	if got := plan.Config["vulnerability"]; got != true {
		t.Fatalf("vulnerability = %#v, want true", got)
	}
}

func TestBuildWorkbenchScanExecutionPlanSkipsCIDRVerificationWithoutWebURL(t *testing.T) {
	plan := BuildWorkbenchScanExecutionPlan(
		"网段评估",
		"192.168.2.1/24",
		nil,
		"verification",
		WorkbenchRiskManualApproval,
	)

	if plan.Executable {
		t.Fatalf("expected CIDR verification without derived web URL to be skipped, got %#v", plan)
	}
	if len(plan.Targets) != 0 {
		t.Fatalf("verification targets = %#v, want empty web targets", plan.Targets)
	}
}

func TestBuildWorkbenchScanExecutionPlanUsesOnlyWebURLsForVerification(t *testing.T) {
	plan := BuildWorkbenchScanExecutionPlan(
		"网段评估",
		"192.168.2.1/24",
		[]string{"192.168.2.1/24", "http://192.168.2.10", "https://192.168.2.11:8443"},
		"verification",
		WorkbenchRiskManualApproval,
	)

	want := []string{"http://192.168.2.10", "https://192.168.2.11:8443"}
	if len(plan.Targets) != len(want) {
		t.Fatalf("verification targets = %#v, want %#v", plan.Targets, want)
	}
	for index := range want {
		if plan.Targets[index] != want[index] {
			t.Fatalf("verification targets = %#v, want %#v", plan.Targets, want)
		}
	}
}

func TestBuildWorkbenchScanExecutionPlanForScanTypeRejectsUnsupportedTargets(t *testing.T) {
	plan := BuildWorkbenchScanExecutionPlanForScanType(
		"网段评估",
		"192.168.2.1/24",
		nil,
		"tool_orchestration",
		"webscan",
		WorkbenchRiskManualApproval,
	)

	if plan.Executable {
		t.Fatalf("webscan should reject CIDR targets, got %#v", plan)
	}
	if len(plan.Targets) != 0 {
		t.Fatalf("webscan targets = %#v, want empty", plan.Targets)
	}
	if plan.ScanType != "webscan" {
		t.Fatalf("ScanType = %q, want webscan", plan.ScanType)
	}
}

func TestBuildWorkbenchScanExecutionPlanForScanTypeAllowsCIDRPortscan(t *testing.T) {
	plan := BuildWorkbenchScanExecutionPlanForScanType(
		"网段评估",
		"192.168.2.1/24",
		nil,
		"tool_orchestration",
		"portscan",
		WorkbenchRiskManualApproval,
	)

	if !plan.Executable {
		t.Fatalf("portscan should accept CIDR targets: %#v", plan)
	}
	if len(plan.Targets) != 1 || plan.Targets[0] != "192.168.2.1/24" {
		t.Fatalf("portscan targets = %#v, want CIDR preserved", plan.Targets)
	}
	if got, ok := plan.Config["portRange"]; ok {
		t.Fatalf("portscan planner should defer to the server default port group, got portRange %#v", got)
	}
}
