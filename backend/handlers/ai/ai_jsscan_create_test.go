package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/mcptools"
	"hephaestus-backend/pkg/scanexec"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskqueue"
	"hephaestus-backend/pkg/taskstate"
)

func TestPentestJSScanIsOfferedWithoutAutomaticDispatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools []pentest.AgentToolSchema `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		found := false
		for _, tool := range request.Tools {
			found = found || tool.Function.Name == "attack_surface_create_jsscan_task"
		}
		if !found {
			t.Error("JSScan creation was not offered to the model")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"无需采集"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	result, err := pentest.RunAgentLoop(context.Background(), pentest.AgentLoopConfig{
		BaseURL: server.URL, APIKey: "test", Model: "test", DisableModelStreaming: true,
		WorkDir: t.TempDir(), MaxIterations: 1,
		Conversation: pentest.Conversation{Mode: pentest.SoloPentestMode},
		NativeTools:  mcptools.NativeSchemas(), ToolAllowlist: pentest.DefaultPentestToolAllowlist(),
		NativeToolRunner: func(context.Context, pentest.AgentLoopConfig, pentest.AgentToolCall) (any, bool, error) {
			t.Error("runtime dispatched a tool without a model tool call")
			return nil, true, errors.New("unexpected dispatch")
		},
	}, "解释现有结果", nil)
	if err != nil || result.ToolCalls != 0 || result.FinalContent != "无需采集" {
		t.Fatalf("unexpected autonomous scan or finalization: %+v err=%v", result, err)
	}
}

func TestPentestScanCreationPersistsDialogueOwner(t *testing.T) {
	call := pentest.AgentToolCall{Function: pentest.AgentFunctionCall{
		Name:      "attack_surface_create_jsscan_task",
		Arguments: map[string]any{"targets": []string{"https://example.test"}},
	}}
	annotatePentestScanOwnership("conversation-owner", &call)
	config, ok := call.Function.Arguments["config"].(map[string]any)
	if !ok || config["aiWorkerConversationId"] != "conversation-owner" {
		t.Fatalf("dialogue owner was not persisted in scan config: %#v", call.Function.Arguments)
	}

	readOnly := pentest.AgentToolCall{Function: pentest.AgentFunctionCall{
		Name:      "attack_surface_get_jsscan_overview",
		Arguments: map[string]any{},
	}}
	annotatePentestScanOwnership("conversation-owner", &readOnly)
	if _, ok := readOnly.Function.Arguments["config"]; ok {
		t.Fatal("read-only tool unexpectedly received scan ownership metadata")
	}
}

func TestPentestJSScanCreateDispatchRejectsInvalidInput(t *testing.T) {
	for _, args := range []map[string]any{
		{}, {"targets": "https://example.test"},
		{"targets": []string{"example.test"}},
		{"targets": []string{"file:///tmp/index.html"}},
		{"targets": []string{"https://user:password@example.test"}},
	} {
		_, handled, err := mcptools.RunTool(context.Background(), pentest.AgentLoopConfig{}, pentest.AgentToolCall{
			Function: pentest.AgentFunctionCall{Name: "attack_surface_create_jsscan_task", Arguments: args},
		})
		if !handled || err == nil {
			t.Fatalf("invalid creation was not handled and rejected: args=%v handled=%v err=%v", args, handled, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, handled, err := mcptools.RunTool(ctx, pentest.AgentLoopConfig{}, pentest.AgentToolCall{
		Function: pentest.AgentFunctionCall{Name: "attack_surface_create_jsscan_task", Arguments: map[string]any{"targets": []string{"https://example.test"}}},
	})
	if !handled || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled creation: handled=%v err=%v", handled, err)
	}
}

func TestPentestJSScanCreateQueuesTaskAndCancelsOwnedWorker(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	if err := db.AutoMigrate(&database.VulnScanTaskItem{}, &database.ScanAgentCommand{}); err != nil {
		t.Fatal(err)
	}
	// Exercise the real task creator, queue and cancellation lifecycle without
	// making network probes. Only the scanner body is replaced with a worker.
	started, exited := make(chan struct{}), make(chan struct{})
	original := scanexec.Handlers["jsscan"]
	scanexec.Handlers["jsscan"] = func(ctx context.Context, _ string, _ []string, _ map[string]interface{}) error {
		close(started)
		defer close(exited)
		<-ctx.Done()
		return ctx.Err()
	}
	t.Cleanup(func() { scanexec.Handlers["jsscan"] = original })
	args := pentestNativeJsscanArgs{Targets: []string{"https://example.test/app"}}
	normalizePentestJSCollectionConfig(&args)
	result, handled, err := createOrReuseJSScanWithDB(context.Background(), db, "js-dialogue", nil, args, scantask.CreateAIScanTaskWithDB)
	if err != nil || !handled {
		t.Fatalf("create: handled=%v err=%v", handled, err)
	}
	execution := materialExecution(t, time.Now(), "create-js", "attack_surface_create_jsscan_task", nil, result)
	t.Cleanup(func() {
		_, _ = cancelPentestOwnedScanTasks(db, []pentest.ToolExecution{execution})
		deadline := time.Now().Add(5 * time.Second)
		for taskqueue.GetGlobalQueue().GetRunningCount() > 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
	})
	awaitBatchSignal(t, started)
	if result["reused"] != false || result["status"] != "created" || !strings.Contains(result["nextAction"].(string), "attack_surface_get_jsscan_overview") {
		t.Fatalf("unexpected async result: %v", result)
	}
	var task database.VulnScanTask
	if err := db.First(&task, "id = ?", result["taskId"]).Error; err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(task.Config), &config); err != nil {
		t.Fatal(err)
	}
	if task.ScanType != "jsscan" || task.Status != taskstate.StatusRunning || !task.CreatedByAI || config["aiEgressRequired"] != nil || config["vulnDetection"].(map[string]any)["enabled"] != false {
		t.Fatalf("unexpected task: %+v", task)
	}
	if err := validatePentestTaskReadType("attack_surface_get_jsscan_overview", task); err != nil {
		t.Fatalf("created task cannot be read: %v", err)
	}
	reused, _, err := createOrReuseJSScanWithDB(context.Background(), db, "js-dialogue", nil, args, scantask.CreateAIScanTaskWithDB)
	if err != nil || reused["reused"] != true || reused["taskId"] != result["taskId"] {
		t.Fatalf("reuse: result=%v err=%v", reused, err)
	}
	cancellation, err := cancelPentestOwnedScanTasks(db, []pentest.ToolExecution{execution})
	if err != nil || len(cancellation.Cancelled) != 1 || cancellation.Cancelled[0] != task.ID {
		t.Fatalf("cancellation=%v err=%v", cancellation, err)
	}
	awaitBatchSignal(t, exited)
	if err := db.First(&task, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != taskstate.StatusCancelled {
		t.Fatalf("JSScan was not cancelled: %s", task.Status)
	}
}
