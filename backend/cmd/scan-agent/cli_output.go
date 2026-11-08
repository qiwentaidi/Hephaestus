package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scanexec"
	"hephaestus-backend/pkg/taskstate"
)

// cliPrinter 终端 + 文件双通道输出
type cliPrinter struct {
	file *os.File
}

func newCLIPrinter(outputPath string) (*cliPrinter, func()) {
	p := &cliPrinter{}
	cleanup := func() {}
	if outputPath = strings.TrimSpace(outputPath); outputPath != "" {
		f, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			fmt.Printf("[!] 无法创建输出文件 %s: %v（结果仅输出到终端）\n", outputPath, err)
			return p, cleanup
		}
		p.file = f
		cleanup = func() { _ = f.Close() }
		fmt.Printf("[*] 结果将保存至: %s\n", outputPath)
	}
	return p, cleanup
}

func (p *cliPrinter) printf(format string, a ...interface{}) {
	fmt.Printf(format+"\n", a...)
	if p.file != nil {
		_, _ = fmt.Fprintf(p.file, format+"\n", a...)
	}
}

func (p *cliPrinter) section(title string) {
	line := strings.Repeat("=", 62)
	p.printf("%s", line)
	p.printf("  %s", title)
	p.printf("%s", line)
}

func (p *cliPrinter) empty(tip string) {
	p.printf("  （%s）", tip)
}

// printPortResults 输出端口扫描结果
func (p *cliPrinter) printPortResults(taskID string) {
	db, err := database.GetDB()
	if err != nil {
		return
	}
	var results []database.PortScanResult
	if err := db.Where("task_id = ? AND version = ?", taskID, 1).Order("host, port").Find(&results).Error; err != nil {
		return
	}
	p.section(fmt.Sprintf("端口扫描结果（%d 个开放端口）", len(results)))
	if len(results) == 0 {
		p.empty("未发现开放端口")
		return
	}
	for _, row := range results {
		service := strings.TrimSpace(row.Protocol)
		if service == "" {
			service = "unknown"
		}
		product := strings.TrimSpace(row.Product)
		if product == "" {
			product = strings.TrimSpace(row.ServiceVersion)
		}
		if product != "" {
			p.printf("[+] %s:%-6d %-4s %-10s %s", row.Host, row.Port, row.Transport, service, product)
		} else {
			p.printf("[+] %s:%-6d %-4s %s", row.Host, row.Port, row.Transport, service)
		}
	}
}

// printVulnResults 输出漏洞/弱口令结果（vulnerability_info 表）
func (p *cliPrinter) printVulnResults(taskID string, title string) {
	db, err := database.GetDB()
	if err != nil {
		return
	}
	var results []database.VulnerabilityInfo
	// vulnerability_info 表没有 created_at 列，按插入顺序（rowid）输出
	if err := db.Where("task_id = ? AND version = ?", taskID, 1).Find(&results).Error; err != nil {
		return
	}
	p.section(fmt.Sprintf("%s（%d 条）", title, len(results)))
	if len(results) == 0 {
		p.empty("未发现漏洞")
		return
	}
	for _, row := range results {
		p.printf("[!] [%s] %s", strings.ToUpper(severityLabel(row.Severity)), row.VulnName)
		if strings.TrimSpace(row.VulnURL) != "" {
			p.printf("    URL: %s", row.VulnURL)
		}
		if extract := strings.TrimSpace(row.Extract); extract != "" {
			extract = strings.ReplaceAll(extract, "\n", " ")
			if len(extract) > 200 {
				extract = extract[:200] + "..."
			}
			p.printf("    证据: %s", extract)
		}
	}
}

func severityLabel(severity string) string {
	if severity = strings.TrimSpace(severity); severity != "" {
		return severity
	}
	return "info"
}

// printFingerprintResults 输出指纹识别结果
func (p *cliPrinter) printFingerprintResults(taskID string) {
	db, err := database.GetDB()
	if err != nil {
		return
	}
	var results []database.FingerprintInfo
	if err := db.Where("task_id = ? AND version = ?", taskID, 1).Order("created_at").Find(&results).Error; err != nil {
		return
	}
	p.section(fmt.Sprintf("指纹识别结果（%d 条）", len(results)))
	if len(results) == 0 {
		p.empty("未识别到指纹")
		return
	}
	for _, row := range results {
		fingers := decodeJSONStringArray(row.Fingerprints)
		title := strings.TrimSpace(row.Title)
		line := fmt.Sprintf("[~] %s", row.URL)
		if title != "" {
			line += fmt.Sprintf("  [%s]", title)
		}
		if len(fingers) > 0 {
			line += fmt.Sprintf("  %s", strings.Join(fingers, ", "))
		}
		p.printf("%s", line)
	}
}

// printDirResults 输出目录扫描结果
func (p *cliPrinter) printDirResults(taskID string) {
	db, err := database.GetDB()
	if err != nil {
		return
	}
	var results []database.VulnScanDirResult
	if err := db.Where("task_id = ? AND version = ?", taskID, 1).Order("url").Find(&results).Error; err != nil {
		return
	}
	p.section(fmt.Sprintf("目录扫描结果（%d 条）", len(results)))
	if len(results) == 0 {
		p.empty("未发现有效路径")
		return
	}
	for _, row := range results {
		title := strings.TrimSpace(row.Title)
		p.printf("[%d] %s  len=%d  %s", row.Status, row.URL, row.Length, title)
	}
}

// printSubdomainResults 输出子域名结果
func (p *cliPrinter) printSubdomainResults(taskID string) {
	db, err := database.GetDB()
	if err != nil {
		return
	}
	var results []database.SubdomainScanResult
	if err := db.Where("task_id = ?", taskID).Order("subdomain").Find(&results).Error; err != nil {
		return
	}
	p.section(fmt.Sprintf("子域名收集结果（%d 条）", len(results)))
	if len(results) == 0 {
		p.empty("未发现子域名")
		return
	}
	for _, row := range results {
		ips := decodeJSONStringArray(row.IPs)
		if len(ips) > 0 {
			p.printf("[+] %-40s %s", row.Subdomain, strings.Join(ips, ", "))
		} else {
			p.printf("[+] %s", row.Subdomain)
		}
	}
}

// printJSResults 输出 JS 扫描结果（漏洞 + 敏感信息）
func (p *cliPrinter) printJSResults(taskID string) {
	db, err := database.GetDB()
	if err != nil {
		return
	}
	var vulns []database.VulnScanJSVulnerability
	if err := db.Where("task_id = ? AND version = ?", taskID, 1).Order("created_at").Find(&vulns).Error; err != nil {
		return
	}
	p.section(fmt.Sprintf("JS 漏洞检测结果（%d 条）", len(vulns)))
	if len(vulns) == 0 {
		p.empty("未发现 JS 漏洞")
	} else {
		for _, row := range vulns {
			p.printf("[!] [%s] %s", strings.ToUpper(row.Level), row.Title)
			if strings.TrimSpace(row.URL) != "" {
				p.printf("    URL: %s", row.URL)
			}
		}
	}

	var assets []database.VulnScanJSAsset
	if err := db.Where("task_id = ? AND version = ?", taskID, 1).Order("created_at").Find(&assets).Error; err != nil {
		return
	}
	p.section(fmt.Sprintf("JS 敏感信息（%d 条）", len(assets)))
	if len(assets) == 0 {
		p.empty("未发现敏感信息")
		return
	}
	for _, row := range assets {
		p.printf("[+] %-12s %s", row.Type, row.Value)
	}
}

func decodeJSONStringArray(raw string) []string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(text), &items); err == nil {
		return items
	}
	return nil
}

// createCLITask 创建一条 CLI 扫描任务行（执行器按 taskID 读取任务）
func createCLITask(scanType, taskName string, targets []string, cfg map[string]interface{}) (string, error) {
	db, err := database.GetDB()
	if err != nil {
		return "", err
	}
	taskID := fmt.Sprintf("cli-%s-%d", scanType, time.Now().UnixNano())
	targetsJSON, _ := json.Marshal(targets)
	cfgJSON, _ := json.Marshal(cfg)
	now := time.Now()
	task := database.VulnScanTask{
		ID:         taskID,
		Version:    1,
		TaskName:   taskName,
		ScanMode:   "local",
		ScanType:   scanType,
		Targets:    string(targetsJSON),
		Status:     taskstate.StatusPending,
		Config:     string(cfgJSON),
		TotalCount: len(targets),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := db.Create(&task).Error; err != nil {
		return "", fmt.Errorf("创建任务失败: %w", err)
	}
	return taskID, nil
}

// runCLITask 执行扫描任务并维护状态机
func runCLITask(ctx context.Context, scanType, taskID string, targets []string, cfg map[string]interface{}) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	handler, ok := scanexec.GetHandler(scanType)
	if !ok {
		return fmt.Errorf("不支持的扫描类型: %s", scanType)
	}
	_ = taskstate.MarkRunning(db, taskID, 1, "cli:"+scanType)

	scanErr := handler(ctx, taskID, targets, cfg)

	switch {
	case scanErr == nil:
		_ = taskstate.MarkCompleted(db, taskID, 1)
		if _, syncErr := assetstore.SyncCompletedTask(ctx, db, taskID, 1); syncErr != nil {
			fmt.Printf("[!] 统一资产目录同步失败: %v\n", syncErr)
		}
	case errors.Is(scanErr, context.Canceled):
		_ = taskstate.MarkCancelled(db, taskID, 1, "用户中断")
	default:
		_ = taskstate.MarkFailed(db, taskID, 1, scanErr)
	}
	return scanErr
}
