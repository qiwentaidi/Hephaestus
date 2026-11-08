package ai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	artexdb "hephaestus-backend/lib/pentest/taskengine/db"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/config"

	"github.com/gin-gonic/gin"
)

var artexProjectionMu sync.Mutex
var artexGraphHashes sync.Map

// defaultOrchestrationWorkers 是智能编排未指定 Worker 并发时的默认值；硬上限 8
// 见下方 min(workers, 8)。可被任务级 hephaestus_task_options.workers 覆盖。
const defaultOrchestrationWorkers = 4

func nativeTaskID(runID string) string {
	return strings.TrimPrefix(runID, pentest.OrchestrationRunPrefix)
}
func nativeConversation(runID, role string) string { return runID + ":" + role }
func nativeNodeID(runID string, id int64) string   { return fmt.Sprintf("%s:node:%d", runID, id) }

func artexProfile(providerID, model string) (artexdb.LLMProfile, error) {
	cfg, err := config.GetAIConfig()
	if err != nil {
		return artexdb.LLMProfile{}, err
	}
	cfg = config.NormalizeAIConfig(cfg)
	id := resolvePentestPrimaryProviderID(cfg, providerID)
	cfg = resolvePentestAIProvider(cfg, id, model)
	if !cfg.Enabled || strings.TrimSpace(cfg.Key) == "" {
		return artexdb.LLMProfile{}, fmt.Errorf("AI provider is not configured")
	}
	format := "openai"
	if cfg.Protocol == config.AIProtocolAnthropic {
		format = "anthropic"
	}
	p := artexdb.LLMProfile{Name: "Hephaestus " + id + " " + cfg.Model, Format: format, Model: cfg.Model, APIKey: cfg.Key, BaseURL: cfg.API, Streaming: true, PoolExclude: true}
	p.HostProviderID = id
	for _, v := range cfg.Providers {
		if v.ID == id {
			p.Streaming = v.SupportsStreaming
			p.MaxTokens = v.MaxOutputTokens
			p.ContextWindowK = v.MaxTotalTokens / 1000
			p.RatePerMinute = float64(v.WorkbenchRequestsPerMinute)
			break
		}
	}
	return p, nil
}

func createARTEXPlan(c *gin.Context) {
	var req createPlanRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		nativeError(c, 400, err)
		return
	}
	targets := normalizePentestNonEmptyStrings(req.Targets)
	if len(targets) == 0 {
		nativeError(c, 400, fmt.Errorf("targets cannot be empty"))
		return
	}
	planner, err := artexProfile(req.PlannerProviderID, req.PlannerModel)
	if err != nil {
		nativeError(c, 400, err)
		return
	}
	worker, err := artexProfile(req.WorkerProviderID, req.WorkerModel)
	if err != nil {
		nativeError(c, 400, err)
		return
	}
	if req.MaxTotalWorkers > 0 {
		nativeError(c, 400, fmt.Errorf("智能编排不使用旧 Worker 总数预算，请设为 0"))
		return
	}
	rt, err := taskengine.Get(c.Request.Context())
	if err != nil {
		nativeError(c, 503, err)
		return
	}
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		nativeError(c, 500, err)
		return
	}
	ctx := c.Request.Context()
	scope := normalizePentestNonEmptyStrings(req.Scope)
	if len(scope) == 0 {
		scope = targets
	}
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		objective = "对选中资产进行授权渗透测试，梳理攻击面并验证风险"
	}
	name := firstNonEmptyPentest(req.ProjectName, "AI 编排 · "+targets[0])
	var project pentest.Project
	if req.ProjectID != "" {
		project, err = store.GetProject(ctx, req.ProjectID)
	} else {
		project, err = store.UpsertProject(ctx, pentest.Project{Name: name, Description: objective, ScopeJSON: marshalPlanScope(scope), Status: "active"})
	}
	if err != nil {
		nativeError(c, 400, err)
		return
	}
	workers := req.MaxWorkers
	if workers <= 0 {
		workers = defaultOrchestrationWorkers
	}
	workers = min(workers, 8)
	t, err := rt.Engine.PrepareHostTask(taskengine.HostTaskOptions{Name: project.Name, Description: "目标资产：" + strings.Join(targets, "\n") + "\n授权范围：" + strings.Join(scope, "\n"), Goal: objective, Planner: planner, Worker: worker, Workers: workers})
	if err != nil {
		nativeError(c, 500, err)
		return
	}
	run := pentest.PlanRun{ID: pentest.OrchestrationRunPrefix + t.ID, ProjectID: project.ID, Objective: objective, ScopeJSON: marshalPlanScope(scope), Status: "planning", PlannerProviderID: req.PlannerProviderID, PlannerModel: planner.Model, WorkerProviderID: req.WorkerProviderID, WorkerModel: worker.Model, MaxWorkers: workers}
	mapped := false
	defer func() {
		if mapped {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if e := rt.Engine.DeleteHostTask(cleanupCtx, t.ID); e != nil {
			log.Printf("[orchestration] rollback prepared task %s: %v", t.ID, e)
		}
		for _, role := range []string{"planner", "controller"} {
			if e := store.DeleteConversation(cleanupCtx, nativeConversation(run.ID, role)); e != nil {
				log.Printf("[orchestration] rollback conversation: %v", e)
			}
		}
	}()
	run.PlannerConversationID = nativeConversation(run.ID, "planner")
	run.ControllerConversationID = nativeConversation(run.ID, "controller")
	for _, role := range []string{"planner", "controller"} {
		id := nativeConversation(run.ID, role)
		title := "Planner · "
		if role == "controller" {
			title = "主控 · "
		}
		_, err = store.UpsertConversation(ctx, pentest.Conversation{ID: id, ProjectID: project.ID, Title: title + project.Name, Target: targets[0], ScopeJSON: run.ScopeJSON, Mode: pentest.SoloPentestMode, Status: "completed"})
		if err == nil {
			err = store.SetAgentRole(ctx, id, role, "", run.ID)
		}
		if err != nil {
			nativeError(c, 500, err)
			return
		}
	}
	run, err = store.CreatePlanRun(ctx, run)
	if err != nil {
		nativeError(c, 500, err)
		return
	}
	mapped = true
	if err = rt.Engine.StartHostTask(t.ID); err != nil {
		nativeError(c, 500, err)
		return
	}
	// Once admitted, a projection read failure must not look like task creation failed.
	projectionErr := syncARTEXRun(ctx, store, rt, run)
	if projectionErr != nil {
		log.Printf("[orchestration] initial projection %s: %v", run.ID, projectionErr)
	}
	if persisted, e := store.GetPlanRun(ctx, run.ID); e == nil {
		run = persisted
	}
	snapshot, err := planRunSnapshot(ctx, store, run)
	if err != nil {
		snapshot = map[string]any{"run": run, "project": planProjectDTO(project), "items": []any{}, "projectionPending": true}
	} else if projectionErr != nil {
		snapshot["projectionPending"] = true
	}
	c.JSON(200, gin.H{"success": true, "data": snapshot})
}
func nativeError(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"success": false, "error": err.Error()})
}

func nativeIntentState(state string) string {
	switch state {
	case "open":
		return "pending"
	case "running":
		return "in_progress"
	case "done":
		return "completed"
	case "deleted":
		return "stopped"
	}
	return state
}
func nativeRunState(state string) string {
	switch state {
	case "done":
		return "completed"
	case "paused":
		return "paused"
	case "cancelling":
		return "pausing"
	case "created", "running":
		return "planning"
	case "queued":
		return "waiting"
	case "timeout":
		return "blocked"
	}
	return state
}

// A finding name is distinct from its narrative summary. Never use the generic
// node-text fallback (which deliberately prefers summary for other node kinds).
func artexFindingTitle(payload map[string]any) string {
	summary, _ := payload["summary"].(string)
	summary = strings.TrimSpace(summary)
	for _, key := range []string{"name", "vulnclass"} {
		value, _ := payload[key].(string)
		value = strings.TrimSpace(value)
		if value != "" && value != summary {
			return value
		}
	}
	return "未命名漏洞"
}

func orchestrationRunTarget(run pentest.PlanRun) string {
	scope := parseScopeJSON(run.ScopeJSON)
	if len(scope) > 0 {
		return scope[0]
	}
	return ""
}

func syncARTEXRun(ctx context.Context, store *pentest.SQLStore, rt *taskengine.Runtime, run pentest.PlanRun) error {
	artexProjectionMu.Lock()
	defer artexProjectionMu.Unlock()
	t, dto, err := rt.Engine.HostTask(nativeTaskID(run.ID))
	if err != nil {
		return err
	}
	// Bound this pass before reading the graph. A concurrently-created intent
	// must not have its newer activity routed through a graph snapshot that has
	// not yet materialized that intent's Worker conversation.
	var activityCeiling int64
	if err = rt.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM activity WHERE exploration_id=$1`, t.ExpID).Scan(&activityCeiling); err != nil {
		return err
	}
	nodes, err := t.Store.Nodes(100000)
	if err != nil {
		return err
	}
	edges, err := t.Store.Edges(100000)
	if err != nil {
		return err
	}
	if len(nodes) >= 100000 || len(edges) >= 100000 {
		return fmt.Errorf("智能编排图谱投影达到上限，原始数据已保留")
	}
	// Reports live in the native findings table, outside the graph payload. Include
	// them in the projection hash so asynchronous report updates reach the UI.
	findingRows, err := rt.DB.QueryContext(ctx, rt.DB.Choose(`SELECT f.node_id,COALESCE(f.name,''),COALESCE(f.report,''),f.vulnclass,f.summary,f.evidence,
        COALESCE((SELECT string_agg(target, E'\n' ORDER BY first_seen) FROM (
            SELECT target, MIN(position) AS first_seen FROM (
                SELECT ids.position, COALESCE(NULLIF(a.url,''),
                    CASE WHEN a.port IS NOT NULL THEN
                        CASE WHEN COALESCE(NULLIF(a.domain,''),a.ip) LIKE '%:%'
                            THEN '[' || COALESCE(NULLIF(a.domain,''),a.ip) || ']'
                            ELSE COALESCE(NULLIF(a.domain,''),a.ip) END || ':' || a.port::text END,
                    NULLIF(a.domain,''),NULLIF(a.ip,''),NULLIF(a.bundle_id,''),NULLIF(a.app_name,'')) AS target
                FROM jsonb_array_elements_text(f.asset_ids) WITH ORDINALITY AS ids(id,position)
                JOIN assets a ON a.id=ids.id::bigint
            ) resolved WHERE target IS NOT NULL GROUP BY target
        ) targets),'')
        FROM findings f WHERE f.task_id=$1 AND f.node_id IS NOT NULL`, `SELECT f.node_id,COALESCE(f.name,''),COALESCE(f.report,''),f.vulnclass,f.summary,f.evidence,
        COALESCE((SELECT group_concat(target, char(10) ORDER BY first_seen) FROM (
            SELECT target, MIN(position) AS first_seen FROM (
                SELECT ids.key AS position, COALESCE(NULLIF(a.url,''),
                    CASE WHEN a.port IS NOT NULL THEN
                        CASE WHEN COALESCE(NULLIF(a.domain,''),a.ip) LIKE '%:%'
                            THEN '[' || COALESCE(NULLIF(a.domain,''),a.ip) || ']'
                            ELSE COALESCE(NULLIF(a.domain,''),a.ip) END || ':' || a.port::text END,
                    NULLIF(a.domain,''),NULLIF(a.ip,''),NULLIF(a.bundle_id,''),NULLIF(a.app_name,'')) AS target
                FROM json_each(f.asset_ids) AS ids
                JOIN assets a ON a.id=ids.value
            ) resolved WHERE target IS NOT NULL GROUP BY target
        ) targets),'')
        FROM findings f WHERE f.task_id=$1 AND f.node_id IS NOT NULL`), t.ID)
	if err != nil {
		return err
	}
	type reportFields struct{ name, report, class, summary, evidence, target string }
	reports := map[int64]reportFields{}
	for findingRows.Next() {
		var nodeID int64
		var fields reportFields
		if err = findingRows.Scan(&nodeID, &fields.name, &fields.report, &fields.class, &fields.summary, &fields.evidence, &fields.target); err != nil {
			findingRows.Close()
			return err
		}
		reports[nodeID] = fields
	}
	err = findingRows.Err()
	findingRows.Close()
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if fields, ok := reports[n.ID]; ok {
			var payload map[string]any
			if err = json.Unmarshal(n.Payload, &payload); err != nil {
				return err
			}
			if payload == nil {
				payload = map[string]any{}
			}
			payload["name"], payload["report"], payload["vulnclass"] = fields.name, fields.report, fields.class
			// Read the canonical standalone values, not the graph's {by,poc} envelope.
			payload["summary"], payload["evidence"] = fields.summary, fields.evidence
			if fields.target != "" {
				payload["target"] = fields.target
			}
			n.Payload, _ = json.Marshal(payload)
		}
	}
	owners := map[int64]string{}
	for _, n := range nodes {
		if n.Kind == "intent" {
			owners[n.ID] = nativeConversation(run.ID, fmt.Sprintf("intent:%d", n.ID))
		}
	}
	for _, e := range edges {
		if e.Rel == "yields" && owners[e.From] != "" {
			owners[e.To] = owners[e.From]
		}
	}
	hashBody, _ := json.Marshal([]any{nodes, edges, dto.Status, dto.Paused})
	hash := sha256.Sum256(hashBody)
	if old, ok := artexGraphHashes.Load(run.ID); !ok || old != hash {
		out := make([]pentest.ARTEXNode, 0, len(nodes))
		refs := map[int64]string{}
		// Plan-item execution metadata (start/finish/result) comes from the native
		// activity stream; it is read once per projection pass.
		executions, err := t.Store.IntentExecutions()
		if err != nil {
			return err
		}
		for _, n := range nodes {
			id := nativeNodeID(run.ID, n.ID)
			owner := owners[n.ID]
			if owner == "" {
				owner = run.PlannerConversationID
			}
			refs[n.ID] = n.Kind + ":" + id
			execution := executions[n.ID]
			var payload map[string]any
			_ = json.Unmarshal(n.Payload, &payload)
			title := taskengine.HostNodeText(n)
			if n.Kind == "finding" {
				title = artexFindingTitle(payload)
			}
			if n.Kind == "intent" {
				conv, e := store.GetConversation(ctx, owner)
				if e != nil {
					conv = pentest.Conversation{ID: owner, ProjectID: run.ProjectID, Title: title, Target: orchestrationRunTarget(run), ScopeJSON: run.ScopeJSON, Mode: pentest.SoloPentestMode}
				}
				conv.Status, _, _ = artexWorkerProjection(n.State, dto.Status, dto.Paused)
				if target := orchestrationRunTarget(run); target != "" {
					conv.Target = target
				}
				conv.ScopeJSON = run.ScopeJSON
				if n.State == "stopped" && payload["disposition"] != nil {
					conv.Status = "cancelled"
				}
				if _, err = store.UpsertConversation(ctx, conv); err != nil {
					return err
				}
				if err = store.SetAgentRole(ctx, owner, pentest.AgentRoleWorker, run.PlannerConversationID, run.ID); err != nil {
					return err
				}
			}
			projectedState, reason := nativeIntentState(n.State), n.BlockedReason
			if n.Kind == "intent" {
				_, projectedState, reason = artexWorkerProjection(n.State, dto.Status, dto.Paused)
				if reason == "" || payload["disposition"] != nil {
					reason = n.BlockedReason
				}
				reason = intentTerminalReasonForPlan(n.State, reason, execution)
				if payload == nil {
					payload = map[string]any{}
				}
				payload["native_state"] = n.State
				if reason != "" {
					payload["status_reason"] = reason
				}
			}
			projected := pentest.ARTEXNode{ID: id, Kind: n.Kind, Title: title, State: projectedState, Body: prettyNativePayload(payload), OwnerConversation: owner, Reason: reason, Priority: n.Priority, CreatedAt: n.CreatedAt, Payload: payload}
			if n.Kind == "intent" {
				projected.StartedAt, projected.CompletedAt, projected.ResultSummary = execution.StartedAt, execution.CompletedAt, execution.Result
			}
			out = append(out, projected)
		}
		relations := make([]pentest.ExplorationRelation, 0, len(edges))
		for _, e := range edges {
			if refs[e.From] != "" && refs[e.To] != "" {
				relations = append(relations, pentest.ExplorationRelation{Source: refs[e.From], Target: refs[e.To], Relation: e.Rel, ConversationID: run.PlannerConversationID})
			}
		}
		if err = store.SyncARTEXGraph(ctx, run, out, relations); err != nil {
			return err
		}
		artexGraphHashes.Store(run.ID, hash)
	}
	var cursor int64
	if err = rt.DB.QueryRowContext(ctx, `SELECT activity_cursor FROM hephaestus_task_options WHERE task_id=$1`, t.ID).Scan(&cursor); err != nil {
		return err
	}
	for {
		records, next, e := rt.Engine.HostActivities(t.ID, cursor)
		if e != nil {
			return e
		}
		for _, a := range records {
			if a.ID > activityCeiling {
				next = activityCeiling
				break
			}
			conv := run.PlannerConversationID
			if a.Worker == "mainagent" {
				conv = nativeConversation(run.ID, "controller")
			} else if a.NodeID != nil && owners[*a.NodeID] != "" {
				conv = owners[*a.NodeID]
			}
			if err = projectARTEXActivity(ctx, store, run, conv, a); err != nil {
				return err
			}
		}
		if next > cursor {
			if _, err = rt.DB.ExecContext(ctx, `UPDATE hephaestus_task_options SET activity_cursor=$1 WHERE task_id=$2`, next, t.ID); err != nil {
				return err
			}
			cursor = next
		}
		if cursor >= activityCeiling || len(records) < 500 {
			break
		}
	}
	status := nativeRunState(dto.Status)
	if status != run.Status || ((status == "paused" || status == "pausing") && run.CompletedAt != nil) {
		if err = store.SyncARTEXRunStatus(ctx, run.ID, status, run.Summary, dto.LLMFailoverReason); err != nil {
			return err
		}
	}
	for _, role := range []string{"planner", "controller"} {
		conv, e := store.GetConversation(ctx, nativeConversation(run.ID, role))
		if e != nil {
			return e
		}
		next := "completed"
		if role == "controller" {
			if rt.Engine.HostChatBusy(t.ID) {
				next = "running"
			}
		} else {
			switch status {
			case "planning", "waiting":
				next = "running"
			case "pausing":
				next = "cancelling"
			case "paused":
				next = "paused"
			case "failed", "blocked":
				next = status
			}
		}
		if conv.Status != next {
			conv.Status = next
			if _, err = store.UpsertConversation(ctx, conv); err != nil {
				return err
			}
		}
	}
	return nil
}

// Parent lifecycle controls presentation only; native intent records are preserved.
func artexWorkerProjection(state, parent string, paused bool) (conversation, intent, reason string) {
	conversation, intent = artexConversationState(state), nativeIntentState(state)
	active := state == "open" || state == "running" || state == "paused"
	terminal := parent == "done" || parent == "failed" || parent == "timeout"
	if terminal && active {
		if state == "open" {
			return "not_executed", "stopped", "未执行 · 任务已结束"
		}
		// A terminal task can briefly retain running children while cancellation drains.
		if state == "running" {
			return "cancelling", "in_progress", "任务已结束，等待 Worker 退出确认"
		}
		return "task_stopped", "stopped", "已停止 · 任务已结束"
	}
	if terminal && state == "stopped" {
		if parent == "done" {
			return "goal_stopped", "stopped", "已停止 · 目标已达成"
		}
		return "task_stopped", "stopped", "已停止 · 任务已结束"
	}
	if parent == "cancelling" && active {
		return "cancelling", intent, "等待 Worker 退出确认"
	}
	if paused && active {
		return "paused", "paused", "任务已暂停"
	}
	return conversation, intent, ""
}

// intentTerminalReasonForPlan returns the operator-facing explanation of an
// intent that ended blocked/exhausted. New runs persist an exact reason in the
// native graph; rows written before that only kept the internal quota code or
// nothing at all, so the Plan projection fills a concrete fallback for them.
func intentTerminalReasonForPlan(state, reason string, execution artexdb.IntentExecution) string {
	if reason == artexdb.IntentBlockedLLMQuota {
		return "模型链路配额或重试已用尽，Worker 未能继续执行"
	}
	if reason != "" {
		return reason
	}
	switch state {
	case "blocked":
		if execution.ResultFailed {
			return "Worker 执行失败结束，错误详情见结果摘要与 Worker 会话"
		}
		return "Worker 执行失败结束，未记录具体错误；可在 Worker 会话中查看执行过程"
	case "exhausted":
		return "达到运行预算上限（步数或时长），已收尾并写回事实"
	}
	return ""
}

func artexConversationState(state string) string {
	switch state {
	case "open":
		return "queued"
	case "running":
		return "running"
	case "done":
		return "completed"
	case "paused":
		return "paused"
	case "stopped", "deleted":
		return "cancelled"
	case "exhausted":
		return "blocked"
	}
	return state
}
func prettyNativePayload(p map[string]any) string {
	b, _ := json.MarshalIndent(p, "", "  ")
	return string(b)
}

func projectARTEXActivity(ctx context.Context, store *pentest.SQLStore, run pentest.PlanRun, conv string, a artexdb.Activity) error {
	if a.Kind == "usage" {
		return nil
	}
	id := fmt.Sprintf("%s:activity:%d", run.ID, a.ID)
	content := firstNonEmptyPentest(a.Detail, a.Summary)
	role := "progress"
	p := pentestProgressPayload{RunID: run.ID, BlockID: id, Sequence: int(a.ID), Status: "completed", Detail: content, Label: a.Summary, ToolName: a.Tool, ToolCallID: a.ToolUseID, UpdatedAt: a.CreatedAt.Format(time.RFC3339Nano)}
	switch a.Kind {
	case "user":
		role = "user"
	case "text", "result":
		role = "assistant"
	case "thinking":
		p.Type = "reasoning"
		p.Label = "推理过程"
	case "tool_use":
		p.Type = "tool_start"
		p.Status = "running"
		p.Label = "正在调用工具：" + a.Tool
	case "tool_result":
		p.Type = "tool_done"
		p.Label = "工具调用完成：" + a.Tool
		if a.IsError {
			p.Status = "failed"
		}
	default:
		p.Type = "assistant_commentary"
	}
	if a.ToolUseID != "" && (a.Kind == "tool_use" || a.Kind == "tool_result") {
		execID := conv + ":tool:" + a.ToolUseID
		exec, e := store.GetToolExecution(ctx, execID)
		if e != nil {
			exec = pentest.ToolExecution{ID: execID, ConversationID: conv, ToolCallID: a.ToolUseID, ToolName: a.Tool, StartTime: a.CreatedAt}
		}
		if a.Kind == "tool_use" {
			exec.ArgumentsJSON = content
			exec.Status = "running"
		} else {
			exec.Status = p.Status
			exec.EndTime = &a.CreatedAt
			exec.ResultJSON = content
			if a.IsError {
				exec.Error = content
			}
		}
		if a.NodeID != nil {
			exec.IntentID = nativeNodeID(run.ID, *a.NodeID)
		}
		if _, err := store.SaveARTEXToolExecution(ctx, run.ID, exec); err != nil {
			return err
		}
		p.ExecutionID = execID
	}
	if role == "progress" {
		b, _ := json.Marshal(p)
		content = string(b)
	}
	if strings.TrimSpace(content) != "" {
		if _, err := store.AddMessage(ctx, pentest.Message{ID: id, ConversationID: conv, Role: role, Content: content, CreatedAt: a.CreatedAt}); err != nil {
			return err
		}
	}
	if a.Kind == "result" && (a.InputTokens != nil || a.OutputTokens != nil) {
		val := func(p *int) int64 {
			if p == nil {
				return 0
			}
			return int64(*p)
		}
		usage := pentest.TokenUsage{InputTokens: val(a.InputTokens), OutputTokens: val(a.OutputTokens), CacheReadTokens: val(a.CacheReadTokens)}
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		messages, err := store.ListMessages(ctx, conv, -1)
		if err != nil {
			return err
		}
		_, turns := buildPentestChatForStatus(messages, "")
		if err := store.RecordTurnTokenUsageDetail(ctx, id, conv, artexMessageTurnIDs(turns)[id], usage); err != nil {
			return err
		}
	}
	return nil
}

// Use the same turn boundaries as the snapshot, including Workers whose first
// message is an assistant message. Never infer a turn from a lifetime total.
func artexMessageTurnIDs(turns []gin.H) map[string]string {
	ids := make(map[string]string)
	for _, turn := range turns {
		id, _ := turn["id"].(string)
		if view, ok := turn["userMessageView"].(gin.H); ok {
			messageID, _ := view["id"].(string)
			ids[messageID] = id
		}
		if views, ok := turn["assistantMessageViews"].([]gin.H); ok {
			for _, view := range views {
				messageID, _ := view["id"].(string)
				ids[messageID] = id
			}
		}
	}
	return ids
}

func syncARTEXConversation(ctx context.Context, store *pentest.SQLStore, conversationID string) error {
	role, err := store.GetAgentRole(ctx, conversationID)
	if err != nil {
		return err
	}
	if !pentest.IsOrchestrationRun(role.PlanRunID) {
		return nil
	}
	rt, err := taskengine.Get(ctx)
	if err != nil {
		return err
	}
	run, err := store.GetPlanRun(ctx, role.PlanRunID)
	if err != nil {
		return err
	}
	return syncARTEXRun(ctx, store, rt, run)
}

func handleARTEXMessage(c *gin.Context, req pentestMessageRequest, stream bool) bool {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		return false
	}
	role, err := store.GetAgentRole(c.Request.Context(), c.Param("id"))
	if err != nil || !pentest.IsOrchestrationRun(role.PlanRunID) {
		return false
	}
	if role.Role != pentest.AgentRoleController {
		nativeError(c, 409, fmt.Errorf("Planner/Worker 会话只读，请使用主控"))
		return true
	}
	if req.EditMessageID != "" {
		nativeError(c, 409, fmt.Errorf("主控历史不可改写，请发送补充消息"))
		return true
	}
	rt, err := taskengine.Get(c.Request.Context())
	if err != nil {
		nativeError(c, 503, err)
		return true
	}
	if req.AgentProviderID != "" || req.AgentModel != "" {
		profile, e := artexProfile(req.AgentProviderID, req.AgentModel)
		if e != nil {
			nativeError(c, 400, e)
			return true
		}
		if e = rt.Engine.ConfigureHostChatProfile(nativeTaskID(role.PlanRunID), req.AgentProviderID, profile); e != nil {
			nativeError(c, 409, e)
			return true
		}
	}
	content, err := nativeMessageContent(c.Request.Context(), rt, role.PlanRunID, c.Param("id"), req.Content, req.Metadata)
	if err != nil {
		nativeError(c, 400, err)
		return true
	}
	if strings.TrimSpace(content) == "" {
		nativeError(c, 400, fmt.Errorf("content is required"))
		return true
	}
	if err = rt.Engine.HostChat(nativeTaskID(role.PlanRunID), content); err != nil {
		nativeError(c, 409, err)
		return true
	}
	if stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			snap, e := buildPentestLiveSessionSnapshot(c.Request.Context(), store, c.Param("id"), "", "")
			if e != nil {
				writeSSE(c, "error", gin.H{"error": e.Error()})
				return true
			}
			writeSSE(c, "session_snapshot", gin.H{"session": snap})
			if !rt.Engine.HostChatBusy(nativeTaskID(role.PlanRunID)) {
				writeSSE(c, "done", gin.H{"session": snap})
				return true
			}
			select {
			case <-c.Request.Context().Done():
				return true
			case <-tick.C:
			}
		}
	}
	snap, err := buildPentestLiveSessionSnapshot(c.Request.Context(), store, c.Param("id"), "", "")
	if err != nil {
		nativeError(c, 500, err)
	} else {
		c.JSON(200, gin.H{"success": true, "data": snap})
	}
	return true
}

func cancelARTEXPlan(c *gin.Context, store *pentest.SQLStore, run pentest.PlanRun) {
	rt, err := taskengine.Get(c.Request.Context())
	if err != nil {
		nativeError(c, 503, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	pauseErr := rt.Engine.PauseHostTask(ctx, nativeTaskID(run.ID))
	if pauseErr != nil && !errors.Is(pauseErr, context.DeadlineExceeded) && !errors.Is(pauseErr, context.Canceled) {
		nativeError(c, 500, pauseErr)
		return
	}
	if err = syncARTEXRun(c.Request.Context(), store, rt, run); err != nil {
		nativeError(c, 500, err)
		return
	}
	run, _ = store.GetPlanRun(c.Request.Context(), run.ID)
	snap, err := planRunSnapshot(c.Request.Context(), store, run)
	if err != nil {
		nativeError(c, 500, err)
		return
	}
	if pauseErr != nil {
		snap["stopPending"] = true
	}
	c.JSON(200, gin.H{"success": true, "data": snap})
}

// Kept separate from the legacy worker queue: only native task state drives it.
func reconcileARTEX(ctx context.Context, store *pentest.SQLStore) {
	rt, err := taskengine.Get(ctx)
	if err != nil {
		return
	}
	rows, err := rt.DB.QueryContext(ctx, `SELECT task_id FROM hephaestus_task_options`)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, strconv.FormatInt(id, 10))
		}
	}
	rows.Close()
	for _, id := range ids {
		run, e := store.GetPlanRun(ctx, pentest.OrchestrationRunPrefix+id)
		if e == nil {
			if err := syncARTEXRun(ctx, store, rt, run); err != nil {
				log.Printf("[orchestration] projection task %s: %v", id, err)
			}
		}
	}
}

func handleARTEXCancel(c *gin.Context, store *pentest.SQLStore) bool {
	role, err := store.GetAgentRole(c.Request.Context(), c.Param("id"))
	if err != nil || role.PlanRunID == "" {
		return false
	}
	if !pentest.IsOrchestrationRun(role.PlanRunID) {
		nativeError(c, 409, pentest.ErrLegacyOrchestrationReadOnly)
		return true
	}
	if role.Role != pentest.AgentRoleController {
		nativeError(c, 409, fmt.Errorf("请从编排任务入口暂停整个任务"))
		return true
	}
	rt, err := taskengine.Get(c.Request.Context())
	if err != nil {
		nativeError(c, 503, err)
		return true
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	stopErr := rt.Engine.StopHostChat(ctx, nativeTaskID(role.PlanRunID))
	snap, err := buildPentestLiveSessionSnapshot(c.Request.Context(), store, c.Param("id"), "", "")
	if err != nil {
		nativeError(c, 500, err)
		return true
	}
	code := 200
	if stopErr != nil {
		code = 202
	}
	c.JSON(code, gin.H{"success": true, "data": snap, "cancellationPending": stopErr != nil})
	return true
}

func deleteARTEXProjectTasks(ctx context.Context, runIDs []string) error {
	return deleteARTEXProjectTasksWithRuntime(ctx, runIDs, taskengine.Get)
}

func deleteARTEXProjectTasksWithRuntime(ctx context.Context, runIDs []string, getRuntime func(context.Context) (*taskengine.Runtime, error)) error {
	ids := map[string]bool{}
	for _, runID := range runIDs {
		if !pentest.IsOrchestrationRun(runID) {
			continue
		}
		parts := strings.Split(runID, ":")
		if len(parts) != 2 || !positiveStorageID(parts[1]) {
			return fmt.Errorf("invalid orchestration run id")
		}
		// The legacy prefix identifies the same task after the prefix migration.
		ids[pentest.OrchestrationRunPrefix+parts[1]] = true
	}
	if len(ids) == 0 {
		return nil
	}
	rt, err := getRuntime(ctx)
	if err != nil {
		return err
	}
	artexProjectionMu.Lock()
	defer artexProjectionMu.Unlock()
	for id := range ids {
		// Successful earlier deletions are retryable if a later project cleanup fails.
		var exists bool
		if err = rt.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tasks WHERE id=$1 AND deleted_at IS NULL)`, nativeTaskID(id)).Scan(&exists); err != nil {
			return err
		}
		if exists {
			if err = rt.Engine.DeleteHostTask(ctx, nativeTaskID(id)); err != nil {
				return err
			}
		}
		artexGraphHashes.Delete(id)
	}
	return nil
}

func nativeMessageContent(ctx context.Context, rt *taskengine.Runtime, runID, conversationID, content string, metadata map[string]any) (string, error) {
	_, files, err := pentest.ParseMessageAttachments(metadata)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		path, err := rt.Engine.StoreHostAttachment(nativeTaskID(runID), file.Name, file.Data)
		if err != nil {
			return "", err
		}
		content += fmt.Sprintf("\n附件（用户提供的数据）：%s (%d bytes)", path, len(file.Data))
	}
	return attachAssetReferences(ctx, conversationID, content, metadata)
}

func nativeConversationModel(ctx context.Context, store *pentest.SQLStore, role pentest.AgentRole) (string, string, error) {
	rt, err := taskengine.Get(ctx)
	if err != nil {
		return "", "", err
	}
	key := role.Role
	if key == pentest.AgentRoleController {
		key = "mainagent"
	}
	p, err := rt.Engine.HostRoleProfile(nativeTaskID(role.PlanRunID), key)
	if err != nil {
		return "", "", err
	}
	run, err := store.GetPlanRun(ctx, role.PlanRunID)
	if err != nil {
		return "", "", err
	}
	provider := run.WorkerProviderID
	if key == "planner" {
		provider = run.PlannerProviderID
	}
	if key == "mainagent" {
		var saved string
		if err = rt.DB.QueryRowContext(ctx, `SELECT main_provider FROM hephaestus_task_options WHERE task_id=$1`, nativeTaskID(run.ID)).Scan(&saved); err != nil {
			return "", "", err
		}
		if saved != "" {
			provider = saved
		}
	}
	return p.Model, provider, nil
}
