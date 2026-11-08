package mcp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	mcpWorkbenchHTTPMaxBodyBytes     = 512 * 1024
	mcpWorkbenchHTTPPreviewLimit     = 12000
	mcpWorkbenchHTTPStoredBodyLimit  = 60000
	mcpWorkbenchHTTPDefaultTimeout   = 10
	mcpWorkbenchHTTPMaxTimeout       = 30
	mcpWorkbenchHTTPSequenceMaxSteps = 8
)

var mcpWorkbenchVariableRefPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

type mcpWorkbenchHTTPRequestArgs struct {
	TaskID          string                         `json:"taskId"`
	StepID          string                         `json:"stepId"`
	Method          string                         `json:"method"`
	URL             string                         `json:"url"`
	Headers         map[string]string              `json:"headers"`
	Body            string                         `json:"body"`
	TimeoutSeconds  int                            `json:"timeoutSeconds"`
	FollowRedirects *bool                          `json:"followRedirects"`
	Extract         []mcpWorkbenchHTTPExtractorArg `json:"extract"`
}

type mcpWorkbenchHTTPSequenceRunArgs struct {
	TaskID          string                         `json:"taskId"`
	StepID          string                         `json:"stepId"`
	Variables       map[string]string              `json:"variables"`
	Requests        []mcpWorkbenchHTTPRequestArgs  `json:"requests"`
	StopOnError     *bool                          `json:"stopOnError"`
	Extract         []mcpWorkbenchHTTPExtractorArg `json:"extract"`
	RecordRelations bool                           `json:"recordRelations"`
}

type mcpWorkbenchHTTPExtractorArg struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	JSONPath   string `json:"jsonPath"`
	Regex      string `json:"regex"`
	HeaderName string `json:"headerName"`
	ValueType  string `json:"valueType"`
}

type mcpWorkbenchHTTPExtractVariableArgs struct {
	TaskID     string `json:"taskId"`
	StepID     string `json:"stepId"`
	Name       string `json:"name"`
	ValueType  string `json:"valueType"`
	SourceType string `json:"sourceType"`
	SourceID   string `json:"sourceId"`
	Input      string `json:"input"`
	JSONPath   string `json:"jsonPath"`
	Regex      string `json:"regex"`
	HeaderName string `json:"headerName"`
	EndpointID string `json:"endpointId"`
	Confidence string `json:"confidence"`
}

type mcpWorkbenchHTTPCompareResponseArgs struct {
	TaskID              string   `json:"taskId"`
	BaselineEvidenceID  string   `json:"baselineEvidenceId"`
	CandidateEvidenceID string   `json:"candidateEvidenceId"`
	Checks              []string `json:"checks"`
}

type mcpWorkbenchAPIEndpointRecordArgs struct {
	TaskID        string                 `json:"taskId"`
	StepID        string                 `json:"stepId"`
	Method        string                 `json:"method"`
	URL           string                 `json:"url"`
	OperationName string                 `json:"operationName"`
	AuthRequired  bool                   `json:"authRequired"`
	AuthProfileID string                 `json:"authProfileId"`
	Source        string                 `json:"source"`
	Tags          []string               `json:"tags"`
	Metadata      map[string]interface{} `json:"metadata"`
}

type mcpWorkbenchAPIRelationRecordArgs struct {
	TaskID           string                 `json:"taskId"`
	SourceEndpointID string                 `json:"sourceEndpointId"`
	TargetEndpointID string                 `json:"targetEndpointId"`
	RelationType     string                 `json:"relationType"`
	VariableName     string                 `json:"variableName"`
	ObjectType       string                 `json:"objectType"`
	EvidenceRef      string                 `json:"evidenceRef"`
	Confidence       string                 `json:"confidence"`
	Metadata         map[string]interface{} `json:"metadata"`
}

type mcpWorkbenchHTTPRequestResult struct {
	TaskID         string            `json:"taskId"`
	ExecutionID    string            `json:"executionId"`
	ToolCallID     string            `json:"toolCallId"`
	EndpointID     string            `json:"endpointId"`
	HTTPEvidenceID string            `json:"httpEvidenceId"`
	Method         string            `json:"method"`
	URL            string            `json:"url"`
	StatusCode     int               `json:"statusCode"`
	ResponseTime   int64             `json:"responseTimeMs"`
	Headers        map[string]string `json:"headers"`
	BodyPreview    string            `json:"bodyPreview"`
	BodyLength     int               `json:"bodyLength"`
	Variables      map[string]string `json:"variables"`
}

type mcpWorkbenchRequestVariableUse struct {
	Name       string
	Location   string
	HeaderName string
}

func mcpCallWorkbenchHTTPRequest(args mcpWorkbenchHTTPRequestArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	result, err := executeMCPWorkbenchHTTPRequest(db, task, args, nil)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(result), nil
}

func mcpCallWorkbenchHTTPSequenceRun(args mcpWorkbenchHTTPSequenceRunArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	if len(args.Requests) == 0 {
		return mcpToolCallResult{}, fmt.Errorf("requests 不能为空")
	}
	if len(args.Requests) > mcpWorkbenchHTTPSequenceMaxSteps {
		return mcpToolCallResult{}, fmt.Errorf("requests 最多支持 %d 步", mcpWorkbenchHTTPSequenceMaxSteps)
	}
	stopOnError := true
	if args.StopOnError != nil {
		stopOnError = *args.StopOnError
	}
	variables := cloneStringMap(args.Variables)
	variableSources := map[string]string{}
	results := make([]mcpWorkbenchHTTPRequestResult, 0, len(args.Requests))
	var previousEndpointID string
	for index, requestArgs := range args.Requests {
		requestArgs.TaskID = task.ID
		if requestArgs.StepID == "" {
			requestArgs.StepID = args.StepID
		}
		if len(requestArgs.Extract) == 0 && len(args.Extract) > 0 {
			requestArgs.Extract = args.Extract
		}
		usedVariables := findMCPWorkbenchRequestVariableUses(requestArgs)
		authVariables := mcpWorkbenchAuthVariableUseSet(usedVariables)
		requestArgs.URL = substituteMCPWorkbenchVariables(requestArgs.URL, variables)
		requestArgs.Body = substituteMCPWorkbenchVariables(requestArgs.Body, variables)
		requestArgs.Headers = substituteMCPWorkbenchHeaderVariables(requestArgs.Headers, variables)

		result, requestErr := executeMCPWorkbenchHTTPRequest(db, task, requestArgs, variables)
		if requestErr != nil {
			if stopOnError {
				return mcpToolCallResult{}, fmt.Errorf("第 %d 个请求失败: %w", index+1, requestErr)
			}
			continue
		}
		if args.RecordRelations && result.EndpointID != "" {
			recordMCPWorkbenchHTTPSequenceVariableRelations(db, task.ID, result, usedVariables, authVariables, variableSources, index)
		}
		for key, value := range result.Variables {
			previousValue, existed := variables[key]
			variables[key] = value
			if !existed || previousValue != value {
				variableSources[key] = result.EndpointID
			}
		}
		if args.RecordRelations && previousEndpointID != "" && result.EndpointID != "" {
			_, _ = upsertMCPWorkbenchAPIRelation(db, mcpWorkbenchAPIRelationRecordArgs{
				TaskID:           task.ID,
				SourceEndpointID: previousEndpointID,
				TargetEndpointID: result.EndpointID,
				RelationType:     "sequence_next",
				EvidenceRef:      "ai_http_request:" + result.HTTPEvidenceID,
				Confidence:       "medium",
				Metadata: map[string]interface{}{
					"sequenceIndex": index,
				},
			})
		}
		previousEndpointID = result.EndpointID
		results = append(results, result)
	}
	if err := createMCPWorkbenchEvent(db, task.ID, args.StepID, "http.sequence.completed", "info", "接口序列执行完成", fmt.Sprintf("已执行 %d/%d 个接口请求，提取变量 %d 个。", len(results), len(args.Requests), len(variables)), gin.H{
		"results":   results,
		"variables": variables,
	}); err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(gin.H{
		"taskId":    task.ID,
		"completed": len(results),
		"total":     len(args.Requests),
		"variables": variables,
		"results":   results,
	}), nil
}

func mcpCallWorkbenchHTTPExtractVariable(args mcpWorkbenchHTTPExtractVariableArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	name := strings.TrimSpace(args.Name)
	if name == "" {
		return mcpToolCallResult{}, fmt.Errorf("name 不能为空")
	}
	input, endpointID, err := resolveMCPWorkbenchExtractionInput(db, task.ID, args)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	value, err := extractMCPWorkbenchValue(input, nil, mcpWorkbenchHTTPExtractorArg{
		Name:       name,
		Source:     args.SourceType,
		JSONPath:   args.JSONPath,
		Regex:      args.Regex,
		HeaderName: args.HeaderName,
		ValueType:  args.ValueType,
	})
	if err != nil {
		return mcpToolCallResult{}, err
	}
	variable, err := recordMCPWorkbenchExtractedVariable(db, database.AIWorkbenchExtractedVariable{
		TaskID:     task.ID,
		StepID:     strings.TrimSpace(args.StepID),
		Name:       name,
		Value:      truncateMCPWorkbenchText(value, 4096),
		ValueType:  normalizeMCPWorkbenchVariableType(args.ValueType, value),
		SourceType: defaultString(strings.TrimSpace(args.SourceType), "manual_input"),
		SourceID:   strings.TrimSpace(args.SourceID),
		JSONPath:   strings.TrimSpace(args.JSONPath),
		Regex:      strings.TrimSpace(args.Regex),
		HeaderName: strings.TrimSpace(args.HeaderName),
		EndpointID: defaultString(strings.TrimSpace(args.EndpointID), endpointID),
		Confidence: normalizeMCPWorkbenchConfidence(args.Confidence),
	})
	if err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(gin.H{
		"taskId":   task.ID,
		"variable": variable,
	}), nil
}

func mcpCallWorkbenchHTTPCompareResponse(args mcpWorkbenchHTTPCompareResponseArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	var baseline database.AIWorkbenchHTTPRequestEvidence
	if err := db.Where("id = ? AND task_id = ?", strings.TrimSpace(args.BaselineEvidenceID), task.ID).First(&baseline).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("baselineEvidenceId 不存在: %w", err)
	}
	var candidate database.AIWorkbenchHTTPRequestEvidence
	if err := db.Where("id = ? AND task_id = ?", strings.TrimSpace(args.CandidateEvidenceID), task.ID).First(&candidate).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("candidateEvidenceId 不存在: %w", err)
	}
	checks := normalizeNonEmptyStrings(args.Checks)
	if len(checks) == 0 {
		checks = []string{"status_code", "body_length", "content_type"}
	}
	diff := compareMCPWorkbenchHTTPResponses(baseline, candidate, checks)
	summary := fmt.Sprintf("响应对比完成：状态码 %d -> %d，长度 %d -> %d，差异项 %d 个。", baseline.StatusCode, candidate.StatusCode, baseline.ResponseLength, candidate.ResponseLength, len(diff))
	if err := createMCPWorkbenchEvent(db, task.ID, "", "http.response.compared", "info", "接口响应对比完成", summary, gin.H{"diff": diff}); err != nil {
		return mcpToolCallResult{}, err
	}
	if err := mcpCallWorkbenchFactUpsertNoResult(mcpWorkbenchFactUpsertArgs{
		TaskID:     task.ID,
		FactKey:    "http_compare/" + baseline.ID + "/" + candidate.ID,
		Source:     "http_compare",
		TargetRef:  candidate.URL,
		Summary:    summary,
		Confidence: "medium",
		Metadata:   map[string]interface{}{"diff": diff},
	}); err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(gin.H{
		"taskId":  task.ID,
		"summary": summary,
		"diff":    diff,
	}), nil
}

func mcpCallWorkbenchAPIEndpointRecord(args mcpWorkbenchAPIEndpointRecordArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	endpoint, err := upsertMCPWorkbenchAPIEndpoint(db, task, args)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(gin.H{
		"taskId":   task.ID,
		"endpoint": endpoint,
	}), nil
}

func mcpCallWorkbenchAPIRelationRecord(args mcpWorkbenchAPIRelationRecordArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	relation, err := upsertMCPWorkbenchAPIRelation(db, args)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(gin.H{
		"taskId":   task.ID,
		"relation": relation,
	}), nil
}

func executeMCPWorkbenchHTTPRequest(db *gorm.DB, task database.AIWorkbenchTask, args mcpWorkbenchHTTPRequestArgs, inheritedVariables map[string]string) (mcpWorkbenchHTTPRequestResult, error) {
	requestURL := strings.TrimSpace(args.URL)
	if requestURL == "" {
		return mcpWorkbenchHTTPRequestResult{}, fmt.Errorf("url 不能为空")
	}
	inScope, reason := isMCPWorkbenchTargetInScope(requestURL, append([]string{task.Target}, task.Scope...))
	if !inScope {
		return mcpWorkbenchHTTPRequestResult{}, fmt.Errorf("目标不在授权范围内: %s", reason)
	}
	method, err := normalizeMCPWorkbenchHTTPMethod(args.Method)
	if err != nil {
		return mcpWorkbenchHTTPRequestResult{}, err
	}
	timeout := args.TimeoutSeconds
	if timeout <= 0 {
		timeout = mcpWorkbenchHTTPDefaultTimeout
	}
	if timeout > mcpWorkbenchHTTPMaxTimeout {
		timeout = mcpWorkbenchHTTPMaxTimeout
	}
	followRedirects := true
	if args.FollowRedirects != nil {
		followRedirects = *args.FollowRedirects
	}
	requestHeaders := sanitizeMCPWorkbenchRequestHeaders(args.Headers)
	inputJSON := marshalMCPWorkbenchJSON(map[string]interface{}{
		"method":          method,
		"url":             requestURL,
		"headers":         redactMCPWorkbenchHeaderMap(requestHeaders),
		"bodyPreview":     truncateMCPWorkbenchText(args.Body, 500),
		"timeoutSeconds":  timeout,
		"followRedirects": followRedirects,
		"extract":         args.Extract,
	})
	executionID := database.GenerateNanoid()
	toolCallID := database.GenerateNanoid()
	toolCall := database.AIWorkbenchToolCall{
		ID:          toolCallID,
		TaskID:      task.ID,
		StepID:      strings.TrimSpace(args.StepID),
		ExecutionID: executionID,
		ToolName:    "http_request",
		InputJSON:   inputJSON,
		Status:      "running",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
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
			ToolName:     "http.request",
			Runtime:      aipkg.WorkbenchToolRuntimeLocalMCP,
			InputSummary: fmt.Sprintf("%s %s", method, requestURL),
			Arguments: map[string]any{
				"method":          method,
				"url":             requestURL,
				"headers":         redactMCPWorkbenchHeaderMap(requestHeaders),
				"bodyPreview":     truncateMCPWorkbenchText(args.Body, 500),
				"timeoutSeconds":  timeout,
				"followRedirects": followRedirects,
				"extract":         args.Extract,
			},
			Status: aipkg.WorkbenchToolExecutionStatusRunning,
			Metadata: map[string]any{
				"toolCallId": toolCall.ID,
			},
		})
	}); err != nil {
		return mcpWorkbenchHTTPRequestResult{}, err
	}
	result, requestErr := performMCPWorkbenchHTTPRequest(method, requestURL, requestHeaders, args.Body, timeout, followRedirects)
	if requestErr != nil {
		toolCall.Status = "failed"
		toolCall.ErrorMessage = requestErr.Error()
		toolCall.OutputSummary = "HTTP 请求执行失败。"
		toolCall.UpdatedAt = time.Now()
		_ = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(&toolCall).Error; err != nil {
				return err
			}
			if err := finishMCPWorkbenchToolExecution(tx, executionID, aipkg.WorkbenchToolExecutionStatusFailed, toolCall.OutputSummary, "", requestErr.Error(), map[string]any{
				"toolCallId": toolCall.ID,
				"url":        requestURL,
				"method":     method,
			}); err != nil {
				return err
			}
			return createMCPWorkbenchEvent(tx, task.ID, toolCall.StepID, "http.request.failed", "error", "HTTP 接口请求失败", requestErr.Error(), gin.H{"url": requestURL, "method": method})
		})
		return mcpWorkbenchHTTPRequestResult{}, requestErr
	}
	endpoint, err := upsertMCPWorkbenchAPIEndpoint(db, task, mcpWorkbenchAPIEndpointRecordArgs{
		TaskID:       task.ID,
		StepID:       args.StepID,
		Method:       method,
		URL:          requestURL,
		AuthRequired: mcpWorkbenchRequestIndicatesAuth(requestHeaders),
		Source:       "http_request",
	})
	if err != nil {
		return mcpWorkbenchHTTPRequestResult{}, err
	}
	extracted, err := extractAndRecordMCPWorkbenchHTTPVariables(db, task.ID, args.StepID, endpoint.ID, result, args.Extract)
	if err != nil {
		return mcpWorkbenchHTTPRequestResult{}, err
	}
	for key, value := range inheritedVariables {
		if _, exists := extracted[key]; !exists {
			extracted[key] = value
		}
	}
	httpEvidence := database.AIWorkbenchHTTPRequestEvidence{
		ID:                  database.GenerateNanoid(),
		TaskID:              task.ID,
		StepID:              strings.TrimSpace(args.StepID),
		ToolCallID:          toolCall.ID,
		EndpointID:          endpoint.ID,
		Method:              method,
		URL:                 requestURL,
		RequestHeaders:      redactMCPWorkbenchHeaderMap(requestHeaders),
		RequestBodyPreview:  truncateMCPWorkbenchText(args.Body, mcpWorkbenchHTTPPreviewLimit),
		StatusCode:          result.StatusCode,
		ResponseHeaders:     result.Headers,
		ResponseBodyPreview: truncateMCPWorkbenchText(result.Body, mcpWorkbenchHTTPStoredBodyLimit),
		ResponseLength:      result.BodyLength,
		DurationMS:          result.DurationMS,
		Variables:           extracted,
		CreatedAt:           time.Now(),
	}
	summary := fmt.Sprintf("%s %s -> HTTP %d，响应长度 %d，耗时 %dms。", method, requestURL, result.StatusCode, result.BodyLength, result.DurationMS)
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&httpEvidence).Error; err != nil {
			return err
		}
		toolCall.Status = "completed"
		toolCall.OutputSummary = summary
		toolCall.RawOutputRef = "ai_http_request:" + httpEvidence.ID
		toolCall.UpdatedAt = time.Now()
		if err := tx.Save(&toolCall).Error; err != nil {
			return err
		}
		if err := finishMCPWorkbenchToolExecution(tx, executionID, aipkg.WorkbenchToolExecutionStatusCompleted, summary, toolCall.RawOutputRef, "", map[string]any{
			"toolCallId":     toolCall.ID,
			"endpointId":     endpoint.ID,
			"httpEvidenceId": httpEvidence.ID,
			"statusCode":     result.StatusCode,
			"responseTimeMs": result.DurationMS,
			"variables":      extracted,
		}); err != nil {
			return err
		}
		if err := createMCPWorkbenchEvent(tx, task.ID, toolCall.StepID, "http.request.completed", "info", "HTTP 接口请求完成", summary, gin.H{
			"toolCallId":     toolCall.ID,
			"endpointId":     endpoint.ID,
			"httpEvidenceId": httpEvidence.ID,
			"variables":      extracted,
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return mcpWorkbenchHTTPRequestResult{}, err
	}
	if err := mcpCallWorkbenchFactUpsertNoResult(mcpWorkbenchFactUpsertArgs{
		TaskID:     task.ID,
		StepID:     args.StepID,
		FactKey:    "http/" + endpoint.ID + "/" + strconv.Itoa(result.StatusCode),
		Source:     "http_request",
		TargetRef:  requestURL,
		Summary:    summary,
		Confidence: "medium",
		Metadata: map[string]interface{}{
			"toolCallId":     toolCall.ID,
			"endpointId":     endpoint.ID,
			"httpEvidenceId": httpEvidence.ID,
			"variables":      extracted,
		},
	}); err != nil {
		return mcpWorkbenchHTTPRequestResult{}, err
	}
	if err := mcpCallWorkbenchEvidenceAddNoResult(mcpWorkbenchEvidenceAddArgs{
		TaskID:     task.ID,
		StepID:     args.StepID,
		SourceType: "http_request",
		SourceID:   httpEvidence.ID,
		TargetRef:  requestURL,
		Title:      "HTTP 接口请求响应",
		Summary:    summary,
		RiskLevel:  "info",
		Confidence: "medium",
		Reference:  "ai_http_request:" + httpEvidence.ID,
		Metadata: map[string]interface{}{
			"endpointId": endpoint.ID,
			"statusCode": result.StatusCode,
			"variables":  extracted,
		},
	}); err != nil {
		return mcpWorkbenchHTTPRequestResult{}, err
	}
	return mcpWorkbenchHTTPRequestResult{
		TaskID:         task.ID,
		ExecutionID:    executionID,
		ToolCallID:     toolCall.ID,
		EndpointID:     endpoint.ID,
		HTTPEvidenceID: httpEvidence.ID,
		Method:         method,
		URL:            requestURL,
		StatusCode:     result.StatusCode,
		ResponseTime:   result.DurationMS,
		Headers:        result.Headers,
		BodyPreview:    truncateMCPWorkbenchText(result.Body, mcpWorkbenchHTTPPreviewLimit),
		BodyLength:     result.BodyLength,
		Variables:      extracted,
	}, nil
}

type mcpWorkbenchRawHTTPResult struct {
	StatusCode int
	Headers    map[string]string
	Body       string
	BodyLength int
	DurationMS int64
}

func performMCPWorkbenchHTTPRequest(method string, requestURL string, headers map[string]string, body string, timeoutSeconds int, followRedirects bool) (mcpWorkbenchRawHTTPResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, requestURL, strings.NewReader(body))
	if err != nil {
		return mcpWorkbenchRawHTTPResult{}, fmt.Errorf("构造 HTTP 请求失败: %w", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Hephaestus-AI-Workbench/1.0")
	}
	client := &http.Client{
		Timeout: time.Duration(timeoutSeconds) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
	if !followRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	started := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(started).Milliseconds()
	if err != nil {
		return mcpWorkbenchRawHTTPResult{}, fmt.Errorf("执行 HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, mcpWorkbenchHTTPMaxBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return mcpWorkbenchRawHTTPResult{}, fmt.Errorf("读取 HTTP 响应失败: %w", err)
	}
	bodyText := string(data)
	if len(data) > mcpWorkbenchHTTPMaxBodyBytes {
		bodyText = string(data[:mcpWorkbenchHTTPMaxBodyBytes]) + "\n[truncated]"
	}
	return mcpWorkbenchRawHTTPResult{
		StatusCode: resp.StatusCode,
		Headers:    flattenMCPWorkbenchHeaders(resp.Header),
		Body:       bodyText,
		BodyLength: len(data),
		DurationMS: duration,
	}, nil
}

func upsertMCPWorkbenchAPIEndpoint(db *gorm.DB, task database.AIWorkbenchTask, args mcpWorkbenchAPIEndpointRecordArgs) (database.AIWorkbenchAPIEndpoint, error) {
	method, err := normalizeMCPWorkbenchHTTPMethod(defaultString(args.Method, "GET"))
	if err != nil {
		return database.AIWorkbenchAPIEndpoint{}, err
	}
	rawURL := strings.TrimSpace(args.URL)
	if rawURL == "" {
		return database.AIWorkbenchAPIEndpoint{}, fmt.Errorf("url 不能为空")
	}
	inScope, reason := isMCPWorkbenchTargetInScope(rawURL, append([]string{task.Target}, task.Scope...))
	if !inScope {
		return database.AIWorkbenchAPIEndpoint{}, fmt.Errorf("接口不在授权范围内: %s", reason)
	}
	host, path := splitMCPWorkbenchURL(rawURL)
	now := time.Now()
	var endpoint database.AIWorkbenchAPIEndpoint
	err = db.Where("task_id = ? AND method = ? AND url = ?", task.ID, method, rawURL).First(&endpoint).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return database.AIWorkbenchAPIEndpoint{}, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		endpoint = database.AIWorkbenchAPIEndpoint{
			ID:             database.GenerateNanoid(),
			TaskID:         task.ID,
			StepID:         strings.TrimSpace(args.StepID),
			Method:         method,
			URL:            rawURL,
			Host:           host,
			Path:           path,
			OperationName:  strings.TrimSpace(args.OperationName),
			AuthRequired:   args.AuthRequired,
			AuthProfileID:  strings.TrimSpace(args.AuthProfileID),
			Source:         defaultString(strings.TrimSpace(args.Source), "manual"),
			Tags:           normalizeNonEmptyStrings(args.Tags),
			Metadata:       args.Metadata,
			CreatedAt:      now,
			UpdatedAt:      now,
			LastObservedAt: now,
		}
		if err := db.Create(&endpoint).Error; err != nil {
			return database.AIWorkbenchAPIEndpoint{}, err
		}
	} else {
		endpoint.StepID = defaultString(strings.TrimSpace(args.StepID), endpoint.StepID)
		endpoint.Host = host
		endpoint.Path = path
		if strings.TrimSpace(args.OperationName) != "" {
			endpoint.OperationName = strings.TrimSpace(args.OperationName)
		}
		if args.AuthRequired {
			endpoint.AuthRequired = true
		}
		if strings.TrimSpace(args.AuthProfileID) != "" {
			endpoint.AuthProfileID = strings.TrimSpace(args.AuthProfileID)
		}
		if strings.TrimSpace(args.Source) != "" {
			endpoint.Source = strings.TrimSpace(args.Source)
		}
		if len(args.Tags) > 0 {
			endpoint.Tags = normalizeNonEmptyStrings(args.Tags)
		}
		if args.Metadata != nil {
			endpoint.Metadata = args.Metadata
		}
		endpoint.UpdatedAt = now
		endpoint.LastObservedAt = now
		if err := db.Save(&endpoint).Error; err != nil {
			return database.AIWorkbenchAPIEndpoint{}, err
		}
	}
	_ = createMCPWorkbenchEvent(db, task.ID, endpoint.StepID, "api.endpoint.recorded", "info", "API 节点已记录", fmt.Sprintf("%s %s", endpoint.Method, endpoint.URL), gin.H{"endpointId": endpoint.ID})
	return endpoint, nil
}

func upsertMCPWorkbenchAPIRelation(db *gorm.DB, args mcpWorkbenchAPIRelationRecordArgs) (database.AIWorkbenchAPIRelation, error) {
	taskID := strings.TrimSpace(args.TaskID)
	sourceID := strings.TrimSpace(args.SourceEndpointID)
	targetID := strings.TrimSpace(args.TargetEndpointID)
	relationType := strings.TrimSpace(args.RelationType)
	if taskID == "" || sourceID == "" || targetID == "" || relationType == "" {
		return database.AIWorkbenchAPIRelation{}, fmt.Errorf("taskId、sourceEndpointId、targetEndpointId、relationType 均不能为空")
	}
	if err := ensureMCPWorkbenchEndpointBelongsToTask(db, taskID, sourceID); err != nil {
		return database.AIWorkbenchAPIRelation{}, err
	}
	if err := ensureMCPWorkbenchEndpointBelongsToTask(db, taskID, targetID); err != nil {
		return database.AIWorkbenchAPIRelation{}, err
	}
	now := time.Now()
	var relation database.AIWorkbenchAPIRelation
	err := db.Where("task_id = ? AND source_endpoint_id = ? AND target_endpoint_id = ? AND relation_type = ? AND variable_name = ?", taskID, sourceID, targetID, relationType, strings.TrimSpace(args.VariableName)).First(&relation).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return database.AIWorkbenchAPIRelation{}, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		relation = database.AIWorkbenchAPIRelation{
			ID:               database.GenerateNanoid(),
			TaskID:           taskID,
			SourceEndpointID: sourceID,
			TargetEndpointID: targetID,
			RelationType:     relationType,
			VariableName:     strings.TrimSpace(args.VariableName),
			ObjectType:       strings.TrimSpace(args.ObjectType),
			EvidenceRef:      strings.TrimSpace(args.EvidenceRef),
			Confidence:       normalizeMCPWorkbenchConfidence(args.Confidence),
			Metadata:         args.Metadata,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := db.Create(&relation).Error; err != nil {
			return database.AIWorkbenchAPIRelation{}, err
		}
	} else {
		relation.ObjectType = defaultString(strings.TrimSpace(args.ObjectType), relation.ObjectType)
		relation.EvidenceRef = defaultString(strings.TrimSpace(args.EvidenceRef), relation.EvidenceRef)
		relation.Confidence = normalizeMCPWorkbenchConfidence(defaultString(args.Confidence, relation.Confidence))
		if args.Metadata != nil {
			relation.Metadata = args.Metadata
		}
		relation.UpdatedAt = now
		if err := db.Save(&relation).Error; err != nil {
			return database.AIWorkbenchAPIRelation{}, err
		}
	}
	_ = createMCPWorkbenchEvent(db, taskID, "", "api.relation.recorded", "info", "API 关系已记录", relation.RelationType, gin.H{"relationId": relation.ID})
	return relation, nil
}

func extractAndRecordMCPWorkbenchHTTPVariables(db *gorm.DB, taskID string, stepID string, endpointID string, result mcpWorkbenchRawHTTPResult, extractors []mcpWorkbenchHTTPExtractorArg) (map[string]string, error) {
	extracted := map[string]string{}
	for _, extractor := range extractors {
		name := strings.TrimSpace(extractor.Name)
		if name == "" {
			continue
		}
		value, err := extractMCPWorkbenchValue(result.Body, result.Headers, extractor)
		if err != nil {
			return extracted, err
		}
		extracted[name] = value
		if _, err := recordMCPWorkbenchExtractedVariable(db, database.AIWorkbenchExtractedVariable{
			TaskID:     taskID,
			StepID:     strings.TrimSpace(stepID),
			Name:       name,
			Value:      truncateMCPWorkbenchText(value, 4096),
			ValueType:  normalizeMCPWorkbenchVariableType(extractor.ValueType, value),
			SourceType: "http_response",
			SourceID:   endpointID,
			JSONPath:   strings.TrimSpace(extractor.JSONPath),
			Regex:      strings.TrimSpace(extractor.Regex),
			HeaderName: strings.TrimSpace(extractor.HeaderName),
			EndpointID: endpointID,
			Confidence: "high",
		}); err != nil {
			return extracted, err
		}
	}
	return extracted, nil
}

func recordMCPWorkbenchExtractedVariable(db *gorm.DB, variable database.AIWorkbenchExtractedVariable) (database.AIWorkbenchExtractedVariable, error) {
	variable.Name = strings.TrimSpace(variable.Name)
	if variable.TaskID == "" || variable.Name == "" {
		return database.AIWorkbenchExtractedVariable{}, fmt.Errorf("taskId 和 name 不能为空")
	}
	if variable.ID == "" {
		variable.ID = database.GenerateNanoid()
	}
	if variable.SourceType == "" {
		variable.SourceType = "unknown"
	}
	if variable.ValueType == "" {
		variable.ValueType = normalizeMCPWorkbenchVariableType("", variable.Value)
	}
	if variable.Confidence == "" {
		variable.Confidence = "medium"
	}
	variable.CreatedAt = time.Now()
	if err := db.Create(&variable).Error; err != nil {
		return database.AIWorkbenchExtractedVariable{}, err
	}
	_ = createMCPWorkbenchEvent(db, variable.TaskID, variable.StepID, "http.variable.extracted", "info", "接口变量已提取", variable.Name, gin.H{
		"variableId": variable.ID,
		"name":       variable.Name,
		"sourceType": variable.SourceType,
		"endpointId": variable.EndpointID,
	})
	return variable, nil
}

func resolveMCPWorkbenchExtractionInput(db *gorm.DB, taskID string, args mcpWorkbenchHTTPExtractVariableArgs) (string, string, error) {
	if strings.TrimSpace(args.Input) != "" {
		return args.Input, strings.TrimSpace(args.EndpointID), nil
	}
	sourceID := strings.TrimSpace(args.SourceID)
	if sourceID == "" {
		return "", "", fmt.Errorf("input 或 sourceId 不能为空")
	}
	var evidence database.AIWorkbenchHTTPRequestEvidence
	if err := db.Where("id = ? AND task_id = ?", sourceID, taskID).First(&evidence).Error; err != nil {
		return "", "", err
	}
	return evidence.ResponseBodyPreview, evidence.EndpointID, nil
}

func extractMCPWorkbenchValue(body string, headers map[string]string, extractor mcpWorkbenchHTTPExtractorArg) (string, error) {
	if headerName := strings.TrimSpace(extractor.HeaderName); headerName != "" {
		for key, value := range headers {
			if strings.EqualFold(key, headerName) {
				return value, nil
			}
		}
		return "", fmt.Errorf("响应头 %s 不存在", headerName)
	}
	if pattern := strings.TrimSpace(extractor.Regex); pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return "", fmt.Errorf("regex 无效: %w", err)
		}
		matches := re.FindStringSubmatch(body)
		if len(matches) > 1 {
			return matches[1], nil
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		return "", fmt.Errorf("regex 未匹配到变量")
	}
	if path := strings.TrimSpace(extractor.JSONPath); path != "" {
		value, err := extractMCPWorkbenchJSONPath(body, path)
		if err != nil {
			return "", err
		}
		return value, nil
	}
	return "", fmt.Errorf("至少提供 headerName、regex 或 jsonPath")
}

func extractMCPWorkbenchJSONPath(body string, path string) (string, error) {
	var payload interface{}
	decoder := json.NewDecoder(bytes.NewBufferString(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return "", fmt.Errorf("响应不是有效 JSON: %w", err)
	}
	path = strings.TrimPrefix(strings.TrimSpace(path), "$")
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return stringifyMCPWorkbenchValue(payload), nil
	}
	current := payload
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		key := segment
		index := -1
		if bracket := strings.Index(segment, "["); bracket >= 0 && strings.HasSuffix(segment, "]") {
			key = segment[:bracket]
			rawIndex := strings.TrimSuffix(segment[bracket+1:], "]")
			parsed, err := strconv.Atoi(rawIndex)
			if err != nil {
				return "", fmt.Errorf("jsonPath 数组索引无效: %s", segment)
			}
			index = parsed
		}
		if key != "" {
			object, ok := current.(map[string]interface{})
			if !ok {
				return "", fmt.Errorf("jsonPath %s 不存在", path)
			}
			current, ok = object[key]
			if !ok {
				return "", fmt.Errorf("jsonPath %s 不存在", path)
			}
		}
		if index >= 0 {
			items, ok := current.([]interface{})
			if !ok || index >= len(items) {
				return "", fmt.Errorf("jsonPath %s 不存在", path)
			}
			current = items[index]
		}
	}
	return stringifyMCPWorkbenchValue(current), nil
}

func compareMCPWorkbenchHTTPResponses(baseline database.AIWorkbenchHTTPRequestEvidence, candidate database.AIWorkbenchHTTPRequestEvidence, checks []string) []map[string]interface{} {
	diff := []map[string]interface{}{}
	for _, check := range checks {
		switch strings.ToLower(strings.TrimSpace(check)) {
		case "status_code":
			if baseline.StatusCode != candidate.StatusCode {
				diff = append(diff, map[string]interface{}{"field": "statusCode", "baseline": baseline.StatusCode, "candidate": candidate.StatusCode})
			}
		case "body_length":
			if baseline.ResponseLength != candidate.ResponseLength {
				diff = append(diff, map[string]interface{}{"field": "responseLength", "baseline": baseline.ResponseLength, "candidate": candidate.ResponseLength})
			}
		case "content_type":
			left := baseline.ResponseHeaders["Content-Type"]
			right := candidate.ResponseHeaders["Content-Type"]
			if left != right {
				diff = append(diff, map[string]interface{}{"field": "Content-Type", "baseline": left, "candidate": right})
			}
		case "body_hash", "body":
			if baseline.ResponseBodyPreview != candidate.ResponseBodyPreview {
				diff = append(diff, map[string]interface{}{"field": "bodyPreview", "baseline": truncateMCPWorkbenchText(baseline.ResponseBodyPreview, 120), "candidate": truncateMCPWorkbenchText(candidate.ResponseBodyPreview, 120)})
			}
		}
	}
	return diff
}

func findMCPWorkbenchRequestVariableUses(args mcpWorkbenchHTTPRequestArgs) []mcpWorkbenchRequestVariableUse {
	uses := make([]mcpWorkbenchRequestVariableUse, 0)
	seen := map[string]bool{}
	addUses := func(location string, headerName string, text string) {
		for _, match := range mcpWorkbenchVariableRefPattern.FindAllStringSubmatch(text, -1) {
			if len(match) < 2 {
				continue
			}
			name := strings.TrimSpace(match[1])
			if name == "" {
				continue
			}
			key := strings.ToLower(location + "\x00" + headerName + "\x00" + name)
			if seen[key] {
				continue
			}
			seen[key] = true
			uses = append(uses, mcpWorkbenchRequestVariableUse{
				Name:       name,
				Location:   location,
				HeaderName: headerName,
			})
		}
	}
	addUses("url", "", args.URL)
	addUses("body", "", args.Body)
	for key, value := range args.Headers {
		addUses("header", key, value)
	}
	return uses
}

func mcpWorkbenchAuthVariableUseSet(uses []mcpWorkbenchRequestVariableUse) map[string]bool {
	authVariables := map[string]bool{}
	for _, use := range uses {
		header := strings.ToLower(strings.TrimSpace(use.HeaderName))
		if use.Location == "header" && (header == "authorization" || strings.Contains(header, "cookie") || strings.Contains(header, "token")) {
			authVariables[use.Name] = true
			continue
		}
		lowerName := strings.ToLower(use.Name)
		if strings.Contains(lowerName, "token") || strings.Contains(lowerName, "session") || strings.Contains(lowerName, "jwt") || strings.Contains(lowerName, "cookie") {
			authVariables[use.Name] = true
		}
	}
	return authVariables
}

func recordMCPWorkbenchHTTPSequenceVariableRelations(db *gorm.DB, taskID string, result mcpWorkbenchHTTPRequestResult, uses []mcpWorkbenchRequestVariableUse, authVariables map[string]bool, variableSources map[string]string, sequenceIndex int) {
	seen := map[string]bool{}
	for _, use := range uses {
		sourceEndpointID := strings.TrimSpace(variableSources[use.Name])
		if sourceEndpointID == "" || sourceEndpointID == result.EndpointID {
			continue
		}
		relationType := "variable_flow"
		objectType := inferMCPWorkbenchRelationObjectType(use.Name)
		if authVariables[use.Name] {
			relationType = "auth_flow"
			objectType = "token"
		}
		key := strings.Join([]string{sourceEndpointID, result.EndpointID, relationType, use.Name}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		_, _ = upsertMCPWorkbenchAPIRelation(db, mcpWorkbenchAPIRelationRecordArgs{
			TaskID:           taskID,
			SourceEndpointID: sourceEndpointID,
			TargetEndpointID: result.EndpointID,
			RelationType:     relationType,
			VariableName:     use.Name,
			ObjectType:       objectType,
			EvidenceRef:      "ai_http_request:" + result.HTTPEvidenceID,
			Confidence:       "high",
			Metadata: map[string]interface{}{
				"sequenceIndex": sequenceIndex,
				"requestPart":   use.Location,
				"headerName":    use.HeaderName,
			},
		})
	}
}

func inferMCPWorkbenchRelationObjectType(variableName string) string {
	name := strings.ToLower(strings.TrimSpace(variableName))
	switch {
	case strings.Contains(name, "token"), strings.Contains(name, "session"), strings.Contains(name, "jwt"), strings.Contains(name, "cookie"):
		return "token"
	case strings.HasSuffix(name, "id"), strings.Contains(name, "_id"), strings.Contains(name, "userid"), strings.Contains(name, "user_id"):
		return "id"
	default:
		return "variable"
	}
}

func mcpWorkbenchRequestIndicatesAuth(headers map[string]string) bool {
	for key, value := range headers {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if (lowerKey == "authorization" || lowerKey == "cookie" || strings.Contains(lowerKey, "token")) && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func normalizeMCPWorkbenchHTTPMethod(method string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodPatch:
		return method, nil
	default:
		return "", fmt.Errorf("HTTP method %s 未开放；当前仅允许 GET/HEAD/OPTIONS/POST/PUT/PATCH", method)
	}
}

func splitMCPWorkbenchURL(rawURL string) (string, string) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	return strings.ToLower(parsed.Hostname()), parsed.EscapedPath()
}

func sanitizeMCPWorkbenchRequestHeaders(headers map[string]string) map[string]string {
	cleaned := map[string]string{}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.ContainsAny(key, "\r\n") {
			continue
		}
		cleaned[key] = strings.TrimSpace(value)
	}
	return cleaned
}

func redactMCPWorkbenchHeaderMap(headers map[string]string) map[string]string {
	redacted := map[string]string{}
	for key, value := range headers {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "key") {
			redacted[key] = "***"
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func flattenMCPWorkbenchHeaders(headers http.Header) map[string]string {
	flat := map[string]string{}
	for key, values := range headers {
		flat[key] = strings.Join(values, ", ")
	}
	return flat
}

func cloneStringMap(input map[string]string) map[string]string {
	output := map[string]string{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func substituteMCPWorkbenchHeaderVariables(headers map[string]string, variables map[string]string) map[string]string {
	output := map[string]string{}
	for key, value := range headers {
		output[key] = substituteMCPWorkbenchVariables(value, variables)
	}
	return output
}

func substituteMCPWorkbenchVariables(value string, variables map[string]string) string {
	for key, variable := range variables {
		value = strings.ReplaceAll(value, "{{"+key+"}}", variable)
	}
	return value
}

func stringifyMCPWorkbenchValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		data, _ := json.Marshal(typed)
		return string(data)
	}
}

func normalizeMCPWorkbenchVariableType(valueType string, value string) string {
	valueType = strings.ToLower(strings.TrimSpace(valueType))
	switch valueType {
	case "string", "number", "boolean", "json", "token", "id":
		return valueType
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return "number"
	}
	if value == "true" || value == "false" {
		return "boolean"
	}
	if strings.HasPrefix(strings.TrimSpace(value), "{") || strings.HasPrefix(strings.TrimSpace(value), "[") {
		return "json"
	}
	return "string"
}

func ensureMCPWorkbenchEndpointBelongsToTask(db *gorm.DB, taskID string, endpointID string) error {
	var count int64
	if err := db.Model(&database.AIWorkbenchAPIEndpoint{}).Where("id = ? AND task_id = ?", endpointID, taskID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("API endpoint 不属于当前任务: %s", endpointID)
	}
	return nil
}

func marshalMCPWorkbenchJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func truncateMCPWorkbenchText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "\n[truncated]"
}

func mcpCallWorkbenchFactUpsertNoResult(args mcpWorkbenchFactUpsertArgs) error {
	_, err := mcpCallWorkbenchFactUpsert(args)
	return err
}

func mcpCallWorkbenchEvidenceAddNoResult(args mcpWorkbenchEvidenceAddArgs) error {
	_, err := mcpCallWorkbenchEvidenceAdd(args)
	return err
}
