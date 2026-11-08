package ai

import (
	"context"
	"testing"

	"hephaestus-backend/lib/pentest"
)

func TestPentestSnapshotIncludesDurableTodoPlan(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "todo-snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "todo", Target: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.SaveTodoPlan(ctx, project.ID, conv.ID, "检查目标", "agent", []pentest.TodoItem{{ID: "one", Title: "收集接口", Status: "in_progress"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := buildPentestSessionSnapshot(ctx, store, conv.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	plan, ok := snapshot["todoPlan"].(*pentest.TodoPlan)
	if !ok || plan.Goal != "检查目标" || len(plan.Items) != 1 || plan.Items[0].Status != "in_progress" {
		t.Fatalf("snapshot todo plan=%+v", snapshot["todoPlan"])
	}
}
