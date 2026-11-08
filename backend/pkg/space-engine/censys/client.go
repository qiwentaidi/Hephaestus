package censys

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	censyssdkgo "github.com/censys/censys-sdk-go"
	"github.com/censys/censys-sdk-go/models/components"
	"github.com/censys/censys-sdk-go/models/operations"
	"github.com/censys/censys-sdk-go/models/sdkerrors"

	"hephaestus-backend/pkg/types"
)

const requestTimeout = 20 * time.Second

type CensysClient struct {
	Auth *types.CensysAuth
	sdk  *censyssdkgo.SDK
}

func NewCensysClient(auth *types.CensysAuth) *CensysClient {
	client := &CensysClient{Auth: auth}
	if auth == nil || strings.TrimSpace(auth.Token) == "" {
		return client
	}

	opts := []censyssdkgo.SDKOption{
		censyssdkgo.WithSecurity(strings.TrimSpace(auth.Token)),
		censyssdkgo.WithTimeout(requestTimeout),
	}
	if orgID := strings.TrimSpace(auth.OrganizationID); orgID != "" {
		opts = append(opts, censyssdkgo.WithOrganizationID(orgID))
	}
	client.sdk = censyssdkgo.New(opts...)
	return client
}

// CensysApiSearch 执行 Censys 搜索
func (c *CensysClient) CensysApiSearch(req *types.CensysSearchRequest) *types.CensysSearchResult {
	result := &types.CensysSearchResult{Data: make([]types.CensysResultData, 0)}

	if c == nil || c.sdk == nil || c.Auth == nil || strings.TrimSpace(c.Auth.Token) == "" {
		result.Code = -1
		result.Message = "未配置 Censys Token"
		return result
	}

	query := strings.TrimSpace(req.Query)
	if query == "" {
		result.Code = -1
		result.Message = "查询语句不能为空"
		return result
	}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var current *components.SearchQueryResponse
	var pageToken string

	for i := 1; i <= page; i++ {
		body := components.SearchQueryInputBody{
			Query:    query,
			PageSize: censyssdkgo.Int64(int64(pageSize)),
		}
		if pageToken != "" {
			body.PageToken = censyssdkgo.String(pageToken)
		}

		sdkReq := operations.V3GlobaldataSearchQueryRequest{SearchQueryInputBody: body}
		if orgID := strings.TrimSpace(c.Auth.OrganizationID); orgID != "" {
			sdkReq.OrganizationID = censyssdkgo.String(orgID)
		}

		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		resp, err := c.sdk.GlobalData.Search(ctx, sdkReq)
		cancel()
		if err != nil {
			result.Code, result.Message = formatSearchError(err)
			if isFreePlanRestriction(result.Message) {
				if fallbackData, ok, fallbackErr := c.lookupFallback(query, page, pageSize); ok {
					result.Code = 0
					result.Data = fallbackData
					result.Total = len(fallbackData)
					result.Message = "当前账号为 Free Plan，已自动回退为 Lookup 模式（仅支持精确 IP/域名）"
					return result
				} else if strings.TrimSpace(fallbackErr) != "" {
					result.Message += "（当前 Free Plan 无法调用 Search API；自动回退 Lookup 失败：" + fallbackErr + "）"
				} else {
					result.Message += "（当前 Free Plan 无法调用 Search API，可使用精确 IP查询触发 Lookup 回退）"
				}
			} else if needsOrgHint(result.Message) {
				result.Message += "（请在后端配置中补充 Censys Organization ID）"
			}
			return result
		}

		if resp == nil || resp.ResponseEnvelopeSearchQueryResponse == nil || resp.ResponseEnvelopeSearchQueryResponse.Result == nil {
			result.Code = -1
			result.Message = "查询失败：Censys 返回空响应"
			return result
		}

		current = resp.ResponseEnvelopeSearchQueryResponse.Result
		if i < page {
			pageToken = strings.TrimSpace(current.NextPageToken)
			if pageToken == "" {
				result.Code = -1
				result.Message = "页码超出可用范围"
				return result
			}
		}
	}

	if current == nil {
		result.Code = -1
		result.Message = "查询失败：未获取到结果"
		return result
	}

	result.Code = 0
	result.Total = int(current.TotalHits)
	result.Data = flattenSearchHits(current.Hits)
	result.Message = "查询成功"
	return result
}

func flattenSearchHits(hits []components.SearchQueryHit) []types.CensysResultData {
	results := make([]types.CensysResultData, 0)
	for _, hit := range hits {
		hostAsset := hit.GetHostV1()
		if hostAsset == nil {
			continue
		}

		results = append(results, buildResultsFromHost(hostAsset.GetResource(), "")...)
	}
	return results
}

func (c *CensysClient) lookupFallback(query string, page, pageSize int) ([]types.CensysResultData, bool, string) {
	if page > 1 {
		return nil, false, "Lookup 回退仅支持第 1 页"
	}
	ips, domains := parseLookupTargets(query)
	if len(ips) == 0 && len(domains) == 0 {
		return nil, false, ""
	}
	if len(ips) == 0 && len(domains) > 0 {
		return nil, false, "Free Plan 回退仅支持精确 IP 查询，不支持域名查询"
	}

	limit := pageSize
	if limit <= 0 {
		limit = 20
	}
	results := make([]types.CensysResultData, 0)
	var errs []string

	for _, ip := range ips {
		if len(results) >= limit {
			break
		}
		req := operations.V3GlobaldataAssetHostRequest{HostID: ip}
		if orgID := strings.TrimSpace(c.Auth.OrganizationID); orgID != "" {
			req.OrganizationID = censyssdkgo.String(orgID)
		}
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		resp, err := c.sdk.GlobalData.GetHost(ctx, req)
		cancel()
		if err != nil {
			_, msg := formatSearchError(err)
			errs = append(errs, "IP "+ip+": "+msg)
			continue
		}
		if resp == nil || resp.ResponseEnvelopeHostAsset == nil || resp.ResponseEnvelopeHostAsset.Result == nil {
			errs = append(errs, "IP "+ip+": 空响应")
			continue
		}
		results = append(results, buildResultsFromHost(resp.ResponseEnvelopeHostAsset.Result.GetResource(), ip)...)
	}

	for _, domain := range domains {
		errs = append(errs, "域名 "+domain+": Free Plan 回退仅支持 IP")
	}

	if len(results) > limit {
		results = results[:limit]
	}

	if len(results) > 0 {
		return uniqueResultRows(results), true, ""
	}
	return nil, false, strings.Join(errs, "; ")
}

func parseLookupTargets(query string) ([]string, []string) {
	extract := func(pattern string) []string {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(query, -1)
		values := make([]string, 0, len(matches))
		for _, m := range matches {
			if len(m) >= 2 {
				values = append(values, strings.TrimSpace(strings.Trim(m[1], `"`)))
			}
		}
		return uniqueStrings(values)
	}

	ipMatches := extract(`(?i)\bhost\.ip\s*(?:=|:)\s*("?[^"\s\)\(]+"?)`)
	domainMatches := extract(`(?i)\bhost\.dns\.names\s*(?:=|:)\s*("?[^"\s\)\(]+"?)`)
	return ipMatches, domainMatches
}

func buildResultsFromHost(host components.Host, ipHint string) []types.CensysResultData {
	ip := firstNonEmpty(ptrString(host.GetIP()), ipHint)
	domain := ""
	if dns := host.GetDNS(); dns != nil {
		domain = firstNonEmpty(dns.GetNames()...)
	}

	services := host.GetServices()
	if len(services) == 0 {
		return []types.CensysResultData{{
			IP:         ip,
			Port:       0,
			Protocol:   "unknown",
			Domain:     domain,
			Title:      "",
			Components: nil,
			URL:        buildURL(domain, ip, 0, ""),
		}}
	}

	results := make([]types.CensysResultData, 0, len(services))
	for _, service := range services {
		port := ptrInt(service.GetPort())
		protocol := firstNonEmpty(
			ptrString(service.GetProtocol()),
			transportProtocolValue(service.GetTransportProtocol()),
		)
		if protocol == "" {
			protocol = "unknown"
		}

		title, headers := extractTitleAndServerHeaders(service)
		componentsList := make([]string, 0)
		componentsList = append(componentsList, headers...)
		for _, sw := range service.GetSoftware() {
			name := firstNonEmpty(ptrString(sw.GetProduct()), ptrString(sw.GetVendor()))
			if name != "" {
				componentsList = append(componentsList, name)
			}
		}
		componentsList = uniqueStrings(componentsList)

		results = append(results, types.CensysResultData{
			IP:         ip,
			Port:       port,
			Protocol:   protocol,
			Domain:     domain,
			Title:      title,
			Components: componentsList,
			URL:        buildURL(domain, ip, port, protocol),
		})
	}
	return results
}

func uniqueResultRows(values []types.CensysResultData) []types.CensysResultData {
	seen := make(map[string]struct{}, len(values))
	result := make([]types.CensysResultData, 0, len(values))
	for _, v := range values {
		key := fmt.Sprintf("%s|%d|%s|%s", v.IP, v.Port, v.Domain, v.Title)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, v)
	}
	return result
}

func extractTitleAndServerHeaders(service components.Service) (string, []string) {
	title := ""
	servers := make([]string, 0)

	for _, ep := range service.GetEndpoints() {
		httpInfo := ep.GetHTTP()
		if httpInfo == nil {
			continue
		}
		if title == "" {
			title = ptrString(httpInfo.GetHTMLTitle())
		}

		for key, values := range httpInfo.GetHeaders() {
			if strings.EqualFold(key, "server") {
				servers = append(servers, values.GetHeaders()...)
			}
		}
	}

	return title, uniqueStrings(servers)
}

func formatSearchError(err error) (code int, message string) {
	code = -1
	message = "查询失败"
	if err == nil {
		return code, message
	}

	switch e := err.(type) {
	case *sdkerrors.ErrorModel:
		if e.Status != nil {
			code = int(*e.Status)
		}
		message = firstNonEmpty(
			ptrString(e.Detail),
			ptrString(e.Title),
			strings.TrimSpace(e.Error()),
		)
	case *sdkerrors.SDKError:
		if e.StatusCode > 0 {
			code = e.StatusCode
		}
		message = firstNonEmpty(
			extractErrorFromBody(e.Body),
			strings.TrimSpace(e.Message),
			strings.TrimSpace(e.Error()),
		)
	default:
		message = strings.TrimSpace(err.Error())
	}

	if message == "" {
		message = "查询失败"
	}
	return code, message
}

func extractErrorFromBody(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}

	if detail := getNestedString(payload, "detail"); detail != "" {
		return detail
	}
	if title := getNestedString(payload, "title"); title != "" {
		return title
	}
	if message := getNestedString(payload, "message"); message != "" {
		return message
	}
	if code := getNestedString(payload, "error.message"); code != "" {
		return code
	}
	return raw
}

func needsOrgHint(message string) bool {
	if isFreePlanRestriction(message) {
		return false
	}
	msg := strings.ToLower(message)
	return strings.Contains(msg, "organization_id") || strings.Contains(msg, "organization id")
}

func isFreePlanRestriction(message string) bool {
	msg := strings.ToLower(message)
	return strings.Contains(msg, "free users can only access this endpoint through the platform ui")
}

func ptrString(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func ptrInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

func transportProtocolValue(tp *components.ServiceTransportProtocol) string {
	if tp == nil {
		return ""
	}
	return strings.TrimSpace(string(*tp))
}

func getNestedValue(data map[string]interface{}, path string) interface{} {
	current := interface{}(data)
	for _, part := range strings.Split(path, ".") {
		switch node := current.(type) {
		case map[string]interface{}:
			current = node[part]
			if current == nil {
				return nil
			}
		default:
			return nil
		}
	}
	return current
}

func getNestedString(data map[string]interface{}, path string) string {
	value := getNestedValue(data, path)
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

func buildURL(domain, ip string, port int, protocol string) string {
	host := domain
	if host == "" {
		host = ip
	}
	if host == "" {
		return ""
	}
	scheme := "http"
	p := strings.ToLower(protocol)
	if port == 443 || strings.Contains(p, "https") || strings.Contains(p, "tls") {
		scheme = "https"
	}
	if port > 0 && !(port == 80 || port == 443) {
		return fmt.Sprintf("%s://%s:%d", scheme, host, port)
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}
