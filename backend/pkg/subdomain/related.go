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
// candidates by the target root domain. Each related host is stored under its
// closest related domain: the related domain itself becomes the root, and its
// SAN subdomains are stored as its subdomains.
func (s *Subdomain) RunnerRelated(ctrlCtx context.Context, domains []string, source string) {
	if len(domains) == 0 || s.callback == nil {
		return
	}

	dnsServers := s.options.DnsServers
	if len(dnsServers) == 0 {
		dnsServers = DefaultDnsServers
	}
	roots := relatedRoots(domains)
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
			Domain:          roots[host],
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

// relatedRoots maps each related host to the closest related domain that covers
// it. A host with no related parent (for example m.testexample.com when only
// m.testexample.com is present) is its own root.
func relatedRoots(domains []string) map[string]string {
	set := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		set[domain] = struct{}{}
	}
	roots := make(map[string]string, len(domains))
	for _, domain := range domains {
		root := domain
		suffix := domain
		for {
			index := strings.IndexByte(suffix, '.')
			if index < 0 {
				break
			}
			suffix = suffix[index+1:]
			if _, exists := set[suffix]; exists {
				root = suffix
				break
			}
		}
		roots[domain] = root
	}
	return roots
}

// filterRelatedDomains keeps valid domains that are not covered by any scan
// target, removes duplicates, and caps the result to keep shared CDN
// certificates from flooding the result table.
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
	if len(hosts) > maxRelatedDomains {
		hosts = hosts[:maxRelatedDomains]
	}
	return hosts
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
