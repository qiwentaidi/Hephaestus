package findings

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	CategoryWeb         = "web"
	CategoryDNS         = "dns"
	CategoryCertificate = "certificate"
	CategoryService     = "service"
)

type Target struct {
	Type        string // url | domain | ip | service | certificate
	URL         string
	Host        string
	Port        int
	Path        string
	Certificate string
}

type Link struct {
	AssetType  string
	Value      string
	Role       string
	Direct     bool
	Resolver   string
	ResolvedAt *time.Time
}

var certificateFingerprintPattern = regexp.MustCompile(`(?i)\b(?:[0-9a-f]{2}:){15,}[0-9a-f]{2}\b|\b[0-9a-f]{64}\b`)
var domainLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]{0,61}[a-z0-9])?$`)
var urlInTextPattern = regexp.MustCompile(`https?://[^\s，。；、（）()<>"']+`)

func ParseTarget(raw string) Target {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{Type: "unknown"}
	}
	candidates := []string{raw}
	for _, field := range strings.Fields(raw) {
		if field != raw && field != "" {
			candidates = append(candidates, field)
		}
	}
	candidates = append(candidates, urlInTextPattern.FindAllString(raw, 3)...)

	for _, candidate := range candidates {
		candidate = strings.Trim(candidate, "<>()[],")
		if candidate == "" {
			continue
		}
		lower := strings.ToLower(candidate)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			parsed, err := url.Parse(candidate)
			if err != nil || parsed.Hostname() == "" {
				continue
			}
			port, _ := parsePort(parsed.Port())
			return Target{
				Type: "url",
				URL:  canonicalURL(parsed),
				Host: strings.ToLower(strings.TrimSuffix(parsed.Hostname(), ".")),
				Port: port,
				Path: parsed.EscapedPath(),
			}
		}
		if ip := net.ParseIP(strings.Trim(candidate, "[]")); ip != nil {
			return Target{Type: "ip", Host: ip.String()}
		}
		if host, port, err := net.SplitHostPort(candidate); err == nil {
			if parsedPort, err := parsePort(port); err == nil && parsedPort > 0 {
				return Target{Type: "service", Host: strings.ToLower(strings.TrimSuffix(host, ".")), Port: parsedPort}
			}
		}
		if fingerprint := certificateFingerprintPattern.FindString(candidate); fingerprint != "" {
			return Target{Type: "certificate", Certificate: strings.ToLower(fingerprint)}
		}
		if strings.Contains(candidate, ".") && !strings.ContainsAny(candidate, " /?#@") {
			return Target{Type: "domain", Host: strings.ToLower(strings.TrimSuffix(candidate, "."))}
		}
	}
	return Target{Type: "unknown"}
}

func canonicalURL(parsed *url.URL) string {
	copyURL := *parsed
	copyURL.Fragment = ""
	copyURL.Scheme = strings.ToLower(copyURL.Scheme)
	copyURL.Host = strings.ToLower(copyURL.Host)
	return copyURL.String()
}

func parsePort(value string) (int, error) {
	if value == "" {
		return 0, fmt.Errorf("empty port")
	}
	var port int
	_, err := fmt.Sscanf(value, "%d", &port)
	return port, err
}

func isWebPort(port int) bool {
	switch port {
	case 80, 81, 443, 3000, 3001, 5000, 7001, 8000, 8008, 8080, 8081, 8088, 8443, 8888, 9000, 9090:
		return true
	default:
		return port == 0
	}
}

func textContains(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func Classify(title, vulnerabilityType, protocol, targetType string, port int) string {
	text := strings.ToLower(strings.Join([]string{title, vulnerabilityType}, " "))
	switch {
	case textContains(text, "certificate", "cert ", "tls", "ssl证书", "ssl 证书", "证书", "弱加密", "弱协议", "加密套件", "https 配置"):
		return CategoryCertificate
	case textContains(text, "takeover", "接管", "dns", "cname", "spf", "dmarc", "域名解析", "解析记录", "ns记录"):
		return CategoryDNS
	case textContains(text, "redis", "ssh", "mysql", "postgres", "postgresql", "mssql", "mongodb", "elasticsearch", "memcached", "ftp", "smb", "rdp", "ldap", "telnet", "弱口令", "weak password", "default credential", "默认口令"):
		return CategoryService
	case targetType == "service":
		return CategoryService
	case port > 0 && !isWebPort(port):
		return CategoryService
	case targetType == "url":
		return CategoryWeb
	case targetType == "domain":
		return CategoryDNS
	case targetType == "ip":
		return CategoryService
	case targetType == "certificate":
		return CategoryCertificate
	default:
		return CategoryWeb
	}
}

func ServiceValue(host string, port int) string {
	if host == "" || port <= 0 {
		return ""
	}
	return net.JoinHostPort(strings.ToLower(strings.TrimSuffix(host, ".")), fmt.Sprint(port))
}

// PrimaryAsset resolves the single remediation owner for a category/target.
// Certificate findings fall back to the serving host until the certificate
// asset type is introduced.
func PrimaryAsset(category string, target Target) (string, string) {
	switch category {
	case CategoryCertificate:
		if target.Certificate != "" {
			fingerprint := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(target.Certificate, "sha256:"), ":", ""))
			if len(fingerprint) == 64 {
				return "certificate", fingerprint
			}
		}
		if target.Host == "" {
			return "", ""
		}
		if net.ParseIP(target.Host) != nil {
			return "ip", target.Host
		}
		return "domain", target.Host
	case CategoryDNS:
		if target.Host == "" {
			return "", ""
		}
		if net.ParseIP(target.Host) != nil {
			return "ip", target.Host
		}
		return "domain", target.Host
	case CategoryService:
		if value := ServiceValue(target.Host, target.Port); value != "" {
			return "service", value
		}
		if target.Host != "" {
			if net.ParseIP(target.Host) != nil {
				return "ip", target.Host
			}
			return "domain", target.Host
		}
		return "", ""
	default:
		switch target.Type {
		case "url":
			return "url", target.URL
		case "ip":
			return "ip", target.Host
		case "domain":
			return "domain", target.Host
		case "service":
			if value := ServiceValue(target.Host, target.Port); value != "" {
				return "service", value
			}
		}
		return "", ""
	}
}

// ResolveLinks builds the impact surface and snapshots DNS answers at write
// time. The caller may supply already resolved IPs from the scanner.
func ResolveLinks(category, primaryType string, target Target, resolvedIPs []string) []Link {
	links := make([]Link, 0, 4)
	add := func(assetType, value, role string, direct bool, resolver string, resolvedAt *time.Time) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		links = append(links, Link{AssetType: assetType, Value: value, Role: role, Direct: direct, Resolver: resolver, ResolvedAt: resolvedAt})
	}

	now := time.Now().UTC()
	if target.Host != "" {
		if net.ParseIP(target.Host) != nil {
			direct := primaryType == "ip" || ((target.Type == "url" || target.Type == "service") && primaryType != "certificate")
			add("ip", target.Host, "host", direct, "target", nil)
		} else {
			add("domain", target.Host, "host", true, "target", nil)
			if category != CategoryDNS {
				ips := append([]string(nil), resolvedIPs...)
				resolver := "provided"
				if len(ips) == 0 {
					ips = resolveHostIPs(target.Host)
					resolver = "dns"
				}
				sort.Strings(ips)
				for index, ip := range ips {
					role := "related_ip"
					if index == 0 {
						role = "resolved_ip"
					}
					add("ip", ip, role, false, resolver, &now)
				}
			}
		}
	}
	if target.Type == "url" && target.Port > 0 {
		if value := ServiceValue(target.Host, target.Port); value != "" {
			add("service", value, "service", false, "target", nil)
		}
	}
	return links
}

// IsValidDomainName rejects single-label strings and malformed labels. A
// scanner/agent target must be a real network host, not a credential or a
// free-form description that happens to contain a dot.
func IsValidDomainName(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || len(host) > 253 || net.ParseIP(host) != nil {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !domainLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

func validTargetHost(target Target) bool {
	if target.Type == "certificate" {
		return target.Certificate != ""
	}
	if target.Host == "" {
		return false
	}
	if net.ParseIP(target.Host) != nil {
		return true
	}
	if strings.EqualFold(target.Host, "localhost") {
		return true
	}
	return IsValidDomainName(target.Host)
}

var credentialTargetHints = []string{
	"accesskey", "access key", "access_key", "accesskeyid", "access key id",
	"secretkey", "secret key", "secret_key", "ak/sk", "api key", "apikey", "api_key",
	"private key", "session key", "credential", "password", "passwd",
	"bearer", "authorization", "token", "密钥", "凭证", "口令", "密码",
}

func LooksLikeCredential(input Input) bool {
	text := strings.ToLower(strings.Join([]string{input.Title, input.VulnerabilityType, input.Target}, " "))
	for _, hint := range credentialTargetHints {
		if strings.Contains(text, hint) {
			return true
		}
	}
	return false
}

var cloudProviderHostSuffixes = []string{
	"aliyuncs.com", "amazonaws.com", "myqcloud.com", "azure.com", "googleapis.com",
	"cloudfront.net", "azureedge.net",
}

func isCloudProviderHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, suffix := range cloudProviderHostSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

type dnsCacheEntry struct {
	ips []string
	at  time.Time
}

type cnameCacheEntry struct {
	cname string
	at    time.Time
}

var cnameCache struct {
	mu      sync.Mutex
	entries map[string]cnameCacheEntry
}

var cdnCNAMEHints = []string{
	"cdn", "waf", "akamai", "cloudfront", "fastly", "kunlun", "alicdn",
	"tencentclb", "saaswaf", "dbappwaf", "cloudflare", "edgekey", "edgesuite",
	"trafficmanager", "azureedge",
}

// DetectCDNEdge looks at the host's CNAME target. DNS answers alone cannot
// distinguish an origin from an edge, but a CDN/WAF CNAME is a reliable signal
// that the resolved addresses should not be blamed as origin IPs.
func isCDNCNAME(cname string) bool {
	cname = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cname), "."))
	if cname == "" {
		return false
	}
	for _, hint := range cdnCNAMEHints {
		if strings.Contains(cname, hint) {
			return true
		}
	}
	return false
}

func DetectCDNEdge(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return false
	}
	cnameCache.mu.Lock()
	if cnameCache.entries == nil {
		cnameCache.entries = make(map[string]cnameCacheEntry)
	}
	if entry, ok := cnameCache.entries[host]; ok && time.Since(entry.at) < dnsCacheTTL {
		cname := entry.cname
		cnameCache.mu.Unlock()
		return cname != ""
	}
	cnameCache.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cname, err := net.DefaultResolver.LookupCNAME(ctx, host)
	if err != nil {
		cname = ""
	}
	cname = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cname), "."))
	if cname == host {
		cname = ""
	}
	if cname != "" {
		if !isCDNCNAME(cname) {
			cname = ""
		}
	}

	cnameCache.mu.Lock()
	cnameCache.entries[host] = cnameCacheEntry{cname: cname, at: time.Now()}
	cnameCache.mu.Unlock()
	return cname != ""
}

var dnsCache struct {
	mu      sync.Mutex
	entries map[string]dnsCacheEntry
}

const dnsCacheTTL = 10 * time.Minute

func resolveHostIPs(host string) []string {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return nil
	}
	dnsCache.mu.Lock()
	if dnsCache.entries == nil {
		dnsCache.entries = make(map[string]dnsCacheEntry)
	}
	if entry, ok := dnsCache.entries[host]; ok && time.Since(entry.at) < dnsCacheTTL {
		ips := append([]string(nil), entry.ips...)
		dnsCache.mu.Unlock()
		return ips
	}
	dnsCache.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		addresses = nil
	}
	seen := map[string]struct{}{}
	ips := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address.IP == nil {
			continue
		}
		ip := address.IP.String()
		if _, exists := seen[ip]; exists {
			continue
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
		if len(ips) >= 8 {
			break
		}
	}
	dnsCache.mu.Lock()
	dnsCache.entries[host] = dnsCacheEntry{ips: ips, at: time.Now()}
	dnsCache.mu.Unlock()
	return ips
}
