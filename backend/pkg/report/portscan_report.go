package report

import (
	"fmt"
	"hephaestus-backend/pkg/database"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

func loadPortScanResults(taskID string, version int) ([]database.PortScanResult, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	var results []database.PortScanResult
	if err := db.Where("task_id = ? AND version = ?", taskID, version).Find(&results).Error; err != nil {
		return nil, fmt.Errorf("查询端口结果失败: %w", err)
	}
	return results, nil
}

// GeneratePortScanWordReportFromDB 导出端口扫描报告（DOCX）
func GeneratePortScanWordReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	ports, err := loadPortScanResults(taskID, version)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}

	// 稳定排序: host, port
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Host == ports[j].Host {
			return ports[i].Port < ports[j].Port
		}
		return ports[i].Host < ports[j].Host
	})

	builder := newDocxReportBuilder()
	builder.addTitle("端口扫描报告")
	builder.addSubtitle(fmt.Sprintf("任务: %s", taskName))
	builder.addSubtitle(fmt.Sprintf("生成时间: %s", time.Now().Format("2006-01-02 15:04:05")))
	builder.addHeading1("任务概览")
	builder.addKeyValueTable([][2]string{
		{"任务ID", task.ID},
		{"任务名称", taskName},
		{"版本", fmt.Sprintf("v%d", version)},
		{"状态", task.Status},
		{"开放端口总数", fmt.Sprintf("%d", len(ports))},
	})

	builder.addHeading1("端口结果")
	if len(ports) == 0 {
		builder.addParagraph("暂无端口结果")
		return builder.build(taskName + " - 端口扫描报告")
	}

	const maxItems = 1200
	limit := len(ports)
	if limit > maxItems {
		limit = maxItems
	}
	rows := make([][]string, 0, limit)
	for i := 0; i < limit; i++ {
		p := ports[i]
		rows = append(rows, []string{
			p.Host,
			fmt.Sprintf("%d", p.Port),
			p.Protocol,
			p.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	builder.addTable([]string{"Host", "Port", "Protocol", "CreatedAt"}, rows)
	if len(ports) > maxItems {
		builder.addParagraph(fmt.Sprintf("端口结果共 %d 条，仅展示前 %d 条。", len(ports), maxItems))
	}
	return builder.build(taskName + " - 端口扫描报告")
}

// GeneratePortScanHTMLReportFromDB 导出端口扫描报告（HTML）
func GeneratePortScanHTMLReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	ports, err := loadPortScanResults(taskID, version)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}

	var sb strings.Builder
	sb.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"/>")
	sb.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"/>")
	sb.WriteString("<title>Port Scan Report</title>")
	sb.WriteString("<style>body{font-family:ui-sans-serif,system-ui,-apple-system,Segoe UI,Roboto,Arial;line-height:1.6;padding:24px;background:#f7f9fc;color:#111}table{border-collapse:collapse;width:100%;background:#fff}th,td{border:1px solid #e5e7eb;padding:8px}th{background:#f3f4f6;text-align:left}h1{margin:0 0 8px} .muted{color:#6b7280}</style></head><body>")
	sb.WriteString(fmt.Sprintf("<h1>端口扫描报告</h1><div class=\"muted\">任务: %s | 版本: v%d | 状态: %s | 生成时间: %s</div>",
		htmlEscape(taskName), version, htmlEscape(task.Status), time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("<p>开放端口总数: <b>%d</b></p>", len(ports)))
	sb.WriteString("<table><thead><tr><th>Host</th><th>Port</th><th>Protocol</th><th>CreatedAt</th></tr></thead><tbody>")

	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Host == ports[j].Host {
			return ports[i].Port < ports[j].Port
		}
		return ports[i].Host < ports[j].Host
	})

	for _, p := range ports {
		sb.WriteString("<tr>")
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(p.Host)))
		sb.WriteString(fmt.Sprintf("<td>%d</td>", p.Port))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(p.Protocol)))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", p.CreatedAt.Format("2006-01-02 15:04:05")))
		sb.WriteString("</tr>")
	}
	sb.WriteString("</tbody></table></body></html>")
	return []byte(sb.String()), nil
}

// GeneratePortScanXLSXReportFromDB 导出端口扫描报告（XLSX）
func GeneratePortScanXLSXReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	ports, err := loadPortScanResults(taskID, version)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	summarySheet := "任务信息"
	f.SetSheetName("Sheet1", summarySheet)
	setSummarySheet(f, summarySheet, taskID, task, taskName, version)

	targetsSheet := "目标列表"
	f.NewSheet(targetsSheet)
	f.SetCellValue(targetsSheet, "A1", "目标")
	targets := parseTargets(task.Targets)
	for i, t := range targets {
		f.SetCellValue(targetsSheet, fmt.Sprintf("A%d", i+2), t)
	}

	resultsSheet := "端口结果"
	f.NewSheet(resultsSheet)
	f.SetCellValue(resultsSheet, "A1", "Host")
	f.SetCellValue(resultsSheet, "B1", "Port")
	f.SetCellValue(resultsSheet, "C1", "Protocol")
	f.SetCellValue(resultsSheet, "D1", "RawResponse")
	f.SetCellValue(resultsSheet, "E1", "CreatedAt")

	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Host == ports[j].Host {
			return ports[i].Port < ports[j].Port
		}
		return ports[i].Host < ports[j].Host
	})

	const cellLimit = 32000
	for i, p := range ports {
		row := i + 2
		f.SetCellValue(resultsSheet, fmt.Sprintf("A%d", row), p.Host)
		f.SetCellValue(resultsSheet, fmt.Sprintf("B%d", row), p.Port)
		f.SetCellValue(resultsSheet, fmt.Sprintf("C%d", row), p.Protocol)
		f.SetCellValue(resultsSheet, fmt.Sprintf("D%d", row), truncateForCell(p.RawResponse, cellLimit))
		f.SetCellValue(resultsSheet, fmt.Sprintf("E%d", row), p.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	if idx, err := f.GetSheetIndex(summarySheet); err == nil {
		f.SetActiveSheet(idx)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
