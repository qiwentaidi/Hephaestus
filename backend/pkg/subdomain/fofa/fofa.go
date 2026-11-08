package fofa

import (
	"encoding/json"
	"fmt"
	"hephaestus-backend/pkg/space-engine/fofa"
	"hephaestus-backend/pkg/subdomain/httpclient"
	"hephaestus-backend/pkg/types"
	"net/url"

	"github.com/qiwentaidi/clients"
)

type Result struct {
	Error   bool     `json:"error"`
	Errmsg  string   `json:"errmsg"`
	Mode    string   `json:"mode"`
	Page    int64    `json:"page"`
	Query   string   `json:"query"`
	Results []string `json:"results"`
	Size    int64    `json:"size"`
}

func FetchHosts(domain string, auth types.FofaAuth) []string {
	return FetchHostsWithProxy(domain, auth, "")
}

func FetchHostsWithProxy(domain string, auth types.FofaAuth, proxy string) []string {
	var result []string
	query := fofa.FOFABaseEncode(fmt.Sprintf("domain=\"%s\"", domain))
	address := fmt.Sprintf("%sapi/v1/search/all?email=%s&key=%s&qbase64=%s&is_fraud=false&is_honeypot=false&page=1&size=10&fields=link", auth.Address, auth.Email, auth.Key, query)
	resp, err := clients.SimpleGet(address, httpclient.Resty(proxy, true))
	if err != nil {
		return result
	}
	var fr Result
	json.Unmarshal(resp.Body(), &fr)
	if !fr.Error && fr.Size > 0 {
		for _, link := range fr.Results {
			url, err := url.Parse(link)
			if err == nil {
				result = append(result, url.Hostname())
			}
		}
	}
	return result
}
