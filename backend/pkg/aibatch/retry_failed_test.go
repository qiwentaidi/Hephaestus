package aibatch

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestBatchRetryFailedImmediatelyRunsFailuresAndPreservesSuccess(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	db := testDB(t)
	runner := func(ctx context.Context, q Queue, item Item, save Checkpoint) (string, error) {
		calls.Add(1)
		if item.Attempts > 1 {
			select {
			case <-release:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		if item.Attempts == 1 {
			item.ConversationID = "original-" + item.ID
			item.Modules = []string{"scan-" + item.ID}
			if err := save(item); err != nil {
				return "", err
			}
			if item.Position > 0 {
				return "partial evidence", fmt.Errorf("provider HTTP 403: blocked")
			}
		}
		if item.ConversationID != "original-"+item.ID || item.Generation != 1 || !reflect.DeepEqual(item.Modules, []string{"scan-" + item.ID}) {
			return "", fmt.Errorf("retry lost conversation or native scan ownership: %+v", item)
		}
		return "saved success", nil
	}
	m, err := New(db, runner)
	if err != nil {
		t.Fatal(err)
	}
	instructions := make([]string, 89)
	for i := range instructions {
		instructions[i] = fmt.Sprintf("fixture %d", i)
	}
	q := queueForTest(t, m, 1, instructions...)
	if err := m.Start(q.ID); err != nil {
		t.Fatal(err)
	}
	ended := waitQueue(t, m, q.ID, "completed_with_errors")
	n, err := m.RetryFailed(q.ID)
	if err != nil || n != 88 {
		t.Fatalf("retry=%d %v", n, err)
	}
	queued := waitQueue(t, m, q.ID, "running")
	if !reflect.DeepEqual(ended.Items[0], queued.Items[0]) {
		t.Fatal("successful item changed")
	}
	if queued.PauseRequested {
		t.Fatal("retry remained paused")
	}
	for _, item := range queued.Items[1:] {
		if (item.Status != "pending" && item.Status != "running") || item.Generation != 1 || item.ConversationID != "original-"+item.ID || len(item.PreviousConversations) != 0 || !reflect.DeepEqual(item.Modules, []string{"scan-" + item.ID}) || item.Result != "partial evidence" {
			t.Fatalf("bad retry checkpoint: %+v", item)
		}
	}
	if n, err := m.RetryFailed(q.ID); n != 0 || err == nil {
		t.Fatalf("active retry accepted: %d %v", n, err)
	}
	// Release the runner without issuing a separate Start request.
	unblock()
	done := waitQueue(t, m, q.ID, "completed")
	if calls.Load() != 177 || !reflect.DeepEqual(ended.Items[0], done.Items[0]) {
		t.Fatal("successful item was rerun")
	}
	for _, item := range done.Items[1:] {
		if item.Status != "completed" || item.Attempts != 2 {
			t.Fatalf("failed item did not execute: %+v", item)
		}
	}
	if n, err := m.RetryFailed(q.ID); n != 0 || err != nil {
		t.Fatalf("retry without failures: %d %v", n, err)
	}
}

func TestBatchRetryFailedRejectsUnstoppedChildrenAndRollsBack(t *testing.T) {
	var calls atomic.Int32
	m := testManager(t, func(_ context.Context, _ Queue, item Item, _ Checkpoint) (string, error) {
		calls.Add(1)
		if item.Position > 1 {
			t.Error("non-failed item dispatched")
		}
		return "", nil
	})
	q := queueForTest(t, m, 1, "a", "b", "completed", "paused")
	q.Status = "completed_with_errors"
	q.Items[0].Status = "failed"
	q.Items[1].Status = "failed"
	q.Items[2].Status = "completed"
	q.Items[3].Status = "pausing"
	if err := m.save(&q); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RetryFailed(q.ID); err == nil {
		t.Fatal("retried before child exit")
	}
	q.Items[3].Status = "paused"
	if err := m.save(&q); err != nil {
		t.Fatal(err)
	}
	before, _ := m.Get(q.ID)
	trigger := fmt.Sprintf("CREATE TRIGGER fail_retry BEFORE UPDATE ON ai_batch_items WHEN NEW.id = '%s' BEGIN SELECT RAISE(ABORT, 'fixture write failure'); END", q.Items[1].ID)
	if err := m.db.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}
	if n, err := m.RetryFailed(q.ID); err == nil || n != 0 {
		t.Fatal("write failure not returned")
	}
	after, _ := m.Get(q.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("partial retry transaction persisted")
	}
	if calls.Load() != 0 {
		t.Fatal("dispatched despite failed transaction")
	}
	if err := m.db.Exec("DROP TRIGGER fail_retry").Error; err != nil {
		t.Fatal(err)
	}
	if n, err := m.RetryFailed(q.ID); err != nil || n != 2 {
		t.Fatalf("retry=%d %v", n, err)
	}
	after = waitQueue(t, m, q.ID, "paused")
	if calls.Load() != 2 {
		t.Fatal("failed items did not execute automatically")
	}
	if !reflect.DeepEqual(before.Items[2:], after.Items[2:]) {
		t.Fatal("non-failed items changed")
	}
}

func TestSingleRetryKeepsCurrentConversationAcrossRepeatedFailures(t *testing.T) {
	var created atomic.Int32
	m := testManager(t, func(ctx context.Context, q Queue, item Item, save Checkpoint) (string, error) {
		if item.ConversationID == "" {
			created.Add(1)
			item.ConversationID = "current-conversation"
			item.Modules = []string{"completed-scan", "paused-scan"}
			if err := save(item); err != nil {
				return "", err
			}
		}
		if item.ConversationID != "current-conversation" || item.Generation != 3 || !reflect.DeepEqual(item.Modules, []string{"completed-scan", "paused-scan"}) {
			return "", fmt.Errorf("lost recovery checkpoint: %+v", item)
		}
		if item.Attempts < 3 {
			return "existing evidence", fmt.Errorf("provider unavailable")
		}
		return "recovered", nil
	})
	q := queueForTest(t, m, 1, "summarize project")
	// Older retries may already have archived conversations. The current one
	// must take precedence, and the archive must remain unchanged.
	q.Items[0].Generation = 3
	q.Items[0].PreviousConversations = []string{"older-conversation"}
	if err := m.save(&q); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		if err := m.Start(q.ID); err != nil {
			t.Fatal(err)
		}
		status := "completed_with_errors"
		if attempt == 3 {
			status = "completed"
		}
		done := waitQueue(t, m, q.ID, status)
		if done.Items[0].Attempts != attempt || created.Load() != 1 || !reflect.DeepEqual(done.Items[0].PreviousConversations, []string{"older-conversation"}) {
			t.Fatalf("retry created another conversation or lost history: %+v", done.Items[0])
		}
		if attempt < 3 {
			if err := m.Retry(q.ID, q.Items[0].ID); err != nil {
				t.Fatal(err)
			}
			pending := waitQueue(t, m, q.ID, "paused")
			if pending.Items[0].Result != "existing evidence" || pending.Items[0].ConversationID != done.Items[0].ConversationID {
				t.Fatal("retry discarded evidence or conversation")
			}
		}
	}
}
