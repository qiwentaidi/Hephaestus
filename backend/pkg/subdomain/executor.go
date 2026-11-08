package subdomain

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/types"
)

const (
	realtimeResultBatchSize = 20
	progressUpdateInterval  = 500 * time.Millisecond
)

func normalizeTargetsList(inputs []string) []string {
	unique := make(map[string]struct{})
	result := make([]string, 0, len(inputs))
	for _, value := range inputs {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := unique[trimmed]; exists {
			continue
		}
		unique[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func toStringSlice(value interface{}) []string {
	switch items := value.(type) {
	case []string:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if text := strings.TrimSpace(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if text, ok := item.(string); ok {
				if text = strings.TrimSpace(text); text != "" {
					out = append(out, text)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func intFromConfig(config map[string]interface{}, key string, fallback int) int {
	switch value := config[key].(type) {
	case int:
		if value > 0 {
			return value
		}
	case int32:
		if value > 0 {
			return int(value)
		}
	case int64:
		if value > 0 {
			return int(value)
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	}
	return fallback
}

func BuildScanOption(targets []string, rawConfig map[string]interface{}) (types.SubdomainOption, error) {
	req := types.SubdomainOption{
		Domains: normalizeTargetsList(targets),
		Thread:  50,
		Timeout: 5,
		Mode:    1,
	}
	if rawConfig == nil {
		rawConfig = map[string]interface{}{}
	}

	if domains := toStringSlice(rawConfig["domains"]); len(domains) > 0 {
		req.Domains = normalizeTargetsList(domains)
	}
	if len(req.Domains) == 0 {
		return req, fmt.Errorf("请至少提供一个子域名收集目标")
	}

	req.Thread = intFromConfig(rawConfig, "thread", req.Thread)
	req.Timeout = intFromConfig(rawConfig, "timeout", req.Timeout)
	req.Mode = intFromConfig(rawConfig, "mode", req.Mode)
	if proxy, ok := rawConfig["proxy"].(string); ok {
		req.Proxy = strings.TrimSpace(proxy)
	}
	req.Subs = toStringSlice(rawConfig["subs"])
	if engines := toStringSlice(rawConfig["appendEngines"]); len(engines) > 0 {
		req.AppendEngines = canonicalSubdomainAppendEngines(engines)
	}
	req.DnsServers = toStringSlice(rawConfig["dnsServers"])

	return PrepareScanOption(req)
}

func PrepareScanOption(req types.SubdomainOption) (types.SubdomainOption, error) {
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		return req, fmt.Errorf("加载配置失败: %w", err)
	}

	aiConfig, err := config.GetAIConfig()
	if err != nil {
		logger.Warning("加载AI配置失败: %v", err)
	} else {
		aiConfig = config.ResolveAIRequestConfigForRoute(aiConfig, config.AIRouteSubdomainAssist)
	}

	if req.FofaApi == "" && spaceConfig.FOFA.Key != "" {
		req.FofaApi = spaceConfig.FOFA.Key
		req.FofaEmail = spaceConfig.FOFA.Email
		req.FofaAddress = spaceConfig.FOFA.API
	}
	if req.HunterApi == "" && spaceConfig.Hunter.Key != "" {
		req.HunterApi = spaceConfig.Hunter.Key
	}
	if req.QuakeApi == "" && spaceConfig.Quake.Key != "" {
		req.QuakeApi = spaceConfig.Quake.Key
	}
	if req.ChaosApi == "" && spaceConfig.Chaos.Key != "" {
		req.ChaosApi = spaceConfig.Chaos.Key
	}
	if req.BevigilApi == "" && spaceConfig.Bevigil.Key != "" {
		req.BevigilApi = spaceConfig.Bevigil.Key
	}
	if req.ZoomeyeApi == "" && spaceConfig.Zoomeye.Key != "" {
		req.ZoomeyeApi = spaceConfig.Zoomeye.Key
	}
	if req.ZoomeyeAddress == "" && spaceConfig.Zoomeye.API != "" {
		req.ZoomeyeAddress = spaceConfig.Zoomeye.API
	}
	if req.GithubApi == "" && spaceConfig.Github.Token != "" {
		req.GithubApi = spaceConfig.Github.Token
	}
	if req.SecuritytrailsApi == "" && spaceConfig.Securitytrails.Key != "" {
		req.SecuritytrailsApi = spaceConfig.Securitytrails.Key
	}

	if aiConfig.Enabled && aiConfig.Protocol == config.AIProtocolOpenAICompatible {
		req.AIEnabled = true
		req.AIApiUrl = strings.TrimRight(aiConfig.API, "/") + "/chat/completions"
		req.AIApiKey = aiConfig.Key
		req.AIModel = aiConfig.Model
	} else if aiConfig.Enabled {
		logger.Warning("子域名 AI 辅助仅支持 OpenAI 兼容协议，已跳过当前 AI 渠道")
	}

	if req.Thread == 0 {
		req.Thread = 50
	}
	if req.Timeout == 0 {
		req.Timeout = 5
	}
	if len(req.AppendEngines) == 0 {
		req.AppendEngines = defaultSubdomainAppendEngines(spaceConfig)
	} else {
		req.AppendEngines = canonicalSubdomainAppendEngines(req.AppendEngines)
	}
	if len(req.DnsServers) > 0 {
		dnsServers, err := NormalizeDNSServerAddresses(req.DnsServers)
		if err != nil {
			return req, fmt.Errorf("DNS服务器配置无效: %w", err)
		}
		req.DnsServers = dnsServers
	} else if dnsServers, err := LoadConfiguredDNSServerAddresses(); err != nil {
		logger.Warning("加载子域名DNS服务器配置失败: %v", err)
	} else if len(dnsServers) > 0 {
		req.DnsServers = dnsServers
	}

	return req, nil
}

func canonicalSubdomainAppendEngines(engines []string) []string {
	canonicalNames := map[string]string{
		"fofa":           "FOFA",
		"hunter":         "Hunter",
		"quake":          "Quake",
		"chaos":          "Chaos",
		"bevigil":        "Bevigil",
		"zoomeye":        "ZoomEye",
		"github":         "GitHub",
		"securitytrails": "Securitytrails",
		"digitorus":      "Digitorus",
		"hackertarget":   "HackerTarget",
		"thc":            "THC",
		"anubis":         "Anubis",
		"ip138":          "IP138",
		"crt.name":       "crt.name",
		"crtname":        "crt.name",
	}
	seen := make(map[string]struct{}, len(engines))
	result := make([]string, 0, len(engines))
	for _, engine := range engines {
		name := strings.TrimSpace(engine)
		if name == "" {
			continue
		}
		if canonical, ok := canonicalNames[strings.ToLower(name)]; ok {
			name = canonical
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}

func defaultSubdomainAppendEngines(spaceConfig config.SpaceEngineConfig) []string {
	engines := make([]string, 0, 13)
	appendIfConfigured := func(name string, values ...string) {
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				engines = append(engines, name)
				return
			}
		}
	}

	appendIfConfigured("FOFA", spaceConfig.FOFA.Key)
	appendIfConfigured("Hunter", spaceConfig.Hunter.Key)
	appendIfConfigured("Quake", spaceConfig.Quake.Key)
	appendIfConfigured("Chaos", spaceConfig.Chaos.Key)
	appendIfConfigured("Bevigil", spaceConfig.Bevigil.Key)
	appendIfConfigured("ZoomEye", spaceConfig.Zoomeye.Key)
	appendIfConfigured("GitHub", spaceConfig.Github.Token)
	appendIfConfigured("Securitytrails", spaceConfig.Securitytrails.Key)

	engines = append(engines, "Digitorus", "HackerTarget", "THC", "Anubis", "IP138")
	return canonicalSubdomainAppendEngines(engines)
}

func ExecuteSubdomainScanWithContext(ctx context.Context, taskID string, targets []string, rawConfig map[string]interface{}) error {
	req, err := BuildScanOption(targets, rawConfig)
	if err != nil {
		return err
	}
	agentItem, _ := rawConfig["agentItemMode"].(bool)
	if !agentItem {
		if err := PrepareScanTask(taskID, req); err != nil {
			return err
		}
	}
	return runScanTask(ctx, taskID, req, !agentItem)
}

func RunScanTask(ctx context.Context, taskID string, req types.SubdomainOption) error {
	return runScanTask(ctx, taskID, req, true)
}

func runScanTask(ctx context.Context, taskID string, req types.SubdomainOption, persistProgress bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	var (
		pendingResults       []SubdomainResult
		seenResults          = make(map[string]int)
		persistedResultCount int64
		totalCount           int64
		currentCount         int64
		mu                   sync.Mutex
		lastProgressUpdate   time.Time
		progressDetail       = ScanProgressDetail{TotalDomains: len(req.Domains)}
		scanFailed           bool
		scanErr              error
	)

	toInt := func(value interface{}) int {
		switch v := value.(type) {
		case int:
			return v
		case int32:
			return int(v)
		case int64:
			return int(v)
		case float64:
			return int(v)
		default:
			return 0
		}
	}
	eventMap := func(data interface{}) map[string]interface{} {
		if m, ok := data.(map[string]interface{}); ok {
			return m
		}
		return nil
	}
	eventString := func(value interface{}) string {
		if text, ok := value.(string); ok {
			return text
		}
		return ""
	}
	clampProgress := func(value int) int {
		if value < 0 {
			return 0
		}
		if value > 100 {
			return 100
		}
		return value
	}

	refreshProgressDetail := func() {
		currentProgress := 0
		if progressDetail.CurrentDomainTotal > 0 {
			currentProgress = int((int64(progressDetail.CurrentDomainResolved) * 100) / int64(progressDetail.CurrentDomainTotal))
		} else if progressDetail.CurrentDomain != "" && progressDetail.CompletedDomains >= progressDetail.CurrentDomainIndex && progressDetail.CurrentDomainIndex > 0 {
			currentProgress = 100
		}
		progressDetail.CurrentDomainProgress = clampProgress(currentProgress)

		if progressDetail.TotalDomains <= 0 {
			progressDetail.OverallProgress = 0
			return
		}
		completedDomains := progressDetail.CompletedDomains
		if completedDomains > progressDetail.TotalDomains {
			completedDomains = progressDetail.TotalDomains
		}

		overallNumerator := int64(completedDomains * 100)
		if completedDomains < progressDetail.TotalDomains && progressDetail.CurrentDomain != "" {
			overallNumerator += int64(progressDetail.CurrentDomainProgress)
		}
		progressDetail.OverallProgress = clampProgress(int(overallNumerator / int64(progressDetail.TotalDomains)))
	}

	persistProgressLocked := func(force bool) {
		refreshProgressDetail()
		if !persistProgress {
			return
		}
		if !force && currentCount%10 != 0 && time.Since(lastProgressUpdate) < progressUpdateInterval {
			return
		}
		lastProgressUpdate = time.Now()
		if err := UpdateTaskProgressWithDetail(
			taskID,
			progressDetail.OverallProgress,
			int(currentCount),
			int(totalCount),
			int(persistedResultCount),
			&progressDetail,
		); err != nil {
			logger.Error("更新任务进度失败: %v", err)
		}
	}

	flushResultsLocked := func() {
		if len(pendingResults) == 0 {
			return
		}
		batch := append([]SubdomainResult(nil), pendingResults...)
		if err := BatchSaveScanResults(taskID, batch); err != nil {
			logger.Error("批量保存结果失败: %v", err)
			return
		}
		pendingResults = pendingResults[:0]
		persistedResultCount += int64(len(batch))
		persistProgressLocked(true)
	}

	callback := func(event string, data interface{}) {
		mu.Lock()
		defer mu.Unlock()

		switch event {
		case "domainStart":
			payload := eventMap(data)
			if payload == nil {
				return
			}
			domain := eventString(payload["domain"])
			if domain == "" {
				return
			}
			if totalDomains := toInt(payload["totalDomains"]); totalDomains > 0 {
				progressDetail.TotalDomains = totalDomains
			}
			progressDetail.CurrentDomain = domain
			progressDetail.CurrentDomainIndex = toInt(payload["domainIndex"])
			progressDetail.CurrentDomainTotal = 0
			progressDetail.CurrentDomainResolved = 0
			progressDetail.CurrentDomainProgress = 0
			persistProgressLocked(true)
		case "domainResolveStart":
			payload := eventMap(data)
			if payload == nil {
				return
			}
			if domain := eventString(payload["domain"]); domain != "" {
				progressDetail.CurrentDomain = domain
			}
			totalSubdomains := toInt(payload["totalSubdomains"])
			if totalSubdomains > 0 {
				totalCount += int64(totalSubdomains)
				progressDetail.CurrentDomainTotal += totalSubdomains
			}
			persistProgressLocked(true)
		case "subdomainLoading":
			if result, ok := data.(SubdomainResult); ok {
				key := result.Domain + "\x00" + result.Subdomain
				if pendingIndex, exists := seenResults[key]; exists {
					if pendingIndex >= 0 && pendingIndex < len(pendingResults) {
						mergedSources := append(pendingResults[pendingIndex].Sources, result.Sources...)
						pendingResults[pendingIndex].Sources = normalizeSources(mergedSources, result.Source)
						pendingResults[pendingIndex].Source = firstSource(pendingResults[pendingIndex].Sources)
					}
					return
				}
				result.Sources = normalizeSources(result.Sources, result.Source)
				result.Source = firstSource(result.Sources)
				seenResults[key] = len(pendingResults)
				pendingResults = append(pendingResults, result)
				if len(pendingResults) >= realtimeResultBatchSize {
					flushResultsLocked()
					for seenKey := range seenResults {
						seenResults[seenKey] = -1
					}
				}
			}
		case "subdomainSourceUpdate":
			payload := eventMap(data)
			if payload == nil {
				return
			}
			domain := eventString(payload["domain"])
			source := eventString(payload["source"])
			subdomains := toStringSlice(payload["subdomains"])
			if err := AddScanResultSource(taskID, domain, subdomains, source); err != nil {
				logger.Error("更新子域名来源失败: %v", err)
			}
		case "subdomainProgressID":
			currentCount++
			if payload := eventMap(data); payload != nil {
				if domain := eventString(payload["domain"]); domain != "" {
					progressDetail.CurrentDomain = domain
				}
			}
			progressDetail.CurrentDomainResolved++
			if progressDetail.CurrentDomainResolved > progressDetail.CurrentDomainTotal {
				progressDetail.CurrentDomainTotal = progressDetail.CurrentDomainResolved
			}
			persistProgressLocked(false)
		case "domainResolveComplete":
			flushResultsLocked()
			persistProgressLocked(true)
		case "domainComplete":
			flushResultsLocked()
			payload := eventMap(data)
			if payload != nil {
				if totalDomains := toInt(payload["totalDomains"]); totalDomains > 0 {
					progressDetail.TotalDomains = totalDomains
				}
				if domainIndex := toInt(payload["domainIndex"]); domainIndex > progressDetail.CompletedDomains {
					progressDetail.CompletedDomains = domainIndex
				} else if progressDetail.CompletedDomains < progressDetail.TotalDomains {
					progressDetail.CompletedDomains++
				}
				if domain := eventString(payload["domain"]); domain != "" {
					progressDetail.CurrentDomain = domain
				}
			} else if progressDetail.CompletedDomains < progressDetail.TotalDomains {
				progressDetail.CompletedDomains++
			}
			progressDetail.CurrentDomainResolved = progressDetail.CurrentDomainTotal
			persistProgressLocked(true)
		}
	}

	engine := NewSubdomainEngine(ctx, req, callback)
	func() {
		defer func() {
			if r := recover(); r != nil {
				scanFailed = true
				scanErr = fmt.Errorf("扫描任务发生错误: %v", r)
				logger.Error("扫描任务发生panic: %v", r)
			}
		}()
		engine.ApiPolymerization(ctx)
	}()

	if scanFailed {
		return scanErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	mu.Lock()
	flushResultsLocked()
	if progressDetail.TotalDomains > 0 {
		progressDetail.CompletedDomains = progressDetail.TotalDomains
	}
	progressDetail.CurrentDomainResolved = progressDetail.CurrentDomainTotal
	refreshProgressDetail()
	progressDetail.OverallProgress = 100
	finalResultCount := persistedResultCount
	finalCurrentCount := currentCount
	finalTotalCount := totalCount
	finalProgressDetail := progressDetail
	mu.Unlock()

	if persistProgress {
		if err := UpdateTaskProgressWithDetail(taskID, 100, int(finalCurrentCount), int(finalTotalCount), int(finalResultCount), &finalProgressDetail); err != nil {
			logger.Error("更新最终进度失败: %v", err)
		}
	}
	return nil
}
