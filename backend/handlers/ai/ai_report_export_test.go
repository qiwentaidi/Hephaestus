package ai

import (
	"strings"
	"testing"
	"time"

	"hephaestus-backend/lib/pentest"
	reportpkg "hephaestus-backend/pkg/report"
)

func TestBuildPentestProjectReportAggregatesSessions(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	item := aiProjectReportData{
		project: pentest.Project{
			ID: "project-1", Name: "Acme", Description: "External assessment",
			ScopeJSON: `["a.example.com","b.example.com"]`, UpdatedAt: now,
		},
		conversations: []pentest.Conversation{
			{ID: "conversation-1", ProjectID: "project-1", Title: "Web app", Target: "a.example.com", Status: "completed", UpdatedAt: now},
			{ID: "conversation-2", ProjectID: "project-1", Title: "API", Target: "b.example.com", Status: "running", UpdatedAt: now},
		},
		facts: []pentest.Fact{
			{ID: "fact-1", ProjectID: "project-1", FactKey: "target:a.example.com", Category: "target", Summary: "admin panel exposed", Confidence: "confirmed"},
		},
		findings: []pentest.Finding{
			{ID: "finding-1", ProjectID: "project-1", ConversationID: "conversation-1", Title: "SQL injection", Severity: "critical", Status: "open", VerificationStatus: "verified", Target: "a.example.com", Evidence: "payload response", CreatedAt: now, UpdatedAt: now},
			{ID: "finding-2", ProjectID: "project-1", ConversationID: "conversation-2", Title: "Stored XSS", Severity: "medium", Status: "fixed", Target: "b.example.com", Evidence: "script executed", CreatedAt: now, UpdatedAt: now},
		},
		retests: map[string][]pentest.FindingRetest{
			"finding-1": {{ID: "retest-1", FindingID: "finding-1", ConversationID: "conversation-1", Result: "still_vulnerable", Summary: "still exploitable after patch", CreatedAt: now}},
		},
	}

	report := buildPentestMergedProjectReport([]aiProjectReportData{item})
	for _, expected := range []string{
		"# Acme 项目测试报告",
		"## 报告信息",
		"## 执行摘要",
		"## 风险总览",
		"## 漏洞详情",
		"## 来源会话",
		"## 黑板事实",
		"## 覆盖边界",
		"SQL injection",
		"Stored XSS",
		"Web app",
		"API",
		"still exploitable after patch",
		"admin panel exposed",
		"a.example.com, b.example.com",
	} {
		if !strings.Contains(report, expected) {
			t.Fatalf("project report is missing %q:\n%s", expected, report)
		}
	}
	if strings.Contains(report, "# AI 工作台项目报告") {
		t.Fatalf("single-project export should not use the merged title:\n%s", report)
	}
}

func TestBuildPentestMergedProjectReportKeepsEveryProject(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	items := []aiProjectReportData{
		{
			project:       pentest.Project{ID: "project-1", Name: "Acme", UpdatedAt: now},
			conversations: []pentest.Conversation{{ID: "conversation-1", Title: "Web app", Target: "a.example.com", UpdatedAt: now}},
			findings:      []pentest.Finding{{ID: "finding-1", ProjectID: "project-1", ConversationID: "conversation-1", Title: "SQL injection", Severity: "high", Status: "open", CreatedAt: now, UpdatedAt: now}},
			retests:       map[string][]pentest.FindingRetest{},
		},
		{
			project:       pentest.Project{ID: "project-2", Name: "Globex", UpdatedAt: now},
			conversations: []pentest.Conversation{{ID: "conversation-2", Title: "Portal", Target: "b.example.com", UpdatedAt: now}},
			findings:      []pentest.Finding{{ID: "finding-2", ProjectID: "project-2", ConversationID: "conversation-2", Title: "Open redirect", Severity: "low", Status: "fixed", CreatedAt: now, UpdatedAt: now}},
			retests:       map[string][]pentest.FindingRetest{},
		},
	}

	report := buildPentestMergedProjectReport(items)
	for _, expected := range []string{
		"# AI 工作台项目报告",
		"## 项目目录",
		"## 1. Acme",
		"## 2. Globex",
		"SQL injection",
		"Open redirect",
		"| 项目数 | 2 个 |",
		"| 来源会话 | 2 个 |",
		"| 漏洞记录 | 2 条 |",
	} {
		if !strings.Contains(report, expected) {
			t.Fatalf("merged project report is missing %q:\n%s", expected, report)
		}
	}
}

func TestBuildPentestFindingsReportContainsOnlySelectedFindings(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	findings := []pentest.Finding{
		{ID: "finding-1", ProjectID: "project-1", ConversationID: "conversation-1", ConversationTitle: "Web app", Title: "SQL injection", Severity: "critical", Status: "open", Target: "a.example.com", Evidence: "payload", CreatedAt: now, UpdatedAt: now},
	}
	retests := map[string][]pentest.FindingRetest{
		"finding-1": {{ID: "retest-1", FindingID: "finding-1", ConversationID: "conversation-1", Result: "fixed", Summary: "no longer reproducible", CreatedAt: now}},
	}

	report := buildPentestFindingsReport("漏洞清单报告", findings, retests, map[string]string{"project-1": "Acme"})
	for _, expected := range []string{
		"# 漏洞清单报告",
		"SQL injection",
		"Web app",
		"Acme",
		"no longer reproducible",
		"| 漏洞数量 | 1 条 |",
		"未勾选的漏洞不会出现在报告中",
	} {
		if !strings.Contains(report, expected) {
			t.Fatalf("selected finding report is missing %q:\n%s", expected, report)
		}
	}
	if strings.Contains(report, "Unselected finding") {
		t.Fatalf("selected finding report included an unselected record:\n%s", report)
	}
}

func TestAIReportFilenameSanitizesAndUsesRequestedFormat(t *testing.T) {
	if got := aiReportFilename("客户/A:报告", "fallback", aiReportFormatMarkdown); got != "客户_A_报告.md" {
		t.Fatalf("unexpected filename: %q", got)
	}
	if got := aiReportFilename("report.MD", "fallback", aiReportFormatMarkdown); got != "report.md" {
		t.Fatalf("markdown extension should be normalized: %q", got)
	}
	if got := aiReportFilename("report.md", "fallback", aiReportFormatHTML); got != "report.html" {
		t.Fatalf("html export should replace the markdown extension: %q", got)
	}
	if got := aiReportFilename("", "fallback", aiReportFormatMarkdown); got != "fallback.md" {
		t.Fatalf("fallback filename should receive the extension: %q", got)
	}
}

func TestBuildAIReportPayloadPreservesMarkdownFormat(t *testing.T) {
	markdown := "# 漏洞清单报告\n\n## 漏洞详情\n\n### 1. SQL injection\n"
	content, contentType, filename, err := buildAIReportPayload(
		"漏洞清单报告",
		"仅包含用户勾选的 1 条漏洞",
		markdown,
		nil,
		aiReportFormatMarkdown,
		"",
		"vulnerability-findings",
	)
	if err != nil {
		t.Fatalf("buildAIReportPayload() error = %v", err)
	}
	if contentType != "text/markdown; charset=utf-8" {
		t.Fatalf("contentType = %q, want text/markdown", contentType)
	}
	if filename != "vulnerability-findings.md" {
		t.Fatalf("filename = %q, want vulnerability-findings.md", filename)
	}
	if string(content) != markdown {
		t.Fatalf("markdown export changed the report body:\n%s", content)
	}
}

func TestBuildAIReportPayloadUsesSharedHTMLReportShell(t *testing.T) {
	markdown := "# 耀出行 项目测试报告\n\n## 报告信息\n\n| 字段 | 值 |\n| --- | --- |\n| 项目 | 耀出行 |\n\n## 漏洞详情\n\n### 1. 示例漏洞\n\n**原始证据**\n\n```text\n<script>alert(1)</script>\n```\n"
	stats := []reportpkg.HTMLReportStat{{Label: "来源会话", Value: "2"}, {Label: "漏洞记录", Value: "1"}}
	content, contentType, filename, err := buildAIReportPayload(
		"耀出行 项目测试报告",
		"项目级测试报告 · 生成时间: 2026-10-21 23:30:00",
		markdown,
		stats,
		aiReportFormatHTML,
		"",
		"耀出行",
	)
	if err != nil {
		t.Fatalf("buildAIReportPayload() error = %v", err)
	}
	if contentType != "text/html; charset=utf-8" {
		t.Fatalf("contentType = %q, want text/html", contentType)
	}
	if filename != "耀出行.html" {
		t.Fatalf("filename = %q, want 耀出行.html", filename)
	}
	html := string(content)
	for _, expected := range []string{
		"<!doctype html>",
		"class=\"hero\"",
		"<h1>耀出行 项目测试报告</h1>",
		"class=\"stats\"",
		"class=\"stat\"",
		"class=\"section report-body\"",
		"<h2>报告信息</h2>",
		"<table>",
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("html report is missing %q:\n%s", expected, html)
		}
	}
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatalf("html report rendered raw script content:\n%s", html)
	}
	if strings.Count(html, "<h1>耀出行 项目测试报告</h1>") != 1 {
		t.Fatalf("html report duplicated the leading title:\n%s", html)
	}
}
