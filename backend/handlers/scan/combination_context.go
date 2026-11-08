package scan

import (
	"encoding/json"
	"fmt"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/portscan"
	"hephaestus-backend/pkg/subdomain"
	"hephaestus-backend/pkg/taskstate"
	"maps"
	"net"
	"net/url"
	"strings"
	"time"
)

type combinationStepLog struct {
	Module      string     `json:"module"`
	TaskID      string     `json:"taskId"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type combinationContext struct {
	InitialTargets    []string                     `json:"initialTargets"`
	PortTargets       []string                     `json:"portTargets,omitempty"`
	PortTargetsReady  bool                         `json:"portTargetsReady,omitempty"`
	PortTargetAliases map[string][]portTargetAlias `json:"portTargetAliases,omitempty"`
	FilterLoopback    bool                         `json:"filterLoopback,omitempty"`
	LoopbackHosts     []string                     `json:"loopbackHosts,omitempty"`
	WebTargets        []string                     `json:"webTargets,omitempty"`
	WebTargetsReady   bool                         `json:"webTargetsReady,omitempty"`
	CrackTargets      []string                     `json:"crackTargets,omitempty"`
	CrackTargetsReady bool                         `json:"crackTargetsReady,omitempty"`
	Completed         map[string]bool              `json:"completedModules,omitempty"`
	Running           map[string]bool              `json:"runningModules,omitempty"`
	Steps             []*combinationStepLog        `json:"steps,omitempty"`
}

type portTargetAlias struct {
	Domain       string `json:"domain,omitempty"`
	Subdomain    string `json:"subdomain"`
	AssetType    string `json:"assetType,omitempty"`
	AssetName    string `json:"assetName,omitempty"`
	SourceModule string `json:"sourceModule,omitempty"`
}

func (c *combinationContext) clone() *combinationContext {
	if c == nil {
		return &combinationContext{}
	}
	cp := *c
	cp.InitialTargets = append([]string(nil), c.InitialTargets...)
	cp.PortTargets = append([]string(nil), c.PortTargets...)
	cp.LoopbackHosts = append([]string(nil), c.LoopbackHosts...)
	cp.WebTargets = append([]string(nil), c.WebTargets...)
	cp.CrackTargets = append([]string(nil), c.CrackTargets...)
	cp.Steps = make([]*combinationStepLog, 0, len(c.Steps))
	for _, step := range c.Steps {
		if step != nil {
			copy := *step
			cp.Steps = append(cp.Steps, &copy)
		}
	}
	if c.Completed != nil {
		cp.Completed = maps.Clone(c.Completed)
	}
	if c.Running != nil {
		cp.Running = maps.Clone(c.Running)
	}
	if c.PortTargetAliases != nil {
		cp.PortTargetAliases = clonePortTargetAliases(c.PortTargetAliases)
	}
	return &cp
}

func clonePortTargetAliases(input map[string][]portTargetAlias) map[string][]portTargetAlias {
	if input == nil {
		return nil
	}
	out := make(map[string][]portTargetAlias, len(input))
	for ip, aliases := range input {
		out[ip] = append([]portTargetAlias(nil), aliases...)
	}
	return out
}

func newCombinationContext(targets []string) *combinationContext {
	return &combinationContext{
		InitialTargets: normalizeTargetsList(targets),
		Completed:      make(map[string]bool),
		Running:        make(map[string]bool),
	}
}

func (c *combinationContext) ensureStepLogs(nodes []combinationPipelineNode, taskID string) {
	if c == nil {
		return
	}
	exists := make(map[string]struct{})
	for _, step := range c.Steps {
		if step != nil {
			exists[step.Module] = struct{}{}
		}
	}
	for _, node := range nodes {
		if node.ModuleID == "" {
			continue
		}
		if _, ok := exists[node.ModuleID]; ok {
			continue
		}
		step := &combinationStepLog{
			Module: node.ModuleID,
			TaskID: taskID,
			Status: "pending",
		}
		c.Steps = append(c.Steps, step)
		exists[node.ModuleID] = struct{}{}
	}
}

func (c *combinationContext) stepLog(module string) *combinationStepLog {
	if c == nil {
		return nil
	}
	for _, step := range c.Steps {
		if step != nil && step.Module == module {
			return step
		}
	}
	return nil
}

func (c *combinationContext) markStepRunning(module, taskID string, ts time.Time) {
	if c == nil {
		return
	}
	step := c.stepLog(module)
	if step == nil {
		step = &combinationStepLog{Module: module}
		c.Steps = append(c.Steps, step)
	}
	step.TaskID = taskID
	step.Status = "running"
	start := ts
	step.StartedAt = &start
	step.CompletedAt = nil
	step.Error = ""
}

func (c *combinationContext) markStepQueued(module, taskID string, ts time.Time) {
	if c == nil {
		return
	}
	step := c.stepLog(module)
	if step == nil {
		step = &combinationStepLog{Module: module}
		c.Steps = append(c.Steps, step)
	}
	step.TaskID = taskID
	step.Status = "queued"
	start := ts
	step.StartedAt = &start
	step.CompletedAt = nil
	step.Error = ""
}

func (c *combinationContext) markStepFinished(module, status string, err error, ts time.Time) {
	if c == nil {
		return
	}
	step := c.stepLog(module)
	if step == nil {
		step = &combinationStepLog{Module: module}
		c.Steps = append(c.Steps, step)
	}
	step.Status = status
	end := ts
	step.CompletedAt = &end
	if err != nil {
		step.Error = err.Error()
	} else {
		step.Error = ""
	}
}

func loadCombinationContext(record *database.CombinationTask) (*combinationContext, error) {
	if record.Context == "" {
		var initial []string
		if err := json.Unmarshal([]byte(record.Targets), &initial); err != nil {
			return newCombinationContext(nil), nil
		}
		return newCombinationContext(initial), nil
	}

	var ctx combinationContext
	if err := json.Unmarshal([]byte(record.Context), &ctx); err != nil {
		return nil, fmt.Errorf("解析组合任务上下文失败: %w", err)
	}
	if len(ctx.InitialTargets) == 0 {
		var initial []string
		if err := json.Unmarshal([]byte(record.Targets), &initial); err == nil {
			ctx.InitialTargets = normalizeTargetsList(initial)
		}
	}
	if ctx.Completed == nil {
		ctx.Completed = make(map[string]bool)
	}
	if ctx.Running == nil {
		ctx.Running = make(map[string]bool)
	}
	return &ctx, nil
}

func combinationStepStatusRank(status string) int {
	switch status {
	case "completed", "failed", "skipped", taskstate.StatusPaused, "cancelled":
		return 4
	case "running":
		return 3
	case "queued":
		return 2
	case "pending":
		return 1
	default:
		return 0
	}
}

func mergeCombinationStepLog(base *combinationStepLog, overlay *combinationStepLog) *combinationStepLog {
	if base == nil && overlay == nil {
		return nil
	}
	if base == nil {
		cp := *overlay
		return &cp
	}
	if overlay == nil {
		cp := *base
		return &cp
	}

	baseRank := combinationStepStatusRank(base.Status)
	overlayRank := combinationStepStatusRank(overlay.Status)

	selected := *base
	if overlayRank > baseRank {
		selected = *overlay
	} else if overlayRank == baseRank {
		if overlay.CompletedAt != nil && (base.CompletedAt == nil || overlay.CompletedAt.After(*base.CompletedAt)) {
			selected = *overlay
		} else if overlay.StartedAt != nil && (base.StartedAt == nil || overlay.StartedAt.After(*base.StartedAt)) {
			selected = *overlay
		}
	}

	if selected.TaskID == "" {
		if overlay.TaskID != "" {
			selected.TaskID = overlay.TaskID
		} else {
			selected.TaskID = base.TaskID
		}
	}
	if selected.StartedAt == nil {
		if base.StartedAt != nil {
			selected.StartedAt = base.StartedAt
		} else {
			selected.StartedAt = overlay.StartedAt
		}
	}
	if selected.CompletedAt == nil {
		if base.CompletedAt != nil {
			selected.CompletedAt = base.CompletedAt
		} else {
			selected.CompletedAt = overlay.CompletedAt
		}
	}
	if selected.Error == "" {
		if overlay.Error != "" {
			selected.Error = overlay.Error
		} else {
			selected.Error = base.Error
		}
	}

	return &selected
}

func mergeCombinationContext(base *combinationContext, overlay *combinationContext) *combinationContext {
	if base == nil && overlay == nil {
		return newCombinationContext(nil)
	}
	if base == nil {
		return overlay.clone()
	}
	if overlay == nil {
		return base.clone()
	}

	out := base.clone()

	// Initial targets: prefer base, but fill if missing
	if len(out.InitialTargets) == 0 && len(overlay.InitialTargets) > 0 {
		out.InitialTargets = normalizeTargetsList(overlay.InitialTargets)
	}

	// Derived targets: union to avoid losing data in parallel updates
	if len(overlay.PortTargets) > 0 {
		out.PortTargets = setToSortedSlice(unionStringSlices(out.PortTargets, overlay.PortTargets))
	}
	if overlay.PortTargetsReady {
		out.PortTargetsReady = true
	}
	if overlay.FilterLoopback {
		out.FilterLoopback = true
	}
	if len(overlay.LoopbackHosts) > 0 {
		out.LoopbackHosts = setToSortedSlice(unionStringSlices(out.LoopbackHosts, overlay.LoopbackHosts))
	}
	if len(overlay.PortTargetAliases) > 0 {
		out.PortTargetAliases = mergePortTargetAliases(out.PortTargetAliases, overlay.PortTargetAliases)
	}
	if len(overlay.WebTargets) > 0 {
		out.WebTargets = setToSortedSlice(unionStringSlices(out.WebTargets, overlay.WebTargets))
	}
	if overlay.WebTargetsReady {
		out.WebTargetsReady = true
	}
	if len(overlay.CrackTargets) > 0 {
		out.CrackTargets = setToSortedSlice(unionStringSlices(out.CrackTargets, overlay.CrackTargets))
	}
	if overlay.CrackTargetsReady {
		out.CrackTargetsReady = true
	}

	// Completed: OR merge
	if out.Completed == nil {
		out.Completed = make(map[string]bool)
	}
	for k, v := range overlay.Completed {
		if v {
			out.Completed[k] = true
		}
	}

	// Running: keep base view; caller should delete the finished module explicitly.
	if out.Running == nil {
		out.Running = make(map[string]bool)
	}
	for k, v := range overlay.Running {
		if v {
			out.Running[k] = true
		}
	}

	// Steps: preserve the most advanced state per module so stale parallel snapshots
	// do not regress queued/running/completed steps back to pending.
	stepIndex := make(map[string]*combinationStepLog)
	for _, s := range out.Steps {
		if s != nil && s.Module != "" {
			stepIndex[s.Module] = s
		}
	}
	for _, s := range overlay.Steps {
		if s == nil || s.Module == "" {
			continue
		}
		if existing, ok := stepIndex[s.Module]; ok {
			mergedStep := mergeCombinationStepLog(existing, s)
			if mergedStep != nil {
				*existing = *mergedStep
			}
		} else {
			cp := *s
			out.Steps = append(out.Steps, &cp)
			stepIndex[s.Module] = &cp
		}
	}

	if out.Completed == nil {
		out.Completed = make(map[string]bool)
	}
	if out.Running == nil {
		out.Running = make(map[string]bool)
	}
	for module, completed := range out.Completed {
		if completed {
			delete(out.Running, module)
		}
	}
	for _, step := range out.Steps {
		if step != nil && (step.Status == "paused" || step.Status == "failed" || step.Status == "cancelled") {
			delete(out.Running, step.Module)
		}
	}
	return out
}

func unionStringSlices(a, b []string) map[string]struct{} {
	out := make(map[string]struct{}, len(a)+len(b))
	for _, s := range a {
		s = strings.TrimSpace(s)
		if s != "" {
			out[s] = struct{}{}
		}
	}
	for _, s := range b {
		s = strings.TrimSpace(s)
		if s != "" {
			out[s] = struct{}{}
		}
	}
	return out
}

func mergePortTargetAliases(base, overlay map[string][]portTargetAlias) map[string][]portTargetAlias {
	out := clonePortTargetAliases(base)
	if out == nil {
		out = make(map[string][]portTargetAlias)
	}

	for ip, aliases := range overlay {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		seen := make(map[string]struct{})
		for _, alias := range out[ip] {
			seen[alias.Subdomain] = struct{}{}
		}
		for _, alias := range aliases {
			alias.Subdomain = strings.TrimSpace(alias.Subdomain)
			if alias.Subdomain == "" {
				continue
			}
			if _, ok := seen[alias.Subdomain]; ok {
				continue
			}
			out[ip] = append(out[ip], alias)
			seen[alias.Subdomain] = struct{}{}
		}
	}

	return out
}

func exportPortTargetAliases(input map[string][]portTargetAlias) map[string][]map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string][]map[string]string, len(input))
	for ip, aliases := range input {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		for _, alias := range aliases {
			if strings.TrimSpace(alias.Subdomain) == "" {
				continue
			}
			out[ip] = append(out[ip], map[string]string{
				"domain":       alias.Domain,
				"subdomain":    alias.Subdomain,
				"assetType":    alias.AssetType,
				"assetName":    alias.AssetName,
				"sourceModule": alias.SourceModule,
			})
		}
	}
	return out
}

func (c *combinationContext) serialize() string {
	raw, _ := json.Marshal(c)
	return string(raw)
}

func (c *combinationContext) targetsFor(module string) ([]string, error) {
	targets, err := c.rawTargetsFor(module)
	if err != nil || c == nil || !c.FilterLoopback || module == "subdomain" {
		return targets, err
	}
	blockedHosts := make(map[string]struct{}, len(c.LoopbackHosts))
	for _, host := range c.LoopbackHosts {
		blockedHosts[normalizeCombinationTargetHost(host)] = struct{}{}
	}
	filtered := make([]string, 0, len(targets))
	for _, target := range targets {
		host := strings.TrimSpace(target)
		if strings.Contains(host, "://") {
			if parsed, parseErr := url.Parse(host); parseErr == nil {
				host = parsed.Hostname()
			}
		} else if parsedHost, _, splitErr := net.SplitHostPort(host); splitErr == nil {
			host = parsedHost
		}
		host = normalizeCombinationTargetHost(host)
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			continue
		}
		if _, blocked := blockedHosts[host]; blocked {
			continue
		}
		filtered = append(filtered, target)
	}
	return filtered, nil
}

func normalizeCombinationTargetHost(host string) string {
	return strings.TrimRight(strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]")), ".")
}

func (c *combinationContext) rawTargetsFor(module string) ([]string, error) {
	if c == nil {
		return nil, fmt.Errorf("上下文缺失")
	}
	switch module {
	case "portscan":
		if len(c.PortTargets) > 0 {
			return c.PortTargets, nil
		}
		if c.PortTargetsReady {
			return nil, nil
		}
		if len(c.InitialTargets) == 0 {
			return nil, fmt.Errorf("没有可用的初始目标")
		}
		return c.InitialTargets, nil
	case "webscan", "dirscan", "jsscan":
		if len(c.WebTargets) == 0 {
			if c.WebTargetsReady {
				return nil, nil
			}
			// 没有端口扫描派生目标时，回退到用户输入的原始目标
			if len(c.InitialTargets) == 0 {
				return nil, nil
			}
			return c.InitialTargets, nil
		}
		return c.WebTargets, nil
	case "crack":
		if len(c.CrackTargets) == 0 {
			if c.CrackTargetsReady {
				return nil, nil
			}
			// 允许直接使用用户输入的协议://host:port 目标发起密码喷洒
			fallback := inferCrackTargetsFromInitialTargets(c.InitialTargets)
			if len(fallback) == 0 {
				return nil, nil
			}
			return fallback, nil
		}
		return c.CrackTargets, nil
	default:
		if len(c.InitialTargets) == 0 {
			return nil, nil
		}
		return c.InitialTargets, nil
	}
}

func inferCrackTargetsFromInitialTargets(targets []string) []string {
	if len(targets) == 0 {
		return nil
	}
	allowed := make(map[string]struct{})
	for _, proto := range portscan.SupportedCrackProtocols() {
		allowed[strings.ToLower(strings.TrimSpace(proto))] = struct{}{}
	}

	out := make(map[string]struct{})
	for _, raw := range targets {
		target := strings.TrimSpace(raw)
		if target == "" {
			continue
		}
		parts := strings.SplitN(target, "://", 2)
		if len(parts) != 2 {
			continue
		}
		scheme := strings.ToLower(strings.TrimSpace(parts[0]))
		hostPort := strings.TrimSpace(parts[1])
		if _, ok := allowed[scheme]; !ok {
			continue
		}
		if hostPort == "" || !strings.Contains(hostPort, ":") {
			continue
		}
		out[target] = struct{}{}
	}
	return setToSortedSlice(out)
}

func isSharedSubdomainInfrastructure(assetType string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(assetType))
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "CDN") ||
		strings.Contains(normalized, "WAF") ||
		strings.Contains(normalized, "CLOUD") ||
		normalized == "COMMON"
}

func subdomainHasLoopbackIP(result subdomain.SubdomainResult) bool {
	for _, rawIP := range result.IPs {
		if ip := net.ParseIP(normalizeCombinationTargetHost(rawIP)); ip != nil && ip.IsLoopback() {
			return true
		}
	}
	return false
}

func (c *combinationContext) applySubdomainResults(results []subdomain.SubdomainResult, nodes []combinationPipelineNode) {
	inputType, err := detectCombinationTargetInputType(nodes)
	c.FilterLoopback = err == nil && inputType == combinationTargetInputTypeDomain
	c.LoopbackHosts = nil
	if c.FilterLoopback {
		hosts := make(map[string]struct{})
		for _, result := range results {
			if host := normalizeCombinationTargetHost(result.Subdomain); host != "" && subdomainHasLoopbackIP(result) {
				hosts[host] = struct{}{}
			}
		}
		c.LoopbackHosts = setToSortedSlice(hosts)
	}
	c.PortTargets, c.PortTargetAliases = derivePortTargetsFromSubdomainResults(results, c.FilterLoopback)
	c.PortTargetsReady = true
}

func derivePortTargetsFromSubdomainResults(results []subdomain.SubdomainResult, filterLoopback bool) ([]string, map[string][]portTargetAlias) {
	targetSet := make(map[string]struct{})
	aliases := make(map[string][]portTargetAlias)
	aliasSeen := make(map[string]map[string]struct{})

	for _, result := range results {
		subdomainName := strings.TrimSpace(result.Subdomain)
		if subdomainName == "" || isSharedSubdomainInfrastructure(result.AssetType) {
			continue
		}

		alias := portTargetAlias{
			Domain:       strings.TrimSpace(result.Domain),
			Subdomain:    subdomainName,
			AssetType:    strings.TrimSpace(result.AssetType),
			AssetName:    strings.TrimSpace(result.AssetName),
			SourceModule: strings.TrimSpace(result.Source),
		}
		// A mixed DNS answer can resolve back to loopback when Web modules use
		// the hostname again. Keep its other IP targets, but omit hostname aliases.
		skipAlias := filterLoopback && subdomainHasLoopbackIP(result)

		for _, rawIP := range result.IPs {
			ip := strings.TrimSpace(rawIP)
			parsedIP := net.ParseIP(strings.Trim(ip, "[]"))
			if ip == "" || parsedIP == nil || (filterLoopback && parsedIP.IsLoopback()) {
				continue
			}
			ip = parsedIP.String()
			targetSet[ip] = struct{}{}
			// Keep the deduplicated IP for service discovery, but do not turn
			// wildcard DNS candidates into a virtual host for every open port.
			if result.IsWildcard || skipAlias {
				continue
			}
			if aliasSeen[ip] == nil {
				aliasSeen[ip] = make(map[string]struct{})
			}
			if _, ok := aliasSeen[ip][subdomainName]; ok {
				continue
			}
			aliases[ip] = append(aliases[ip], alias)
			aliasSeen[ip][subdomainName] = struct{}{}
		}
	}

	return setToSortedSlice(targetSet), aliases
}
