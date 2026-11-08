package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"gorm.io/gorm"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/database"
)

type pentestScanReadArgs struct {
	TaskID     string `json:"taskId"`
	Version    int    `json:"version"`
	ResultType string `json:"resultType"`
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
}

func pentestScanReadTool(name string) pentest.AgentToolSchema {
	description := "读取网站、端口或目录扫描状态；只读，不恢复暂停任务。返回固定 version 和结果类型。created/running 不代表完成，应等待后重新查询。"
	if name == "attack_surface_list_scan_results" {
		description = "按 taskId/version 分页读取已保存的网站指纹、漏洞证据、端口或目录结果。先查询状态；使用返回的 nextOffset 读完所需类型，status!=completed 时仅是部分结果。不会启动或恢复扫描。"
	}
	return pentest.AgentToolSchema{Type: "function", Function: pentest.AgentToolFunction{Name: name, Description: description, Parameters: pentestNativeObjectSchema(map[string]any{
		"taskId":     map[string]any{"type": "string", "description": "任务 ID 本身，不要拼接 /vN。"},
		"version":    map[string]any{"type": "integer", "minimum": 1, "description": "首次可省略取最新版本，后续固定返回的版本。"},
		"resultType": map[string]any{"type": "string", "enum": []string{"fingerprints", "vulnerabilities", "ports", "directories"}},
		"offset":     map[string]any{"type": "integer", "minimum": 0},
		"limit":      map[string]any{"type": "integer", "default": 50, "maximum": 100},
	}, "taskId")}}
}

func runPentestScanRead(ctx context.Context, call pentest.AgentToolCall) (any, bool, error) {
	var args pentestScanReadArgs
	if err := decodePentestToolArgs(call.Function.Arguments, &args); err != nil {
		return nil, true, err
	}
	db, err := database.GetDB()
	if err != nil {
		return nil, true, err
	}
	result, err := readPentestScanWithDB(ctx, db, call.Function.Name, args)
	return result, true, err
}

func readPentestScanWithDB(ctx context.Context, db *gorm.DB, name string, args pentestScanReadArgs) (map[string]any, error) {
	var err error
	args.TaskID, args.Version, err = normalizePentestTaskReference(args.TaskID, args.Version)
	if err != nil {
		return nil, err
	}
	if args.Offset < 0 {
		return nil, fmt.Errorf("offset 不能为负数")
	}
	query := db.WithContext(ctx).Where("id = ?", args.TaskID)
	if args.Version > 0 {
		query = query.Where("version = ?", args.Version)
	}
	var task database.VulnScanTask
	if err := query.Order("version DESC").First(&task).Error; err != nil {
		return nil, err
	}
	kinds := map[string][]string{"webscan": {"fingerprints", "vulnerabilities"}, "portscan": {"ports"}, "dirscan": {"directories"}}
	types, ok := kinds[task.ScanType]
	if !ok {
		return nil, fmt.Errorf("请使用 %s 对应的结果读取工具", task.ScanType)
	}
	var targets []string
	_ = json.Unmarshal([]byte(task.Targets), &targets)
	out := map[string]any{"taskId": task.ID, "version": task.Version, "scanType": task.ScanType, "targets": targets, "status": task.Status, "pauseRequested": task.PauseRequested, "progress": task.Progress, "error": task.ErrorMsg, "resultTypes": types}
	if name == "attack_surface_get_scan_task" {
		return out, nil
	}
	if args.ResultType == "" {
		args.ResultType = types[0]
	}
	valid := false
	for _, kind := range types {
		if args.ResultType == kind {
			valid = true
		}
	}
	if !valid {
		return nil, fmt.Errorf("任务类型 %s 不提供 %s", task.ScanType, args.ResultType)
	}
	if args.Limit <= 0 {
		args.Limit = 50
	}
	if args.Limit > 100 {
		args.Limit = 100
	}
	var model any
	var fields, order string
	switch args.ResultType {
	case "fingerprints":
		model = &database.FingerprintInfo{}
		fields = "url, status, title, host, port, fingerprints, high_risk_fingerprints"
		order = "url ASC, host ASC, port ASC, created_at ASC"
	case "vulnerabilities":
		model = &database.VulnerabilityInfo{}
		fields = "template_id, vuln_name, severity, vuln_url, description, request, response, impact, remediation"
		order = "vuln_url ASC, template_id ASC, vuln_name ASC"
	case "ports":
		model = &database.PortScanResult{}
		fields = "id, host, port, transport, protocol, product, service_version, raw_response"
		order = "id ASC"
	case "directories":
		model = &database.VulnScanDirResult{}
		fields = "id, url, status, title, location, content_type, length"
		order = "id ASC"
	}
	base := db.WithContext(ctx).Model(model).Where("task_id = ? AND version = ?", task.ID, task.Version)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, err
	}
	rows := []map[string]any{}
	if err := base.Select(fields).Order(order).Offset(args.Offset).Limit(args.Limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	// Bound external content, including headers/banners and stored request bodies.
	for _, row := range rows {
		for key, value := range row {
			if text, ok := value.(string); ok && len([]rune(text)) > 2048 {
				row[key] = string([]rune(text)[:2048])
				row[key+"_truncated"] = true
			}
		}
	}
	out["resultType"] = args.ResultType
	out["results"] = rows
	out["total"] = total
	out["offset"] = args.Offset
	out["limit"] = args.Limit
	out["nextOffset"] = args.Offset + len(rows)
	out["hasMore"] = int64(args.Offset+len(rows)) < total
	return out, nil
}

func pentestDNSLookupTool() pentest.AgentToolSchema {
	return pentest.AgentToolSchema{Type: "function", Function: pentest.AgentToolFunction{Name: "dns_lookup", Description: "查询指定域名的 A/AAAA 或 CNAME 记录。只核对给定域名；DNS 失败不能外推到整个域。", Parameters: pentestNativeObjectSchema(map[string]any{"host": map[string]any{"type": "string"}, "recordType": map[string]any{"type": "string", "enum": []string{"A", "AAAA", "CNAME"}, "default": "A"}}, "host")}}
}

func runPentestDNSLookup(ctx context.Context, call pentest.AgentToolCall) (any, bool, error) {
	var args struct {
		Host       string `json:"host"`
		RecordType string `json:"recordType"`
	}
	if err := decodePentestToolArgs(call.Function.Arguments, &args); err != nil {
		return nil, true, err
	}
	args.Host = strings.TrimSpace(args.Host)
	if args.Host == "" || strings.ContainsAny(args.Host, "/: \t\r\n") {
		return nil, true, fmt.Errorf("host 必须为纯域名")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var records []string
	kind := strings.ToUpper(args.RecordType)
	if kind == "" {
		kind = "A"
	}
	if kind == "CNAME" {
		name, err := net.DefaultResolver.LookupCNAME(ctx, args.Host)
		if err != nil {
			return nil, true, err
		}
		records = []string{name}
	} else {
		network := "ip4"
		if kind == "AAAA" {
			network = "ip6"
		} else if kind != "A" {
			return nil, true, fmt.Errorf("不支持的 DNS 类型")
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, network, args.Host)
		if err != nil {
			return nil, true, err
		}
		for _, ip := range ips {
			records = append(records, ip.String())
		}
	}
	return map[string]any{"host": args.Host, "recordType": kind, "records": records}, true, nil
}
