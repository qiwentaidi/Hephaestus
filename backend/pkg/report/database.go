package report

import (
	"encoding/json"
	"fmt"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/types"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

func loadReportData(taskID string, version int) (database.VulnScanTask, []database.FingerprintInfo, []database.VulnerabilityInfo, error) {
	db, err := database.GetDB()
	if err != nil {
		return database.VulnScanTask{}, nil, nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 获取任务信息
	var task database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", taskID, version).First(&task).Error; err != nil {
		return database.VulnScanTask{}, nil, nil, fmt.Errorf("任务不存在: %w", err)
	}

	// 获取指纹信息（只获取最新版本）
	var fingerprints []database.FingerprintInfo
	if err := db.Where("task_id = ? AND version = ?", taskID, version).Find(&fingerprints).Error; err != nil {
		return database.VulnScanTask{}, nil, nil, fmt.Errorf("查询指纹信息失败: %w", err)
	}

	// 获取漏洞信息（只获取最新版本）
	var vulnerabilities []database.VulnerabilityInfo
	if err := db.Where("task_id = ? AND version = ?", taskID, version).Find(&vulnerabilities).Error; err != nil {
		return database.VulnScanTask{}, nil, nil, fmt.Errorf("查询漏洞信息失败: %w", err)
	}

	return task, fingerprints, vulnerabilities, nil
}

// GenerateReportFromDB 从数据库生成报告（DOCX）
func GenerateReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, fingerprints, vulnerabilities, err := loadReportData(taskID, version)
	if err != nil {
		return nil, err
	}

	ports, _ := loadPortScanResults(taskID, version)
	return generateWebScanDOCXReport(task, fingerprints, vulnerabilities, ports, taskName, version)
}

// GenerateHTMLReportFromDB 从数据库生成 HTML 报告
func GenerateHTMLReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	_, fingerprints, vulnerabilities, err := loadReportData(taskID, version)
	if err != nil {
		return nil, err
	}

	fpResults := make([]types.InfoResult, 0, len(fingerprints))
	for _, fp := range fingerprints {
		var fpList []string
		_ = json.Unmarshal([]byte(fp.Fingerprints), &fpList)

		var highRisk []string
		_ = json.Unmarshal([]byte(fp.HighRiskFingerprints), &highRisk)

		fpResults = append(fpResults, types.InfoResult{
			TaskId:               taskID,
			URL:                  fp.URL,
			Scheme:               fp.Scheme,
			Host:                 fp.Host,
			Port:                 fp.Port,
			StatusCode:           fp.Status,
			Length:               fp.Length,
			Title:                fp.Title,
			Fingerprints:         fpList,
			HighRiskFingerprints: highRisk,
			Detect:               fp.Detect,
			Screenshot:           fp.Screenshot,
			Favicon:              fp.Favicon,
		})
	}

	pocs := make([]types.VulnerabilityInfo, 0, len(vulnerabilities))
	for _, v := range vulnerabilities {
		pocs = append(pocs, types.VulnerabilityInfo{
			TaskId:             taskID,
			ID:                 v.TemplateID,
			Name:               v.VulnName,
			Description:        v.Description,
			Reference:          v.Reference,
			Type:               v.Protocol,
			Severity:           v.Severity,
			URL:                v.VulnURL,
			Request:            v.Request,
			Response:           v.Response,
			ResponseTime:       v.ResponseTime,
			Extract:            v.Extract,
			PrincipleDetection: v.PrincipDetection,
		})
	}

	_ = taskName // 目前 HTML 模板里标题是固定的；保留参数便于未来扩展
	html := GenerateReport(fpResults, pocs)
	return []byte(html), nil
}

// GenerateXLSXReportFromDB 从数据库生成 XLSX 报告
func GenerateXLSXReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, fingerprints, vulnerabilities, err := loadReportData(taskID, version)
	if err != nil {
		return nil, err
	}
	ports, _ := loadPortScanResults(taskID, version)

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// 任务信息
	summarySheet := "任务信息"
	f.SetSheetName("Sheet1", summarySheet)
	f.SetCellValue(summarySheet, "A1", "任务ID")
	f.SetCellValue(summarySheet, "B1", taskID)
	f.SetCellValue(summarySheet, "A2", "任务名称")
	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}
	f.SetCellValue(summarySheet, "B2", taskName)
	f.SetCellValue(summarySheet, "A3", "版本")
	f.SetCellValue(summarySheet, "B3", version)
	f.SetCellValue(summarySheet, "A4", "状态")
	f.SetCellValue(summarySheet, "B4", task.Status)
	if !task.CreatedAt.IsZero() {
		f.SetCellValue(summarySheet, "A5", "开始时间")
		f.SetCellValue(summarySheet, "B5", task.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if task.CompletedAt != nil {
		f.SetCellValue(summarySheet, "A6", "完成时间")
		f.SetCellValue(summarySheet, "B6", task.CompletedAt.Format("2006-01-02 15:04:05"))
	}
	f.SetCellValue(summarySheet, "A8", "生成时间")
	f.SetCellValue(summarySheet, "B8", time.Now().Format("2006-01-02 15:04:05"))

	stats := calculateStats(fingerprints, vulnerabilities)
	f.SetCellValue(summarySheet, "A10", "统计")
	f.SetCellValue(summarySheet, "A11", "指纹总数")
	f.SetCellValue(summarySheet, "B11", stats.TotalFingerprints)
	f.SetCellValue(summarySheet, "A12", "漏洞总数")
	f.SetCellValue(summarySheet, "B12", stats.TotalVulnerabilities)
	f.SetCellValue(summarySheet, "A13", "严重漏洞")
	f.SetCellValue(summarySheet, "B13", stats.SeverityCount["critical"])
	f.SetCellValue(summarySheet, "A14", "高危漏洞")
	f.SetCellValue(summarySheet, "B14", stats.SeverityCount["high"])
	f.SetCellValue(summarySheet, "A15", "中危漏洞")
	f.SetCellValue(summarySheet, "B15", stats.SeverityCount["medium"])
	f.SetCellValue(summarySheet, "A16", "低危漏洞")
	f.SetCellValue(summarySheet, "B16", stats.SeverityCount["low"])
	f.SetCellValue(summarySheet, "A17", "信息漏洞")
	f.SetCellValue(summarySheet, "B17", stats.SeverityCount["info"])

	// 目标列表
	targetsSheet := "目标列表"
	f.NewSheet(targetsSheet)
	f.SetCellValue(targetsSheet, "A1", "目标")
	var targets []string
	if strings.TrimSpace(task.Targets) != "" {
		if err := json.Unmarshal([]byte(task.Targets), &targets); err != nil {
			targets = []string{task.Targets}
		}
	}
	for i, t := range targets {
		f.SetCellValue(targetsSheet, fmt.Sprintf("A%d", i+2), t)
	}

	// 指纹信息
	fpsSheet := "指纹信息"
	f.NewSheet(fpsSheet)
	f.SetCellValue(fpsSheet, "A1", "URL")
	f.SetCellValue(fpsSheet, "B1", "状态码")
	f.SetCellValue(fpsSheet, "C1", "标题")
	f.SetCellValue(fpsSheet, "D1", "指纹")
	f.SetCellValue(fpsSheet, "E1", "高危指纹")
	f.SetCellValue(fpsSheet, "F1", "资产标签")
	for i, fp := range fingerprints {
		row := i + 2
		f.SetCellValue(fpsSheet, fmt.Sprintf("A%d", row), fp.URL)
		f.SetCellValue(fpsSheet, fmt.Sprintf("B%d", row), fp.Status)
		f.SetCellValue(fpsSheet, fmt.Sprintf("C%d", row), fp.Title)
		f.SetCellValue(fpsSheet, fmt.Sprintf("D%d", row), normalizeJSONList(fp.Fingerprints))
		f.SetCellValue(fpsSheet, fmt.Sprintf("E%d", row), normalizeJSONList(fp.HighRiskFingerprints))
		f.SetCellValue(fpsSheet, fmt.Sprintf("F%d", row), normalizeJSONList(fp.AssetTags))
	}

	// 漏洞信息
	vulnSheet := "漏洞信息"
	f.NewSheet(vulnSheet)
	f.SetCellValue(vulnSheet, "A1", "风险等级")
	f.SetCellValue(vulnSheet, "B1", "漏洞名称")
	f.SetCellValue(vulnSheet, "C1", "协议")
	f.SetCellValue(vulnSheet, "D1", "URL")
	f.SetCellValue(vulnSheet, "E1", "模板ID")
	f.SetCellValue(vulnSheet, "F1", "提取信息")
	f.SetCellValue(vulnSheet, "G1", "描述")
	f.SetCellValue(vulnSheet, "H1", "参考链接")
	f.SetCellValue(vulnSheet, "I1", "请求(截断)")
	f.SetCellValue(vulnSheet, "J1", "响应(截断)")
	f.SetCellValue(vulnSheet, "K1", "处置状态")

	const cellLimit = 32000 // 避免单元格过大导致 Excel 打开卡顿
	for i, v := range vulnerabilities {
		row := i + 2
		f.SetCellValue(vulnSheet, fmt.Sprintf("A%d", row), v.Severity)
		f.SetCellValue(vulnSheet, fmt.Sprintf("B%d", row), v.VulnName)
		f.SetCellValue(vulnSheet, fmt.Sprintf("C%d", row), v.Protocol)
		f.SetCellValue(vulnSheet, fmt.Sprintf("D%d", row), v.VulnURL)
		f.SetCellValue(vulnSheet, fmt.Sprintf("E%d", row), v.TemplateID)
		f.SetCellValue(vulnSheet, fmt.Sprintf("F%d", row), truncateForCell(v.Extract, cellLimit))
		f.SetCellValue(vulnSheet, fmt.Sprintf("G%d", row), truncateForCell(v.Description, cellLimit))
		f.SetCellValue(vulnSheet, fmt.Sprintf("H%d", row), truncateForCell(v.Reference, cellLimit))
		f.SetCellValue(vulnSheet, fmt.Sprintf("I%d", row), truncateForCell(v.Request, cellLimit))
		f.SetCellValue(vulnSheet, fmt.Sprintf("J%d", row), truncateForCell(v.Response, cellLimit))
		f.SetCellValue(vulnSheet, fmt.Sprintf("K%d", row), disposalStatusLabel(v.DisposalStatus))
	}

	// 端口信息
	portSheet := "端口信息"
	f.NewSheet(portSheet)
	f.SetCellValue(portSheet, "A1", "Host")
	f.SetCellValue(portSheet, "B1", "Port")
	f.SetCellValue(portSheet, "C1", "Protocol")
	f.SetCellValue(portSheet, "D1", "RawResponse")
	f.SetCellValue(portSheet, "E1", "CreatedAt")
	for i, p := range ports {
		row := i + 2
		f.SetCellValue(portSheet, fmt.Sprintf("A%d", row), p.Host)
		f.SetCellValue(portSheet, fmt.Sprintf("B%d", row), p.Port)
		f.SetCellValue(portSheet, fmt.Sprintf("C%d", row), p.Protocol)
		f.SetCellValue(portSheet, fmt.Sprintf("D%d", row), truncateForCell(p.RawResponse, cellLimit))
		f.SetCellValue(portSheet, fmt.Sprintf("E%d", row), p.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	// 默认激活首页
	if idx, err := f.GetSheetIndex(summarySheet); err == nil {
		f.SetActiveSheet(idx)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func truncateForCell(s string, limit int) string {
	if limit <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "..."
}

// generateReportRTF 生成Word可读的RTF报告
func generateReportRTF(task database.VulnScanTask, fingerprints []database.FingerprintInfo, vulnerabilities []database.VulnerabilityInfo, ports []database.PortScanResult, taskName string, version int) string {
	var sb strings.Builder

	// 统计信息
	stats := calculateStats(fingerprints, vulnerabilities)

	reportTitle := "漏洞扫描报告"
	if taskName != "" {
		reportTitle = fmt.Sprintf("%s - 漏洞扫描报告", taskName)
	}

	sb.WriteString("{\\rtf1\\ansi\\deff0{\\fonttbl{\\f0 Arial;}{\\f1 SimSun;}}\\uc1\n")
	sb.WriteString("\\fs32\\b ")
	sb.WriteString(rtfEscapeText(reportTitle))
	sb.WriteString("\\b0\\par\n")
	sb.WriteString("\\fs20 ")
	sb.WriteString(rtfEscapeText(fmt.Sprintf("生成时间: %s", time.Now().Format("2006-01-02 15:04:05"))))
	sb.WriteString("\\par\n")
	sb.WriteString(rtfEscapeText(fmt.Sprintf("任务版本: %d", version)))
	sb.WriteString("\\par\n")
	sb.WriteString(rtfEscapeText(fmt.Sprintf("任务状态: %s", task.Status)))
	sb.WriteString("\\par\n")
	if !task.CreatedAt.IsZero() {
		sb.WriteString(rtfEscapeText(fmt.Sprintf("开始时间: %s", task.CreatedAt.Format("2006-01-02 15:04:05"))))
		sb.WriteString("\\par\n")
	}
	if task.CompletedAt != nil {
		sb.WriteString(rtfEscapeText(fmt.Sprintf("完成时间: %s", task.CompletedAt.Format("2006-01-02 15:04:05"))))
		sb.WriteString("\\par\n")
	}
	sb.WriteString("\\par\n")

	writeSectionTitle(&sb, "统计概览")
	writeKeyValue(&sb, "指纹总数", fmt.Sprintf("%d", stats.TotalFingerprints))
	writeKeyValue(&sb, "漏洞总数", fmt.Sprintf("%d", stats.TotalVulnerabilities))
	writeKeyValue(&sb, "严重漏洞", fmt.Sprintf("%d", stats.SeverityCount["critical"]))
	writeKeyValue(&sb, "高危漏洞", fmt.Sprintf("%d", stats.SeverityCount["high"]))
	writeKeyValue(&sb, "中危漏洞", fmt.Sprintf("%d", stats.SeverityCount["medium"]))
	writeKeyValue(&sb, "低危漏洞", fmt.Sprintf("%d", stats.SeverityCount["low"]))
	writeKeyValue(&sb, "信息漏洞", fmt.Sprintf("%d", stats.SeverityCount["info"]))
	sb.WriteString("\\par\n")

	if len(task.Targets) > 0 {
		writeSectionTitle(&sb, "目标列表")
		var targets []string
		if err := json.Unmarshal([]byte(task.Targets), &targets); err != nil {
			targets = []string{task.Targets}
		}
		if len(targets) == 0 {
			sb.WriteString(rtfEscapeText("无目标数据"))
			sb.WriteString("\\par\n")
		} else {
			for _, target := range targets {
				writeBullet(&sb, target)
			}
		}
		sb.WriteString("\\par\n")
	}

	writeSectionTitle(&sb, "指纹信息")
	if len(fingerprints) == 0 {
		sb.WriteString(rtfEscapeText("暂无指纹信息"))
		sb.WriteString("\\par\n")
	} else {
		for i, fp := range fingerprints {
			writeSubTitle(&sb, fmt.Sprintf("指纹 #%d", i+1))
			writeKeyValue(&sb, "URL", fp.URL)
			writeKeyValue(&sb, "状态码", fmt.Sprintf("%d", fp.Status))
			writeKeyValue(&sb, "标题", fp.Title)
			writeKeyValue(&sb, "指纹", normalizeJSONList(fp.Fingerprints))
			writeKeyValue(&sb, "高危指纹", normalizeJSONList(fp.HighRiskFingerprints))
			writeKeyValue(&sb, "资产标签", normalizeJSONList(fp.AssetTags))
			sb.WriteString("\\par\n")
		}
	}

	writeSectionTitle(&sb, "漏洞信息")
	if len(vulnerabilities) == 0 {
		sb.WriteString(rtfEscapeText("暂无漏洞信息"))
		sb.WriteString("\\par\n")
	} else {
		sort.Slice(vulnerabilities, func(i, j int) bool {
			if strings.EqualFold(vulnerabilities[i].Severity, vulnerabilities[j].Severity) {
				return vulnerabilities[i].VulnURL < vulnerabilities[j].VulnURL
			}
			return strings.ToLower(vulnerabilities[i].Severity) < strings.ToLower(vulnerabilities[j].Severity)
		})

		const maxItems = 200
		limit := len(vulnerabilities)
		if limit > maxItems {
			limit = maxItems
		}
		for i, vuln := range vulnerabilities {
			if i >= limit {
				break
			}
			writeSubTitle(&sb, fmt.Sprintf("漏洞 #%d", i+1))
			writeKeyValue(&sb, "漏洞名称", vuln.VulnName)
			writeKeyValue(&sb, "风险等级", vuln.Severity)
			writeKeyValue(&sb, "处置状态", disposalStatusLabel(vuln.DisposalStatus))
			writeKeyValue(&sb, "协议", vuln.Protocol)
			writeKeyValue(&sb, "URL", vuln.VulnURL)
			writeKeyValue(&sb, "模板ID", vuln.TemplateID)
			writeKeyValue(&sb, "描述", truncateForReportText(vuln.Description, 3000))
			writeKeyValue(&sb, "参考", truncateForReportText(vuln.Reference, 2000))
			writeKeyValue(&sb, "请求包", truncateForReportText(vuln.Request, 5000))
			writeKeyValue(&sb, "响应包", truncateForReportText(vuln.Response, 5000))
			sb.WriteString("\\par\n")
		}
		if len(vulnerabilities) > maxItems {
			sb.WriteString(rtfEscapeText(fmt.Sprintf("...（共 %d 条，仅展示前 %d 条）", len(vulnerabilities), maxItems)))
			sb.WriteString("\\par\n")
		}
	}

	writeSectionTitle(&sb, "端口信息")
	if len(ports) == 0 {
		sb.WriteString(rtfEscapeText("暂无端口信息"))
		sb.WriteString("\\par\n")
	} else {
		const maxItems = 500
		limit := len(ports)
		if limit > maxItems {
			limit = maxItems
		}
		for i := 0; i < limit; i++ {
			p := ports[i]
			writeBullet(&sb, fmt.Sprintf("%s:%d (%s)", p.Host, p.Port, p.Protocol))
		}
		if len(ports) > maxItems {
			sb.WriteString(rtfEscapeText(fmt.Sprintf("...（共 %d 条，仅展示前 %d 条）", len(ports), maxItems)))
			sb.WriteString("\\par\n")
		}
	}

	sb.WriteString("}")
	return sb.String()
}

// calculateStats 计算统计信息
func calculateStats(fingerprints []database.FingerprintInfo, vulnerabilities []database.VulnerabilityInfo) Stats {
	stats := Stats{
		TotalFingerprints:    len(fingerprints),
		TotalVulnerabilities: len(vulnerabilities),
		SeverityCount:        make(map[string]int),
		StatusCodeCount:      make(map[int]int),
	}

	// 统计漏洞严重程度
	for _, vuln := range vulnerabilities {
		severity := strings.ToLower(vuln.Severity)
		stats.SeverityCount[severity]++
	}

	// 统计状态码
	for _, fp := range fingerprints {
		stats.StatusCodeCount[fp.Status]++
	}

	return stats
}

// Stats 统计信息
type Stats struct {
	TotalFingerprints    int
	TotalVulnerabilities int
	SeverityCount        map[string]int
	StatusCodeCount      map[int]int
}

func normalizeJSONList(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "-"
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err == nil && len(items) > 0 {
		return strings.Join(items, ", ")
	}
	return raw
}

func writeSectionTitle(sb *strings.Builder, title string) {
	sb.WriteString("\\fs24\\b ")
	sb.WriteString(rtfEscapeText(title))
	sb.WriteString("\\b0\\par\n")
}

func writeSubTitle(sb *strings.Builder, title string) {
	sb.WriteString("\\fs22\\b ")
	sb.WriteString(rtfEscapeText(title))
	sb.WriteString("\\b0\\par\n")
}

func writeKeyValue(sb *strings.Builder, key, value string) {
	if strings.TrimSpace(value) == "" {
		value = "-"
	}
	sb.WriteString("\\fs20\\b ")
	sb.WriteString(rtfEscapeText(key))
	sb.WriteString(":\\b0 ")
	sb.WriteString(rtfEscapeText(value))
	sb.WriteString("\\par\n")
}

func writeBullet(sb *strings.Builder, text string) {
	sb.WriteString("\\pard\\fi-360\\li720\\tx720\\bullet\\tab ")
	sb.WriteString(rtfEscapeText(text))
	sb.WriteString("\\par\n\\pard\n")
}

func rtfEscapeText(input string) string {
	var sb strings.Builder
	for _, r := range input {
		switch r {
		case '\\':
			sb.WriteString("\\\\")
		case '{':
			sb.WriteString("\\{")
		case '}':
			sb.WriteString("\\}")
		case '\n':
			sb.WriteString("\\par ")
		case '\r':
			continue
		case '\t':
			sb.WriteString("\\tab ")
		default:
			if r <= 0x7f {
				sb.WriteRune(r)
			} else {
				val := int32(r)
				if val > 0x7fff {
					val = val - 0x10000
				}
				sb.WriteString(fmt.Sprintf("\\u%d?", val))
			}
		}
	}
	return sb.String()
}
