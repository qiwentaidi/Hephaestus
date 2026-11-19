package crtsh

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/subdomain/httpclient"
)

const (
	defaultURL     = "https://crt.sh/"
	requestTimeout = 30 * time.Second

	maxResponseBytes = 16 << 20
	maxCertBytes     = 1 << 20
	maxNames         = 5000

	// crt.sh only exposes the identities that matched the query in its JSON
	// response. Fetching the matching certificates gives us the complete SAN
	// list, but each fetch costs one request, so the work is bounded.
	maxCertFetches       = 50
	certFetchConcurrency = 6
	certFetchBudget      = 30 * time.Second
)

var hostnameLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

type searchResult struct {
	ID         int64  `json:"id"`
	CommonName string `json:"common_name"`
	NameValue  string `json:"name_value"`
}

type certGroup struct {
	id         int64
	commonName string
}

// FetchNames returns every DNS name on certificates that cover domain.
//
// crt.sh's identity search returns one row per matching certificate, but the
// row's name_value field only contains the identities that matched the query,
// not the certificate's complete subjectAltName list. To find sibling domains
// that do not contain the queried name, the matching certificates are fetched
// by crt.sh ID and parsed locally; the resulting SANs are merged with the
// matching identities.
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
	groups := make([]certGroup, 0, len(rows))
	groupIDs := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		names := normalizeNames(row.NameValue)
		// crt.sh matches identities by substring, so lookalike certificates
		// (for example www.example.com--com.com) are returned as well. Only
		// keep rows whose matching identities actually cover the domain.
		if len(names) == 0 || !coversDomain(names, rootDomain) {
			continue
		}
		for _, name := range names {
			if isValidHostname(name) {
				uniq[name] = struct{}{}
			}
		}
		if row.ID <= 0 {
			continue
		}
		key := row.CommonName + "\x00" + row.NameValue
		if _, exists := groupIDs[key]; exists {
			continue
		}
		groupIDs[key] = struct{}{}
		groups = append(groups, certGroup{id: row.ID, commonName: normalizeHost(row.CommonName)})
	}

	// Certificates whose common name did not match the query are the most
	// likely to carry extra SANs outside the queried name, so fetch those
	// first when the request budget is limited.
	sort.SliceStable(groups, func(i, j int) bool {
		_, iMatched := uniq[groups[i].commonName]
		_, jMatched := uniq[groups[j].commonName]
		return !iMatched && jMatched
	})
	if len(groups) > maxCertFetches {
		groups = groups[:maxCertFetches]
	}

	if len(groups) > 0 {
		fetchCtx, cancel := context.WithTimeout(ctx, certFetchBudget)
		defer cancel()

		var wg sync.WaitGroup
		var mu sync.Mutex
		sem := make(chan struct{}, certFetchConcurrency)
		for _, group := range groups {
			if fetchCtx.Err() != nil {
				break
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(id int64) {
				defer wg.Done()
				defer func() { <-sem }()
				names, err := fetchCertificateNames(fetchCtx, client, apiURL, id)
				if err != nil {
					return
				}
				mu.Lock()
				for _, name := range names {
					uniq[name] = struct{}{}
				}
				mu.Unlock()
			}(group.id)
		}
		wg.Wait()
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

func fetchCertificateNames(ctx context.Context, client *http.Client, apiURL string, id int64) ([]string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Del("q")
	q.Del("output")
	q.Set("d", strconv.FormatInt(id, 10))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/x-pem-file, application/pkix-cert, */*")
	req.Header.Set("User-Agent", "Hephaestus/crt.sh")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("crt.sh certificate %d returned %d: %s", id, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCertBytes))
	if err != nil {
		return nil, err
	}

	der := body
	if block, _ := pem.Decode(body); block != nil {
		der = block.Bytes
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(cert.DNSNames)+1)
	names = append(names, cert.DNSNames...)
	if cert.Subject.CommonName != "" {
		names = append(names, cert.Subject.CommonName)
	}

	result := make([]string, 0, len(names))
	for _, name := range names {
		normalized := normalizeHost(name)
		if normalized != "" && isValidHostname(normalized) {
			result = append(result, normalized)
		}
	}
	return result, nil
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
