package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine"
	artexdb "hephaestus-backend/lib/pentest/taskengine/db"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/mcpclient"
	"hephaestus-backend/pkg/mcptools"
)

// This test owns an isolated PostgreSQL database. A child process keeps the
// application's singleton DB/config/runtime from leaking into other tests.
func TestNativeWholeTaskReferenceMessage(t *testing.T) {
	dsn := os.Getenv("HEPHAESTUS_NATIVE_REFERENCE_TEST_DSN")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	if os.Getenv("HEPHAESTUS_REFERENCE_TEST_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNativeWholeTaskReferenceMessage$", "-test.v")
		cmd.Env = append(os.Environ(), "HEPHAESTUS_REFERENCE_TEST_CHILD=1", "HEPHAESTUS_CONFIG_DIR="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native reference lifecycle: %v\n%s", err, out)
		}
		return
	}
	ctx := context.Background()
	pg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.GetDefaultConfig()
	cfg.Database = config.DatabaseConfig{Type: "postgres", Host: pg.Host, Port: strconv.Itoa(int(pg.Port)), User: pg.User, Password: pg.Password, DBName: pg.Database, SSLMode: "disable"}
	if err = config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	appDB, err := database.GetDB()
	if err != nil {
		t.Fatal(err)
	}
	store, err := aipkg.StoreFromAppDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	RegisterMCPBridge()
	mcptools.RegisterServerProvider(referenceTestProvider{})
	scanID := "native-ports-" + uuid.NewString()
	defer appDB.Where("task_id = ?", scanID).Delete(&database.AIAssetReference{})
	defer appDB.Where("id = ?", scanID).Delete(&database.VulnScanTask{})
	var readTail atomic.Bool
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		msg := map[string]any{"role": "assistant", "content": "Read the final page; earlier pages have not been reviewed."}
		finish := "stop"
		if len(req.Messages) == 0 {
			t.Error("model request has no messages")
			w.WriteHeader(400)
			return
		}
		last := req.Messages[len(req.Messages)-1]
		if last.Role == "tool" {
			text, _ := last.Content.(string)
			var page struct {
				Total      int               `json:"total"`
				NextOffset int               `json:"nextOffset"`
				HasMore    bool              `json:"hasMore"`
				Assets     []referencedAsset `json:"assets"`
			}
			if err := json.Unmarshal([]byte(text), &page); err != nil || page.Total != 10001 || page.NextOffset != 10001 || page.HasMore || len(page.Assets) != 1 {
				t.Errorf("reference tool did not return the requested page: %s (%v)", text, err)
			} else {
				readTail.Store(true)
			}
		} else {
			found := false
			for _, tool := range req.Tools {
				if tool.Function.Name == "attack_surface_read_reference" {
					found = true
				}
			}
			if !found {
				t.Error("main agent did not receive reference tool")
			}
			var ref database.AIAssetReference
			if err := appDB.Where("task_id = ?", scanID).First(&ref).Error; err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			args, _ := json.Marshal(map[string]any{"referenceId": ref.ID, "offset": 10000, "limit": 100})
			msg["tool_calls"] = []any{map[string]any{"id": "read-tail", "type": "function", "function": map[string]any{"name": "attack_surface_read_reference", "arguments": string(args)}}}
			msg["content"] = nil
			finish = "tool_calls"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "choices": []any{map[string]any{"index": 0, "message": msg, "finish_reason": finish}}})
	}))
	defer fake.Close()
	rt, err := taskengine.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := taskengine.Stop(); err != nil {
			t.Error(err)
		}
	}()
	profile := artexdb.LLMProfile{Name: "Reference fixture", Format: "openai", BaseURL: fake.URL + "/v1", APIKey: "fixture", Model: "fixture", PoolExclude: true}
	task, err := rt.Engine.PrepareHostTask(taskengine.HostTaskOptions{Name: "Reference fixture", Goal: "Read existing results only", Planner: profile, Worker: profile})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rt.Engine.DeleteHostTask(ctx, task.ID); err != nil {
			t.Error(err)
		}
	}()
	tid, _ := strconv.ParseInt(task.ID, 10, 64)
	if err = rt.DB.SetStatus(tid, "done"); err != nil {
		t.Fatal(err)
	}
	if err = taskengine.Stop(); err != nil {
		t.Fatal(err)
	}
	rt, err = taskengine.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, dto, err := rt.Engine.HostTask(task.ID)
	if err != nil || dto.Status != "done" {
		t.Fatalf("completed task restore: %+v %v", dto, err)
	}
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "Reference fixture"})
	if err != nil {
		t.Fatal(err)
	}
	run := pentest.PlanRun{ID: "orchestration:" + task.ID, ProjectID: project.ID, Objective: "Read existing results only"}
	run.PlannerConversationID = nativeConversation(run.ID, "planner")
	run.ControllerConversationID = nativeConversation(run.ID, "controller")
	for _, role := range []string{"planner", "controller"} {
		id := nativeConversation(run.ID, role)
		if _, err = store.UpsertConversation(ctx, pentest.Conversation{ID: id, ProjectID: project.ID, Mode: pentest.SoloPentestMode, Status: "completed"}); err != nil {
			t.Fatal(err)
		}
		if err = store.SetAgentRole(ctx, id, role, "", run.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = store.CreatePlanRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err = appDB.Create(&database.VulnScanTask{ID: scanID, Version: 1, ScanType: "portscan", Status: "completed"}).Error; err != nil {
		t.Fatal(err)
	}
	ports := make([]database.PortScanResult, 10001)
	for i := range ports {
		ports[i] = database.PortScanResult{TaskID: scanID, Version: 1, Host: fmt.Sprintf("fixture-%d.test", i), Port: 443}
	}
	if err = appDB.CreateInBatches(ports, 300).Error; err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"content": "Read the final page only; do not scan.", "metadata": map[string]any{"assetReferences": []assetReferenceSelection{{TaskID: scanID, Version: 1, Selection: "all"}}}})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/sessions/:id/message/stream", CreateAIPentestSessionMessageStreamHandler)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/sessions/"+run.ControllerConversationID+"/message/stream", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "event: done") {
		t.Fatalf("stream: %d %s", recorder.Code, recorder.Body.String())
	}
	if !readTail.Load() {
		t.Fatal("main agent never read beyond 10,000 assets")
	}
	var ref database.AIAssetReference
	if err = appDB.Where("conversation_id = ?", run.ControllerConversationID).First(&ref).Error; err != nil {
		t.Fatal(err)
	}
	if ref.ReadMode != "task" || ref.AssetsJSON != "[]" {
		t.Fatal("whole task was eagerly copied")
	}
	if _, err = readAssetReference(appDB, run.PlannerConversationID, ref.ID, "", 0, 1); err == nil {
		t.Fatal("reference leaked to another conversation")
	}
	if !strings.Contains(recorder.Body.String(), `\"readMode\":\"task\"`) && !strings.Contains(recorder.Body.String(), `"readMode":"task"`) {
		t.Fatal("reference metadata missing from history")
	}
}

type referenceTestProvider struct{}

func (referenceTestProvider) LoadServers() ([]mcpclient.Server, error) { return nil, nil }
func (referenceTestProvider) ExternalSchemas([]mcpclient.Server) []pentest.AgentToolSchema {
	return nil
}
func (referenceTestProvider) Authorize(string) error                      { return nil }
func (referenceTestProvider) RefreshTools(*pentest.AgentLoopConfig) error { return nil }
func (referenceTestProvider) RunExternal(context.Context, pentest.AgentLoopConfig, pentest.AgentToolCall) (any, bool, error) {
	return nil, false, nil
}
