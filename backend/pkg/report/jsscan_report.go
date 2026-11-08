package report

import (
	"fmt"
	"hephaestus-backend/pkg/database"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

func loadJSScanData(taskID string, version int) ([]database.VulnScanJSVulnerability, []database.VulnScanJSAsset, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var vulns []database.VulnScanJSVulnerability
	if err := db.Where("task_id = ? AND version = ?", taskID, version).Find(&vulns).Error; err != nil {
		return nil, nil, fmt.Errorf("查询 JS 漏洞失败: %w", err)
	}

	var assets []database.VulnScanJSAsset
	if err := db.Where("task_id = ? AND version = ?", taskID, version).Find(&assets).Error; err != nil {
		return nil, nil, fmt.Errorf("查询 JS 资产失败: %w", err)
	}

	return vulns, assets, nil
}

func jsSeverityStats(vulns []database.VulnScanJSVulnerability) map[string]int {
	stats := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}
	for _, v := range vulns {
		switch strings.ToLower(strings.TrimSpace(v.Level)) {
		case "critical":
			stats["critical"]++
		case "high":
			stats["high"]++
		case "medium":
			stats["medium"]++
		case "low":
			stats["low"]++
		default:
			stats["info"]++
		}
	}
	return stats
}

func jsSeverityRank(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 4
	}
}

func GenerateJSScanWordReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	vulns, assets, err := loadJSScanData(taskID, version)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}

	stats := jsSeverityStats(vulns)
	builder := newDocxReportBuilder()
	builder.addTitle("JS 扫描报告")
	builder.addSubtitle(fmt.Sprintf("任务: %s", taskName))
	builder.addSubtitle(fmt.Sprintf("生成时间: %s", time.Now().Format("2006-01-02 15:04:05")))
	builder.addHeading1("任务概览")
	builder.addKeyValueTable([][2]string{
		{"任务ID", task.ID},
		{"任务名称", taskName},
		{"版本", fmt.Sprintf("v%d", version)},
		{"状态", task.Status},
	})

	builder.addHeading1("统计概览")
	builder.addTable([]string{"维度", "数量"}, [][]string{
		{"漏洞总数", fmt.Sprintf("%d", len(vulns))},
		{"严重漏洞", fmt.Sprintf("%d", stats["critical"])},
		{"高危漏洞", fmt.Sprintf("%d", stats["high"])},
		{"中危漏洞", fmt.Sprintf("%d", stats["medium"])},
		{"低危漏洞", fmt.Sprintf("%d", stats["low"])},
		{"信息漏洞", fmt.Sprintf("%d", stats["info"])},
		{"资产条目", fmt.Sprintf("%d", len(assets))},
	})

	builder.addHeading1("漏洞列表")
	if len(vulns) == 0 {
		builder.addParagraph("暂无漏洞")
	} else {
		sort.Slice(vulns, func(i, j int) bool {
			ri, rj := jsSeverityRank(vulns[i].Level), jsSeverityRank(vulns[j].Level)
			if ri == rj {
				return vulns[i].URL < vulns[j].URL
			}
			return ri < rj
		})

		const maxItems = 180
		limit := len(vulns)
		if limit > maxItems {
			limit = maxItems
		}
		summaryRows := make([][]string, 0, limit)
		for i := 0; i < limit; i++ {
			v := vulns[i]
			summaryRows = append(summaryRows, []string{
				v.Level,
				disposalStatusLabel(v.DisposalStatus),
				v.Title,
				v.Type,
				v.Method,
				v.URL,
				v.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
		builder.addTable([]string{"Level", "处置状态", "Title", "Type", "Method", "URL", "CreatedAt"}, summaryRows)
		if len(vulns) > maxItems {
			builder.addParagraph(fmt.Sprintf("漏洞共 %d 条，仅展示前 %d 条。", len(vulns), maxItems))
		}

		for i := 0; i < limit; i++ {
			v := vulns[i]
			builder.addHeading2(fmt.Sprintf("漏洞 #%d - %s", i+1, v.Title))
			builder.addKeyValueTable([][2]string{
				{"风险等级", v.Level},
				{"处置状态", disposalStatusLabel(v.DisposalStatus)},
				{"漏洞类型", v.Type},
				{"请求方法", v.Method},
				{"URL", v.URL},
				{"描述", truncateForReportText(v.Description, 2600)},
			})
			builder.addHeading2("请求包")
			builder.addCodeBlock(v.Request)
			builder.addHeading2("响应包")
			builder.addCodeBlock(v.Response)
		}
	}

	builder.addHeading1("资产信息")
	if len(assets) == 0 {
		builder.addParagraph("暂无资产信息")
	} else {
		sort.Slice(assets, func(i, j int) bool {
			if assets[i].Type == assets[j].Type {
				return assets[i].Value < assets[j].Value
			}
			return assets[i].Type < assets[j].Type
		})

		const maxItems = 300
		limit := len(assets)
		if limit > maxItems {
			limit = maxItems
		}
		rows := make([][]string, 0, limit)
		for i := 0; i < limit; i++ {
			a := assets[i]
			rows = append(rows, []string{
				a.Type,
				a.Value,
				a.Source,
				a.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
		builder.addTable([]string{"Type", "Value", "Source", "CreatedAt"}, rows)
		if len(assets) > maxItems {
			builder.addParagraph(fmt.Sprintf("资产共 %d 条，仅展示前 %d 条。", len(assets), maxItems))
		}
	}

	return builder.build(taskName + " - JS 扫描报告")
}

func GenerateJSScanHTMLReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	vulns, assets, err := loadJSScanData(taskID, version)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}

	stats := jsSeverityStats(vulns)
	sort.Slice(vulns, func(i, j int) bool {
		ri, rj := jsSeverityRank(vulns[i].Level), jsSeverityRank(vulns[j].Level)
		if ri == rj {
			return vulns[i].URL < vulns[j].URL
		}
		return ri < rj
	})
	sort.Slice(assets, func(i, j int) bool {
		if assets[i].Type == assets[j].Type {
			return assets[i].Value < assets[j].Value
		}
		return assets[i].Type < assets[j].Type
	})

	var sb strings.Builder
	sb.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"/>")
	sb.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"/>")
	sb.WriteString("<title>JS 扫描报告</title>")
	sb.WriteString("<style>")
	sb.WriteString("body{margin:0;font-family:'Segoe UI',PingFang SC,Microsoft YaHei,sans-serif;background:linear-gradient(180deg,#f3f7ff 0,#f8fafc 100%);color:#111827;line-height:1.6}")
	sb.WriteString(".container{max-width:1320px;margin:0 auto;padding:28px 18px 40px}")
	sb.WriteString(".hero{background:#0f172a;color:#e2e8f0;border-radius:16px;padding:20px 22px;box-shadow:0 14px 28px rgba(2,6,23,.16)}")
	sb.WriteString(".hero h1{margin:0;font-size:26px}.hero p{margin:8px 0 0;color:#cbd5e1;font-size:13px}")
	sb.WriteString(".stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(128px,1fr));gap:10px;margin:14px 0 18px}")
	sb.WriteString(".stat{background:#fff;border:1px solid #e2e8f0;border-radius:12px;padding:10px 12px}.stat .k{font-size:12px;color:#64748b}.stat .v{font-size:22px;font-weight:700;color:#0f172a}")
	sb.WriteString(".section{background:#fff;border:1px solid #e2e8f0;border-radius:14px;padding:14px 16px;margin-top:14px}")
	sb.WriteString(".section h2{margin:0 0 10px;font-size:18px}")
	sb.WriteString(".vuln{border:1px solid #e2e8f0;border-radius:12px;padding:10px 12px;margin-bottom:10px;background:#fff}")
	sb.WriteString(".vuln-head{display:flex;justify-content:space-between;align-items:center;gap:8px;flex-wrap:wrap}")
	sb.WriteString(".mono{font-family:ui-monospace,Consolas,Menlo,monospace;word-break:break-all}")
	sb.WriteString(".sev{display:inline-block;padding:2px 8px;border-radius:999px;border:1px solid;font-size:12px}.sev-critical{color:#7f1d1d;background:#fee2e2;border-color:#fca5a5}.sev-high{color:#9a3412;background:#ffedd5;border-color:#fdba74}.sev-medium{color:#854d0e;background:#fef3c7;border-color:#fcd34d}.sev-low{color:#1d4ed8;background:#dbeafe;border-color:#93c5fd}.sev-info{color:#334155;background:#e2e8f0;border-color:#cbd5e1}")
	sb.WriteString("details{border:1px solid #e2e8f0;border-radius:10px;background:#f8fafc;margin-top:8px}summary{cursor:pointer;padding:8px 10px;color:#334155;font-weight:600}pre{margin:0;padding:10px;border-top:1px solid #e2e8f0;background:#fff;white-space:pre-wrap;word-break:break-all;max-height:360px;overflow:auto}")
	sb.WriteString("table{width:100%;border-collapse:collapse;font-size:13px}th,td{border:1px solid #e2e8f0;padding:8px 9px;vertical-align:top}th{background:#f8fafc;text-align:left;color:#334155}")
	sb.WriteString("</style></head><body><div class=\"container\">")
	sb.WriteString(fmt.Sprintf("<div class=\"hero\"><h1>JS 扫描报告</h1><p>任务: %s | 版本: v%d | 状态: %s | 生成时间: %s</p></div>",
		htmlEscape(taskName), version, htmlEscape(task.Status), time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("<div class=\"stats\">")
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">漏洞</div><div class=\"v\">%d</div></div>", len(vulns)))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">严重</div><div class=\"v\">%d</div></div>", stats["critical"]))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">高危</div><div class=\"v\">%d</div></div>", stats["high"]))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">中危</div><div class=\"v\">%d</div></div>", stats["medium"]))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">低危</div><div class=\"v\">%d</div></div>", stats["low"]))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">信息</div><div class=\"v\">%d</div></div>", stats["info"]))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">资产</div><div class=\"v\">%d</div></div>", len(assets)))
	sb.WriteString("</div>")

	sb.WriteString("<section class=\"section\"><h2>漏洞列表</h2>")
	for _, v := range vulns {
		sev := strings.ToLower(strings.TrimSpace(v.Level))
		if sev == "" {
			sev = "info"
		}
		sb.WriteString("<article class=\"vuln\">")
		sb.WriteString("<div class=\"vuln-head\">")
		sb.WriteString(fmt.Sprintf("<div><span class=\"sev sev-%s\">%s</span> <b>%s</b></div>", htmlEscape(sev), htmlEscape(v.Level), htmlEscape(v.Title)))
		sb.WriteString(fmt.Sprintf("<div class=\"mono\">%s %s</div>", htmlEscape(v.Method), htmlEscape(v.URL)))
		sb.WriteString("</div>")
		sb.WriteString(fmt.Sprintf("<div><b>处置状态：</b>%s</div>", htmlEscape(disposalStatusLabel(v.DisposalStatus))))
		sb.WriteString(fmt.Sprintf("<div><b>类型：</b>%s</div>", htmlEscape(v.Type)))
		sb.WriteString(fmt.Sprintf("<div><b>描述：</b>%s</div>", htmlEscape(truncateForReportText(v.Description, 2000))))
		sb.WriteString("<details><summary>请求包</summary><pre>")
		sb.WriteString(htmlEscape(truncateForReportText(v.Request, 10000)))
		sb.WriteString("</pre></details>")
		sb.WriteString("<details><summary>响应包</summary><pre>")
		sb.WriteString(htmlEscape(truncateForReportText(v.Response, 10000)))
		sb.WriteString("</pre></details>")
		sb.WriteString("</article>")
	}
	if len(vulns) == 0 {
		sb.WriteString("<div>暂无漏洞信息</div>")
	}
	sb.WriteString("</section>")

	sb.WriteString("<section class=\"section\"><h2>资产信息</h2>")
	sb.WriteString("<table><thead><tr><th>Type</th><th>Value</th><th>Source</th><th>CreatedAt</th></tr></thead><tbody>")
	for _, a := range assets {
		sb.WriteString("<tr>")
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(a.Type)))
		sb.WriteString(fmt.Sprintf("<td class=\"mono\">%s</td>", htmlEscape(a.Value)))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(a.Source)))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", a.CreatedAt.Format("2006-01-02 15:04:05")))
		sb.WriteString("</tr>")
	}
	if len(assets) == 0 {
		sb.WriteString("<tr><td colspan=\"4\">暂无资产信息</td></tr>")
	}
	sb.WriteString("</tbody></table></section>")
	sb.WriteString("</div></body></html>")
	return []byte(sb.String()), nil
}

func GenerateJSScanXLSXReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	vulns, assets, err := loadJSScanData(taskID, version)
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

	vulnSheet := "JS漏洞"
	f.NewSheet(vulnSheet)
	f.SetCellValue(vulnSheet, "A1", "Level")
	f.SetCellValue(vulnSheet, "B1", "Title")
	f.SetCellValue(vulnSheet, "C1", "Type")
	f.SetCellValue(vulnSheet, "D1", "URL")
	f.SetCellValue(vulnSheet, "E1", "Method")
	f.SetCellValue(vulnSheet, "F1", "AI Verified")
	f.SetCellValue(vulnSheet, "G1", "Description")
	f.SetCellValue(vulnSheet, "H1", "Request(截断)")
	f.SetCellValue(vulnSheet, "I1", "Response(截断)")
	f.SetCellValue(vulnSheet, "J1", "CreatedAt")
	f.SetCellValue(vulnSheet, "K1", "处置状态")

	sort.Slice(vulns, func(i, j int) bool {
		ri, rj := jsSeverityRank(vulns[i].Level), jsSeverityRank(vulns[j].Level)
		if ri == rj {
			return vulns[i].URL < vulns[j].URL
		}
		return ri < rj
	})

	const cellLimit = 32000
	for i, v := range vulns {
		row := i + 2
		f.SetCellValue(vulnSheet, fmt.Sprintf("A%d", row), v.Level)
		f.SetCellValue(vulnSheet, fmt.Sprintf("B%d", row), truncateForCell(v.Title, cellLimit))
		f.SetCellValue(vulnSheet, fmt.Sprintf("C%d", row), v.Type)
		f.SetCellValue(vulnSheet, fmt.Sprintf("D%d", row), truncateForCell(v.URL, cellLimit))
		f.SetCellValue(vulnSheet, fmt.Sprintf("E%d", row), v.Method)
		if v.AIVerified {
			f.SetCellValue(vulnSheet, fmt.Sprintf("F%d", row), "是")
		} else {
			f.SetCellValue(vulnSheet, fmt.Sprintf("F%d", row), "否")
		}
		f.SetCellValue(vulnSheet, fmt.Sprintf("G%d", row), truncateForCell(v.Description, cellLimit))
		f.SetCellValue(vulnSheet, fmt.Sprintf("H%d", row), truncateForCell(v.Request, cellLimit))
		f.SetCellValue(vulnSheet, fmt.Sprintf("I%d", row), truncateForCell(v.Response, cellLimit))
		f.SetCellValue(vulnSheet, fmt.Sprintf("J%d", row), v.CreatedAt.Format("2006-01-02 15:04:05"))
		f.SetCellValue(vulnSheet, fmt.Sprintf("K%d", row), disposalStatusLabel(v.DisposalStatus))
	}

	assetSheet := "JS资产"
	f.NewSheet(assetSheet)
	f.SetCellValue(assetSheet, "A1", "Type")
	f.SetCellValue(assetSheet, "B1", "Value")
	f.SetCellValue(assetSheet, "C1", "Source")
	f.SetCellValue(assetSheet, "D1", "CreatedAt")

	sort.Slice(assets, func(i, j int) bool {
		if assets[i].Type == assets[j].Type {
			return assets[i].Value < assets[j].Value
		}
		return assets[i].Type < assets[j].Type
	})

	for i, a := range assets {
		row := i + 2
		f.SetCellValue(assetSheet, fmt.Sprintf("A%d", row), a.Type)
		f.SetCellValue(assetSheet, fmt.Sprintf("B%d", row), truncateForCell(a.Value, cellLimit))
		f.SetCellValue(assetSheet, fmt.Sprintf("C%d", row), truncateForCell(a.Source, cellLimit))
		f.SetCellValue(assetSheet, fmt.Sprintf("D%d", row), a.CreatedAt.Format("2006-01-02 15:04:05"))
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
