package toolbox

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/apidoctest"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/httprequest"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type apiDocTestTaskCreateRequest struct {
	SourceURL      string            `json:"sourceUrl"`
	BaseURL        string            `json:"baseUrl"`
	DocumentType   string            `json:"documentType"`
	Headers        map[string]string `json:"headers"`
	MaxEndpoints   int               `json:"maxEndpoints"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
}

type apiDocTestTaskDetail struct {
	Task    database.APIDocTestTask     `json:"task"`
	Results []database.APIDocTestResult `json:"results"`
}

func APIDocTestAnalyzeHandler(c *gin.Context) {
	var req apidoctest.Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	result, err := apidoctest.AnalyzeAndTest(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func APIDocTestRunHandler(c *gin.Context) {
	var req apidoctest.RunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	result, err := apidoctest.RunEndpoint(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func APIDocTestCreateTaskHandler(c *gin.Context) {
	var req apiDocTestTaskCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	req.SourceURL = strings.TrimSpace(req.SourceURL)
	if req.SourceURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "文档 URL 不能为空"})
		return
	}
	if req.DocumentType == "" {
		req.DocumentType = "auto"
	}
	if req.MaxEndpoints <= 0 {
		req.MaxEndpoints = apidoctest.DefaultMaxEndpoints
	}
	if req.MaxEndpoints > apidoctest.MaxEndpoints {
		req.MaxEndpoints = apidoctest.MaxEndpoints
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	now := time.Now()
	task := database.APIDocTestTask{
		ID:           "api_doc_test_" + database.GenerateNanoid(),
		TaskName:     defaultAPIDocTaskName(req.SourceURL, now),
		Status:       "pending",
		Progress:     0,
		SourceURL:    req.SourceURL,
		BaseURL:      strings.TrimSpace(req.BaseURL),
		DocumentType: strings.TrimSpace(req.DocumentType),
		Headers:      sanitizeAPIDocHeaders(req.Headers),
		MaxEndpoints: req.MaxEndpoints,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.Create(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "创建任务失败: " + err.Error()})
		return
	}

	go executeAPIDocTestTask(context.Background(), task.ID, req.TimeoutSeconds)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": task, "message": "任务已创建，正在后台执行"})
}

func APIDocTestUpdateTaskHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "任务ID不能为空"})
		return
	}
	var req apiDocTestTaskCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	req.SourceURL = strings.TrimSpace(req.SourceURL)
	if req.SourceURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "文档 URL 不能为空"})
		return
	}
	if req.DocumentType == "" {
		req.DocumentType = "auto"
	}
	if req.MaxEndpoints <= 0 {
		req.MaxEndpoints = apidoctest.DefaultMaxEndpoints
	}
	if req.MaxEndpoints > apidoctest.MaxEndpoints {
		req.MaxEndpoints = apidoctest.MaxEndpoints
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	var task database.APIDocTestTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询任务失败: " + err.Error()})
		return
	}
	if task.Status == "pending" || task.Status == "running" {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "任务正在运行，不能编辑"})
		return
	}

	if err := updateAPIDocTestTask(db, taskID, req, time.Now()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "更新任务失败: " + err.Error()})
		return
	}
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "读取任务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": task, "message": "任务已更新"})
}

// updateAPIDocTestTask 通过结构体更新任务，让 headers 这类带 serializer 的字段
// 走 GORM 的序列化逻辑。map[string]interface{} 形式的 Updates 不应用字段
// serializer，会把 map 原样交给数据库驱动并报 unsupported type 错误。
// Select 显式列出列，保证 BaseURL、Headers 等被清空时零值也会写入。
func updateAPIDocTestTask(db *gorm.DB, taskID string, req apiDocTestTaskCreateRequest, now time.Time) error {
	updates := database.APIDocTestTask{
		TaskName:     defaultAPIDocTaskName(req.SourceURL, now),
		SourceURL:    req.SourceURL,
		BaseURL:      strings.TrimSpace(req.BaseURL),
		DocumentType: strings.TrimSpace(req.DocumentType),
		Headers:      sanitizeAPIDocHeaders(req.Headers),
		MaxEndpoints: req.MaxEndpoints,
		UpdatedAt:    now,
	}
	return db.Model(&database.APIDocTestTask{}).
		Where("id = ?", taskID).
		Select("task_name", "source_url", "base_url", "document_type", "headers", "max_endpoints", "updated_at").
		Updates(updates).Error
}

func APIDocTestRetestTaskHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "任务ID不能为空"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	var task database.APIDocTestTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询任务失败: " + err.Error()})
		return
	}
	if task.Status == "pending" || task.Status == "running" {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "任务正在运行，请勿重复测试"})
		return
	}

	now := time.Now()
	if err := db.Model(&database.APIDocTestTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":         "pending",
		"progress":       0,
		"endpoint_count": 0,
		"tested_count":   0,
		"error_msg":      "",
		"started_at":     nil,
		"completed_at":   nil,
		"updated_at":     now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "重置任务失败: " + err.Error()})
		return
	}
	go executeAPIDocTestTask(context.Background(), taskID, 15)

	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "读取任务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": task, "message": "任务已重新测试"})
}

func APIDocTestDeleteTaskHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "任务ID不能为空"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	var task database.APIDocTestTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询任务失败: " + err.Error()})
		return
	}
	if task.Status == "pending" || task.Status == "running" {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "任务正在运行，不能删除"})
		return
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_id = ?", taskID).Delete(&database.APIDocTestResult{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", taskID).Delete(&database.APIDocTestTask{}).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "删除任务失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "任务已删除"})
}

func APIDocTestListTasksHandler(c *gin.Context) {
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	offset := 0
	if raw := c.Query("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	var total int64
	if err := db.Model(&database.APIDocTestTask{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "统计任务失败: " + err.Error()})
		return
	}
	var tasks []database.APIDocTestTask
	if err := db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询任务列表失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"tasks":  tasks,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}})
}

func APIDocTestGetTaskHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "任务ID不能为空"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	var task database.APIDocTestTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询任务失败: " + err.Error()})
		return
	}

	var results []database.APIDocTestResult
	if err := db.Where("task_id = ?", taskID).Order("created_at ASC").Find(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询任务结果失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": apiDocTestTaskDetail{Task: task, Results: results}})
}

func executeAPIDocTestTask(ctx context.Context, taskID string, timeoutSeconds int) {
	db, err := database.GetDB()
	if err != nil {
		return
	}

	var task database.APIDocTestTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		return
	}

	startedAt := time.Now()
	if err := db.Model(&database.APIDocTestTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":     "running",
		"started_at": startedAt,
		"updated_at": startedAt,
	}).Error; err != nil {
		return
	}
	_ = db.Where("task_id = ?", taskID).Delete(&database.APIDocTestResult{}).Error

	analysis, err := apidoctest.AnalyzeAndTest(ctx, apidoctest.Request{
		SourceURL:      task.SourceURL,
		BaseURL:        normalizeAPIDocBaseURLForSource(task.BaseURL, task.SourceURL),
		DocumentType:   task.DocumentType,
		Headers:        task.Headers,
		MaxEndpoints:   task.MaxEndpoints,
		GenerateCases:  false,
		RunSafeChecks:  false,
		TimeoutSeconds: timeoutSeconds,
	})
	if err != nil {
		markAPIDocTaskFailed(db, taskID, "解析接口文档失败: "+err.Error())
		return
	}

	total := len(analysis.Endpoints)
	parsedTaskName := apiDocTaskNameFromAnalysis(analysis, task.SourceURL)
	if total == 0 {
		completedAt := time.Now()
		_ = db.Model(&database.APIDocTestTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
			"status":         "completed",
			"task_name":      parsedTaskName,
			"progress":       100,
			"endpoint_count": 0,
			"tested_count":   0,
			"completed_at":   completedAt,
			"updated_at":     completedAt,
		}).Error
		return
	}

	_ = db.Model(&database.APIDocTestTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"endpoint_count": total,
		"task_name":      parsedTaskName,
		"base_url":       analysis.BaseURL,
		"document_type":  analysis.DocumentType,
		"updated_at":     time.Now(),
	}).Error

	for index, endpoint := range analysis.Endpoints {
		runReq, tags := buildAPIDocEndpointRunRequest(endpoint, task.Headers, timeoutSeconds)
		runResult, _ := apidoctest.RunEndpoint(ctx, runReq)
		if runResult.Error != "" {
			tags = appendUniqueAPIDocTag(tags, "request_error")
		}
		saveAPIDocTestResult(db, taskID, endpoint.Summary, runReq, runResult, tags)

		tested := index + 1
		progress := int(float64(tested) / float64(total) * 100)
		_ = db.Model(&database.APIDocTestTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
			"tested_count": tested,
			"progress":     progress,
			"updated_at":   time.Now(),
		}).Error
	}

	completedAt := time.Now()
	_ = db.Model(&database.APIDocTestTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":       "completed",
		"progress":     100,
		"completed_at": completedAt,
		"updated_at":   completedAt,
	}).Error
}

func saveAPIDocTestResult(db *gorm.DB, taskID string, summary string, runReq apidoctest.RunRequest, runResult apidoctest.RunResult, tags []string) {
	result := database.APIDocTestResult{
		ID:                "api_doc_result_" + database.GenerateNanoid(),
		TaskID:            taskID,
		URL:               runReq.URL,
		Method:            runReq.Method,
		Summary:           strings.TrimSpace(summary),
		StatusCode:        runResult.StatusCode,
		StatusText:        runResult.StatusText,
		ResponseLength:    runResult.ResponseLength,
		DurationMS:        runResult.DurationMS,
		TestTags:          tags,
		RawRequest:        buildAPIDocRawRequest(runReq),
		RequestHeaders:    runReq.Headers,
		RequestBody:       runReq.Body,
		ResponseHeaders:   runResult.ResponseHeaders,
		ResponseBody:      runResult.ResponseBody,
		ResponseTruncated: runResult.ResponseTruncated,
		ErrorMsg:          runResult.Error,
		CreatedAt:         time.Now(),
	}
	if result.Method == "" {
		result.Method = runResult.Method
	}
	if result.URL == "" {
		result.URL = runResult.URL
	}
	_ = db.Create(&result).Error
}

func buildAPIDocRawRequest(runReq apidoctest.RunRequest) string {
	return httprequest.BuildRawRequest(httprequest.RawRequest{
		Method:  runReq.Method,
		URL:     runReq.URL,
		Headers: runReq.Headers,
		Body:    runReq.Body,
	})
}

func markAPIDocTaskFailed(db *gorm.DB, taskID string, message string) {
	completedAt := time.Now()
	_ = db.Model(&database.APIDocTestTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":       "failed",
		"error_msg":    message,
		"completed_at": completedAt,
		"updated_at":   completedAt,
	}).Error
}

func buildAPIDocEndpointRunRequest(endpoint apidoctest.Endpoint, headers map[string]string, timeoutSeconds int) (apidoctest.RunRequest, []string) {
	method := strings.ToUpper(strings.TrimSpace(endpoint.Method))
	if method == "" {
		method = http.MethodGet
	}
	targetURL := fillAPIDocEndpointURL(endpoint)
	tags := append([]string{}, endpoint.RiskTags...)
	if endpoint.AuthRequired {
		tags = appendUniqueAPIDocTag(tags, "auth_required")
	}
	if len(tags) == 0 {
		tags = []string{"normal"}
	}
	requestHeaders := sanitizeAPIDocHeaders(headers)
	if endpoint.RequestBody != "" && endpoint.ContentType != "" && !hasAPIDocHeader(requestHeaders, "Content-Type") {
		requestHeaders["Content-Type"] = endpoint.ContentType
	}
	return apidoctest.RunRequest{
		Method:         method,
		URL:            targetURL,
		Headers:        requestHeaders,
		Body:           endpoint.RequestBody,
		TimeoutSeconds: timeoutSeconds,
	}, tags
}

func fillAPIDocEndpointURL(endpoint apidoctest.Endpoint) string {
	targetURL := endpoint.URL
	for _, param := range endpoint.Parameters {
		if strings.EqualFold(param.In, "path") && param.Name != "" {
			targetURL = strings.ReplaceAll(targetURL, "{"+param.Name+"}", sampleAPIDocParamValue(param))
		}
	}
	targetURL = regexp.MustCompile(`\{[^}/]+\}`).ReplaceAllString(targetURL, "1")

	parsed, err := url.Parse(targetURL)
	if err != nil {
		return targetURL
	}
	query := parsed.Query()
	for _, param := range endpoint.Parameters {
		if strings.EqualFold(param.In, "query") && param.Name != "" && query.Get(param.Name) == "" {
			if param.Required {
				query.Set(param.Name, sampleAPIDocParamValue(param))
			}
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func sampleAPIDocParamValue(param apidoctest.Parameter) string {
	name := strings.ToLower(param.Name)
	typ := strings.ToLower(param.Type)
	switch {
	case strings.Contains(name, "id") || strings.Contains(typ, "int") || strings.Contains(typ, "number"):
		return "1"
	case strings.Contains(name, "page"):
		return "1"
	case strings.Contains(name, "size") || strings.Contains(name, "limit"):
		return "10"
	case strings.Contains(name, "date"):
		return time.Now().Format("2006-01-02")
	case strings.Contains(name, "email"):
		return "test@example.com"
	default:
		return "test"
	}
}

func defaultAPIDocTaskName(sourceURL string, now time.Time) string {
	parsed, err := url.Parse(sourceURL)
	if err == nil && parsed.Host != "" {
		path := strings.Trim(parsed.Path, "/")
		if path != "" {
			parts := strings.Split(path, "/")
			path = parts[len(parts)-1]
			return fmt.Sprintf("%s %s", parsed.Host, path)
		}
		return parsed.Host
	}
	return "接口文档测试 " + now.Format("01-02 15:04")
}

func apiDocTaskNameFromAnalysis(result apidoctest.Result, sourceURL string) string {
	if strings.TrimSpace(result.Title) != "" {
		if strings.TrimSpace(result.Version) != "" && result.Version != result.DocumentType {
			return strings.TrimSpace(result.Title) + " " + strings.TrimSpace(result.Version)
		}
		return strings.TrimSpace(result.Title)
	}
	if strings.TrimSpace(result.BaseURL) != "" {
		parsed, err := url.Parse(result.BaseURL)
		if err == nil && parsed.Host != "" {
			return parsed.Host
		}
	}
	return defaultAPIDocTaskName(sourceURL, time.Now())
}

func normalizeAPIDocBaseURLForSource(baseURL string, sourceURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}
	baseParsed, baseErr := url.Parse(baseURL)
	sourceParsed, sourceErr := url.Parse(strings.TrimSpace(sourceURL))
	if baseErr != nil || sourceErr != nil {
		return baseURL
	}
	if baseParsed.Scheme == "" || baseParsed.Host == "" || sourceParsed.Scheme == "" || sourceParsed.Host == "" {
		return baseURL
	}
	if baseParsed.Host != sourceParsed.Host {
		return baseURL
	}
	if sourceParsed.Scheme != "http" && sourceParsed.Scheme != "https" {
		return baseURL
	}
	baseParsed.Scheme = sourceParsed.Scheme
	return strings.TrimRight(baseParsed.String(), "/")
}

func sanitizeAPIDocHeaders(headers map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func appendUniqueAPIDocTag(tags []string, tag string) []string {
	for _, existing := range tags {
		if existing == tag {
			return tags
		}
	}
	return append(tags, tag)
}

func hasAPIDocHeader(headers map[string]string, name string) bool {
	for key := range headers {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}
