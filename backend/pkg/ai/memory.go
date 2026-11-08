package ai

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

// WorkbenchTargetContext 是从 Hephaestus 既有扫描表归并出的 AI 可读上下文。
type WorkbenchTargetContext struct {
	Refs               []string `json:"refs"`
	FingerprintCount   int64    `json:"fingerprintCount"`
	VulnerabilityCount int64    `json:"vulnerabilityCount"`
	JSAssetCount       int64    `json:"jsAssetCount"`
	DirResultCount     int64    `json:"dirResultCount"`
	PortCount          int64    `json:"portCount"`
	Highlights         []string `json:"highlights"`
}

// CollectWorkbenchDerivedHTTPURLs derives observable HTTP(S) URLs from the
// target, scope, and existing scan evidence. It is used after asset discovery
// so CIDR/IP/domain tasks can hand confirmed Web entrances to the read-only
// HTTP observer instead of repeatedly observing the raw network target.
func CollectWorkbenchDerivedHTTPURLs(db *gorm.DB, target string, scope []string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 20
	}
	seen := make(map[string]bool, limit)
	urls := make([]string, 0, limit)
	add := func(value string) {
		normalized := NormalizeWorkbenchURLCandidate(value)
		if normalized == "" || seen[normalized] || len(urls) >= limit {
			return
		}
		seen[normalized] = true
		urls = append(urls, normalized)
	}

	for _, item := range normalizeWorkbenchScanTargets(target, scope) {
		add(item)
	}
	if len(urls) >= limit || db == nil {
		return urls, nil
	}

	portURLs, err := collectWorkbenchPortHTTPURLs(db, target, scope, limit-len(urls))
	if err != nil {
		return urls, err
	}
	for _, item := range portURLs {
		add(item)
	}
	if len(urls) >= limit {
		return urls, nil
	}

	refs := buildWorkbenchTargetRefs(target, scope)
	if len(refs) > 0 {
		var fingerprints []database.FingerprintInfo
		if err := applyWorkbenchTargetFilter(db.Model(&database.FingerprintInfo{}), []string{"url", "host"}, refs).
			Order("created_at DESC").
			Limit(limit).
			Find(&fingerprints).Error; err != nil {
			return urls, err
		}
		for _, item := range fingerprints {
			add(item.URL)
		}

		var dirResults []database.VulnScanDirResult
		if err := applyWorkbenchTargetFilter(db.Model(&database.VulnScanDirResult{}), []string{"url", "location"}, refs).
			Order("created_at DESC").
			Limit(limit).
			Find(&dirResults).Error; err != nil {
			return urls, err
		}
		for _, item := range dirResults {
			add(item.URL)
			add(item.Location)
		}
	}
	return urls, nil
}

func collectWorkbenchPortHTTPURLs(db *gorm.DB, target string, scope []string, limit int) ([]string, error) {
	if db == nil || limit <= 0 {
		return nil, nil
	}
	targets := normalizeWorkbenchScanTargets(target, scope)
	refs := buildWorkbenchTargetRefs(target, scope)
	cidrs := make([]*net.IPNet, 0)
	for _, item := range targets {
		if _, network, err := net.ParseCIDR(item); err == nil {
			cidrs = append(cidrs, network)
		}
	}

	var ports []database.PortScanResult
	queryLimit := limit * 20
	if queryLimit < 200 {
		queryLimit = 200
	}
	if len(cidrs) > 0 {
		if err := db.Model(&database.PortScanResult{}).Order("created_at DESC").Limit(queryLimit).Find(&ports).Error; err != nil {
			return nil, err
		}
	} else if len(refs) > 0 {
		if err := applyWorkbenchTargetFilter(db.Model(&database.PortScanResult{}), []string{"host", "protocol", "product"}, refs).
			Order("created_at DESC").
			Limit(queryLimit).
			Find(&ports).Error; err != nil {
			return nil, err
		}
	}

	seen := make(map[string]bool, limit)
	result := make([]string, 0, limit)
	for _, item := range ports {
		if !workbenchPortResultMatchesTargets(item.Host, refs, cidrs) {
			continue
		}
		for _, candidate := range WorkbenchWebURLsFromPortResults([]int{item.Port}, item.Host, item.Protocol, item.Product) {
			normalized := NormalizeWorkbenchURLCandidate(candidate)
			if normalized == "" || seen[normalized] {
				continue
			}
			seen[normalized] = true
			result = append(result, normalized)
			if len(result) >= limit {
				return result, nil
			}
		}
	}
	return result, nil
}

func workbenchPortResultMatchesTargets(host string, refs []string, cidrs []*net.IPNet) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		for _, network := range cidrs {
			if network.Contains(ip) {
				return true
			}
		}
	}
	hostLower := strings.ToLower(host)
	for _, ref := range refs {
		ref = strings.ToLower(strings.TrimSpace(ref))
		if ref != "" && strings.Contains(hostLower, ref) {
			return true
		}
	}
	return false
}

// CollectWorkbenchTargetContext 读取既有扫描结果并汇总为统一目标上下文。
func CollectWorkbenchTargetContext(db *gorm.DB, target string, scope []string) (WorkbenchTargetContext, error) {
	refs := buildWorkbenchTargetRefs(target, scope)
	ctx := WorkbenchTargetContext{Refs: refs, Highlights: make([]string, 0, 16)}
	if len(refs) == 0 {
		return ctx, nil
	}

	fingerprintQuery := applyWorkbenchTargetFilter(db.Model(&database.FingerprintInfo{}), []string{"url", "host"}, refs)
	if err := fingerprintQuery.Count(&ctx.FingerprintCount).Error; err != nil {
		return ctx, err
	}
	var fingerprints []database.FingerprintInfo
	if err := applyWorkbenchTargetFilter(db.Model(&database.FingerprintInfo{}), []string{"url", "host"}, refs).
		Order("created_at DESC").
		Limit(5).
		Find(&fingerprints).Error; err != nil {
		return ctx, err
	}
	for _, item := range fingerprints {
		label := strings.TrimSpace(item.Title)
		if label == "" {
			label = strings.TrimSpace(item.Detect)
		}
		if label == "" {
			label = item.URL
		}
		ctx.Highlights = append(ctx.Highlights, fmt.Sprintf("指纹: %s %s", item.URL, label))
	}

	vulnQuery := applyWorkbenchTargetFilter(db.Model(&database.VulnerabilityInfo{}), []string{"vuln_url", "template_id", "vuln_name"}, refs)
	if err := vulnQuery.Count(&ctx.VulnerabilityCount).Error; err != nil {
		return ctx, err
	}
	var vulns []database.VulnerabilityInfo
	if err := applyWorkbenchTargetFilter(db.Model(&database.VulnerabilityInfo{}), []string{"vuln_url", "template_id", "vuln_name"}, refs).
		Order(workbenchVulnerabilityOrder()).
		Limit(5).
		Find(&vulns).Error; err != nil {
		return ctx, err
	}
	for _, item := range vulns {
		ctx.Highlights = append(ctx.Highlights, fmt.Sprintf("漏洞: [%s] %s %s", item.Severity, item.VulnName, item.VulnURL))
	}

	jsAssetQuery := applyWorkbenchTargetFilter(db.Model(&database.VulnScanJSAsset{}), []string{"value", "source"}, refs)
	if err := jsAssetQuery.Count(&ctx.JSAssetCount).Error; err != nil {
		return ctx, err
	}
	var jsAssets []database.VulnScanJSAsset
	if err := applyWorkbenchTargetFilter(db.Model(&database.VulnScanJSAsset{}), []string{"value", "source"}, refs).
		Order("created_at DESC").
		Limit(5).
		Find(&jsAssets).Error; err != nil {
		return ctx, err
	}
	for _, item := range jsAssets {
		ctx.Highlights = append(ctx.Highlights, fmt.Sprintf("JS资产: [%s] %s", item.Type, truncateWorkbenchText(item.Value, 140)))
	}

	dirQuery := applyWorkbenchTargetFilter(db.Model(&database.VulnScanDirResult{}), []string{"url", "title", "location"}, refs)
	if err := dirQuery.Count(&ctx.DirResultCount).Error; err != nil {
		return ctx, err
	}
	var dirResults []database.VulnScanDirResult
	if err := applyWorkbenchTargetFilter(db.Model(&database.VulnScanDirResult{}), []string{"url", "title", "location"}, refs).
		Order("created_at DESC").
		Limit(5).
		Find(&dirResults).Error; err != nil {
		return ctx, err
	}
	for _, item := range dirResults {
		ctx.Highlights = append(ctx.Highlights, fmt.Sprintf("目录: [%d] %s %s", item.Status, item.URL, item.Title))
	}

	portQuery := applyWorkbenchTargetFilter(db.Model(&database.PortScanResult{}), []string{"host", "protocol", "product"}, refs)
	if err := portQuery.Count(&ctx.PortCount).Error; err != nil {
		return ctx, err
	}
	var ports []database.PortScanResult
	if err := applyWorkbenchTargetFilter(db.Model(&database.PortScanResult{}), []string{"host", "protocol", "product"}, refs).
		Order("created_at DESC").
		Limit(5).
		Find(&ports).Error; err != nil {
		return ctx, err
	}
	for _, item := range ports {
		ctx.Highlights = append(ctx.Highlights, fmt.Sprintf("端口: %s:%d %s %s", item.Host, item.Port, item.Protocol, item.Product))
	}

	if len(ctx.Highlights) == 0 {
		ctx.Highlights = append(ctx.Highlights, "暂未从既有扫描结果中匹配到目标上下文。")
	}
	return ctx, nil
}

func workbenchVulnerabilityOrder() string {
	return "task_id DESC, version DESC"
}

func (c WorkbenchTargetContext) Summary() string {
	return fmt.Sprintf(
		"匹配指纹 %d 条、漏洞 %d 条、JS 资产 %d 条、目录结果 %d 条、端口 %d 条。",
		c.FingerprintCount,
		c.VulnerabilityCount,
		c.JSAssetCount,
		c.DirResultCount,
		c.PortCount,
	)
}

func (c WorkbenchTargetContext) RiskScore() int {
	score := 10
	score += int(c.VulnerabilityCount) * 15
	score += int(c.FingerprintCount) * 2
	score += int(c.JSAssetCount)
	score += int(c.PortCount)
	if score > 100 {
		return 100
	}
	return score
}

func buildWorkbenchTargetRefs(target string, scope []string) []string {
	values := append([]string{target}, scope...)
	seen := make(map[string]bool, len(values)*3)
	refs := make([]string, 0, len(values)*3)
	for _, value := range values {
		for _, candidate := range expandWorkbenchTargetRef(value) {
			if candidate == "" || seen[candidate] {
				continue
			}
			seen[candidate] = true
			refs = append(refs, candidate)
			if len(refs) >= 20 {
				return refs
			}
		}
	}
	return refs
}

func expandWorkbenchTargetRef(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	result := []string{value}
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		result = append(result, parsed.Hostname())
	}
	trimmed := strings.TrimPrefix(strings.TrimPrefix(value, "https://"), "http://")
	trimmed = strings.Trim(trimmed, "/")
	if slash := strings.Index(trimmed, "/"); slash >= 0 {
		trimmed = trimmed[:slash]
	}
	if colon := strings.LastIndex(trimmed, ":"); colon > 0 {
		trimmed = trimmed[:colon]
	}
	result = append(result, trimmed)
	return result
}

func applyWorkbenchTargetFilter(query *gorm.DB, fields []string, refs []string) *gorm.DB {
	if len(refs) == 0 || len(fields) == 0 {
		return query.Where("1 = 0")
	}

	clauses := make([]string, 0, len(fields)*len(refs))
	args := make([]any, 0, len(fields)*len(refs))
	for _, field := range fields {
		for _, ref := range refs {
			if strings.TrimSpace(ref) == "" {
				continue
			}
			clauses = append(clauses, field+" LIKE ?")
			args = append(args, "%"+ref+"%")
		}
	}
	if len(clauses) == 0 {
		return query.Where("1 = 0")
	}
	return query.Where(strings.Join(clauses, " OR "), args...)
}

func truncateWorkbenchText(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}
