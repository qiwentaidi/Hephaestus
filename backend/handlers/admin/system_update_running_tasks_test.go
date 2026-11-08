package admin

import (
	"testing"

	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/pkg/database"
)

func TestCollectRunningUpdateTasksReportsInterruptibleWork(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	if err := db.AutoMigrate(
		&database.CompanyInfoTask{},
		&database.APIDocTestTask{},
		&database.SpaceExportTask{},
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE pentest_conversations (id TEXT PRIMARY KEY, status TEXT)`).Error; err != nil {
		t.Fatal(err)
	}

	scanTasks := []database.VulnScanTask{
		{ID: "web-1", Version: 1, ScanType: "webscan", Status: "running"},
		{ID: "web-2", Version: 1, ScanType: "webscan", Status: "recovering"},
		{ID: "web-3", Version: 1, ScanType: "webscan", Status: "completed"},
		{ID: "dir-1", Version: 1, ScanType: "dirsearch", Status: "running"},
		{ID: "combo-1", Version: 1, ScanType: "combination", Status: "running"},
	}
	if err := db.Create(&scanTasks).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.CompanyInfoTask{ID: "company-1", Status: "running", Config: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.APIDocTestTask{ID: "apidoc-1", TaskName: "doc", Status: "running"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.SpaceExportTask{ID: "space-1", Engine: "fofa", Status: "running"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, conversation := range []struct{ id, status string }{
		{"conversation-1", "running"},
		{"conversation-2", "waiting_tool"},
		{"conversation-3", "completed"},
	} {
		if err := db.Exec(
			`INSERT INTO pentest_conversations (id, status) VALUES (?, ?)`,
			conversation.id, conversation.status,
		).Error; err != nil {
			t.Fatal(err)
		}
	}

	summary := collectRunningUpdateTasks(db)
	if summary.Total != 9 {
		t.Fatalf("unexpected running task total: %d", summary.Total)
	}

	wantKeys := []string{
		"webscan", "dirsearch", "combination", "company_info",
		"api_doc_test", "space_export", "ai_dialogue",
	}
	wantCounts := []int64{2, 1, 1, 1, 1, 1, 2}
	if len(summary.Groups) != len(wantKeys) {
		t.Fatalf("unexpected group count: %#v", summary.Groups)
	}
	for index, group := range summary.Groups {
		if group.Key != wantKeys[index] || group.Count != wantCounts[index] {
			t.Fatalf("unexpected group at %d: %#v", index, group)
		}
	}
}

func TestCollectRunningUpdateTasksIgnoresFinishedWork(t *testing.T) {
	db := testfixture.SQLiteDB(t)

	if err := db.Create(&database.VulnScanTask{
		ID: "done-1", Version: 1, ScanType: "webscan", Status: "completed",
	}).Error; err != nil {
		t.Fatal(err)
	}

	summary := collectRunningUpdateTasks(db)
	if summary.Total != 0 {
		t.Fatalf("finished tasks counted as running: %#v", summary)
	}
	if len(summary.Groups) != 0 {
		t.Fatalf("expected no groups for finished work: %#v", summary.Groups)
	}
}
