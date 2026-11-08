package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"hephaestus-backend/handlers/scan"
	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/aibatch"
	"hephaestus-backend/pkg/airun"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"
)

func TestWorkerPostgresBatchPauseWaitsForActualLoopExit(t *testing.T) {
	store, db, conv, turn := testfixture.PostgresWorker(t)
	ctx := context.Background()
	if err := store.FinishWorkerTurn(ctx, turn, "completed", ""); err != nil {
		t.Fatal(err)
	}
	entered, requestExited := make(chan struct{}), make(chan struct{})
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
		close(requestExited)
	}))
	defer model.Close()
	defer model.CloseClientConnections()
	manager, err := aibatch.New(db, func(ctx context.Context, q aibatch.Queue, item aibatch.Item, save aibatch.Checkpoint) (string, error) {
		item.ConversationID = conv.ID
		return runDistributedAIBatchItem(ctx, db, store, q, item, save)
	})
	if err != nil {
		t.Fatal(err)
	}
	queue, err := manager.Create(aibatch.Queue{Name: "pause fixture", ProjectID: conv.ProjectID, Concurrency: 1}, []string{"controlled pause fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Start(queue.ID); err != nil {
		t.Fatal(err)
	}
	var claimed *pentest.WorkerTurn
	deadline := time.Now().Add(5 * time.Second)
	for claimed == nil && time.Now().Before(deadline) {
		claimed, err = store.ClaimWorkerTurn(ctx, "fixture-worker", 30*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if claimed == nil {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if claimed == nil {
		t.Fatal("batch did not enqueue Worker")
	}
	workspace := t.TempDir()
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		executeAIWorkerTurnWithBuilder(ctx, store, *claimed, true, "", nil, func(_ context.Context, s *pentest.SQLStore, _ pentest.WorkerTurn) (pentest.AgentLoopConfig, error) {
			return pentest.AgentLoopConfig{Store: s, BaseURL: model.URL, APIKey: "fixture", Model: "fixture", WorkDir: workspace, HTTPClient: model.Client(), ToolCatalog: &pentest.SystemToolCatalog{}, DisableModelStreaming: true}, nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("loop never entered model")
	}
	if err = manager.Pause(queue.ID); err != nil {
		t.Fatal(err)
	}
	var current aibatch.Queue
	if err = db.First(&current, "id=?", queue.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != "pausing" {
		t.Fatal("queue paused before Worker acknowledgement")
	}
	select {
	case <-exited:
	case <-time.After(8 * time.Second):
		t.Fatal("Worker did not exit")
	}
	select {
	case <-requestExited:
	case <-time.After(3 * time.Second):
		t.Fatal("model context still active")
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		db.First(&current, "id=?", queue.ID)
		if current.Status == "paused" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if current.Status != "paused" {
		t.Fatalf("queue state %s: %s", current.Status, current.Error)
	}
	if err = manager.Recover(func(item aibatch.Item) error {
		return airun.WaitWorkerStoppedWith(ctx, store, item.ConversationID, func(ctx context.Context, s *pentest.SQLStore, id string) error {
			return airun.FinalizeDistributedWorkerStopWithDB(ctx, s, db, id)
		})
	}); err != nil {
		t.Fatal(err)
	}
	if next, e := store.ClaimWorkerTurn(ctx, "fixture-worker", 30*time.Second); e != nil || next != nil {
		t.Fatalf("restart resumed paused turn %v %v", next, e)
	}
}

func TestWorkerPostgresRestartPauseKeepsUnacknowledgedScanNonterminal(t *testing.T) {
	_, db, _, _ := testfixture.PostgresWorker(t)
	if err := db.AutoMigrate(&database.CombinationTask{}); err != nil {
		t.Fatal(err)
	}
	combo := database.CombinationTask{ID: "combo-fixture", Name: "fixture", Targets: `["fixture"]`, Nodes: `[{"moduleId":"jsscan"}]`, Context: `{"runningModules":{"jsscan":true}}`, Status: "running", PauseRequested: true}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatal(err)
	}
	parent := database.VulnScanTask{ID: "combo-parent", Version: 1, ScanType: "combination", ScanMode: "auto", ParentCombinationID: combo.ID, Status: "running", PauseRequested: true, Targets: `["fixture"]`, Config: `{}`}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	item := database.VulnScanTaskItem{TaskID: parent.ID, Version: 1, ScanType: "jsscan", Status: "running", WorkerAgentID: "remote", WorkerGeneration: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	if err := scantask.ReconcilePausedTasksAfterRestart(db, scan.ReconcilePausedCombinationTasksAfterRestart); err != nil {
		t.Fatal(err)
	}
	db.First(&combo, "id=?", combo.ID)
	db.First(&parent, "id=?", parent.ID)
	if combo.Status != "pausing" || parent.Status != "pausing" {
		t.Fatal("restart claimed remote scan had already stopped")
	}
	if err := db.Model(&item).Update("status", "paused").Error; err != nil {
		t.Fatal(err)
	}
	if err := scantask.ReconcilePausedTasksAfterRestart(db, scan.ReconcilePausedCombinationTasksAfterRestart); err != nil {
		t.Fatal(err)
	}
	db.First(&combo, "id=?", combo.ID)
	db.First(&parent, "id=?", parent.ID)
	if combo.Status != "paused" || parent.Status != "paused" {
		t.Fatalf("final states %s %s", combo.Status, parent.Status)
	}
}

func TestWorkerPostgresRealLoopCancellationAndRestart(t *testing.T) {
	store, db, conv, turn := testfixture.PostgresWorker(t)
	ctx := context.Background()
	workspace := t.TempDir()
	modelStarted := make(chan struct{})
	requestExited := make(chan struct{})
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(modelStarted)
		<-r.Context().Done()
		close(requestExited)
	}))
	defer model.Close()
	defer model.CloseClientConnections()
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		executeAIWorkerTurnWithBuilder(ctx, store, turn, true, "", nil, func(_ context.Context, s *pentest.SQLStore, _ pentest.WorkerTurn) (pentest.AgentLoopConfig, error) {
			return pentest.AgentLoopConfig{Store: s, BaseURL: model.URL, APIKey: "fixture", Model: "fixture", WorkDir: workspace, HTTPClient: model.Client(), ToolCatalog: &pentest.SystemToolCatalog{}, DisableModelStreaming: true, ModelRetryMaxRetries: 1}, nil
		})
	}()
	select {
	case <-modelStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("model did not start")
	}
	if err := store.RequestWorkerStop(ctx, conv.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := airun.FinalizeDistributedWorkerStopWithDB(ctx, store, db, conv.ID); err == nil {
		t.Fatal("public cancellation before worker acknowledgement")
	}
	// Primary restart must not requeue the durable cancellation request.
	if count, err := store.ImportLegacyWorkerTurns(ctx); err != nil || count != 0 {
		t.Fatalf("restart requeued cancelled work %d %v", count, err)
	}
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		t.Fatal("worker never acknowledged exit")
	}
	select {
	case <-requestExited:
	case <-time.After(5 * time.Second):
		t.Fatal("model request context not cancelled")
	}
	saved, err := store.LatestWorkerTurn(ctx, conv.ID)
	if err != nil || saved.Status != "stopped" {
		t.Fatalf("worker exit %v %v", saved, err)
	}
	if err = airun.FinalizeDistributedWorkerStopWithDB(ctx, store, db, conv.ID); err != nil {
		t.Fatal(err)
	}
	current, err := store.GetConversation(ctx, conv.ID)
	if err != nil || current.Status != "cancelled" {
		t.Fatalf("public state %v %v", current, err)
	}
	// The worker's transcript and progress must survive a different workspace.
	destination := t.TempDir()
	if err = store.RestoreWorkerWorkspace(ctx, conv.ID, destination); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(destination, "transcript.json")); err != nil {
		t.Fatalf("missing persisted transcript %v", err)
	}
}

func TestWorkerPostgresCancelWaitsForOwnedScanExit(t *testing.T) {
	store, db, conv, turn := testfixture.PostgresWorker(t)
	ctx := context.Background()
	scoped, _ := store.WithWorkerLease(turn)
	scan := database.VulnScanTask{ID: "owned-scan", Version: 1, ScanType: "jsscan", ScanMode: "auto", Status: "running", Config: "{}", Targets: `["fixture"]`}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	item := database.VulnScanTaskItem{TaskID: scan.ID, Version: 1, ScanType: "jsscan", Status: "running", WorkerAgentID: "scan-worker", WorkerGeneration: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	execution := pentest.ToolExecution{ID: uuid.NewString(), ConversationID: conv.ID, ToolName: "attack_surface_create_jsscan_task", ArgumentsJSON: `{}`, Status: pentest.ToolExecutionStatusCompleted, StartTime: time.Now(), ResultJSON: fmt.Sprintf(`{"ok":true,"tool_name":"attack_surface_create_jsscan_task","execution_id":"fixture-execution","status":"completed","result":{"taskId":%q}}`, scan.ID)}
	if _, err := scoped.SaveToolExecution(ctx, execution); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestWorkerStop(ctx, conv.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishWorkerTurn(ctx, turn, "stopped", ""); err != nil {
		t.Fatal(err)
	}
	if err := airun.FinalizeDistributedWorkerStopWithDB(ctx, store, db, conv.ID); err == nil {
		t.Fatal("cancelled before scan worker exit acknowledgement")
	}
	var pending database.VulnScanTaskItem
	if err := db.First(&pending, item.ID).Error; err != nil || pending.Status != "running" {
		t.Fatal("cancellation falsely marked running scan stopped")
	}
	if err := db.Model(&item).Update("status", "cancelled").Error; err != nil {
		t.Fatal(err)
	}
	if err := airun.FinalizeDistributedWorkerStopWithDB(ctx, store, db, conv.ID); err != nil {
		t.Fatal(err)
	}
	current, err := store.GetConversation(ctx, conv.ID)
	if err != nil || current.Status != "cancelled" {
		t.Fatalf("final parent %v %v", current, err)
	}
}

func TestWorkerPostgresMissingOwnedScanIsTerminal(t *testing.T) {
	store, db, conv, turn := testfixture.PostgresWorker(t)
	ctx := context.Background()
	scoped, err := store.WithWorkerLease(turn)
	if err != nil {
		t.Fatal(err)
	}
	execution := pentest.ToolExecution{
		ID:             uuid.NewString(),
		ConversationID: conv.ID,
		ToolName:       "attack_surface_create_webscan_task",
		ArgumentsJSON:  `{}`,
		Status:         pentest.ToolExecutionStatusCompleted,
		StartTime:      time.Now(),
		ResultJSON:     `{"ok":true,"result":{"taskId":"removed-scan"}}`,
	}
	if _, err = scoped.SaveToolExecution(ctx, execution); err != nil {
		t.Fatal(err)
	}
	if err = store.RequestWorkerStop(ctx, conv.ID, false); err != nil {
		t.Fatal(err)
	}
	if err = store.FinishWorkerTurn(ctx, turn, "stopped", ""); err != nil {
		t.Fatal(err)
	}
	if err = airun.FinalizeDistributedWorkerStopWithDB(ctx, store, db, conv.ID); err != nil {
		t.Fatalf("missing owned scan kept worker nonterminal: %v", err)
	}
	current, err := store.GetConversation(ctx, conv.ID)
	if err != nil || current.Status != "cancelled" {
		t.Fatalf("final parent %v %v", current, err)
	}
	finished, err := store.LatestWorkerTurn(ctx, conv.ID)
	if err != nil || finished.Status != "cancelled" {
		t.Fatalf("worker status %v %v", finished, err)
	}
	if err = scantask.RequestPause(db, "removed-scan"); !errors.Is(err, scantask.ErrNotCancelable) {
		t.Fatalf("missing pause target error = %v", err)
	}
}

func TestWorkerPostgresDeletedConversationStopReconcileIsIdempotent(t *testing.T) {
	store, db, conv, turn := testfixture.PostgresWorker(t)
	ctx := context.Background()
	if err := store.RequestWorkerStop(ctx, conv.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishWorkerTurn(ctx, turn, "stopped", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteConversation(ctx, conv.ID); err != nil {
		t.Fatal(err)
	}
	if err := airun.FinalizeDistributedWorkerStopWithDB(ctx, store, db, conv.ID); err != nil {
		t.Fatalf("deleted conversation remained a stop failure: %v", err)
	}
	if err := airun.WaitWorkerStoppedWith(ctx, store, conv.ID, func(ctx context.Context, s *pentest.SQLStore, id string) error {
		return airun.FinalizeDistributedWorkerStopWithDB(ctx, s, db, id)
	}); err != nil {
		t.Fatalf("deleted conversation remained in wait path: %v", err)
	}
}
