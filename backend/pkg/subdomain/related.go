package subdomain

import (
	"context"
	"sort"
	"strings"
	"sync"

	"hephaestus-backend/pkg/logger"

	"github.com/panjf2000/ants/v2"
)

const maxRelatedDomains = 300

// RunnerRelated resolves certificate-related domains and emits them through the
// same subdomain result callback. Unlike RunnerWithSources, it does not filter
// candidates by the target root domain: each related domain is stored as its
// own root (Domain == Subdomain), which lets the existing scan result table
// hold it without a dedicated schema.
func (s *Subdomain) RunnerRelated(ctrlCtx context.Context, domains []string, source string) {
	if len(domains) == 0 || s.callback == nil {
		return
	}

	dnsServers := s.options.DnsServers
	if len(dnsServers) == 0 {
		dnsServers = DefaultDnsServers
	}
	thread := s.options.Thread
	if thread <= 0 {
		thread = 50
	}
	if thread > len(domains) {
		thread = len(domains)
	}

	var wg sync.WaitGroup
	pool, err := ants.NewPoolWithFunc(thread, func(value interface{}) {
		defer wg.Done()
		host := value.(string)
		if ctrlCtx.Err() != nil {
			return
		}

		ips, cnames, resolveErr := resolveDNSRecords(host, dnsServers, s.options.Timeout)
		resolutionError := ""
		if resolveErr != nil {
			resolutionError = resolveErr.Error()
		}
		assetType, assetName := s.DetectAsset(ips, cnames)
		s.callback("subdomainLoading", SubdomainResult{
			Domain:          host,
			Subdomain:       host,
			IPs:             ips,
			AssetType:       assetType,
			AssetName:       assetName,
			Source:          source,
			Sources:         []string{source},
			ResolutionError: resolutionError,
		})
	})
	if err != nil {
		logger.Warning("初始化关联域名解析池失败: %v", err)
		return
	}
	defer pool.Release()

	for _, host := range domains {
		if ctrlCtx.Err() != nil {
			break
		}
		wg.Add(1)
		if err := pool.Invoke(host); err != nil {
			wg.Done()
			logger.Warning("提交关联域名[%s]解析失败: %v", host, err)
		}
	}
	wg.Wait()
}

// filterRelatedDomains keeps valid domains that are not covered by any scan
// target, removes duplicates, drops names that are subdomains of another
// related domain, and caps the result to keep shared CDN certificates from
// flooding the result table.
func filterRelatedDomains(candidates []string, targets []string) []string {
	targetRoots := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if root := normalizeRootDomain(target); root != "" {
			targetRoots[root] = struct{}{}
		}
	}

	uniq := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		host, ok := normalizeRelatedHost(candidate)
		if !ok {
			continue
		}
		underTarget := false
		for root := range targetRoots {
			if host == root || strings.HasSuffix(host, "."+root) {
				underTarget = true
				break
			}
		}
		if underTarget {
			continue
		}
		uniq[host] = struct{}{}
	}

	hosts := make([]string, 0, len(uniq))
	for host := range uniq {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)

	result := make([]string, 0, len(hosts))
	for _, host := range hosts {
		covered := false
		suffix := host
		for {
			index := strings.IndexByte(suffix, '.')
			if index < 0 {
				break
			}
			suffix = suffix[index+1:]
			if _, exists := uniq[suffix]; exists {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		result = append(result, host)
		if len(result) >= maxRelatedDomains {
			break
		}
	}
	return result
}

func normalizeRelatedHost(host string) (string, bool) {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimPrefix(host, "*.")
	host = strings.TrimSuffix(host, ".")
	if host == "" || strings.ContainsAny(host, " \t\r\n") || !strings.Contains(host, ".") {
		return "", false
	}
	if !isValidHostname(host) {
		return "", false
	}
	return host, true
}
