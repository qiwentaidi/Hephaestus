package subdomain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/taskstate"
	"hephaestus-backend/pkg/types"
)

func TestDNSResolutionErrorsSurviveStorageAndResultRead(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "dns-errors.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	checkDNSResolutionErrorStorage(t, db)
}

func TestDNSResolutionErrorsPostgres(t *testing.T) {
	dsn := os.Getenv("SUBDOMAIN_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires an isolated PostgreSQL database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	checkDNSResolutionErrorStorage(t, db)
}

func checkDNSResolutionErrorStorage(t *testing.T, db *gorm.DB) {
	t.Helper()
	// Start with the pre-change schema to verify additive migration, too.
	if err := db.Exec(`CREATE TABLE subdomain_scan_result (id varchar(255) PRIMARY KEY, task_id varchar(255), domain varchar(255), subdomain varchar(255), ips text NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.SubdomainScanResult{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropColumn(&database.SubdomainScanResult{}, "ResolutionError"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := database.AutoMigrate(db); err != nil {
			t.Fatal(err)
		}
	}
	result := SubdomainResult{
		Domain: "example.test", Subdomain: "failed.example.test", IPs: []string{}, Source: "Chaos",
		ResolutionError: "DNS failed.example.test via 127.0.0.1:5353: i/o timeout; 127.0.0.2:5353: SERVFAIL",
	}
	if err := saveScanResultWithDB(db, "single", result); err != nil {
		t.Fatal(err)
	}
	if err := batchSaveScanResultsWithDB(db, "batch", []SubdomainResult{result}); err != nil {
		t.Fatal(err)
	}
	for _, task := range []string{"single", "batch"} {
		rows, total, err := getScanResultsWithDB(db, task, 10, 0, "", "", "")
		if err != nil || total != 1 || len(rows) != 1 {
			t.Fatalf("failed reading %s: rows=%v total=%d err=%v", task, rows, total, err)
		}
		if rows[0].ResolutionError != result.ResolutionError || len(rows[0].IPs) != 0 {
			t.Fatalf("DNS failure lost its context after persistence: %#v", rows[0])
		}
		encoded, err := json.Marshal(rows[0])
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(encoded, &payload); err != nil || payload["resolutionError"] != result.ResolutionError {
			t.Fatalf("DNS error missing from API result: %s, err=%v", encoded, err)
		}
	}
}

func TestScanResultSourceStatsUsesWholeTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "source-stats.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.SubdomainScanResult{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 208; i++ {
		result := database.SubdomainScanResult{
			ID: fmt.Sprintf("result-%03d", i), TaskID: "task", Domain: "example.test",
			Subdomain: fmt.Sprintf("host-%03d.example.test", i), IPs: `[]`,
			Source: "FOFA", Sources: `["FOFA"," Hunter ","FOFA"]`, IsWildcard: i%2 == 0,
		}
		if i < 50 {
			result.Source, result.Sources = "IP138", `["IP138","Chaos"]`
		}
		if err := db.Create(&result).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, result := range []database.SubdomainScanResult{
		{ID: "other-1", TaskID: "other", Source: "OtherSource", IPs: `[]`},
		{ID: "legacy-1", TaskID: "legacy", Source: "FOFA", IPs: `[]`},
		{ID: "legacy-2", TaskID: "legacy", Source: " Hunter ", Sources: `broken-json`, IPs: `[]`},
		{ID: "legacy-3", TaskID: "legacy", Source: "Chaos", Sources: `["", " "]`, IPs: `[]`},
		{ID: "legacy-4", TaskID: "legacy", Source: "", Sources: `[]`, IPs: `[]`},
	} {
		if err := db.Create(&result).Error; err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]int64{"IP138": 50, "Chaos": 50, "FOFA": 158, "Hunter": 158}
	for _, tc := range []struct {
		name, search, source, wildcard string
		offset, wantRows               int
		wantTotal                      int64
	}{
		{name: "first page", wantRows: 50, wantTotal: 208},
		{name: "second page", offset: 50, wantRows: 50, wantTotal: 208},
		{name: "source filter", source: "FOFA", wantRows: 50, wantTotal: 158},
		{name: "wildcard filter", wildcard: "only", wantRows: 50, wantTotal: 104},
		{name: "search filter", search: "host-200", wantRows: 1, wantTotal: 1},
		{name: "empty results", search: "missing", wantRows: 0, wantTotal: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, total, err := getScanResultsWithDB(db, "task", 50, tc.offset, tc.search, tc.source, tc.wildcard)
			if err != nil || total != tc.wantTotal || len(results) != tc.wantRows {
				t.Fatalf("unexpected page: rows=%d total=%d err=%v", len(results), total, err)
			}
			stats, err := getScanResultSourceStatsWithDB(db, "task")
			if err != nil || !reflect.DeepEqual(stats, want) {
				t.Fatalf("source statistics must cover all 208 results: got=%v err=%v", stats, err)
			}
		})
	}
	legacyStats, err := getScanResultSourceStatsWithDB(db, "legacy")
	if err != nil || !reflect.DeepEqual(legacyStats, map[string]int64{"FOFA": 1, "Hunter": 1, "Chaos": 1}) {
		t.Fatalf("legacy source fallback failed: stats=%v err=%v", legacyStats, err)
	}
	emptyStats, err := getScanResultSourceStatsWithDB(db, "missing-task")
	if err != nil || emptyStats == nil || len(emptyStats) != 0 {
		t.Fatalf("expected an empty map for a task without results: stats=%v err=%v", emptyStats, err)
	}
}

func TestScanResultsWildcardFilterBeforePagination(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "wildcard-results.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.SubdomainScanResult{}); err != nil {
		t.Fatal(err)
	}
	for i, result := range []database.SubdomainScanResult{
		{ID: "normal-1", TaskID: "task", Domain: "example.test", Subdomain: "api.example.test", IPs: `[]`, Source: "FOFA"},
		{ID: "wildcard-1", TaskID: "task", Domain: "example.test", Subdomain: "api-wild.example.test", IPs: `[]`, Source: "Hunter", Sources: `["Hunter","FOFA"]`, IsWildcard: true},
		{ID: "normal-2", TaskID: "task", Domain: "example.test", Subdomain: "www.example.test", IPs: `[]`, Source: "Hunter"},
		{ID: "wildcard-2", TaskID: "task", Domain: "example.test", Subdomain: "wild.example.test", IPs: `[]`, Source: "FOFA", IsWildcard: true},
		{ID: "other-task", TaskID: "other", Domain: "example.test", Subdomain: "api-other.example.test", IPs: `[]`, Source: "FOFA", IsWildcard: true},
	} {
		result.CreatedAt = time.Unix(int64(i), 0)
		if err := db.Create(&result).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, filter, search, source, wantSubdomain string
		offset                                      int
		wantTotal                                   int64
	}{
		{name: "default", wantTotal: 4, wantSubdomain: "wild.example.test"},
		{name: "all", filter: "all", wantTotal: 4, wantSubdomain: "wild.example.test"},
		{name: "exclude first page", filter: "exclude", wantTotal: 2, wantSubdomain: "www.example.test"},
		{name: "exclude second page", filter: "exclude", offset: 1, wantTotal: 2, wantSubdomain: "api.example.test"},
		{name: "only first page", filter: "only", wantTotal: 2, wantSubdomain: "wild.example.test"},
		{name: "only second page", filter: "only", offset: 1, wantTotal: 2, wantSubdomain: "api-wild.example.test"},
		{name: "combined search and secondary source", filter: "only", search: "api", source: "FOFA", wantTotal: 1, wantSubdomain: "api-wild.example.test"},
		{name: "no matches", filter: "exclude", search: "wild", wantTotal: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, total, err := getScanResultsWithDB(db, "task", 1, tc.offset, tc.search, tc.source, tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			if total != tc.wantTotal {
				t.Fatalf("total = %d, want %d", total, tc.wantTotal)
			}
			if tc.wantTotal == 0 {
				if len(results) != 0 {
					t.Fatalf("expected no results, got %#v", results)
				}
				return
			}
			if len(results) != 1 || results[0].Subdomain != tc.wantSubdomain {
				t.Fatalf("unexpected page: %#v", results)
			}
			if tc.filter == "only" && !results[0].IsWildcard || tc.filter == "exclude" && results[0].IsWildcard {
				t.Fatalf("incorrect wildcard flag: %#v", results[0])
			}
		})
	}
}

func TestUnifiedTaskProgressDoesNotCreateLegacyTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "subdomain.db")+"?_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.SubdomainScanResult{}); err != nil {
		t.Fatal(err)
	}
	task := database.VulnScanTask{ID: "sub-task", Version: 2, ScanType: "subdomain", Status: "running", Progress: 10}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.SubdomainScanResult{ID: "stale", TaskID: task.ID, Domain: "example.test", Subdomain: "old.example.test", IPs: `[]`}).Error; err != nil {
		t.Fatal(err)
	}
	config := types.SubdomainOption{Domains: []string{"example.test", "example.org"}}
	if err := prepareScanTaskWithDB(db, task.ID, config); err != nil {
		t.Fatal(err)
	}
	var resultCount int64
	if err := db.Model(&database.SubdomainScanResult{}).Where("task_id = ?", task.ID).Count(&resultCount).Error; err != nil {
		t.Fatal(err)
	}
	if resultCount != 0 {
		t.Fatalf("stale results were not cleared: %d", resultCount)
	}
	detail := &ScanProgressDetail{TotalDomains: 2, CompletedDomains: 1, OverallProgress: 50}
	if err := updateTaskProgressWithDetailDB(db, task.ID, 50, 7, 14, 3, detail); err != nil {
		t.Fatal(err)
	}
	var updated database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", task.ID, task.Version).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Progress != 50 || updated.CurrentCount != 7 || updated.TotalCount != 14 || updated.ProgressDetail == "" {
		t.Fatalf("unified progress was not updated: %#v", updated)
	}
	if db.Migrator().HasTable("subdomain_scan_task") {
		t.Fatal("runtime progress path recreated legacy subdomain task table")
	}
}

func TestCombinationSubdomainStepPreservesParentProgress(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "combination.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.SubdomainScanResult{}); err != nil {
		t.Fatal(err)
	}
	task := database.VulnScanTask{
		ID: "combination-task", Version: 1, ScanType: "combination",
		StepModule: "subdomain", Status: "running", Progress: 10,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := prepareScanTaskWithDB(db, task.ID, types.SubdomainOption{Domains: []string{"example.test"}}); err != nil {
		t.Fatal(err)
	}
	if err := updateTaskProgressWithDetailDB(db, task.ID, 50, 1, 2, 1,
		&ScanProgressDetail{TotalDomains: 1, OverallProgress: 50}); err != nil {
		t.Fatal(err)
	}
	var updated database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", task.ID, task.Version).Take(&updated).Error; err != nil {
		t.Fatal(err)
	}
	checkpoint := taskstate.ScanCheckpoint(updated, "subdomain", true)
	if updated.ScanType != "combination" || updated.Progress != 10 || updated.CurrentCount != 0 || checkpoint.Progress != 50 || checkpoint.Meta["currentCount"] != float64(1) {
		t.Fatalf("subdomain progress must be isolated from the parent: %#v checkpoint=%#v", updated, checkpoint)
	}

	if err := db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ?", task.ID, task.Version).
		Update("step_module", "portscan").Error; err != nil {
		t.Fatal(err)
	}
	if err := updateTaskProgressWithDetailDB(db, task.ID, 100, 2, 2, 1, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("id = ? AND version = ?", task.ID, task.Version).Take(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Progress != 10 || updated.StepModule != "portscan" || taskstate.ScanCheckpoint(updated, "subdomain", true).Progress != 100 {
		t.Fatalf("subdomain completion changed the parent's current module or overall progress: %#v", updated)
	}
}
