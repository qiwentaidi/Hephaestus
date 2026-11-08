package cdncheck

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"hephaestus-backend/pkg/types"
	"net"
	"strings"

	netutil "github.com/qiwentaidi/utils/net"
)

// InputCompiled contains a compiled list of input structure
type InputCompiled struct {
	// CDN contains a list of ranges for CDN cidrs
	CDN map[string][]string `yaml:"cdn,omitempty" json:"cdn,omitempty"`
	// WAF contains a list of ranges for WAF cidrs
	WAF map[string][]string `yaml:"waf,omitempty" json:"waf,omitempty"`
	// Cloud contains a list of ranges for Cloud cidrs
	Cloud map[string][]string `yaml:"cloud,omitempty" json:"cloud,omitempty"`
	// Common contains a list of suffixes for major sources
	Common map[string][]string `yaml:"common,omitempty" json:"common,omitempty"`
}

//go:embed sources_data.json
var data string

var generatedData InputCompiled

var (
	DefaultDnsServers = []string{"114.114.114.114:53", "223.6.6.6:53", "8.8.8.8:53"}
)

func init() {
	if err := json.Unmarshal([]byte(data), &generatedData); err != nil {
		panic(fmt.Sprintf("Could not parse cidr data: %s", err))
	}
}

func Detect(host string) (*types.Tag, bool) {
	if net.ParseIP(host) != nil {
		if name, ok := matchByIP(host, generatedData.CDN); ok {
			return &types.Tag{ProductName: name, AssetType: "cdn", Source: "ip"}, true
		}
		if name, ok := matchByIP(host, generatedData.WAF); ok {
			return &types.Tag{ProductName: name, AssetType: "waf", Source: "ip"}, true
		}
		if name, ok := matchByIP(host, generatedData.Cloud); ok {
			return &types.Tag{ProductName: name, AssetType: "cloud", Source: "ip"}, true
		}
		return nil, false
	}
	ips, cnames, err := netutil.Resolution(host, DefaultDnsServers, 10)
	if err != nil {
		return nil, false
	}

	// ======================
	// 1️⃣ CNAME 优先检测
	// ======================
	for _, cname := range cnames {
		if name, ok := matchByCNAME(cname, generatedData.Common); ok {
			return &types.Tag{ProductName: name, AssetType: "cdn", Source: "cname"}, true
		}
	}

	// ======================
	// 2️⃣ IP CIDR 检测
	// ======================
	for _, ip := range ips {
		if name, ok := matchByIP(ip, generatedData.CDN); ok {
			return &types.Tag{ProductName: name, AssetType: "cdn", Source: "ip"}, true
		}
		if name, ok := matchByIP(ip, generatedData.WAF); ok {
			return &types.Tag{ProductName: name, AssetType: "waf", Source: "ip"}, true
		}
		if name, ok := matchByIP(ip, generatedData.Cloud); ok {
			return &types.Tag{ProductName: name, AssetType: "cloud", Source: "ip"}, true
		}
	}

	return nil, false
}

func DetectWithCnameAndIp(ips, cnames []string) (*types.Tag, bool) {
	// ======================
	// 1️⃣ CNAME 优先检测
	// ======================
	for _, cname := range cnames {
		if name, ok := matchByCNAME(cname, generatedData.Common); ok {
			return &types.Tag{ProductName: name, AssetType: "cdn", Source: "cname"}, true
		}
	}

	// ======================
	// 2️⃣ IP CIDR 检测
	// ======================
	for _, ip := range ips {
		if name, ok := matchByIP(ip, generatedData.CDN); ok {
			return &types.Tag{ProductName: name, AssetType: "cdn", Source: "ip"}, true
		}
		if name, ok := matchByIP(ip, generatedData.WAF); ok {
			return &types.Tag{ProductName: name, AssetType: "waf", Source: "ip"}, true
		}
		if name, ok := matchByIP(ip, generatedData.Cloud); ok {
			return &types.Tag{ProductName: name, AssetType: "cloud", Source: "ip"}, true
		}
	}

	return nil, false
}

func matchByCNAME(cname string, sources map[string][]string) (string, bool) {
	cname = strings.TrimSuffix(cname, ".")

	for provider, suffixes := range sources {
		for _, suffix := range suffixes {
			if strings.Contains(cname, suffix) {
				return provider, true
			}
		}
	}
	return "", false
}

func matchByIP(ip string, sources map[string][]string) (string, bool) {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "", false
	}

	for provider, cidrs := range sources {
		for _, c := range cidrs {
			if IPInCIDR(parsedIP, c) {
				return provider, true
			}
		}
	}
	return "", false
}

func IPInCIDR(ip net.IP, cidr string) bool {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	return ipNet.Contains(ip)
}
