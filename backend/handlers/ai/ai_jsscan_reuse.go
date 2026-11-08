package ai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/jsscan"
	"hephaestus-backend/pkg/scantask"
)

const jsCollectionScopeKey = "_aiJSCollectionScope"

var jsCollectionCreateMu sync.Mutex

func pentestJSScanCreateTool() pentest.AgentToolSchema {
	return pentest.AgentToolSchema{
		Type: "function",
		Function: pentest.AgentToolFunction{
			Name:        "attack_surface_create_jsscan_task",
			Description: "为已授权的 HTTP/HTTPS 网站创建异步 JS/API 采集任务，采集 JavaScript、接口模板和运行时请求记录。默认仅采集，config.vulnDetection.enabled=true 时额外执行平台漏洞检测。同一会话中相同目标和配置复用已有任务；forceRescan=true 并提供 rescanReason 可重新采集。返回 taskId、version、status 和 reused，创建成功不代表扫描完成。可用 attack_surface_get_jsscan_overview 查询进度，按需检索接口、JS 资源及原始证据，或读取 OpenAPI 文档。",
			Parameters: pentestNativeObjectSchema(map[string]any{
				"targets":  map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"}, "description": "已授权的网站完整 HTTP/HTTPS URL，可包含路径和 SPA 路由。"},
				"taskName": map[string]any{"type": "string", "description": "可选，任务名称。"},
				"scanMode": map[string]any{"type": "string", "enum": []string{"auto", "local", "agent"}, "default": "auto", "description": "本地或扫描节点执行。"},
				"agentId":  map[string]any{"type": "string", "description": "可选，agent 模式下的扫描节点 ID。"},
				"proxy":    map[string]any{"type": "string", "description": "可选，采集和检测使用的代理地址。"},
				"config": pentestNativeObjectSchema(map[string]any{
					"vulnDetection": pentestNativeObjectSchema(map[string]any{
						"enabled": map[string]any{"type": "boolean", "default": false, "description": "是否启用平台漏洞检测。"},
					}),
					"openai": pentestNativeObjectSchema(map[string]any{
						"enabled": map[string]any{"type": "boolean", "description": "是否启用扫描器的 AI 研判。"},
					}),
				}),
				"forceRescan":  map[string]any{"type": "boolean", "default": false, "description": "是否跳过同会话已有任务复用并创建新任务。"},
				"rescanReason": map[string]any{"type": "string", "description": "forceRescan=true 时必填，说明重新采集的原因。"},
			}, "targets"),
		},
	}
}

func jsCollectionSignature(args pentestNativeJsscanArgs) (string, error) {
	targets := normalizePentestNonEmptyStrings(args.Targets)
	if len(targets) == 0 {
		return "", fmt.Errorf("targets 不能为空")
	}
	for _, target := range targets {
		parsed, err := url.Parse(target)
		if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
			return "", fmt.Errorf("JS/API 采集目标必须是无内嵌凭据的完整 HTTP/HTTPS URL")
		}
	}
	sort.Strings(targets) // Preserve path, query and fragment: SPA routes can differ.
	mode, err := scantask.NormalizeMode(args.ScanMode)
	if err != nil {
		return "", err
	}
	agent := strings.TrimSpace(args.AgentID)
	if mode == "local" {
		agent = ""
	}
	config := copyPentestStringInterfaceMap(args.Config)
	delete(config, "aiWorkerExecutionId")
	delete(config, "aiWorkerConversationId")
	delete(config, "targets")
	delete(config, "agentId")
	delete(config, jsCollectionScopeKey)
	if proxy := strings.TrimSpace(args.Proxy); proxy != "" {
		config["proxy"] = proxy
	}
	if config["proxy"] == "" {
		delete(config, "proxy")
	}
	// Omitted vulnerability detection and explicit false are the same collection mode.
	if detection, ok := config["vulnDetection"].(map[string]interface{}); ok && len(detection) == 1 && detection["enabled"] == false {
		delete(config, "vulnDetection")
	}
	raw, err := json.Marshal([]any{targets, mode, agent, config})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func jsCollectionHistoryIDs(results []string) []string {
	seen := map[string]bool{}
	ids := []string{}
	for _, raw := range results {
		var result struct {
			OK     bool `json:"ok"`
			Result struct {
				TaskID string `json:"taskId"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(raw), &result) == nil && result.OK && result.Result.TaskID != "" && !seen[result.Result.TaskID] {
			seen[result.Result.TaskID] = true
			ids = append(ids, result.Result.TaskID)
		}
	}
	return ids
}

func createOrReusePentestJSScan(ctx context.Context, cfg pentest.AgentLoopConfig, args pentestNativeJsscanArgs) (gin.H, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, true, err
	}
	if err := normalizePentestJSCollectionConfig(&args); err != nil {
		return nil, true, err
	}
	if _, err := jsCollectionSignature(args); err != nil {
		return nil, true, err
	}
	db, err := database.GetDB()
	if err != nil {
		return nil, true, err
	}
	var historyIDs []string
	if cfg.Store != nil && cfg.Conversation.ID != "" {
		results, err := cfg.Store.CompletedToolResults(ctx, cfg.Conversation.ID, "attack_surface_create_jsscan_task")
		if err != nil {
			return nil, true, err
		}
		historyIDs = jsCollectionHistoryIDs(results)
	}
	return createOrReuseJSScanWithDB(ctx, db, cfg.Conversation.ID, historyIDs, args, scantask.CreateAIScanTaskWithDB)
}

func normalizePentestJSCollectionConfig(args *pentestNativeJsscanArgs) error {
	if args == nil {
		return nil
	}
	// The AI tool contract is collection-first. The scanner's product default is
	// vulnerability-enabled, so omission must be made explicit before creating
	// a task; callers that intentionally need testing must opt in with true.
	args.Config = copyPentestStringInterfaceMap(args.Config)
	if args.Config["vulnDetection"] == nil {
		args.Config["vulnDetection"] = map[string]interface{}{"enabled": false}
	} else if detection, ok := args.Config["vulnDetection"].(map[string]interface{}); ok {
		detection = copyPentestStringInterfaceMap(detection)
		if detection["enabled"] == nil {
			detection["enabled"] = false
		} else if enabled, ok := detection["enabled"].(string); ok {
			switch strings.ToLower(strings.TrimSpace(enabled)) {
			case "true":
				detection["enabled"] = true
			case "false":
				detection["enabled"] = false
			default:
				return fmt.Errorf("config.vulnDetection.enabled 必须是布尔值")
			}
		}
		args.Config["vulnDetection"] = detection
	}
	// Reject malformed AI-provided config before it becomes an asynchronous task.
	raw, err := json.Marshal(args.Config)
	if err != nil {
		return fmt.Errorf("JSScan 配置无效: %w", err)
	}
	if err := json.Unmarshal(raw, &jsscan.ScanConfig{}); err != nil {
		return fmt.Errorf("JSScan 配置无效: %w", err)
	}
	return nil
}

type jsCollectionCreator func(*gorm.DB, string, string, string, string, []string, map[string]interface{}) (string, string, string, string, error)

func createOrReuseJSScanWithDB(ctx context.Context, db *gorm.DB, conversationID string, historyIDs []string, args pentestNativeJsscanArgs, create jsCollectionCreator) (gin.H, bool, error) {
	if db != nil && db.Dialector.Name() != "postgres" && args.ScanMode == "" {
		args.ScanMode = "local"
	}

	if err := normalizePentestJSCollectionConfig(&args); err != nil {
		return nil, true, err
	}
	signature, err := jsCollectionSignature(args)
	if err != nil {
		return nil, true, err
	}
	if args.ForceRescan && strings.TrimSpace(args.RescanReason) == "" {
		return nil, true, fmt.Errorf("forceRescan 必须提供 rescanReason：说明资源变化、采集失败、缺失文件或采集条件变化；仅缺少加密分析结论应读取已有 JS")
	}
	// Serialize lookup + creation in this server. Persisted scope also survives restarts.
	jsCollectionCreateMu.Lock()
	defer jsCollectionCreateMu.Unlock()
	scope := ""
	if conversationID != "" {
		scope = fmt.Sprintf("%x", sha256.Sum256([]byte(conversationID)))
	}
	if !args.ForceRescan && (scope != "" || len(historyIDs) > 0) {
		query := db.WithContext(ctx).Model(&database.VulnScanTask{}).Where("scan_type = ?", "jsscan")
		if scope != "" && len(historyIDs) > 0 {
			query = query.Where("config LIKE ? OR id IN ?", "%"+scope+"%", historyIDs)
		} else if scope != "" {
			query = query.Where("config LIKE ?", "%"+scope+"%")
		} else {
			query = query.Where("id IN ?", historyIDs)
		}
		var tasks []database.VulnScanTask
		if err := query.Order("created_at DESC").Order("version DESC").Find(&tasks).Error; err != nil {
			return nil, true, err
		}
		for _, task := range tasks {
			var latest int
			if err := db.WithContext(ctx).Model(&database.VulnScanTask{}).Select("MAX(version)").Where("id = ?", task.ID).Scan(&latest).Error; err != nil {
				return nil, true, err
			}
			if task.Version != latest {
				continue
			}
			var targets []string
			var config map[string]interface{}
			if json.Unmarshal([]byte(task.Targets), &targets) != nil || json.Unmarshal([]byte(task.Config), &config) != nil {
				continue
			}
			owned := scope != "" && config[jsCollectionScopeKey] == scope
			for _, id := range historyIDs {
				if id == task.ID {
					owned = true
					break
				}
			}
			if !owned {
				continue
			}
			old, err := jsCollectionSignature(pentestNativeJsscanArgs{Targets: targets, Config: config, ScanMode: task.ScanMode, AgentID: task.AssignedAgentID})
			if err != nil || old != signature {
				continue
			}
			switch task.Status {
			case "pending", "running", "recovering", "paused", "completed":
				return jsCollectionResult(task, true), true, nil
			}
			// A newer failed/cancelled acquisition supersedes older snapshots
			// with the same conditions; do not silently reuse stale data.
			break
		}
	}
	config := copyPentestStringInterfaceMap(args.Config)
	delete(config, jsCollectionScopeKey)
	if scope != "" {
		config[jsCollectionScopeKey] = scope
	}
	if proxy := strings.TrimSpace(args.Proxy); proxy != "" {
		config["proxy"] = proxy
	}
	name := strings.TrimSpace(args.TaskName)
	if name == "" {
		name = "AI 智脑 JS/API 采集"
	}
	id, scanType, mode, agent, err := create(db.WithContext(ctx), "jsscan", name, args.ScanMode, args.AgentID, normalizePentestNonEmptyStrings(args.Targets), config)
	if err != nil {
		return nil, true, err
	}
	result := jsCollectionResult(database.VulnScanTask{ID: id, Version: 1, ScanType: scanType, ScanMode: mode, AssignedAgentID: agent, Status: "pending"}, false)
	result["status"] = "created"
	return result, true, nil
}

func jsCollectionResult(task database.VulnScanTask, reused bool) gin.H {
	return gin.H{"taskId": task.ID, "version": task.Version, "scanType": task.ScanType, "scanMode": task.ScanMode, "agentId": task.AssignedAgentID, "status": task.Status, "reused": reused, "nextAction": "可使用 taskId/version 调用 attack_surface_get_jsscan_overview 查询进度与统计；按需用 attack_surface_search_jsscan_operations 分页检索接口、attack_surface_search_jsscan_resources 检索 JS、attack_surface_get_jsscan_evidence 读取原始证据，参数结构需要补充时可读取 attack_surface_export_jsscan_openapi。created/pending/running 不代表扫描完成；paused 表示任务已暂停。"}
}
