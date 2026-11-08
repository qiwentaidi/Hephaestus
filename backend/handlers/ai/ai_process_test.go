package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/airun"
)

func TestAIProcessPersistenceCancellationAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "process.db")
	open := func() (*sql.DB, *pentest.SQLStore) {
		db, err := sql.Open("sqlite3", path)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		store := pentest.NewSQLStore(db)
		if err := store.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		return db, store
	}
	db, store := open()
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "fixture", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("完整证据说明。", 150)
	runCtx, cancel := context.WithCancel(ctx)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunk, _ := json.Marshal(gin.H{"choices": []gin.H{{"index": 0, "delta": gin.H{"reasoning_content": long}}}})
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	record := recordPentestAgentProgress(store, conv.ID)
	_, err = pentest.RunAgentLoop(runCtx, pentest.AgentLoopConfig{BaseURL: server.URL, APIKey: "fixture", Model: "fixture", Project: project, Conversation: conv, Store: store, Progress: func(e pentest.AgentLoopEvent) {
		record(e)
		if e.Type == "reasoning" && e.Status == "running" {
			cancel()
		}
	}}, "Inspect fixture", nil)
	server.Close()
	cancel()
	if err == nil {
		t.Fatal("expected cancellation")
	}
	_ = airun.MarkConversationStatus(ctx, store, conv.ID, "cancelled")
	_ = db.Close()
	db, store = open()
	defer db.Close()
	messages, err := store.ListMessages(ctx, conv.ID, -1)
	if err != nil {
		t.Fatal(err)
	}
	blocks := 0
	oldRun := ""
	for _, m := range messages {
		var p pentestProgressPayload
		_ = json.Unmarshal([]byte(m.Content), &p)
		if p.Type == "reasoning" {
			blocks++
			oldRun = p.RunID
			if p.Status != "cancelled" || p.Detail != long {
				t.Fatal("partial text/status did not survive restart")
			}
		}
	}
	if blocks != 1 || oldRun == "" {
		t.Fatalf("blocks=%d run=%q", blocks, oldRun)
	}
	items, turns := buildPentestChatForStatus(messages, "cancelled")
	if len(turns) != 1 {
		t.Fatalf("turns=%d", len(turns))
	}
	found := false
	for _, item := range items {
		if item["description"] == long {
			found = true
		}
	}
	if !found {
		t.Fatal("full process text missing from snapshot")
	}
	// Resume in a fresh run against the same durable conversation. No real tools.
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","reasoning_content":"New run analysis.","content":"Final report."}}]}`)
	}))
	defer plain.Close()
	_, err = pentest.RunAgentLoop(ctx, pentest.AgentLoopConfig{BaseURL: plain.URL, APIKey: "fixture", Model: "fixture", Project: project, Conversation: conv, Store: store, Progress: recordPentestAgentProgress(store, conv.ID)}, "Continue", nil)
	if err != nil {
		t.Fatal(err)
	}
	messages, _ = store.ListMessages(ctx, conv.ID, -1)
	items, turns = buildPentestChatForStatus(messages, "completed")
	if len(turns) != 2 {
		t.Fatalf("resume mixed turns: %d", len(turns))
	}
	oldCount, newCount, finalCount := 0, 0, 0
	for _, item := range items {
		if item["description"] == long {
			oldCount++
		}
		if item["description"] == "New run analysis." {
			newCount++
			if item["runId"] == oldRun {
				t.Fatal("resume reused previous run id")
			}
		}
		if item["description"] == "Final report." {
			t.Fatal("final text duplicated as process content")
		}
		if item["type"] == "message" {
			if m, ok := item["message"].(gin.H); ok && m["content"] == "Final report." {
				finalCount++
			}
		}
	}
	if oldCount != 1 || newCount != 1 || finalCount != 1 {
		t.Fatalf("old=%d new=%d final=%d", oldCount, newCount, finalCount)
	}
}

func TestAIProcessUpsertPreservesOrderingAndBoundsSnapshot(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, _ := store.UpsertProject(ctx, pentest.Project{Name: "fixture"})
	conv, _ := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "fixture", Status: "running"})
	_, _ = store.AddMessage(ctx, pentest.Message{ConversationID: conv.ID, Role: "user", Content: "fixture"})
	record := recordPentestAgentProgress(store, conv.ID)
	record(pentest.AgentLoopEvent{Type: "reasoning", BlockID: "block", Message: "first", Status: "running", Iteration: 1})
	for i := 0; i < 510; i++ {
		record(pentest.AgentLoopEvent{Type: "synthesis", Iteration: i + 1})
	}
	record(pentest.AgentLoopEvent{Type: "reasoning", BlockID: "block", Message: "complete", Status: "completed", Iteration: 1})
	messages, err := store.ListMessages(ctx, conv.ID, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 512 {
		t.Fatalf("lost history or duplicated block: %d", len(messages))
	}
	if messages[1].ID != "block" {
		t.Fatal("updating a block moved it after later tool events")
	}
	snapshot, err := buildPentestSessionSnapshot(ctx, store, conv.ID, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	// The snapshot is deliberately bounded (see snapshotProgressMessages): a long
	// run must not ship its whole progress history on every poll. Ordering and
	// completeness are asserted above against ListMessages, the unbounded source.
	items := snapshot["chatItems"].([]gin.H)
	if len(items) == 0 {
		t.Fatal("snapshot lost every round")
	}
	if len(items) > snapshotProgressMessages+8 {
		t.Fatalf("snapshot is not bounded: %d", len(items))
	}
}

func TestAIProcessSkipsIterationEvents(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, _ := store.UpsertProject(ctx, pentest.Project{Name: "fixture"})
	conv, _ := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "fixture", Status: "running"})
	record := recordPentestAgentProgress(store, conv.ID)
	record(pentest.AgentLoopEvent{Type: "iteration", Iteration: 1})
	record(pentest.AgentLoopEvent{Type: "iteration", Iteration: 2})
	messages, err := store.ListMessages(ctx, conv.ID, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("iteration events persisted: %d", len(messages))
	}
}

func TestAIProcessTerminalNormalizationPreservesBody(t *testing.T) {
	payload, _ := json.Marshal(pentestProgressPayload{Type: "reasoning", Label: "推理过程", Detail: "Retained text", Status: "running"})
	item := pentestProgressChatItem(pentest.Message{ID: "block", Content: string(payload), CreatedAt: time.Now()})
	normalized := normalizePentestTerminalChatItem(item, "cancelled")
	if normalized["marker"].(gin.H)["description"] != "Retained text" {
		t.Fatal("terminal normalization erased analysis")
	}
}
