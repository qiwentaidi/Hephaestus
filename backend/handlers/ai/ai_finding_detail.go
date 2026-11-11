package ai

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/findings"
	"hephaestus-backend/pkg/logger"
)

func GetAIPentestFindingDetailHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		findingDetailError(c, err)
		return
	}
	data, err := buildPentestFindingDetail(c.Request.Context(), store, c.Param("findingId"))
	if err != nil {
		findingDetailError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func GetAIPentestFindingLineageHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		findingDetailError(c, err)
		return
	}
	lineage, err := store.GetFindingLineage(c.Request.Context(), c.Param("findingId"))
	if err != nil {
		findingDetailError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": lineage})
}

func UpdateAIPentestFindingStatusHandler(c *gin.Context) {
	var input struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid status request"})
		return
	}
	switch input.Status {
	case "open", "in_progress", "confirmed", "fixed", "false_positive", "ignored", "accepted_risk":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid finding status"})
		return
	}
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		findingDetailError(c, err)
		return
	}
	finding, err := store.UpdateFindingStatus(c.Request.Context(), c.Param("findingId"), input.Status)
	if err != nil {
		findingDetailError(c, err)
		return
	}
	if db, dbErr := database.GetDB(); dbErr == nil {
		if statusErr := findings.UpdateStatusBySourceID(db, "ai", finding.ID, input.Status); statusErr != nil && !errors.Is(statusErr, sql.ErrNoRows) {
			logger.Warning("同步 AI 漏洞归因状态失败: %v", statusErr)
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": pentestFindingView(finding)})
}

func findingDetailError(c *gin.Context, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "finding not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
}

var findingExecutionRefPattern = regexp.MustCompile(`(?i)(?:tool_call:|execution_id["'\s:=]+)([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

func buildPentestFindingDetail(ctx context.Context, store *pentest.SQLStore, id string) (gin.H, error) {
	finding, err := store.GetFinding(ctx, id)
	if err != nil {
		return nil, err
	}
	project, err := store.GetProject(ctx, finding.ProjectID)
	if err != nil {
		return nil, err
	}
	view := pentestFindingView(finding)
	retestTarget, err := resolveFindingRetestTarget(ctx, store, finding)
	if err != nil {
		return nil, err
	}
	var conversation any
	if finding.ConversationID != "" {
		conv, e := store.GetConversation(ctx, finding.ConversationID)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return nil, e
		}
		if e == nil {
			conversation = pentestConversationView(conv)
		}
	}
	linked, err := store.ListFacts(ctx, finding.ProjectID, pentest.FactFilter{RelatedFindingID: finding.ID, IncludeDeprecated: true, Limit: 1000})
	if err != nil {
		return nil, err
	}
	allEdges, err := store.ListEdges(ctx, finding.ProjectID)
	if err != nil {
		return nil, err
	}
	factMap := map[string]pentest.Fact{}
	queue := []string{}
	for _, fact := range linked {
		factMap[fact.FactKey] = fact
		queue = append(queue, fact.FactKey)
	}
	if finding.SourceFactKey != "" {
		queue = append(queue, finding.SourceFactKey)
	}
	visited := map[string]bool{}
	missing := []string{}
	// Walk recorded upstream relations only; unrelated session activity is not evidence.
	for len(queue) > 0 && len(visited) < 64 {
		key := queue[0]
		queue = queue[1:]
		if visited[key] {
			continue
		}
		visited[key] = true
		if _, ok := factMap[key]; !ok {
			fact, e := store.GetFact(ctx, finding.ProjectID, key)
			if errors.Is(e, sql.ErrNoRows) {
				missing = append(missing, key)
				continue
			}
			if e != nil {
				return nil, e
			}
			factMap[key] = fact
		}
		for _, edge := range allEdges {
			if edge.TargetFactKey == key && !visited[edge.SourceFactKey] {
				queue = append(queue, edge.SourceFactKey)
			}
		}
	}
	facts := []pentest.Fact{}
	for _, fact := range factMap {
		facts = append(facts, fact)
	}
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].CreatedAt.Equal(facts[j].CreatedAt) {
			return facts[i].FactKey < facts[j].FactKey
		}
		return facts[i].CreatedAt.Before(facts[j].CreatedAt)
	})
	refs := map[string]bool{}
	addRefs := func(text string) {
		for _, m := range findingExecutionRefPattern.FindAllStringSubmatch(text, -1) {
			refs[m[1]] = true
		}
	}
	addRefs(finding.Evidence)
	addRefs(finding.ReproductionSteps)
	factViews := []gin.H{}
	for _, fact := range facts {
		factViews = append(factViews, pentestFactView(fact))
		addRefs(fact.Body)
		if strings.HasPrefix(fact.FactKey, "auto.clue.") || strings.HasPrefix(fact.FactKey, "auto.tool.") {
			if fact.SourceMessageID != "" {
				refs[fact.SourceMessageID] = true
			}
		}
	}
	edgeViews := []gin.H{}
	for _, edge := range pentest.FilterEdgesForFacts(allEdges, facts) {
		edgeViews = append(edgeViews, pentestFactEdgeView(edge))
	}
	executions := []pentest.ToolExecution{}
	for ref := range refs {
		execution, e := store.GetToolExecution(ctx, ref)
		if e != nil {
			if !errors.Is(e, sql.ErrNoRows) {
				return nil, e
			}
			missing = append(missing, ref)
			continue
		}
		source, e := store.GetConversation(ctx, execution.ConversationID)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return nil, e
		}
		if e != nil || source.ProjectID != finding.ProjectID {
			missing = append(missing, ref)
			continue
		}
		executions = append(executions, execution)
	}
	sort.Slice(executions, func(i, j int) bool { return executions[i].StartTime.Before(executions[j].StartTime) })
	executionViews := []gin.H{}
	for _, execution := range executions {
		executionViews = append(executionViews, gin.H{"id": execution.ID, "conversationId": execution.ConversationID, "toolName": execution.ToolName, "status": execution.Status, "startedAt": execution.StartTime, "completedAt": execution.EndTime})
	}
	dispositions, err := store.ListFindingDispositions(ctx, id)
	if err != nil {
		return nil, err
	}
	retests, err := store.ListFindingRetests(ctx, id)
	if err != nil {
		return nil, err
	}
	sort.Strings(missing)
	return gin.H{"dispositions": dispositions, "retests": retests, "retestTarget": retestTarget, "view": view, "project": projectViewBase(project), "conversation": conversation, "facts": factViews, "edges": edgeViews, "executionRefs": executionViews, "missingEvidenceRefs": missing, "chainTruncated": len(queue) > 0}, nil
}
