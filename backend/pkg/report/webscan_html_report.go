package report

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// GenerateWebScanHTMLReportFromDB 导出 WebScan/Crack 报告（HTML，包含指纹/漏洞/端口）
func GenerateWebScanHTMLReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, fingerprints, vulnerabilities, err := loadReportData(taskID, version)
	if err != nil {
		return nil, err
	}
	ports, _ := loadPortScanResults(taskID, version)

	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}

	sort.Slice(fingerprints, func(i, j int) bool {
		return fingerprints[i].URL < fingerprints[j].URL
	})
	sort.Slice(vulnerabilities, func(i, j int) bool {
		if vulnerabilities[i].Severity == vulnerabilities[j].Severity {
			return vulnerabilities[i].VulnURL < vulnerabilities[j].VulnURL
		}
		return vulnerabilities[i].Severity < vulnerabilities[j].Severity
	})
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Host == ports[j].Host {
			return ports[i].Port < ports[j].Port
		}
		return ports[i].Host < ports[j].Host
	})

	stats := calculateStats(fingerprints, vulnerabilities)

	var sb strings.Builder
	sb.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"/>")
	sb.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"/>")
	sb.WriteString("<title>漏洞扫描报告</title>")
	sb.WriteString("<style>")
	sb.WriteString(reportHTMLCoreStyle)
	sb.WriteString(".section{background:#fff;border:1px solid #e2e8f0;border-radius:14px;padding:14px 16px;margin-top:14px}")
	sb.WriteString(".section h2{margin:0 0 10px;font-size:18px}")
	sb.WriteString(".targets-area{width:100%;min-height:96px;max-height:220px;padding:10px 12px;border:1px solid #e2e8f0;border-radius:10px;background:#f8fafc;color:#0f172a;line-height:1.5;resize:vertical;overflow:auto;box-sizing:border-box}")
	sb.WriteString(".sev{display:inline-block;padding:2px 8px;border-radius:999px;border:1px solid;font-size:12px}.sev-critical{color:#7f1d1d;background:#fee2e2;border-color:#fca5a5}.sev-high{color:#9a3412;background:#ffedd5;border-color:#fdba74}.sev-medium{color:#854d0e;background:#fef3c7;border-color:#fcd34d}.sev-low{color:#1d4ed8;background:#dbeafe;border-color:#93c5fd}.sev-info{color:#334155;background:#e2e8f0;border-color:#cbd5e1}")
	sb.WriteString("</style></head><body><div class=\"container\">")
	sb.WriteString(fmt.Sprintf("<div class=\"hero\"><h1>漏洞扫描报告</h1><p>任务: %s | 类型: %s | 版本: v%d | 状态: %s | 生成时间: %s</p></div>",
		htmlEscape(taskName), htmlEscape(task.ScanType), version, htmlEscape(task.Status), time.Now().Format("2006-01-02 15:04:05")))

	sb.WriteString("<div class=\"stats\">")
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">指纹</div><div class=\"v\">%d</div></div>", stats.TotalFingerprints))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">漏洞</div><div class=\"v\">%d</div></div>", stats.TotalVulnerabilities))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">严重</div><div class=\"v\">%d</div></div>", stats.SeverityCount["critical"]))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">高危</div><div class=\"v\">%d</div></div>", stats.SeverityCount["high"]))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">中危</div><div class=\"v\">%d</div></div>", stats.SeverityCount["medium"]))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">低危</div><div class=\"v\">%d</div></div>", stats.SeverityCount["low"]))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">信息</div><div class=\"v\">%d</div></div>", stats.SeverityCount["info"]))
	sb.WriteString(fmt.Sprintf("<div class=\"stat\"><div class=\"k\">端口</div><div class=\"v\">%d</div></div>", len(ports)))
	sb.WriteString("</div>")

	// Targets
	targets := parseTargets(task.Targets)
	sb.WriteString("<section class=\"section\"><h2>目标列表</h2>")
	if len(targets) == 0 {
		sb.WriteString("<div>无目标数据</div>")
	} else {
		sb.WriteString("<textarea class=\"targets-area\" readonly>")
		for i, t := range targets {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(htmlEscape(t))
		}
		sb.WriteString("</textarea>")
	}
	sb.WriteString("</section>")

	// Fingerprints
	sb.WriteString("<section class=\"section\"><h2>指纹信息</h2>")
	sb.WriteString("<table><thead><tr><th>URL</th><th>Status</th><th>Title</th><th>Fingerprints</th><th>High Risk</th></tr></thead><tbody>")
	for _, fp := range fingerprints {
		sb.WriteString("<tr>")
		sb.WriteString(fmt.Sprintf("<td class=\"mono\">%s</td>", htmlEscape(fp.URL)))
		sb.WriteString(fmt.Sprintf("<td>%d</td>", fp.Status))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(fp.Title)))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(normalizeJSONList(fp.Fingerprints))))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(normalizeJSONList(fp.HighRiskFingerprints))))
		sb.WriteString("</tr>")
	}
	if len(fingerprints) == 0 {
		sb.WriteString("<tr><td colspan=\"5\">暂无指纹信息</td></tr>")
	}
	sb.WriteString("</tbody></table></section>")

	// Vulnerabilities
	sb.WriteString("<section class=\"section\"><h2>漏洞信息</h2>")
	sb.WriteString("<table><thead><tr><th>Severity</th><th>处置状态</th><th>Name</th><th>Protocol</th><th>URL</th><th>TemplateID</th></tr></thead><tbody>")
	for _, v := range vulnerabilities {
		sev := strings.ToLower(strings.TrimSpace(v.Severity))
		if sev == "" {
			sev = "info"
		}
		sb.WriteString("<tr>")
		sb.WriteString(fmt.Sprintf("<td><span class=\"sev sev-%s\">%s</span></td>", htmlEscape(sev), htmlEscape(v.Severity)))
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
		sb.WriteString(htmlEscape(truncateForReportText(v.Request, 10000)))
		sb.WriteString("</pre></details>")
		sb.WriteString("<details><summary>响应包</summary><pre>")
		sb.WriteString(htmlEscape(truncateForReportText(v.Response, 10000)))
		sb.WriteString("</pre></details>")
		sb.WriteString("</td></tr>")
	}
	if len(vulnerabilities) == 0 {
		sb.WriteString("<tr><td colspan=\"6\">暂无漏洞信息</td></tr>")
	}
	sb.WriteString("</tbody></table></section>")

	// Ports
	sb.WriteString("<section class=\"section\"><h2>端口信息</h2>")
	sb.WriteString("<table><thead><tr><th>Host</th><th>Port</th><th>Protocol</th><th>CreatedAt</th></tr></thead><tbody>")
	for _, p := range ports {
		sb.WriteString("<tr>")
		sb.WriteString(fmt.Sprintf("<td class=\"mono\">%s</td>", htmlEscape(p.Host)))
		sb.WriteString(fmt.Sprintf("<td>%d</td>", p.Port))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(p.Protocol)))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", p.CreatedAt.Format("2006-01-02 15:04:05")))
		sb.WriteString("</tr>")
	}
	if len(ports) == 0 {
		sb.WriteString("<tr><td colspan=\"4\">暂无端口信息</td></tr>")
	}
	sb.WriteString("</tbody></table></section>")

	sb.WriteString("</div></body></html>")
	return []byte(sb.String()), nil
}
