package ctradar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultURL = "https://cert.imfht.com/api/search"

type searchResponse struct {
	Results           []searchResult `json:"results"`
	Error             string         `json:"error"`
	Message           string         `json:"message"`
	RetryAfterSeconds int            `json:"retry_after_seconds"`
}

type searchResult struct {
	Domain string `json:"domain"`
}

// FetchHosts returns full subdomains discovered from ct-radar.
// ct-radar supports anonymous requests, while apiKey raises the quota when supplied.
func FetchHosts(ctx context.Context, domain, apiKey string) []string {
	limit := 50
	if strings.TrimSpace(apiKey) != "" {
		limit = 100
	}
	hosts, err := fetchHosts(ctx, defaultURL, domain, strings.TrimSpace(apiKey), limit)
	if err != nil {
		return nil
	}
	return hosts
}

func fetchHosts(ctx context.Context, apiURL, domain, apiKey string, limit int) ([]string, error) {
	rootDomain := normalizeHost(domain)
	if rootDomain == "" {
		return nil, fmt.Errorf("empty ct-radar domain")
	}

	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid ct-radar url: %w", err)
	}

	q := u.Query()
	q.Set("q", rootDomain)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", "0")
	q.Set("exclude_wildcard", "true")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Hephaestus/ct-radar")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if data.Message != "" {
			return nil, fmt.Errorf("ct-radar returned %d: %s", resp.StatusCode, data.Message)
		}
		if data.Error != "" {
			return nil, fmt.Errorf("ct-radar returned %d: %s", resp.StatusCode, data.Error)
		}
		return nil, fmt.Errorf("ct-radar returned %d", resp.StatusCode)
	}

	uniq := make(map[string]struct{}, len(data.Results))
	for _, item := range data.Results {
		host := normalizeHost(item.Domain)
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
