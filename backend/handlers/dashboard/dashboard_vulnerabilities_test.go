package dashboard

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/vulnerabilitycenter"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDashboardVulnerabilityStatsUsesSameSourcesForBothBreakdowns(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "stats.db")), &gorm.Config{})
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
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -7)
	rows := []struct{ severity, aiStatus, scanStatus string }{
		{" CRITICAL ", "open", "open"},
		{"HIGH", "confirmed", "in_progress"},
		{"medium", "fixed", "fixed"},
		{"low", "false_positive", "false_positive"},
		{"info", "accepted_risk", "ignored"},
		{"information", "", ""},
		{"unknown", "legacy", "legacy"},
		{"high", "in_progress", "in_progress"},
		{"low", "ignored", "ignored"},
	}
	check := func(t *testing.T, sources int64, since *time.Time, extraOld bool) {
		t.Helper()
		stats, err := LoadDashboardVulnerabilityStats(ctx, db, since)
		if err != nil {
			t.Fatal(err)
		}
		wantSeverity := map[string]int64{"critical": sources, "high": 2 * sources, "medium": sources, "low": 3 * sources, "info": 2 * sources}
		wantDisposal := VulnDisposalStatsResponse{Open: 3 * sources, InProgress: 2 * sources, Fixed: sources, FalsePositive: sources, Ignored: 2 * sources}
		if extraOld {
			wantSeverity["high"]++
			wantDisposal.Open++
		}
		if sources == 3 {
			// Both retained historical scan versions remain visible in the center.
			wantSeverity["critical"] += 4
			wantDisposal.Fixed += 4
		}
		if !reflect.DeepEqual(stats.Severity, wantSeverity) || stats.Disposal != wantDisposal {
			t.Fatalf("got %+v, want severity=%v disposal=%+v", stats, wantSeverity, wantDisposal)
		}
		var severityTotal int64
		for _, count := range stats.Severity {
			severityTotal += count
		}
		d := stats.Disposal
		if severityTotal != d.Open+d.InProgress+d.Fixed+d.FalsePositive+d.Ignored {
			t.Fatalf("breakdown totals differ: %+v", stats)
		}
		if since == nil {
			assertCount := func(filter vulnerabilitycenter.Filter, want int64) {
				t.Helper()
				result, err := vulnerabilitycenter.Query(ctx, db, filter)
				if err != nil || result.Total != want {
					t.Fatalf("dashboard/center mismatch for %+v: got %d want %d, err=%v", filter, result.Total, want, err)
				}
			}
			assertCount(vulnerabilitycenter.Filter{}, severityTotal)
			for severity, count := range stats.Severity {
				assertCount(vulnerabilitycenter.Filter{Severities: []string{severity}}, count)
			}
			for status, count := range map[string]int64{"open": d.Open, "in_progress": d.InProgress, "fixed": d.Fixed, "false_positive": d.FalsePositive, "ignored": d.Ignored} {
				assertCount(vulnerabilitycenter.Filter{Statuses: []string{status}}, count)
			}
		}
	}
	t.Run("empty", func(t *testing.T) { check(t, 0, nil, false) })
	for i, row := range rows {
		_, err := store.RecordFinding(ctx, pentest.Finding{ID: fmt.Sprintf("ai-%d", i), ProjectID: "project", ConversationID: "session", Title: fmt.Sprintf("finding %d", i), Severity: row.severity, Status: row.aiStatus, CreatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
	}
	// RecordFinding normalizes unknown severities to info; simulate a legacy
	// stored value to exercise the dashboard's shared fallback for all sources.
	if err := db.Exec("UPDATE pentest_findings SET severity = ? WHERE id = ?", "unknown", "ai-6").Error; err != nil {
		t.Fatal(err)
	}
	t.Run("AI without scans", func(t *testing.T) { check(t, 1, nil, false) })

	for _, task := range []database.VulnScanTask{
		{ID: "scan", Version: 1, CreatedAt: now},
		{ID: "scan", Version: 2, CreatedAt: now},
		// Date filtering considers each retained version's own creation time.
		{ID: "out-of-window", Version: 1, CreatedAt: now},
		{ID: "out-of-window", Version: 2, CreatedAt: now.AddDate(0, 0, -30)},
	} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i, row := range rows {
		v := database.VulnerabilityInfo{TaskID: "scan", Version: 2, TemplateID: fmt.Sprintf("v-%d", i), Severity: row.severity, DisposalStatus: row.scanStatus}
		if err := db.Create(&v).Error; err != nil {
			t.Fatal(err)
		}
		js := database.VulnScanJSVulnerability{ID: fmt.Sprintf("js-%d", i), TaskID: "scan", Version: 2, Title: "JS finding", Level: row.severity, DisposalStatus: row.scanStatus, CreatedAt: now}
		if err := db.Create(&js).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Nullable legacy disposition values must still contribute to pending.
	if err := db.Model(&database.VulnerabilityInfo{}).Where("template_id = ?", "v-5").Update("disposal_status", nil).Error; err != nil {
		t.Fatal(err)
	}
	for _, taskID := range []string{"scan", "out-of-window", "orphan"} {
		v := database.VulnerabilityInfo{TaskID: taskID, Version: 1, TemplateID: "obsolete", Severity: "critical", DisposalStatus: "fixed"}
		if err := db.Create(&v).Error; err != nil {
			t.Fatal(err)
		}
		js := database.VulnScanJSVulnerability{ID: taskID + "-old-js", TaskID: taskID, Version: 1, Title: "obsolete", Level: "critical", DisposalStatus: "fixed", CreatedAt: now}
		if err := db.Create(&js).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Superseded AI findings must not inflate statistics or linked results.
	if _, err := store.RecordFinding(ctx, pentest.Finding{ID: "superseded-ai", ProjectID: "project", Title: "replaced", Severity: "critical", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE pentest_findings SET superseded_at = ? WHERE id = ?", now, "superseded-ai").Error; err != nil {
		t.Fatal(err)
	}
	t.Run("all sources and retained versions", func(t *testing.T) { check(t, 3, nil, false) })
	if _, err := store.RecordFinding(ctx, pentest.Finding{ID: "old-ai", ProjectID: "other-project", Title: "old finding updated today", Severity: "high", Status: "open", CreatedAt: now.AddDate(0, 0, -30), UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	t.Run("all time", func(t *testing.T) { check(t, 3, nil, true) })
	t.Run("date filter applies to AI and retained scans", func(t *testing.T) { check(t, 3, &start, false) })
	if err := db.Exec("DROP TABLE pentest_findings").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDashboardVulnerabilityStats(ctx, db, nil); err == nil {
		t.Fatal("query failures must not return successful zero counts")
	}
}
