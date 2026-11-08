package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hephaestus-backend/lib/pentest"
)

func TestSQLiteLocalWorkerFinishesFailedTurn(t *testing.T) {
	pentest.EnableLocalWorker()
	defer pentest.DisableLocalWorker()
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, conv, err := createPentestConversationWithUserMessage(ctx, store, pentest.Project{Name: "sqlite-worker"}, pentest.Conversation{Mode: pentest.SoloPentestMode, Status: "running"}, "run")
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := store.LatestUserMessageID(ctx, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnqueueWorkerTurn(ctx, conv.ID, messageID, "", ""); err != nil {
		t.Fatal(err)
	}
	turn, err := store.ClaimWorkerTurn(ctx, "local-worker", 30*time.Second)
	if err != nil || turn == nil {
		t.Fatalf("claim = %#v, err=%v", turn, err)
	}
	executeAIWorkerTurnWithBuilder(ctx, store, *turn, true, "", nil, func(context.Context, *pentest.SQLStore, pentest.WorkerTurn) (pentest.AgentLoopConfig, error) {
		return pentest.AgentLoopConfig{}, errors.New("fixture builder failure")
	})
	current, err := store.LatestWorkerTurn(ctx, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "failed" {
		t.Fatalf("turn status = %q, want failed", current.Status)
	}
	_ = project
}

func TestSQLiteLocalWorkerRunsTurnToCompletion(t *testing.T) {
	pentest.EnableLocalWorker()
	defer pentest.DisableLocalWorker()
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, conv, err := createPentestConversationWithUserMessage(ctx, store, pentest.Project{Name: "sqlite-worker-run"}, pentest.Conversation{Mode: pentest.SoloPentestMode, Status: "running"}, "run")
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := store.LatestUserMessageID(ctx, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnqueueWorkerTurn(ctx, conv.ID, messageID, "", ""); err != nil {
		t.Fatal(err)
	}
	turn, err := store.ClaimWorkerTurn(ctx, "local-worker", 30*time.Second)
	if err != nil || turn == nil {
		t.Fatalf("claim = %#v, err=%v", turn, err)
	}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"finished"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`))
	}))
	defer model.Close()
	executeAIWorkerTurnWithBuilder(ctx, store, *turn, true, "", nil, func(_ context.Context, s *pentest.SQLStore, _ pentest.WorkerTurn) (pentest.AgentLoopConfig, error) {
		return pentest.AgentLoopConfig{
			Store: s, Project: project, Conversation: conv, BaseURL: model.URL, APIKey: "fixture", Model: "fixture",
			WorkDir: t.TempDir(), HTTPClient: model.Client(), ToolCatalog: &pentest.SystemToolCatalog{}, DisableModelStreaming: true,
		}, nil
	})
	current, err := store.LatestWorkerTurn(ctx, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "completed" {
		t.Fatalf("turn status = %q, want completed", current.Status)
	}
	messages, err := store.ListMessages(ctx, conv.ID, -1)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range messages {
		if message.Role == "assistant" && message.Content == "finished" {
			found = true
		}
	}
	if !found {
		t.Fatalf("assistant answer missing: %#v", messages)
	}
}
