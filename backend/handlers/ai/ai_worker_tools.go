package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/agentruntime"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/mcptools"
	"hephaestus-backend/pkg/toolguard"
)

const workerProtocolVersion = 1
const workerPlatformToolPath = "/api/v1/internal/agents/v1/tools/call"

type workerToolRequest struct {
	Version     int                   `json:"version"`
	Turn        pentest.WorkerTurn    `json:"turn"`
	ExecutionID string                `json:"executionId"`
	Call        pentest.AgentToolCall `json:"call"`
}
type workerToolResponse struct {
	Result  any    `json:"result"`
	Handled bool   `json:"handled"`
	Error   string `json:"error,omitempty"`
}

func workerLocalNativeTool(name string) bool {
	switch name {
	case "nuclei", "nuclei_list_templates", "nuclei_read_template", "dns_lookup", "http-framework-test", "jwt_list_dictionaries", "jwt_bruteforce", "waybackurls":
		return true
	}
	return strings.HasPrefix(name, "external_mcp_")
}
func workerNativeToolRunner(local bool, serverURL string, session *agentruntime.NodeSession) pentest.AgentNativeToolRunner {
	return func(ctx context.Context, cfg pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error) {
		if workerLocalNativeTool(call.Function.Name) {
			return mcptools.RunTool(ctx, cfg, call)
		}
		// YAML command tools are handled by the leased worker's system runner.
		native := false
		for _, schema := range cfg.NativeTools {
			if schema.Function.Name == call.Function.Name {
				native = true
				break
			}
		}
		if !native {
			return nil, false, nil
		}
		if cfg.WorkerTurn == nil {
			return nil, true, pentest.ErrWorkerLeaseLost
		}
		req := workerToolRequest{Version: workerProtocolVersion, Turn: *cfg.WorkerTurn, ExecutionID: cfg.ToolExecutionID, Call: call}
		if local {
			return executeWorkerPlatformTool(ctx, req)
		}
		body, err := json.Marshal(req)
		if err != nil {
			return nil, true, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(serverURL, "/")+workerPlatformToolPath, bytes.NewReader(body))
		if err != nil {
			return nil, true, err
		}
		request.Header.Set("Content-Type", "application/json")
		if err := session.AuthorizePlatformRequest(ctx, req.Turn.NodeID, request); err != nil {
			return nil, true, fmt.Errorf("platform tool node authentication unavailable; restart the updated Agent to register its credential: %w", err)
		}
		// Side effects are never blindly retried on an unknown HTTP outcome.
		client := &http.Client{Timeout: 10 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		response, err := client.Do(request)
		if err != nil {
			return nil, true, fmt.Errorf("platform tool outcome unknown; execution %s must be reconciled: %w", req.ExecutionID, err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			if response.StatusCode == http.StatusUnauthorized {
				var rejection struct {
					Code string `json:"code"`
				}
				if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&rejection) == nil && rejection.Code == "agent_authentication_failed" {
					return nil, true, fmt.Errorf("platform tool authentication failed (HTTP 401); request rejected before tool execution; update the primary and Agent, then restart the Agent to register its node credential")
				}
			}
			return nil, true, fmt.Errorf("platform tool HTTP %d; reconcile execution %s before retry", response.StatusCode, req.ExecutionID)
		}
		var result workerToolResponse
		if err = json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&result); err != nil {
			return nil, true, err
		}
		if result.Error != "" {
			return result.Result, result.Handled, fmt.Errorf("%s", result.Error)
		}
		return result.Result, result.Handled, nil
	}
}

func WorkerPlatformToolHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Agent authentication unavailable"})
		return
	}
	serveWorkerPlatformTool(c, db, executeWorkerPlatformTool)
}

func serveWorkerPlatformTool(c *gin.Context, db *gorm.DB, execute func(context.Context, workerToolRequest) (any, bool, error)) {
	nodeID := c.GetHeader(agentruntime.NodeCredentialHeader)
	presented, bearer := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !bearer {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid Agent credential", "code": "agent_authentication_failed"})
		return
	}
	err := agentruntime.VerifyNodeCredential(c.Request.Context(), db, nodeID, presented)
	if errors.Is(err, agentruntime.ErrNodeCredential) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid Agent credential", "code": "agent_authentication_failed"})
		return
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Agent authentication unavailable"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	var req workerToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Turn.NodeID != nodeID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Agent credential does not own this worker turn"})
		return
	}
	data, handled, err := execute(c.Request.Context(), req)
	response := workerToolResponse{Result: data, Handled: handled}
	if err != nil {
		response.Error = err.Error()
	}
	c.JSON(http.StatusOK, response)
}

func executeWorkerPlatformTool(ctx context.Context, req workerToolRequest) (any, bool, error) {
	if req.Version != workerProtocolVersion {
		return nil, true, fmt.Errorf("unsupported worker protocol version")
	}
	if req.ExecutionID == "" || workerLocalNativeTool(req.Call.Function.Name) {
		return nil, true, fmt.Errorf("invalid platform tool request")
	}
	store, err := aipkg.StoreFromAppDB(ctx)
	if err != nil {
		return nil, true, err
	}
	actualTurn, err := validateWorkerPlatformTurn(ctx, store, req.Turn)
	if err != nil {
		return nil, true, err
	}
	req.Turn = actualTurn
	scoped, err := store.WithWorkerLease(req.Turn)
	if err != nil {
		return nil, true, err
	}
	// Persistent execution identity must belong to this conversation and tool.
	execution, err := scoped.GetToolExecution(ctx, req.ExecutionID)
	if err != nil {
		return nil, true, err
	}
	if execution.ConversationID != req.Turn.ConversationID || execution.ToolName != req.Call.Function.Name {
		return nil, true, fmt.Errorf("execution identity mismatch")
	}
	var recorded map[string]any
	if err = json.Unmarshal([]byte(execution.ArgumentsJSON), &recorded); err != nil {
		return nil, true, err
	}
	delete(recorded, "exploration_intent_id")
	expected, _ := json.Marshal(recorded)
	actual, _ := json.Marshal(req.Call.Function.Arguments)
	if !bytes.Equal(expected, actual) {
		return nil, true, fmt.Errorf("execution arguments mismatch")
	}
	cfg, err := buildPentestAgentConfig(ctx, scoped, req.Turn.ProviderID, req.Turn.Model, "", req.Turn.ConversationID)
	if err != nil {
		return nil, true, err
	}
	if err = mcptools.RefreshTools(&cfg); err != nil {
		return nil, true, err
	}
	if role, roleErr := scoped.GetAgentRole(ctx, req.Turn.ConversationID); roleErr == nil {
		switch role.Role {
		case pentest.AgentRoleController:
			cfg.PromptRole = pentest.AgentRoleController
			cfg.ToolAllowlist = pentest.ControllerToolAllowlist()
		case pentest.AgentRolePlanner:
			cfg.PromptRole = pentest.AgentRolePlanner
			cfg.ToolAllowlist = pentest.PlannerToolAllowlist()
		case pentest.AgentRoleWorker:
			cfg.PromptRole = pentest.AgentRoleWorker
			cfg.ToolAllowlist = pentest.WorkerToolAllowlist()
		}
	}
	allowed := false
	for _, name := range cfg.ToolAllowlist {
		if name == req.Call.Function.Name {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, true, fmt.Errorf("platform tool is disabled")
	}
	if err = mcptools.AuthorizeTool(req.Call.Function.Name); err != nil {
		return nil, true, err
	}
	if match := toolguard.CheckToolCall(req.Call.Function.Name, req.Call.Function.Arguments); match != nil {
		return nil, true, toolguard.NewBlockedError(match)
	}
	cfg.Conversation, err = scoped.GetConversation(ctx, req.Turn.ConversationID)
	if err != nil {
		return nil, true, err
	}
	cfg.Project, err = scoped.GetProject(ctx, cfg.Conversation.ProjectID)
	if err != nil {
		return nil, true, err
	}
	cfg.ToolExecutionID = req.ExecutionID
	cfg.WorkerTurn = &req.Turn
	replay, stored, err := scoped.BeginWorkerToolRequest(ctx, req.ExecutionID, req.Turn.ID, req.Call.Function.Name, string(actual))
	if err != nil {
		return nil, true, err
	}
	if replay {
		var result workerToolResponse
		if err = json.Unmarshal([]byte(stored), &result); err != nil {
			return nil, true, err
		}
		if result.Error != "" {
			return result.Result, result.Handled, fmt.Errorf("%s", result.Error)
		}
		return result.Result, result.Handled, nil
	}
	call := req.Call
	var prepareErr error
	if _, creates := pentestOwnedScanCreateTools[call.Function.Name]; creates {
		prepareErr = prepareWorkerScanEvidence(ctx, scoped, req.Turn, req.ExecutionID, &call)
		args := map[string]any{}
		for k, v := range call.Function.Arguments {
			args[k] = v
		}
		conf := map[string]any{}
		if original, ok := args["config"].(map[string]any); ok {
			for k, v := range original {
				conf[k] = v
			}
		}
		if req.Turn.BatchItemID != "" {
			conf["aiBatchItemId"] = req.Turn.BatchItemID
		}
		conf["aiWorkerExecutionId"] = req.ExecutionID
		conf["aiWorkerConversationId"] = req.Turn.ConversationID
		args["config"] = conf
		call.Function.Arguments = args
	}
	var data any
	handled, runErr := true, prepareErr
	if runErr == nil {
		data, handled, runErr = mcptools.RunNative(ctx, cfg, call)
	}
	result := workerToolResponse{Result: data, Handled: handled}
	if runErr != nil {
		result.Error = runErr.Error()
	}
	body, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return nil, true, marshalErr
	}
	if err = scoped.FinishWorkerToolRequest(context.Background(), req.ExecutionID, string(body)); err != nil {
		return nil, true, fmt.Errorf("platform tool result not durably saved; reconcile %s: %w", req.ExecutionID, err)
	}
	return data, handled, runErr
}

func validateWorkerPlatformTurn(ctx context.Context, store *pentest.SQLStore, presented pentest.WorkerTurn) (pentest.WorkerTurn, error) {
	actual, err := store.WorkerTurnByID(ctx, presented.ID)
	if err != nil {
		return actual, err
	}
	if actual.ConversationID != presented.ConversationID || actual.NodeID != presented.NodeID || actual.Generation != presented.Generation {
		return actual, pentest.ErrWorkerLeaseLost
	}
	if actual.CancelRequested || actual.DeleteRequested {
		return actual, context.Canceled
	}
	// Verify lease/status using database time before rebuilding tool context.
	// BeginWorkerToolRequest fences it again before dispatching any side effect.
	_, err = store.WorkerStopRequested(ctx, actual)
	return actual, err
}
