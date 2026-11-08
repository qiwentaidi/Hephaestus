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
	listSQL, args, _, _ := buildCombinedTasksQueries("", "", "", 20, 0)
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
