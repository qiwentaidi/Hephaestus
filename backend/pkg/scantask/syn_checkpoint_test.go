package scantask

import (
	"context"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scanexec"
	"hephaestus-backend/pkg/taskstate"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Exercises the real local task controller/lifecycle and on-disk SQLite restart.
// The native scanner is a deterministic worker that exposes cancellation and
// delayed cleanup, so no Windows driver, raw-socket permission or network is used.
// portscan's companion tests verify construction of this checkpoint contract.
func TestSYNCheckpointPauseAcknowledgementAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scan.db")
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000"), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { sqlDB.Close() })
		return db
	}
	db := open()
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	task := database.VulnScanTask{ID: "syn-checkpoint-pause", Version: 1, ScanMode: "local", ScanType: "portscan", Status: taskstate.StatusRunning, Targets: `["192.0.2.1"]`, Config: `{"portScanMode":"syn"}`}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	started, cancelSeen, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	old := scanexec.Handlers["portscan"]
	calls := 0
	scanexec.Handlers["portscan"] = func(ctx context.Context, id string, _ []string, _ map[string]interface{}) error {
		calls++
		// First round's send progress must not certify any completed resume prefix.
		if err := taskstate.SaveCheckpoint(db, id, 1, "portscan:scanning", 40, map[string]interface{}{"completed": 0, "displayCompleted": 128, "total": 320, "resumePolicy": "syn-replay-v1"}); err != nil {
			return err
		}
		close(started)
		<-ctx.Done()
		close(cancelSeen)
		<-release // e.g. packet receivers/result callbacks have not stopped yet.
		return ctx.Err()
	}
	exited := make(chan error, 1)
	finished := make(chan struct{})
	go func() { defer close(finished); exited <- RunLocalTaskWithDB(db, task.ID) }()
	t.Cleanup(func() {
		control.GetTaskController().CancelTask(task.ID)
		unblock()
		<-finished
		scanexec.Handlers["portscan"] = old
	})
	wait := func(ch <-chan struct{}) {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not reach expected state")
		}
	}
	wait(started)
	// Local pause request: persist intent, then cancel the registered worker.
	if err := db.Model(&database.VulnScanTask{}).Where("id=?", task.ID).Update("pause_requested", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := control.GetTaskController().CancelTask(task.ID); err != nil {
		t.Fatal(err)
	}
	wait(cancelSeen)
	read := func(db *gorm.DB) database.VulnScanTask {
		var got database.VulnScanTask
		if err := db.Where("id=?", task.ID).First(&got).Error; err != nil {
			t.Fatal(err)
		}
		return got
	}
	got := read(db)
	if got.Status == taskstate.StatusPaused || !got.PauseRequested {
		t.Fatalf("premature pause acknowledgement: %+v", got)
	}
	unblock()
	select {
	case err := <-exited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not acknowledge exit")
	}
	got = read(db)
	if got.Status != taskstate.StatusPaused || got.PauseRequested {
		t.Fatalf("pause not finalized: %+v", got)
	}
	checkpoint := taskstate.ParseCheckpoint(got.CheckpointJSON)
	if checkpoint.Meta["completed"] != float64(0) || checkpoint.Progress != 40 || ShouldFinalizeRecoveredTask(&got) {
		t.Fatalf("unsafe checkpoint: %+v", checkpoint)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := open()
	if err := ReconcilePausedTasksAfterRestart(restarted, nil); err != nil {
		t.Fatal(err)
	}
	if err := RunLocalTaskWithDB(restarted, task.ID); err != nil {
		t.Fatal(err)
	}
	if read(restarted).Status != taskstate.StatusPaused || calls != 1 {
		t.Fatal("restart resumed paused scan")
	}
	// Crash between persisted intent and worker acknowledgement: restart itself
	// acknowledges the vanished worker, and must preserve the replay checkpoint.
	if err := restarted.Model(&database.VulnScanTask{}).Where("id=?", task.ID).Updates(map[string]interface{}{"status": taskstate.StatusRunning, "pause_requested": true}).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, _ = restarted.DB()
	sqlDB.Close()
	restarted = open()
	if err := ReconcilePausedTasksAfterRestart(restarted, nil); err != nil {
		t.Fatal(err)
	}
	got = read(restarted)
	if got.Status != taskstate.StatusPaused || got.PauseRequested || taskstate.ParseCheckpoint(got.CheckpointJSON).Meta["completed"] != float64(0) {
		t.Fatal("restart lost pause intent or safe cursor")
	}
}
