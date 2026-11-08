package ai

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/pkg/aibatch"
	"hephaestus-backend/pkg/database"
)

func TestAIBatchRetryFailedActionReportsEachQueue(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	var calls atomic.Int32
	manager, err := aibatch.New(db, func(_ context.Context, _ aibatch.Queue, item aibatch.Item, _ aibatch.Checkpoint) (string, error) {
		calls.Add(1)
		if item.Instruction != "failed" {
			t.Error("retry executed a completed item")
		}
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := manager.Create(aibatch.Queue{Name: "failed", ProjectID: "fixture", Concurrency: 1}, []string{"failed", "success"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&aibatch.Queue{}).Where("id = ?", q.ID).Update("status", "completed_with_errors").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&aibatch.Item{}).Where("id = ?", q.Items[0].ID).Update("status", "failed").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&aibatch.Item{}).Where("id = ?", q.Items[1].ID).Update("status", "completed").Error; err != nil {
		t.Fatal(err)
	}
	aiBatchRuntime.Lock()
	old := aiBatchRuntime.manager
	aiBatchRuntime.manager = manager
	aiBatchRuntime.Unlock()
	defer func() { aiBatchRuntime.Lock(); aiBatchRuntime.manager = old; aiBatchRuntime.Unlock() }()
	r := gin.New()
	r.POST("/actions", actionAIBatchQueues)
	body, _ := json.Marshal(map[string]any{"ids": []string{q.ID, "missing", q.ID}, "action": "retry_failed"})
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("POST", "/actions", strings.NewReader(string(body))))
	var payload struct {
		Success bool              `json:"success"`
		Data    map[string]string `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || !payload.Success || payload.Data[q.ID] != "" || payload.Data["missing"] == "" {
		t.Fatal(response.Body.String())
	}
	actual, err := manager.Get(q.ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for actual.Status != "completed" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		actual, err = manager.Get(q.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if actual.Status != "completed" || actual.Items[0].Status != "completed" || actual.Items[0].Generation != 1 || actual.Items[1].Status != "completed" || actual.Items[1].Attempts != 0 || calls.Load() != 1 {
		t.Fatalf("unexpected retry result: %+v", actual)
	}
}

func TestAIBatchRetryRetainsNativeScanDiscovery(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	manager, err := aibatch.New(db, func(context.Context, aibatch.Queue, aibatch.Item, aibatch.Checkpoint) (string, error) {
		t.Error("retry started execution")
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := manager.Create(aibatch.Queue{Name: "retry", ProjectID: "fixture", Concurrency: 1}, []string{"summarize project"})
	if err != nil {
		t.Fatal(err)
	}
	item := q.Items[0]
	config, _ := json.Marshal(map[string]any{"aiBatchItemId": batchItemGeneration(item)})
	// This completed scan was persisted before its ID could be checkpointed.
	// Recovery must still find it by the generation marker and leave it alone.
	native := database.VulnScanTask{ID: "completed-before-checkpoint", Version: 1, TaskName: "fixture", ScanType: "webscan", ScanMode: "local", Status: "completed", Config: string(config)}
	if err := db.Create(&native).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&aibatch.Item{}).Where("id = ?", item.ID).Updates(map[string]any{"status": "failed", "conversation_id": "existing-conversation"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.Retry(q.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := manager.Get(q.ID)
	if err != nil {
		t.Fatal(err)
	}
	modules, err := batchModules(db, updated.Items[0])
	if err != nil || len(modules) != 1 || modules[0].ID != native.ID {
		t.Fatalf("lost native scan after retry: %+v, %v", modules, err)
	}
	if err := resumeAIBatchModules(context.Background(), db, updated.Items[0]); err != nil {
		t.Fatal(err)
	}
	var actual database.VulnScanTask
	if err := db.First(&actual, "id = ?", native.ID).Error; err != nil {
		t.Fatal(err)
	}
	if actual.Status != "completed" || actual.Version != 1 {
		t.Fatalf("completed scan restarted: %+v", actual)
	}
}
