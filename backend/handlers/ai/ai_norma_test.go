package ai

import (
	"context"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine/db"
	"hephaestus-backend/pkg/toolruntime"
	"testing"
	"time"
)

func TestNormaBackgroundProjectionCountsOriginalCallOnce(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "norma stats"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.UpsertConversation(ctx, pentest.Conversation{ID: "orchestration:norma:planner", ProjectID: project.ID, Mode: pentest.SoloPentestMode})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreatePlanRun(ctx, pentest.PlanRun{ID: "orchestration:norma", ProjectID: project.ID, PlannerConversationID: conv.ID, Objective: "stats"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	o := toolruntime.Outcome{Runtime: "norma", ToolName: "Bash", ToolUseID: "launch", Status: "running", TaskID: "task_1", StartedAt: start}
	launch := db.Activity{ID: 1, Kind: "tool_use", Tool: "Bash", ToolUseID: "launch", Detail: `{"command":"fixture","run_in_background":true}`, CreatedAt: start}
	if err = projectARTEXActivity(ctx, store, run, conv.ID, launch); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		launch.ID = 2
		launch.Kind = "tool_result"
		launch.Detail = o.JSON()
		if err = projectARTEXActivity(ctx, store, run, conv.ID, launch); err != nil {
			t.Fatal(err)
		}
	}
	calls, err := store.ListToolExecutionsByConversation(ctx, conv.ID, 10)
	if err != nil || len(calls) != 1 || calls[0].Status != "running" || calls[0].EndTime != nil {
		t.Fatalf("premature completion: %+v %v", calls, err)
	}
	end := start.Add(time.Second)
	o.Status = "completed"
	o.CompletedAt = &end
	o.Output = "done"
	launch.ID = 3
	launch.Kind = "tool_execution_update"
	launch.Detail = o.JSON()
	launch.CreatedAt = end
	for i := 0; i < 2; i++ {
		if err = projectARTEXActivity(ctx, store, run, conv.ID, launch); err != nil {
			t.Fatal(err)
		}
	}
	calls, err = store.ListToolExecutionsByConversation(ctx, conv.ID, 10)
	if err != nil || len(calls) != 1 || calls[0].Status != "completed" || calls[0].EndTime == nil || calls[0].Duration != time.Second {
		t.Fatalf("duplicate or lost completion: %+v %v", calls, err)
	}
}
