package taskstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func combinationCheckpointTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	var db *gorm.DB
	var err error
	if dsn := os.Getenv("HEPHAESTUS_TEST_POSTGRES_DSN"); dsn != "" {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		schema := fmt.Sprintf("checkpoint_test_%d", time.Now().UnixNano())
		if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Exec("DROP SCHEMA " + schema + " CASCADE") })
		db, err = gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	} else {
		db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "checkpoint.db")+"?_busy_timeout=5000&_journal_mode=WAL&_txlock=immediate"), &gorm.Config{})
		if err == nil {
			sqlDB, _ := db.DB()
			sqlDB.SetMaxOpenConns(8)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCombinationCheckpointsPreserveEachModuleAndTimeline(t *testing.T) {
	db := combinationCheckpointTestDB(t)
	task := database.VulnScanTask{ID: "parent", Version: 1, ScanType: "combination", Status: StatusRunning, Progress: 33}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	events := []map[string]interface{}{{"stage": "active", "time": time.Now().Format(time.RFC3339Nano), "label": "主动目录探测"}}
	for _, module := range []string{"webscan", "jsscan", "dirscan", "portscan", "crack", "subdomain"} {
		meta := map[string]interface{}{"processedTargets": 3}
		if module == "webscan" {
			meta["stageEvents"] = events
		}
		if err := SaveCombinationCheckpoint(db, task.ID, task.Version, module, "completed", 100, meta); err != nil {
			t.Fatal(err)
		}
	}
	var got database.VulnScanTask
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Progress != 33 || got.Status != StatusRunning || got.CurrentStage != "combination" {
		t.Fatalf("module completion changed parent progress/lifecycle: %#v", got)
	}
	for _, module := range []string{"webscan", "jsscan", "dirscan", "portscan", "crack", "subdomain"} {
		checkpoint := ScanCheckpoint(got, module, true)
		if checkpoint.Progress != 100 || checkpoint.Stage != "completed" || checkpoint.Meta["processedTargets"] != float64(3) {
			t.Fatalf("module %s lost its checkpoint: %#v", module, checkpoint)
		}
	}
	if len(checkpointEvents(ParseCheckpoint(got.CheckpointJSON).Meta["stageEvents"])) != 1 {
		t.Fatal("later modules erased the fingerprint timeline")
	}
	if err := MarkCompleted(db, task.ID, task.Version); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Progress != 100 || ScanCheckpoint(got, "webscan", true).Progress != 100 || len(checkpointEvents(ParseCheckpoint(got.CheckpointJSON).Meta["stageEvents"])) != 1 {
		t.Fatal("final parent completion erased its module checkpoints")
	}
}

func TestCombinationCheckpointConcurrentWritesDoNotLoseModules(t *testing.T) {
	db := combinationCheckpointTestDB(t)
	task := database.VulnScanTask{ID: "parallel", Version: 1, ScanType: "combination", Status: StatusRunning, Progress: 50}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 6)
	for _, module := range []string{"webscan", "jsscan", "dirscan", "portscan", "crack", "subdomain"} {
		wg.Go(func() {
			for progress := 10; progress <= 100; progress += 10 {
				if err := SaveCombinationCheckpoint(db, task.ID, task.Version, module, module+":scanning", progress, map[string]interface{}{"processed": progress}); err != nil {
					errors <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var got database.VulnScanTask
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	modules := moduleCheckpoints(ParseCheckpoint(got.CheckpointJSON))
	if len(modules) != 6 || got.Progress != 50 {
		t.Fatalf("lost parallel updates: parent=%d modules=%#v", got.Progress, modules)
	}
	for module, checkpoint := range modules {
		if checkpoint.Progress != 100 || checkpoint.Meta["processed"] != float64(100) {
			t.Fatalf("module %s lost progress: %#v", module, checkpoint)
		}
	}
}

func TestCombinationCheckpointIgnoresStaleProgressButKeepsEvents(t *testing.T) {
	db := combinationCheckpointTestDB(t)
	task := database.VulnScanTask{ID: "stale", Version: 1, Status: StatusRunning, Progress: 66}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := SaveCombinationCheckpoint(db, task.ID, 1, "webscan", "webscan:fingerprint", 40, map[string]interface{}{"processed": 9}); err != nil {
		t.Fatal(err)
	}
	if err := SaveCombinationCheckpoint(db, task.ID, 1, "webscan", "webscan:probe", 8, map[string]interface{}{
		"stageEvents": []map[string]interface{}{{"stage": "active", "time": time.Now(), "label": "主动目录探测"}},
	}); err != nil {
		t.Fatal(err)
	}
	var got database.VulnScanTask
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	checkpoint := ScanCheckpoint(got, "webscan", true)
	if checkpoint.Progress != 40 || checkpoint.Stage != "webscan:fingerprint" || checkpoint.Meta["processed"] != float64(9) || len(checkpointEvents(checkpoint.Meta["stageEvents"])) != 1 {
		t.Fatalf("stale probe regressed resume data or lost events: %#v", checkpoint)
	}
}

func TestCombinationResumeDoesNotUseOtherModuleCheckpoint(t *testing.T) {
	task := database.VulnScanTask{CheckpointJSON: MarshalCheckpoint("webscan:fingerprint_done", 50, nil)}
	if checkpoint := ScanCheckpoint(task, "dirscan", true); checkpoint.Stage != "" || checkpoint.Progress != 0 {
		t.Fatalf("directory scan inherited fingerprint progress: %#v", checkpoint)
	}
	if checkpoint := ScanCheckpoint(task, "webscan", true); checkpoint.Progress != 50 {
		t.Fatal("matching historical checkpoint must remain usable")
	}
	task.CheckpointJSON = CombinationSummaryCheckpoint(task.CheckpointJSON, 33)
	if checkpoint := ScanCheckpoint(task, "webscan", true); checkpoint.Progress != 50 {
		t.Fatal("scheduler aggregation erased the historical module checkpoint")
	}
	task.CheckpointJSON = MarshalCheckpoint("completed", 100, nil)
	if checkpoint := ScanCheckpoint(task, "webscan", true); checkpoint.Stage != "" {
		t.Fatal("unattributable completed checkpoint must not skip another module")
	}
}

func TestCombinationCheckpointPauseDoesNotFinalizeParent(t *testing.T) {
	db := combinationCheckpointTestDB(t)
	task := database.VulnScanTask{ID: "pause", Version: 1, Status: StatusRunning, PauseRequested: true, Progress: 25}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	_, checkpointAndPause := CombinationCheckpointHandlers("dirscan")
	if err := checkpointAndPause(context.Background(), db, task.ID, 1, "dirscan:scanning", 60, map[string]interface{}{"processed": 6}); !errors.Is(err, ErrTaskPaused) {
		t.Fatalf("expected pause signal, got %v", err)
	}
	var got database.VulnScanTask
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusRunning || !got.PauseRequested || got.Progress != 25 || ScanCheckpoint(got, "dirscan", true).Progress != 60 {
		t.Fatalf("a child finalized its parent before other workers exited: %#v", got)
	}
}
