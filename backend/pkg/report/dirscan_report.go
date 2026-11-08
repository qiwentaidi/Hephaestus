package report

import (
	"fmt"
	"hephaestus-backend/pkg/database"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

func loadDirScanResults(taskID string, version int) ([]database.VulnScanDirResult, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	var results []database.VulnScanDirResult
	if err := db.Where("task_id = ? AND version = ?", taskID, version).Find(&results).Error; err != nil {
		return nil, fmt.Errorf("查询目录扫描结果失败: %w", err)
	}
	return results, nil
}

func GenerateDirScanWordReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	results, err := loadDirScanResults(taskID, version)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Status == results[j].Status {
			return results[i].URL < results[j].URL
		}
		return results[i].Status < results[j].Status
	})

	builder := newDocxReportBuilder()
	builder.addTitle("目录扫描报告")
	builder.addSubtitle(fmt.Sprintf("任务: %s", taskName))
	builder.addSubtitle(fmt.Sprintf("生成时间: %s", time.Now().Format("2006-01-02 15:04:05")))
	builder.addHeading1("任务概览")
	builder.addKeyValueTable([][2]string{
		{"任务ID", task.ID},
		{"任务名称", taskName},
		{"版本", fmt.Sprintf("v%d", version)},
		{"状态", task.Status},
		{"发现路径总数", fmt.Sprintf("%d", len(results))},
	})

	builder.addHeading1("目录结果")
	if len(results) == 0 {
		builder.addParagraph("暂无目录扫描结果")
		return builder.build(taskName + " - 目录扫描报告")
	}

	const maxItems = 1200
	limit := len(results)
	if limit > maxItems {
		limit = maxItems
	}
	rows := make([][]string, 0, limit)
	for i := 0; i < limit; i++ {
		r := results[i]
		rows = append(rows, []string{
			fmt.Sprintf("%d", r.Status),
			r.URL,
			r.Title,
			fmt.Sprintf("%d", r.Length),
			r.ContentType,
			r.Location,
			r.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	builder.addTable([]string{"Status", "URL", "Title", "Length", "Content-Type", "Location", "CreatedAt"}, rows)
	if len(results) > maxItems {
		builder.addParagraph(fmt.Sprintf("目录结果共 %d 条，仅展示前 %d 条。", len(results), maxItems))
	}
	return builder.build(taskName + " - 目录扫描报告")
}

func GenerateDirScanHTMLReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	results, err := loadDirScanResults(taskID, version)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}

	var sb strings.Builder
	sb.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"/>")
	sb.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"/>")
	sb.WriteString("<title>Dir Scan Report</title>")
	sb.WriteString("<style>body{font-family:ui-sans-serif,system-ui,-apple-system,Segoe UI,Roboto,Arial;line-height:1.6;padding:24px;background:#f7f9fc;color:#111}table{border-collapse:collapse;width:100%;background:#fff}th,td{border:1px solid #e5e7eb;padding:8px}th{background:#f3f4f6;text-align:left}h1{margin:0 0 8px} .muted{color:#6b7280}</style></head><body>")
	sb.WriteString(fmt.Sprintf("<h1>目录扫描报告</h1><div class=\"muted\">任务: %s | 版本: v%d | 状态: %s | 生成时间: %s</div>",
		htmlEscape(taskName), version, htmlEscape(task.Status), time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("<p>发现路径总数: <b>%d</b></p>", len(results)))
	sb.WriteString("<table><thead><tr><th>Status</th><th>URL</th><th>Title</th><th>Length</th><th>Content-Type</th><th>Location</th><th>CreatedAt</th></tr></thead><tbody>")

	sort.Slice(results, func(i, j int) bool {
		if results[i].Status == results[j].Status {
			return results[i].URL < results[j].URL
		}
		return results[i].Status < results[j].Status
	})

	for _, r := range results {
		sb.WriteString("<tr>")
		sb.WriteString(fmt.Sprintf("<td>%d</td>", r.Status))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(r.URL)))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(r.Title)))
		sb.WriteString(fmt.Sprintf("<td>%d</td>", r.Length))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(r.ContentType)))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(r.Location)))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", r.CreatedAt.Format("2006-01-02 15:04:05")))
		sb.WriteString("</tr>")
	}
	sb.WriteString("</tbody></table></body></html>")
	return []byte(sb.String()), nil
}

func GenerateDirScanXLSXReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	results, err := loadDirScanResults(taskID, version)
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

	resultsSheet := "目录结果"
	f.NewSheet(resultsSheet)
	f.SetCellValue(resultsSheet, "A1", "URL")
	f.SetCellValue(resultsSheet, "B1", "Status")
	f.SetCellValue(resultsSheet, "C1", "Title")
	f.SetCellValue(resultsSheet, "D1", "Location")
	f.SetCellValue(resultsSheet, "E1", "ContentType")
	f.SetCellValue(resultsSheet, "F1", "Length")
	f.SetCellValue(resultsSheet, "G1", "Recursion")
	f.SetCellValue(resultsSheet, "H1", "CreatedAt")

	sort.Slice(results, func(i, j int) bool {
		if results[i].Status == results[j].Status {
			return results[i].URL < results[j].URL
		}
		return results[i].Status < results[j].Status
	})

	const cellLimit = 32000
	for i, r := range results {
		row := i + 2
		f.SetCellValue(resultsSheet, fmt.Sprintf("A%d", row), truncateForCell(r.URL, cellLimit))
		f.SetCellValue(resultsSheet, fmt.Sprintf("B%d", row), r.Status)
		f.SetCellValue(resultsSheet, fmt.Sprintf("C%d", row), truncateForCell(r.Title, cellLimit))
		f.SetCellValue(resultsSheet, fmt.Sprintf("D%d", row), truncateForCell(r.Location, cellLimit))
		f.SetCellValue(resultsSheet, fmt.Sprintf("E%d", row), truncateForCell(r.ContentType, cellLimit))
		f.SetCellValue(resultsSheet, fmt.Sprintf("F%d", row), r.Length)
		f.SetCellValue(resultsSheet, fmt.Sprintf("G%d", row), r.Recursion)
		f.SetCellValue(resultsSheet, fmt.Sprintf("H%d", row), r.CreatedAt.Format("2006-01-02 15:04:05"))
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
