package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"hephaestus-backend/handlers/scan"
	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/pkg/aibatch"
	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"
)

func TestShouldBypassAIBatchQueueGuardForCompletedMessage(t *testing.T) {
	path := "/api/v1/ai/sessions/session-1/message/stream"
	for _, status := range []string{"completed", "failed", "cancelled", "blocked"} {
		if !shouldBypassAIBatchQueueGuardForMessage(path, "POST", status) {
			t.Fatalf("terminal status %q was still blocked", status)
		}
	}
	for _, status := range []string{"queued", "running", "waiting_tool", "waiting_approval"} {
		if shouldBypassAIBatchQueueGuardForMessage(path, "POST", status) {
			t.Fatalf("active status %q bypassed the queue guard", status)
		}
	}
	if shouldBypassAIBatchQueueGuardForMessage(path, "GET", "completed") {
		t.Fatal("GET unexpectedly matched the message bypass")
	}
	if shouldBypassAIBatchQueueGuardForMessage("/api/v1/ai/sessions/session-1/action", "POST", "completed") {
		t.Fatal("session action unexpectedly matched the message bypass")
	}
}

func awaitBatchSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("worker acknowledgement timed out")
	}
}
func TestAIBatchPauseCancelsNativeModuleAndWaitsForAcknowledgement(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	ack := make(chan struct{})
	nativeDone := make(chan struct{})
	workerErrors := make(chan error, 2)
	manager, err := aibatch.New(db, func(ctx context.Context, q aibatch.Queue, item aibatch.Item, save aibatch.Checkpoint) (string, error) {
		config, _ := json.Marshal(map[string]any{"aiBatchItemId": batchItemGeneration(item)})
		native := database.VulnScanTask{ID: "batch-native-pause", Version: 1, TaskName: "module", ScanType: "webscan", ScanMode: "local", Status: "running", Config: string(config)}
		if err := db.Create(&native).Error; err != nil {
			return "", err
		}
		nativeCtx, nativeCancel := control.GetTaskController().CreateTaskContext(native.ID)
		go func() {
			defer close(nativeDone)
			defer nativeCancel()
			<-nativeCtx.Done()
			close(cancelled)
			<-ack
			var state database.VulnScanTask
			if err := db.First(&state, "id = ?", native.ID).Error; err != nil {
				workerErrors <- err
				return
			}
			if !state.PauseRequested {
				workerErrors <- fmt.Errorf("cancel preceded persisted pause intent")
				return
			}
			if err := taskstate.MarkPaused(db, native.ID, 1, "worker acknowledged exit"); err != nil {
				workerErrors <- err
			}
		}()
		close(started)
		<-ctx.Done()
		if err := stopAIBatchModules(db, item, false); err != nil {
			return "", fmt.Errorf("%w: %v", aibatch.ErrUnacknowledged, err)
		}
		return "retained evidence", ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := manager.Create(aibatch.Queue{Name: "pause", ProjectID: "project", Concurrency: 1}, []string{"scan host", "next task"})
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Start(q.ID); err != nil {
		t.Fatal(err)
	}
	awaitBatchSignal(t, started)
	if err = manager.Pause(q.ID); err != nil {
		t.Fatal(err)
	}
	awaitBatchSignal(t, cancelled)
	state, _ := manager.Get(q.ID)
	if state.Status != "pausing" || !state.PauseRequested {
		t.Fatalf("premature parent pause %+v", state)
	}
	if state.Items[1].Status != "pending" {
		t.Fatal("pending item dispatched during pause")
	}
	close(ack)
	awaitBatchSignal(t, nativeDone)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, _ = manager.Get(q.ID)
		if state.Status == "paused" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if state.Status != "paused" || state.Items[0].Status != "paused" {
		t.Fatalf("parent not finalized after acknowledgement %+v", state)
	}
	select {
	case err := <-workerErrors:
		t.Fatal(err)
	default:
	}
}
func TestAIBatchRestartReconcilesNativeScansBeforeInterruptedRecovery(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	manager, err := aibatch.New(db, func(context.Context, aibatch.Queue, aibatch.Item, aibatch.Checkpoint) (string, error) {
		t.Error("restart dispatched work")
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := manager.Create(aibatch.Queue{Name: "restart", ProjectID: "project", Concurrency: 1}, []string{"scan host"})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&aibatch.Queue{}).Where("id = ?", q.ID).Updates(map[string]any{"status": "running", "pause_requested": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&aibatch.Item{}).Where("id = ?", q.Items[0].ID).Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(map[string]any{"aiBatchItemId": batchItemGeneration(q.Items[0])})
	native := database.VulnScanTask{ID: "restart-module", Version: 1, TaskName: "native", ScanType: "webscan", ScanMode: "local", Status: "running", Config: string(config)}
	if err = db.Create(&native).Error; err != nil {
		t.Fatal(err)
	}
	if err = manager.Recover(func(i aibatch.Item) error { return stopAIBatchModules(db, i, true) }); err != nil {
		t.Fatal(err)
	}
	if err = scantask.ReconcilePausedTasksAfterRestart(db, scan.ReconcilePausedCombinationTasksAfterRestart); err != nil {
		t.Fatal(err)
	}
	scan.RecoverInterruptedLocalTasksWithDB(db, time.Now())
	var got database.VulnScanTask
	if err = db.First(&got, "id = ?", native.ID).Error; err != nil {
		t.Fatal(err)
	}
	state, _ := manager.Get(q.ID)
	if state.Status != "paused" || got.Status != "paused" {
		t.Fatalf("restart resumed paused work: %s/%s", state.Status, got.Status)
	}
}

func TestAIBatchLateNativeWorkerHonorsPauseBeforeStarting(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	task := database.VulnScanTask{ID: "late-queue-worker", Version: 1, TaskName: "late worker", ScanType: "webscan", ScanMode: "local", Status: "running", PauseRequested: true, Targets: "[]", Config: "{}"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := scantask.RunLocalTaskWithDB(db, task.ID); err != nil {
		t.Fatal(err)
	}
	var state database.VulnScanTask
	if err := db.First(&state, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if state.Status != "paused" {
		t.Fatalf("late worker restarted paused scan: %+v", state)
	}
	if control.GetTaskController().IsTaskRunning(task.ID) {
		t.Fatal("late worker left active context")
	}
}
