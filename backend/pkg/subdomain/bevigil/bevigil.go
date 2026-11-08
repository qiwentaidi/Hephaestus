package bevigil

import (
	"encoding/json"
	"fmt"

	"github.com/qiwentaidi/clients"
	"hephaestus-backend/pkg/subdomain/httpclient"
)

const bevigilURL = "https://osint.bevigil.com/"

type BevigilHosts struct {
	Domain     string   `json:"domain"`
	Subdomains []string `json:"subdomains"`
}

// subdomains return is the complete subdomain
func FetchHosts(domain, apikey string) *BevigilHosts {
	return FetchHostsWithProxy(domain, apikey, "")
}

func FetchHostsWithProxy(domain, apikey, proxy string) *BevigilHosts {
	header := map[string]string{
		"X-Access-Token": apikey,
	}
	searchURL := fmt.Sprintf("%sapi/%s/subdomains/", bevigilURL, domain)
	resp, err := clients.DoRequest("GET", searchURL, header, nil, 10, httpclient.Resty(proxy, true))
	if err != nil {
		return nil
	}
	// 积分不足
	if resp.StatusCode() == 402 {
		return nil
	}
	bh := BevigilHosts{}
	json.Unmarshal(resp.Body(), &bh)
	return &bh
}
