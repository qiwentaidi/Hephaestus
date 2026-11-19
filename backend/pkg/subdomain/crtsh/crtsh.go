package crtsh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/subdomain/httpclient"
)

const (
	defaultURL       = "https://crt.sh/"
	requestTimeout   = 30 * time.Second
	maxResponseBytes = 16 << 20
	maxNames         = 5000
)

var hostnameLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

type searchResult struct {
	NameValue string `json:"name_value"`
}

// FetchNames returns every DNS name on certificates that cover domain.
//
// crt.sh's identity search returns one row per matching certificate and the
// row's name_value field holds the certificate's complete subjectAltName list.
// Names from other apexes that share a certificate with domain are therefore
// included as well; callers decide whether they are subdomains or related
// domains.
func FetchNames(ctx context.Context, domain, proxy string) ([]string, error) {
	return fetchNames(ctx, defaultURL, domain, proxy)
}

func fetchNames(ctx context.Context, apiURL, domain, proxy string) ([]string, error) {
	rootDomain := normalizeHost(domain)
	if rootDomain == "" {
		return nil, fmt.Errorf("empty crt.sh domain")
	}

	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid crt.sh url: %w", err)
	}
	q := u.Query()
	q.Set("q", rootDomain)
	q.Set("output", "json")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Hephaestus/crt.sh")

	client := httpclient.New(proxy, requestTimeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if message := strings.TrimSpace(string(body)); message != "" {
			return nil, fmt.Errorf("crt.sh returned %d: %s", resp.StatusCode, message)
		}
		return nil, fmt.Errorf("crt.sh returned %d", resp.StatusCode)
	}

	var rows []searchResult
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes))
	if err := decoder.Decode(&rows); err != nil {
		return nil, fmt.Errorf("decode crt.sh response: %w", err)
	}

	uniq := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		names := normalizeNames(row.NameValue)
		// crt.sh matches identities by substring, so lookalike certificates
		// (for example www.example.com--com.com) are returned as well. Only
		// keep rows whose SAN list actually covers the queried domain.
		if len(names) == 0 || !coversDomain(names, rootDomain) {
			continue
		}
		for _, name := range names {
			if !isValidHostname(name) {
				continue
			}
			uniq[name] = struct{}{}
		}
	}

	names := make([]string, 0, len(uniq))
	for name := range uniq {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > maxNames {
		names = names[:maxNames]
	}
	return names, nil
}

func normalizeNames(value string) []string {
	names := make([]string, 0, 8)
	for _, raw := range strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == '\r' }) {
		if name := normalizeHost(raw); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func coversDomain(names []string, rootDomain string) bool {
	for _, name := range names {
		if name == rootDomain || strings.HasSuffix(name, "."+rootDomain) {
			return true
		}
	}
	return false
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

func isValidHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || !hostnameLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}
