package ai

import (
	"context"
	"errors"
	"fmt"
	"hephaestus-backend/lib/pentest/taskengine/agent"
	"hephaestus-backend/lib/pentest/taskengine/db"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestARTEXNativePromptPersistence(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "orchestration.db")
		testTestARTEXNativePromptPersistence(t, func(ctx context.Context, dir string) (*taskengine.Runtime, error) {
			return taskengine.OpenSQLite(ctx, path, dir)
		})
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("ARTEX_TEST_DSN")
		if dsn == "" {
			t.Skip("ARTEX_TEST_DSN required (isolated PostgreSQL)")
		}
		testTestARTEXNativePromptPersistence(t, func(ctx context.Context, dir string) (*taskengine.Runtime, error) {
			return taskengine.Open(ctx, dsn, dir)
		})
	})
}
func testTestARTEXNativePromptPersistence(t *testing.T, open func(context.Context, string) (*taskengine.Runtime, error)) {
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
	t.Run("native report projection", func(t *testing.T) { testARTEXReportProjection(t, rt) })
	for _, role := range []string{"planner", "worker", "controller"} {
		item, err := loadARTEXPrompt(ctx, rt, role)
		if err != nil {
			t.Fatal(err)
		}
		if item.DefaultPrompt != agent.BuiltinPromptSeeds()[nativePromptKey(role)] || item.Version < 1 {
			t.Fatalf("wrong native default: %s", role)
		}
		updated, err := updateARTEXPrompt(ctx, rt, role, item.Prompt+"\nFixture prompt note.", item.Version)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = updateARTEXPrompt(ctx, rt, role, "stale", item.Version); !errors.Is(err, pentest.ErrAgentPromptConflict) {
			t.Fatalf("lost update: %v", err)
		}
		loaded, err := loadARTEXPrompt(ctx, rt, role)
		if err != nil || loaded.Prompt != updated.Prompt || loaded.Version != updated.Version {
			t.Fatalf("persistence: %v", err)
		}
		if _, err = updateARTEXPrompt(ctx, rt, role, item.Prompt, updated.Version); err != nil {
			t.Fatal(err)
		}
	}
}

func testARTEXReportProjection(t *testing.T, rt *taskengine.Runtime) {
	ctx := context.Background()
	profile := db.LLMProfile{Name: "projection fixture", Format: "openai", BaseURL: "http://127.0.0.1:1/v1", APIKey: "fixture", Model: "fixture", PoolExclude: true}
	task, err := rt.Engine.PrepareHostTask(taskengine.HostTaskOptions{Name: "projection fixture", Planner: profile, Worker: profile})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rt.Engine.DeleteHostTask(ctx, task.ID); err != nil {
			t.Error(err)
		}
	}()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "projection"})
	if err != nil {
		t.Fatal(err)
	}
	run := pentest.PlanRun{ID: pentest.OrchestrationRunPrefix + task.ID, ProjectID: project.ID, Objective: "Offline projection"}
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
	intent, err := task.Store.AddNode("intent", map[string]any{"summary": "worker execution fixture"}, 5, "running", "planner", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"tool_use", "tool_result"} {
		if _, err = task.Store.AppendActivity(db.Activity{NodeID: &intent, Worker: "work#1", Kind: kind, Tool: "fixture", ToolUseID: "fixture-call", Detail: `{"fixture":true}`}); err != nil {
			t.Fatal(err)
		}
	}
	tid, _ := strconv.ParseInt(task.ID, 10, 64)
	recorded, err := task.Store.RecordFinding(ctx, db.RecordFindingInput{TaskID: tid, IntentID: intent, Name: "Named finding", VulnClass: "TEST", Severity: "low", Summary: "fixture summary", Evidence: "observed", Worker: "worker", Preconditions: "test account", Reproduction: "1. Replay fixture request", Impact: "fixture impact", Recommendation: "check authorization"})
	if err != nil {
		t.Fatal(err)
	}
	fid := recorded.FindingID
	// First sync reproduces the migration's pre-existing empty target record.
	if err = syncARTEXRun(ctx, store, rt, run); err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateFindingStatus(ctx, nativeNodeID(run.ID, recorded.NodeID), pentest.FindingStatusFixed); err != nil {
		t.Fatal(err)
	}
	var assetID int64
	if err = rt.DB.QueryRow(`INSERT INTO assets(type,url,method) VALUES('endpoint','https://projection.example.test/resource','GET') RETURNING id`).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	defer rt.DB.Exec(`DELETE FROM assets WHERE id=$1`, assetID)
	if _, err = rt.DB.Exec(`UPDATE findings SET asset_ids=$1 WHERE id=$2`, fmt.Sprintf("[%d,%d]", assetID, assetID), fid); err != nil {
		t.Fatal(err)
	}
	for _, report := range []string{"Initial native report", "Updated asynchronous native report"} {
		if _, err = rt.DB.Exec(`UPDATE findings SET report=$1 WHERE id=$2`, report, fid); err != nil {
			t.Fatal(err)
		}
		if err = syncARTEXRun(ctx, store, rt, run); err != nil {
			t.Fatal(err)
		}
		findings, err := store.ListFindings(ctx, project.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 {
			t.Fatalf("missing finding: %+v", findings)
		}
		f := findings[0]
		if f.Status != pentest.FindingStatusFixed || f.Title != "Named finding" || f.Target != "https://projection.example.test/resource" || f.Impact != "fixture summary\n\nfixture impact" || f.Preconditions != "test account" || f.ReproductionSteps != "1. Replay fixture request" || f.Recommendation != "check authorization" || f.Evidence != "observed\n\n"+report {
			t.Fatalf("report projection: %+v", f)
		}
	}
	// Changing only an associated asset must invalidate the projection hash too.
	var secondAsset int64
	if err = rt.DB.QueryRow(`INSERT INTO assets(type,ip,port,service_name,service_type) VALUES('service','2001:db8::1',8443,'fixture','other') RETURNING id`).Scan(&secondAsset); err != nil {
		t.Fatal(err)
	}
	defer rt.DB.Exec(`DELETE FROM assets WHERE id=$1`, secondAsset)
	if _, err = rt.DB.Exec(`UPDATE findings SET asset_ids=$1 WHERE id=$2`, fmt.Sprintf("[%d,%d,%d]", assetID, secondAsset, assetID), fid); err != nil {
		t.Fatal(err)
	}
	if err = syncARTEXRun(ctx, store, rt, run); err != nil {
		t.Fatal(err)
	}
	if _, err = rt.DB.Exec(`UPDATE assets SET url='https://projection.example.test/updated' WHERE id=$1`, assetID); err != nil {
		t.Fatal(err)
	}
	if err = syncARTEXRun(ctx, store, rt, run); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetFinding(ctx, nativeNodeID(run.ID, recorded.NodeID))
	if err != nil || updated.Target != "https://projection.example.test/updated\n[2001:db8::1]:8443" {
		t.Fatalf("asset update projection: %+v %v", updated, err)
	}
	// Removing the native name must repair a previously projected title, using
	// the vulnerability class rather than the (potentially very long) summary.
	if _, err = rt.DB.Exec(`UPDATE findings SET name='   ' WHERE id=$1`, fid); err != nil {
		t.Fatal(err)
	}
	if err = syncARTEXRun(ctx, store, rt, run); err != nil {
		t.Fatal(err)
	}
	unnamed, err := store.GetFinding(ctx, nativeNodeID(run.ID, recorded.NodeID))
	if err != nil || unnamed.Title != "TEST" || unnamed.ID != updated.ID || unnamed.Impact != updated.Impact {
		t.Fatalf("unnamed finding fallback: %+v %v", unnamed, err)
	}
	worker := nativeConversation(run.ID, "intent:"+strconv.FormatInt(intent, 10))
	executions, err := store.ListToolExecutionsByConversation(ctx, worker, 10)
	if err != nil || len(executions) != 1 || executions[0].IntentID != nativeNodeID(run.ID, intent) || executions[0].Status != "completed" {
		t.Fatalf("native Worker activity projection: %+v %v", executions, err)
	}
	for _, conversation := range []string{run.PlannerConversationID, worker} {
		if _, err = store.GetConversation(ctx, conversation); err != nil {
			t.Fatalf("projected session %s failed to load: %v", conversation, err)
		}
	}
}
