package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine"
	"hephaestus-backend/lib/pentest/taskengine/db"
)

// Exercise real Worker/MainAgent cancellation and native restart recovery, then
// project each confirmed state through the same path consumed by the task UI.
func TestARTEXPauseResumeProjectionLifecycle(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "orchestration.db")
		testTestARTEXPauseResumeProjectionLifecycle(t, func(ctx context.Context, dir string) (*taskengine.Runtime, error) {
			return taskengine.OpenSQLite(ctx, path, dir)
		})
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("ARTEX_TEST_DSN")
		if dsn == "" {
			t.Skip("ARTEX_TEST_DSN required (isolated PostgreSQL)")
		}
		testTestARTEXPauseResumeProjectionLifecycle(t, func(ctx context.Context, dir string) (*taskengine.Runtime, error) {
			return taskengine.Open(ctx, dsn, dir)
		})
	})
}
func testTestARTEXPauseResumeProjectionLifecycle(t *testing.T, open func(context.Context, string) (*taskengine.Runtime, error)) {
	entered := make(chan string, 32)
	cancelled := make(chan string, 32)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		entered <- req.Model
		<-r.Context().Done()
		cancelled <- req.Model
	}))
	defer fake.Close()
	ctx := context.Background()
	dir := t.TempDir()
	rt, err := open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if rt != nil {
			c, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := rt.Close(c); err != nil {
				t.Error(err)
			}
		}
	}()
	profile := db.LLMProfile{Name: "pause lifecycle", Format: "openai", BaseURL: fake.URL + "/v1", APIKey: "fixture", Model: "planner", PoolExclude: true}
	worker := profile
	worker.Model = "worker"
	task, err := rt.Engine.PrepareHostTask(taskengine.HostTaskOptions{Name: "Pause fixture", Goal: "Offline lifecycle fixture", Planner: profile, Worker: worker, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	main := profile
	main.Model = "mainagent"
	if err = rt.Engine.ConfigureHostChatProfile(task.ID, "fixture", main); err != nil {
		t.Fatal(err)
	}
	preserved := map[int64]string{}
	for _, node := range []struct{ kind, state, summary string }{
		{"goal", "open", "Offline lifecycle fixture"},
		{"intent", "done", "Previously completed direction"},
		{"fact", "confirmed", "Previously observed evidence"},
		{"intent", "open", "Unfinished direction"},
	} {
		id, err := task.Store.AddNode(node.kind, map[string]any{"summary": node.summary}, 5, node.state, "fixture", nil)
		if err != nil {
			t.Fatal(err)
		}
		if node.state == "done" || node.kind == "fact" {
			saved, err := task.Store.GetNode(id)
			if err != nil {
				t.Fatal(err)
			}
			preserved[id] = saved.State + ":" + string(saved.Payload)
		}
	}
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "Pause lifecycle fixture"})
	if err != nil {
		t.Fatal(err)
	}
	run := pentest.PlanRun{ID: pentest.OrchestrationRunPrefix + task.ID, ProjectID: project.ID, Objective: task.Goal}
	run.PlannerConversationID = nativeConversation(run.ID, "planner")
	for _, role := range []string{"planner", "controller"} {
		id := nativeConversation(run.ID, role)
		if _, err = store.UpsertConversation(ctx, pentest.Conversation{ID: id, ProjectID: project.ID, Mode: pentest.SoloPentestMode}); err != nil {
			t.Fatal(err)
		}
		if err = store.SetAgentRole(ctx, id, role, "", run.ID); err != nil {
			t.Fatal(err)
		}
	}
	run, err = store.CreatePlanRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce the historical false cancellation and its terminal timestamp.
	if err = store.UpdatePlanRunStatus(ctx, run.ID, "cancelled", "Existing summary", ""); err != nil {
		t.Fatal(err)
	}
	run, err = store.GetPlanRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	projectState := func(want string) {
		t.Helper()
		current, _, err := rt.Engine.HostTask(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		for id, original := range preserved {
			saved, err := current.Store.GetNode(id)
			if err != nil || saved.State+":"+string(saved.Payload) != original {
				t.Fatalf("completed result/evidence %d changed: %+v %v", id, saved, err)
			}
		}
		if err := syncARTEXRun(ctx, store, rt, run); err != nil {
			t.Fatal(err)
		}
		run, err = store.GetPlanRun(ctx, run.ID)
		if err != nil || run.Status != want || run.CompletedAt != nil || run.Summary != "Existing summary" {
			t.Fatalf("run projection: %+v %v", run, err)
		}
		items, err := store.ListPlanItems(ctx, run.ID)
		if err != nil || len(items) != 2 {
			t.Fatalf("items: %+v %v", items, err)
		}
		done, paused := 0, 0
		for _, item := range items {
			if item.Status == "completed" {
				done++
			}
			if item.Status == "paused" {
				paused++
			}
		}
		if done != 1 || (want == "paused" && paused != 1) {
			t.Fatalf("completed work changed or pause missing: %+v", items)
		}
		if want == "paused" {
			planner, err := store.GetConversation(ctx, run.PlannerConversationID)
			if err != nil || planner.Status != "paused" {
				t.Fatalf("planner projection: %+v %v", planner, err)
			}
			for _, item := range items {
				if item.Status == "paused" {
					conv, err := store.GetConversation(ctx, item.ChildConversationID)
					if err != nil || conv.Status != "paused" {
						t.Fatalf("worker projection: %+v %v", conv, err)
					}
				}
			}
		}
	}
	projectState("paused")
	waitModels := func(ch <-chan string, models ...string) {
		t.Helper()
		pending := map[string]bool{}
		for _, model := range models {
			pending[model] = true
		}
		timer := time.NewTimer(15 * time.Second)
		defer timer.Stop()
		for len(pending) > 0 {
			select {
			case model := <-ch:
				delete(pending, model)
			case <-timer.C:
				t.Fatalf("model lifecycle acknowledgement missing: %v", pending)
			}
		}
	}
	for cycle := 0; cycle < 2; cycle++ {
		if err = rt.Engine.ResumeHostTask(task.ID); err != nil {
			t.Fatal(err)
		}
		if err = rt.Engine.HostChat(task.ID, "Wait for the local fixture"); err != nil {
			t.Fatal(err)
		}
		waitModels(entered, "worker", "mainagent")
		projectState("planning")
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = rt.Engine.PauseHostTask(c, task.ID)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		waitModels(cancelled, "worker", "mainagent")
		_, state, err := rt.Engine.HostTask(task.ID)
		if err != nil || state.Status != "paused" || !rt.Engine.HostTaskDrained(task.ID) {
			t.Fatalf("unconfirmed pause: %+v %v", state, err)
		}
		projectState("paused")
		// Restart must preserve the intent and remain drained until explicit resume.
		c, cancel = context.WithTimeout(ctx, 10*time.Second)
		err = rt.Close(c)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		rt = nil
		rt, err = open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		_, state, err = rt.Engine.HostTask(task.ID)
		if err != nil || state.Status != "paused" || !rt.Engine.HostTaskDrained(task.ID) {
			t.Fatalf("restart admitted paused task: %+v %v", state, err)
		}
		projectState("paused")
	}
	if err = rt.Engine.DeleteHostTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
}
