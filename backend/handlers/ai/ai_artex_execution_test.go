package ai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine"
	"hephaestus-backend/lib/pentest/taskengine/db"
)

// TestARTEXPlanItemExecutionProjection covers the Plan drawer's result summary
// and execution window end to end: the native activity stream feeds the plan
// item projection instead of leaving those fields empty.
func TestARTEXPlanItemExecutionProjection(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "orchestration.db")
		testTestARTEXPlanItemExecutionProjection(t, func(ctx context.Context, dir string) (*taskengine.Runtime, error) {
			return taskengine.OpenSQLite(ctx, path, dir)
		})
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("ARTEX_TEST_DSN")
		if dsn == "" {
			t.Skip("ARTEX_TEST_DSN required (isolated PostgreSQL)")
		}
		testTestARTEXPlanItemExecutionProjection(t, func(ctx context.Context, dir string) (*taskengine.Runtime, error) {
			return taskengine.Open(ctx, dsn, dir)
		})
	})
}
func testTestARTEXPlanItemExecutionProjection(t *testing.T, open func(context.Context, string) (*taskengine.Runtime, error)) {
	ctx := context.Background()
	rt, err := open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := rt.Close(c); err != nil {
			t.Error(err)
		}
	}()
	profile := db.LLMProfile{Name: "execution fixture", Format: "openai", BaseURL: "http://127.0.0.1:1/v1", APIKey: "fixture", Model: "fixture", PoolExclude: true}
	task, err := rt.Engine.PrepareHostTask(taskengine.HostTaskOptions{Name: "plan item execution", Planner: profile, Worker: profile})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rt.Engine.DeleteHostTask(ctx, task.ID); err != nil {
			t.Error(err)
		}
	}()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "plan item execution"})
	if err != nil {
		t.Fatal(err)
	}
	run := pentest.PlanRun{ID: pentest.OrchestrationRunPrefix + task.ID, ProjectID: project.ID, Objective: "Offline execution projection"}
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
	intent, err := task.Store.AddNode("intent", map[string]any{"summary": "execution fixture"}, 5, "running", "planner", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, activity := range []db.Activity{
		{NodeID: &intent, Worker: "work#1", Kind: "tool_use", Tool: "fixture", ToolUseID: "fixture-call", Detail: `{"fixture":true}`},
		{NodeID: &intent, Worker: "work#1", Kind: "result", Summary: "fixture summary", Detail: "fixture summary\n- confirmed asset coverage"},
	} {
		if _, err = task.Store.AppendActivity(activity); err != nil {
			t.Fatal(err)
		}
	}
	changed, err := task.Store.CompareAndSetIntentState(intent, "running", "done")
	if err != nil || !changed {
		t.Fatalf("intent did not finish: changed=%v err=%v", changed, err)
	}
	// A second direction ended without a persisted native reason; the projection
	// must still explain the blocked badge instead of leaving the drawer empty.
	blockedIntent, err := task.Store.AddNode("intent", map[string]any{"summary": "blocked fixture"}, 3, "running", "planner", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = task.Store.AppendActivity(db.Activity{NodeID: &blockedIntent, Worker: "work#2", Kind: "result", IsError: true, Summary: "执行出错: fixture failure", Detail: "执行出错: fixture failure"}); err != nil {
		t.Fatal(err)
	}
	changed, err = task.Store.CompareAndSetIntentState(blockedIntent, "running", "blocked")
	if err != nil || !changed {
		t.Fatalf("blocked intent did not settle: changed=%v err=%v", changed, err)
	}
	if err = syncARTEXRun(ctx, store, rt, run); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListPlanItems(ctx, run.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	var item, blockedItem pentest.PlanItem
	for _, value := range items {
		if value.ID == nativeNodeID(run.ID, intent) {
			item = value
		}
		if value.ID == nativeNodeID(run.ID, blockedIntent) {
			blockedItem = value
		}
	}
	if item.Status != "completed" || item.ResultSummary != "fixture summary\n- confirmed asset coverage" {
		t.Fatalf("result summary not projected: %+v", item)
	}
	if item.StartedAt == nil || item.CompletedAt == nil {
		t.Fatalf("execution window not projected: %+v", item)
	}
	if item.CompletedAt.Before(*item.StartedAt) {
		t.Fatalf("execution window inverted: %+v", item)
	}
	if blockedItem.Status != "blocked" || !strings.Contains(blockedItem.Error, "错误详情见结果摘要") {
		t.Fatalf("blocked reason not projected: %+v", blockedItem)
	}
}

func TestIntentTerminalReasonForPlan(t *testing.T) {
	cases := []struct {
		name      string
		state     string
		reason    string
		execution db.IntentExecution
		want      string
	}{
		{name: "persisted reason wins", state: "blocked", reason: "Worker 执行失败：provider 502", want: "provider 502"},
		{name: "legacy quota code", state: "blocked", reason: db.IntentBlockedLLMQuota, want: "配额"},
		{name: "blocked with error output", state: "blocked", execution: db.IntentExecution{ResultFailed: true}, want: "错误详情"},
		{name: "blocked without output", state: "blocked", want: "未记录具体错误"},
		{name: "exhausted", state: "exhausted", want: "运行预算上限"},
		{name: "running", state: "running", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := intentTerminalReasonForPlan(tc.state, tc.reason, tc.execution)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("unexpected reason %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("reason %q does not contain %q", got, tc.want)
			}
		})
	}
}
