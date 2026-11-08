package ai

import (
	"context"
	"strings"

	"gorm.io/gorm"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/database"
)

const jsScanVulnerabilitySearchTool = "attack_surface_search_jsscan_vulnerabilities"

type pentestJSScanVulnerabilitySearchArgs struct {
	pentestJSScanTaskArgs
	Query    string `json:"query"`
	Level    string `json:"level"`
	Type     string `json:"type"`
	Method   string `json:"method"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

func pentestJSScanVulnerabilitySearchSchema() pentest.AgentToolSchema {
	return pentest.AgentToolSchema{
		Type: "function",
		Function: pentest.AgentToolFunction{
			Name:        jsScanVulnerabilitySearchTool,
			Description: "分页搜索 JS/API 扫描保存的漏洞，返回真实漏洞 ID、taskId/version、标题、等级、类型、URL 和证据可用性。按返回的 id 调用 attack_surface_get_jsscan_evidence（evidenceType=vulnerability），并固定 taskId/version 读取请求、响应和静态上下文；不要猜测漏洞 ID 或使用任务 ID 代替。扫描命中是候选线索，仍需独立复核；缺少请求/响应不代表可确认漏洞。",
			Parameters: pentestNativeObjectSchema(map[string]any{
				"taskId":   map[string]any{"type": "string", "description": "JS/API 扫描任务 ID。"},
				"version":  map[string]any{"type": "integer", "description": "可选，扫描版本；默认当前版本，后续证据读取应固定返回的版本。"},
				"query":    map[string]any{"type": "string", "description": "可选，在漏洞 ID、标题、类型、URL 或描述中搜索单个字面子串；不支持正则或 OR。"},
				"level":    map[string]any{"type": "string", "description": "可选，等级筛选，例如 high、medium、low、info。"},
				"type":     map[string]any{"type": "string", "description": "可选，按漏洞类型精确筛选。"},
				"method":   map[string]any{"type": "string", "description": "可选，按 HTTP 请求方法筛选。"},
				"page":     map[string]any{"type": "integer", "minimum": 1, "description": "页码，默认 1。"},
				"pageSize": map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "description": "每页条数，默认 20，最多 50。"},
			}, "taskId"),
		},
	}
}

func searchPentestJSScanVulnerabilities(ctx context.Context, raw map[string]any) (any, bool, error) {
	var args pentestJSScanVulnerabilitySearchArgs
	if err := decodePentestToolArgs(raw, &args); err != nil {
		return nil, true, err
	}
	db, task, err := loadPentestJSScanTask(args.pentestJSScanTaskArgs)
	if err != nil {
		return nil, true, err
	}
	return searchPentestJSScanVulnerabilitiesWithDB(db.WithContext(ctx), task, args)
}

func searchPentestJSScanVulnerabilitiesWithDB(db *gorm.DB, task database.VulnScanTask, args pentestJSScanVulnerabilitySearchArgs) (any, bool, error) {
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
	query := db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if value := strings.TrimSpace(args.Query); value != "" {
		escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(value))
		like := "%" + escaped + "%"
		query = query.Where("LOWER(id) LIKE ? ESCAPE '!' OR LOWER(title) LIKE ? ESCAPE '!' OR LOWER(type) LIKE ? ESCAPE '!' OR LOWER(url) LIKE ? ESCAPE '!' OR LOWER(description) LIKE ? ESCAPE '!'", like, like, like, like, like)
	}
	for _, filter := range []struct{ column, value string }{{"level", args.Level}, {"type", args.Type}, {"method", args.Method}} {
		if value := strings.TrimSpace(filter.value); value != "" {
			query = query.Where("LOWER("+filter.column+") = ?", strings.ToLower(value))
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, true, err
	}
	// Leave large payloads in the evidence reader; only inspect their presence here.
	var rows []struct {
		database.VulnScanJSVulnerability
		HasRequest  bool
		HasResponse bool
	}
	if err := query.Select("id, task_id, version, title, level, type, url, method, confidence, response_type, ai_verified, disposal_status, created_at, (COALESCE(request, '') <> '') AS has_request, (COALESCE(response, '') <> '') AS has_response").Order("created_at DESC").Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&rows).Error; err != nil {
		return nil, true, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"id": row.ID, "taskId": row.TaskID, "version": row.Version,
			"title": row.Title, "level": row.Level, "vulnerabilityType": row.Type,
			"url": row.URL, "method": row.Method, "confidence": row.Confidence,
			"responseType": row.ResponseType, "aiVerified": row.AIVerified,
			"disposalStatus": row.DisposalStatus, "createdAt": row.CreatedAt,
			"hasRequest": row.HasRequest, "hasResponse": row.HasResponse,
		})
	}
	return map[string]any{
		"taskId": task.ID, "version": task.Version, "vulnerabilities": items,
		"total": total, "page": page, "pageSize": pageSize,
		"hasMore":    int64(page)*int64(pageSize) < total,
		"nextAction": "使用返回的漏洞 id 作为 evidenceId，并携带 taskId/version 调用 attack_surface_get_jsscan_evidence，evidenceType=vulnerability；需要原始报文时使用 detailLevel=full，并检查 truncated。",
	}, true, nil
}
