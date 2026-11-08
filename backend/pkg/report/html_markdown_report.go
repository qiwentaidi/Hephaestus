package report

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// HTMLReportStat is one card in the shared report summary grid.
type HTMLReportStat struct {
	Label string
	Value string
}

// RenderMarkdownReportHTML converts delivered Markdown into the same HTML
// shell used by the scan-task reports: shared hero, summary cards, section
// cards, tables, and code blocks. The report title is supplied by the caller
// and the duplicated leading Markdown H1 is removed.
func RenderMarkdownReportHTML(title, subtitle, markdown string, stats []HTMLReportStat) ([]byte, error) {
	var rendered bytes.Buffer
	markdown = stripReportMarkdownTitle(markdown)
	if strings.TrimSpace(markdown) == "" {
		return nil, fmt.Errorf("report content is empty")
	}
	renderer := goldmark.New(goldmark.WithExtensions(extension.GFM))
	if err := renderer.Convert([]byte(markdown), &rendered); err != nil {
		return nil, fmt.Errorf("render markdown report: %w", err)
	}

	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"/>")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"/>")
	b.WriteString("<title>")
	b.WriteString(htmlEscape(title))
	b.WriteString("</title><style>")
	b.WriteString(reportHTMLCoreStyle)
	b.WriteString(reportHTMLMarkdownBodyStyle)
	b.WriteString("</style></head><body><div class=\"container\">")
	b.WriteString("<div class=\"hero\"><h1>")
	b.WriteString(htmlEscape(title))
	b.WriteString("</h1>")
	if strings.TrimSpace(subtitle) != "" {
		b.WriteString("<p>")
		b.WriteString(htmlEscape(subtitle))
		b.WriteString("</p>")
	}
	b.WriteString("</div>")
	if len(stats) > 0 {
		b.WriteString("<div class=\"stats\">")
		for _, stat := range stats {
			b.WriteString("<div class=\"stat\"><div class=\"k\">")
			b.WriteString(htmlEscape(stat.Label))
			b.WriteString("</div><div class=\"v\">")
			b.WriteString(htmlEscape(stat.Value))
			b.WriteString("</div></div>")
		}
		b.WriteString("</div>")
	}
	b.WriteString("<section class=\"section report-body\">")
	b.Write(rendered.Bytes())
	b.WriteString("</section></div></body></html>")
	return []byte(b.String()), nil
}

func stripReportMarkdownTitle(markdown string) string {
	trimmed := strings.TrimLeft(markdown, "\r\n")
	if !strings.HasPrefix(trimmed, "# ") {
		return markdown
	}
	if index := strings.IndexAny(trimmed, "\r\n"); index >= 0 {
		return strings.TrimLeft(trimmed[index:], "\r\n")
	}
	return ""
}
