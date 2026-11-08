package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/database"
)

type pentestSubdomainReadArgs struct {
	TaskID  string `json:"taskId"`
	Version int    `json:"version"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
}

type pentestSubdomainReadResult struct {
	Domain     string   `json:"domain"`
	Subdomain  string   `json:"subdomain"`
	IPs        []string `json:"ips"`
	AssetType  string   `json:"assetType"`
	AssetName  string   `json:"assetName"`
	Source     string   `json:"source"`
	Sources    []string `json:"sources"`
	IsWildcard bool     `json:"isWildcard"`
}

func pentestSubdomainReadTool(name string) pentest.AgentToolSchema {
	description := "读取子域名任务状态。只有 status=completed 才表示发现完成；created/running 需要继续等待。任务配置与凭据不会返回。"
	if name == "subdomain_list_results" {
		description = "分页读取子域名发现结果。先通过 subdomain_get_task 确认完成，再读取全部分页；部分结果不能当成完整资产清单。"
	}
	return pentest.AgentToolSchema{Type: "function", Function: pentest.AgentToolFunction{Name: name, Description: description, Parameters: pentestNativeObjectSchema(map[string]any{
		"taskId":  map[string]any{"type": "string", "description": "任务 ID 本身，不要拼接 /vN。"},
		"version": map[string]any{"type": "integer", "minimum": 1, "description": "可选；版本号须单独传递，首次可省略取最新版本。"},
		"limit":   map[string]any{"type": "integer", "default": 100, "maximum": 500},
		"offset":  map[string]any{"type": "integer", "default": 0, "minimum": 0},
	}, "taskId")}}
}
func runPentestSubdomainRead(ctx context.Context, call pentest.AgentToolCall) (any, bool, error) {
	var args pentestSubdomainReadArgs
	if err := decodePentestToolArgs(call.Function.Arguments, &args); err != nil {
		return nil, true, err
	}
	db, err := database.GetDB()
	if err != nil {
		return nil, true, err
	}
	result, err := readPentestSubdomainWithDB(ctx, db, call.Function.Name, args)
	return result, true, err
}

func readPentestSubdomainWithDB(ctx context.Context, db *gorm.DB, name string, args pentestSubdomainReadArgs) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var err error
	args.TaskID, args.Version, err = normalizePentestTaskReference(args.TaskID, args.Version)
	if err != nil {
		return nil, err
	}
	if name != "subdomain_get_task" && name != "subdomain_list_results" {
		return nil, fmt.Errorf("不支持的子域名读取工具: %s", name)
	}

	var unifiedTask database.VulnScanTask
	query := db.WithContext(ctx).Where("id = ?", args.TaskID)
	if args.Version > 0 {
		query = query.Where("version = ?", args.Version)
	}
	if err := query.Order("version DESC").Take(&unifiedTask).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("子域名任务不存在: taskId=%s version=%d", args.TaskID, args.Version)
		}
		return nil, err
	}
	if unifiedTask.ScanType != "subdomain" {
		return nil, fmt.Errorf("任务 %s 的类型为 %s，请使用对应的结果读取工具", args.TaskID, unifiedTask.ScanType)
	}
	return readPentestUnifiedSubdomain(db.WithContext(ctx), name, args, unifiedTask)
}

func readPentestUnifiedSubdomain(db *gorm.DB, name string, args pentestSubdomainReadArgs, task database.VulnScanTask) (map[string]any, error) {
	var resultCount int64
	if err := db.Model(&database.SubdomainScanResult{}).Where("task_id = ?", task.ID).Count(&resultCount).Error; err != nil {
		return nil, err
	}
	var targets []string
	_ = json.Unmarshal([]byte(task.Targets), &targets)
	out := map[string]any{
		"taskId": task.ID, "version": task.Version, "scanType": task.ScanType, "targets": targets,
		"status": task.Status, "pauseRequested": task.PauseRequested, "progress": task.Progress,
		"totalCount": task.TotalCount, "currentCount": task.CurrentCount,
		"resultCount": resultCount, "progressDetail": task.ProgressDetail, "error": task.ErrorMsg,
	}
	if name == "subdomain_get_task" {
		return out, nil
	}
	return readPentestSubdomainResultPage(db, args, out)
}

func readPentestSubdomainResultPage(db *gorm.DB, args pentestSubdomainReadArgs, out map[string]any) (map[string]any, error) {
	if args.Limit <= 0 {
		args.Limit = 100
	}
	if args.Limit > 500 {
		args.Limit = 500
	}
	if args.Offset < 0 {
		args.Offset = 0
	}
	base := db.Model(&database.SubdomainScanResult{}).Where("task_id = ?", args.TaskID)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, err
	}
	var records []database.SubdomainScanResult
	if err := base.Order("created_at DESC, id DESC").Offset(args.Offset).Limit(args.Limit).Find(&records).Error; err != nil {
		return nil, err
	}
	results := make([]pentestSubdomainReadResult, 0, len(records))
	for _, record := range records {
		var ips []string
		if err := json.Unmarshal([]byte(record.IPs), &ips); err != nil {
			return nil, fmt.Errorf("解析子域名 %s 的 IP 列表失败: %w", record.Subdomain, err)
		}
		sources := pentestSubdomainSources(record.Sources, record.Source)
		source := record.Source
		if len(sources) > 0 {
			source = sources[0]
		}
		results = append(results, pentestSubdomainReadResult{
			Domain: record.Domain, Subdomain: record.Subdomain, IPs: ips,
			AssetType: record.AssetType, AssetName: record.AssetName, Source: source,
			Sources: sources, IsWildcard: record.IsWildcard,
		})
	}
	out["results"] = results
	out["total"] = total
	out["offset"] = args.Offset
	out["limit"] = args.Limit
	out["nextOffset"] = args.Offset + len(results)
	out["hasMore"] = int64(args.Offset+len(results)) < total
	return out, nil
}

func pentestSubdomainSources(raw string, fallback string) []string {
	var decoded []string
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &decoded)
	}
	if len(decoded) == 0 && strings.TrimSpace(fallback) != "" {
		decoded = append(decoded, fallback)
	}
	seen := make(map[string]struct{}, len(decoded))
	normalized := make([]string, 0, len(decoded))
	for _, source := range decoded {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		if _, exists := seen[source]; exists {
			continue
		}
		seen[source] = struct{}{}
		normalized = append(normalized, source)
	}
	return normalized
}
