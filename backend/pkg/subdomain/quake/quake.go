package quake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hephaestus-backend/pkg/subdomain/httpclient"
	"hephaestus-backend/pkg/types"
	"strconv"

	"github.com/qiwentaidi/clients"
)

func FetchHosts(domain, apikey string) []string {
	return FetchHostsWithProxy(domain, apikey, "")
}

func FetchHostsWithProxy(domain, apikey, proxy string) []string {
	var result []string
	type quakeHostItem struct {
		Service types.Service `json:"service"`
	}
	type quakeEnvelope struct {
		Code    interface{}     `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	data := make(map[string]interface{})
	data["query"] = fmt.Sprintf("domain: %s", domain)
	data["start"] = 0
	data["size"] = 500
	data["latest"] = true
	bytesData, _ := json.Marshal(data)
	header := map[string]string{
		"Content-Type": "application/json",
		"X-QuakeToken": apikey,
	}
	resp, err := clients.DoRequest("POST", "https://quake.360.net/api/v3/search/quake_service", header, bytes.NewReader(bytesData), 10, httpclient.Resty(proxy, true))
	if err != nil {
		return result
	}
	body := resp.Body()
	if string(body) == "/quake/login" {
		return result
	}
	if string(body) == "暂不支持搜索该内容" {
		return result
	}
	var qrk quakeEnvelope
	if err := json.Unmarshal(body, &qrk); err != nil {
		return result
	}
	if code, err := strconv.Atoi(fmt.Sprintf("%v", qrk.Code)); err == nil && code != 0 {
		return result
	}
	if len(qrk.Data) == 0 || string(qrk.Data) == "null" || string(qrk.Data) == "{}" {
		return result
	}
	var items []quakeHostItem
	if err := json.Unmarshal(qrk.Data, &items); err != nil {
		return result
	}
	for _, item := range items {
		result = append(result, item.Service.HTTP.Host)
	}
	return result
}
