package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/agentruntime"
	"hephaestus-backend/pkg/database"
)

func workerCredentialFixture(t *testing.T) (*pentest.SQLStore, *gorm.DB, pentest.WorkerTurn, *agentruntime.NodeSession) {
	t.Helper()
	store, db, _, turn := testfixture.PostgresWorker(t)
	if err := db.Model(&database.ScanAgentNode{}).Where("agent_id=?", turn.NodeID).Update("status", "offline").Error; err != nil {
		t.Fatal(err)
	}
	session, err := agentruntime.AcquireNodeSession(context.Background(), db, uuid.NewString(), turn.NodeID, "fixture", "192.0.2.1", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	if err = db.Model(&database.ScanAgentNode{}).Where("agent_id=?", turn.NodeID).
		Updates(map[string]any{"status": "online", "last_heartbeat": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	return store, db, turn, session
}

func TestWorkerAuthPostgresRemoteToolWithoutSharedToken(t *testing.T) {
	t.Setenv("AGENT_INTERNAL_TOKEN", "")
	store, db, turn, session := workerCredentialFixture(t)
	var requests, executed atomic.Int32
	router := gin.New()
	router.POST(workerPlatformToolPath, func(c *gin.Context) {
		requests.Add(1)
		serveWorkerPlatformTool(c, db, func(ctx context.Context, req workerToolRequest) (any, bool, error) {
			if _, err := validateWorkerPlatformTurn(ctx, store, req.Turn); err != nil {
				return nil, true, err
			}
			executed.Add(1)
			return map[string]any{"fixture": "completed"}, true, nil
		})
	})
	server := httptest.NewServer(router)
	defer server.Close()
	call := pentest.AgentToolCall{ID: "fixture-call", Function: pentest.AgentFunctionCall{Name: "fixture_platform_tool", Arguments: map[string]any{}}}
	cfg := pentest.AgentLoopConfig{WorkerTurn: &turn, ToolExecutionID: "fixture-execution", NativeTools: []pentest.AgentToolSchema{{Function: pentest.AgentToolFunction{Name: call.Function.Name}}}}
	runner := workerNativeToolRunner(false, server.URL, session)
	result, handled, err := runner(context.Background(), cfg, call)
	if err != nil || !handled || result == nil || executed.Load() != 1 || requests.Load() != 1 {
		t.Fatalf("automatic authentication failed: handled=%v result=%v error=%v", handled, result, err)
	}
	// Expired execution cannot use an otherwise valid node credential.
	if err = db.Exec(`UPDATE pentest_worker_turns SET lease_expires_at=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=?`, turn.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = runner(context.Background(), cfg, call); err == nil || !strings.Contains(err.Error(), pentest.ErrWorkerLeaseLost.Error()) {
		t.Fatalf("expired lease accepted: %v", err)
	}
	if executed.Load() != 1 || requests.Load() != 2 {
		t.Fatal("expired call executed or was automatically retried")
	}
	// The server's explicit authentication rejection is distinct from an
	// unknown side-effect outcome, and is never automatically retried.
	if err = db.Model(&database.ScanAgentNode{}).Where("agent_id=?", turn.NodeID).Update("platform_token_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = runner(context.Background(), cfg, call); err == nil || !strings.Contains(err.Error(), "authentication failed") || strings.Contains(err.Error(), "reconcile") {
		t.Fatalf("authentication rejection misreported: %v", err)
	}
	if executed.Load() != 1 || requests.Load() != 3 {
		t.Fatal("authentication rejection executed or was automatically retried")
	}
}

func TestWorkerAuthPostgresRejectsMissingForgedAndCrossNodeCredentials(t *testing.T) {
	legacyToken := strings.Repeat("a", 43)
	t.Setenv("AGENT_INTERNAL_TOKEN", legacyToken)
	_, db, turn, session := workerCredentialFixture(t)
	for _, tc := range []struct {
		name   string
		status int
		mutate func(*http.Request, *workerToolRequest)
	}{
		{"missing bearer", http.StatusUnauthorized, func(r *http.Request, _ *workerToolRequest) { r.Header.Del("Authorization") }},
		{"wrong bearer", http.StatusUnauthorized, func(r *http.Request, _ *workerToolRequest) {
			r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 43))
		}},
		{"legacy shared token", http.StatusUnauthorized, func(r *http.Request, _ *workerToolRequest) { r.Header.Set("Authorization", "Bearer "+legacyToken) }},
		{"missing node", http.StatusUnauthorized, func(r *http.Request, _ *workerToolRequest) { r.Header.Del(agentruntime.NodeCredentialHeader) }},
		{"wrong node", http.StatusUnauthorized, func(r *http.Request, _ *workerToolRequest) {
			r.Header.Set(agentruntime.NodeCredentialHeader, "other-node")
		}},
		{"cross-node turn", http.StatusForbidden, func(_ *http.Request, req *workerToolRequest) { req.Turn.NodeID = "other-node" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := workerToolRequest{Version: workerProtocolVersion, Turn: turn, ExecutionID: "fixture"}
			r := httptest.NewRequest(http.MethodPost, workerPlatformToolPath, nil)
			if err := session.AuthorizePlatformRequest(context.Background(), turn.NodeID, r); err != nil {
				t.Fatal(err)
			}
			tc.mutate(r, &payload)
			body, _ := json.Marshal(payload)
			request := httptest.NewRequest(http.MethodPost, workerPlatformToolPath, bytes.NewReader(body))
			request.Header = r.Header
			request.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = request
			serveWorkerPlatformTool(c, db, func(context.Context, workerToolRequest) (any, bool, error) {
				t.Error("rejected request reached tool executor")
				return nil, true, nil
			})
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestWorkerAuthPostgresValidatesTurnIdentityAndState(t *testing.T) {
	for _, tc := range []struct {
		name, update string
		mutate       func(*pentest.WorkerTurn)
	}{
		{name: "wrong conversation", mutate: func(t *pentest.WorkerTurn) { t.ConversationID = "other" }},
		{name: "wrong node", mutate: func(t *pentest.WorkerTurn) { t.NodeID = "other" }},
		{name: "stale generation", mutate: func(t *pentest.WorkerTurn) { t.Generation-- }},
		{name: "cancelled", update: `UPDATE pentest_worker_turns SET cancel_requested=1 WHERE id=?`},
		{name: "deleted", update: `UPDATE pentest_worker_turns SET delete_requested=1 WHERE id=?`},
		{name: "finished", update: `UPDATE pentest_worker_turns SET status='completed',lease_expires_at=NULL WHERE id=?`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, db, _, turn := testfixture.PostgresWorker(t)
			if tc.mutate != nil {
				tc.mutate(&turn)
			} else if err := db.Exec(tc.update, turn.ID).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := validateWorkerPlatformTurn(context.Background(), store, turn); err == nil {
				t.Fatal("invalid turn admitted")
			}
		})
	}
}
