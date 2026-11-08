package ai

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// WorkbenchScanExecutionPlan 是 AI 智脑准备交给 Hephaestus 扫描模块的受控执行计划。
type WorkbenchScanExecutionPlan struct {
	ScanType   string                 `json:"scanType"`
	TaskName   string                 `json:"taskName"`
	ScanMode   string                 `json:"scanMode"`
	Targets    []string               `json:"targets"`
	Config     map[string]interface{} `json:"config"`
	ToolName   string                 `json:"toolName"`
	Mode       string                 `json:"mode"`
	RiskLevel  string                 `json:"riskLevel"`
	Executable bool                   `json:"executable"`
	Optional   bool                   `json:"optional,omitempty"`
	Reason     string                 `json:"reason"`
}

// BuildWorkbenchScanExecutionPlan 根据 AI 步骤生成安全默认的 Hephaestus 扫描计划。
func BuildWorkbenchScanExecutionPlan(taskName string, target string, scope []string, stepType string, riskPolicy string) WorkbenchScanExecutionPlan {
	plans := BuildWorkbenchScanExecutionPlans(taskName, target, scope, stepType, riskPolicy)
	if len(plans) == 0 {
		return WorkbenchScanExecutionPlan{
			TaskName:   fmt.Sprintf("AI 智脑 - %s - %s", stepType, safeWorkbenchTaskName(taskName, target)),
			ScanMode:   "local",
			Config:     map[string]interface{}{},
			Executable: false,
			Reason:     "该步骤不需要创建 Hephaestus 扫描任务。",
		}
	}
	return plans[0]
}

// BuildWorkbenchScanExecutionPlans 根据 AI 步骤和目标形态生成一组受控扫描计划。
func BuildWorkbenchScanExecutionPlans(taskName string, target string, scope []string, stepType string, riskPolicy string) []WorkbenchScanExecutionPlan {
	targets := normalizeWorkbenchScanTargets(target, scope)
	switch stepType {
	case "tool_orchestration":
		return buildWorkbenchDiscoveryPlans(taskName, target, targets)
	case "verification":
		webTargets := collectWorkbenchWebTargets(targets)
		plan := baseWorkbenchScanExecutionPlan(taskName, target, stepType, "webscan", "web.vuln.verify", "vulnerability_verification", "high", webTargets)
		plan.ToolName = "web.vuln.verify"
		plan.Mode = "vulnerability_verification"
		plan.RiskLevel = "high"
		if riskPolicy == WorkbenchRiskReadOnly {
			plan.Executable = false
			plan.Reason = "当前风险策略为只读优先，不自动创建漏洞验证扫描任务。"
			return []WorkbenchScanExecutionPlan{plan}
		}
		plan.Config["webscanOption"] = 3
		plan.Config["vulnerability"] = true
		plan.Config["deepScan"] = true
		plan.Config["skipNucleiWithoutTags"] = true
		plan.Config["skipNucleiWithWAF"] = true
		plan.Config["threads"] = 8
		plan.Config["nucleiThreads"] = 2
		plan.Reason = "执行已批准的最小化漏洞验证扫描。"
		if len(webTargets) == 0 {
			plan.Executable = false
			plan.Reason = "未识别到可验证的 HTTP/HTTPS Web 入口，跳过漏洞验证扫描；请先完成资产和服务发现。"
		}
		return []WorkbenchScanExecutionPlan{plan}
	default:
		plan := baseWorkbenchScanExecutionPlan(taskName, target, stepType, "", "", "", "low", nil)
		plan.Executable = false
		plan.Reason = "该步骤不需要创建 Hephaestus 扫描任务。"
		return []WorkbenchScanExecutionPlan{plan}
	}
}

func buildWorkbenchDiscoveryPlans(taskName string, target string, targets []string) []WorkbenchScanExecutionPlan {
	if len(targets) == 0 {
		plan := baseWorkbenchScanExecutionPlan(taskName, target, "tool_orchestration", "webscan", "web.fingerprint", "fingerprint", "low", nil)
		plan.Reason = "未找到可执行扫描目标。"
		return []WorkbenchScanExecutionPlan{plan}
	}

	domains := collectWorkbenchRootDomainTargets(targets)
	hosts := collectWorkbenchHostTargets(targets)
	webTargets := collectWorkbenchWebTargets(targets)
	plans := make([]WorkbenchScanExecutionPlan, 0, 5)

	if len(domains) > 0 {
		plan := baseWorkbenchScanExecutionPlan(taskName, target, "tool_orchestration", "subdomain", "asset.subdomain", "subdomain_discovery", "medium", domains)
		plan.Config["domains"] = domains
		plan.Config["thread"] = 20
		plan.Config["timeout"] = 8
		plan.Reason = "目标包含根域名，执行子域名收集以扩展授权资产上下文。"
		plans = append(plans, plan)
	}

	if len(hosts) > 0 {
		plan := baseWorkbenchScanExecutionPlan(taskName, target, "tool_orchestration", "portscan", "asset.portscan", "service_discovery", "medium", hosts)
		plan.Config["threads"] = 100
		plan.Config["timeout"] = 3
		plan.Config["aliveDetection"] = true
		plan.Reason = "目标包含主机或网络地址，使用系统默认端口组执行服务识别。"
		plans = append(plans, plan)
	}

	if len(webTargets) > 0 {
		webPlan := buildWorkbenchDefaultWebscanPlan(taskName, target, "tool_orchestration", webTargets)
		plans = append(plans, webPlan)

		jsPlan := baseWorkbenchScanExecutionPlan(taskName, target, "tool_orchestration", "jsscan", "web.jsscan", "js_api_discovery", "medium", webTargets)
		jsPlan.Config["targets"] = webTargets
		jsPlan.Config["vulnDetection"] = map[string]interface{}{"enabled": false}
		jsPlan.Config["openai"] = map[string]interface{}{"enabled": false}
		jsPlan.Reason = "Web 资产参考扫描：对标 katana 执行 JS、接口和 API 根路径识别，并缓存 JS 内容供后续分析。"
		plans = append(plans, jsPlan)

		dirPlan := baseWorkbenchScanExecutionPlan(taskName, target, "tool_orchestration", "dirscan", "web.dirscan", "content_discovery", "medium", webTargets)
		dirPlan.Config["workers"] = 20
		dirPlan.Config["timeout"] = 6
		dirPlan.Config["redirect"] = false
		dirPlan.Config["recursive"] = 0
		dirPlan.Config["paths"] = []string{"/robots.txt", "/sitemap.xml", "/admin", "/login", "/api", "/swagger-ui.html", "/actuator"}
		dirPlan.Optional = true
		dirPlan.Reason = "条件补充扫描：仅当站点结构、历史反馈或当前证据需要补充敏感入口发现时执行小字典目录探测。"
		plans = append(plans, dirPlan)
	}

	if len(plans) == 0 {
		plan := baseWorkbenchScanExecutionPlan(taskName, target, "tool_orchestration", "", "", "", "low", nil)
		plan.Executable = false
		plan.Reason = "未能明确目标类型，未创建扫描任务；请先补充明确的 URL、域名、IP、IP:Port 或 CIDR。"
		plans = append(plans, plan)
	}
	return plans
}

// BuildWorkbenchScanExecutionPlanForScanType builds an explicit scan plan and
// enforces the selected capability's supported target shape before execution.
func BuildWorkbenchScanExecutionPlanForScanType(taskName string, target string, scope []string, stepType string, scanType string, riskPolicy string) WorkbenchScanExecutionPlan {
	scanType = strings.ToLower(strings.TrimSpace(scanType))
	targets := normalizeWorkbenchScanTargets(target, scope)

	switch scanType {
	case "subdomain":
		domains := collectWorkbenchRootDomainTargets(targets)
		plan := baseWorkbenchScanExecutionPlan(taskName, target, stepType, "subdomain", "asset.subdomain", "subdomain_discovery", "medium", domains)
		plan.Config["domains"] = domains
		plan.Config["thread"] = 20
		plan.Config["timeout"] = 8
		plan.Reason = "目标包含根域名，执行子域名收集以扩展授权资产上下文。"
		if len(domains) == 0 {
			return unsupportedWorkbenchScanPlan(taskName, target, stepType, "subdomain", "asset.subdomain", "medium", "subdomain 仅支持根域名目标。")
		}
		return plan
	case "portscan":
		hosts := collectWorkbenchHostTargets(targets)
		plan := baseWorkbenchScanExecutionPlan(taskName, target, stepType, "portscan", "asset.portscan", "service_discovery", "medium", hosts)
		plan.Config["threads"] = 100
		plan.Config["timeout"] = 3
		plan.Config["aliveDetection"] = true
		plan.Reason = "目标包含主机或网络地址，使用系统默认端口组执行服务识别。"
		if len(hosts) == 0 {
			return unsupportedWorkbenchScanPlan(taskName, target, stepType, "portscan", "asset.portscan", "medium", "portscan 仅支持 CIDR、IP、主机名或可提取主机的 URL。")
		}
		return plan
	case "webscan":
		webTargets := collectWorkbenchWebTargets(targets)
		plan := buildWorkbenchDefaultWebscanPlan(taskName, target, stepType, webTargets)
		if len(webTargets) == 0 {
			return unsupportedWorkbenchScanPlan(taskName, target, stepType, "webscan", "web.fingerprint", "low", "webscan 仅支持明确的 HTTP/HTTPS URL。")
		}
		return plan
	case "dirscan":
		webTargets := collectWorkbenchWebTargets(targets)
		plan := baseWorkbenchScanExecutionPlan(taskName, target, stepType, "dirscan", "web.dirscan", "content_discovery", "medium", webTargets)
		plan.Config["workers"] = 20
		plan.Config["timeout"] = 6
		plan.Config["redirect"] = false
		plan.Config["recursive"] = 0
		plan.Config["paths"] = []string{"/robots.txt", "/sitemap.xml", "/admin", "/login", "/api", "/swagger-ui.html", "/actuator"}
		plan.Reason = "目标包含 Web URL，执行小字典目录和敏感入口探测。"
		if len(webTargets) == 0 {
			return unsupportedWorkbenchScanPlan(taskName, target, stepType, "dirscan", "web.dirscan", "medium", "dirscan 仅支持明确的 HTTP/HTTPS URL。")
		}
		return plan
	case "jsscan":
		webTargets := collectWorkbenchWebTargets(targets)
		plan := baseWorkbenchScanExecutionPlan(taskName, target, stepType, "jsscan", "web.jsscan", "js_api_discovery", "medium", webTargets)
		plan.Config["targets"] = webTargets
		plan.Config["vulnDetection"] = map[string]interface{}{"enabled": false}
		plan.Config["openai"] = map[string]interface{}{"enabled": false}
		plan.Reason = "目标包含 Web URL，对标 katana 执行 JS、接口和 API 根路径识别，并缓存 JS 内容供后续分析。"
		if len(webTargets) == 0 {
			return unsupportedWorkbenchScanPlan(taskName, target, stepType, "jsscan", "web.jsscan", "medium", "jsscan 仅支持明确的 HTTP/HTTPS URL。")
		}
		return plan
	case "verification":
		return BuildWorkbenchScanExecutionPlan(taskName, target, scope, "verification", riskPolicy)
	default:
		return unsupportedWorkbenchScanPlan(taskName, target, stepType, scanType, scanType, "low", "未知扫描能力，未创建扫描任务。")
	}
}

func buildWorkbenchDefaultWebscanPlan(taskName string, target string, stepType string, webTargets []string) WorkbenchScanExecutionPlan {
	plan := baseWorkbenchScanExecutionPlan(taskName, target, stepType, "webscan", "web.fingerprint", "fingerprint_asset_discovery", "low", webTargets)
	plan.Config["deepScan"] = true
	plan.Config["skipNucleiWithoutTags"] = true
	plan.Config["skipNucleiWithWAF"] = true
	plan.Config["threads"] = 10
	plan.Config["nucleiThreads"] = 2
	plan.Config["webscanOption"] = 2
	plan.Config["vulnerability"] = false
	plan.Reason = "前置资产发现默认使用 webscanOption=2 且 deepScan=true 执行全指纹扫描，补充 Web 指纹、WAF、主动目录和资产线索；漏洞验证留到 verification 阶段按证据触发 webscanOption=3。"
	return plan
}

func unsupportedWorkbenchScanPlan(taskName string, target string, stepType string, scanType string, toolName string, riskLevel string, reason string) WorkbenchScanExecutionPlan {
	plan := baseWorkbenchScanExecutionPlan(taskName, target, stepType, scanType, toolName, "", riskLevel, nil)
	plan.Executable = false
	plan.Reason = reason
	return plan
}

func baseWorkbenchScanExecutionPlan(taskName string, target string, stepType string, scanType string, toolName string, mode string, riskLevel string, targets []string) WorkbenchScanExecutionPlan {
	config := map[string]interface{}{}
	if len(targets) > 0 {
		config["targets"] = targets
	}
	return WorkbenchScanExecutionPlan{
		ScanType:   scanType,
		TaskName:   fmt.Sprintf("AI 智脑 - %s - %s", stepType, safeWorkbenchTaskName(taskName, target)),
		ScanMode:   "local",
		Targets:    targets,
		Config:     config,
		ToolName:   toolName,
		Mode:       mode,
		RiskLevel:  riskLevel,
		Executable: len(targets) > 0,
	}
}

func normalizeWorkbenchScanTargets(target string, scope []string) []string {
	values := append([]string{target}, scope...)
	seen := make(map[string]bool, len(values))
	targets := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeWorkbenchScanTarget(value)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		targets = append(targets, normalized)
		if len(targets) >= 20 {
			break
		}
	}
	return targets
}

func normalizeWorkbenchScanTarget(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Hostname() != "" {
		return strings.TrimRight(value, "/")
	}
	if strings.Contains(value, "://") {
		return ""
	}
	if isWorkbenchCIDR(value) {
		return value
	}
	if strings.Contains(value, "/") {
		value = strings.Trim(value, "/")
		if slash := strings.Index(value, "/"); slash >= 0 {
			value = value[:slash]
		}
	}
	return value
}

func collectWorkbenchRootDomainTargets(targets []string) []string {
	result := make([]string, 0, len(targets))
	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		host := hostFromWorkbenchTarget(target)
		if host == "" || !isLikelyRootDomain(host) || seen[host] {
			continue
		}
		seen[host] = true
		result = append(result, host)
	}
	return result
}

func collectWorkbenchHostTargets(targets []string) []string {
	result := make([]string, 0, len(targets))
	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		host := hostFromWorkbenchTarget(target)
		if host == "" || !isWorkbenchHostTarget(host) || seen[host] {
			continue
		}
		seen[host] = true
		result = append(result, host)
	}
	return result
}

func collectWorkbenchWebTargets(targets []string) []string {
	result := make([]string, 0, len(targets))
	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		if !isWorkbenchWebTarget(target) || seen[target] {
			continue
		}
		seen[target] = true
		result = append(result, target)
	}
	return result
}

func isWorkbenchWebTarget(target string) bool {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != ""
}

func isWorkbenchHostTarget(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return false
	}
	if isWorkbenchCIDR(host) || net.ParseIP(host) != nil {
		return true
	}
	if strings.ContainsAny(host, " \t\r\n/") {
		return false
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, ch := range label {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func hostFromWorkbenchTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if isWorkbenchCIDR(target) {
		return strings.ToLower(target)
	}
	if parsed, err := url.Parse(target); err == nil && parsed.Hostname() != "" {
		return strings.ToLower(parsed.Hostname())
	}
	if strings.Contains(target, "://") {
		return ""
	}
	if host, _, err := net.SplitHostPort(target); err == nil {
		return strings.ToLower(strings.Trim(host, "[]"))
	}
	if slash := strings.Index(target, "/"); slash >= 0 {
		target = target[:slash]
	}
	return strings.ToLower(strings.Trim(target, "[]"))
}

func isWorkbenchCIDR(target string) bool {
	_, _, err := net.ParseCIDR(strings.TrimSpace(target))
	return err == nil
}

func isLikelyRootDomain(host string) bool {
	if host == "" || net.ParseIP(host) != nil {
		return false
	}
	if isWorkbenchCIDR(host) {
		return false
	}
	parts := strings.Split(host, ".")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	return true
}

func safeWorkbenchTaskName(taskName string, target string) string {
	name := strings.TrimSpace(taskName)
	if name == "" {
		name = strings.TrimSpace(target)
	}
	runes := []rune(name)
	if len(runes) > 40 {
		return string(runes[:40])
	}
	return name
}
