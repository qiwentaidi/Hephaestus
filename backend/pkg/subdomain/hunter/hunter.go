package hunter

import (
	"fmt"

	spacehunter "hephaestus-backend/pkg/space-engine/hunter"
	"hephaestus-backend/pkg/types"
)

func FetchHosts(domain, apikey string) []string {
	return FetchHostsWithProxy(domain, apikey, "")
}

func FetchHostsWithProxy(domain, apikey, proxy string) []string {
	client := spacehunter.NewHunterClient(&types.HunterAuth{
		Key:   apikey,
		Proxy: proxy,
	})
	hr := client.HunterApiSearch(&types.HunterSearchRequest{
		Query:         fmt.Sprintf(`domain.suffix="%s"`, domain),
		Page:          1,
		PageSize:      100,
		TimeFilter:    "0",
		IsWeb:         3,
		Deduplication: false,
	})
	if hr == nil || hr.Code != 200 {
		return []string{}
	}

	var result []string
	for _, item := range hr.Data.Arr {
		if item.Domain != "" {
			result = append(result, item.Domain)
		}
	}
	return result
}
