package ai

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine/db"
	"testing"
	"time"
)

func TestOrchestrationArchiveAndQueueBoundary(t *testing.T) {
	pentest.EnableLocalWorker()
	defer pentest.DisableLocalWorker()
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	for _, runID := range []string{"legacy-run", "artex:123"} {
		for _, role := range []string{pentest.AgentRolePlanner, pentest.AgentRoleWorker, pentest.AgentRoleController} {
			id := runID + ":" + role
			_, err = store.UpsertConversation(ctx, pentest.Conversation{ID: id, ProjectID: project.ID, Mode: pentest.SoloPentestMode, Status: "completed"})
			if err != nil {
				t.Fatal(err)
			}
			if err = store.SetAgentRole(ctx, id, role, "", runID); err != nil {
				t.Fatal(err)
			}
			if _, err = store.EnqueueWorkerMessage(ctx, pentest.Message{ConversationID: id, Content: "must not run"}, "", ""); err == nil {
				t.Fatalf("%s entered old queue", id)
			}
			if _, err = store.EnqueueWorkerTurn(ctx, id, "old-message", "", ""); err == nil {
				t.Fatalf("%s reentered old queue", id)
			}
			if runID == "legacy-run" {
				if !errors.Is(store.RejectOrchestrationTurn(ctx, id), pentest.ErrLegacyOrchestrationReadOnly) {
					t.Fatal("archive rejection missing")
				}
				snap, err := buildPentestSessionSnapshot(ctx, store, id, "", "")
				if err != nil {
					t.Fatal(err)
				}
				if snap["task"].(gin.H)["readOnly"] != true {
					t.Fatal("archive is writable in snapshot")
				}
			}
		}
	}
	if err = store.RetireLegacyOrchestration(ctx); err != nil {
		t.Fatal(err)
	}
	conv, err := store.GetConversation(ctx, "legacy-run:controller")
	if err != nil || conv.Status != "completed" {
		t.Fatalf("history status was rewritten: %+v %v", conv, err)
	}
	solo, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Mode: pentest.SoloPentestMode, Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnqueueWorkerMessage(ctx, pentest.Message{ConversationID: solo.ID, Content: "independent dialogue"}, "", ""); err != nil {
		t.Fatalf("solo changed: %v", err)
	}
}

func TestARTEXGraphAndActivityProjection(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "native"})
	if err != nil {
		t.Fatal(err)
	}
	run := pentest.PlanRun{ID: "artex:99", Objective: "Verify fixture", ProjectID: project.ID, PlannerConversationID: "artex:99:planner", ControllerConversationID: "artex:99:controller", MaxWorkers: 1}
	for _, id := range []string{run.PlannerConversationID, run.ControllerConversationID, "artex:99:intent:1"} {
		if _, err = store.UpsertConversation(ctx, pentest.Conversation{ID: id, ProjectID: project.ID, Mode: pentest.SoloPentestMode}); err != nil {
			t.Fatal(err)
		}
	}
	run, err = store.CreatePlanRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	nodes := []pentest.ARTEXNode{
		{ID: "artex:99:node:1", Kind: "intent", Title: "Verify fixture", State: "completed", OwnerConversation: "artex:99:intent:1", Priority: 5},
		{ID: "artex:99:node:2", Kind: "goal", Title: "Fixture verified", State: "met", OwnerConversation: run.PlannerConversationID},
		{ID: "artex:99:node:3", Kind: "fact", Title: "Observed fixture", OwnerConversation: "artex:99:intent:1", Payload: map[string]any{"confidence": "observed"}},
		{ID: "artex:99:node:4", Kind: "finding", Title: "Fixture finding", OwnerConversation: "artex:99:intent:1", Payload: map[string]any{"severity": "low"}},
	}
	relations := []pentest.ExplorationRelation{{Source: "intent:artex:99:node:1", Target: "fact:artex:99:node:3", Relation: "yields"}, {Source: "fact:artex:99:node:3", Target: "goal:artex:99:node:2", Relation: "proves"}}
	for i := 0; i < 2; i++ {
		if err = store.SyncARTEXGraph(ctx, run, nodes, relations); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.ListPlanItems(ctx, run.ID)
	if err != nil || len(items) != 1 || items[0].Status != "completed" {
		t.Fatalf("items: %+v %v", items, err)
	}
	graph, err := store.GetExploration(ctx, project.ID, run.PlannerConversationID, "")
	if err != nil || len(graph.Goals) != 1 {
		t.Fatalf("graph: %+v %v", graph, err)
	}
	in, out := 10, 5
	activity := db.Activity{ID: 1, Kind: "result", Detail: "Native result", Worker: "planner", CreatedAt: time.Now(), InputTokens: &in, OutputTokens: &out}
	for i := 0; i < 2; i++ {
		if err = projectARTEXActivity(ctx, store, run, run.PlannerConversationID, activity); err != nil {
			t.Fatal(err)
		}
	}
	// Usage events are cumulative and must not be counted again alongside result.
	activity.ID = 2
	activity.Kind = "usage"
	if err = projectARTEXActivity(ctx, store, run, run.PlannerConversationID, activity); err != nil {
		t.Fatal(err)
	}
	usage, err := store.TokenUsage(ctx, run.PlannerConversationID)
	if err != nil || usage.TotalTokens != 15 {
		t.Fatalf("usage: %+v %v", usage, err)
	}
	msgs, err := store.ListMessages(ctx, run.PlannerConversationID, -1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("messages duplicated: %+v %v", msgs, err)
	}
}

func TestARTEXWorkerExecutionProjectionKeepsTaskOwnership(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "native execution ownership"})
	if err != nil {
		t.Fatal(err)
	}
	run := pentest.PlanRun{ID: "artex:ownership", Objective: "verify native activity ownership", ProjectID: project.ID, PlannerConversationID: "artex:ownership:planner", ControllerConversationID: "artex:ownership:controller"}
	worker := nativeConversation(run.ID, "intent:1")
	otherWorker := nativeConversation(run.ID, "intent:2")
	for _, id := range []string{run.PlannerConversationID, run.ControllerConversationID, worker, otherWorker} {
		if _, err = store.UpsertConversation(ctx, pentest.Conversation{ID: id, ProjectID: project.ID, Mode: pentest.SoloPentestMode}); err != nil {
			t.Fatal(err)
		}
	}
	run, err = store.CreatePlanRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{worker, otherWorker} {
		if err = store.SetAgentRole(ctx, id, pentest.AgentRoleWorker, run.PlannerConversationID, run.ID); err != nil {
			t.Fatal(err)
		}
	}
	intentID := nativeNodeID(run.ID, 1)
	// Match the already-persisted graph from the failing page: Planner owns the
	// intent, and the plan item assigns its execution to a Worker conversation.
	if err = store.SyncARTEXGraph(ctx, run, []pentest.ARTEXNode{{ID: intentID, Kind: "intent", Title: "fixture", State: "in_progress", OwnerConversation: worker}}, nil); err != nil {
		t.Fatal(err)
	}
	nodeID := int64(1)
	activity := db.Activity{ID: 10, NodeID: &nodeID, Kind: "tool_use", Worker: "work#1", Tool: "fixture", ToolUseID: "call-1", Detail: `{"input":"fixture"}`, CreatedAt: time.Now()}
	for i := 0; i < 2; i++ {
		if err = projectARTEXActivity(ctx, store, run, worker, activity); err != nil {
			t.Fatalf("worker tool start: %v", err)
		}
		result := activity
		result.ID = 11
		result.Kind = "tool_result"
		result.Detail = "fixture result"
		if err = projectARTEXActivity(ctx, store, run, worker, result); err != nil {
			t.Fatalf("worker tool result: %v", err)
		}
	}
	executions, err := store.ListToolExecutionsByConversation(ctx, worker, 10)
	if err != nil || len(executions) != 1 {
		t.Fatalf("executions: %+v %v", executions, err)
	}
	execution := executions[0]
	if execution.IntentID != intentID || execution.Status != "completed" || execution.ResultJSON != "fixture result" {
		t.Fatalf("lost native execution association: %+v", execution)
	}
	graph, err := store.GetExploration(ctx, project.ID, run.PlannerConversationID, "")
	if err != nil || len(graph.Intents) != 1 || len(graph.Executions) != 1 {
		t.Fatalf("Planner graph: %+v %v", graph, err)
	}
	// Generic dialogue writes must retain strict same-conversation ownership.
	execution.ID = "generic-cross-conversation"
	if _, err = store.SaveToolExecution(ctx, execution); err == nil {
		t.Fatal("ordinary execution accepted cross-conversation intent")
	}
	for _, test := range []struct{ name, run, conversation string }{{"wrong worker", run.ID, otherWorker}, {"wrong run", "artex:other", worker}, {"planner cannot impersonate worker", run.ID, run.PlannerConversationID}, {"legacy run", "legacy", worker}} {
		t.Run(test.name, func(t *testing.T) {
			bad := execution
			bad.ID = "rejected:" + test.name
			bad.ConversationID = test.conversation
			if _, err = store.SaveARTEXToolExecution(ctx, test.run, bad); err == nil {
				t.Fatal("unrelated owner accepted")
			}
			if _, err = store.GetToolExecution(ctx, bad.ID); err == nil {
				t.Fatal("failed ownership write was not rolled back")
			}
		})
	}
}

func TestARTEXWorkerTerminalProjection(t *testing.T) {
	for _, tc := range []struct {
		state, parent string
		paused        bool
		conv, intent  string
	}{
		{"open", "running", false, "queued", "pending"},
		{"running", "running", false, "running", "in_progress"},
		{"open", "done", false, "not_executed", "stopped"},
		{"open", "failed", false, "not_executed", "stopped"},
		{"open", "timeout", false, "not_executed", "stopped"},
		{"running", "done", false, "cancelling", "in_progress"},
		{"stopped", "done", false, "goal_stopped", "stopped"},
		{"stopped", "failed", false, "task_stopped", "stopped"},
		{"done", "done", false, "completed", "completed"},
		{"open", "paused", true, "paused", "paused"},
		{"paused", "paused", true, "paused", "paused"},
		{"done", "paused", true, "completed", "completed"},
		{"stopped", "paused", true, "cancelled", "stopped"},
		{"running", "cancelling", true, "cancelling", "in_progress"},
	} {
		t.Run(tc.parent+"/"+tc.state, func(t *testing.T) {
			conv, intent, _ := artexWorkerProjection(tc.state, tc.parent, tc.paused)
			if conv != tc.conv || intent != tc.intent {
				t.Fatalf("got %s/%s want %s/%s", conv, intent, tc.conv, tc.intent)
			}
		})
	}
}

func TestARTEXFindingTitleDoesNotUseSummary(t *testing.T) {
	for _, tc := range []struct{ name, kind, summary, want string }{
		{"Short name", "IDOR", "Full summary", "Short name"},
		{"  ", " IDOR ", "Full summary", "IDOR"},
		{"Full summary", "IDOR", "Full summary", "IDOR"},
		{"", "", "Full summary", "未命名漏洞"},
		{"Full summary", "Full summary", "Full summary", "未命名漏洞"},
	} {
		if got := artexFindingTitle(map[string]any{"name": tc.name, "vulnclass": tc.kind, "summary": tc.summary}); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
}

func TestARTEXPauseStatusRepairsLegacyCancelledProjection(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "pause status projection"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Mode: pentest.SoloPentestMode})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreatePlanRun(ctx, pentest.PlanRun{ID: "orchestration:pause-projection", ProjectID: project.ID, PlannerConversationID: conv.ID, Objective: "Keep the original objective"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.UpdatePlanRunStatus(ctx, run.ID, "cancelled", "Existing result", ""); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"paused", "cancelling", "running", "queued", "timeout", "paused"} {
		status := nativeRunState(state)
		if err = store.SyncARTEXRunStatus(ctx, run.ID, status, "Existing result", ""); err != nil {
			t.Fatal(err)
		}
		got, err := store.GetPlanRun(ctx, run.ID)
		if err != nil || got.Status != status || got.CompletedAt != nil || got.Objective != run.Objective || got.Summary != "Existing result" {
			t.Fatalf("%s projection: %+v %v", state, got, err)
		}
	}
	for native, want := range map[string]string{"paused": "paused", "cancelling": "pausing", "done": "completed", "running": "planning", "queued": "waiting"} {
		if got := nativeRunState(native); got != want {
			t.Fatalf("%s: got %s want %s", native, got, want)
		}
	}
	if err = store.SyncARTEXRunStatus(ctx, "legacy-run", "paused", "", ""); !errors.Is(err, pentest.ErrLegacyOrchestrationReadOnly) {
		t.Fatalf("historical run accepted native projection: %v", err)
	}
}
