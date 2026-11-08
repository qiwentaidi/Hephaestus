package report

import (
	"encoding/json"
	"fmt"
	"hephaestus-backend/pkg/company"
	"hephaestus-backend/pkg/database"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type CompanyTaskRef struct {
	ID string
}

func LoadCompanyInfoTaskByID(taskID string) (database.CompanyInfoTask, error) {
	db, err := database.GetDB()
	if err != nil {
		return database.CompanyInfoTask{}, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	var task database.CompanyInfoTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return database.CompanyInfoTask{}, fmt.Errorf("任务不存在")
		}
		return database.CompanyInfoTask{}, fmt.Errorf("查询任务失败: %w", err)
	}
	return task, nil
}

func loadCompanyInfoTaskResult(taskID string) ([]company.CompanyInfo, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var result database.CompanyInfoTaskResult
	if err := db.Where("task_id = ?", taskID).First(&result).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("任务结果不存在")
		}
		return nil, fmt.Errorf("查询任务结果失败: %w", err)
	}

	var companiesInfo []company.CompanyInfo
	if err := jsonUnmarshalString(result.Data, &companiesInfo); err != nil {
		return nil, fmt.Errorf("解析结果数据失败: %w", err)
	}
	return company.NormalizeCompanyInfos(companiesInfo), nil
}

func jsonUnmarshalString(raw string, target interface{}) error {
	return json.Unmarshal([]byte(raw), target)
}

func buildCompanyInfoWorkbook(companiesInfo []company.CompanyInfo) (*excelize.File, error) {
	f := excelize.NewFile()

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#E0E0E0"}, Pattern: 1},
	})

	summarySheet := "公司信息"
	index, _ := f.NewSheet(summarySheet)
	f.SetActiveSheet(index)

	headers1 := []string{"公司名称", "经营状态", "域名数量", "子公司数量", "公众号数量", "APP数量", "小程序数量"}
	for i, header := range headers1 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(summarySheet, cell, header)
	}
	cellRange1, _ := excelize.CoordinatesToCellName(1, 1)
	cellRange2, _ := excelize.CoordinatesToCellName(len(headers1), 1)
	f.SetCellStyle(summarySheet, cellRange1, cellRange2, headerStyle)

	row := 2
	walkCompanies(companiesInfo, func(comp company.CompanyInfo, level int, parentCompany string) {
		f.SetCellValue(summarySheet, fmt.Sprintf("A%d", row), comp.CompanyName)
		f.SetCellValue(summarySheet, fmt.Sprintf("B%d", row), comp.RegStatus)
		f.SetCellValue(summarySheet, fmt.Sprintf("C%d", row), len(comp.Domains))
		f.SetCellValue(summarySheet, fmt.Sprintf("D%d", row), len(comp.Subsidiaries))
		f.SetCellValue(summarySheet, fmt.Sprintf("E%d", row), len(comp.OfficialAccounts))
		f.SetCellValue(summarySheet, fmt.Sprintf("F%d", row), len(comp.Apps))
		f.SetCellValue(summarySheet, fmt.Sprintf("G%d", row), len(comp.Applets))
		row++
	})

	emailSheet := "邮箱"
	_, _ = f.NewSheet(emailSheet)
	emailHeaders := []string{"邮箱", "类型", "所属公司", "关联域名", "来源", "标签", "泄露次数", "泄露账号", "最近泄露时间"}
	for i, header := range emailHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(emailSheet, cell, header)
	}
	cellRangeEmail1, _ := excelize.CoordinatesToCellName(1, 1)
	cellRangeEmail2, _ := excelize.CoordinatesToCellName(len(emailHeaders), 1)
	f.SetCellStyle(emailSheet, cellRangeEmail1, cellRangeEmail2, headerStyle)
	rowEmail := 2
	walkCompanies(companiesInfo, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, email := range comp.Emails {
			f.SetCellValue(emailSheet, fmt.Sprintf("A%d", rowEmail), email.Address)
			f.SetCellValue(emailSheet, fmt.Sprintf("B%d", rowEmail), email.Type)
			f.SetCellValue(emailSheet, fmt.Sprintf("C%d", rowEmail), comp.CompanyName)
			f.SetCellValue(emailSheet, fmt.Sprintf("D%d", rowEmail), strings.Join(email.Domains, "\n"))
			f.SetCellValue(emailSheet, fmt.Sprintf("E%d", rowEmail), strings.Join(email.Sources, "\n"))
			f.SetCellValue(emailSheet, fmt.Sprintf("F%d", rowEmail), strings.Join(email.Tags, "\n"))
			f.SetCellValue(emailSheet, fmt.Sprintf("G%d", rowEmail), email.LeakageCount)
			f.SetCellValue(emailSheet, fmt.Sprintf("H%d", rowEmail), strings.Join(email.LeakageAccounts, "\n"))
			f.SetCellValue(emailSheet, fmt.Sprintf("I%d", rowEmail), email.LeakageUpdatedAt)
			rowEmail++
		}
	})

	domainSheet := "域名"
	_, _ = f.NewSheet(domainSheet)
	headers2 := []string{"域名", "所属公司"}
	for i, header := range headers2 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(domainSheet, cell, header)
	}
	cellRange3, _ := excelize.CoordinatesToCellName(1, 1)
	cellRange4, _ := excelize.CoordinatesToCellName(len(headers2), 1)
	f.SetCellStyle(domainSheet, cellRange3, cellRange4, headerStyle)
	row2 := 2
	walkCompanies(companiesInfo, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, domain := range comp.Domains {
			f.SetCellValue(domainSheet, fmt.Sprintf("A%d", row2), domain)
			f.SetCellValue(domainSheet, fmt.Sprintf("B%d", row2), comp.CompanyName)
			row2++
		}
	})

	subsidiarySheet := "对外投资"
	_, _ = f.NewSheet(subsidiarySheet)
	headersSubsidiary := []string{"公司名称", "层级", "母公司", "投资比例", "注册资本", "经营状态", "域名数量", "子公司数量", "公众号数量", "APP数量", "小程序数量"}
	for i, header := range headersSubsidiary {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(subsidiarySheet, cell, header)
	}
	cellRange5, _ := excelize.CoordinatesToCellName(1, 1)
	cellRange6, _ := excelize.CoordinatesToCellName(len(headersSubsidiary), 1)
	f.SetCellStyle(subsidiarySheet, cellRange5, cellRange6, headerStyle)
	row3 := 2
	walkCompanies(companiesInfo, func(comp company.CompanyInfo, level int, parentCompany string) {
		if level == 0 {
			return
		}
		f.SetCellValue(subsidiarySheet, fmt.Sprintf("A%d", row3), comp.CompanyName)
		f.SetCellValue(subsidiarySheet, fmt.Sprintf("B%d", row3), level)
		f.SetCellValue(subsidiarySheet, fmt.Sprintf("C%d", row3), parentCompany)
		f.SetCellValue(subsidiarySheet, fmt.Sprintf("D%d", row3), fmt.Sprintf("%.0f%%", comp.Investment))
		f.SetCellValue(subsidiarySheet, fmt.Sprintf("E%d", row3), comp.Amount)
		f.SetCellValue(subsidiarySheet, fmt.Sprintf("F%d", row3), comp.RegStatus)
		f.SetCellValue(subsidiarySheet, fmt.Sprintf("G%d", row3), len(comp.Domains))
		f.SetCellValue(subsidiarySheet, fmt.Sprintf("H%d", row3), len(comp.Subsidiaries))
		f.SetCellValue(subsidiarySheet, fmt.Sprintf("I%d", row3), len(comp.OfficialAccounts))
		f.SetCellValue(subsidiarySheet, fmt.Sprintf("J%d", row3), len(comp.Apps))
		f.SetCellValue(subsidiarySheet, fmt.Sprintf("K%d", row3), len(comp.Applets))
		row3++
	})

	officialSheet := "公众号"
	_, _ = f.NewSheet(officialSheet)
	headers3 := []string{"名称", "账号", "简介", "所属公司"}
	for i, header := range headers3 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(officialSheet, cell, header)
	}
	cellRange7, _ := excelize.CoordinatesToCellName(1, 1)
	cellRange8, _ := excelize.CoordinatesToCellName(len(headers3), 1)
	f.SetCellStyle(officialSheet, cellRange7, cellRange8, headerStyle)
	row4 := 2
	walkCompanies(companiesInfo, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, acc := range comp.OfficialAccounts {
			f.SetCellValue(officialSheet, fmt.Sprintf("A%d", row4), acc.Name)
			f.SetCellValue(officialSheet, fmt.Sprintf("B%d", row4), acc.Numbers)
			f.SetCellValue(officialSheet, fmt.Sprintf("C%d", row4), acc.Introduction)
			f.SetCellValue(officialSheet, fmt.Sprintf("D%d", row4), comp.CompanyName)
			row4++
		}
	})

	appSheet := "APP"
	_, _ = f.NewSheet(appSheet)
	headers4 := []string{"服务名称", "许可证", "单位名称", "更新时间", "所属公司"}
	for i, header := range headers4 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(appSheet, cell, header)
	}
	cellRange9, _ := excelize.CoordinatesToCellName(1, 1)
	cellRange10, _ := excelize.CoordinatesToCellName(len(headers4), 1)
	f.SetCellStyle(appSheet, cellRange9, cellRange10, headerStyle)
	row5 := 2
	walkCompanies(companiesInfo, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, app := range comp.Apps {
			f.SetCellValue(appSheet, fmt.Sprintf("A%d", row5), app.ServiceName)
			f.SetCellValue(appSheet, fmt.Sprintf("B%d", row5), app.ServiceLicence)
			f.SetCellValue(appSheet, fmt.Sprintf("C%d", row5), app.UnitName)
			f.SetCellValue(appSheet, fmt.Sprintf("D%d", row5), app.UpdateRecordTime)
			f.SetCellValue(appSheet, fmt.Sprintf("E%d", row5), comp.CompanyName)
			row5++
		}
	})

	appletSheet := "小程序"
	_, _ = f.NewSheet(appletSheet)
	headers5 := []string{"服务名称", "许可证", "单位名称", "更新时间", "所属公司"}
	for i, header := range headers5 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(appletSheet, cell, header)
	}
	cellRange11, _ := excelize.CoordinatesToCellName(1, 1)
	cellRange12, _ := excelize.CoordinatesToCellName(len(headers5), 1)
	f.SetCellStyle(appletSheet, cellRange11, cellRange12, headerStyle)
	row6 := 2
	walkCompanies(companiesInfo, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, applet := range comp.Applets {
			f.SetCellValue(appletSheet, fmt.Sprintf("A%d", row6), applet.ServiceName)
			f.SetCellValue(appletSheet, fmt.Sprintf("B%d", row6), applet.ServiceLicence)
			f.SetCellValue(appletSheet, fmt.Sprintf("C%d", row6), applet.UnitName)
			f.SetCellValue(appletSheet, fmt.Sprintf("D%d", row6), applet.UpdateRecordTime)
			f.SetCellValue(appletSheet, fmt.Sprintf("E%d", row6), comp.CompanyName)
			row6++
		}
	})

	f.DeleteSheet("Sheet1")
	f.SetColWidth(summarySheet, "A", "A", 30)
	f.SetColWidth(summarySheet, "B", "B", 15)
	f.SetColWidth(emailSheet, "A", "A", 36)
	f.SetColWidth(emailSheet, "C", "C", 28)
	f.SetColWidth(emailSheet, "D", "I", 32)
	f.SetColWidth(domainSheet, "A", "A", 40)
	f.SetColWidth(domainSheet, "B", "B", 30)
	f.SetColWidth(subsidiarySheet, "A", "A", 30)
	f.SetColWidth(subsidiarySheet, "C", "C", 30)
	f.SetColWidth(subsidiarySheet, "E", "E", 20)
	f.SetColWidth(subsidiarySheet, "F", "F", 15)
	f.SetColWidth(officialSheet, "A", "A", 30)
	f.SetColWidth(officialSheet, "B", "B", 30)
	f.SetColWidth(officialSheet, "C", "C", 50)
	f.SetColWidth(officialSheet, "D", "D", 30)
	f.SetColWidth(appSheet, "A", "A", 30)
	f.SetColWidth(appSheet, "B", "B", 30)
	f.SetColWidth(appSheet, "C", "C", 30)
	f.SetColWidth(appSheet, "D", "D", 20)
	f.SetColWidth(appSheet, "E", "E", 30)
	f.SetColWidth(appletSheet, "A", "A", 30)
	f.SetColWidth(appletSheet, "B", "B", 30)
	f.SetColWidth(appletSheet, "C", "C", 30)
	f.SetColWidth(appletSheet, "D", "D", 20)
	f.SetColWidth(appletSheet, "E", "E", 30)

	return f, nil
}

func walkCompanies(companies []company.CompanyInfo, fn func(comp company.CompanyInfo, level int, parentCompany string)) {
	var walk func(items []company.CompanyInfo, level int, parentCompany string)
	walk = func(items []company.CompanyInfo, level int, parentCompany string) {
		for _, comp := range items {
			fn(comp, level, parentCompany)
			if len(comp.Subsidiaries) > 0 {
				walk(comp.Subsidiaries, level+1, comp.CompanyName)
			}
		}
	}
	walk(companies, 0, "")
}

func sortCompanies(companies []company.CompanyInfo) {
	sort.SliceStable(companies, func(i, j int) bool {
		return companies[i].CompanyName < companies[j].CompanyName
	})
	for i := range companies {
		if len(companies[i].Subsidiaries) > 0 {
			sortCompanies(companies[i].Subsidiaries)
		}
	}
}

func generateCompanyInfoSummaryRows(companies []company.CompanyInfo) [][2]string {
	totalCompanies := 0
	totalDomains := 0
	totalSubsidiaries := 0
	totalOfficials := 0
	totalEmails := 0
	totalApps := 0
	totalApplets := 0

	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		totalCompanies++
		totalDomains += len(comp.Domains)
		totalSubsidiaries += len(comp.Subsidiaries)
		totalOfficials += len(comp.OfficialAccounts)
		totalEmails += len(comp.Emails)
		totalApps += len(comp.Apps)
		totalApplets += len(comp.Applets)
	})

	return [][2]string{
		{"生成时间", time.Now().Format("2006-01-02 15:04:05")},
		{"公司总数", fmt.Sprintf("%d", totalCompanies)},
		{"域名总数", fmt.Sprintf("%d", totalDomains)},
		{"子公司总数", fmt.Sprintf("%d", totalSubsidiaries)},
		{"公众号总数", fmt.Sprintf("%d", totalOfficials)},
		{"邮箱总数", fmt.Sprintf("%d", totalEmails)},
		{"APP 总数", fmt.Sprintf("%d", totalApps)},
		{"小程序总数", fmt.Sprintf("%d", totalApplets)},
	}
}

func isSafeCompanyReportImageSource(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:image/")
}

func renderCompanyReportMediaCell(value string, alt string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "<span class=\"muted\">-</span>"
	}
	if !isSafeCompanyReportImageSource(value) {
		return fmt.Sprintf("<div class=\"media-text\">%s</div>", htmlEscape(value))
	}
	escaped := htmlEscape(value)
	return fmt.Sprintf(
		"<a class=\"media-link\" href=\"%s\" target=\"_blank\" rel=\"noopener noreferrer\"><img class=\"media-image\" src=\"%s\" alt=\"%s\" loading=\"lazy\" referrerpolicy=\"no-referrer\"/></a>",
		escaped,
		escaped,
		htmlEscape(alt),
	)
}

func GenerateCompanyInfoWordReportFromDB(taskID string, taskName string) ([]byte, error) {
	task, err := LoadCompanyInfoTaskByID(taskID)
	if err != nil {
		return nil, err
	}
	companies, err := loadCompanyInfoTaskResult(taskID)
	if err != nil {
		return nil, err
	}
	return generateCompanyInfoWordReport(task, companies, taskName, false)
}

func GenerateMergedCompanyInfoWordReportFromDB(refs []CompanyTaskRef, reportName string) ([]byte, error) {
	_, companies, err := loadMergedCompanyInfos(refs)
	if err != nil {
		return nil, err
	}
	return generateMergedCompanyInfoWordReport(companies, reportName)
}

func generateCompanyInfoWordReport(task database.CompanyInfoTask, companies []company.CompanyInfo, taskName string, merged bool) ([]byte, error) {
	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}
	if strings.TrimSpace(taskName) == "" {
		if merged {
			taskName = "企业信息合并报告"
		} else {
			taskName = "企业信息报告"
		}
	}
	sortCompanies(companies)

	builder := newDocxReportBuilder()
	if merged {
		builder.addTitle(taskName)
	} else {
		builder.addTitle("企业信息报告")
		builder.addSubtitle(fmt.Sprintf("任务: %s", taskName))
		builder.addSubtitle(fmt.Sprintf("状态: %s", task.Status))
	}
	builder.addHeading1("任务概览")
	builder.addKeyValueTable(generateCompanyInfoSummaryRows(companies))

	builder.addHeading1("公司信息")
	companyRows := make([][]string, 0)
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		name := strings.Repeat("  ", level) + comp.CompanyName
		companyRows = append(companyRows, []string{
			name,
			comp.RegStatus,
			fmt.Sprintf("%d", len(comp.Domains)),
			fmt.Sprintf("%d", len(comp.Subsidiaries)),
			fmt.Sprintf("%d", len(comp.OfficialAccounts)),
			fmt.Sprintf("%d", len(comp.Apps)),
			fmt.Sprintf("%d", len(comp.Applets)),
		})
	})
	if len(companyRows) == 0 {
		builder.addParagraph("暂无企业信息结果")
	} else {
		builder.addTable([]string{"公司名称", "经营状态", "域名数", "子公司数", "公众号数", "APP数", "小程序数"}, companyRows)
	}

	builder.addHeading1("域名列表")
	domainRows := make([][]string, 0)
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, domain := range comp.Domains {
			domainRows = append(domainRows, []string{domain, comp.CompanyName})
		}
	})
	if len(domainRows) == 0 {
		builder.addParagraph("暂无域名结果")
	} else {
		builder.addTable([]string{"域名", "所属公司"}, domainRows)
	}

	builder.addHeading1("子公司列表")
	subsidiaryRows := make([][]string, 0)
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		if level == 0 {
			return
		}
		subsidiaryRows = append(subsidiaryRows, []string{
			comp.CompanyName,
			fmt.Sprintf("%d", level),
			parentCompany,
			fmt.Sprintf("%.0f%%", comp.Investment),
			comp.Amount,
			comp.RegStatus,
		})
	})
	if len(subsidiaryRows) == 0 {
		builder.addParagraph("暂无子公司结果")
	} else {
		builder.addTable([]string{"公司名称", "层级", "母公司", "投资比例", "注册资本", "经营状态"}, subsidiaryRows)
	}

	builder.addHeading1("邮箱列表")
	emailRows := make([][]string, 0)
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, email := range comp.Emails {
			emailRows = append(emailRows, []string{
				email.Address,
				email.Type,
				comp.CompanyName,
				strings.Join(email.Domains, ", "),
				fmt.Sprintf("%d", email.LeakageCount),
			})
		}
	})
	if len(emailRows) == 0 {
		builder.addParagraph("暂无邮箱结果")
	} else {
		builder.addTable([]string{"邮箱", "类型", "所属公司", "关联域名", "泄露次数"}, emailRows)
	}

	builder.addHeading1("公众号列表")
	officialRows := make([][]string, 0)
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, acc := range comp.OfficialAccounts {
			officialRows = append(officialRows, []string{acc.Name, acc.Numbers, acc.Introduction, comp.CompanyName})
		}
	})
	if len(officialRows) == 0 {
		builder.addParagraph("暂无公众号结果")
	} else {
		builder.addTable([]string{"名称", "账号", "简介", "所属公司"}, officialRows)
	}

	builder.addHeading1("APP 列表")
	appRows := make([][]string, 0)
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, app := range comp.Apps {
			appRows = append(appRows, []string{app.ServiceName, app.ServiceLicence, app.UnitName, app.UpdateRecordTime, comp.CompanyName})
		}
	})
	if len(appRows) == 0 {
		builder.addParagraph("暂无 APP 结果")
	} else {
		builder.addTable([]string{"服务名称", "许可证", "单位名称", "更新时间", "所属公司"}, appRows)
	}

	builder.addHeading1("小程序列表")
	appletRows := make([][]string, 0)
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, applet := range comp.Applets {
			appletRows = append(appletRows, []string{applet.ServiceName, applet.ServiceLicence, applet.UnitName, applet.UpdateRecordTime, comp.CompanyName})
		}
	})
	if len(appletRows) == 0 {
		builder.addParagraph("暂无小程序结果")
	} else {
		builder.addTable([]string{"服务名称", "许可证", "单位名称", "更新时间", "所属公司"}, appletRows)
	}

	return builder.build(taskName)
}

func generateMergedCompanyInfoWordReport(companies []company.CompanyInfo, reportName string) ([]byte, error) {
	return generateCompanyInfoWordReport(database.CompanyInfoTask{}, companies, reportName, true)
}

func GenerateCompanyInfoHTMLReportFromDB(taskID string, taskName string) ([]byte, error) {
	task, err := LoadCompanyInfoTaskByID(taskID)
	if err != nil {
		return nil, err
	}
	companies, err := loadCompanyInfoTaskResult(taskID)
	if err != nil {
		return nil, err
	}
	return generateCompanyInfoHTMLReport(task, companies, taskName, false)
}

func GenerateMergedCompanyInfoHTMLReportFromDB(refs []CompanyTaskRef, reportName string) ([]byte, error) {
	_, companies, err := loadMergedCompanyInfos(refs)
	if err != nil {
		return nil, err
	}
	return generateCompanyInfoHTMLReport(database.CompanyInfoTask{}, companies, reportName, true)
}

func generateCompanyInfoHTMLReport(task database.CompanyInfoTask, companies []company.CompanyInfo, taskName string, merged bool) ([]byte, error) {
	if strings.TrimSpace(taskName) == "" {
		if merged {
			taskName = "企业信息合并报告"
		} else {
			taskName = task.TaskName
		}
	}
	if strings.TrimSpace(taskName) == "" {
		taskName = "企业信息报告"
	}
	sortCompanies(companies)

	var sb strings.Builder
	sb.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"/>")
	sb.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"/>")
	sb.WriteString("<title>Company Info Report</title>")
	sb.WriteString("<style>body{font-family:ui-sans-serif,system-ui,-apple-system,Segoe UI,Roboto,Arial;line-height:1.6;padding:24px;background:#f7f9fc;color:#111}table{border-collapse:collapse;width:100%;background:#fff;margin-top:12px}th,td{border:1px solid #e5e7eb;padding:8px;vertical-align:top}th{background:#f3f4f6;text-align:left}.muted{color:#6b7280}h1{margin-bottom:8px}h2{margin-top:28px}.media-link{text-decoration:none}.media-image{display:block;max-width:96px;max-height:96px;min-width:64px;min-height:64px;object-fit:contain;border:1px solid #e5e7eb;border-radius:8px;background:#fff;padding:4px}.media-text{word-break:break-all;white-space:pre-wrap}</style></head><body>")
	sb.WriteString(fmt.Sprintf("<h1>%s</h1>", htmlEscape(taskName)))
	sb.WriteString(fmt.Sprintf("<div class=\"muted\">生成时间: %s</div>", time.Now().Format("2006-01-02 15:04:05")))
	if !merged {
		sb.WriteString(fmt.Sprintf("<div class=\"muted\">状态: %s</div>", htmlEscape(task.Status)))
	}

	sb.WriteString("<h2>任务概览</h2><table><tbody>")
	for _, item := range generateCompanyInfoSummaryRows(companies) {
		sb.WriteString(fmt.Sprintf("<tr><th>%s</th><td>%s</td></tr>", htmlEscape(item[0]), htmlEscape(item[1])))
	}
	sb.WriteString("</tbody></table>")

	sb.WriteString("<h2>公司信息</h2><table><thead><tr><th>公司名称</th><th>经营状态</th><th>域名数</th><th>子公司数</th><th>公众号数</th><th>APP数</th><th>小程序数</th></tr></thead><tbody>")
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		sb.WriteString("<tr>")
		sb.WriteString(fmt.Sprintf("<td>%s%s</td>", strings.Repeat("&nbsp;&nbsp;", level), htmlEscape(comp.CompanyName)))
		sb.WriteString(fmt.Sprintf("<td>%s</td>", htmlEscape(comp.RegStatus)))
		sb.WriteString(fmt.Sprintf("<td>%d</td>", len(comp.Domains)))
		sb.WriteString(fmt.Sprintf("<td>%d</td>", len(comp.Subsidiaries)))
		sb.WriteString(fmt.Sprintf("<td>%d</td>", len(comp.OfficialAccounts)))
		sb.WriteString(fmt.Sprintf("<td>%d</td>", len(comp.Apps)))
		sb.WriteString(fmt.Sprintf("<td>%d</td>", len(comp.Applets)))
		sb.WriteString("</tr>")
	})
	sb.WriteString("</tbody></table>")

	sb.WriteString("<h2>域名列表</h2><table><thead><tr><th>域名</th><th>所属公司</th></tr></thead><tbody>")
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, domain := range comp.Domains {
			sb.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>", htmlEscape(domain), htmlEscape(comp.CompanyName)))
		}
	})
	sb.WriteString("</tbody></table>")

	sb.WriteString("<h2>子公司列表</h2><table><thead><tr><th>公司名称</th><th>层级</th><th>母公司</th><th>投资比例</th><th>注册资本</th><th>经营状态</th></tr></thead><tbody>")
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		if level == 0 {
			return
		}
		sb.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%d</td><td>%s</td><td>%.0f%%</td><td>%s</td><td>%s</td></tr>",
			htmlEscape(comp.CompanyName), level, htmlEscape(parentCompany), comp.Investment, htmlEscape(comp.Amount), htmlEscape(comp.RegStatus)))
	})
	sb.WriteString("</tbody></table>")

	sb.WriteString("<h2>邮箱列表</h2><table><thead><tr><th>邮箱</th><th>类型</th><th>所属公司</th><th>关联域名</th><th>来源</th><th>标签</th><th>泄露次数</th></tr></thead><tbody>")
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, email := range comp.Emails {
			sb.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%d</td></tr>",
				htmlEscape(email.Address),
				htmlEscape(email.Type),
				htmlEscape(comp.CompanyName),
				htmlEscape(strings.Join(email.Domains, ", ")),
				htmlEscape(strings.Join(email.Sources, ", ")),
				htmlEscape(strings.Join(email.Tags, ", ")),
				email.LeakageCount,
			))
		}
	})
	sb.WriteString("</tbody></table>")

	sb.WriteString("<h2>公众号列表</h2><table><thead><tr><th>名称</th><th>账号</th><th>图标</th><th>二维码</th><th>简介</th><th>所属公司</th></tr></thead><tbody>")
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, acc := range comp.OfficialAccounts {
			sb.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>",
				htmlEscape(acc.Name),
				htmlEscape(acc.Numbers),
				renderCompanyReportMediaCell(acc.Logo, acc.Name+" 图标"),
				renderCompanyReportMediaCell(acc.Qrcode, acc.Name+" 二维码"),
				htmlEscape(acc.Introduction),
				htmlEscape(comp.CompanyName),
			))
		}
	})
	sb.WriteString("</tbody></table>")

	sb.WriteString("<h2>APP 列表</h2><table><thead><tr><th>服务名称</th><th>许可证</th><th>单位名称</th><th>更新时间</th><th>所属公司</th></tr></thead><tbody>")
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, app := range comp.Apps {
			sb.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>",
				htmlEscape(app.ServiceName), htmlEscape(app.ServiceLicence), htmlEscape(app.UnitName), htmlEscape(app.UpdateRecordTime), htmlEscape(comp.CompanyName)))
		}
	})
	sb.WriteString("</tbody></table>")

	sb.WriteString("<h2>小程序列表</h2><table><thead><tr><th>服务名称</th><th>许可证</th><th>单位名称</th><th>更新时间</th><th>所属公司</th></tr></thead><tbody>")
	walkCompanies(companies, func(comp company.CompanyInfo, level int, parentCompany string) {
		for _, applet := range comp.Applets {
			sb.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>",
				htmlEscape(applet.ServiceName), htmlEscape(applet.ServiceLicence), htmlEscape(applet.UnitName), htmlEscape(applet.UpdateRecordTime), htmlEscape(comp.CompanyName)))
		}
	})
	sb.WriteString("</tbody></table></body></html>")
	return []byte(sb.String()), nil
}

func GenerateCompanyInfoXLSXReportFromDB(taskID string, taskName string) ([]byte, error) {
	task, err := LoadCompanyInfoTaskByID(taskID)
	if err != nil {
		return nil, err
	}
	companies, err := loadCompanyInfoTaskResult(taskID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}
	f, err := buildCompanyInfoWorkbook(companies)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func GenerateMergedCompanyInfoXLSXReportFromDB(refs []CompanyTaskRef, reportName string) ([]byte, error) {
	_ = reportName
	_, companies, err := loadMergedCompanyInfos(refs)
	if err != nil {
		return nil, err
	}
	f, err := buildCompanyInfoWorkbook(companies)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func loadMergedCompanyInfos(refs []CompanyTaskRef) ([]database.CompanyInfoTask, []company.CompanyInfo, error) {
	tasks := make([]database.CompanyInfoTask, 0, len(refs))
	companies := make([]company.CompanyInfo, 0)
	for _, ref := range refs {
		id := strings.TrimSpace(ref.ID)
		if id == "" {
			continue
		}
		task, err := LoadCompanyInfoTaskByID(id)
		if err != nil {
			return nil, nil, err
		}
		if !isReportExportableTaskStatus(task.Status) {
			return nil, nil, fmt.Errorf("任务 %s 尚未完成，无法导出", task.ID)
		}
		items, err := loadCompanyInfoTaskResult(id)
		if err != nil {
			return nil, nil, err
		}
		tasks = append(tasks, task)
		companies = append(companies, items...)
	}
	if len(tasks) == 0 {
		return nil, nil, fmt.Errorf("未提供有效任务")
	}
	return tasks, companies, nil
}
