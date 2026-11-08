package portscan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/taskstate"

	gonmaproot "github.com/qiwentaidi/gonmap"
	gonmap "github.com/qiwentaidi/gonmap/lib"
	portutil "github.com/qiwentaidi/utils/port"
)

type hostAdmissionConfig struct {
	Ping   bool
	TCP    *gonmap.HostDiscoveryOptions
	Subnet *subnetAdmissionConfig `json:",omitempty"`
}

// hostAdmission is saved with every scan checkpoint. Restoring the exact host
// set keeps a Connect resume cursor from shifting when liveness changes.
type hostAdmission struct {
	Signature   string           `json:"signature"`
	Hosts       []string         `json:"hosts"`
	Total       int              `json:"total"`
	PingAlive   int              `json:"pingAlive"`
	TCPAlive    int              `json:"tcpAlive"`
	Unconfirmed int              `json:"unconfirmed"`
	Subnets     *subnetAdmission `json:"subnets,omitempty"`
}

func parseHostAdmissionConfig(config map[string]interface{}, portRange, proxyURL string, workers int) (hostAdmissionConfig, error) {
	result := hostAdmissionConfig{Ping: true}
	if raw, ok := config["aliveDetection"].(bool); ok {
		result.Ping = raw
	}
	var err error
	result.Subnet, err = parseSubnetAdmissionConfig(config)
	if err != nil {
		return result, err
	}
	if !boolFromConfig(config["tcpHeuristicEnabled"], false) && result.Subnet == nil {
		return result, nil
	}
	ports := gonmap.DefaultTCPHostDiscoveryPorts()
	if raw, exists := config["tcpHeuristicPorts"]; exists && raw != nil {
		value, ok := raw.(string)
		if !ok {
			return result, fmt.Errorf("TCP 启发式端口必须是逗号分隔的端口列表")
		}
		if strings.TrimSpace(value) != "" {
			ports = nil
			seen := make(map[int]bool)
			for _, item := range strings.Split(value, ",") {
				port, err := strconv.Atoi(strings.TrimSpace(item))
				if err != nil || port < 1 || port > 65535 {
					return result, fmt.Errorf("TCP 启发式端口无效：%q（需为 1–65535 的整数）", item)
				}
				if !seen[port] {
					seen[port] = true
					ports = append(ports, port)
				}
			}
			if len(ports) > 64 {
				return result, fmt.Errorf("TCP 启发式探测最多配置 64 个端口")
			}
		}
	}
	timeout := 3000
	if raw, exists := config["tcpHeuristicTimeoutMs"]; exists && raw != nil {
		value, err := strconv.Atoi(fmt.Sprint(raw))
		if err != nil || value < 100 || value > 30000 {
			return result, fmt.Errorf("TCP 启发式探测超时必须为 100–30000 毫秒")
		}
		timeout = value
	}
	definitions, err := portutil.ParsePortDefinitions(portRange)
	if err != nil {
		return result, fmt.Errorf("解析端口范围失败: %w", err)
	}
	for _, port := range definitions {
		if port.Protocol == "udp" {
			return result, fmt.Errorf("TCP 启发式探测仅适用于纯 TCP 端口范围，请移除 UDP 端口或关闭启发式探测")
		}
	}
	if proxyURL != "" && parseTCPDiscoveryConfig(config).Mode == gonmap.PortDiscoveryModeSYN {
		return result, fmt.Errorf("代理模式下 TCP 启发式探测需配合 Connect 扫描，不能使用 SYN")
	}
	if workers < 1 || workers > 256 {
		workers = 256
	}
	result.TCP = &gonmap.HostDiscoveryOptions{
		Ports: ports, Timeout: time.Duration(timeout) * time.Millisecond,
		Concurrency: workers, PortsPerHost: 5, ProxyURL: proxyURL,
	}
	// ICMP cannot use the SOCKS5 route. A proxied heuristic task must be
	// admitted exclusively by TCP responses observed through that proxy.
	if proxyURL != "" {
		result.Ping = false
	}
	return result, nil
}

func hostAdmissionSignature(hosts []string, config hostAdmissionConfig, portRange string) string {
	ordered := append([]string(nil), hosts...)
	sort.Strings(ordered)
	payload, _ := json.Marshal(struct {
		Hosts  []string
		Config hostAdmissionConfig
		Ports  string
	}{ordered, config, portRange})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func restoreHostAdmission(checkpoint taskstate.Checkpoint, signature string) (*hostAdmission, bool) {
	if checkpoint.Stage != "portscan:scanning" && checkpoint.Stage != "portscan:alive_done" {
		return nil, false
	}
	raw, ok := checkpoint.Meta["hostAdmission"]
	if !ok {
		return nil, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var result hostAdmission
	if json.Unmarshal(data, &result) != nil || result.Signature != signature || result.Hosts == nil || result.Total < len(result.Hosts) {
		return nil, false
	}
	return &result, true
}

func discoverAdmittedHosts(ctx context.Context, hosts []string, config hostAdmissionConfig, signature string, ping func(context.Context, []string, bool) []string) (*hostAdmission, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := &hostAdmission{Signature: signature, Hosts: make([]string, 0, len(hosts)), Total: len(hosts)}
	confirmed := make(map[string]bool, len(hosts))
	if config.Ping {
		for _, host := range ping(ctx, hosts, true) {
			confirmed[host] = true
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	remaining := make([]string, 0, len(hosts))
	for _, host := range hosts {
		if confirmed[host] {
			result.PingAlive++
		} else {
			remaining = append(remaining, host)
		}
	}
	if config.TCP != nil {
		probes, err := gonmaproot.DiscoverLiveHostsWithContext(ctx, remaining, config.TCP)
		if err != nil {
			return nil, fmt.Errorf("TCP 启发式探测未完成: %w", err)
		}
		for _, probe := range probes {
			if probe.Status == gonmap.HostConfirmed {
				confirmed[probe.Host] = true
				result.TCPAlive++
			}
		}
	}
	for _, host := range hosts {
		if confirmed[host] || (!config.Ping && config.TCP == nil) {
			result.Hosts = append(result.Hosts, host)
		}
	}
	sort.Strings(result.Hosts)
	result.Unconfirmed = result.Total - len(result.Hosts)
	return result, ctx.Err()
}

func (admission *hostAdmission) Meta(fields map[string]interface{}) map[string]interface{} {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["hostAdmission"] = admission
	return fields
}
