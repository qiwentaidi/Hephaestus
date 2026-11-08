package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

type pentestJSScanTaskArgs struct {
	TaskID  string `json:"taskId"`
	Version int    `json:"version"`
}

type pentestJSScanOperationSearchArgs struct {
	pentestJSScanTaskArgs
	Query           string `json:"query"`
	Method          string `json:"method"`
	Source          string `json:"source"`
	RequiresRuntime *bool  `json:"requiresRuntime"`
	Page            int    `json:"page"`
	PageSize        int    `json:"pageSize"`
}

type pentestJSScanResourceSearchArgs struct {
	pentestJSScanTaskArgs
	Query    string `json:"query"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type pentestJSScanEvidenceArgs struct {
	pentestJSScanTaskArgs
	EvidenceType string `json:"evidenceType"`
	EvidenceID   string `json:"evidenceId"`
	DetailLevel  string `json:"detailLevel"`
	Field        string `json:"field"`
	Offset       int    `json:"offset"`
	Length       int    `json:"length"`
	Query        string `json:"query"`
	ContentHash  string `json:"contentHash"`
}

type pentestJSScanOpenAPIArgs struct {
	pentestJSScanTaskArgs
	Target      string `json:"target"`
	DocumentID  string `json:"documentId"`
	Offset      int    `json:"offset"`
	Length      int    `json:"length"`
	Query       string `json:"query"`
	ContentHash string `json:"contentHash"`
}

func loadPentestJSScanTask(args pentestJSScanTaskArgs) (*gorm.DB, database.VulnScanTask, error) {
	taskID, version, err := normalizePentestTaskReference(args.TaskID, args.Version)
	if err != nil {
		return nil, database.VulnScanTask{}, err
	}
	db, err := database.GetDB()
	if err != nil {
		return nil, database.VulnScanTask{}, err
	}
	query := db.Where("id = ?", taskID)
	if version > 0 {
		query = query.Where("version = ?", version)
	} else {
		query = query.Order("version DESC")
	}
	var task database.VulnScanTask
	if err := query.First(&task).Error; err != nil {
		return nil, database.VulnScanTask{}, fmt.Errorf("JS/API 扫描任务不存在: %s", taskID)
	}
	if task.ScanType != "jsscan" {
		return nil, database.VulnScanTask{}, fmt.Errorf("任务不是 JS/API 扫描: %s", taskID)
	}
	return db, task, nil
}

func getPentestJSScanOverview(raw map[string]any) (any, bool, error) {
	var args pentestJSScanTaskArgs
	if err := decodePentestToolArgs(raw, &args); err != nil {
		return nil, true, err
	}
	db, task, err := loadPentestJSScanTask(args)
	if err != nil {
		return nil, true, err
	}
	var targets []string
	_ = json.Unmarshal([]byte(task.Targets), &targets)
	where := "task_id = ? AND version = ?"
	params := []any{task.ID, task.Version}
	var jsResources, runtimeRecords, operations, vulnerabilities, assets, openAPI int64
	for _, item := range []struct {
		model any
		out   *int64
	}{{&database.VulnScanJSResource{}, &jsResources}, {&database.VulnScanJSAPIResource{}, &runtimeRecords}, {&database.VulnScanJSOperation{}, &operations}, {&database.VulnScanJSVulnerability{}, &vulnerabilities}, {&database.VulnScanJSAsset{}, &assets}, {&database.VulnScanJSOpenAPIDocument{}, &openAPI}} {
		if err := db.Model(item.model).Where(where, params...).Count(item.out).Error; err != nil {
			return nil, true, err
		}
	}
	var severity []struct {
		Level string
		Count int64
	}
	if err := db.Model(&database.VulnScanJSVulnerability{}).Select("LOWER(level) AS level, COUNT(*) AS count").Where(where, params...).Group("LOWER(level)").Scan(&severity).Error; err != nil {
		return nil, true, err
	}
	return map[string]any{
		"taskId": task.ID, "version": task.Version, "targets": targets, "status": task.Status, "progress": task.Progress,
		"counts":                map[string]any{"jsResources": jsResources, "runtimeRecords": runtimeRecords, "staticOperations": operations, "vulnerabilities": vulnerabilities, "assets": assets, "openAPIDocuments": openAPI},
		"vulnerabilitySeverity": severity,
		"nextAction":            "复用本 taskId/version。漏洞分析先用 attack_surface_search_jsscan_vulnerabilities 分页获取真实漏洞 ID，再用 attack_surface_get_jsscan_evidence（evidenceType=vulnerability，evidenceId=漏洞 id）读取证据，勿使用任务 ID 或猜测序号。接口分析用 attack_surface_search_jsscan_operations；加密协议分析用 attack_surface_search_jsscan_resources 检索 JS，再按 offset/length 分段读取。采集不自动生成加密协议，勿因缺少分析结论重复扫描。",
	}, true, nil
}

func searchPentestJSScanOperations(raw map[string]any) (any, bool, error) {
	var args pentestJSScanOperationSearchArgs
	if err := decodePentestToolArgs(raw, &args); err != nil {
		return nil, true, err
	}
	db, task, err := loadPentestJSScanTask(args.pentestJSScanTaskArgs)
	if err != nil {
		return nil, true, err
	}
	page, pageSize := args.Page, args.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}
	query := db.Model(&database.VulnScanJSOperation{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if value := strings.TrimSpace(args.Query); value != "" {
		like := "%" + value + "%"
		query = query.Where("operation_id LIKE ? OR path_template LIKE ? OR target_url LIKE ?", like, like, like)
	}
	if value := strings.TrimSpace(args.Method); value != "" {
		query = query.Where("UPPER(method) = UPPER(?)", value)
	}
	if value := strings.TrimSpace(args.Source); value != "" {
		query = query.Where("source = ?", value)
	}
	if args.RequiresRuntime != nil {
		query = query.Where("requires_runtime = ?", *args.RequiresRuntime)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, true, err
	}
	var rows []database.VulnScanJSOperation
	if err := query.Order("requires_runtime DESC").Order("confidence DESC").Order("path_template ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, true, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"id": row.ID, "operationId": row.OperationID, "target": row.TargetURL, "method": row.Method, "pathTemplate": row.PathTemplate, "source": row.Source, "confidence": row.Confidence, "requiresRuntime": row.RequiresRuntime})
	}
	return map[string]any{"taskId": task.ID, "version": task.Version, "operations": items, "total": total, "page": page, "pageSize": pageSize}, true, nil
}

func searchPentestJSScanResources(raw map[string]any) (any, bool, error) {
	var args pentestJSScanResourceSearchArgs
	if err := decodePentestToolArgs(raw, &args); err != nil {
		return nil, true, err
	}
	db, task, err := loadPentestJSScanTask(args.pentestJSScanTaskArgs)
	if err != nil {
		return nil, true, err
	}
	return searchPentestJSScanResourcesWithDB(db, task, args)
}

func searchPentestJSScanResourcesWithDB(db *gorm.DB, task database.VulnScanTask, args pentestJSScanResourceSearchArgs) (any, bool, error) {
	page, pageSize := args.Page, args.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}
	query := db.Model(&database.VulnScanJSResource{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if value := strings.TrimSpace(args.Query); value != "" {
		escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(value))
		like := "%" + escaped + "%"
		query = query.Where("LOWER(url) LIKE ? ESCAPE '!' OR LOWER(content) LIKE ? ESCAPE '!'", like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, true, err
	}
	var rows []database.VulnScanJSResource
	if err := query.Order("fetched_at DESC").Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, true, err
	}
	resources := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		resources = append(resources, map[string]any{
			"id": row.ID, "url": row.URL, "responseCode": row.ResponseCode, "size": row.Size,
			"fetchedAt": row.FetchedAt, "contentPreview": compactPentestProgressText(row.Content, 1200),
			"contentTruncated": len([]rune(row.Content)) > 1200,
			"matches":          jsContentMatches(row.Content, strings.TrimSpace(args.Query)),
			"totalLength":      len([]rune(row.Content)), "offsetUnit": "unicode_code_points",
		})
	}
	return map[string]any{
		"taskId": task.ID, "version": task.Version, "resources": resources,
		"total": total, "page": page, "pageSize": pageSize,
	}, true, nil
}

func getPentestJSScanEvidence(raw map[string]any) (any, bool, error) {
	var args pentestJSScanEvidenceArgs
	if err := decodePentestToolArgs(raw, &args); err != nil {
		return nil, true, err
	}
	if strings.TrimSpace(args.EvidenceID) == "" {
		return nil, true, fmt.Errorf("evidenceId 必填；漏洞 ID 请先通过 %s 获取，不能使用 taskId 代替", jsScanVulnerabilitySearchTool)
	}
	db, task, err := loadPentestJSScanTask(args.pentestJSScanTaskArgs)
	if err != nil {
		return nil, true, err
	}
	return getPentestJSScanEvidenceWithDB(db, task, args)
}

func getPentestJSScanEvidenceWithDB(db *gorm.DB, task database.VulnScanTask, args pentestJSScanEvidenceArgs) (any, bool, error) {
	full := strings.EqualFold(strings.TrimSpace(args.DetailLevel), "full")
	limit := 3000
	if full {
		limit = 16000
	}
	switch strings.TrimSpace(args.EvidenceType) {
	case "operation":
		var row database.VulnScanJSOperation
		if err := db.Where("id = ? AND task_id = ? AND version = ?", args.EvidenceID, task.ID, task.Version).First(&row).Error; err != nil {
			return nil, true, fmt.Errorf("API 操作证据不存在: %s", args.EvidenceID)
		}
		return map[string]any{"type": "operation", "id": row.ID, "operationId": row.OperationID, "method": row.Method, "pathTemplate": row.PathTemplate, "source": row.Source, "confidence": row.Confidence, "requiresRuntime": row.RequiresRuntime, "operation": compactPentestProgressText(row.OperationJSON, limit), "truncated": len([]rune(row.OperationJSON)) > limit}, true, nil
	case "api_record":
		var row database.VulnScanJSAPIResource
		if err := db.Where("id = ? AND task_id = ? AND version = ?", args.EvidenceID, task.ID, task.Version).First(&row).Error; err != nil {
			return nil, true, fmt.Errorf("运行时 API 证据不存在: %s", args.EvidenceID)
		}
		return map[string]any{"type": "api_record", "id": row.ID, "url": row.URL, "method": row.Method, "responseCode": row.ResponseCode, "requestHeaders": redactPentestJSScanHeaders(row.RequestHeaders), "responseHeaders": redactPentestJSScanHeaders(row.ResponseHeaders), "requestBody": compactPentestProgressText(row.RequestBody, limit), "responseBody": compactPentestProgressText(row.ResponseBody, limit), "truncated": len([]rune(row.RequestBody)) > limit || len([]rune(row.ResponseBody)) > limit}, true, nil
	case "vulnerability":
		var row database.VulnScanJSVulnerability
		if err := db.Where("id = ? AND task_id = ? AND version = ?", args.EvidenceID, task.ID, task.Version).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, true, fmt.Errorf("漏洞证据不存在: %s（taskId=%s, version=%d）；请用 %s 查询该版本的真实漏洞 ID，勿使用任务 ID 或猜测序号", args.EvidenceID, task.ID, task.Version, jsScanVulnerabilitySearchTool)
			}
			return nil, true, fmt.Errorf("读取漏洞证据失败: %w", err)
		}
		if field := strings.TrimSpace(args.Field); field != "" {
			var content string
			switch field {
			case "request":
				content = row.Request
			case "response":
				content = row.Response
			default:
				return nil, true, fmt.Errorf("漏洞证据 field 只支持 request 或 response")
			}
			window, err := jsContentWindow(content, args, limit)
			if err != nil {
				return nil, true, err
			}
			window["type"], window["field"], window["id"] = "vulnerability", field, row.ID
			window["taskId"], window["version"] = row.TaskID, row.Version
			return window, true, nil
		}
		request, response := []rune(row.Request), []rune(row.Response)
		return map[string]any{
			"type": "vulnerability", "id": row.ID, "taskId": row.TaskID, "version": row.Version,
			"title": row.Title, "level": row.Level, "vulnerabilityType": row.Type, "url": row.URL, "method": row.Method,
			"confidence": row.Confidence, "confidenceReason": row.ConfidenceReason, "description": row.Description, "staticContexts": row.StaticContexts,
			"request": string(request[:min(len(request), limit)]), "response": string(response[:min(len(response), limit)]),
			"hasRequest": row.Request != "", "hasResponse": row.Response != "",
			"requestTruncated": len(request) > limit, "responseTruncated": len(response) > limit,
			"truncated":  len(request) > limit || len(response) > limit,
			"nextAction": "报文截断时指定 field=request 或 response，并用 offset/length 分段读取，固定 taskId/version/contentHash，按 nextOffset 继续到 hasMore=false；content 保留原始换行。",
		}, true, nil
	case "js_resource":
		var row database.VulnScanJSResource
		if err := db.Where("id = ? AND task_id = ? AND version = ?", args.EvidenceID, task.ID, task.Version).First(&row).Error; err != nil {
			return nil, true, fmt.Errorf("JS 资源证据不存在: %s", args.EvidenceID)
		}
		window, err := jsContentWindow(row.Content, args, limit)
		if err != nil {
			return nil, true, err
		}
		window["type"], window["id"], window["url"] = "js_resource", row.ID, row.URL
		window["taskId"], window["version"] = task.ID, task.Version
		window["responseCode"], window["size"] = row.ResponseCode, row.Size
		return window, true, nil
	default:
		return nil, true, fmt.Errorf("不支持的 evidenceType: %s", args.EvidenceType)
	}
}

func exportPentestJSScanOpenAPI(raw map[string]any) (any, bool, error) {
	var args pentestJSScanOpenAPIArgs
	if err := decodePentestToolArgs(raw, &args); err != nil {
		return nil, true, err
	}
	db, task, err := loadPentestJSScanTask(args.pentestJSScanTaskArgs)
	if err != nil {
		return nil, true, err
	}
	query := db.Where("task_id = ? AND version = ?", task.ID, task.Version)
	if target := strings.TrimSpace(args.Target); target != "" {
		query = query.Where("target_url = ?", target)
	}
	documentID := strings.TrimSpace(args.DocumentID)
	if documentID != "" {
		query = query.Where("id = ?", documentID)
	}
	var rows []database.VulnScanJSOpenAPIDocument
	if err := query.Order("target_url ASC").Find(&rows).Error; err != nil {
		return nil, true, err
	}
	if documentID != "" {
		if len(rows) != 1 {
			return nil, true, fmt.Errorf("OpenAPI 文档不存在: %s", documentID)
		}
		window, err := openAPIDocumentWindow(rows[0].Document, args)
		if err != nil {
			return nil, true, err
		}
		return map[string]any{"taskId": task.ID, "version": task.Version, "documentId": rows[0].ID, "target": rows[0].TargetURL, "window": window}, true, nil
	}
	documents := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		documents = append(documents, map[string]any{"id": row.ID, "target": row.TargetURL, "document": compactPentestProgressText(row.Document, 24000), "truncated": len([]rune(row.Document)) > 24000, "totalLength": len([]rune(row.Document))})
	}
	return map[string]any{"taskId": task.ID, "version": task.Version, "documents": documents}, true, nil
}

func openAPIDocumentWindow(document string, args pentestJSScanOpenAPIArgs) (map[string]any, error) {
	return jsContentWindow(document, pentestJSScanEvidenceArgs{
		Offset: args.Offset, Length: args.Length, Query: args.Query, ContentHash: args.ContentHash,
	}, 8000)
}

func redactPentestJSScanHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for name, value := range headers {
		lower := strings.ToLower(strings.TrimSpace(name))
		if strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "api-key") {
			out[name] = "[REDACTED]"
		} else {
			out[name] = value
		}
	}
	return out
}
