package subdomain

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/logger"

	"github.com/miekg/dns"
)

const maxDNSAttempts = 3

// resolveDNSWithRetry tries distinct resolvers in configuration order. A failed
// address lookup must never be hidden by a successful CNAME lookup. In
// particular, NXDOMAIN, SERVFAIL and transport failures remain diagnosable.
func resolveDNSWithRetry(domain string, servers []string, timeout int) ([]string, []string, error) {
	if len(servers) == 0 {
		servers = DefaultDnsServers
	}
	if timeout <= 0 {
		timeout = 5
	}
	var failures []error
	seen := make(map[string]bool)
	for _, server := range servers {
		address, err := NormalizeDNSServerAddress(server)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if seen[address] {
			continue
		}
		if len(seen) == maxDNSAttempts {
			break
		}
		seen[address] = true
		ips, cnames, err := resolveDNSAtServer(domain, address, time.Duration(timeout)*time.Second)
		if err == nil {
			return ips, cnames, nil
		}
		failures = append(failures, err)
		logger.Debug("子域名[%s] DNS[%s]解析未成功，将尝试下一解析源: %v", domain, address, err)
	}
	return nil, nil, fmt.Errorf("DNS resolution for %s failed: %w", domain, errors.Join(failures...))
}

type dnsQueryResult struct {
	ips, cnames []string
	err         error
}

func resolveDNSAtServer(domain, server string, timeout time.Duration) ([]string, []string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	results := make(chan dnsQueryResult, 2)
	// Query both families within one per-server deadline; IPv6-only hosts are
	// valid too, and a broken AAAA path must not discard a successful A answer.
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		go func() {
			results <- queryDNSAddresses(ctx, domain, server, qtype, timeout)
		}()
	}
	var ips, cnames []string
	var failures []error
	for range 2 {
		result := <-results
		ips = append(ips, result.ips...)
		cnames = append(cnames, result.cnames...)
		if result.err != nil {
			failures = append(failures, result.err)
		}
	}
	if len(ips) > 0 {
		return uniqueDNSValues(ips), uniqueDNSValues(cnames), nil
	}
	if len(failures) == 0 {
		failures = append(failures, &net.DNSError{Err: "no A or AAAA records", Name: domain, Server: server, IsNotFound: true})
	}
	return nil, nil, fmt.Errorf("DNS %s: %w", server, errors.Join(failures...))
}

func queryDNSAddresses(ctx context.Context, domain, server string, qtype uint16, timeout time.Duration) dnsQueryResult {
	request := new(dns.Msg)
	request.SetQuestion(dns.Fqdn(domain), qtype)
	client := &dns.Client{Net: "udp", Timeout: timeout}
	response, _, err := client.ExchangeContext(ctx, request, server)
	if err == nil && response.Truncated {
		client.Net = "tcp"
		response, _, err = client.ExchangeContext(ctx, request, server)
	}
	if err != nil {
		return dnsQueryResult{err: fmt.Errorf("%s %s via %s: %w", domain, dns.TypeToString[qtype], server, err)}
	}
	if response.Rcode != dns.RcodeSuccess {
		return dnsQueryResult{err: &net.DNSError{
			Err: dns.RcodeToString[response.Rcode], Name: domain, Server: server,
			IsNotFound:  response.Rcode == dns.RcodeNameError,
			IsTemporary: response.Rcode == dns.RcodeServerFailure,
		}}
	}
	var result dnsQueryResult
	for _, answer := range response.Answer {
		switch record := answer.(type) {
		case *dns.A:
			result.ips = append(result.ips, record.A.String())
		case *dns.AAAA:
			result.ips = append(result.ips, record.AAAA.String())
		case *dns.CNAME:
			result.cnames = append(result.cnames, normalizeWildcardCNAME(record.Target))
		}
	}
	return result
}

func uniqueDNSValues(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
