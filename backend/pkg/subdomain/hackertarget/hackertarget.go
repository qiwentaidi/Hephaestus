package hackertarget

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/subdomain/httpclient"
)

const defaultURL = "https://api.hackertarget.com/hostsearch/"

// FetchHosts returns full subdomains discovered from HackerTarget hostsearch.
func FetchHosts(ctx context.Context, domain string) []string {
	return FetchHostsWithProxy(ctx, domain, "")
}

func FetchHostsWithProxy(ctx context.Context, domain, proxy string) []string {
	hosts, err := fetchHosts(ctx, defaultURL, domain, proxy)
	if err != nil {
		return nil
	}
	return hosts
}

func fetchHosts(ctx context.Context, apiURL, domain, proxy string) ([]string, error) {
	rootDomain := normalizeHost(domain)
	if rootDomain == "" {
		return nil, fmt.Errorf("empty hackertarget domain")
	}

	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", rootDomain)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("User-Agent", "Hephaestus/hackertarget")

	client := httpclient.New(proxy, 30*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hackertarget returned %d", resp.StatusCode)
	}

	var candidates []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		candidates = append(candidates, extractHosts(rootDomain, scanner.Text())...)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return uniqueHosts(rootDomain, candidates), nil
}

func extractHosts(rootDomain, text string) []string {
	pattern := regexp.MustCompile(`(?i)(?:\*\.)?([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*\.` + regexp.QuoteMeta(rootDomain) + `)`)
	matches := pattern.FindAllStringSubmatch(text, -1)
	hosts := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			hosts = append(hosts, match[1])
		}
	}
	return hosts
}

func uniqueHosts(rootDomain string, candidates []string) []string {
	uniq := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		host := normalizeHost(candidate)
		if host == "" || !(host == rootDomain || strings.HasSuffix(host, "."+rootDomain)) {
			continue
		}
		uniq[host] = struct{}{}
	}

	hosts := make([]string, 0, len(uniq))
	for host := range uniq {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimPrefix(host, "*.")
	host = strings.TrimSuffix(host, ".")
	if host == "" || strings.ContainsAny(host, " \t\r\n") {
		return ""
	}
	return host
}
