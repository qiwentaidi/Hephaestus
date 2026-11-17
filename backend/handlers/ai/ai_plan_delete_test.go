package ai

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"hephaestus-backend/lib/pentest"

	_ "github.com/mattn/go-sqlite3"
)

func openPlanDeleteStore(t *testing.T) *pentest.SQLStore {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "plan-delete.db")+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := pentest.NewSQLStoreWithDialect(db, "sqlite")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestDeletePlanRunDataRemovesRunSessionsAndKeepsProject(t *testing.T) {
	ctx := context.Background()
	store := openPlanDeleteStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "plan-delete"})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "planner", Mode: pentest.SoloPentestMode, Status: "paused"})
	if err != nil {
		t.Fatal(err)
	}
	controller, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "controller", Mode: pentest.SoloPentestMode, Status: "paused"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "worker", Mode: pentest.SoloPentestMode, Status: "paused"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreatePlanRun(ctx, pentest.PlanRun{
		ID: "orchestration:1", ProjectID: project.ID, PlannerConversationID: planner.ID,
		Objective: "delete plan", Status: "paused", MaxWorkers: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []struct {
		id   string
		kind string
	}{
		{planner.ID, pentest.AgentRolePlanner},
		{controller.ID, pentest.AgentRoleController},
		{worker.ID, pentest.AgentRoleWorker},
	} {
		if err := store.SetAgentRole(ctx, role.id, role.kind, planner.ID, run.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.CreatePlanItem(ctx, pentest.PlanItem{
		RunID: run.ID, Seq: 1, BatchNo: 1, Title: "worker item", Instruction: "work",
		Status: "completed", ChildConversationID: worker.ID,
	}); err != nil {
		t.Fatal(err)
	}
	sessionsDir := t.TempDir()
	for _, id := range []string{planner.ID, controller.ID, worker.ID} {
		path := filepath.Join(sessionsDir, id, "marker.txt")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(id), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var deletedRuns []string
	ids, err := deletePlanRunData(ctx, store, sessionsDir, run, func(_ context.Context, runIDs []string) error {
		deletedRuns = append(deletedRuns, runIDs...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(deletedRuns) != 1 || deletedRuns[0] != run.ID {
		t.Fatalf("deleteTasks calls = %#v, want [%s]", deletedRuns, run.ID)
	}
	if len(ids) != 3 {
		t.Fatalf("conversation ids = %#v, want 3", ids)
	}
	if _, err := store.GetPlanRun(ctx, run.ID); err == nil {
		t.Fatal("plan run was not deleted")
	}
	for _, id := range []string{planner.ID, controller.ID, worker.ID} {
		if _, err := store.GetConversation(ctx, id); err == nil {
			t.Fatalf("conversation %s was not deleted", id)
		}
		if _, err := os.Stat(filepath.Join(sessionsDir, id)); !os.IsNotExist(err) {
			t.Fatalf("session directory %s was not deleted: %v", id, err)
		}
	}
	if _, err := store.GetProject(ctx, project.ID); err != nil {
		t.Fatalf("project was deleted: %v", err)
	}
}

func TestDeletePlanRunDataRejectsActiveRun(t *testing.T) {
	ctx := context.Background()
	store := openPlanDeleteStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "active"})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "planner", Mode: pentest.SoloPentestMode, Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreatePlanRun(ctx, pentest.PlanRun{
		ID: "orchestration:2", ProjectID: project.ID, PlannerConversationID: planner.ID,
		Objective: "active", Status: "planning", MaxWorkers: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	_, err = deletePlanRunData(ctx, store, t.TempDir(), run, func(context.Context, []string) error {
		called++
		return nil
	})
	if !errors.Is(err, errPlanRunNotDeletable) {
		t.Fatalf("expected active-run error, got %v", err)
	}
	if called != 0 {
		t.Fatal("deleteTasks was called for an active run")
	}
	if _, err := store.GetPlanRun(ctx, run.ID); err != nil {
		t.Fatalf("active run was removed: %v", err)
	}
}

func TestDeletePlanRunDataRejectsLegacyRun(t *testing.T) {
	ctx := context.Background()
	store := openPlanDeleteStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "planner", Mode: pentest.SoloPentestMode, Status: "paused"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreatePlanRun(ctx, pentest.PlanRun{
		ID: "legacy-run", ProjectID: project.ID, PlannerConversationID: planner.ID,
		Objective: "legacy", Status: "paused", MaxWorkers: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.GetPlanRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = deletePlanRunData(ctx, store, t.TempDir(), run, func(context.Context, []string) error { return nil })
	if !errors.Is(err, pentest.ErrLegacyOrchestrationReadOnly) {
		t.Fatalf("expected legacy error, got %v", err)
	}
}
