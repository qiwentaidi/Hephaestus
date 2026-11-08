package thc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/subdomain/httpclient"
)

const (
	defaultURL   = "https://ip.thc.org/api/v1/lookup/subdomains"
	defaultLimit = 1000
	maxPages     = 20
)

type requestBody struct {
	Domain    string `json:"domain"`
	PageState string `json:"page_state"`
	Limit     int    `json:"limit"`
}

type responseBody struct {
	Domains []struct {
		Domain string `json:"domain"`
	} `json:"domains"`
	NextPageState string `json:"next_page_state"`
}

// FetchHosts returns full subdomains discovered from ip.thc.org.
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
		return nil, fmt.Errorf("empty thc domain")
	}

	client := httpclient.New(proxy, 30*time.Second)
	pageState := ""
	var candidates []string
	for page := 0; page < maxPages; page++ {
		payload, err := json.Marshal(requestBody{
			Domain:    rootDomain,
			PageState: pageState,
			Limit:     defaultLimit,
		})
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Hephaestus/thc")

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		var data responseBody
		decodeErr := json.NewDecoder(resp.Body).Decode(&data)
		closeErr := resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("thc returned %d", resp.StatusCode)
		}
		if decodeErr != nil {
			return nil, decodeErr
		}
		if closeErr != nil {
			return nil, closeErr
		}

		for _, item := range data.Domains {
			candidates = append(candidates, item.Domain)
		}
		pageState = strings.TrimSpace(data.NextPageState)
		if pageState == "" {
			break
		}
	}
	return uniqueHosts(rootDomain, candidates), nil
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
