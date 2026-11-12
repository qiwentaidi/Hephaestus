package admin

import (
	"os"
	"path/filepath"
	"testing"

	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/pkg/database"
)

func TestValidDataPurgeConfirmation(t *testing.T) {
	for _, value := range []string{"清空数据", " 清空数据 ", "CLEAR DATA", "clear data", "Clear Data"} {
		if !validDataPurgeConfirmation(value) {
			t.Fatalf("%q should be accepted", value)
		}
	}
	for _, value := range []string{"", "清空", "CLEAR", "delete all", "清空数据!"} {
		if validDataPurgeConfirmation(value) {
			t.Fatalf("%q should be rejected", value)
		}
	}
}

func TestCollectActivePurgeTasksReportsRunningWork(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	if err := db.AutoMigrate(
		&database.CompanyInfoTask{},
		&database.APIDocTestTask{},
		&database.SpaceExportTask{},
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE pentest_worker_turns (id TEXT PRIMARY KEY, status TEXT NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}

	scanTasks := []database.VulnScanTask{
		{ID: "scan-pending", Version: 1, ScanType: "webscan", Status: "pending"},
		{ID: "scan-running", Version: 1, ScanType: "portscan", Status: "running"},
		{ID: "scan-paused", Version: 1, ScanType: "dirsearch", Status: "paused"},
		{ID: "scan-done", Version: 1, ScanType: "jsscan", Status: "completed"},
	}
	for index := range scanTasks {
		if err := db.Create(&scanTasks[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&database.CombinationTask{
		ID: "combo-running", Name: "combo", Targets: "[]", Nodes: "[]", Status: "running",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.CombinationTask{
		ID: "combo-paused", Name: "paused combo", Targets: "[]", Nodes: "[]", Status: "paused",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.CompanyInfoTask{ID: "company", Status: "running", Config: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.APIDocTestTask{
		ID: "api-doc", TaskName: "doc", SourceURL: "https://example.test/openapi.json", Status: "pending",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.SpaceExportTask{ID: "space-export", Engine: "fofa", Status: "running"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"running", "waiting", "queued", "completed"} {
		if err := db.Exec(`INSERT INTO pentest_worker_turns (id, status) VALUES (?, ?)`, "turn-"+status, status).Error; err != nil {
			t.Fatal(err)
		}
	}

	groups, err := collectActivePurgeTasks(db)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{
		"scan": 2, "combination": 1, "company_info": 1,
		"api_doc_test": 1, "space_export": 1, "ai_worker": 3,
	}
	if len(groups) != len(want) {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	wantOrder := []string{"scan", "combination", "company_info", "api_doc_test", "space_export", "ai_worker"}
	for index, group := range groups {
		if group.Kind != wantOrder[index] {
			t.Fatalf("group %d = %q, want %q", index, group.Kind, wantOrder[index])
		}
		if group.Count != want[group.Kind] {
			t.Fatalf("group %q count = %d, want %d", group.Kind, group.Count, want[group.Kind])
		}
	}
}

func TestCollectActivePurgeTasksIgnoresTerminalWork(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	if err := db.Create(&database.VulnScanTask{
		ID: "scan-paused", Version: 1, ScanType: "webscan", Status: "paused",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.CombinationTask{
		ID: "combo-paused", Name: "combo", Targets: "[]", Nodes: "[]", Status: "paused",
	}).Error; err != nil {
		t.Fatal(err)
	}

	groups, err := collectActivePurgeTasks(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("terminal work reported as active: %#v", groups)
	}
}

func TestStagePurgeDirectoriesRestoresOnDemand(t *testing.T) {
	dataDir := t.TempDir()
	mustWrite(t, filepath.Join(dataDir, "ai-pentest", "sessions", "one", "chat.json"), "chat")
	mustWrite(t, filepath.Join(dataDir, "screenshot", "one.png"), "png")
	mustWrite(t, filepath.Join(dataDir, "ipdb", "geolite.mmdb"), "keep")
	mustWrite(t, filepath.Join(dataDir, "machine-identity.json"), "keep")

	stage, err := stagePurgeDirectories(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "ai-pentest")); !os.IsNotExist(err) {
		t.Fatalf("ai-pentest was not moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "ipdb", "geolite.mmdb")); err != nil {
		t.Fatalf("preserved directory was moved: %v", err)
	}
	if errs := stage.Restore(); len(errs) > 0 {
		t.Fatal(errs)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "ai-pentest", "sessions", "one", "chat.json")); err != nil {
		t.Fatalf("restored directory is missing: %v", err)
	}

	stage, err = stagePurgeDirectories(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if errs := stage.Discard(); len(errs) > 0 {
		t.Fatal(errs)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "ai-pentest")); !os.IsNotExist(err) {
		t.Fatalf("discarded directory still exists: %v", err)
	}
	if got := stage.DiscardedCount(); got != 2 {
		t.Fatalf("discarded directory count = %d, want 2", got)
	}
}

func TestClearRuntimeLogsTruncatesFilesAndSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "2026-01-01.log"), "old log")
	mustWrite(t, filepath.Join(dir, "2026-01-02.log"), "current log")
	if err := os.Mkdir(filepath.Join(dir, "archive"), 0755); err != nil {
		t.Fatal(err)
	}

	cleared, warnings := clearRuntimeLogsIn(dir)
	if !cleared || len(warnings) != 0 {
		t.Fatalf("unexpected result: cleared=%v warnings=%v", cleared, warnings)
	}
	for _, name := range []string{"2026-01-01.log", "2026-01-02.log"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() != 0 {
			t.Fatalf("%s was not truncated: %d bytes", name, info.Size())
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "archive")); err != nil || !info.IsDir() {
		t.Fatalf("archive directory was removed: %v", err)
	}
}

func TestCleanupStalePurgeStagingRemovesOnlyOwnDirectories(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dataDir, ".purge-staging-123"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dataDir, "exports"), 0755); err != nil {
		t.Fatal(err)
	}
	cleanupStalePurgeStaging(dataDir)
	if _, err := os.Stat(filepath.Join(dataDir, ".purge-staging-123")); !os.IsNotExist(err) {
		t.Fatalf("stale staging directory still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "exports")); err != nil {
		t.Fatalf("unrelated directory was removed: %v", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
