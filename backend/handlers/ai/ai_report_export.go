package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	aipkg "hephaestus-backend/pkg/ai"
	reportpkg "hephaestus-backend/pkg/report"
)

const (
	aiReportFormatMarkdown = "markdown"
	aiReportFormatHTML     = "html"
)

const (
	aiProjectReportMaxProjects = 200
	aiFindingReportMaxFindings = 2000
	aiProjectReportFindingsCap = 1000
)

type aiProjectReportRequest struct {
	ProjectIDs []string `json:"projectIds"`
	FileName   string   `json:"fileName"`
	Format     string   `json:"format"`
}

type aiFindingReportRequest struct {
	FindingIDs []string `json:"findingIds"`
	FileName   string   `json:"fileName"`
	Format     string   `json:"format"`
}

type aiProjectReportData struct {
	project       pentest.Project
	conversations []pentest.Conversation
	facts         []pentest.Fact
	edges         []pentest.FactEdge
	findings      []pentest.Finding
	retests       map[string][]pentest.FindingRetest
}

// ExportAIPentestProjectReportsHandler exports one Markdown report per project,
// merging multiple selected projects into a single document. It intentionally
// uses persisted pentest records only and never asks the model to synthesize
// new conclusions.
func ExportAIPentestProjectReportsHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	var req aiProjectReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid report request: " + err.Error()})
		return
	}
	projectIDs := uniqueAIReportIDs(req.ProjectIDs)
	if len(projectIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "projectIds is required"})
		return
	}
	if len(projectIDs) > aiProjectReportMaxProjects {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": fmt.Sprintf("at most %d projects can be exported at once", aiProjectReportMaxProjects)})
		return
	}
	format, err := parseAIReportFormat(req.Format)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	items := make([]aiProjectReportData, 0, len(projectIDs))
	for _, projectID := range projectIDs {
		item, loadErr := loadAIProjectReportData(c.Request.Context(), store, projectID)
		if loadErr != nil {
			c.JSON(aiReportHTTPStatus(loadErr), gin.H{"success": false, "error": loadErr.Error()})
			return
		}
		items = append(items, item)
	}
	title := aiProjectReportTitle(items)
	markdown := buildPentestMergedProjectReport(items)
	content, contentType, filename, err := buildAIReportPayload(
		title,
		aiReportSubtitle("项目级测试报告"),
		markdown,
		aiProjectReportStats(items),
		format,
		req.FileName,
		aiProjectReportFallbackName(items),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	writeAIReport(c, content, filename, contentType)
}

// ExportAIPentestFindingReportsHandler exports the checked findings as one
// Markdown report. Findings keep their original evidence, disposition status,
// and retest history; the export does not mutate them.
func ExportAIPentestFindingReportsHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	var req aiFindingReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid report request: " + err.Error()})
		return
	}
	findingIDs := uniqueAIReportIDs(req.FindingIDs)
	if len(findingIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "findingIds is required"})
		return
	}
	if len(findingIDs) > aiFindingReportMaxFindings {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": fmt.Sprintf("at most %d findings can be exported at once", aiFindingReportMaxFindings)})
		return
	}
	format, err := parseAIReportFormat(req.Format)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	findings := make([]pentest.Finding, 0, len(findingIDs))
	projectNames := make(map[string]string)
	for _, findingID := range findingIDs {
		finding, loadErr := store.GetFinding(c.Request.Context(), findingID)
		if loadErr != nil {
			c.JSON(aiReportHTTPStatus(loadErr), gin.H{"success": false, "error": fmt.Sprintf("finding %s is unavailable: %v", findingID, loadErr)})
			return
		}
		findings = append(findings, finding)
		if _, seen := projectNames[finding.ProjectID]; seen {
			continue
		}
		if project, projectErr := store.GetProject(c.Request.Context(), finding.ProjectID); projectErr == nil {
			projectNames[finding.ProjectID] = firstNonEmptyPentest(project.Name, project.ID)
		} else {
			projectNames[finding.ProjectID] = finding.ProjectID
		}
	}
	retests, err := loadPentestFindingRetests(c.Request.Context(), store, findings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	const title = "漏洞清单报告"
	markdown := buildPentestFindingsReport(title, findings, retests, projectNames)
	content, contentType, filename, err := buildAIReportPayload(
		title,
		aiReportSubtitle(fmt.Sprintf("仅包含用户勾选的 %d 条漏洞", len(findings))),
		markdown,
		aiFindingReportStats(findings),
		format,
		req.FileName,
		fmt.Sprintf("vulnerability-findings-%s", time.Now().Format("20060102")),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	writeAIReport(c, content, filename, contentType)
}

func loadAIProjectReportData(ctx context.Context, store *pentest.SQLStore, projectID string) (aiProjectReportData, error) {
	project, err := store.GetProject(ctx, projectID)
	if err != nil {
		return aiProjectReportData{}, err
	}
	conversations, err := store.ListConversationsByProject(ctx, project.ID, 1000)
	if err != nil {
		return aiProjectReportData{}, err
	}
	facts, err := store.ListFacts(ctx, project.ID, pentest.FactFilter{Limit: 1000, IncludeDeprecated: true})
	if err != nil {
		return aiProjectReportData{}, err
	}
	edges, err := store.ListEdges(ctx, project.ID)
	if err != nil {
		return aiProjectReportData{}, err
	}
	findings, err := store.ListFindings(ctx, project.ID, aiProjectReportFindingsCap)
	if err != nil {
		return aiProjectReportData{}, err
	}
	sortPentestFindingsForReport(findings)
	retests, err := loadPentestFindingRetests(ctx, store, findings)
	if err != nil {
		return aiProjectReportData{}, err
	}
	return aiProjectReportData{
		project:       project,
		conversations: conversations,
		facts:         facts,
		edges:         pentest.FilterEdgesForFacts(edges, facts),
		findings:      findings,
		retests:       retests,
	}, nil
}

func loadPentestFindingRetests(ctx context.Context, store *pentest.SQLStore, findings []pentest.Finding) (map[string][]pentest.FindingRetest, error) {
	out := make(map[string][]pentest.FindingRetest, len(findings))
	for _, finding := range findings {
		items, err := store.ListFindingRetests(ctx, finding.ID)
		if err != nil {
			return nil, err
		}
		out[finding.ID] = items
	}
	return out, nil
}

func uniqueAIReportIDs(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func aiReportHTTPStatus(err error) int {
	if errors.Is(err, sql.ErrNoRows) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func parseAIReportFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "md", "markdown":
		return aiReportFormatMarkdown, nil
	case "html", "htm":
		return aiReportFormatHTML, nil
	default:
		return "", fmt.Errorf("unsupported report format: %s", raw)
	}
}

func aiReportFilename(requested, fallback, format string) string {
	name := strings.TrimSpace(requested)
	if name == "" {
		name = strings.TrimSpace(fallback)
	}
	if name == "" {
		name = "ai-report"
	}
	name = strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "\"", "_", "\r", "_", "\n", "_", " ", "_",
	).Replace(name)
	lower := strings.ToLower(name)
	for _, extension := range []string{".markdown", ".md", ".html", ".htm"} {
		if strings.HasSuffix(lower, extension) {
			name = strings.TrimSpace(name[:len(name)-len(extension)])
			break
		}
	}
	if name == "" {
		name = "ai-report"
	}
	if format == aiReportFormatHTML {
		return name + ".html"
	}
	return name + ".md"
}

func aiReportSubtitle(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "生成时间: " + time.Now().Format("2006-01-02 15:04:05")
	}
	return prefix + " · 生成时间: " + time.Now().Format("2006-01-02 15:04:05")
}

func buildAIReportPayload(title, subtitle, markdown string, stats []reportpkg.HTMLReportStat, format, requestedName, fallbackName string) ([]byte, string, string, error) {
	filename := aiReportFilename(requestedName, fallbackName, format)
	if format == aiReportFormatHTML {
		content, err := reportpkg.RenderMarkdownReportHTML(title, subtitle, markdown, stats)
		if err != nil {
			return nil, "", "", err
		}
		return content, "text/html; charset=utf-8", filename, nil
	}
	return []byte(markdown), "text/markdown; charset=utf-8", filename, nil
}

func writeAIReport(c *gin.Context, content []byte, filename, contentType string) {
	safeFilename := strings.NewReplacer("\"", "_", "\r", "_", "\n", "_").Replace(filename)
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", safeFilename, url.PathEscape(safeFilename)))
	c.Data(http.StatusOK, contentType, content)
}

func aiProjectReportTitle(items []aiProjectReportData) string {
	if len(items) == 1 {
		return firstNonEmptyPentest(items[0].project.Name, items[0].project.ID, "未命名项目") + " 项目测试报告"
	}
	return "AI 工作台项目报告"
}

func aiProjectReportFallbackName(items []aiProjectReportData) string {
	if len(items) == 1 {
		return firstNonEmptyPentest(items[0].project.Name, items[0].project.ID, "ai-project-report")
	}
	return fmt.Sprintf("ai-workbench-project-reports-%d", len(items))
}

func aiProjectReportStats(items []aiProjectReportData) []reportpkg.HTMLReportStat {
	conversations := 0
	findings := 0
	for _, item := range items {
		conversations += len(item.conversations)
		findings += len(item.findings)
	}
	stats := aiFindingStats(findings, countAIReportSeverity(items, "critical"), countAIReportSeverity(items, "high"))
	stats = append([]reportpkg.HTMLReportStat{{Label: "来源会话", Value: fmt.Sprintf("%d", conversations)}}, stats...)
	if len(items) > 1 {
		stats = append([]reportpkg.HTMLReportStat{{Label: "项目", Value: fmt.Sprintf("%d", len(items))}}, stats...)
	}
	return stats
}

func aiFindingReportStats(findings []pentest.Finding) []reportpkg.HTMLReportStat {
	return aiFindingStats(len(findings), countFindingSeverity(findings, "critical"), countFindingSeverity(findings, "high"))
}

func aiFindingStats(total, critical, high int) []reportpkg.HTMLReportStat {
	return []reportpkg.HTMLReportStat{
		{Label: "漏洞记录", Value: fmt.Sprintf("%d", total)},
		{Label: "严重", Value: fmt.Sprintf("%d", critical)},
		{Label: "高危", Value: fmt.Sprintf("%d", high)},
	}
}

func countAIReportSeverity(items []aiProjectReportData, severity string) int {
	total := 0
	for _, item := range items {
		total += countFindingSeverity(item.findings, severity)
	}
	return total
}

func countFindingSeverity(findings []pentest.Finding, severity string) int {
	total := 0
	for _, finding := range findings {
		if strings.EqualFold(strings.TrimSpace(finding.Severity), severity) {
			total++
		}
	}
	return total
}

func buildPentestMergedProjectReport(items []aiProjectReportData) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(aiProjectReportTitle(items))
	b.WriteString("\n\n")
	if len(items) == 1 {
		writePentestProjectReportSections(&b, items[0], 2)
		return b.String()
	}

	totalFindings := 0
	totalConversations := 0
	for _, item := range items {
		totalFindings += len(item.findings)
		totalConversations += len(item.conversations)
	}
	b.WriteString("## 报告信息\n\n")
	writePentestMarkdownKeyValueTable(&b, [][2]string{
		{"项目数", fmt.Sprintf("%d 个", len(items))},
		{"来源会话", fmt.Sprintf("%d 个", totalConversations)},
		{"漏洞记录", fmt.Sprintf("%d 条", totalFindings)},
		{"生成时间", time.Now().Format(time.RFC3339)},
	})
	b.WriteString("## 项目目录\n\n")
	b.WriteString("| # | 项目 | 来源会话 | 漏洞记录 | 更新时间 |\n| ---: | --- | ---: | ---: | --- |\n")
	for index, item := range items {
		b.WriteString(fmt.Sprintf("| %d | %s | %d | %d | %s |\n",
			index+1,
			reportMarkdownCell(firstNonEmptyPentest(item.project.Name, item.project.ID, "未命名项目")),
			len(item.conversations),
			len(item.findings),
			formatAIPentestReportTime(item.project.UpdatedAt),
		))
	}
	b.WriteString("\n")
	for index, item := range items {
		b.WriteString(fmt.Sprintf("## %d. %s\n\n", index+1, firstNonEmptyPentest(item.project.Name, item.project.ID, "未命名项目")))
		writePentestProjectReportSections(&b, item, 3)
	}
	return b.String()
}

func writePentestProjectReportSections(b *strings.Builder, item aiProjectReportData, level int) {
	scope := "未指定"
	if values := parseScopeJSON(item.project.ScopeJSON); len(values) > 0 {
		scope = strings.Join(values, ", ")
	}
	writePentestReportHeading(b, level, "报告信息")
	writePentestMarkdownKeyValueTable(b, [][2]string{
		{"项目", firstNonEmptyPentest(item.project.Name, item.project.ID, "未命名项目")},
		{"项目 ID", item.project.ID},
		{"描述", firstNonEmptyPentest(item.project.Description, "未填写")},
		{"范围", scope},
		{"来源会话", fmt.Sprintf("%d 个", len(item.conversations))},
		{"漏洞记录", fmt.Sprintf("%d 条", len(item.findings))},
		{"生成时间", time.Now().Format(time.RFC3339)},
	})

	writePentestReportHeading(b, level, "执行摘要")
	if description := strings.TrimSpace(item.project.Description); description != "" {
		b.WriteString(description)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("本项目共汇总 %d 个来源会话、%d 条已持久化漏洞记录。%s\n\n", len(item.conversations), len(item.findings), pentestFindingRiskSentence(item.findings)))
	b.WriteString("本报告仅使用项目内已持久化的事实、漏洞、复测记录和工具证据，不改变任何漏洞处置状态。\n\n")

	writePentestReportHeading(b, level, "风险总览")
	writePentestFindingRiskOverview(b, item.findings)

	writePentestReportHeading(b, level, "漏洞详情")
	if len(item.findings) == 0 {
		b.WriteString("当前项目没有已持久化的漏洞记录。\n\n")
	}
	conversationTitles := make(map[string]string, len(item.conversations))
	for _, conversation := range item.conversations {
		conversationTitles[conversation.ID] = firstNonEmptyPentest(conversation.Title, conversation.Target, conversation.ID)
	}
	for index, finding := range item.findings {
		writePentestFindingDetail(b, level+1, index+1, finding, item.retests[finding.ID], firstNonEmptyPentest(item.project.Name, item.project.ID), conversationTitles[finding.ConversationID])
	}

	writePentestReportHeading(b, level, "来源会话")
	if len(item.conversations) == 0 {
		b.WriteString("当前项目没有来源会话记录。\n\n")
	} else {
		findingCounts := make(map[string]int, len(item.conversations))
		for _, finding := range item.findings {
			findingCounts[finding.ConversationID]++
		}
		b.WriteString("| # | 会话 | 目标 | 状态 | 更新时间 | 漏洞记录 |\n| ---: | --- | --- | --- | --- | ---: |\n")
		for index, conversation := range item.conversations {
			b.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %d |\n",
				index+1,
				reportMarkdownCell(firstNonEmptyPentest(conversation.Title, conversation.Target, conversation.ID)),
				reportMarkdownCell(firstNonEmptyPentest(conversation.Target, "未指定")),
				reportMarkdownCell(firstNonEmptyPentest(conversation.Status, "unknown")),
				formatAIPentestReportTime(conversation.UpdatedAt),
				findingCounts[conversation.ID],
			))
		}
		b.WriteString("\n")
	}

	writePentestReportHeading(b, level, "黑板事实")
	writePentestProjectFacts(b, item.facts, item.edges)

	writePentestReportHeading(b, level, "覆盖边界")
	b.WriteString("本报告汇总项目内全部会话中已持久化的事实、漏洞记录、复测记录和工具执行证据。未观察、未执行或缺少证据的路径不视为已验证；报告生成不会改变漏洞处置状态，也不会新增测试结论。\n")
	if len(item.findings) >= aiProjectReportFindingsCap {
		b.WriteString(fmt.Sprintf("\n注意：单次项目报告最多汇总 %d 条漏洞记录，本报告可能未包含更早的记录。\n", aiProjectReportFindingsCap))
	}
}

func buildPentestFindingsReport(title string, findings []pentest.Finding, retests map[string][]pentest.FindingRetest, projectNames map[string]string) string {
	sortPentestFindingsForReport(findings)
	sessionIDs := make(map[string]bool, len(findings))
	for _, finding := range findings {
		if finding.ConversationID != "" {
			sessionIDs[finding.ConversationID] = true
		}
	}
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(firstNonEmptyPentest(title, "漏洞清单报告"))
	b.WriteString("\n\n")
	b.WriteString("## 报告信息\n\n")
	writePentestMarkdownKeyValueTable(&b, [][2]string{
		{"漏洞数量", fmt.Sprintf("%d 条", len(findings))},
		{"来源会话", fmt.Sprintf("%d 个", len(sessionIDs))},
		{"生成时间", time.Now().Format(time.RFC3339)},
	})
	b.WriteString("## 风险总览\n\n")
	writePentestFindingRiskOverview(&b, findings)
	b.WriteString("## 漏洞详情\n\n")
	for index, finding := range findings {
		writePentestFindingDetail(&b, 3, index+1, finding, retests[finding.ID], firstNonEmptyPentest(projectNames[finding.ProjectID], finding.ProjectID), finding.ConversationTitle)
	}
	b.WriteString("## 覆盖边界\n\n")
	b.WriteString("本报告仅包含用户勾选的漏洞记录，并保留各记录的原始证据、处置状态和复测历史。未勾选的漏洞不会出现在报告中；导出不会改变漏洞处置状态。\n")
	return b.String()
}

func writePentestFindingDetail(b *strings.Builder, level, index int, finding pentest.Finding, retests []pentest.FindingRetest, projectName, conversationTitle string) {
	b.WriteString(strings.Repeat("#", level))
	b.WriteString(fmt.Sprintf(" %d. %s\n\n", index, firstNonEmptyPentest(finding.Title, finding.ID, "未命名漏洞")))
	writePentestMarkdownKeyValueTable(b, [][2]string{
		{"漏洞 ID", finding.ID},
		{"所属项目", firstNonEmptyPentest(projectName, finding.ProjectID, "未记录")},
		{"来源会话", firstNonEmptyPentest(conversationTitle, finding.ConversationTitle, finding.ConversationID, "未记录")},
		{"严重性", firstNonEmptyPentest(finding.Severity, "未评级")},
		{"状态", firstNonEmptyPentest(finding.Status, "未记录")},
		{"验证状态", finding.VerificationStatus},
		{"类型", finding.VulnerabilityType},
		{"目标", finding.Target},
		{"发现时间", formatAIPentestReportTime(finding.CreatedAt)},
		{"更新时间", formatAIPentestReportTime(finding.UpdatedAt)},
	})
	for _, section := range []struct{ title, value string }{
		{"前置条件", finding.Preconditions},
		{"复现步骤", finding.ReproductionSteps},
		{"原始证据", finding.Evidence},
		{"影响", finding.Impact},
		{"修复建议", finding.Recommendation},
		{"复测备注", finding.RetestNotes},
	} {
		if strings.TrimSpace(section.value) == "" {
			continue
		}
		b.WriteString("**")
		b.WriteString(section.title)
		b.WriteString("**\n\n")
		b.WriteString(section.value)
		b.WriteString("\n\n")
	}
	if len(retests) > 0 {
		b.WriteString("**复测记录**\n\n")
		for _, retest := range retests {
			b.WriteString(fmt.Sprintf("- `%s`（%s，%s）：%s\n",
				retest.CreatedAt.Format(time.RFC3339),
				firstNonEmptyPentest(retest.ConversationID, "未知会话"),
				firstNonEmptyPentest(retest.Result, "unknown"),
				firstNonEmptyPentest(retest.Summary, "无摘要"),
			))
			if evidence := strings.TrimSpace(retest.Evidence); evidence != "" {
				b.WriteString("  - 证据：")
				b.WriteString(evidence)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
}

func writePentestProjectFacts(b *strings.Builder, facts []pentest.Fact, edges []pentest.FactEdge) {
	active := make([]pentest.Fact, 0, len(facts))
	deprecated := 0
	for _, fact := range facts {
		if strings.EqualFold(strings.TrimSpace(fact.Confidence), pentest.ConfidenceDeprecated) {
			deprecated++
			continue
		}
		active = append(active, fact)
	}
	if len(active) == 0 {
		b.WriteString("当前项目尚未记录有效黑板事实。\n\n")
		return
	}
	b.WriteString("| 事实键 | 分类 | 置信度 | 摘要 | 来源会话 |\n| --- | --- | --- | --- | --- |\n")
	for _, fact := range active {
		b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
			reportMarkdownCell(fact.FactKey),
			reportMarkdownCell(firstNonEmptyPentest(fact.Category, "未分类")),
			reportMarkdownCell(firstNonEmptyPentest(fact.Confidence, "未标注")),
			reportMarkdownCell(firstNonEmptyPentest(fact.Summary, "无摘要")),
			reportMarkdownCell(firstNonEmptyPentest(fact.SourceConversationID, "未记录")),
		))
	}
	b.WriteString("\n")
	if deprecated > 0 {
		b.WriteString(fmt.Sprintf("已忽略 %d 条已废弃事实。\n\n", deprecated))
	}
	if len(edges) > 0 {
		b.WriteString("**事实关系**\n\n")
		for _, edge := range edges {
			b.WriteString(fmt.Sprintf("- `%s` --[%s]--> `%s`\n",
				reportMarkdownCell(edge.SourceFactKey),
				reportMarkdownCell(firstNonEmptyPentest(edge.EdgeType, "related")),
				reportMarkdownCell(edge.TargetFactKey),
			))
		}
		b.WriteString("\n")
	}
}

func writePentestFindingRiskOverview(b *strings.Builder, findings []pentest.Finding) {
	if len(findings) == 0 {
		b.WriteString("暂无漏洞记录。\n\n")
		return
	}
	severityCounts := make(map[string]int)
	statusCounts := make(map[string]int)
	for _, finding := range findings {
		severity := strings.ToLower(strings.TrimSpace(finding.Severity))
		if severity == "" {
			severity = "未评级"
		}
		severityCounts[severity]++
		status := strings.TrimSpace(finding.Status)
		if status == "" {
			status = "未记录"
		}
		statusCounts[status]++
	}
	b.WriteString("**按严重性**\n\n")
	b.WriteString("| 严重性 | 数量 |\n| --- | ---: |\n")
	writtenSeverities := make(map[string]bool)
	for _, severity := range []string{"critical", "high", "medium", "low", "info", "未评级"} {
		if count := severityCounts[severity]; count > 0 {
			b.WriteString(fmt.Sprintf("| %s | %d |\n", reportMarkdownCell(severity), count))
			writtenSeverities[severity] = true
		}
	}
	remainingSeverities := make([]string, 0)
	for severity := range severityCounts {
		if !writtenSeverities[severity] {
			remainingSeverities = append(remainingSeverities, severity)
		}
	}
	sort.Strings(remainingSeverities)
	for _, severity := range remainingSeverities {
		b.WriteString(fmt.Sprintf("| %s | %d |\n", reportMarkdownCell(severity), severityCounts[severity]))
	}
	b.WriteString("\n**按状态**\n\n")
	b.WriteString("| 状态 | 数量 |\n| --- | ---: |\n")
	written := make(map[string]bool)
	for _, status := range []string{"open", "in_progress", "confirmed", "fixed", "false_positive", "accepted_risk", "ignored", "未记录"} {
		if count := statusCounts[status]; count > 0 {
			b.WriteString(fmt.Sprintf("| %s | %d |\n", reportMarkdownCell(status), count))
			written[status] = true
		}
	}
	remaining := make([]string, 0)
	for status := range statusCounts {
		if !written[status] {
			remaining = append(remaining, status)
		}
	}
	sort.Strings(remaining)
	for _, status := range remaining {
		b.WriteString(fmt.Sprintf("| %s | %d |\n", reportMarkdownCell(status), statusCounts[status]))
	}
	b.WriteString("\n")
}

func writePentestReportHeading(b *strings.Builder, level int, title string) {
	if level < 1 {
		level = 1
	}
	b.WriteString(strings.Repeat("#", level))
	b.WriteString(" ")
	b.WriteString(title)
	b.WriteString("\n\n")
}

func writePentestMarkdownKeyValueTable(b *strings.Builder, rows [][2]string) {
	b.WriteString("| 字段 | 值 |\n| --- | --- |\n")
	for _, row := range rows {
		value := strings.TrimSpace(row[1])
		if value == "" {
			continue
		}
		b.WriteString("| ")
		b.WriteString(reportMarkdownCell(row[0]))
		b.WriteString(" | ")
		b.WriteString(reportMarkdownCell(value))
		b.WriteString(" |\n")
	}
	b.WriteString("\n")
}

func pentestFindingRiskSentence(findings []pentest.Finding) string {
	if len(findings) == 0 {
		return "当前没有可汇总的漏洞记录。"
	}
	counts := make(map[string]int, 6)
	for _, finding := range findings {
		severity := strings.ToLower(strings.TrimSpace(finding.Severity))
		if severity == "" {
			severity = "未评级"
		}
		counts[severity]++
	}
	parts := make([]string, 0, 6)
	written := make(map[string]bool)
	for _, severity := range []string{"critical", "high", "medium", "low", "info", "未评级"} {
		if count := counts[severity]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d 条", severity, count))
			written[severity] = true
		}
	}
	remaining := make([]string, 0)
	for severity := range counts {
		if !written[severity] {
			remaining = append(remaining, severity)
		}
	}
	sort.Strings(remaining)
	for _, severity := range remaining {
		parts = append(parts, fmt.Sprintf("%s %d 条", severity, counts[severity]))
	}
	return "风险分布：" + strings.Join(parts, "，") + "。"
}

func sortPentestFindingsForReport(findings []pentest.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		left, right := pentestReportSeverityRank(findings[i].Severity), pentestReportSeverityRank(findings[j].Severity)
		if left != right {
			return left > right
		}
		if !findings[i].UpdatedAt.Equal(findings[j].UpdatedAt) {
			return findings[i].UpdatedAt.After(findings[j].UpdatedAt)
		}
		return findings[i].ID > findings[j].ID
	})
}

func pentestReportSeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func formatAIPentestReportTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
