package scan

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRecentVulnerabilitiesMergesSourcesBeforeLimit(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "recent.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.VulnerabilityInfo{}, &database.VulnScanJSVulnerability{}); err != nil {
		t.Fatal(err)
	}
	store := pentest.NewSQLStore(sqlDB)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

	items, err := loadRecentVulnerabilities(ctx, db, store, 10)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("empty list: %v, %v", items, err)
	}
	for _, task := range []database.VulnScanTask{
		{ID: "scan", Version: 1, CreatedAt: base.Add(20 * time.Hour)}, // Must be excluded even with a newer timestamp.
		{ID: "scan", Version: 2, CreatedAt: base.Add(2 * time.Hour)},
	} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, vuln := range []database.VulnerabilityInfo{
		{TaskID: "scan", Version: 1, TemplateID: "obsolete", VulnName: "obsolete", Severity: "HIGH"},
		{TaskID: "scan", Version: 2, TemplateID: "current", VulnName: "scan finding", VulnURL: "https://scan.example", Severity: "HIGH"},
	} {
		if err := db.Create(&vuln).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, vuln := range []database.VulnScanJSVulnerability{
		{ID: "js-old", TaskID: "scan", Version: 1, Title: "obsolete JS", Level: "high", CreatedAt: base.Add(20 * time.Hour)},
		{ID: "shared-id", TaskID: "scan", Version: 2, Title: "JS finding", Level: "medium", AIVerified: true, CreatedAt: base.Add(3 * time.Hour)},
	} {
		if err := db.Create(&vuln).Error; err != nil {
			t.Fatal(err)
		}
	}
	items, err = loadRecentVulnerabilities(ctx, db, store, 10)
	if err != nil || len(items) != 2 {
		t.Fatalf("scan-only list: %v, %v", items, err)
	}

	for _, finding := range []pentest.Finding{
		{ID: "shared-id", ProjectID: "a", ConversationID: "session-a", Title: "AI newest", Target: "https://ai.example/new", Severity: "high", CreatedAt: base.Add(4 * time.Hour)},
		{ID: "ai-middle", ProjectID: "b", ConversationID: "session-b", Title: "AI middle", Target: "https://ai.example/middle", Severity: "low", CreatedAt: base.Add(150 * time.Minute)},
		{ID: "ai-oldest", ProjectID: "a", ConversationID: "session-a", Title: "AI oldest", Target: "https://ai.example/old", Severity: "info", CreatedAt: base},
	} {
		if _, err := store.RecordFinding(ctx, finding); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		limit  int
		titles []string
	}{
		{1, []string{"AI newest"}},
		{3, []string{"AI newest", "JS finding", "AI middle"}},
		{10, []string{"AI newest", "JS finding", "AI middle", "scan finding", "AI oldest"}},
	} {
		items, err := loadRecentVulnerabilities(ctx, db, store, tc.limit)
		if err != nil {
			t.Fatal(err)
		}
		titles := make([]string, 0, len(items))
		for _, item := range items {
			titles = append(titles, item.VulnName)
		}
		if !reflect.DeepEqual(titles, tc.titles) {
			t.Fatalf("limit %d: got %v, want %v", tc.limit, titles, tc.titles)
		}
		ai := items[0]
		if ai.ID != "shared-id" || ai.TaskID != "session-a" || ai.SourceType != "pentest_blackboard" || ai.AIVerified || ai.Target != "https://ai.example/new" || ai.Severity != "high" {
			t.Fatalf("AI provenance/detail data: %+v", ai)
		}
		if len(items) > 1 && (items[1].SourceType != "jsscan" || !items[1].AIVerified) {
			t.Fatalf("JS provenance changed: %+v", items[1])
		}
	}
	if err := db.Exec("DELETE FROM vul_scan_task").Error; err != nil {
		t.Fatal(err)
	}
	items, err = loadRecentVulnerabilities(ctx, db, store, 10)
	if err != nil || len(items) != 3 {
		t.Fatalf("AI-only list: %v, %v", items, err)
	}
	if err := db.Exec("DROP TABLE pentest_findings").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := loadRecentVulnerabilities(ctx, db, store, 10); err == nil {
		t.Fatal("AI query failure must not be reported as successful partial data")
	}
}
