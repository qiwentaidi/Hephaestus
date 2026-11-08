package github

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/qiwentaidi/clients"
	"hephaestus-backend/pkg/subdomain/httpclient"
)

// subdomains return is complete subdomain
func FetchHosts(domain, apikey string) []string {
	return FetchHostsWithProxy(domain, apikey, "")
}

func FetchHostsWithProxy(domain, apikey, proxy string) []string {
	headers := map[string]string{
		"Accept":        "application/vnd.github.v3.text-match+json",
		"Authorization": "token " + apikey,
	}
	searchURL := fmt.Sprintf("https://api.github.com/search/code?per_page=100&q=%s&sort=created&order=asc", domain)
	resp, err := clients.DoRequest("GET", searchURL, headers, nil, 10, httpclient.Resty(proxy, true))
	if err != nil {
	}
	r := domainRegexp(domain)
	return r.FindAllString(string(resp.Body()), -1)
}

func domainRegexp(domain string) *regexp.Regexp {
	rdomain := strings.ReplaceAll(domain, ".", "\\.")
	return regexp.MustCompile("(\\w[a-zA-Z0-9][a-zA-Z0-9-\\.]*)" + rdomain)
}
