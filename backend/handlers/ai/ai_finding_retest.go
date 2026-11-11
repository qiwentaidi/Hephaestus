package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	aipkg "hephaestus-backend/pkg/ai"
)

func pentestFindingRetestToolSchemas() []pentest.AgentToolSchema {
	return []pentest.AgentToolSchema{
		{
			Type: "function",
			Function: pentest.AgentToolFunction{
				Name:        "retest_vulnerability",
				Description: "读取指定漏洞的原始复现步骤、证据、来源会话和目标，准备一次修复后的回归复测。仅在需要判断修复状态或用户明确要求复测时使用；补充同一漏洞的新证明请使用 update_vulnerability。该工具不会替代实际验证请求；由执行复测的会话使用 record_vulnerability_retest 写回原漏洞。",
				Parameters:  pentestNativeObjectSchema(map[string]any{"findingId": map[string]any{"type": "string", "description": "漏洞 ID。必须来自当前项目或当前会话的真实漏洞记录。"}}, "findingId"),
			},
		},
		{
			Type: "function",
			Function: pentest.AgentToolFunction{
				Name:        "record_vulnerability_retest",
				Description: "保存一次修复后的回归复测结论和证据引用。不会覆盖原始漏洞证据或人工处置状态；补充同一漏洞的新证明不应调用此工具。result 使用 still_vulnerable、fixed 或 inconclusive。",
				Parameters: pentestNativeObjectSchema(map[string]any{
					"findingId":     map[string]any{"type": "string", "description": "被复测的漏洞 ID。"},
					"result":        map[string]any{"type": "string", "enum": []string{"still_vulnerable", "fixed", "inconclusive"}, "description": "复测结论。"},
					"summary":       map[string]any{"type": "string", "description": "简明复测结论，说明实际观察和边界。"},
					"evidence":      map[string]any{"type": "string", "description": "复测请求/响应、命令输出或其他原始证据；缺少证据时明确说明。"},
					"executionRefs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "支撑本次复测的真实工具执行 ID 列表；只能引用当前会话 status=completed 的执行。编排 Worker 也可传本次工具调用的 tool_use_id，系统会解析为当前会话的执行 ID。Bash 即使产生了有用输出，只要最终退出码非 0 也不能引用，请重新运行一个退出码为 0 的证据命令。"},
				}, "findingId", "result", "summary"),
			},
		},
	}
}

func pentestReportToolSchema() pentest.AgentToolSchema {
	return pentest.AgentToolSchema{
		Type: "function",
		Function: pentest.AgentToolFunction{
			Name:        "generate_target_test_report",
			Description: "汇总当前会话中已持久化的目标、事实、漏洞、复测记录和工具证据，生成可交付的 Markdown 目标测试报告。只使用真实记录，缺失内容必须标注为未验证或未覆盖；完成后返回可下载的报告附件。",
			Parameters: pentestNativeObjectSchema(map[string]any{
				"title": map[string]any{"type": "string", "description": "可选的报告标题。"},
			}),
		},
	}
}

func generatePentestReport(ctx context.Context, cfg pentest.AgentLoopConfig, args map[string]any) (any, bool, error) {
	if cfg.Store == nil {
		return nil, true, fmt.Errorf("pentest store is not configured")
	}
	if cfg.Project.ID == "" && cfg.Conversation.ID != "" {
		if conversation, conversationErr := cfg.Store.GetConversation(ctx, cfg.Conversation.ID); conversationErr == nil {
			cfg.Project.ID = conversation.ProjectID
		}
	}
	facts, err := cfg.Store.ListFacts(ctx, cfg.Project.ID, pentest.FactFilter{SourceConversationID: cfg.Conversation.ID, IncludeDeprecated: true, Limit: 1000})
	if err != nil {
		return nil, true, err
	}
	edges, err := cfg.Store.ListEdges(ctx, cfg.Project.ID)
	if err != nil {
		return nil, true, err
	}
	edges = pentest.FilterEdgesForFacts(edges, facts)
	findings, err := cfg.Store.ListFindingsByConversation(ctx, cfg.Project.ID, cfg.Conversation.ID, 1000)
	if err != nil {
		return nil, true, err
	}
	retests := make(map[string][]pentest.FindingRetest, len(findings))
	for _, finding := range findings {
		items, listErr := cfg.Store.ListFindingRetests(ctx, finding.ID)
		if listErr != nil {
			return nil, true, listErr
		}
		retests[finding.ID] = items
	}
	messages, err := cfg.Store.ListMessages(ctx, cfg.Conversation.ID, 1000)
	if err != nil {
		return nil, true, err
	}
	title, _ := args["title"].(string)
	report := buildPentestReport(cfg.Project, cfg.Conversation, facts, edges, findings, messages, retests)
	if strings.TrimSpace(title) != "" {
		lines := strings.SplitN(report, "\n", 2)
		report = "# " + strings.TrimSpace(title)
		if len(lines) == 2 {
			report += "\n" + lines[1]
		}
	}
	return gin.H{
		"format":       "markdown",
		"content":      report,
		"findingCount": len(findings),
		"attachment": gin.H{
			"name":        reportAttachmentName(cfg.Conversation),
			"downloadUrl": "/api/v1/ai/sessions/" + cfg.Conversation.ID + "/report/download",
			"mimeType":    "text/markdown",
		},
	}, true, nil
}

func reportAttachmentName(conv pentest.Conversation) string {
	name := firstNonEmptyPentest(conv.Target, conv.Title, "target-test")
	name = strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(name)
	return name + "-pentest-report.md"
}

func DownloadAIPentestReportHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	snapshot, err := buildPentestSessionSnapshot(c.Request.Context(), store, c.Param("id"), "", "")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	preview, ok := snapshot["reportPreview"].(gin.H)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "report preview unavailable"})
		return
	}
	content, ok := preview["content"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "report content unavailable"})
		return
	}
	conv, err := store.GetConversation(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	attachmentName := reportAttachmentName(conv)
	if executions, executionErr := store.ListToolExecutionsByConversation(c.Request.Context(), conv.ID, 1000); executionErr == nil {
		for i := len(executions) - 1; i >= 0; i-- {
			if executions[i].ToolName != "generate_target_test_report" || executions[i].Status != pentest.ToolExecutionStatusCompleted {
				continue
			}
			var observation struct {
				Result map[string]any `json:"result"`
			}
			if json.Unmarshal([]byte(executions[i].ResultJSON), &observation) == nil {
				if generated, generatedOK := observation.Result["content"].(string); generatedOK && generated != "" {
					content = generated
				}
				if attachment, attachmentOK := observation.Result["attachment"].(map[string]any); attachmentOK {
					if generatedName, generatedNameOK := attachment["name"].(string); generatedNameOK && generatedName != "" {
						attachmentName = generatedName
					}
				}
			}
			break
		}
	}
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	safeAttachmentName := strings.NewReplacer("\"", "_", "\r", "_", "\n", "_").Replace(attachmentName)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", safeAttachmentName, url.PathEscape(safeAttachmentName)))
	c.String(http.StatusOK, content)
}

// Refresh native executions before validating references, independently of
// whether anyone currently has the Workbench open. Replaced by offline tests.
var syncFindingRetestConversation = syncARTEXConversation

func runPentestFindingRetestTool(ctx context.Context, cfg pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error) {
	if cfg.Store == nil {
		return nil, true, fmt.Errorf("pentest store is not configured")
	}
	if cfg.Project.ID == "" && cfg.Conversation.ID != "" {
		if conversation, conversationErr := cfg.Store.GetConversation(ctx, cfg.Conversation.ID); conversationErr == nil {
			cfg.Project.ID = conversation.ProjectID
		}
	}
	var args struct {
		FindingID     string   `json:"findingId"`
		Result        string   `json:"result"`
		Summary       string   `json:"summary"`
		Evidence      string   `json:"evidence"`
		ExecutionRefs []string `json:"executionRefs"`
	}
	if call.Function.Name == "generate_target_test_report" {
		return generatePentestReport(ctx, cfg, call.Function.Arguments)
	}
	if err := decodePentestToolArgs(call.Function.Arguments, &args); err != nil {
		return nil, true, err
	}
	args.FindingID = strings.TrimSpace(args.FindingID)
	if args.FindingID == "" {
		return nil, true, fmt.Errorf("findingId 不能为空")
	}
	finding, err := cfg.Store.GetFinding(ctx, args.FindingID)
	if err != nil {
		return nil, true, err
	}
	if finding.ProjectID != cfg.Project.ID {
		return nil, true, fmt.Errorf("finding 不属于当前项目")
	}
	role, err := cfg.Store.GetAgentRole(ctx, cfg.Conversation.ID)
	if err != nil {
		return nil, true, err
	}
	orchestrated := pentest.IsOrchestrationRun(role.PlanRunID)
	if orchestrated {
		if err := syncFindingRetestConversation(ctx, cfg.Store, cfg.Conversation.ID); err != nil {
			return nil, true, fmt.Errorf("同步复测执行记录失败: %w", err)
		}
	}
	switch call.Function.Name {
	case "retest_vulnerability":
		retests, err := cfg.Store.ListFindingRetests(ctx, finding.ID)
		if err != nil {
			return nil, true, err
		}
		instruction := "仅在授权范围内对该 findingId 做一次最小化复测；不要把一次请求失败解释为漏洞已修复。由实际执行复测的会话调用 record_vulnerability_retest，关联原 findingId，保留原始证据和人工处置状态，不重复创建漏洞。executionRefs 只能引用当前会话 status=completed 的新执行；exit_nonzero、blocked、cancelled 或 timeout 即使有部分输出也不能引用。"
		if orchestrated {
			instruction += "编排会话可传当前工具调用的 tool_use_id，系统自动解析为本会话执行 ID。"
			if role.Role == pentest.AgentRoleController {
				instruction += "主 Agent 使用 add_intent 派发一条专门的复测意图，把 findingId、目标、来源和原始证据及写回要求交给新的 Worker；不新增全局目标、不扩展扫描。由该 Worker 保存复测结果，主 Agent 不得将其他会话的执行引用当成本会话证据。"
			}
		}
		var sourceScope []string
		if finding.ConversationID != "" {
			source, sourceErr := cfg.Store.GetConversation(ctx, finding.ConversationID)
			if sourceErr != nil && !errors.Is(sourceErr, sql.ErrNoRows) {
				return nil, true, sourceErr
			}
			if sourceErr == nil && source.ProjectID == finding.ProjectID {
				_ = json.Unmarshal([]byte(source.ScopeJSON), &sourceScope)
			}
		}
		return gin.H{
			"findingId":            finding.ID,
			"sourceConversationId": finding.ConversationID,
			"sourceFactKey":        finding.SourceFactKey,
			"sourceScope":          sourceScope,
			"title":                finding.Title,
			"severity":             finding.Severity,
			"target":               finding.Target,
			"vulnerabilityType":    finding.VulnerabilityType,
			"preconditions":        finding.Preconditions,
			"reproductionSteps":    finding.ReproductionSteps,
			"originalEvidence":     finding.Evidence,
			"impact":               finding.Impact,
			"recommendation":       finding.Recommendation,
			"existingRetestNote":   finding.RetestNotes,
			"retests":              retests,
			"instruction":          instruction,
		}, true, nil
	case "record_vulnerability_retest":
		if strings.TrimSpace(args.Result) == "" || strings.TrimSpace(args.Summary) == "" {
			return nil, true, fmt.Errorf("result 和 summary 不能为空")
		}
		for i, ref := range args.ExecutionRefs {
			ref = strings.TrimSpace(ref)
			execution, executionErr := cfg.Store.GetToolExecution(ctx, ref)
			if orchestrated && errors.Is(executionErr, sql.ErrNoRows) {
				execution, executionErr = cfg.Store.GetToolExecution(ctx, cfg.Conversation.ID+":tool:"+ref)
			}
			if executionErr != nil {
				return nil, true, fmt.Errorf("executionRef %q 不存在", ref)
			}
			if execution.ConversationID != cfg.Conversation.ID {
				return nil, true, fmt.Errorf("executionRef %q 不属于当前会话", ref)
			}
			if execution.Status != pentest.ToolExecutionStatusCompleted {
				detail := []string{"status=" + firstNonEmptyPentest(execution.Status, "unknown")}
				if execution.FailureKind != "" {
					detail = append(detail, "failure_kind="+execution.FailureKind)
				}
				if execution.Error != "" {
					detail = append(detail, "error="+execution.Error)
				}
				return nil, true, fmt.Errorf("executionRef %q 不是当前会话已完成的执行（%s）；请重新运行一个退出码为 0 的证据命令后再引用", ref, strings.Join(detail, "; "))
			}
			args.ExecutionRefs[i] = execution.ID
		}
		stored, err := cfg.Store.RecordFindingRetest(ctx, pentest.FindingRetest{
			FindingID: args.FindingID, ConversationID: cfg.Conversation.ID, Result: args.Result,
			Summary: args.Summary, Evidence: args.Evidence, ExecutionRefs: args.ExecutionRefs,
		})
		if err != nil {
			return nil, true, err
		}
		return stored, true, nil
	default:
		return nil, false, nil
	}
}
