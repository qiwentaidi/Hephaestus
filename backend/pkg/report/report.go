package report

import (
	"fmt"
	"hephaestus-backend/pkg/types"
	"strings"
)

// GenerateReport 生成报告（兼容旧接口）
func GenerateReport(Fingerprints []types.InfoResult, POCs []types.VulnerabilityInfo) string {
	return defaultHeader() + reportBody(Fingerprints, POCs) + defaultFooter()
}

func reportBody(Fingerprints []types.InfoResult, POCs []types.VulnerabilityInfo) string {
	var sb strings.Builder

	// Fingerprints
	for i, fp := range Fingerprints {
		statusClass := "ok"
		if fp.StatusCode >= 400 && fp.StatusCode < 500 {
			statusClass = "warn"
		} else if fp.StatusCode >= 500 {
			statusClass = "bad"
		}

		// 指纹信息
		fpTags := ""
		if len(fp.Fingerprints) > 0 {
			fpTags += `<div class="fp-tags">`
			for _, f := range fp.Fingerprints {
				fpTags += fmt.Sprintf(`<span class="fp-tag">%s</span>`, f)
			}
			fpTags += `</div>`
		}

		title := fp.Title
		if title == "" {
			title = "-"
		}

		sb.WriteString(fmt.Sprintf(`
<div class="fp-row">
  <div class="fp-id">%d</div>
  <a href="%s" target="_blank">%s</a>
  <span class="badge %s">%d</span>
  <span>%d</span>
  <span>%s</span>
  %s
  %s
</div>`,
			i+1, xssfilter(fp.URL), xssfilter(fp.URL), statusClass, fp.StatusCode, fp.Length, xssfilter(title), "", fpTags))
	}

	sb.WriteString(`</div><div id="vuln" class="tab-content">`)

	// Vulnerabilities
	// Vulnerabilities
	for i, poc := range POCs {
		severityClass := strings.ToLower(poc.Severity)
		fullurl := xssfilter(poc.URL) // 安全 URL

		vulnName := poc.Name
		if vulnName == "" {
			vulnName = poc.ID
		}

		sb.WriteString(fmt.Sprintf(`
<table>
  <thead class="vuln-head">
    <tr>
      <td class="vuln">#%d %s</td>
      <td class="severity"><span class="security %s">%s</span></td>
      <td class="url"><a href="%s" target="_blank">%s</a></td>
    </tr>
  </thead>
  <tbody style="display:none;">
    <tr>
      <td colspan="3">
        <b>漏洞名称:</b> %s &nbsp;&nbsp; <b>严重程度:</b> %s`,
			i+1, xssfilter(poc.ID), severityClass, poc.Severity, fullurl, fullurl, xssfilter(vulnName), poc.Severity))

		if poc.Extract != "" {
			sb.WriteString("<br/><b>提取信息:</b> " + xssfilter(poc.Extract))
		}
		if poc.Description != "" {
			sb.WriteString("<br/><b>描述:</b> " + xssfilter(poc.Description))
		}
		if poc.Reference != "" {
			refs := strings.Split(poc.Reference, "\n")
			if len(refs) == 1 {
				refs = strings.Split(poc.Reference, ",")
			}
			sb.WriteString("<br/><b>参考链接:</b>")
			for _, r := range refs {
				r = strings.TrimSpace(r)
				if r != "" {
					sb.WriteString(fmt.Sprintf(`<br/> - <a href="%s" target="_blank">%s</a>`, xssfilter(r), xssfilter(r)))
				}
			}
		}

		// 请求/响应显示
		sb.WriteString(fmt.Sprintf(`
      </td>
    </tr>
    <tr>
      <td colspan="3" style="background:var(--panel-alt);">
        <div class="clr">
          <div class="request w50">
            <div class="reqresp-header">
              <div class="actions">
                <div class="toggle" onclick="$(this).closest('.request').toggleClass('w100').toggleClass('w50').siblings('.response').toggle();">Request</div>
              </div>
              <div class="time btn-mini" onclick="copyXmpContent($(this));">Copy</div>
            </div>
            <xmp>%s</xmp>
          </div>
          <div class="response w50">
            <div class="reqresp-header">
              <div class="actions">
                <div class="toggle" onclick="$(this).closest('.response').toggleClass('w100').toggleClass('w50').siblings('.request').toggle();">Response</div>
              </div>
              <div class="time">%s</div>
            </div>
            <xmp>%s</xmp>
          </div>
        </div>
      </td>
    </tr>
  </tbody>
</table>`, xssfilter(poc.Request), poc.ResponseTime, xssfilter(poc.Response)))
	}

	return sb.String()
}
