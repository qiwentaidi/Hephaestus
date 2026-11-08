package subdomain

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hephaestus-backend/lib/cdncheck"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/subdomain/anubis"
	"hephaestus-backend/pkg/subdomain/bevigil"
	"hephaestus-backend/pkg/subdomain/chaos"
	"hephaestus-backend/pkg/subdomain/crtname"
	"hephaestus-backend/pkg/subdomain/digitorus"
	// "hephaestus-backend/pkg/subdomain/ctradar" // 已下线，暂时注释
	"hephaestus-backend/pkg/subdomain/fofa"
	"hephaestus-backend/pkg/subdomain/github"
	"hephaestus-backend/pkg/subdomain/hackertarget"
	"hephaestus-backend/pkg/subdomain/httpclient"
	"hephaestus-backend/pkg/subdomain/hunter"
	"hephaestus-backend/pkg/subdomain/ip138"
	"hephaestus-backend/pkg/subdomain/quake"
	"hephaestus-backend/pkg/subdomain/securitytrails"
	"hephaestus-backend/pkg/subdomain/thc"
	"hephaestus-backend/pkg/subdomain/zoomeye"
	"hephaestus-backend/pkg/types"

	"bytes"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "embed"

	"github.com/panjf2000/ants/v2"
	arrayutil "github.com/qiwentaidi/utils/array"
)

// Callback 回调函数类型
type Callback func(event string, data interface{})

type Subdomain struct {
	ctx      context.Context
	options  *types.SubdomainOption
	callback Callback // 回调函数，用于发送数据到前端
}

type SubdomainResult struct {
	Domain          string   `json:"domain"`                    // 主域名
	Subdomain       string   `json:"subdomain"`                 // 子域名
	IPs             []string `json:"ips"`                       // IP地址列表
	AssetType       string   `json:"assetType"`                 // 资产类型：CDN、WAF、Cloud、空字符串
	AssetName       string   `json:"assetName"`                 // 资产名称
	Source          string   `json:"source"`                    // 数据来源
	Sources         []string `json:"sources"`                   // 数据来源列表
	IsWildcard      bool     `json:"isWildcard"`                // 是否命中泛解析
	ResolutionError string   `json:"resolutionError,omitempty"` // 多 DNS 均失败时保留真实错误
}

var (
	// DefaultDnsServers 默认DNS服务器列表（包含国内外多个公共DNS服务器）
	DefaultDnsServers = []string{
		// 国内DNS服务器
		"223.6.6.6:53",       // 阿里云DNS
		"119.29.29.29:53",    // 腾讯DNS
		"114.114.114.114:53", // 114DNS
		"180.76.76.76:53",    // 百度DNS
		"1.2.4.8:53",         // 114DNS备用
		"101.226.4.6:53",     // 腾讯DNS备用
		// 国外DNS服务器
		"8.8.8.8:53",        // Google DNS
		"8.8.4.4:53",        // Google DNS备用
		"1.1.1.1:53",        // Cloudflare DNS
		"1.0.0.1:53",        // Cloudflare DNS备用
		"9.9.9.9:53",        // Quad9 DNS
		"208.67.222.222:53", // OpenDNS
		"208.67.220.220:53", // OpenDNS备用
	}
)

var hostnameLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

var resolveDNSRecords = resolveDNSWithRetry

// NewSubdomainEngine 创建子域名扫描引擎
// callback 用于接收扫描结果和进度更新，event 可以是: "subdomainLoading", "subdomainProgressID", "subdomainCounts", "subdomainComplete"
func NewSubdomainEngine(ctx context.Context, option types.SubdomainOption, callback Callback) *Subdomain {
	subdomain := &Subdomain{
		ctx:      ctx,
		options:  &option,
		callback: callback,
	}
	if len(subdomain.options.DnsServers) == 0 {
		subdomain.options.DnsServers = DefaultDnsServers
	}
	return subdomain
}

// Runner 执行子域名解析，subdomains 为完整的域名列表
func (s *Subdomain) Runner(ctrlCtx context.Context, domain string, subdomains []string, source string) {
	sourcesBySubdomain := make(map[string][]string, len(subdomains))
	for _, subdomain := range subdomains {
		sourcesBySubdomain[subdomain] = []string{source}
	}
	s.RunnerWithSources(ctrlCtx, domain, subdomains, source, sourcesBySubdomain)
}

// RunnerWithSources 执行子域名解析，并保留每个子域名的具体来源列表。
func (s *Subdomain) RunnerWithSources(ctrlCtx context.Context, domain string, subdomains []string, fallbackSource string, sourcesBySubdomain map[string][]string) {
	var skippedInvalid int
	subdomains, skippedInvalid = filterSubdomainCandidates(domain, subdomains)
	if skippedInvalid > 0 {
		logger.Debug("已过滤[%s]的无效子域名候选: %d个", domain, skippedInvalid)
	}
	totalSubdomains := len(subdomains)

	// 发送域名解析开始事件
	if s.callback != nil {
		s.callback("domainResolveStart", map[string]interface{}{
			"domain":          domain,
			"totalSubdomains": totalSubdomains,
		})
	}

	dnsServers := s.options.DnsServers
	if len(dnsServers) == 0 {
		dnsServers = DefaultDnsServers
	}
	wildcard := probeWildcardDNS(domain, dnsServers, s.options.Timeout)
	if wildcard.detected() {
		logger.Info("检测到[%s]存在泛解析", domain)
	}

	single := make(chan struct{})
	retChan := make(chan SubdomainResult)
	var wg sync.WaitGroup
	var id int32
	var resolvedCount int32
	var failedCount int32
	var emptyIPCount int32
	var validIPCount int32

	go func() {
		count := 0
		for sr := range retChan {
			count++
			atomic.AddInt32(&resolvedCount, 1)
			if s.callback != nil {
				s.callback("subdomainLoading", sr)
				// 发送解析进度
				s.callback("domainResolveProgress", map[string]interface{}{
					"domain":          domain,
					"resolved":        int(atomic.LoadInt32(&resolvedCount)),
					"totalSubdomains": totalSubdomains,
				})
			} else {
				logger.Debug("回调函数为 nil，无法发送结果")
			}
		}
		close(single)
		logger.Info(
			"已完成 %s 的解析，共处理 %d/%d 个候选，有效IP结果 %d 个，空IP结果 %d 个，解析失败 %d 个",
			domain,
			count,
			totalSubdomains,
			atomic.LoadInt32(&validIPCount),
			atomic.LoadInt32(&emptyIPCount),
			atomic.LoadInt32(&failedCount),
		)
		if s.callback != nil {
			s.callback("subdomainComplete", fmt.Sprintf("已完成 %s 的解析", domain))
			s.callback("domainResolveComplete", map[string]interface{}{
				"domain":          domain,
				"resolved":        count,
				"totalSubdomains": totalSubdomains,
			})
		}
	}()

	resolutionScan := func(subdomain string) {
		ips, cnames, err := resolveDNSRecords(subdomain, dnsServers, s.options.Timeout)
		resolutionError := ""
		if err != nil {
			atomic.AddInt32(&failedCount, 1)
			resolutionError = err.Error()
			logger.Warning("子域名[%s] DNS解析失败: %v", subdomain, err)
		}

		validIPs := ips

		if err == nil {
			if len(validIPs) == 0 {
				atomic.AddInt32(&emptyIPCount, 1)
			} else {
				atomic.AddInt32(&validIPCount, 1)
			}
		}

		// 检测资产类型（CDN/WAF）
		assetType, assetName := s.DetectAsset(validIPs, cnames)

		// 即使没有有效IP，也发送结果（但标记为空IP）
		// 这样前端可以看到所有解析过的子域名
		sources := resolveSourcesForSubdomain(subdomain, fallbackSource, sourcesBySubdomain)
		result := SubdomainResult{
			Domain:          domain,
			Subdomain:       subdomain,
			IPs:             validIPs,
			AssetType:       assetType,
			AssetName:       assetName,
			Source:          firstSource(sources),
			Sources:         sources,
			IsWildcard:      normalizeRootDomain(subdomain) != normalizeRootDomain(domain) && wildcard.matches(validIPs, cnames),
			ResolutionError: resolutionError,
		}

		retChan <- result
	}

	threadPool, _ := ants.NewPoolWithFunc(s.options.Thread, func(p interface{}) {
		subdomain := p.(string)
		defer wg.Done()

		resolutionScan(subdomain)
		processed := atomic.AddInt32(&id, 1)
		if s.callback != nil {
			s.callback("subdomainProgressID", map[string]interface{}{
				"domain":          domain,
				"processed":       int(processed),
				"totalSubdomains": totalSubdomains,
			})
		}
	})
	defer threadPool.Release()

	if s.callback != nil {
		s.callback("subdomainCounts", len(subdomains))
	}

	for _, subdomain := range subdomains {
		if ctrlCtx.Err() != nil {
			return
		}
		wg.Add(1)
		threadPool.Invoke(subdomain)
	}

	wg.Wait()
	close(retChan)
	<-single
}

func resolveSourcesForSubdomain(subdomain string, fallbackSource string, sourcesBySubdomain map[string][]string) []string {
	if sourcesBySubdomain != nil {
		if sources := normalizeSources(sourcesBySubdomain[subdomain], fallbackSource); len(sources) > 0 {
			return sources
		}
	}
	return normalizeSources(nil, fallbackSource)
}

func firstSource(sources []string) string {
	if len(sources) == 0 {
		return ""
	}
	return sources[0]
}

func normalizeSources(sources []string, fallbackSource string) []string {
	seen := make(map[string]struct{}, len(sources)+1)
	result := make([]string, 0, len(sources)+1)
	appendSource := func(source string) {
		source = strings.TrimSpace(source)
		if source == "" {
			return
		}
		if _, exists := seen[source]; exists {
			return
		}
		seen[source] = struct{}{}
		result = append(result, source)
	}
	for _, source := range sources {
		appendSource(source)
	}
	if len(result) == 0 {
		appendSource(fallbackSource)
	}
	return result
}

func appendEngineEnabled(engines []string, names ...string) bool {
	for _, engine := range engines {
		normalizedEngine := strings.ToLower(strings.TrimSpace(engine))
		for _, name := range names {
			if normalizedEngine == strings.ToLower(strings.TrimSpace(name)) {
				return true
			}
		}
	}
	return false
}

func filterSubdomainCandidates(domain string, candidates []string) ([]string, int) {
	filtered := make([]string, 0, len(candidates))
	skipped := 0
	for _, candidate := range candidates {
		normalized, ok := normalizeSubdomainCandidate(domain, candidate)
		if !ok {
			skipped++
			continue
		}
		filtered = append(filtered, normalized)
	}
	return filtered, skipped
}

func normalizeSubdomainCandidate(domain, candidate string) (string, bool) {
	rootDomain := normalizeRootDomain(domain)
	host := strings.TrimSpace(candidate)
	if rootDomain == "" || host == "" {
		return "", false
	}

	host = strings.Trim(host, "\"'`[]()<>")
	if strings.Contains(host, "://") || strings.HasPrefix(host, "//") {
		parsed, err := url.Parse(host)
		if err != nil {
			return "", false
		}
		host = parsed.Hostname()
	} else {
		if idx := strings.IndexAny(host, "/?#"); idx >= 0 {
			host = host[:idx]
		}
		if strings.Contains(host, "@") {
			return "", false
		}
		if strings.Contains(host, ":") {
			parsed, err := url.Parse("//" + host)
			if err != nil {
				return "", false
			}
			host = parsed.Hostname()
		}
	}

	host = strings.TrimSpace(strings.ToLower(host))
	if strings.HasPrefix(host, "*.") {
		host = strings.TrimPrefix(host, "*.")
		if host == rootDomain {
			return "", false
		}
	}
	if strings.Contains(host, "*") {
		return "", false
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" || !(host == rootDomain || strings.HasSuffix(host, "."+rootDomain)) {
		return "", false
	}
	if !isValidHostname(host) {
		return "", false
	}
	return host, true
}

func normalizeRootDomain(domain string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(domain)), ".")
}

func qualifySubdomain(domain, subdomain string) string {
	rootDomain := normalizeRootDomain(domain)
	candidate := strings.TrimSpace(subdomain)
	candidate = strings.TrimSuffix(candidate, ".")
	normalizedCandidate := strings.ToLower(candidate)
	if candidate == "" || rootDomain == "" || normalizedCandidate == rootDomain || strings.HasSuffix(normalizedCandidate, "."+rootDomain) {
		return candidate
	}
	return candidate + "." + rootDomain
}

func isValidHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || !hostnameLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

// detectWildcard reports whether random children under the scan root resolve.
func detectWildcard(domain string, dnsServers []string, timeout int) bool {
	return probeWildcardDNS(domain, dnsServers, timeout).detected()
}

func generateRandomSubdomain(rootDomain string) string {
	b := make([]byte, 8)
	if _, err := cryptorand.Read(b); err != nil {
		return fmt.Sprintf("%d.%s", time.Now().UnixNano(), rootDomain)
	}
	return hex.EncodeToString(b) + "." + rootDomain
}

// DetectAsset 检测资产类型（CDN/WAF/Cloud）和名称
// 使用cdncheck包进行检测，支持基于CNAME和IP CIDR的识别
func (s *Subdomain) DetectAsset(ips, cnames []string) (assetType string, assetName string) {
	// 使用cdncheck包检测CNAME
	if tag, detected := cdncheck.DetectWithCnameAndIp(ips, cnames); detected {
		// 转换assetType为大写格式（CDN/WAF/CLOUD）
		assetTypeUpper := strings.ToUpper(tag.AssetType)
		if assetTypeUpper == "COMMON" {
			// common类型一般为CDN服务
			assetTypeUpper = "CDN"
		}
		return assetTypeUpper, tag.ProductName
	}
	return "", ""
}

func (s *Subdomain) ApiPolymerization(ctrlCtx context.Context) {
	totalDomains := len(s.options.Domains)
	for idx, domain := range s.options.Domains {
		// 发送域名开始处理事件
		if s.callback != nil {
			s.callback("domainStart", map[string]interface{}{
				"domain":       domain,
				"domainIndex":  idx + 1,
				"totalDomains": totalDomains,
			})
		}

		var subdomains []string
		sourcesBySubdomain := make(map[string][]string)
		apiResults := make(map[string]int) // API名称 -> 收集到的子域名数量
		filteredInvalidCount := 0
		addSource := func(subdomain string, source string) {
			if source == "" {
				return
			}
			sources := sourcesBySubdomain[subdomain]
			if !arrayutil.ArrayContains(source, sources) {
				sourcesBySubdomain[subdomain] = append(sources, source)
			}
		}
		appendCandidates := func(candidates []string, source string) int {
			filtered, skipped := filterSubdomainCandidates(domain, candidates)
			filteredInvalidCount += skipped
			subdomains = append(subdomains, filtered...)
			for _, subdomain := range filtered {
				addSource(subdomain, source)
			}
			return len(filtered)
		}
		appendQualifiedCandidates := func(candidates []string, source string) int {
			qualified := make([]string, 0, len(candidates))
			for _, candidate := range candidates {
				qualified = append(qualified, qualifySubdomain(domain, candidate))
			}
			return appendCandidates(qualified, source)
		}

		// Chaos API
		if appendEngineEnabled(s.options.AppendEngines, "Chaos", "chaos") && s.options.ChaosApi != "" {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "Chaos",
				})
			}
			ch := chaos.FetchHostsWithProxy(domain, s.options.ChaosApi, s.options.Proxy)
			count := 0
			if ch != nil {
				count = appendQualifiedCandidates(ch.Subdomains, "Chaos")
			}
			apiResults["Chaos"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "Chaos",
					"count":  count,
				})
			}
		}

		// FOFA API
		if arrayutil.ArrayContains("FOFA", s.options.AppendEngines) && s.options.FofaApi != "" {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "FOFA",
				})
			}
			fh := fofa.FetchHostsWithProxy(domain, types.FofaAuth{
				Address: s.options.FofaAddress,
				Email:   s.options.FofaEmail,
				Key:     s.options.FofaApi,
			}, s.options.Proxy)
			count := appendCandidates(fh, "FOFA")
			apiResults["FOFA"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "FOFA",
					"count":  count,
				})
			}
		}

		// Hunter API
		if arrayutil.ArrayContains("Hunter", s.options.AppendEngines) && s.options.HunterApi != "" {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "Hunter",
				})
			}
			hh := hunter.FetchHostsWithProxy(domain, s.options.HunterApi, s.options.Proxy)
			count := appendCandidates(hh, "Hunter")
			apiResults["Hunter"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "Hunter",
					"count":  count,
				})
			}
		}

		// Quake API
		if arrayutil.ArrayContains("Quake", s.options.AppendEngines) && s.options.QuakeApi != "" {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "Quake",
				})
			}
			qh := quake.FetchHostsWithProxy(domain, s.options.QuakeApi, s.options.Proxy)
			count := appendCandidates(qh, "Quake")
			apiResults["Quake"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "Quake",
					"count":  count,
				})
			}
		}

		// Bevigil API
		if appendEngineEnabled(s.options.AppendEngines, "Bevigil", "bevigil") && s.options.BevigilApi != "" {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "Bevigil",
				})
			}
			bh := bevigil.FetchHostsWithProxy(domain, s.options.BevigilApi, s.options.Proxy)
			count := 0
			if bh != nil {
				count = appendCandidates(bh.Subdomains, "Bevigil")
			}
			apiResults["Bevigil"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "Bevigil",
					"count":  count,
				})
			}
		}

		// ZoomEye API
		if arrayutil.ArrayContains("ZoomEye", s.options.AppendEngines) && s.options.ZoomeyeApi != "" {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "ZoomEye",
				})
			}
			zh := zoomeye.FetchHostsWithProxy(domain, s.options.ZoomeyeAddress, s.options.ZoomeyeApi, s.options.Proxy)
			count := 0
			if zh != nil {
				candidates := make([]string, 0, len(zh.List))
				for _, item := range zh.List {
					candidates = append(candidates, item.Name)
				}
				count = appendCandidates(candidates, "ZoomEye")
			}
			apiResults["ZoomEye"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "ZoomEye",
					"count":  count,
				})
			}
		}

		// GitHub API
		if appendEngineEnabled(s.options.AppendEngines, "GitHub", "github") && s.options.GithubApi != "" {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "GitHub",
				})
			}
			result := github.FetchHostsWithProxy(domain, s.options.GithubApi, s.options.Proxy)
			count := appendCandidates(result, "GitHub")
			apiResults["GitHub"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "GitHub",
					"count":  count,
				})
			}
		}

		// Securitytrails API
		if appendEngineEnabled(s.options.AppendEngines, "Securitytrails", "securitytrails") && s.options.SecuritytrailsApi != "" {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "Securitytrails",
				})
			}
			sh := securitytrails.FetchHostsWithProxy(domain, s.options.SecuritytrailsApi, s.options.Proxy)
			count := 0
			if sh != nil {
				count = appendQualifiedCandidates(sh.Subdomains, "Securitytrails")
			}
			apiResults["Securitytrails"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "Securitytrails",
					"count":  count,
				})
			}
		}

		// CT-Radar API (API Key optional)
		// 已下线，暂时注释
		// if arrayutil.ArrayContains("CT-Radar", s.options.AppendEngines) {
		// 	if s.callback != nil {
		// 		s.callback("apiStart", map[string]interface{}{
		// 			"domain": domain,
		// 			"api":    "CT-Radar",
		// 		})
		// 	}
		// 	ch := ctradar.FetchHosts(ctrlCtx, domain, s.options.CTRadarApi)
		// 	count := len(ch)
		// 	subdomains = append(subdomains, ch...)
		// 	apiResults["CT-Radar"] = count
		// 	if s.callback != nil {
		// 		s.callback("apiComplete", map[string]interface{}{
		// 			"domain": domain,
		// 			"api":    "CT-Radar",
		// 			"count":  count,
		// 		})
		// 	}
		// }

		// crt.name API (免费，无需API Key，单IP每日配额有限)
		if appendEngineEnabled(s.options.AppendEngines, "crt.name", "crtname", "CRTName") {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "crt.name",
				})
			}
			ch := crtname.FetchHostsWithProxy(ctrlCtx, domain, s.options.Proxy)
			count := appendCandidates(ch, "crt.name")
			apiResults["crt.name"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "crt.name",
					"count":  count,
				})
			}
		}

		// Digitorus/certificatedetails (free, no API key required)
		if appendEngineEnabled(s.options.AppendEngines, "Digitorus", "digitorus") {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "Digitorus",
				})
			}
			dh := digitorus.FetchHostsWithProxy(ctrlCtx, domain, s.options.Proxy)
			count := appendCandidates(dh, "Digitorus")
			apiResults["Digitorus"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "Digitorus",
					"count":  count,
				})
			}
		}

		// HackerTarget hostsearch (free, no API key required)
		if appendEngineEnabled(s.options.AppendEngines, "HackerTarget", "hackertarget") {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "HackerTarget",
				})
			}
			hh := hackertarget.FetchHostsWithProxy(ctrlCtx, domain, s.options.Proxy)
			count := appendCandidates(hh, "HackerTarget")
			apiResults["HackerTarget"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "HackerTarget",
					"count":  count,
				})
			}
		}

		// THC passive subdomain lookup (free, no API key required)
		if appendEngineEnabled(s.options.AppendEngines, "THC", "thc") {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "THC",
				})
			}
			th := thc.FetchHostsWithProxy(ctrlCtx, domain, s.options.Proxy)
			count := appendCandidates(th, "THC")
			apiResults["THC"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "THC",
					"count":  count,
				})
			}
		}

		// AnubisDB (free, no API key required)
		if appendEngineEnabled(s.options.AppendEngines, "Anubis", "anubis") {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "Anubis",
				})
			}
			ah := anubis.FetchHostsWithProxy(ctrlCtx, domain, s.options.Proxy)
			count := appendCandidates(ah, "Anubis")
			apiResults["Anubis"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "Anubis",
					"count":  count,
				})
			}
		}

		// IP138 (免费，无需API)
		if appendEngineEnabled(s.options.AppendEngines, "IP138", "ip138") {
			if s.callback != nil {
				s.callback("apiStart", map[string]interface{}{
					"domain": domain,
					"api":    "IP138",
				})
			}
			domains := ip138.FetchHostsWithProxy(s.ctx, domain, s.options.Proxy)
			count := appendCandidates(domains, "IP138")
			apiResults["IP138"] = count
			if s.callback != nil {
				s.callback("apiComplete", map[string]interface{}{
					"domain": domain,
					"api":    "IP138",
					"count":  count,
				})
			}
		}

		// 去重
		subdomains = arrayutil.RemoveDuplicates(subdomains)
		if filteredInvalidCount > 0 {
			logger.Info("已从API获取到[%s]的有效子域名: %d个（过滤无效 %d 个），正在验证存活", domain, len(subdomains), filteredInvalidCount)
		} else {
			logger.Info("已从API获取到[%s]的有效子域名: %d个，正在验证存活", domain, len(subdomains))
		}

		// 发送域名API收集完成事件
		if s.callback != nil {
			s.callback("domainApiComplete", map[string]interface{}{
				"domain":     domain,
				"totalCount": len(subdomains),
				"apiResults": apiResults,
			})
		}

		// 先解析API收集的子域名
		s.RunnerWithSources(ctrlCtx, domain, subdomains, "API", sourcesBySubdomain)

		// AI分析：分析不规则子域名并生成更多可能的子域名
		if s.options.AIEnabled && s.options.AIApiUrl != "" && s.options.AIApiKey != "" {
			if s.callback != nil {
				s.callback("aiAnalysisStart", map[string]interface{}{
					"domain": domain,
				})
			}

			aiSubdomains := s.AnalyzeWithAI(ctrlCtx, domain, subdomains)
			if filteredAISubdomains, skippedAI := filterSubdomainCandidates(domain, aiSubdomains); skippedAI > 0 {
				logger.Debug("已过滤AI生成的无效子域名候选: %d个", skippedAI)
				aiSubdomains = filteredAISubdomains
			} else {
				aiSubdomains = filteredAISubdomains
			}
			if len(aiSubdomains) > 0 {
				logger.Info("AI分析生成了 %d 个新的子域名候选", len(aiSubdomains))

				// 去重：过滤掉已经存在的子域名
				newAISubdomains := make([]string, 0)
				existingAISubdomains := make([]string, 0)
				for _, sub := range aiSubdomains {
					if !arrayutil.ArrayContains(sub, subdomains) {
						newAISubdomains = append(newAISubdomains, sub)
					} else {
						existingAISubdomains = append(existingAISubdomains, sub)
					}
				}
				if len(existingAISubdomains) > 0 && s.callback != nil {
					s.callback("subdomainSourceUpdate", map[string]interface{}{
						"domain":     domain,
						"subdomains": existingAISubdomains,
						"source":     "AI",
					})
				}

				if len(newAISubdomains) > 0 {
					logger.Info("AI生成了 %d 个新子域名（过滤重复后）", len(newAISubdomains))

					// 单独解析AI生成的子域名，来源标记为"AI"
					s.Runner(ctrlCtx, domain, newAISubdomains, "AI")

					if s.callback != nil {
						s.callback("aiAnalysisComplete", map[string]interface{}{
							"domain":         domain,
							"generatedCount": len(newAISubdomains),
						})
					}
				} else {
					logger.Info("AI生成的子域名全部已存在")
					if s.callback != nil {
						s.callback("aiAnalysisComplete", map[string]interface{}{
							"domain":         domain,
							"generatedCount": 0,
						})
					}
				}
			} else {
				if s.callback != nil {
					s.callback("aiAnalysisComplete", map[string]interface{}{
						"domain":         domain,
						"generatedCount": 0,
					})
				}
			}
		}

		if s.callback != nil {
			s.callback("domainComplete", map[string]interface{}{
				"domain":       domain,
				"domainIndex":  idx + 1,
				"totalDomains": totalDomains,
			})
		}

		time.Sleep(time.Second)
	}
}

// AnalyzeWithAI 使用AI分析子域名模式并生成新的子域名候选
func (s *Subdomain) AnalyzeWithAI(ctx context.Context, domain string, existingSubdomains []string) []string {
	if len(existingSubdomains) == 0 {
		return nil
	}

	// 筛选出不规则的子域名（包含数字、连字符或特殊模式的）
	irregularSubdomains := make([]string, 0)
	for _, sub := range existingSubdomains {
		// 移除域名后缀，只分析子域名部分
		subPart := strings.TrimSuffix(sub, "."+domain)
		if subPart == sub {
			continue // 不包含基域名，跳过
		}

		// 判断是否为不规则子域名
		// 1. 包含数字
		// 2. 包含连字符
		// 3. 包含k8s/prod/test/dev等关键词
		if regexp.MustCompile(`\d`).MatchString(subPart) ||
			strings.Contains(subPart, "-") ||
			regexp.MustCompile(`(?i)(k8s|kube|prod|test|dev|stage|staging|uat|pre|beta|alpha|v\d+)`).MatchString(subPart) {
			irregularSubdomains = append(irregularSubdomains, sub)
		}
	}

	// 如果没有不规则子域名，直接返回
	if len(irregularSubdomains) == 0 {
		logger.Info("未发现不规则子域名，跳过AI分析")
		return nil
	}

	// 限制样本数量，避免过多的token消耗
	maxSamples := 30
	if len(irregularSubdomains) > maxSamples {
		irregularSubdomains = irregularSubdomains[:maxSamples]
	}

	logger.Info("发现 %d 个不规则子域名，正在使用AI分析...", len(irregularSubdomains))

	// 构造提示词
	prompt := fmt.Sprintf(`你是一个安全专家，擅长分析子域名命名模式。

对于域名: %s
我发现了以下不规则的子域名：
%s

请分析这些子域名的命名模式（比如k8s命名、环境标识、版本号等），并生成更多可能存在的子域名变体。

请注意：
1. 只生成完整的子域名（包含.%s后缀）
2. 每行一个子域名
3. 不要生成已经存在的子域名
4. 生成的20-50个新的子域名即可
5. 不要添加任何解释或注释，直接输出子域名列表

请直接输出子域名列表：`, domain, strings.Join(irregularSubdomains, "\n"), domain)

	// 调用OpenAI兼容API
	generatedSubdomains := s.callOpenAICompatibleAPI(ctx, prompt)

	if len(generatedSubdomains) > 0 {
		logger.Info("AI生成了 %d 个新子域名", len(generatedSubdomains))
	}

	return generatedSubdomains
}

// callOpenAICompatibleAPI 调用OpenAI兼容的API
func (s *Subdomain) callOpenAICompatibleAPI(ctx context.Context, prompt string) []string {
	// 构造请求体
	requestBody := map[string]interface{}{
		"model": s.options.AIModel,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.7,
		"max_tokens":  2000,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		logger.Warning("JSON序列化失败: %v", err)
		return nil
	}

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", s.options.AIApiUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Warning("HTTP请求创建失败: %v", err)
		return nil
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.options.AIApiKey)

	// 发送请求
	client := httpclient.New(s.options.Proxy, 60*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		logger.Warning("AI API请求失败: %v", err)
		return nil
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Warning("读取响应失败: %v", err)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		logger.Warning("AI API返回错误: %d, 响应: %s", resp.StatusCode, string(body))
		return nil
	}

	// 解析响应
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		logger.Warning("JSON解析失败: %v", err)
		return nil
	}

	if len(response.Choices) == 0 {
		logger.Warning("AI响应为空")
		return nil
	}

	// 解析AI返回的子域名列表
	content := response.Choices[0].Message.Content
	lines := strings.Split(content, "\n")

	generatedSubdomains := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 过滤注释和非子域名内容
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") ||
			strings.Contains(line, "：") || strings.Contains(line, ":") ||
			!strings.Contains(line, ".") {
			continue
		}

		// 去除markdown符号和序号
		line = regexp.MustCompile(`^[\d\.\-\*\s]+`).ReplaceAllString(line, "")
		line = strings.TrimSpace(line)

		// 验证是否为有效的子域名格式
		if regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)*$`).MatchString(line) {
			generatedSubdomains = append(generatedSubdomains, line)
		}
	}

	return generatedSubdomains
}
