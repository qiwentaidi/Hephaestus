package crtname

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/subdomain/httpclient"
)

const defaultURL = "https://crt.name/v1/search"

type searchResult struct {
	FirstSeen *string `json:"first_seen"`
	Sub       string  `json:"sub"`
}

// FetchHosts returns full subdomains discovered from crt.name.
// crt.name supports anonymous requests with a small per-IP quota.
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

func fetchHosts(ctx context.Context, apiURL, domain string, proxies ...string) ([]string, error) {
	proxy := ""
	if len(proxies) > 0 {
		proxy = proxies[0]
	}
	rootDomain := normalizeHost(domain)
	if rootDomain == "" {
		return nil, fmt.Errorf("empty crt.name domain")
	}

	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid crt.name url: %w", err)
	}

	q := u.Query()
	q.Set("apex", rootDomain)
	q.Set("format", "json")
	q.Set("dates", "1")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Hephaestus/crt.name")

	client := httpclient.New(proxy, 30*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		message := strings.TrimSpace(string(body))
		if message != "" {
			return nil, fmt.Errorf("crt.name returned %d: %s", resp.StatusCode, message)
		}
		return nil, fmt.Errorf("crt.name returned %d", resp.StatusCode)
	}

	var data []searchResult
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	uniq := make(map[string]struct{}, len(data))
	for _, item := range data {
		host := normalizeHost(item.Sub)
		if host == "" {
			continue
		}
		if host == rootDomain || strings.HasSuffix(host, "."+rootDomain) {
			uniq[host] = struct{}{}
		}
	}

	hosts := make([]string, 0, len(uniq))
	for host := range uniq {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts, nil
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
