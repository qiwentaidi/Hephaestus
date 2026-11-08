package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func combinationProgressTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	var db *gorm.DB
	var err error
	if dsn := os.Getenv("HEPHAESTUS_TEST_POSTGRES_DSN"); dsn != "" {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		schema := fmt.Sprintf("combination_test_%d", time.Now().UnixNano())
		if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Exec("DROP SCHEMA " + schema + " CASCADE") })
		db, err = gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	} else {
		db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "combination.db")+"?_busy_timeout=5000&_journal_mode=WAL&_txlock=immediate"), &gorm.Config{})
		if err == nil {
			sqlDB, _ := db.DB()
			sqlDB.SetMaxOpenConns(8)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.CombinationTask{}, &database.VulnerabilityInfo{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCombinationParallelModulesPreserveProgressAndCheckpoints(t *testing.T) {
	db := combinationProgressTestDB(t)
	nodes := []combinationPipelineNode{{ID: "web", ModuleID: "webscan"}, {ID: "dir", ModuleID: "dirscan"}}
	state := newCombinationContext([]string{"http://example.test"})
	for _, node := range nodes {
		state.Running[node.ModuleID] = true
		state.markStepQueued(node.ModuleID, "parent", time.Now())
	}
	combo := database.CombinationTask{ID: "parallel", Status: "running", Context: state.serialize()}
	parent := database.VulnScanTask{ID: "parent", Version: 1, ScanType: "combination", ParentCombinationID: combo.ID, Status: "running"}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	started := make(chan string, 2)
	release := make(chan struct{})
	for _, node := range nodes {
		module := node.ModuleID
		original := scanTypeHandlers[module]
		scanTypeHandlers[module] = func(ctx context.Context, taskID string, targets []string, config map[string]interface{}) error {
			if err := taskstate.SaveCombinationCheckpoint(db, taskID, 1, module, module+":scanning", 80, map[string]interface{}{
				"stageEvents": []map[string]interface{}{{"stage": "complete", "label": module, "time": time.Now()}},
			}); err != nil {
				return err
			}
			started <- module
			<-release
			return nil
		}
		t.Cleanup(func() { scanTypeHandlers[module] = original })
	}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for i, node := range nodes {
		localCombo, localParent, localState := combo, parent, state.clone()
		wg.Go(func() {
			errors <- runCombinationModule(db, &localCombo, &localParent, nodes, node, i, []string{"http://example.test"}, localState)
		})
	}
	for range nodes {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			close(release)
			wg.Wait()
			t.Fatal("parallel modules did not start")
		}
	}
	close(release)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var got database.VulnScanTask
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Progress != 100 || got.Status != "completed" {
		t.Fatalf("parallel completion lost a module: %#v", got)
	}
	for _, node := range nodes {
		if taskstate.ScanCheckpoint(got, node.ModuleID, true).Progress != 80 {
			t.Fatalf("scheduler erased module %s's checkpoint", node.ModuleID)
		}
	}
	if err := db.First(&combo).Error; err != nil {
		t.Fatal(err)
	}
	state, err := loadCombinationContext(&combo)
	if err != nil || countCompletedModules(state, nodes) != 2 || len(state.Running) != 0 {
		t.Fatalf("completed modules were lost or resurrected: state=%#v err=%v", state, err)
	}
}

func TestCombinationSchedulingUsesLatestContext(t *testing.T) {
	db := combinationProgressTestDB(t)
	nodes := []combinationPipelineNode{{ID: "web", ModuleID: "webscan"}, {ID: "dir", ModuleID: "dirscan", StageIndex: 1}}
	state := newCombinationContext([]string{"http://example.test"})
	state.Completed["webscan"] = true
	state.markStepFinished("webscan", "completed", nil, time.Now())
	combo := database.CombinationTask{ID: "queue", Status: "running", Context: state.serialize(), Progress: 50}
	parent := database.VulnScanTask{ID: "parent", Version: 1, ScanType: "combination", ParentCombinationID: combo.ID, Status: "running", Progress: 50}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	oldEvents, oldParallel := combinationEvents, maxCombinationParallel
	combinationEvents, maxCombinationParallel = make(chan combinationEvent, 4), 2
	t.Cleanup(func() { combinationEvents, maxCombinationParallel = oldEvents, oldParallel })
	stale := newCombinationContext([]string{"http://example.test"})
	if count, err := scheduleNextCombinationNodes(db, &combo, nodes, stale); err != nil || count != 1 {
		t.Fatalf("schedule from stale snapshot: count=%d err=%v", count, err)
	}
	if !stale.Completed["webscan"] || stale.stepLog("dirscan").Status != "queued" {
		t.Fatal("scheduling overwrote a previously completed module")
	}
	secondSnapshot := newCombinationContext(nil)
	if count, err := scheduleNextCombinationNodes(db, &combo, nodes, secondSnapshot); err != nil || count != 0 {
		t.Fatalf("stale scheduler queued the same module twice: count=%d err=%v", count, err)
	}
	if len(combinationEvents) != 1 {
		t.Fatal("expected exactly one queue event")
	}
}

func TestCombinationPauseWaitsForAllModulesAndPreservesRestartCheckpoints(t *testing.T) {
	db := combinationProgressTestDB(t)
	nodes := []combinationPipelineNode{{ID: "web", ModuleID: "webscan"}, {ID: "dir", ModuleID: "dirscan"}}
	state := newCombinationContext([]string{"http://example.test"})
	for _, node := range nodes {
		state.Running[node.ModuleID] = true
		state.markStepQueued(node.ModuleID, "parent", time.Now())
	}
	combo := database.CombinationTask{
		ID: "pause-chain", Status: "running", Context: state.serialize(),
		Targets: `["http://example.test"]`, Nodes: `[{"id":"web","moduleId":"webscan"},{"id":"dir","moduleId":"dirscan"}]`,
	}
	parent := database.VulnScanTask{ID: "parent", Version: 1, ScanMode: "local", ScanType: "combination", ParentCombinationID: combo.ID, Status: "running", Progress: 25}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	started, cancelled := make(chan string, 2), make(chan string, 2)
	releases := map[string]chan struct{}{"webscan": make(chan struct{}), "dirscan": make(chan struct{})}
	done := map[string]chan error{"webscan": make(chan error, 1), "dirscan": make(chan error, 1)}
	for _, node := range nodes {
		module := node.ModuleID
		original := scanTypeHandlers[module]
		scanTypeHandlers[module] = func(ctx context.Context, taskID string, targets []string, config map[string]interface{}) error {
			if err := taskstate.SaveCombinationCheckpoint(db, taskID, 1, module, module+":scanning", 60, map[string]interface{}{"processed": 6}); err != nil {
				return err
			}
			started <- module
			<-ctx.Done()
			cancelled <- module
			<-releases[module]
			return ctx.Err()
		}
		t.Cleanup(func() { scanTypeHandlers[module] = original })
	}
	for i, node := range nodes {
		localCombo, localParent, localState := combo, parent, state.clone()
		go func() {
			done[node.ModuleID] <- runCombinationModule(db, &localCombo, &localParent, nodes, node, i, []string{"http://example.test"}, localState)
		}()
	}
	for range nodes {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("modules did not start")
		}
	}
	if err := pauseCombinationTask(db, &parent); err != nil {
		t.Fatal(err)
	}
	for range nodes {
		select {
		case <-cancelled:
		case <-time.After(5 * time.Second):
			t.Fatal("pause did not cancel every module context")
		}
	}
	close(releases["webscan"])
	if err := <-done["webscan"]; err != nil {
		t.Fatal(err)
	}
	var intermediateParent database.VulnScanTask
	var intermediateCombo database.CombinationTask
	if err := db.First(&intermediateParent).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&intermediateCombo).Error; err != nil {
		t.Fatal(err)
	}
	if intermediateParent.Status != "running" || !intermediateParent.PauseRequested {
		t.Fatal("parent was paused before the second worker acknowledged exit")
	}
	close(releases["dirscan"])
	if err := <-done["dirscan"]; err != nil {
		t.Fatal(err)
	}
	var got database.VulnScanTask
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != "paused" || got.PauseRequested || got.Progress != 25 {
		t.Fatalf("parent did not finalize after both exits: %#v", got)
	}
	// Emulate a restart from the persisted intermediate pause request, after
	// all old workers have exited. Recovery must not automatically requeue it.
	if err := db.Model(&database.CombinationTask{}).Where("id = ?", combo.ID).Updates(map[string]interface{}{
		"context": intermediateCombo.Context, "status": "running", "pause_requested": true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.VulnScanTask{}).Where("id = ?", parent.ID).Updates(map[string]interface{}{
		"status": "recovering", "pause_requested": true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := scantask.ReconcilePausedTasksAfterRestart(db, ReconcilePausedCombinationTasksAfterRestart); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != "paused" || got.PauseRequested {
		t.Fatal("restart requeued an explicitly paused combination")
	}
	for _, node := range nodes {
		checkpoint := taskstate.ScanCheckpoint(got, node.ModuleID, true)
		if checkpoint.Progress != 60 || checkpoint.Meta["processed"] != float64(6) {
			t.Fatalf("restart lost %s's resume checkpoint: %#v", node.ModuleID, checkpoint)
		}
	}
}
