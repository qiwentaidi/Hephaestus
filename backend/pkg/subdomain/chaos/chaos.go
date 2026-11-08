package chaos

import (
	"encoding/json"
	"fmt"

	"github.com/qiwentaidi/clients"
	"hephaestus-backend/pkg/subdomain/httpclient"
)

const chaosURL = "https://dns.projectdiscovery.io/"

type ChaosHosts struct {
	Domain     string   `json:"domain"`
	Subdomains []string `json:"subdomains"`
	Count      float64  `json:"count"`
}

// subdomains return is not complete subdomain, e.g www
func FetchHosts(domain, apikey string) *ChaosHosts {
	return FetchHostsWithProxy(domain, apikey, "")
}

func FetchHostsWithProxy(domain, apikey, proxy string) *ChaosHosts {
	header := map[string]string{
		"Authorization": apikey,
	}
	searchURL := fmt.Sprintf("%sdns/%s/subdomains", chaosURL, domain)
	resp, err := clients.DoRequest("GET", searchURL, header, nil, 10, httpclient.Resty(proxy, true))
	if err != nil {
		return nil
	}
	var ch ChaosHosts
	json.Unmarshal(resp.Body(), &ch)
	return &ch
}
