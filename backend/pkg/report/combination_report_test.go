package report

import (
	"testing"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
)

func TestLoadMergedTaskDataIncludesCombinationChildrenResults(t *testing.T) {
	config.SetRuntimeMode(config.RuntimeModeDesktop)
	t.Setenv("HEPHAESTUS_CONFIG_DIR", t.TempDir())

	db, err := database.GetDB()
	if err != nil {
		t.Fatalf("GetDB() error = %v", err)
	}

	now := time.Now()
	comboID := "combo-report-test"
	parentTaskID := "combo-parent-task"
	childTaskID := "combo-child-webscan"

	parentTask := database.VulnScanTask{
		ID:                  parentTaskID,
		TaskName:            "组合任务",
		ScanType:            "combination",
		ParentCombinationID: comboID,
		Status:              "completed",
		Version:             1,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	childTask := database.VulnScanTask{
		ID:                  childTaskID,
		TaskName:            "子任务-webscan",
		ScanType:            "webscan",
		ParentCombinationID: comboID,
		StepIndex:           1,
		StepModule:          "webscan",
		Status:              "completed",
		Version:             1,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	childVuln := database.VulnerabilityInfo{
		TaskID:      childTaskID,
		Version:     1,
		VulnName:    "test vuln",
		TemplateID:  "test-template",
		VulnURL:     "https://example.com",
		Severity:    "high",
		Protocol:    "http",
		Description: "desc",
		Reference:   "ref",
	}
	parentFingerprint := database.FingerprintInfo{
		TaskID:       parentTaskID,
		Version:      1,
		URL:          "https://combo.example.com",
		Title:        "combo",
		Status:       200,
		Fingerprints: "[\"nginx\"]",
	}

	for _, value := range []interface{}{&parentTask, &childTask, &childVuln, &parentFingerprint} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed test data failed: %v", err)
		}
	}

	children, err := loadCombinationChildren(parentTaskID, 1)
	if err != nil {
		t.Fatalf("loadCombinationChildren() error = %v", err)
	}
	if len(children) != 1 || children[0].ID != childTaskID {
		t.Fatalf("loadCombinationChildren() = %#v, want child %q", children, childTaskID)
	}

	items, err := loadMergedTaskData([]MergeTaskRef{{ID: parentTaskID, Version: 1}})
	if err != nil {
		t.Fatalf("loadMergedTaskData() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if len(items[0].Children) != 1 || items[0].Children[0].ID != childTaskID {
		t.Fatalf("children = %#v, want child %q", items[0].Children, childTaskID)
	}
	if len(items[0].Vulnerabilities) != 1 || items[0].Vulnerabilities[0].TaskID != childTaskID {
		t.Fatalf("vulnerabilities = %#v, want child task results", items[0].Vulnerabilities)
	}
	if len(items[0].Fingerprints) != 1 || items[0].Fingerprints[0].TaskID != parentTaskID {
		t.Fatalf("fingerprints = %#v, want parent task direct results", items[0].Fingerprints)
	}
}
