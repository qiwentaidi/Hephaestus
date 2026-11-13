package scan

import (
	"testing"

	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/pkg/database"
)

func TestCombinedTaskListIncludesAIOrigin(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	for _, task := range []database.VulnScanTask{
		{ID: "ai", Version: 1, TaskName: "AI scan", ScanType: "jsscan", CreatedByAI: true},
		{ID: "user", Version: 1, TaskName: "User scan", ScanType: "webscan"},
	} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	listSQL, args, _, _ := buildCombinedTasksQueries(nil, nil, "", 20, 0)
	var rows []combinedTaskRow
	if err := db.Raw(listSQL, args...).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("task count = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if row.CreatedByAI != (row.ID == "ai") {
			t.Fatalf("task %s createdByAI = %v", row.ID, row.CreatedByAI)
		}
	}
}

func TestCombinedTaskListFiltersMultipleStatuses(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	for _, task := range []database.VulnScanTask{
		{ID: "pending", Version: 1, TaskName: "Pending", ScanType: "webscan", Status: "pending"},
		{ID: "running", Version: 1, TaskName: "Running", ScanType: "webscan", Status: "running"},
		{ID: "completed", Version: 1, TaskName: "Completed", ScanType: "webscan", Status: "completed"},
	} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	listSQL, args, _, _ := buildCombinedTasksQueries(nil, []string{"pending", "completed"}, "", 20, 0)
	var rows []combinedTaskRow
	if err := db.Raw(listSQL, args...).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("task count = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if row.Status != "pending" && row.Status != "completed" {
			t.Fatalf("unexpected status %q", row.Status)
		}
	}
}

func TestCombinedTaskListFiltersMultipleScanTypes(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	for _, task := range []database.VulnScanTask{
		{ID: "webscan", Version: 1, TaskName: "Web", ScanType: "webscan", Status: "pending"},
		{ID: "portscan", Version: 1, TaskName: "Port", ScanType: "portscan", Status: "pending"},
		{ID: "combination", Version: 1, TaskName: "Combo", ScanType: "combination", Status: "pending"},
	} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	listSQL, args, _, _ := buildCombinedTasksQueries([]string{"webscan", "combination"}, nil, "", 20, 0)
	var rows []combinedTaskRow
	if err := db.Raw(listSQL, args...).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("task count = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if row.ScanType != "webscan" && row.ScanType != "combination" {
			t.Fatalf("unexpected scan type %q", row.ScanType)
		}
	}
}
