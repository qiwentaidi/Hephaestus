package portscan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	gonmap "github.com/qiwentaidi/gonmap"
	"hephaestus-backend/pkg/taskstate"
)

type subnetAdmissionConfig struct {
	Mode          string
	HostSuffixes  []int
	RandomSamples int
}

type subnetAdmission struct {
	Signature   string                         `json:"signature"`
	Results     []gonmap.SubnetDiscoveryResult `json:"results"`
	Confirmed   int                            `json:"confirmed"`
	Unconfirmed int                            `json:"unconfirmed"`
}

func parseSubnetAdmissionConfig(config map[string]interface{}) (*subnetAdmissionConfig, error) {
	if !boolFromConfig(config["subnetDiscoveryEnabled"], false) {
		return nil, nil
	}
	result := &subnetAdmissionConfig{Mode: "full", HostSuffixes: []int{1, 2, 254}, RandomSamples: 2}
	if raw, ok := config["subnetDiscoveryMode"]; ok && raw != nil && raw != "" {
		mode, ok := raw.(string)
		if !ok || (mode != "fast" && mode != "full") {
			return nil, fmt.Errorf("网段预探测策略必须为 fast 或 full")
		}
		result.Mode = mode
	}
	if raw, ok := config["subnetDiscoveryHostSuffixes"]; ok && raw != nil && raw != "" {
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("抽样地址尾数必须是逗号分隔的整数")
		}
		if strings.TrimSpace(value) == "" {
			value = "1,2,254"
		}
		result.HostSuffixes = nil
		seen := map[int]bool{}
		for _, item := range strings.Split(value, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(item))
			if err != nil || n < 1 || n > 254 {
				return nil, fmt.Errorf("抽样地址尾数必须为 1–254")
			}
			if !seen[n] {
				result.HostSuffixes = append(result.HostSuffixes, n)
				seen[n] = true
			}
		}
		if len(result.HostSuffixes) > 32 {
			return nil, fmt.Errorf("固定抽样地址最多 32 个")
		}
	}
	if raw, ok := config["subnetDiscoveryRandomSamples"]; ok && raw != nil {
		n, err := strconv.Atoi(fmt.Sprint(raw))
		if err != nil || n < 0 || n > 32 {
			return nil, fmt.Errorf("随机抽样数必须为 0–32")
		}
		result.RandomSamples = n
	}
	return result, nil
}

// Only IPv4 CIDRs larger than /24 use the prefilter. Explicit IPs, small
// networks and hostnames retain their original semantics, even if overlapping.
func splitSubnetTargets(targets []string) (cidrs, passthrough []string) {
	for _, target := range targets {
		host := normalizePortScanHost(target)
		prefix, err := netip.ParsePrefix(host)
		if err == nil && prefix.Addr().Is4() && prefix.Bits() < 24 {
			cidrs = append(cidrs, prefix.Masked().String())
		} else {
			passthrough = append(passthrough, target)
		}
	}
	return
}

func restoreSubnetAdmission(checkpoint taskstate.Checkpoint, signature string) *subnetAdmission {
	raw, ok := checkpoint.Meta["subnetAdmission"]
	if !ok {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var result subnetAdmission
	if json.Unmarshal(data, &result) != nil || result.Signature != signature || result.Results == nil {
		return nil
	}
	for _, r := range result.Results {
		if r.Status != gonmap.HostConfirmed && r.Status != gonmap.HostUnconfirmed {
			return nil
		}
	}
	return &result
}

// Network sampling happens before address expansion. The snapshot is persisted
// before host admission; resume reuses its evidence and deterministic ordering.
func discoverSubnetAdmission(ctx context.Context, targets []string, config hostAdmissionConfig, signature string, ping func(context.Context, []string, bool) []string, onProgress func(int, int)) (*subnetAdmission, error) {
	cidrs, _ := splitSubnetTargets(targets)
	opts := gonmap.DefaultSubnetDiscoveryOptions()
	opts.TCP = *config.TCP
	opts.HostSuffixes = config.Subnet.HostSuffixes
	opts.RandomSamples = config.Subnet.RandomSamples
	if config.Ping {
		opts.AdditionalProbe = func(ctx context.Context, host string) bool { return len(ping(ctx, []string{host}, true)) > 0 }
	}
	done, hits := 0, 0
	opts.OnResult = func(r gonmap.SubnetDiscoveryResult) {
		done++
		if r.Status == gonmap.HostConfirmed {
			hits++
		}
		if onProgress != nil {
			onProgress(done, hits)
		}
	}
	results, err := gonmap.DiscoverSubnetsWithContext(ctx, cidrs, &opts)
	if err != nil {
		return nil, fmt.Errorf("网段预探测未完成: %w", err)
	}
	return &subnetAdmission{Signature: signature, Results: results, Confirmed: hits, Unconfirmed: len(results) - hits}, nil
}

func discoverSubnetHosts(ctx context.Context, targets []string, config hostAdmissionConfig, snapshot *subnetAdmission, ping func(context.Context, []string, bool) []string) (*hostAdmission, error) {
	_, passthrough := splitSubnetTargets(targets)
	result := &hostAdmission{Signature: snapshot.Signature, Hosts: []string{}, Subnets: snapshot}
	seen := map[string]bool{}
	// Retain explicit targets even when their enclosing sampled network missed.
	// Confirmed networks come next, then unconfirmed networks in full mode.
	groups := append([]string(nil), passthrough...)
	evidence := map[string]gonmap.SubnetDiscoveryResult{}
	for _, r := range snapshot.Results {
		if r.Status == gonmap.HostConfirmed {
			groups = append(groups, r.CIDR)
			evidence[r.EvidenceHost] = r
		}
	}
	if config.Subnet.Mode == "full" {
		for _, r := range snapshot.Results {
			if r.Status != gonmap.HostConfirmed {
				groups = append(groups, r.CIDR)
			}
		}
	}
	for groupIndex, group := range groups {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hosts := expandPortScanHosts([]string{group})
		pending := make([]string, 0, len(hosts))
		for _, host := range hosts {
			if seen[host] {
				continue
			}
			if groupIndex < len(passthrough) {
				seen[host] = true
			}
			result.Total++
			if r, ok := evidence[host]; ok {
				result.Hosts = append(result.Hosts, host)
				if r.Source == "tcp" {
					result.TCPAlive++
				} else {
					result.PingAlive++
				}
			} else {
				pending = append(pending, host)
			}
		}
		admitted, err := discoverAdmittedHosts(ctx, pending, config, "", ping)
		if err != nil {
			return nil, err
		}
		result.Hosts = append(result.Hosts, admitted.Hosts...)
		result.PingAlive += admitted.PingAlive
		result.TCPAlive += admitted.TCPAlive
		result.Unconfirmed += admitted.Unconfirmed
	}
	return result, ctx.Err()
}
