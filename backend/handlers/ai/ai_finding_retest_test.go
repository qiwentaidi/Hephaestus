package ai

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	artexdb "hephaestus-backend/lib/pentest/taskengine/db"
	"hephaestus-backend/pkg/capabilities"
)

func TestOrchestrationFindingRetestWritesFreshWorkerEvidence(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "retest"})
	if err != nil {
		t.Fatal(err)
	}
	run := pentest.PlanRun{ID: "orchestration:44", ProjectID: project.ID, PlannerConversationID: "source-worker", Objective: "fixture"}
	for _, entry := range []struct{ id, role string }{{"source-worker", "worker"}, {"retest-worker", "worker"}, {"main", "controller"}} {
		if _, err = store.UpsertConversation(ctx, pentest.Conversation{ID: entry.id, ProjectID: project.ID, Mode: pentest.SoloPentestMode}); err != nil {
			t.Fatal(err)
		}
		if err = store.SetAgentRole(ctx, entry.id, entry.role, "", run.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = store.CreatePlanRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	finding, err := store.RecordFinding(ctx, pentest.Finding{ProjectID: project.ID, ConversationID: "source-worker", Title: "fixture", Severity: "high", Status: "open", Evidence: "original evidence", ReproductionSteps: "original steps"})
	if err != nil {
		t.Fatal(err)
	}
	// Evidence has not yet reached the UI projection. The native-tool boundary
	// must refresh it before resolving the Worker's short tool-use reference.
	previous := syncFindingRetestConversation
	defer func() { syncFindingRetestConversation = previous }()
	syncCount := 0
	syncFindingRetestConversation = func(ctx context.Context, store *pentest.SQLStore, conversationID string) error {
		syncCount++
		return projectARTEXActivity(ctx, store, run, conversationID, artexdb.Activity{ID: int64(syncCount), Kind: "tool_result", Tool: "Bash", ToolUseID: "fresh-call", Detail: "fresh response", CreatedAt: time.Now()})
	}
	var readTool, recordTool capabilities.Tool
	for _, tool := range capabilities.Tools(ctx, capabilities.RuntimeEngine, "worker") {
		switch tool.Name {
		case "retest_vulnerability":
			readTool = tool
		case "record_vulnerability_retest":
			recordTool = tool
		}
	}
	if readTool.Name == "" || recordTool.Name == "" || !readTool.ReadOnly || recordTool.ReadOnly {
		t.Fatal("retest tools are unavailable or have incorrect mutability")
	}
	host := capabilities.Host{Store: store, ProjectID: project.ID, ConversationID: "retest-worker", AgentKey: "worker"}
	value, err := capabilities.Call(ctx, host, readTool, map[string]any{"findingId": finding.ID})
	if err != nil {
		t.Fatal(err)
	}
	retestContext := value.(gin.H)
	if retestContext["sourceConversationId"] != "source-worker" || retestContext["originalEvidence"] != "original evidence" {
		t.Fatalf("lost source evidence: %v", retestContext)
	}
	if strings.Contains(retestContext["instruction"].(string), "add_intent") {
		t.Fatal("Worker must execute the retest, not delegate it again")
	}
	value, err = capabilities.Call(ctx, host, recordTool, map[string]any{"findingId": finding.ID, "result": "still_vulnerable", "summary": "still reproducible", "evidence": "fresh response", "executionRefs": []string{"fresh-call"}})
	if err != nil {
		t.Fatal(err)
	}
	retest := value.(pentest.FindingRetest)
	if syncCount != 2 || retest.ConversationID != "retest-worker" || len(retest.ExecutionRefs) != 1 || retest.ExecutionRefs[0] != "retest-worker:tool:fresh-call" {
		t.Fatalf("incorrect writeback: %+v, sync=%d", retest, syncCount)
	}
	mainHost := host
	mainHost.ConversationID, mainHost.AgentKey = "main", "mainagent"
	mainContext, err := capabilities.Call(ctx, mainHost, readTool, map[string]any{"findingId": finding.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mainContext.(gin.H)["instruction"].(string), "add_intent") {
		t.Fatal("main Agent did not receive the one-shot delegation contract")
	}
	detail, err := buildPentestFindingDetail(ctx, store, finding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail["retests"].([]pentest.FindingRetest)) != 1 || detail["retestTarget"].(findingRetestTarget).ConversationID != "main" {
		t.Fatalf("detail did not retain retest and main: %+v", detail)
	}
	original, err := store.GetFinding(ctx, finding.ID)
	if err != nil || original.Evidence != finding.Evidence || original.Status != finding.Status || original.ConversationID != finding.ConversationID || original.ReproductionSteps != finding.ReproductionSteps {
		t.Fatalf("original finding changed: %+v %v", original, err)
	}
	for _, tc := range []struct{ name, owner, status string }{
		{"foreign worker evidence", "source-worker", "completed"},
		{"failed execution", "retest-worker", "failed"},
		{"running execution", "retest-worker", "running"},
		{"cancelled execution", "retest-worker", "cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := "invalid-" + tc.status + "-" + tc.owner
			if _, err = store.SaveToolExecution(ctx, pentest.ToolExecution{ID: id, ConversationID: tc.owner, Status: tc.status, ToolName: "fixture", StartTime: time.Now()}); err != nil {
				t.Fatal(err)
			}
			if _, err = capabilities.Call(ctx, host, recordTool, map[string]any{"findingId": finding.ID, "result": "fixed", "summary": "must not save", "executionRefs": []string{id}}); err == nil {
				t.Fatal("invalid execution accepted")
			}
		})
	}
	syncFindingRetestConversation = func(context.Context, *pentest.SQLStore, string) error { return fmt.Errorf("projection unavailable") }
	if _, err = capabilities.Call(ctx, host, recordTool, map[string]any{"findingId": finding.ID, "result": "fixed", "summary": "must not save"}); err == nil {
		t.Fatal("failed projection must not silently save an unverified result")
	}
	items, err := store.ListFindingRetests(ctx, finding.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("invalid calls wrote retests: %v %v", items, err)
	}
}

func TestIndependentFindingRetestStaysInProject(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, _ := store.UpsertProject(ctx, pentest.Project{Name: "owner"})
	other, _ := store.UpsertProject(ctx, pentest.Project{Name: "other"})
	finding, err := store.RecordFinding(ctx, pentest.Finding{ProjectID: project.ID, Title: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []pentest.Project{project, other} {
		conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: p.ID, Mode: pentest.SoloPentestMode})
		if err != nil {
			t.Fatal(err)
		}
		cfg := pentest.AgentLoopConfig{Store: store, Project: p, Conversation: conv}
		for _, name := range []string{"retest_vulnerability", "record_vulnerability_retest"} {
			value, _, callErr := runPentestFindingRetestTool(ctx, cfg, pentest.AgentToolCall{Function: pentest.AgentFunctionCall{Name: name, Arguments: map[string]any{"findingId": finding.ID, "result": "inconclusive", "summary": "credentials unavailable"}}})
			err = callErr
			if (err != nil) != (p.ID != project.ID) {
				t.Fatalf("project=%s tool=%s err=%v", p.ID, name, err)
			}
			if err == nil && name == "retest_vulnerability" && strings.Contains(value.(gin.H)["instruction"].(string), "add_intent") {
				t.Fatal("standalone retest inherited orchestration instructions")
			}
		}
	}
}
