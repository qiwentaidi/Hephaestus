package ai

import (
	"path/filepath"
	"strings"
	"testing"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/taskstate"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCollectWorkbenchScanIncrementSummary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.FingerprintInfo{},
		&database.VulnerabilityInfo{},
		&database.VulnScanJSAsset{},
		&database.VulnScanDirResult{},
		&database.PortScanResult{},
		&database.VulnScanTaskItem{},
	); err != nil {
		t.Fatalf("migrate tables: %v", err)
	}

	scanTask := database.VulnScanTask{
		ID:           "scan-1",
		Version:      2,
		ScanType:     "webscan",
		Status:       "running",
		CurrentStage: "fingerprint",
		CheckpointJSON: taskstate.MarshalCheckpoint("webscan:fingerprint", 40, map[string]interface{}{
			"processed":     2,
			"authorization": "Bearer should-not-leak",
			"stageEvents": []map[string]interface{}{
				{"stage": "probe", "label": "存活探测"},
				{"stage": "fingerprint", "label": "指纹扫描"},
			},
		}),
		Progress:   40,
		TotalCount: 3,
		ErrorCount: 1,
		ErrorMsg:   "全局连接超时",
	}
	if err := db.Create(&database.FingerprintInfo{
		TaskID:       scanTask.ID,
		Version:      scanTask.Version,
		URL:          "https://example.test",
		Title:        "Example",
		Fingerprints: `["nginx"]`,
	}).Error; err != nil {
		t.Fatalf("create fingerprint: %v", err)
	}
	if err := db.Create(&database.VulnScanJSAsset{
		ID:      "js-1",
		TaskID:  scanTask.ID,
		Version: scanTask.Version,
		Type:    "apiRoute",
		Value:   "/api/users",
	}).Error; err != nil {
		t.Fatalf("create js asset: %v", err)
	}
	if err := db.Create(&database.VulnScanDirResult{
		ID:      "dir-1",
		TaskID:  scanTask.ID,
		Version: scanTask.Version,
		URL:     "https://example.test/admin",
		Status:  200,
		Title:   "Admin",
	}).Error; err != nil {
		t.Fatalf("create dir result: %v", err)
	}
	if err := db.Create(&database.VulnScanTaskItem{
		TaskID:   scanTask.ID,
		Version:  scanTask.Version,
		ScanType: scanTask.ScanType,
		Target:   "https://example.test/slow",
		Status:   "failed",
		ErrorMsg: "request timeout",
	}).Error; err != nil {
		t.Fatalf("create task item: %v", err)
	}

	summary, err := CollectWorkbenchScanIncrementSummary(db, scanTask)
	if err != nil {
		t.Fatalf("collect summary: %v", err)
	}
	if summary.FingerprintCount != 1 || summary.JSAssetCount != 1 || summary.DirResultCount != 1 {
		t.Fatalf("unexpected counts: %#v", summary)
	}
	if len(summary.StageEvents) == 0 {
		t.Fatalf("expected stage events in summary")
	}
	if len(summary.ErrorSamples) != 2 {
		t.Fatalf("expected task and target error samples, got %#v", summary.ErrorSamples)
	}
	if len(summary.CheckpointSignals) == 0 {
		t.Fatalf("expected checkpoint signals, got %#v", summary.CheckpointSignals)
	}
	text := summary.Text()
	for _, want := range []string{"进度 40%", "指纹 1 条", "/api/users", "/admin", "阶段事件", "阶段检查点", "错误样本", "request timeout", "执行器检查点信号", "processed=2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary text %q missing %q", text, want)
		}
	}
	if strings.Contains(text, "should-not-leak") {
		t.Fatalf("checkpoint sensitive signal leaked: %s", text)
	}
}

func TestCollectWorkbenchDerivedHTTPURLsPrefersCIDRPortResults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.FingerprintInfo{},
		&database.VulnScanDirResult{},
		&database.PortScanResult{},
	); err != nil {
		t.Fatalf("migrate tables: %v", err)
	}

	if err := db.Create(&database.FingerprintInfo{
		TaskID: "old-scan",
		URL:    "http://192.168.2.101:48080/actuator",
		Host:   "192.168.2.101",
	}).Error; err != nil {
		t.Fatalf("create fingerprint: %v", err)
	}
	for _, item := range []database.PortScanResult{
		{TaskID: "port-scan", Host: "192.168.2.1", Port: 80, Protocol: "http"},
		{TaskID: "port-scan", Host: "192.168.2.101", Port: 9000, Protocol: "http", Product: "OpenResty web app server"},
	} {
		if err := db.Create(&item).Error; err != nil {
			t.Fatalf("create port result: %v", err)
		}
	}

	urls, err := CollectWorkbenchDerivedHTTPURLs(db, "192.168.2.1/24", nil, 3)
	if err != nil {
		t.Fatalf("collect derived urls: %v", err)
	}
	portURLs := map[string]bool{
		"http://192.168.2.1":         true,
		"http://192.168.2.101:9000":  true,
		"http://192.168.2.101:48080": false,
	}
	if len(urls) < 3 {
		t.Fatalf("expected at least 3 URLs, got %#v", urls)
	}
	for _, got := range urls[:2] {
		if !portURLs[got] {
			t.Fatalf("expected first two URLs to come from port scan results, got %#v", urls)
		}
	}
	if urls[2] != "http://192.168.2.101:48080/actuator" {
		t.Fatalf("expected historical fingerprint after port URLs, got %#v", urls)
	}
}

func TestBuildWorkbenchScanIncrementCompressionPromptIncludesSummaryAndGuardrails(t *testing.T) {
	prompt := BuildWorkbenchScanIncrementCompressionPrompt(WorkbenchScanIncrementCompressionEvidence{
		TaskName:        "扫描任务",
		Target:          "https://example.test",
		RiskPolicy:      WorkbenchRiskManualApproval,
		StrategyPacks:   []string{"single_web_deep_test"},
		SuccessCriteria: "输出证据链",
		Increment: WorkbenchScanIncrementSummary{
			ScanTaskID:       "scan-1",
			ScanType:         "webscan",
			Status:           "running",
			CurrentStage:     "dirscan",
			Progress:         60,
			FingerprintCount: 1,
			StageEvents: []WorkbenchScanStageEvent{
				{Kind: "stage_checkpoint", Stage: "dirscan", Status: "running", Progress: 60, Message: "阶段检查点: dirscan，状态 running，进度 60%。"},
			},
			ErrorSamples: []string{"目标错误: [failed] https://example.test/slow request timeout"},
			CheckpointSignals: []string{
				"checkpoint.stage=webscan:dirscan progress=60",
				"checkpoint.processed=8",
			},
			Highlights: []string{"目录样本: [200] https://example.test/admin Admin"},
		},
	})

	for _, want := range []string{"长扫描增量压缩器", "不要编造发现", "只输出 JSON", "scan-1", "阶段事件", "stage_checkpoint", "错误样本", "request timeout", "执行器检查点信号", "processed=8", "/admin"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestBuildWorkbenchCheckpointSignalsFiltersAndBoundsRawMeta(t *testing.T) {
	raw := taskstate.MarshalCheckpoint("webscan:nuclei", 80, map[string]interface{}{
		"processed": 12,
		"token":     "secret-token",
		"queue":     []interface{}{"target-1", "target-2", "target-3", "target-4", "target-5"},
		"stageEvents": []map[string]interface{}{
			{"stage": "probe", "label": "存活探测"},
			{"stage": "nuclei", "label": "漏洞扫描"},
		},
	})

	signals := BuildWorkbenchCheckpointSignals(raw, 8)
	joined := strings.Join(signals, "\n")
	for _, want := range []string{"checkpoint.stage=webscan:nuclei progress=80", "processed=12", "stageEvent", "漏洞扫描", "target-1"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("signals missing %q: %#v", want, signals)
		}
	}
	if strings.Contains(joined, "secret-token") {
		t.Fatalf("sensitive checkpoint value leaked: %#v", signals)
	}
}

func TestBuildWorkbenchScanStageEventsIncludesErrorCount(t *testing.T) {
	events := BuildWorkbenchScanStageEvents(WorkbenchScanIncrementSummary{
		ScanTaskID:   "scan-1",
		Status:       "running",
		CurrentStage: "dirscan",
		Progress:     70,
		ErrorCount:   2,
	})

	if len(events) != 3 {
		t.Fatalf("expected checkpoint, count and error events, got %#v", events)
	}
	if events[2].Kind != "error_count" || !strings.Contains(events[2].Message, "2 个错误") {
		t.Fatalf("unexpected error event: %#v", events[2])
	}
}

func TestParseWorkbenchScanIncrementCompressionResult(t *testing.T) {
	fallback := "关联扫描任务 scan-1 运行中摘要：状态 running。"
	text, result, ok := ParseWorkbenchScanIncrementCompressionResult(`{
		"summary": "目录阶段发现后台入口，尚未出现漏洞命中。",
		"keyFindings": ["发现 /admin 200", "发现 /admin 200"],
		"failedBranches": ["2 个目标超时"],
		"nextSignals": ["管理入口", "访问控制"],
		"confidence": "high"
	}`, fallback)
	if !ok {
		t.Fatal("expected compression result to parse")
	}
	if result.Confidence != "high" || len(result.KeyFindings) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	for _, want := range []string{"关联扫描模型压缩摘要", "关键发现", "失败或阻塞分支", "原始增量摘要", fallback} {
		if !strings.Contains(text, want) {
			t.Fatalf("compressed text missing %q: %s", want, text)
		}
	}
}

func TestParseWorkbenchScanIncrementCompressionResultFallsBackOnInvalidOutput(t *testing.T) {
	fallback := "原始摘要"
	text, _, ok := ParseWorkbenchScanIncrementCompressionResult("继续观察", fallback)
	if ok {
		t.Fatal("expected invalid compression output to be rejected")
	}
	if text != fallback {
		t.Fatalf("expected fallback text, got %q", text)
	}
}
