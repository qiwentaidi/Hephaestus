package subdomain

import (
	"net"
	"strings"
)

// A root using wildcard DNS does not make every child a wildcard result.
// Retain the observed answers so independently resolved children stay eligible
// for downstream scanning. Multiple probes cover rotating wildcard answers.
type wildcardDNSBaseline struct {
	ips    map[string]struct{}
	cnames map[string]struct{}
}

func probeWildcardDNS(domain string, dnsServers []string, timeout int) wildcardDNSBaseline {
	baseline := wildcardDNSBaseline{ips: make(map[string]struct{}), cnames: make(map[string]struct{})}
	root := normalizeRootDomain(domain)
	if root == "" || len(dnsServers) == 0 {
		return baseline
	}
	for i := 0; i < 3; i++ {
		ips, cnames, err := resolveDNSRecords(generateRandomSubdomain(root), dnsServers, timeout)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if parsed := net.ParseIP(strings.TrimSpace(ip)); parsed != nil {
				baseline.ips[parsed.String()] = struct{}{}
			}
		}
		for _, cname := range cnames {
			if name := normalizeWildcardCNAME(cname); name != "" {
				baseline.cnames[name] = struct{}{}
			}
		}
	}
	return baseline
}

func (b wildcardDNSBaseline) detected() bool {
	return len(b.ips) > 0 || len(b.cnames) > 0
}

func (b wildcardDNSBaseline) matches(ips, cnames []string) bool {
	// A distinct CNAME is independent DNS evidence even when its IP is shared.
	if len(cnames) > 0 {
		for _, cname := range cnames {
			if _, ok := b.cnames[normalizeWildcardCNAME(cname)]; !ok {
				return false
			}
		}
		return true
	}
	if len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		parsed := net.ParseIP(strings.TrimSpace(ip))
		if parsed == nil {
			return false
		}
		if _, ok := b.ips[parsed.String()]; !ok {
			return false
		}
	}
	return true
}

func normalizeWildcardCNAME(cname string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cname)), ".")
}
