package mcp

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/apidoctest"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	mcpWorkbenchAPIAnalyzeMaxInputBytes = 500000
	mcpWorkbenchAPIAnalyzeDefaultLimit  = 80
	mcpWorkbenchAPIAnalyzeMaxLimit      = 300
)

var (
	mcpWorkbenchAPIAnalyzeFetchPattern      = regexp.MustCompile(`(?is)\bfetch\s*\(\s*["']([^"']+)["']`)
	mcpWorkbenchAPIAnalyzeAxiosPattern      = regexp.MustCompile(`(?is)\baxios\.(get|post|put|patch|delete|head|options)\s*\(\s*["']([^"']+)["']`)
	mcpWorkbenchAPIAnalyzeMethodPathPattern = regexp.MustCompile(`(?is)\b(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+["']?([^"'\s<>{}\\]+)`)
	mcpWorkbenchAPIAnalyzeQuotedURLPattern  = regexp.MustCompile(`(?is)["']((?:https?://|//|/|api/)[^"'\s<>{}\\]+)["']`)
	mcpWorkbenchAPIAnalyzeVariablePattern   = regexp.MustCompile(`(?i)(?:^|[/?&._-])([a-z][a-z0-9_]*(?:id|token|key|sid|uid|uuid|session)[a-z0-9_]*)`)
)

type mcpWorkbenchAPISchemaAnalyzeArgs struct {
	TaskID       string `json:"taskId"`
	StepID       string `json:"stepId"`
	Input        string `json:"input"`
	BaseURL      string `json:"baseUrl"`
	SourceType   string `json:"sourceType"`
	SourceID     string `json:"sourceId"`
	MaxEndpoints int    `json:"maxEndpoints"`
}

type mcpWorkbenchAPIDocTestArgs struct {
	TaskID         string            `json:"taskId"`
	StepID         string            `json:"stepId"`
	SourceURL      string            `json:"sourceUrl"`
	Input          string            `json:"input"`
	BaseURL        string            `json:"baseUrl"`
	DocumentType   string            `json:"documentType"`
	Headers        map[string]string `json:"headers"`
	MaxEndpoints   int               `json:"maxEndpoints"`
	GenerateCases  *bool             `json:"generateCases"`
	RunSafeChecks  bool              `json:"runSafeChecks"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
}

type mcpWorkbenchAPIAnalyzeCandidate struct {
	Method    string   `json:"method"`
	URL       string   `json:"url"`
	Source    string   `json:"source"`
	Variables []string `json:"variables"`
}

type mcpWorkbenchAPIAnalyzeSkipped struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

func mcpCallWorkbenchAPISchemaAnalyze(args mcpWorkbenchAPISchemaAnalyzeArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	input := strings.TrimSpace(args.Input)
	if input == "" {
		return mcpToolCallResult{}, fmt.Errorf("input 不能为空")
	}
	if len(input) > mcpWorkbenchAPIAnalyzeMaxInputBytes {
		return mcpToolCallResult{}, fmt.Errorf("input 超过最大长度 %d bytes", mcpWorkbenchAPIAnalyzeMaxInputBytes)
	}
	limit := args.MaxEndpoints
	if limit <= 0 {
		limit = mcpWorkbenchAPIAnalyzeDefaultLimit
	}
	if limit > mcpWorkbenchAPIAnalyzeMaxLimit {
		limit = mcpWorkbenchAPIAnalyzeMaxLimit
	}
	baseURL := defaultString(strings.TrimSpace(args.BaseURL), task.Target)
	executionID := database.GenerateNanoid()
	toolCallID := database.GenerateNanoid()
	inputSummary := fmt.Sprintf("分析 %s 输入，长度 %d bytes", defaultString(strings.TrimSpace(args.SourceType), "text"), len(input))
	toolCall := database.AIWorkbenchToolCall{
		ID:          toolCallID,
		TaskID:      task.ID,
		StepID:      strings.TrimSpace(args.StepID),
		ExecutionID: executionID,
		ToolName:    "api_schema_analyze",
		InputJSON: marshalMCPWorkbenchJSON(map[string]any{
			"baseUrl":      baseURL,
			"sourceType":   args.SourceType,
			"sourceId":     args.SourceID,
			"maxEndpoints": limit,
			"inputPreview": truncateMCPWorkbenchText(input, 1000),
		}),
		Status: "running",
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&toolCall).Error; err != nil {
			return err
		}
		return createMCPWorkbenchToolExecution(tx, database.AIWorkbenchToolExecution{
			ID:           executionID,
			TaskID:       task.ID,
			StepID:       toolCall.StepID,
			ToolCallID:   toolCall.ID,
			ToolName:     "api.schema.analyze",
			Runtime:      aipkg.WorkbenchToolRuntimeLocalMCP,
			InputSummary: inputSummary,
			Arguments: map[string]any{
				"baseUrl":      baseURL,
				"sourceType":   args.SourceType,
				"sourceId":     args.SourceID,
				"maxEndpoints": limit,
				"inputPreview": truncateMCPWorkbenchText(input, 1000),
			},
			Status: aipkg.WorkbenchToolExecutionStatusRunning,
			Metadata: map[string]any{
				"toolCallId": toolCall.ID,
			},
		})
	}); err != nil {
		return mcpToolCallResult{}, err
	}

	candidates := extractMCPWorkbenchAPICandidates(input, baseURL, limit)
	endpoints := make([]database.AIWorkbenchAPIEndpoint, 0, len(candidates))
	skipped := make([]mcpWorkbenchAPIAnalyzeSkipped, 0)
	for _, candidate := range candidates {
		inScope, reason := isMCPWorkbenchTargetInScope(candidate.URL, append([]string{task.Target}, task.Scope...))
		if !inScope {
			skipped = append(skipped, mcpWorkbenchAPIAnalyzeSkipped{Method: candidate.Method, URL: candidate.URL, Reason: reason})
			continue
		}
		endpoint, err := upsertMCPWorkbenchAPIEndpoint(db, task, mcpWorkbenchAPIEndpointRecordArgs{
			TaskID: task.ID,
			StepID: args.StepID,
			Method: candidate.Method,
			URL:    candidate.URL,
			Source: "api_schema_analyze",
			Tags:   []string{"api_candidate", candidate.Source},
			Metadata: map[string]any{
				"sourceType": args.SourceType,
				"sourceId":   args.SourceID,
				"variables":  candidate.Variables,
			},
		})
		if err != nil {
			skipped = append(skipped, mcpWorkbenchAPIAnalyzeSkipped{Method: candidate.Method, URL: candidate.URL, Reason: err.Error()})
			continue
		}
		endpoints = append(endpoints, endpoint)
	}
	relations := recordMCPWorkbenchAPICandidateRelations(db, task.ID, endpoints, args.SourceID)
	summary := fmt.Sprintf("API schema 分析完成：候选 %d 个，落库 endpoint %d 个，候选关系 %d 条，跳过 %d 个。", len(candidates), len(endpoints), len(relations), len(skipped))
	if err := db.Transaction(func(tx *gorm.DB) error {
		toolCall.Status = "completed"
		toolCall.OutputSummary = summary
		toolCall.RawOutputRef = "api_schema_analyze:" + executionID
		if err := tx.Save(&toolCall).Error; err != nil {
			return err
		}
		if err := finishMCPWorkbenchToolExecution(tx, executionID, aipkg.WorkbenchToolExecutionStatusCompleted, summary, toolCall.RawOutputRef, "", map[string]any{
			"toolCallId":    toolCall.ID,
			"endpointCount": len(endpoints),
			"relationCount": len(relations),
			"skippedCount":  len(skipped),
		}); err != nil {
			return err
		}
		return createMCPWorkbenchEvent(tx, task.ID, strings.TrimSpace(args.StepID), "api.schema.analyzed", "info", "API Schema 已分析", summary, gin.H{
			"toolCallId":    toolCall.ID,
			"executionId":   executionID,
			"endpointCount": len(endpoints),
			"relationCount": len(relations),
			"skippedCount":  len(skipped),
		})
	}); err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(gin.H{
		"taskId":      task.ID,
		"executionId": executionID,
		"toolCallId":  toolCall.ID,
		"summary":     summary,
		"endpoints":   endpoints,
		"relations":   relations,
		"skipped":     skipped,
	}), nil
}

func mcpCallWorkbenchAPIDocTest(args mcpWorkbenchAPIDocTestArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	if strings.TrimSpace(args.Input) == "" && strings.TrimSpace(args.SourceURL) == "" {
		return mcpToolCallResult{}, fmt.Errorf("input 或 sourceUrl 不能为空")
	}
	generateCases := true
	if args.GenerateCases != nil {
		generateCases = *args.GenerateCases
	}
	baseURL := defaultString(strings.TrimSpace(args.BaseURL), task.Target)
	executionID := database.GenerateNanoid()
	toolCallID := database.GenerateNanoid()
	inputSummary := fmt.Sprintf("测试接口文档 %s，类型 %s", defaultString(strings.TrimSpace(args.SourceURL), "inline input"), defaultString(strings.TrimSpace(args.DocumentType), "auto"))
	toolCall := database.AIWorkbenchToolCall{
		ID:          toolCallID,
		TaskID:      task.ID,
		StepID:      strings.TrimSpace(args.StepID),
		ExecutionID: executionID,
		ToolName:    "api_doc_test",
		InputJSON: marshalMCPWorkbenchJSON(map[string]any{
			"sourceUrl":      args.SourceURL,
			"baseUrl":        baseURL,
			"documentType":   args.DocumentType,
			"maxEndpoints":   args.MaxEndpoints,
			"generateCases":  generateCases,
			"runSafeChecks":  args.RunSafeChecks,
			"timeoutSeconds": args.TimeoutSeconds,
			"inputPreview":   truncateMCPWorkbenchText(args.Input, 1000),
		}),
		Status: "running",
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&toolCall).Error; err != nil {
			return err
		}
		return createMCPWorkbenchToolExecution(tx, database.AIWorkbenchToolExecution{
			ID:           executionID,
			TaskID:       task.ID,
			StepID:       toolCall.StepID,
			ToolCallID:   toolCall.ID,
			ToolName:     "api.doc.test",
			Runtime:      aipkg.WorkbenchToolRuntimeLocalMCP,
			InputSummary: inputSummary,
			Arguments: map[string]any{
				"sourceUrl":      args.SourceURL,
				"baseUrl":        baseURL,
				"documentType":   args.DocumentType,
				"maxEndpoints":   args.MaxEndpoints,
				"generateCases":  generateCases,
				"runSafeChecks":  args.RunSafeChecks,
				"timeoutSeconds": args.TimeoutSeconds,
				"inputPreview":   truncateMCPWorkbenchText(args.Input, 1000),
			},
			Status: aipkg.WorkbenchToolExecutionStatusRunning,
			Metadata: map[string]any{
				"toolCallId": toolCall.ID,
			},
		})
	}); err != nil {
		return mcpToolCallResult{}, err
	}

	result, runErr := apidoctest.AnalyzeAndTest(context.Background(), apidoctest.Request{
		SourceURL:      args.SourceURL,
		Input:          args.Input,
		BaseURL:        baseURL,
		DocumentType:   args.DocumentType,
		Headers:        args.Headers,
		MaxEndpoints:   args.MaxEndpoints,
		GenerateCases:  generateCases,
		RunSafeChecks:  args.RunSafeChecks,
		TimeoutSeconds: args.TimeoutSeconds,
	})
	if runErr != nil {
		toolCall.Status = "failed"
		toolCall.ErrorMessage = runErr.Error()
		toolCall.OutputSummary = "接口文档测试失败。"
		_ = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(&toolCall).Error; err != nil {
				return err
			}
			if err := finishMCPWorkbenchToolExecution(tx, executionID, aipkg.WorkbenchToolExecutionStatusFailed, toolCall.OutputSummary, "", runErr.Error(), map[string]any{
				"toolCallId": toolCall.ID,
			}); err != nil {
				return err
			}
			return createMCPWorkbenchEvent(tx, task.ID, toolCall.StepID, "api.doc.test.failed", "error", "接口文档测试失败", runErr.Error(), gin.H{"toolCallId": toolCall.ID})
		})
		return mcpToolCallResult{}, runErr
	}

	endpoints := make([]database.AIWorkbenchAPIEndpoint, 0, len(result.Endpoints))
	skipped := make([]mcpWorkbenchAPIAnalyzeSkipped, 0)
	for _, endpoint := range result.Endpoints {
		record, err := upsertMCPWorkbenchAPIEndpoint(db, task, mcpWorkbenchAPIEndpointRecordArgs{
			TaskID:        task.ID,
			StepID:        args.StepID,
			Method:        endpoint.Method,
			URL:           endpoint.URL,
			OperationName: endpoint.OperationID,
			AuthRequired:  endpoint.AuthRequired,
			Source:        "api_doc_test",
			Tags:          append([]string{"api_doc", result.DocumentType}, endpoint.RiskTags...),
			Metadata: map[string]any{
				"path":       endpoint.Path,
				"summary":    endpoint.Summary,
				"parameters": endpoint.Parameters,
				"riskTags":   endpoint.RiskTags,
				"sourceUrl":  args.SourceURL,
			},
		})
		if err != nil {
			skipped = append(skipped, mcpWorkbenchAPIAnalyzeSkipped{Method: endpoint.Method, URL: endpoint.URL, Reason: err.Error()})
			continue
		}
		endpoints = append(endpoints, record)
	}
	for _, finding := range result.Findings {
		_ = mcpCallWorkbenchEvidenceAddNoResult(mcpWorkbenchEvidenceAddArgs{
			TaskID:     task.ID,
			StepID:     args.StepID,
			SourceType: "api_doc_test",
			SourceID:   executionID,
			TargetRef:  finding.Endpoint,
			Title:      finding.Title,
			Summary:    finding.Summary,
			RiskLevel:  finding.RiskLevel,
			Confidence: finding.Confidence,
			Status:     "candidate",
			Reference:  "api_doc_test:" + executionID,
			Metadata: map[string]interface{}{
				"evidence": finding.Evidence,
			},
		})
	}
	summary := fmt.Sprintf("接口文档测试完成：解析 %d 个 endpoint，生成 %d 个用例，执行 %d 个安全用例，候选风险 %d 个，落库 endpoint %d 个，跳过 %d 个。",
		result.Summary.EndpointCount,
		result.Summary.GeneratedCaseCount,
		result.Summary.ExecutedCaseCount,
		result.Summary.FindingCount,
		len(endpoints),
		len(skipped),
	)
	if err := db.Transaction(func(tx *gorm.DB) error {
		toolCall.Status = "completed"
		toolCall.OutputSummary = summary
		toolCall.RawOutputRef = "api_doc_test:" + executionID
		if err := tx.Save(&toolCall).Error; err != nil {
			return err
		}
		if err := finishMCPWorkbenchToolExecution(tx, executionID, aipkg.WorkbenchToolExecutionStatusCompleted, summary, toolCall.RawOutputRef, "", map[string]any{
			"toolCallId":    toolCall.ID,
			"endpointCount": len(endpoints),
			"caseCount":     len(result.Cases),
			"findingCount":  len(result.Findings),
			"skippedCount":  len(skipped),
		}); err != nil {
			return err
		}
		return createMCPWorkbenchEvent(tx, task.ID, strings.TrimSpace(args.StepID), "api.doc.test.completed", "info", "接口文档测试完成", summary, gin.H{
			"toolCallId":    toolCall.ID,
			"executionId":   executionID,
			"endpointCount": len(endpoints),
			"caseCount":     len(result.Cases),
			"findingCount":  len(result.Findings),
			"skippedCount":  len(skipped),
		})
	}); err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(gin.H{
		"taskId":      task.ID,
		"executionId": executionID,
		"toolCallId":  toolCall.ID,
		"summary":     summary,
		"result":      result,
		"endpoints":   endpoints,
		"skipped":     skipped,
	}), nil
}

func extractMCPWorkbenchAPICandidates(input string, baseURL string, limit int) []mcpWorkbenchAPIAnalyzeCandidate {
	seen := map[string]bool{}
	seenURL := map[string]bool{}
	candidates := make([]mcpWorkbenchAPIAnalyzeCandidate, 0)
	add := func(method string, raw string, source string) {
		if len(candidates) >= limit {
			return
		}
		resolved, ok := resolveMCPWorkbenchAPIAnalyzeURL(raw, baseURL)
		if !ok || !looksLikeMCPWorkbenchAPIEndpoint(resolved) {
			return
		}
		if source == "quoted_url" && seenURL[resolved] {
			return
		}
		method = strings.ToUpper(defaultString(strings.TrimSpace(method), "GET"))
		key := method + " " + resolved
		if seen[key] {
			return
		}
		seen[key] = true
		seenURL[resolved] = true
		candidates = append(candidates, mcpWorkbenchAPIAnalyzeCandidate{
			Method:    method,
			URL:       resolved,
			Source:    source,
			Variables: extractMCPWorkbenchAPIAnalyzeVariables(resolved),
		})
	}
	for _, match := range mcpWorkbenchAPIAnalyzeAxiosPattern.FindAllStringSubmatch(input, -1) {
		add(match[1], match[2], "axios")
	}
	for _, match := range mcpWorkbenchAPIAnalyzeFetchPattern.FindAllStringSubmatch(input, -1) {
		method := inferMCPWorkbenchFetchMethod(input, match[0])
		add(method, match[1], "fetch")
	}
	for _, match := range mcpWorkbenchAPIAnalyzeMethodPathPattern.FindAllStringSubmatch(input, -1) {
		add(match[1], match[2], "method_path")
	}
	for _, match := range mcpWorkbenchAPIAnalyzeQuotedURLPattern.FindAllStringSubmatch(input, -1) {
		add("GET", match[1], "quoted_url")
	}
	return candidates
}

func inferMCPWorkbenchFetchMethod(input string, matchText string) string {
	index := strings.Index(input, matchText)
	if index < 0 {
		return "GET"
	}
	windowEnd := index + len(matchText) + 200
	if windowEnd > len(input) {
		windowEnd = len(input)
	}
	window := input[index:windowEnd]
	methodPattern := regexp.MustCompile(`(?is)\bmethod\s*:\s*["'](GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)["']`)
	if match := methodPattern.FindStringSubmatch(window); len(match) == 2 {
		return strings.ToUpper(match[1])
	}
	return "GET"
}

func resolveMCPWorkbenchAPIAnalyzeURL(raw string, baseURL string) (string, bool) {
	raw = strings.TrimSpace(strings.Trim(raw, "`"))
	if raw == "" {
		return "", false
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", false
	}
	if strings.HasPrefix(raw, "//") {
		raw = base.Scheme + ":" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		return parsed.String(), true
	}
	if strings.HasPrefix(raw, "api/") {
		raw = "/" + raw
	}
	if !strings.HasPrefix(raw, "/") {
		return "", false
	}
	relative, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	return base.ResolveReference(relative).String(), true
}

func looksLikeMCPWorkbenchAPIEndpoint(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	path := strings.ToLower(parsed.EscapedPath())
	if path == "" || path == "/" {
		return false
	}
	if strings.Contains(path, "/api/") || strings.HasPrefix(path, "/api") || strings.Contains(path, "/v1/") || strings.Contains(path, "/v2/") {
		return true
	}
	if strings.HasSuffix(path, ".do") || strings.HasSuffix(path, ".action") || strings.HasSuffix(path, ".json") {
		return true
	}
	for _, marker := range []string{"login", "logout", "token", "auth", "user", "admin", "profile", "graphql"} {
		if strings.Contains(path, marker) {
			return true
		}
	}
	return parsed.RawQuery != ""
}

func extractMCPWorkbenchAPIAnalyzeVariables(rawURL string) []string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	variables := make([]string, 0)
	add := func(value string) {
		value = strings.Trim(strings.ToLower(strings.TrimSpace(value)), "{}:$")
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		variables = append(variables, value)
	}
	for _, match := range mcpWorkbenchAPIAnalyzeVariablePattern.FindAllStringSubmatch(parsed.Path+"?"+parsed.RawQuery, -1) {
		add(match[1])
	}
	for key := range parsed.Query() {
		add(key)
	}
	sort.Strings(variables)
	return variables
}

func recordMCPWorkbenchAPICandidateRelations(db *gorm.DB, taskID string, endpoints []database.AIWorkbenchAPIEndpoint, sourceID string) []database.AIWorkbenchAPIRelation {
	relations := make([]database.AIWorkbenchAPIRelation, 0)
	for index := 1; index < len(endpoints); index++ {
		relation, err := upsertMCPWorkbenchAPIRelation(db, mcpWorkbenchAPIRelationRecordArgs{
			TaskID:           taskID,
			SourceEndpointID: endpoints[index-1].ID,
			TargetEndpointID: endpoints[index].ID,
			RelationType:     "candidate_sequence",
			EvidenceRef:      strings.TrimSpace(sourceID),
			Confidence:       "low",
			Metadata: map[string]any{
				"source":        "api_schema_analyze",
				"sequenceIndex": index,
			},
		})
		if err == nil {
			relations = append(relations, relation)
		}
		for _, variable := range mcpWorkbenchAPIEndpointVariables(endpoints[index]) {
			relationType := "candidate_variable_flow"
			objectType := inferMCPWorkbenchRelationObjectType(variable)
			if objectType == "token" {
				relationType = "candidate_auth_flow"
			}
			relation, err := upsertMCPWorkbenchAPIRelation(db, mcpWorkbenchAPIRelationRecordArgs{
				TaskID:           taskID,
				SourceEndpointID: endpoints[index-1].ID,
				TargetEndpointID: endpoints[index].ID,
				RelationType:     relationType,
				VariableName:     variable,
				ObjectType:       objectType,
				EvidenceRef:      strings.TrimSpace(sourceID),
				Confidence:       "low",
				Metadata: map[string]any{
					"source":        "api_schema_analyze",
					"sequenceIndex": index,
					"basis":         "static_candidate_variable",
				},
			})
			if err == nil {
				relations = append(relations, relation)
			}
		}
	}
	return relations
}

func mcpWorkbenchAPIEndpointVariables(endpoint database.AIWorkbenchAPIEndpoint) []string {
	if endpoint.Metadata == nil {
		return nil
	}
	raw, ok := endpoint.Metadata["variables"]
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	variables := make([]string, 0)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		variables = append(variables, value)
	}
	switch typed := raw.(type) {
	case []string:
		for _, item := range typed {
			add(item)
		}
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				add(text)
			}
		}
	}
	sort.Strings(variables)
	return variables
}
