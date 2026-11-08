package report

import (
	"fmt"
	"hephaestus-backend/pkg/database"
	"sort"
	"strings"
	"time"
)

func generateWebScanDOCXReport(task database.VulnScanTask, fingerprints []database.FingerprintInfo, vulnerabilities []database.VulnerabilityInfo, ports []database.PortScanResult, taskName string, version int) ([]byte, error) {
	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}
	if strings.TrimSpace(taskName) == "" {
		taskName = "攻击面评估任务"
	}

	sort.Slice(fingerprints, func(i, j int) bool {
		return fingerprints[i].URL < fingerprints[j].URL
	})
	sort.Slice(vulnerabilities, func(i, j int) bool {
		ri, rj := severityRank(vulnerabilities[i].Severity), severityRank(vulnerabilities[j].Severity)
		if ri == rj {
			return vulnerabilities[i].VulnURL < vulnerabilities[j].VulnURL
		}
		return ri < rj
	})
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Host == ports[j].Host {
			return ports[i].Port < ports[j].Port
		}
		return ports[i].Host < ports[j].Host
	})

	stats := calculateStats(fingerprints, vulnerabilities)
	builder := newDocxReportBuilder()
	builder.addTitle("攻击面评估报告")
	builder.addSubtitle(fmt.Sprintf("任务: %s", taskName))
	builder.addSubtitle(fmt.Sprintf("生成时间: %s", time.Now().Format("2006-01-02 15:04:05")))

	metaRows := [][2]string{
		{"任务ID", task.ID},
		{"任务名称", taskName},
		{"任务类型", scanTypeLabel(task.ScanType)},
		{"任务版本", fmt.Sprintf("v%d", version)},
		{"任务状态", task.Status},
	}
	if !task.CreatedAt.IsZero() {
		metaRows = append(metaRows, [2]string{"开始时间", task.CreatedAt.Format("2006-01-02 15:04:05")})
	}
	if task.CompletedAt != nil {
		metaRows = append(metaRows, [2]string{"完成时间", task.CompletedAt.Format("2006-01-02 15:04:05")})
	}
	builder.addHeading1("任务概览")
	builder.addKeyValueTable(metaRows)

	builder.addHeading1("统计概览")
	builder.addTable(
		[]string{"维度", "数量"},
		[][]string{
			{"指纹数", fmt.Sprintf("%d", stats.TotalFingerprints)},
			{"漏洞数", fmt.Sprintf("%d", stats.TotalVulnerabilities)},
			{"严重漏洞", fmt.Sprintf("%d", stats.SeverityCount["critical"])},
			{"高危漏洞", fmt.Sprintf("%d", stats.SeverityCount["high"])},
			{"中危漏洞", fmt.Sprintf("%d", stats.SeverityCount["medium"])},
			{"低危漏洞", fmt.Sprintf("%d", stats.SeverityCount["low"])},
			{"信息漏洞", fmt.Sprintf("%d", stats.SeverityCount["info"])},
			{"开放端口", fmt.Sprintf("%d", len(ports))},
		},
	)

	builder.addHeading1("目标列表")
	targets := parseTargets(task.Targets)
	if len(targets) == 0 {
		builder.addParagraph("无目标数据")
	} else {
		rows := make([][]string, 0, len(targets))
		for _, target := range targets {
			rows = append(rows, []string{target})
		}
		builder.addTable([]string{"目标"}, rows)
	}

	builder.addHeading1("指纹信息")
	if len(fingerprints) == 0 {
		builder.addParagraph("暂无指纹信息")
	} else {
		const maxItems = 600
		limit := len(fingerprints)
		if limit > maxItems {
			limit = maxItems
		}
		rows := make([][]string, 0, limit)
		for i := 0; i < limit; i++ {
			fp := fingerprints[i]
			rows = append(rows, []string{
				fp.URL,
				fmt.Sprintf("%d", fp.Status),
				fp.Title,
				normalizeJSONList(fp.Fingerprints),
				normalizeJSONList(fp.HighRiskFingerprints),
			})
		}
		builder.addTable([]string{"URL", "Status", "Title", "Fingerprints", "High Risk"}, rows)
		if len(fingerprints) > maxItems {
			builder.addParagraph(fmt.Sprintf("指纹信息共 %d 条，仅展示前 %d 条。", len(fingerprints), maxItems))
		}
	}

	builder.addHeading1("漏洞信息")
	if len(vulnerabilities) == 0 {
		builder.addParagraph("暂无漏洞信息")
	} else {
		const maxItems = 220
		limit := len(vulnerabilities)
		if limit > maxItems {
			limit = maxItems
		}
		summaryRows := make([][]string, 0, limit)
		for i := 0; i < limit; i++ {
			v := vulnerabilities[i]
			summaryRows = append(summaryRows, []string{
				v.Severity,
				disposalStatusLabel(v.DisposalStatus),
				v.VulnName,
				v.Protocol,
				v.VulnURL,
				v.TemplateID,
			})
		}
		builder.addTable([]string{"Severity", "处置状态", "Name", "Protocol", "URL", "TemplateID"}, summaryRows)
		if len(vulnerabilities) > maxItems {
			builder.addParagraph(fmt.Sprintf("漏洞信息共 %d 条，仅展示前 %d 条。", len(vulnerabilities), maxItems))
		}

		for i := 0; i < limit; i++ {
			v := vulnerabilities[i]
			builder.addHeading2(fmt.Sprintf("漏洞 #%d - %s", i+1, v.VulnName))
			builder.addKeyValueTable([][2]string{
				{"风险等级", v.Severity},
				{"处置状态", disposalStatusLabel(v.DisposalStatus)},
				{"协议", v.Protocol},
				{"URL", v.VulnURL},
				{"模板ID", v.TemplateID},
				{"描述", truncateForReportText(v.Description, 2600)},
				{"参考", truncateForReportText(v.Reference, 1800)},
			})
			builder.addHeading2("请求包")
			builder.addCodeBlock(v.Request)
			builder.addHeading2("响应包")
			builder.addCodeBlock(v.Response)
		}
	}

	builder.addHeading1("端口信息")
	if len(ports) == 0 {
		builder.addParagraph("暂无端口信息")
	} else {
		const maxItems = 800
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
			builder.addParagraph(fmt.Sprintf("端口信息共 %d 条，仅展示前 %d 条。", len(ports), maxItems))
		}
	}

	docBytes, err := builder.build(taskName + " - 攻击面评估报告")
	if err != nil {
		// 出错时回退一个最小可读内容，避免导出失败
		fallback := newDocxReportBuilder()
		fallback.addTitle("攻击面评估报告")
		fallback.addParagraph("报告生成过程中出现异常，当前展示基础摘要。")
		fallback.addKeyValueTable(metaRows)
		b, fallbackErr := fallback.build(taskName + " - 攻击面评估报告")
		if fallbackErr != nil {
			return nil, err
		}
		return b, nil
	}
	return docBytes, nil
}
