package subdomain

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"hephaestus-backend/pkg/database"
)

const defaultDNSServerName = "自定义DNS"

type DNSServerSetting struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type DNSServerSettings struct {
	Servers []DNSServerSetting `json:"servers"`
}

var DefaultDNSServerSettings = []DNSServerSetting{
	{Address: "223.6.6.6:53", Name: "阿里DNS", Enabled: true},
	{Address: "8.8.8.8:53", Name: "Google DNS", Enabled: true},
	{Address: "114.114.114.114:53", Name: "114 DNS", Enabled: false},
	{Address: "1.1.1.1:53", Name: "Cloudflare DNS", Enabled: false},
	{Address: "119.29.29.29:53", Name: "腾讯DNS", Enabled: false},
}

func DefaultDNSServerConfig() DNSServerSettings {
	servers := make([]DNSServerSetting, len(DefaultDNSServerSettings))
	copy(servers, DefaultDNSServerSettings)
	return DNSServerSettings{Servers: servers}
}

func LoadDNSServerSettings() (DNSServerSettings, error) {
	raw, err := database.GetSetting(database.SettingKeySubdomainDNSServers)
	if err != nil {
		return DNSServerSettings{}, err
	}
	if raw == nil {
		return DefaultDNSServerConfig(), nil
	}

	settings, err := decodeDNSServerSettings(raw)
	if err != nil {
		return DNSServerSettings{}, err
	}

	normalized, err := NormalizeDNSServerSettings(settings, false)
	if err != nil || len(normalized.Servers) == 0 {
		return DefaultDNSServerConfig(), nil
	}
	return normalized, nil
}

func LoadConfiguredDNSServerAddresses() ([]string, error) {
	raw, err := database.GetSetting(database.SettingKeySubdomainDNSServers)
	if err != nil || raw == nil {
		return nil, err
	}

	settings, err := decodeDNSServerSettings(raw)
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizeDNSServerSettings(settings, false)
	if err != nil {
		return nil, err
	}
	return EnabledDNSServerAddresses(normalized), nil
}

func SaveDNSServerSettings(settings DNSServerSettings) (DNSServerSettings, error) {
	normalized, err := NormalizeDNSServerSettings(settings, true)
	if err != nil {
		return DNSServerSettings{}, err
	}
	if err := database.SetSetting(database.SettingKeySubdomainDNSServers, normalized); err != nil {
		return DNSServerSettings{}, err
	}
	return normalized, nil
}

func NormalizeDNSServerSettings(settings DNSServerSettings, requireEnabled bool) (DNSServerSettings, error) {
	normalized := DNSServerSettings{
		Servers: make([]DNSServerSetting, 0, len(settings.Servers)),
	}
	seen := make(map[string]int)
	enabledCount := 0

	for _, server := range settings.Servers {
		address, err := NormalizeDNSServerAddress(server.Address)
		if err != nil {
			return DNSServerSettings{}, err
		}
		if existingIndex, exists := seen[address]; exists {
			if server.Enabled && !normalized.Servers[existingIndex].Enabled {
				normalized.Servers[existingIndex].Enabled = true
				enabledCount++
			}
			continue
		}
		seen[address] = len(normalized.Servers)

		name := strings.TrimSpace(server.Name)
		if name == "" {
			name = defaultDNSServerName
		}
		if server.Enabled {
			enabledCount++
		}

		normalized.Servers = append(normalized.Servers, DNSServerSetting{
			Address: address,
			Name:    name,
			Enabled: server.Enabled,
		})
	}

	if len(normalized.Servers) == 0 {
		return DNSServerSettings{}, fmt.Errorf("DNS服务器不能为空")
	}
	if requireEnabled && enabledCount == 0 {
		return DNSServerSettings{}, fmt.Errorf("至少需要启用一个DNS服务器")
	}
	return normalized, nil
}

func NormalizeDNSServerAddresses(addresses []string) ([]string, error) {
	normalized := make([]string, 0, len(addresses))
	seen := make(map[string]struct{})
	for _, rawAddress := range addresses {
		address, err := NormalizeDNSServerAddress(rawAddress)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		normalized = append(normalized, address)
	}
	return normalized, nil
}

func NormalizeDNSServerAddress(rawAddress string) (string, error) {
	address := strings.TrimSpace(rawAddress)
	if address == "" {
		return "", fmt.Errorf("DNS服务器地址不能为空")
	}

	if ip := net.ParseIP(address); ip != nil {
		return net.JoinHostPort(ip.String(), "53"), nil
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("DNS服务器地址格式无效: %s", rawAddress)
	}

	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if host == "" || port == "" {
		return "", fmt.Errorf("DNS服务器地址格式无效: %s", rawAddress)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("DNS服务器地址必须为IP: %s", rawAddress)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", fmt.Errorf("DNS服务器端口无效: %s", rawAddress)
	}

	return net.JoinHostPort(ip.String(), strconv.Itoa(portNumber)), nil
}

func EnabledDNSServerAddresses(settings DNSServerSettings) []string {
	addresses := make([]string, 0, len(settings.Servers))
	for _, server := range settings.Servers {
		if server.Enabled {
			addresses = append(addresses, server.Address)
		}
	}
	return addresses
}

func decodeDNSServerSettings(raw interface{}) (DNSServerSettings, error) {
	payload, err := json.Marshal(raw)
	if err != nil {
		return DNSServerSettings{}, fmt.Errorf("序列化DNS服务器配置失败: %w", err)
	}

	var settings DNSServerSettings
	if err := json.Unmarshal(payload, &settings); err != nil {
		return DNSServerSettings{}, fmt.Errorf("解析DNS服务器配置失败: %w", err)
	}
	return settings, nil
}
