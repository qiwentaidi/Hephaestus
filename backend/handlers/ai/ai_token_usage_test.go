package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/handlers/dashboard"
	"hephaestus-backend/lib/pentest"
)

func TestPentestTokenUsageSnapshotAndSidebar(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "usage test"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "session", Mode: pentest.SoloPentestMode, Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordTokenUsage(ctx, "req-1", conv.ID, 12345); err != nil {
		t.Fatal(err)
	}
	snapshot, err := buildPentestSessionSnapshot(ctx, store, conv.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["totalTokens"] != int64(12345) {
		t.Fatalf("snapshot tokens=%v", snapshot["totalTokens"])
	}
	list, err := buildPentestSessionSummaries(ctx, store, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0]["totalTokens"] != snapshot["totalTokens"] {
		t.Fatalf("sidebar=%v", list)
	}
	total, err := store.TotalTokenUsage(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(dashboard.DashboardTasksResponse{AITotalTokens: &total})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["aiTotalTokens"] != float64(12345) {
		t.Fatalf("dashboard=%s", data)
	}
}

func TestPentestSessionSummariesArePagedAndLightweight(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "paged project"})
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"first", "second", "third"} {
		conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: title, Target: title + ".example", Mode: pentest.SoloPentestMode, Status: "completed"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.AddMessage(ctx, pentest.Message{ConversationID: conv.ID, Role: "user", Content: "initial " + title}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.AddMessage(ctx, pentest.Message{ConversationID: conv.ID, Role: "tool", Content: strings.Repeat("large evidence ", 1000)}); err != nil {
			t.Fatal(err)
		}
	}
	firstPage, total, err := buildPentestSessionSummariesPage(ctx, store, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	secondPage, secondTotal, err := buildPentestSessionSummariesPage(ctx, store, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || secondTotal != 3 || len(firstPage) != 2 || len(secondPage) != 1 {
		t.Fatalf("unexpected pages: first=%d second=%d total=%d secondTotal=%d", len(firstPage), len(secondPage), total, secondTotal)
	}
	firstTask := firstPage[0]["task"].(gin.H)
	secondTask := secondPage[0]["task"].(gin.H)
	if firstTask["id"] == secondTask["id"] || firstPage[0]["project"].(gin.H)["name"] != project.Name {
		t.Fatalf("incorrect summary metadata: first=%v second=%v", firstTask, secondTask)
	}
	if items := firstPage[0]["recentChatItems"].([]gin.H); len(items) != 0 {
		t.Fatalf("list included detailed chat items: %v", items)
	}
}

func TestPentestSnapshotAttributesUsageToEachTurn(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "turn usage"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "session", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AddMessage(ctx, pentest.Message{ConversationID: conv.ID, Role: "user", Content: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AddMessage(ctx, pentest.Message{ConversationID: conv.ID, Role: "user", Content: "second"})
	if err != nil {
		t.Fatal(err)
	}
	// A late first-turn response still belongs to the first turn.
	if err := store.RecordTurnTokenUsage(ctx, "req-first", conv.ID, first.ID, 100); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordTurnTokenUsage(ctx, "req-second", conv.ID, second.ID, 250); err != nil {
		t.Fatal(err)
	}
	snapshot, err := buildPentestSessionSnapshot(ctx, store, conv.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	turns := snapshot["turns"].([]gin.H)
	if len(turns) != 2 || turns[0]["totalTokens"] != int64(100) || turns[1]["totalTokens"] != int64(250) {
		t.Fatalf("turns=%v", turns)
	}
	if snapshot["totalTokens"] != int64(350) {
		t.Fatalf("session=%v", snapshot["totalTokens"])
	}
}
