package aibatch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "batch.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestDelayedPauseAcknowledgementReconcilesWithoutStoppingOtherQueues(t *testing.T) {
	m := testManager(t, nil)
	paused := queueForTest(t, m, 1, "pending acknowledgement")
	running := queueForTest(t, m, 1, "unrelated running queue")
	if err := m.db.Model(&Queue{}).Where("id=?", paused.ID).Updates(map[string]any{"status": "pausing", "pause_requested": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.db.Model(&Item{}).Where("queue_id=?", paused.ID).Update("status", "pausing").Error; err != nil {
		t.Fatal(err)
	}
	if err := m.db.Model(&Queue{}).Where("id=?", running.ID).Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}
	if err := m.ReconcilePaused(func(Item) error { return ErrUnacknowledged }); err != nil {
		t.Fatal(err)
	}
	if q, _ := m.Get(paused.ID); q.Status != "pausing" {
		t.Fatal("missing acknowledgement was ignored")
	}
	if err := m.ReconcilePaused(func(Item) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if q, _ := m.Get(paused.ID); q.Status != "paused" {
		t.Fatal("acknowledged pause never finalized")
	}
	if q, _ := m.Get(running.ID); q.Status != "running" || q.PauseRequested {
		t.Fatal("reconciliation stopped unrelated work")
	}
}
func testManager(t *testing.T, run Runner) *Manager {
	t.Helper()
	m, err := New(testDB(t), run)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func queueForTest(t *testing.T, m *Manager, concurrency int, instructions ...string) Queue {
	t.Helper()
	q, err := m.Create(Queue{Name: "test", ProjectID: "project", Concurrency: concurrency}, instructions)
	if err != nil {
		t.Fatal(err)
	}
	return q
}
func waitQueue(t *testing.T, m *Manager, id, status string) Queue {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		q, err := m.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if q.Status == status {
			return q
		}
		time.Sleep(5 * time.Millisecond)
	}
	q, _ := m.Get(id)
	t.Fatalf("queue state %s, wanted %s: %+v", q.Status, status, q)
	return q
}
func waitSignal(t *testing.T, c <-chan struct{}) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for worker")
	}
}

func TestBatchPauseCancelsRequestWaitsForWorkerAndResumesSameConversation(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCancelled := make(chan struct{})
	ack := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(requestStarted) })
		<-r.Context().Done()
		close(requestCancelled)
	}))
	defer server.Close()
	var calls atomic.Int32
	m := testManager(t, func(ctx context.Context, q Queue, item Item, save Checkpoint) (string, error) {
		if item.Instruction == "completed first" {
			return "saved result", nil
		}
		if calls.Add(1) == 1 {
			item.ConversationID = "conversation-preserved"
			if err := save(item); err != nil {
				return "", err
			}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			resp, err := http.DefaultClient.Do(req)
			if resp != nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			// The actual HTTP request must be cancelled, and worker cleanup must finish.
			<-ack
			return "partial evidence", err
		}
		if item.ConversationID != "conversation-preserved" {
			return "", fmt.Errorf("lost conversation")
		}
		return "continued", nil
	})
	q := queueForTest(t, m, 1, "completed first", "summarize existing findings")
	if err := m.Start(q.ID); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, requestStarted)
	if err := m.Pause(q.ID); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, requestCancelled)
	paused, _ := m.Get(q.ID)
	if !paused.PauseRequested || paused.Status != "pausing" {
		t.Fatalf("premature pause: %+v", paused)
	}
	if err := m.Start(q.ID); err == nil {
		t.Fatal("resumed before acknowledgement")
	}
	if err := m.Delete(q.ID); err == nil {
		t.Fatal("deleted active queue")
	}
	close(ack)
	paused = waitQueue(t, m, q.ID, "paused")
	if paused.Items[0].Status != "completed" || paused.Items[1].Status != "paused" {
		t.Fatalf("wrong child states: %+v", paused.Items)
	}
	if err := m.Start(q.ID); err != nil {
		t.Fatal(err)
	}
	done := waitQueue(t, m, q.ID, "completed")
	if done.Items[0].Attempts != 1 || done.Items[1].Attempts != 2 || done.Items[1].ConversationID != "conversation-preserved" {
		t.Fatalf("resume repeated completed work or lost history: %+v", done.Items)
	}
}
func TestBatchConcurrencyAndDuplicateStart(t *testing.T) {
	release := make(chan struct{})
	var running, peak atomic.Int32
	started := make(chan struct{}, 4)
	m := testManager(t, func(ctx context.Context, q Queue, item Item, save Checkpoint) (string, error) {
		n := running.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		running.Add(-1)
		return "ok", ctx.Err()
	})
	q := queueForTest(t, m, 2, "one", "two", "three", "four")
	if err := m.Start(q.ID); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, started)
	waitSignal(t, started)
	if err := m.Start(q.ID); err == nil {
		t.Fatal("duplicate start allowed")
	}
	if peak.Load() != 2 {
		t.Fatal("concurrency not respected")
	}
	close(release)
	done := waitQueue(t, m, q.ID, "completed")
	for _, i := range done.Items {
		if i.Attempts != 1 {
			t.Fatal("duplicate dispatch")
		}
	}
}
func TestBatchRestartHonorsPauseBeforeRecovery(t *testing.T) {
	var calls atomic.Int32
	m := testManager(t, func(context.Context, Queue, Item, Checkpoint) (string, error) { calls.Add(1); return "", nil })
	q := queueForTest(t, m, 1, "scan host", "summarize")
	q.Status = "running"
	q.PauseRequested = true
	q.Items[0].Status = "running"
	q.Items[0].ConversationID = "existing"
	q.Items[0].Modules = []string{"native-scan"}
	if err := m.save(&q); err != nil {
		t.Fatal(err)
	}
	recovered, err := New(m.db, m.run)
	if err != nil {
		t.Fatal(err)
	}
	reconciled := false
	if err = recovered.Recover(func(item Item) error {
		reconciled = true
		current, _ := m.Get(q.ID)
		if current.Status != "pausing" || !current.PauseRequested {
			t.Fatal("pause intent not persisted first")
		}
		if item.Modules[0] != "native-scan" {
			t.Fatal("lost module ownership")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	result, _ := recovered.Get(q.ID)
	if !reconciled || calls.Load() != 0 || result.Status != "paused" || result.Items[0].ConversationID != "existing" || result.Items[1].Status != "pending" {
		t.Fatalf("unsafe restart: %+v", result)
	}
}
func TestBatchUnacknowledgedChildrenNeverBecomePaused(t *testing.T) {
	m := testManager(t, func(context.Context, Queue, Item, Checkpoint) (string, error) { return "", ErrUnacknowledged })
	q := queueForTest(t, m, 1, "one", "two")
	if err := m.Start(q.ID); err != nil {
		t.Fatal(err)
	}
	result := waitQueue(t, m, q.ID, "pausing")
	if result.Items[1].Status != "pending" {
		t.Fatal("continued dispatch after failed exit")
	}
	if err := m.Start(q.ID); err == nil {
		t.Fatal("unconfirmed worker resumed")
	}
}
func TestBatchRetryAndNonURLInstructions(t *testing.T) {
	m := testManager(t, func(ctx context.Context, q Queue, i Item, save Checkpoint) (string, error) {
		if i.Instruction == "summarize project" && i.Attempts == 1 {
			return "", fmt.Errorf("temporary failure")
		}
		return i.Instruction, nil
	})
	q := queueForTest(t, m, 1, "summarize project", "scan 192.168.1.1", "enumerate example.com")
	if err := m.Start(q.ID); err != nil {
		t.Fatal(err)
	}
	done := waitQueue(t, m, q.ID, "completed_with_errors")
	if err := m.Retry(q.ID, done.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(q.ID); err != nil {
		t.Fatal(err)
	}
	done = waitQueue(t, m, q.ID, "completed")
	if done.Items[0].Attempts != 2 || done.Items[1].Attempts != 1 {
		t.Fatalf("bad retry %+v", done.Items)
	}
}

func TestRemoveConversationDeletesReferencingItemsAndEmptyQueues(t *testing.T) {
	m := testManager(t, func(ctx context.Context, q Queue, i Item, save Checkpoint) (string, error) {
		return i.Instruction, nil
	})
	q := queueForTest(t, m, 1, "one", "two")
	if err := m.checkpoint(q.ID, Item{ID: q.Items[0].ID, ConversationID: "conv-1"}); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveConversation("conv-1"); err != nil {
		t.Fatal(err)
	}
	got, err := m.Get(q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Instruction != "two" {
		t.Fatalf("referencing item not removed: %+v", got.Items)
	}

	single := queueForTest(t, m, 1, "only")
	if err := m.checkpoint(single.ID, Item{ID: single.Items[0].ID, ConversationID: "conv-1"}); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveConversation("conv-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(single.ID); err == nil {
		t.Fatal("empty queue was not deleted")
	}
	if err := m.RemoveConversation("missing-conv"); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveConversationCancelsActiveQueueBeforeDeleting(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	m := testManager(t, func(ctx context.Context, q Queue, i Item, save Checkpoint) (string, error) {
		i.ConversationID = "conv-run"
		if err := save(i); err != nil {
			return "", err
		}
		once.Do(func() { close(started) })
		<-ctx.Done()
		return "", ctx.Err()
	})
	q := queueForTest(t, m, 1, "run")
	if err := m.Start(q.ID); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, started)
	if err := m.RemoveConversation("conv-run"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(q.ID); err == nil {
		t.Fatal("queue left empty by the delete was not removed")
	}
	var count int64
	if err := m.db.Model(&Item{}).Where("conversation_id = ?", "conv-run").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("referencing item survived the delete")
	}
}
