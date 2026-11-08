package report

import (
	"fmt"
	"hephaestus-backend/pkg/database"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type MergeTaskRef struct {
	ID      string
	Version int
}

type mergedTaskData struct {
	Task            database.VulnScanTask
	Fingerprints    []database.FingerprintInfo
	Vulnerabilities []database.VulnerabilityInfo
	Ports           []database.PortScanResult
	JSVulns         []database.VulnScanJSVulnerability
	JSAssets        []database.VulnScanJSAsset
	DirResults      []database.VulnScanDirResult
	Children        []database.VulnScanTask
}

func mergeChildTaskData(dst *mergedTaskData, child mergedTaskData) {
	dst.Fingerprints = append(dst.Fingerprints, child.Fingerprints...)
	dst.Vulnerabilities = append(dst.Vulnerabilities, child.Vulnerabilities...)
	dst.Ports = append(dst.Ports, child.Ports...)
	dst.JSVulns = append(dst.JSVulns, child.JSVulns...)
	dst.JSAssets = append(dst.JSAssets, child.JSAssets...)
	dst.DirResults = append(dst.DirResults, child.DirResults...)
	dst.Children = append(dst.Children, child.Children...)
}

func loadAllTaskArtifacts(taskID string, version int) mergedTaskData {
	item := mergedTaskData{}
	if _, fps, vulns, err := loadReportData(taskID, version); err == nil {
		item.Fingerprints = fps
		item.Vulnerabilities = vulns
	}
	if ports, err := loadPortScanResults(taskID, version); err == nil {
		item.Ports = ports
	}
	if vulns, assets, err := loadJSScanData(taskID, version); err == nil {
		item.JSVulns = vulns
		item.JSAssets = assets
	}
	if results, err := loadDirScanResults(taskID, version); err == nil {
		item.DirResults = results
	}
	return item
}

func loadMergedTaskItem(task database.VulnScanTask) (mergedTaskData, error) {
	item := mergedTaskData{Task: task}
	scanType := strings.ToLower(strings.TrimSpace(task.ScanType))

	switch scanType {
	case "webscan", "crack":
		_, fps, vulns, err := loadReportData(task.ID, task.Version)
		if err != nil {
			return mergedTaskData{}, err
		}
		ports, err := loadPortScanResults(task.ID, task.Version)
		if err != nil {
			ports = nil
		}
		item.Fingerprints = fps
		item.Vulnerabilities = vulns
		item.Ports = ports
	case "jsscan":
		vulns, assets, err := loadJSScanData(task.ID, task.Version)
		if err != nil {
			return mergedTaskData{}, err
		}
		item.JSVulns = vulns
		item.JSAssets = assets
	case "dirscan":
		results, err := loadDirScanResults(task.ID, task.Version)
		if err != nil {
			return mergedTaskData{}, err
		}
		item.DirResults = results
	case "portscan":
		ports, err := loadPortScanResults(task.ID, task.Version)
		if err != nil {
			return mergedTaskData{}, err
		}
		item.Ports = ports
	case "combination":
		mergeChildTaskData(&item, loadAllTaskArtifacts(task.ID, task.Version))

		combinationID := strings.TrimSpace(task.ParentCombinationID)
		if combinationID == "" {
			combinationID = task.ID
		}
		children, err := loadCombinationChildrenByCombinationID(combinationID, task.Version)
		if err != nil {
			return mergedTaskData{}, err
		}
		item.Children = append(item.Children, children...)
		for _, childTask := range children {
			childItem, err := loadMergedTaskItem(childTask)
			if err != nil {
				return mergedTaskData{}, err
			}
			mergeChildTaskData(&item, childItem)
		}
	default:
		_, fps, vulns, err := loadReportData(task.ID, task.Version)
		if err == nil {
			item.Fingerprints = fps
			item.Vulnerabilities = vulns
		}
	}

	return item, nil
}

func normalizeMergeRefs(refs []MergeTaskRef) []MergeTaskRef {
	out := make([]MergeTaskRef, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		id := strings.TrimSpace(ref.ID)
		if id == "" || ref.Version <= 0 {
			continue
		}
		key := fmt.Sprintf("%s#%d", id, ref.Version)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, MergeTaskRef{ID: id, Version: ref.Version})
	}
	return out
}

func loadMergedTaskData(refs []MergeTaskRef) ([]mergedTaskData, error) {
	normalized := normalizeMergeRefs(refs)
	if len(normalized) == 0 {
		return nil, fmt.Errorf("未提供有效任务")
	}

	items := make([]mergedTaskData, 0, len(normalized))
	for _, ref := range normalized {
		task, err := loadTaskByIDAndVersion(ref.ID, ref.Version)
		if err != nil {
			return nil, err
		}
		if !isReportExportableTaskStatus(task.Status) {
			return nil, fmt.Errorf("任务 %s v%d 尚未完成，无法导出合并报告", task.ID, task.Version)
		}

		item, err := loadMergedTaskItem(task)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, nil
}

func scanTypeLabel(scanType string) string {
	switch strings.ToLower(strings.TrimSpace(scanType)) {
	case "webscan":
		return "Web 扫描"
	case "crack":
		return "弱口令扫描"
	case "portscan":
		return "端口扫描"
	case "dirscan":
		return "目录扫描"
	case "jsscan":
		return "JS 扫描"
	case "combination":
		return "组合任务"
	default:
		return scanType
	}
}

func safeTaskName(task database.VulnScanTask) string {
	if strings.TrimSpace(task.TaskName) != "" {
		return task.TaskName
	}
	id := task.ID
	if len(id) > 8 {
		id = id[:8]
	}
	return fmt.Sprintf("任务_%s", id)
}

func mergedVulnCount(item mergedTaskData) int {
	return len(item.Vulnerabilities) + len(item.JSVulns)
}

func mergedAssetCount(item mergedTaskData) int {
	return len(item.Fingerprints) + len(item.Ports) + len(item.DirResults) + len(item.JSAssets)
}

func severityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
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

func GenerateMergedWordReportFromDB(refs []MergeTaskRef, reportName string) ([]byte, error) {
	items, err := loadMergedTaskData(refs)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(reportName) == "" {
		reportName = "攻击面评估合并报告"
	}

	totalVuln := 0
	totalAssets := 0
	for _, item := range items {
		totalVuln += mergedVulnCount(item)
		totalAssets += mergedAssetCount(item)
	}

	builder := newDocxReportBuilder()
	builder.addTitle(reportName)
	builder.addSubtitle(fmt.Sprintf("生成时间: %s", time.Now().Format("2006-01-02 15:04:05")))
	builder.addHeading1("汇总信息")
	builder.addTable([]string{"维度", "数量"}, [][]string{
		{"任务数量", fmt.Sprintf("%d", len(items))},
		{"漏洞总数", fmt.Sprintf("%d", totalVuln)},
		{"资产/结果总数", fmt.Sprintf("%d", totalAssets)},
	})

	for idx, item := range items {
		task := item.Task
		builder.addHeading1(fmt.Sprintf("任务 %d - %s", idx+1, safeTaskName(task)))
		metaRows := [][2]string{
			{"任务ID", task.ID},
			{"版本", fmt.Sprintf("v%d", task.Version)},
			{"类型", scanTypeLabel(task.ScanType)},
			{"状态", task.Status},
			{"漏洞数", fmt.Sprintf("%d", mergedVulnCount(item))},
			{"资产/结果数", fmt.Sprintf("%d", mergedAssetCount(item))},
		}
		if !task.CreatedAt.IsZero() {
			metaRows = append(metaRows, [2]string{"开始时间", task.CreatedAt.Format("2006-01-02 15:04:05")})
		}
		if task.CompletedAt != nil {
			metaRows = append(metaRows, [2]string{"完成时间", task.CompletedAt.Format("2006-01-02 15:04:05")})
		}
		builder.addKeyValueTable(metaRows)

		if len(item.Vulnerabilities) > 0 {
			builder.addHeading2("漏洞信息")
			sort.Slice(item.Vulnerabilities, func(i, j int) bool {
				ri, rj := severityRank(item.Vulnerabilities[i].Severity), severityRank(item.Vulnerabilities[j].Severity)
				if ri == rj {
					return item.Vulnerabilities[i].VulnURL < item.Vulnerabilities[j].VulnURL
				}
				return ri < rj
			})
			const maxItems = 160
			limit := len(item.Vulnerabilities)
			if limit > maxItems {
				limit = maxItems
			}
			rows := make([][]string, 0, limit)
			for i := 0; i < limit; i++ {
				v := item.Vulnerabilities[i]
				rows = append(rows, []string{v.Severity, disposalStatusLabel(v.DisposalStatus), v.VulnName, v.Protocol, v.VulnURL, v.TemplateID})
			}
			builder.addTable([]string{"Severity", "处置状态", "Name", "Protocol", "URL", "TemplateID"}, rows)
			if len(item.Vulnerabilities) > maxItems {
				builder.addParagraph(fmt.Sprintf("漏洞共 %d 条，仅展示前 %d 条。", len(item.Vulnerabilities), maxItems))
			}
			for i := 0; i < limit; i++ {
				v := item.Vulnerabilities[i]
				builder.addHeading2(fmt.Sprintf("漏洞详情 #%d - %s", i+1, v.VulnName))
				builder.addKeyValueTable([][2]string{
					{"风险等级", v.Severity},
					{"处置状态", disposalStatusLabel(v.DisposalStatus)},
					{"协议", v.Protocol},
					{"URL", v.VulnURL},
					{"模板ID", v.TemplateID},
					{"描述", truncateForReportText(v.Description, 2200)},
					{"参考", truncateForReportText(v.Reference, 1600)},
				})
				builder.addHeading2("请求包")
				builder.addCodeBlock(v.Request)
				builder.addHeading2("响应包")
				builder.addCodeBlock(v.Response)
			}
		}

		if len(item.JSVulns) > 0 {
			builder.addHeading2("JS 漏洞信息")
			sort.Slice(item.JSVulns, func(i, j int) bool {
				ri, rj := jsSeverityRank(item.JSVulns[i].Level), jsSeverityRank(item.JSVulns[j].Level)
				if ri == rj {
					return item.JSVulns[i].URL < item.JSVulns[j].URL
				}
				return ri < rj
			})
			const maxItems = 160
			limit := len(item.JSVulns)
			if limit > maxItems {
				limit = maxItems
			}
			rows := make([][]string, 0, limit)
			for i := 0; i < limit; i++ {
				v := item.JSVulns[i]
				rows = append(rows, []string{v.Level, disposalStatusLabel(v.DisposalStatus), v.Title, v.Type, v.Method, v.URL})
			}
			builder.addTable([]string{"Level", "处置状态", "Title", "Type", "Method", "URL"}, rows)
			if len(item.JSVulns) > maxItems {
				builder.addParagraph(fmt.Sprintf("JS 漏洞共 %d 条，仅展示前 %d 条。", len(item.JSVulns), maxItems))
			}
		}

		if len(item.Fingerprints) > 0 {
			builder.addHeading2("指纹信息")
			const maxItems = 300
			limit := len(item.Fingerprints)
			if limit > maxItems {
				limit = maxItems
			}
			rows := make([][]string, 0, limit)
			for i := 0; i < limit; i++ {
				fp := item.Fingerprints[i]
				rows = append(rows, []string{
					fp.URL,
					fmt.Sprintf("%d", fp.Status),
					fp.Title,
				})
			}
			builder.addTable([]string{"URL", "Status", "Title"}, rows)
		}

		if len(item.Ports) > 0 {
			builder.addHeading2("端口信息")
			const maxItems = 400
			limit := len(item.Ports)
			if limit > maxItems {
				limit = maxItems
			}
			rows := make([][]string, 0, limit)
			for i := 0; i < limit; i++ {
				p := item.Ports[i]
				rows = append(rows, []string{
					p.Host,
					fmt.Sprintf("%d", p.Port),
					p.Protocol,
				})
			}
			builder.addTable([]string{"Host", "Port", "Protocol"}, rows)
		}

		if len(item.DirResults) > 0 {
			builder.addHeading2("目录结果")
			const maxItems = 400
			limit := len(item.DirResults)
			if limit > maxItems {
				limit = maxItems
			}
			rows := make([][]string, 0, limit)
			for i := 0; i < limit; i++ {
				r := item.DirResults[i]
				rows = append(rows, []string{
					fmt.Sprintf("%d", r.Status),
					r.URL,
					r.Title,
				})
			}
			builder.addTable([]string{"Status", "URL", "Title"}, rows)
		}

		if len(item.JSAssets) > 0 {
			builder.addHeading2("JS 资产")
			const maxItems = 400
			limit := len(item.JSAssets)
			if limit > maxItems {
				limit = maxItems
			}
			rows := make([][]string, 0, limit)
			for i := 0; i < limit; i++ {
				a := item.JSAssets[i]
				rows = append(rows, []string{a.Type, a.Value, a.Source})
			}
			builder.addTable([]string{"Type", "Value", "Source"}, rows)
		}

		if len(item.Children) > 0 {
			builder.addHeading2("组合子任务")
			rows := make([][]string, 0, len(item.Children))
			for _, child := range item.Children {
				rows = append(rows, []string{
					fmt.Sprintf("%d", child.StepIndex),
					child.StepModule,
					child.ScanType,
					child.Status,
					child.ID,
				})
			}
			builder.addTable([]string{"Step", "Module", "Type", "Status", "TaskID"}, rows)
		}
	}

	return builder.build(reportName)
}

func GenerateMergedHTMLReportFromDB(refs []MergeTaskRef, reportName string) ([]byte, error) {
	items, err := loadMergedTaskData(refs)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(reportName) == "" {
		reportName = "攻击面评估合并报告"
	}

	totalVuln := 0
	totalAssets := 0
	for _, item := range items {
		totalVuln += mergedVulnCount(item)
		totalAssets += mergedAssetCount(item)
	}

	var sb strings.Builder
	sb.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"/>")
	sb.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"/>")
	sb.WriteString("<title>")
	sb.WriteString(htmlEscape(reportName))
	sb.WriteString("</title>")
	sb.WriteString("<style>")
	sb.WriteString(reportHTMLCoreStyle)
	sb.WriteString(".hero h1{font-size:28px}")
	sb.WriteString(".task{background:#fff;border:1px solid #e2e8f0;border-radius:14px;padding:14px 16px;margin-top:14px}")
	sb.WriteString(".task h2{margin:0 0 8px;font-size:20px}")
	sb.WriteString(".meta{font-size:12px;color:#475569;display:flex;gap:8px;flex-wrap:wrap}.chip{padding:2px 8px;border:1px solid #cbd5e1;border-radius:999px;background:#f8fafc}")
	sb.WriteString("table{margin-top:8px}")
	sb.WriteString(".h3{margin-top:12px;margin-bottom:8px;font-size:15px;color:#1e293b;font-weight:700}")
	sb.WriteString("</style></head><body><div class=\"container\">")
	sb.WriteString(fmt.Sprintf("<div class=\"hero\"><h1>%s</h1><p>生成时间: %s</p></div>", htmlEscape(reportName), time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("<div class=\"stats\">")
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">任务</div><div class=\"v\">%d</div></div>", len(items)))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">漏洞</div><div class=\"v\">%d</div></div>", totalVuln))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">资产/结果</div><div class=\"v\">%d</div></div>", totalAssets))
	sb.WriteString("</div>")

	for i, item := range items {
		task := item.Task
		sb.WriteString("<section class=\"task\">")
		sb.WriteString(fmt.Sprintf("<h2>%d. %s</h2>", i+1, htmlEscape(safeTaskName(task))))
		sb.WriteString("<div class=\"meta\">")
		sb.WriteString(fmt.Sprintf("<span class=\"chip\">ID: %s</span>", htmlEscape(task.ID)))
		sb.WriteString(fmt.Sprintf("<span class=\"chip\">版本: v%d</span>", task.Version))
		sb.WriteString(fmt.Sprintf("<span class=\"chip\">类型: %s</span>", htmlEscape(scanTypeLabel(task.ScanType))))
		sb.WriteString(fmt.Sprintf("<span class=\"chip\">状态: %s</span>", htmlEscape(task.Status)))
		sb.WriteString(fmt.Sprintf("<span class=\"chip\">漏洞: %d</span>", mergedVulnCount(item)))
		sb.WriteString(fmt.Sprintf("<span class=\"chip\">资产/结果: %d</span>", mergedAssetCount(item)))
		sb.WriteString("</div>")

		if len(item.Vulnerabilities) > 0 {
			sb.WriteString("<div class=\"h3\">漏洞信息</div>")
			sb.WriteString("<table><thead><tr><th>Severity</th><th>处置状态</th><th>Name</th><th>Protocol</th><th>URL</th><th>TemplateID</th></tr></thead><tbody>")
			for _, v := range item.Vulnerabilities {
				sb.WriteString("<tr>")
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(v.Severity)))
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(disposalStatusLabel(v.DisposalStatus))))
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(v.VulnName)))
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(v.Protocol)))
				sb.WriteString(fmt.Sprintf("<td class=\"mono\">%s</td>", htmlEscape(v.VulnURL)))
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(v.TemplateID)))
				sb.WriteString("</tr>")
				sb.WriteString("<tr><td colspan=\"6\">")
				sb.WriteString(fmt.Sprintf("<div><b>描述：</b>%s</div>", htmlEscape(truncateForReportText(v.Description, 2000))))
				if strings.TrimSpace(v.Reference) != "" {
					sb.WriteString(fmt.Sprintf("<div><b>参考：</b>%s</div>", htmlEscape(truncateForReportText(v.Reference, 1200))))
				}
				sb.WriteString("<details><summary>请求包</summary><pre>")
				sb.WriteString(htmlEscape(truncateForReportText(v.Request, 9000)))
				sb.WriteString("</pre></details><details><summary>响应包</summary><pre>")
				sb.WriteString(htmlEscape(truncateForReportText(v.Response, 9000)))
				sb.WriteString("</pre></details>")
				sb.WriteString("</td></tr>")
			}
			sb.WriteString("</tbody></table>")
		}

		if len(item.JSVulns) > 0 {
			sb.WriteString("<div class=\"h3\">JS 漏洞信息</div>")
			for _, v := range item.JSVulns {
				sb.WriteString("<div style=\"border:1px solid #e2e8f0;border-radius:10px;padding:10px;margin:8px 0;background:#fff\">")
				sb.WriteString(fmt.Sprintf("<div><b>[%s]</b> %s</div>", htmlEscape(v.Level), htmlEscape(v.Title)))
				sb.WriteString(fmt.Sprintf("<div class=\"mono\">%s %s</div>", htmlEscape(v.Method), htmlEscape(v.URL)))
				sb.WriteString(fmt.Sprintf("<div><b>处置状态：</b>%s</div>", htmlEscape(disposalStatusLabel(v.DisposalStatus))))
				sb.WriteString(fmt.Sprintf("<div><b>类型：</b>%s</div>", htmlEscape(v.Type)))
				sb.WriteString(fmt.Sprintf("<div><b>描述：</b>%s</div>", htmlEscape(truncateForReportText(v.Description, 2000))))
				sb.WriteString("<details><summary>请求包</summary><pre>")
				sb.WriteString(htmlEscape(truncateForReportText(v.Request, 9000)))
				sb.WriteString("</pre></details><details><summary>响应包</summary><pre>")
				sb.WriteString(htmlEscape(truncateForReportText(v.Response, 9000)))
				sb.WriteString("</pre></details></div>")
			}
		}

		if len(item.Fingerprints) > 0 {
			sb.WriteString("<div class=\"h3\">指纹信息</div><table><thead><tr><th>URL</th><th>Status</th><th>Title</th></tr></thead><tbody>")
			for _, fp := range item.Fingerprints {
				sb.WriteString("<tr>")
				sb.WriteString(fmt.Sprintf("<td class=\"mono\">%s</td>", htmlEscape(fp.URL)))
				sb.WriteString(fmt.Sprintf("<td>%d</td>", fp.Status))
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(fp.Title)))
				sb.WriteString("</tr>")
			}
			sb.WriteString("</tbody></table>")
		}

		if len(item.Ports) > 0 {
			sb.WriteString("<div class=\"h3\">端口信息</div><table><thead><tr><th>Host</th><th>Port</th><th>Protocol</th></tr></thead><tbody>")
			for _, p := range item.Ports {
				sb.WriteString("<tr>")
				sb.WriteString(fmt.Sprintf("<td class=\"mono\">%s</td>", htmlEscape(p.Host)))
				sb.WriteString(fmt.Sprintf("<td>%d</td>", p.Port))
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(p.Protocol)))
				sb.WriteString("</tr>")
			}
			sb.WriteString("</tbody></table>")
		}

		if len(item.DirResults) > 0 {
			sb.WriteString("<div class=\"h3\">目录结果</div><table><thead><tr><th>Status</th><th>URL</th><th>Title</th></tr></thead><tbody>")
			for _, r := range item.DirResults {
				sb.WriteString("<tr>")
				sb.WriteString(fmt.Sprintf("<td>%d</td>", r.Status))
				sb.WriteString(fmt.Sprintf("<td class=\"mono\">%s</td>", htmlEscape(r.URL)))
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(r.Title)))
				sb.WriteString("</tr>")
			}
			sb.WriteString("</tbody></table>")
		}

		if len(item.JSAssets) > 0 {
			sb.WriteString("<div class=\"h3\">JS 资产</div><table><thead><tr><th>Type</th><th>Value</th><th>Source</th></tr></thead><tbody>")
			for _, a := range item.JSAssets {
				sb.WriteString("<tr>")
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(a.Type)))
				sb.WriteString(fmt.Sprintf("<td class=\"mono\">%s</td>", htmlEscape(a.Value)))
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(a.Source)))
				sb.WriteString("</tr>")
			}
			sb.WriteString("</tbody></table>")
		}

		if len(item.Children) > 0 {
			sb.WriteString("<div class=\"h3\">组合子任务</div><table><thead><tr><th>Step</th><th>Module</th><th>Type</th><th>Status</th><th>TaskID</th></tr></thead><tbody>")
			for _, child := range item.Children {
				sb.WriteString("<tr>")
				sb.WriteString(fmt.Sprintf("<td>%d</td>", child.StepIndex))
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(child.StepModule)))
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(child.ScanType)))
				sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(child.Status)))
				sb.WriteString(fmt.Sprintf("<td class=\"mono\">%s</td>", htmlEscape(child.ID)))
				sb.WriteString("</tr>")
			}
			sb.WriteString("</tbody></table>")
		}

		sb.WriteString("</section>")
	}

	sb.WriteString("</div></body></html>")
	return []byte(sb.String()), nil
}

func GenerateMergedXLSXReportFromDB(refs []MergeTaskRef, reportName string) ([]byte, error) {
	items, err := loadMergedTaskData(refs)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(reportName) == "" {
		reportName = "攻击面评估合并报告"
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	summarySheet := "汇总"
	f.SetSheetName("Sheet1", summarySheet)
	f.SetCellValue(summarySheet, "A1", "报告名称")
	f.SetCellValue(summarySheet, "B1", reportName)
	f.SetCellValue(summarySheet, "A2", "任务数量")
	f.SetCellValue(summarySheet, "B2", len(items))
	f.SetCellValue(summarySheet, "A3", "生成时间")
	f.SetCellValue(summarySheet, "B3", time.Now().Format("2006-01-02 15:04:05"))

	f.SetCellValue(summarySheet, "A5", "任务ID")
	f.SetCellValue(summarySheet, "B5", "版本")
	f.SetCellValue(summarySheet, "C5", "任务名称")
	f.SetCellValue(summarySheet, "D5", "任务类型")
	f.SetCellValue(summarySheet, "E5", "状态")
	f.SetCellValue(summarySheet, "F5", "漏洞数")
	f.SetCellValue(summarySheet, "G5", "资产/结果数")

	for i, item := range items {
		row := i + 6
		f.SetCellValue(summarySheet, fmt.Sprintf("A%d", row), item.Task.ID)
		f.SetCellValue(summarySheet, fmt.Sprintf("B%d", row), item.Task.Version)
		f.SetCellValue(summarySheet, fmt.Sprintf("C%d", row), safeTaskName(item.Task))
		f.SetCellValue(summarySheet, fmt.Sprintf("D%d", row), scanTypeLabel(item.Task.ScanType))
		f.SetCellValue(summarySheet, fmt.Sprintf("E%d", row), item.Task.Status)
		f.SetCellValue(summarySheet, fmt.Sprintf("F%d", row), mergedVulnCount(item))
		f.SetCellValue(summarySheet, fmt.Sprintf("G%d", row), mergedAssetCount(item))
	}

	const cellLimit = 32000
	for i, item := range items {
		task := item.Task
		prefix := fmt.Sprintf("%02d", i+1)

		if len(item.Vulnerabilities) > 0 {
			sheet := sanitizeSheetName(fmt.Sprintf("%s_漏洞_%s", prefix, safeTaskName(task)))
			f.NewSheet(sheet)
			f.SetCellValue(sheet, "A1", "Severity")
			f.SetCellValue(sheet, "B1", "VulnName")
			f.SetCellValue(sheet, "C1", "Protocol")
			f.SetCellValue(sheet, "D1", "URL")
			f.SetCellValue(sheet, "E1", "TemplateID")
			f.SetCellValue(sheet, "F1", "Description")
			f.SetCellValue(sheet, "G1", "Reference")
			f.SetCellValue(sheet, "H1", "Request")
			f.SetCellValue(sheet, "I1", "Response")
			f.SetCellValue(sheet, "J1", "处置状态")
			for rowIdx, v := range item.Vulnerabilities {
				row := rowIdx + 2
				f.SetCellValue(sheet, fmt.Sprintf("A%d", row), v.Severity)
				f.SetCellValue(sheet, fmt.Sprintf("B%d", row), truncateForCell(v.VulnName, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("C%d", row), truncateForCell(v.Protocol, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("D%d", row), truncateForCell(v.VulnURL, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("E%d", row), truncateForCell(v.TemplateID, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("F%d", row), truncateForCell(v.Description, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("G%d", row), truncateForCell(v.Reference, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("H%d", row), truncateForCell(v.Request, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("I%d", row), truncateForCell(v.Response, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("J%d", row), disposalStatusLabel(v.DisposalStatus))
			}
		}

		if len(item.JSVulns) > 0 {
			sheet := sanitizeSheetName(fmt.Sprintf("%s_JS漏洞_%s", prefix, safeTaskName(task)))
			f.NewSheet(sheet)
			f.SetCellValue(sheet, "A1", "Level")
			f.SetCellValue(sheet, "B1", "Title")
			f.SetCellValue(sheet, "C1", "Type")
			f.SetCellValue(sheet, "D1", "Method")
			f.SetCellValue(sheet, "E1", "URL")
			f.SetCellValue(sheet, "F1", "Description")
			f.SetCellValue(sheet, "G1", "Request")
			f.SetCellValue(sheet, "H1", "Response")
			f.SetCellValue(sheet, "I1", "CreatedAt")
			f.SetCellValue(sheet, "J1", "处置状态")
			for rowIdx, v := range item.JSVulns {
				row := rowIdx + 2
				f.SetCellValue(sheet, fmt.Sprintf("A%d", row), v.Level)
				f.SetCellValue(sheet, fmt.Sprintf("B%d", row), truncateForCell(v.Title, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("C%d", row), truncateForCell(v.Type, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("D%d", row), truncateForCell(v.Method, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("E%d", row), truncateForCell(v.URL, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("F%d", row), truncateForCell(v.Description, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("G%d", row), truncateForCell(v.Request, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("H%d", row), truncateForCell(v.Response, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("I%d", row), v.CreatedAt.Format("2006-01-02 15:04:05"))
				f.SetCellValue(sheet, fmt.Sprintf("J%d", row), disposalStatusLabel(v.DisposalStatus))
			}
		}

		if len(item.Fingerprints) > 0 {
			sheet := sanitizeSheetName(fmt.Sprintf("%s_指纹_%s", prefix, safeTaskName(task)))
			f.NewSheet(sheet)
			f.SetCellValue(sheet, "A1", "URL")
			f.SetCellValue(sheet, "B1", "Status")
			f.SetCellValue(sheet, "C1", "Title")
			f.SetCellValue(sheet, "D1", "Fingerprints")
			for rowIdx, fp := range item.Fingerprints {
				row := rowIdx + 2
				f.SetCellValue(sheet, fmt.Sprintf("A%d", row), truncateForCell(fp.URL, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("B%d", row), fp.Status)
				f.SetCellValue(sheet, fmt.Sprintf("C%d", row), truncateForCell(fp.Title, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("D%d", row), truncateForCell(normalizeJSONList(fp.Fingerprints), cellLimit))
			}
		}

		if len(item.Ports) > 0 {
			sheet := sanitizeSheetName(fmt.Sprintf("%s_端口_%s", prefix, safeTaskName(task)))
			f.NewSheet(sheet)
			f.SetCellValue(sheet, "A1", "Host")
			f.SetCellValue(sheet, "B1", "Port")
			f.SetCellValue(sheet, "C1", "Protocol")
			f.SetCellValue(sheet, "D1", "RawResponse")
			for rowIdx, p := range item.Ports {
				row := rowIdx + 2
				f.SetCellValue(sheet, fmt.Sprintf("A%d", row), truncateForCell(p.Host, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("B%d", row), p.Port)
				f.SetCellValue(sheet, fmt.Sprintf("C%d", row), truncateForCell(p.Protocol, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("D%d", row), truncateForCell(p.RawResponse, cellLimit))
			}
		}

		if len(item.DirResults) > 0 {
			sheet := sanitizeSheetName(fmt.Sprintf("%s_目录_%s", prefix, safeTaskName(task)))
			f.NewSheet(sheet)
			f.SetCellValue(sheet, "A1", "Status")
			f.SetCellValue(sheet, "B1", "URL")
			f.SetCellValue(sheet, "C1", "Title")
			f.SetCellValue(sheet, "D1", "ContentType")
			for rowIdx, r := range item.DirResults {
				row := rowIdx + 2
				f.SetCellValue(sheet, fmt.Sprintf("A%d", row), r.Status)
				f.SetCellValue(sheet, fmt.Sprintf("B%d", row), truncateForCell(r.URL, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("C%d", row), truncateForCell(r.Title, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("D%d", row), truncateForCell(r.ContentType, cellLimit))
			}
		}

		if len(item.JSAssets) > 0 {
			sheet := sanitizeSheetName(fmt.Sprintf("%s_JS资产_%s", prefix, safeTaskName(task)))
			f.NewSheet(sheet)
			f.SetCellValue(sheet, "A1", "Type")
			f.SetCellValue(sheet, "B1", "Value")
			f.SetCellValue(sheet, "C1", "Source")
			for rowIdx, a := range item.JSAssets {
				row := rowIdx + 2
				f.SetCellValue(sheet, fmt.Sprintf("A%d", row), truncateForCell(a.Type, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("B%d", row), truncateForCell(a.Value, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("C%d", row), truncateForCell(a.Source, cellLimit))
			}
		}

		if len(item.Children) > 0 {
			sheet := sanitizeSheetName(fmt.Sprintf("%s_组合_%s", prefix, safeTaskName(task)))
			f.NewSheet(sheet)
			f.SetCellValue(sheet, "A1", "Step")
			f.SetCellValue(sheet, "B1", "Module")
			f.SetCellValue(sheet, "C1", "Type")
			f.SetCellValue(sheet, "D1", "Status")
			f.SetCellValue(sheet, "E1", "TaskID")
			for rowIdx, c := range item.Children {
				row := rowIdx + 2
				f.SetCellValue(sheet, fmt.Sprintf("A%d", row), c.StepIndex)
				f.SetCellValue(sheet, fmt.Sprintf("B%d", row), truncateForCell(c.StepModule, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("C%d", row), truncateForCell(c.ScanType, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("D%d", row), truncateForCell(c.Status, cellLimit))
				f.SetCellValue(sheet, fmt.Sprintf("E%d", row), truncateForCell(c.ID, cellLimit))
			}
		}
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
