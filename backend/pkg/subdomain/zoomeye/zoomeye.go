package zoomeye

import (
	"fmt"
	"net/url"
	"strings"

	spacezoomeye "hephaestus-backend/pkg/space-engine/zoomeye"
	"hephaestus-backend/pkg/types"
)

type List struct {
	Name      string   `json:"name"`
	Timestamp string   `json:"timestamp"`
	Ip        []string `json:"ip"`
}

type ZoomeyeHost struct {
	Status float64 `json:"status"`
	Total  float64 `json:"total"`
	List   []List  `json:"list"`
	Msg    string  `json:"msg"`
	Type   float64 `json:"type"`
}

// subdomains return is complete subdomain
func FetchHosts(domain, address, apikey string) *ZoomeyeHost {
	return FetchHostsWithProxy(domain, address, apikey, "")
}

func FetchHostsWithProxy(domain, address, apikey, proxy string) *ZoomeyeHost {
	client := spacezoomeye.NewZoomeyeClient(&types.ZoomeyeAuth{
		Address: address,
		Key:     apikey,
		Proxy:   proxy,
	})
	resp := client.ZoomeyeApiSearch(&types.ZoomeyeSearchRequest{
		Query:    fmt.Sprintf(`domain:"%s"`, domain),
		Page:     1,
		PageSize: 1000,
		SubType:  "web",
		Fields:   "domain,hostname,url",
	})
	if resp == nil || resp.Code != 60000 {
		return nil
	}
	return &ZoomeyeHost{
		Status: 60000,
		Total:  float64(resp.Total),
		List:   buildHostList(resp.Data),
	}
}

func buildHostList(rows []types.ZoomeyeDataItem) []List {
	seen := make(map[string]struct{}, len(rows))
	results := make([]List, 0, len(rows))
	addHost := func(host string) {
		host = strings.TrimSpace(strings.ToLower(host))
		host = strings.TrimSuffix(host, ".")
		if host == "" {
			return
		}
		if _, exists := seen[host]; exists {
			return
		}
		seen[host] = struct{}{}
		results = append(results, List{Name: host})
	}

	for _, row := range rows {
		addHost(row.Domain)
		addHost(row.Hostname)
		if parsed, err := url.Parse(strings.TrimSpace(row.URL)); err == nil {
			addHost(parsed.Hostname())
		}
	}
	return results
}
