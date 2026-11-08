package space

import (
	"fmt"
	"strings"

	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/space-engine/censys"
	"hephaestus-backend/pkg/space-engine/fofa"
	"hephaestus-backend/pkg/space-engine/hunter"
	"hephaestus-backend/pkg/space-engine/quake"
	"hephaestus-backend/pkg/types"
)

// Uncover 聚合搜索多个空间引擎
func Uncover(query, queryType string, o types.SpaceOption) []types.SpaceResult {
	var results []types.SpaceResult

	// 默认查询1页，最大10页
	maxPages := o.MaxPages
	if maxPages <= 0 {
		maxPages = 1
	}
	if maxPages > 10 {
		maxPages = 10
	}

	// FOFA 搜索
	if o.EnableFofa && o.FofaApi != "" && o.FofaEmail != "" && o.FofaKey != "" {
		config := fofa.NewFofaClient(&types.FofaAuth{
			Address: o.FofaApi,
			Email:   o.FofaEmail,
			Key:     o.FofaKey,
		})
		fofaQuery := FormatQuery("fofa", queryType, query)
		for page := 1; page <= maxPages; page++ {
			fs := config.FofaApiSearch(fofaQuery, "100", fmt.Sprintf("%d", page), false, false)
			logger.Info("[uncover] fofa page %d: %d results", page, fs.Size)
			for _, r := range fs.Results {
				results = append(results, types.SpaceResult{
					URL:        r.URL,
					IP:         r.IP,
					Domain:     r.Domain,
					Port:       r.Port,
					Protocol:   r.Protocol,
					Title:      r.Title,
					Components: r.Product,
					ICP:        r.ICP,
					Source:     "FOFA",
				})
			}
			// 如果返回结果少于每页数量，说明已经没有更多数据
			if fs.Size < 100 {
				break
			}
		}
	}

	// Hunter 搜索
	if o.EnableHunter && o.HunterKey != "" {
		hunterClient := hunter.NewHunterClient(&types.HunterAuth{
			API: "https://hunter.qianxin.com/",
			Key: o.HunterKey,
		})
		hunterQuery := FormatQuery("hunter", queryType, query)
		for page := 1; page <= maxPages; page++ {
			hs := hunterClient.HunterApiSearch(&types.HunterSearchRequest{
				Query:         hunterQuery,
				Page:          page,
				PageSize:      100,
				TimeFilter:    "0", // 最近一个月
				IsWeb:         3,   // 全部资产
				Deduplication: false,
			})
			logger.Info("[uncover] hunter page %d: %d results", page, len(hs.Data.Arr))
			for _, r := range hs.Data.Arr {
				var component []string
				for _, c := range r.Component {
					compStr := c.Name
					if c.Version != "" {
						compStr = fmt.Sprintf("%s/%s", c.Name, c.Version)
					}
					if compStr != "" {
						component = append(component, compStr)
					}
				}
				results = append(results, types.SpaceResult{
					URL:        r.URL,
					IP:         r.IP,
					Domain:     r.Domain,
					Port:       fmt.Sprintf("%v", r.Port),
					Protocol:   r.Protocol,
					Title:      r.WebTitle,
					Components: strings.Join(component, ","),
					ICP:        r.Number,
					ICPName:    r.Company,
					Source:     "Hunter",
				})
			}
			// 如果返回结果少于每页数量，说明已经没有更多数据
			if len(hs.Data.Arr) < 100 {
				break
			}
		}
	}

	// Quake 搜索
	if o.EnableQuake && o.QuakeKey != "" {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("[uncover] quake panic: %v", r)
				}
			}()
			quakeClient := quake.NewQuakeClient(&types.QuakeAuth{
				Token: o.QuakeKey,
			})
			quakeQuery := FormatQuery("quake", queryType, query)
			for page := 1; page <= maxPages; page++ {
				options := &types.QuakeRequestOptions{
					Query:    quakeQuery,
					PageNum:  page,
					PageSize: 100,
					Latest:   true,
				}
				result := quakeClient.QuakeApiSearch(options)
				if result == nil {
					logger.Error("[uncover] quake error: result is nil")
					break
				}
				if result.Code != 0 {
					logger.Error("[uncover] quake error: code=%d, message=%s", result.Code, result.Message)
					break
				}
				if result.Data != nil {
					logger.Info("[uncover] quake page %d: %d results", page, len(result.Data))
					for _, r := range result.Data {
						results = append(results, types.SpaceResult{
							URL:        r.URL,
							IP:         r.IP,
							Domain:     r.Host,
							Port:       fmt.Sprintf("%v", r.Port),
							Protocol:   r.Protocol,
							Title:      r.Title,
							Components: strings.Join(r.Components, ","),
							ICP:        r.IcpNumber,
							ICPName:    r.IcpName,
							Source:     "Quake",
						})
					}
					// 如果返回结果少于每页数量，说明已经没有更多数据
					if len(result.Data) < 100 {
						break
					}
				}
			}
		}()
	}

	// Censys 搜索
	if o.EnableCensys && o.CensysToken != "" {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("[uncover] censys panic: %v", r)
				}
			}()
			censysClient := censys.NewCensysClient(&types.CensysAuth{
				Token:          o.CensysToken,
				OrganizationID: o.CensysOrgID,
			})
			censysQuery := FormatQuery("censys", queryType, query)
			for page := 1; page <= maxPages; page++ {
				cs := censysClient.CensysApiSearch(&types.CensysSearchRequest{
					Query:    censysQuery,
					Page:     page,
					PageSize: 100,
				})
				if cs == nil {
					logger.Error("[uncover] censys error: result is nil")
					break
				}
				if cs.Code != 0 {
					logger.Error("[uncover] censys error: code=%d, message=%s", cs.Code, cs.Message)
					break
				}
				if cs.Data != nil {
					logger.Info("[uncover] censys page %d: %d results", page, len(cs.Data))
					for _, r := range cs.Data {
						results = append(results, types.SpaceResult{
							URL:        r.URL,
							IP:         r.IP,
							Domain:     r.Domain,
							Port:       fmt.Sprintf("%v", r.Port),
							Protocol:   r.Protocol,
							Title:      r.Title,
							Components: strings.Join(r.Components, ","),
							Source:     "Censys",
						})
					}
					// 如果返回结果少于每页数量，说明已经没有更多数据
					if len(cs.Data) < 100 {
						break
					}
				}
			}
		}()
	}

	// 去重：基于 URL 和 IP:Port:Title 的组合
	uniqueResults := make([]types.SpaceResult, 0)
	seenURL := make(map[string]bool)
	seenKey := make(map[string]bool)

	for _, r := range results {
		key := fmt.Sprintf("%s:%s:%s", r.IP, r.Port, r.Title)
		if !seenURL[r.URL] && !seenKey[key] {
			seenURL[r.URL] = true
			seenKey[key] = true
			uniqueResults = append(uniqueResults, r)
		}
	}

	return uniqueResults
}

// formatQueryString 格式化查询字符串
func formatQueryString(prefix string, queries []string, operator string) string {
	var temp []string
	for _, v := range queries {
		temp = append(temp, fmt.Sprintf("%s\"%s\"", prefix, v))
	}
	return strings.Join(temp, operator)
}

// FormatQuery 根据空间引擎类型格式化查询语句
func FormatQuery(space, queryType, query string) string {
	qs := strings.Split(query, ",")
	operator := " || "
	var prefix string

	switch space {
	case "fofa":
		switch queryType {
		case "标题":
			prefix = "title="
		case "备案号":
			prefix = "icp="
		case "备案名称":
			return "不支持"
		case "域名":
			prefix = "domain="
		case "IP":
			prefix = "ip="
		case "应用", "组件", "App", "app":
			prefix = "app="
		case "产品", "Product", "product":
			prefix = "product="
		default:
			prefix = query
		}
	case "hunter":
		switch queryType {
		case "标题":
			prefix = "title="
		case "备案号":
			prefix = "icp.number="
		case "备案名称":
			prefix = "icp.name="
		case "IP":
			prefix = "ip="
		case "应用", "组件", "App", "app", "产品", "Product", "product":
			prefix = "app.name="
		default:
			prefix = query
		}
	case "quake":
		operator = " OR "
		switch queryType {
		case "标题":
			prefix = "title:"
		case "备案号":
			prefix = "icp:"
		case "备案名称":
			prefix = "icp_keywords:"
		case "域名":
			prefix = "domain:"
		case "IP":
			prefix = "ip:"
		case "应用", "组件", "App", "app", "产品", "Product", "product":
			prefix = "app:"
		default:
			prefix = query
		}
	case "censys":
		operator = " OR "
		switch queryType {
		case "标题":
			prefix = "host.services.http.response.html_title="
		case "备案号":
			prefix = "host.services.http.response.html_title="
		case "备案名称":
			prefix = "host.services.http.response.html_title="
		case "域名":
			prefix = "host.dns.names="
		case "IP":
			prefix = "host.ip="
		case "应用", "组件", "App", "app", "产品", "Product", "product":
			prefix = "host.services.software.product="
		default:
			prefix = query
		}
	default:
		return ""
	}

	return formatQueryString(prefix, qs, operator)
}
