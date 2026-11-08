package anubis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/subdomain/httpclient"
)

const defaultURL = "https://anubisdb.com/anubis/subdomains/%s"

// FetchHosts returns full subdomains discovered from AnubisDB.
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
		return nil, fmt.Errorf("empty anubis domain")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(apiURL, rootDomain), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Hephaestus/anubis")

	client := httpclient.New(proxy, 30*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anubis returned %d", resp.StatusCode)
	}

	var records []string
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, err
	}
	return uniqueHosts(rootDomain, records), nil
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
