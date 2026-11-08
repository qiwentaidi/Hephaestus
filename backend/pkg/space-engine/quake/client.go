package quake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"hephaestus-backend/pkg/types"

	"github.com/qiwentaidi/clients"
)

const (
	quakeTipsApi   = "https://quake.360.net/api/visitor/search/app"
	quakeServerApi = "https://quake.360.net/api/v3/search/quake_service"
	quakeUserApi   = "https://quake.360.net/api/v3/user/info"
)

type QuakeClient struct {
	Auth *types.QuakeAuth
}

func NewQuakeClient(auth *types.QuakeAuth) *QuakeClient {
	return &QuakeClient{
		Auth: auth,
	}
}

// QuakeApiSearch 执行搜索
func (q *QuakeClient) QuakeApiSearch(req *types.QuakeRequestOptions) *types.QuakeResult {
	result := types.QuakeResult{
		// 确保 JSON 始终输出 [] 而不是 null，避免前端出现 data:null
		Data: make([]types.QuakeData, 0),
	}

	type quakeRawItem struct {
		Components []types.Component `json:"components"`
		Hostname   string            `json:"hostname"`
		Org        string            `json:"org"`
		Port       int               `json:"port"`
		Service    types.Service     `json:"service"`
		Version    string            `json:"version"`
		IP         string            `json:"ip"`
		Location   types.Location    `json:"location"`
		IsIPv6     bool              `json:"is_ipv6"`
		Transport  string            `json:"transport"`
		Time       string            `json:"time"`
		ASN        int               `json:"asn"`
		ID         string            `json:"id"`
	}
	type quakeRawEnvelope struct {
		Code    interface{}     `json:"code"`
		Message string          `json:"message"`
		Credit  interface{}     `json:"credit"`
		Data    json.RawMessage `json:"data"`
		Meta    struct {
			Pagination struct {
				Count     int `json:"count"`
				PageIndex int `json:"page_index"`
				PageSize  int `json:"page_size"`
				Total     int `json:"total"`
			} `json:"pagination"`
		} `json:"meta"`
	}

	// 构建请求体
	requestBody := map[string]interface{}{
		"start":  (req.PageNum - 1) * req.PageSize,
		"size":   req.PageSize,
		"latest": req.Latest,
	}

	// 如果是批量查询，使用 ip_list，否则使用 query
	if len(req.IpList) > 0 {
		requestBody["ip_list"] = req.IpList
	} else {
		requestBody["query"] = req.Query
	}

	bytesData, _ := json.Marshal(requestBody)

	header := map[string]string{
		"Content-Type": "application/json",
		"X-QuakeToken": q.Auth.Token,
	}

	resp, err := clients.DoRequest("POST", quakeServerApi, header, bytes.NewReader(bytesData), 20, clients.DefaultRestyClient())

	if err != nil {
		result.Code = -1
		result.Message = "请求失败: " + err.Error()
		return &result
	}

	// 获取响应体
	bodyBytes := resp.Body()
	bodyStr := string(bodyBytes)
	// 检查特殊响应
	if bodyStr == "/quake/login" {
		result.Code = 302
		result.Message = "Token is error"
		return &result
	}

	if bodyStr == "暂不支持搜索该内容" {
		result.Code = 302
		result.Message = bodyStr
		return &result
	}

	var qrk quakeRawEnvelope
	if err := json.Unmarshal(bodyBytes, &qrk); err != nil {
		result.Code = -1
		result.Message = "解析响应失败: " + err.Error()
		return &result
	}

	rawCode := fmt.Sprintf("%v", qrk.Code)
	// 解析 code。Quake 在权限不足等场景会返回字符串错误码，如 u3025。
	// 这类响应不应再继续按正常数据数组解析。
	if code, err := strconv.Atoi(rawCode); err == nil {
		result.Code = code
	} else {
		result.Code = -1
	}
	if credit, err := strconv.Atoi(fmt.Sprintf("%v", qrk.Credit)); err == nil {
		result.Credit = credit
	}
	result.Total = qrk.Meta.Pagination.Total
	rawDataCount := 0

	logQuery := req.Query
	if len(req.IpList) > 0 {
		logQuery = fmt.Sprintf("ip_list(%d)", len(req.IpList))
	}
	if len(logQuery) > 200 {
		logQuery = logQuery[:200] + "..."
	}

	// 如果 code 不是 0，说明有错误，不解析 Data
	if result.Code != 0 {
		result.Message = translateQuakeError(rawCode, qrk.Message)
		return &result
	}

	if len(qrk.Data) == 0 || string(qrk.Data) == "null" || string(qrk.Data) == "{}" {
		result.Message = qrk.Message
		if result.Message == "" {
			result.Message = "查询成功"
		}
		return &result
	}

	var items []quakeRawItem
	if err := json.Unmarshal(qrk.Data, &items); err != nil {
		result.Code = -1
		result.Message = "解析响应失败: " + err.Error()
		return &result
	}

	rawDataCount = len(items)
	fmt.Printf("[Quake] search summary query=%q page=%d size=%d latest=%t code=%d total=%d rawDataCount=%d\n",
		logQuery, req.PageNum, req.PageSize, req.Latest, result.Code, result.Total, rawDataCount)

	for _, item := range items {
		// 确定协议
		protocol := item.Service.Name
		if protocol == "http/ssl" {
			protocol = "https"
		}
		if protocol == "" {
			protocol = "http"
		}

		// 构建 URL
		host := item.Service.HTTP.Host
		if host == "" {
			host = item.Hostname
		}
		if host == "" {
			host = item.IP
		}

		var url string
		if item.Port == 80 || item.Port == 443 {
			url = fmt.Sprintf("%s://%s", protocol, host)
		} else {
			url = fmt.Sprintf("%s://%s:%d", protocol, host, item.Port)
		}

		// 组件处理（合并版本号）
		components := make([]string, 0)
		for _, comp := range item.Components {
			var compName string
			if comp.ProductNameCN != "" {
				compName = comp.ProductNameCN
			} else if comp.ProductNameEN != "" {
				compName = comp.ProductNameEN
			}
			if compName != "" {
				if comp.Version != "" {
					components = append(components, fmt.Sprintf("%s/%s", compName, comp.Version))
				} else {
					components = append(components, compName)
				}
			}
		}
		var faviconURL string
		if fav, ok := item.Service.HTTP.Favicon.(map[string]interface{}); ok {
			if s3URL, ok := fav["s3_url"].(string); ok {
				faviconURL = s3URL
			}
		}

		quakeData := types.QuakeData{
			URL:        url,
			IP:         item.IP,
			Port:       item.Port,
			Protocol:   protocol,
			Host:       item.Service.HTTP.Host,
			Title:      item.Service.HTTP.Title,
			Components: components,
			IcpName:    item.Service.HTTP.ICP.Main_licence.Unit,
			IcpNumber:  item.Service.HTTP.ICP.Main_licence.Licence,
			CertName:   extractOrg(item.Service.Cert),
			FaviconURL: faviconURL,
			Isp:        item.Location.ISP,
			Position: extractPosition(types.Position{
				Country:   item.Location.CountryCN,
				Province:  item.Location.ProvinceCN,
				City:      item.Location.CityCN,
				District:  item.Location.DistrictCN,
				Connector: "/",
			}),
		}

		// 至少保证 IP/Host 存在其一，避免塞入无效空记录
		if quakeData.IP == "" && quakeData.Host == "" {
			continue
		}
		result.Data = append(result.Data, quakeData)
	}

	// 设置消息
	result.Message = qrk.Message
	if result.Message == "" {
		result.Message = "查询成功"
	}

	return &result
}

func translateQuakeError(rawCode string, fallback string) string {
	code := strings.ToLower(strings.TrimSpace(rawCode))
	switch code {
	case "u3025":
		return "当前语法需要高级会员权限"
	case "u401", "401":
		return "Quake Token 无效或已失效"
	case "u403", "403":
		return "当前账号无权执行该查询"
	case "u429", "429":
		return "查询过于频繁，请稍后重试"
	}

	message := strings.TrimSpace(fallback)
	if message == "" || strings.EqualFold(message, "Successful.") {
		return "查询失败"
	}
	return message
}

// QuakeApiGetTips 获取搜索提示
func (q *QuakeClient) QuakeApiGetTips(keyword string) *types.QuakeTipsResult {
	var result types.QuakeTipsResult
	jsonData := fmt.Sprintf(`{"app_name":"%v","device":{"UUID":"aa963dba-1bfa-54cf-9fdd-7b9be5a30890"}}`, keyword)
	header := map[string]string{
		"Content-Type": "application/json",
	}
	resp, err := clients.DoRequest("POST", quakeTipsApi, header, strings.NewReader(jsonData), 10, clients.NewRestyClient(nil, true))
	if err != nil {
		return &result
	}
	json.Unmarshal(resp.Body(), &result)
	return &result
}

// QuakeApiGetUserInfo 获取用户信息
func (q *QuakeClient) QuakeApiGetUserInfo(ctx context.Context) *types.QuakeUserInfo {
	var result types.QuakeUserInfo

	client := clients.DefaultRestyClient()
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("X-QuakeToken", q.Auth.Token).
		Get(quakeUserApi)

	if err != nil {
		result.Code = -1
		result.Message = "请求失败: " + err.Error()
		return &result
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		result.Code = -1
		result.Message = "解析响应失败: " + err.Error()
		return &result
	}

	return &result
}

func extractOrg(cert string) string {
	re := regexp.MustCompile(`Subject:.*?O=([^,]+)`)
	match := re.FindStringSubmatch(cert)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func extractPosition(position types.Position) string {
	fields := []string{}
	if position.Country != "" {
		fields = append(fields, position.Country)
	}
	if position.Province != "" && position.Province != position.City {
		fields = append(fields, position.Province)
	}
	if position.City != "" {
		fields = append(fields, position.City)
	}
	if position.District != "" {
		fields = append(fields, position.District)
	}
	return strings.Join(fields, position.Connector)
}
