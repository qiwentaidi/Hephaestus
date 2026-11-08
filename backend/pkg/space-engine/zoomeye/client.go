package zoomeye

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"hephaestus-backend/pkg/types"

	"github.com/qiwentaidi/clients"
)

const (
	DefaultAPIAddress = "https://api.zoomeye.ai/"
)

type ZoomeyeClient struct {
	Auth *types.ZoomeyeAuth
}

func NewZoomeyeClient(auth *types.ZoomeyeAuth) *ZoomeyeClient {
	if auth == nil {
		auth = &types.ZoomeyeAuth{}
	}
	if strings.TrimSpace(auth.Address) == "" {
		auth.Address = DefaultAPIAddress
	}
	return &ZoomeyeClient{Auth: auth}
}

func BuildAPIURL(address, endpoint string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		address = DefaultAPIAddress
	}
	if !strings.HasPrefix(strings.ToLower(address), "http://") && !strings.HasPrefix(strings.ToLower(address), "https://") {
		address = "https://" + strings.TrimLeft(address, "/")
	}
	return strings.TrimRight(address, "/") + "/" + strings.TrimLeft(endpoint, "/")
}

func (z *ZoomeyeClient) ZoomeyeApiSearch(req *types.ZoomeyeSearchRequest) *types.ZoomeyeSearchResponse {
	result := &types.ZoomeyeSearchResponse{
		Data: make([]types.ZoomeyeDataItem, 0),
	}
	if z == nil || z.Auth == nil || strings.TrimSpace(z.Auth.Key) == "" {
		result.Code = -1
		result.Message = "未配置 ZoomEye API Key"
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
	if pageSize > 10000 {
		pageSize = 10000
	}

	payload := map[string]interface{}{
		"qbase64":  base64.StdEncoding.EncodeToString([]byte(req.Query)),
		"page":     page,
		"pagesize": pageSize,
	}
	if strings.TrimSpace(req.SubType) != "" {
		payload["sub_type"] = req.SubType
	}
	if strings.TrimSpace(req.Fields) != "" {
		payload["fields"] = req.Fields
	}
	if req.IgnoreCache {
		payload["ignore_cache"] = true
	}

	body, _ := json.Marshal(payload)
	headers := map[string]string{
		"Content-Type": "application/json",
		"API-KEY":      z.Auth.Key,
	}

	resp, err := clients.DoRequest("POST", BuildAPIURL(z.Auth.Address, "v2/search"), headers, bytes.NewReader(body), 20, clients.NewRestyClientWithProxy(nil, true, z.Auth.Proxy))
	if err != nil {
		result.Code = -1
		result.Message = "请求失败: " + err.Error()
		return result
	}

	if err := json.Unmarshal(resp.Body(), result); err != nil {
		result.Code = -1
		result.Message = "解析响应失败: " + err.Error()
		return result
	}

	if result.Data == nil {
		result.Data = make([]types.ZoomeyeDataItem, 0)
	}
	if strings.Contains(strings.ToLower(result.Message), "resource credits is insufficient") {
		result.Message = "当前 ZoomEye 账号积分不足，无法执行该查询"
	}
	if result.Code != 60000 && result.Message == "" {
		result.Message = "ZoomEye 查询失败"
	}
	return result
}

func (z *ZoomeyeClient) ZoomeyeApiGetUserInfo() *types.ZoomeyeUserInfo {
	result := &types.ZoomeyeUserInfo{}
	if z == nil || z.Auth == nil || strings.TrimSpace(z.Auth.Key) == "" {
		result.Code = -1
		result.Message = "未配置 ZoomEye API Key"
		return result
	}

	headers := map[string]string{
		"API-KEY": z.Auth.Key,
	}
	resp, err := clients.DoRequest("POST", BuildAPIURL(z.Auth.Address, "v2/userinfo"), headers, nil, 20, clients.NewRestyClientWithProxy(nil, true, z.Auth.Proxy))
	if err != nil {
		result.Code = -1
		result.Message = "请求失败: " + err.Error()
		return result
	}
	if err := json.Unmarshal(resp.Body(), result); err != nil {
		result.Code = -1
		result.Message = "解析响应失败: " + err.Error()
		return result
	}
	return result
}

func ParseZoomeyeInt(v interface{}) int64 {
	switch value := v.(type) {
	case json.Number:
		n, err := value.Int64()
		if err == nil {
			return n
		}
		f, err := strconv.ParseFloat(value.String(), 64)
		if err == nil {
			return int64(f)
		}
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err == nil {
			return n
		}
	case int:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case float32:
		return int64(value)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprintf("%v", v)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func BuildComponentList(item types.ZoomeyeDataItem) []string {
	parts := make([]string, 0, 3)
	if item.Product != "" {
		if item.Version != "" {
			parts = append(parts, fmt.Sprintf("%s/%s", item.Product, item.Version))
		} else {
			parts = append(parts, item.Product)
		}
	}
	if item.Device != "" && item.Device != item.Product {
		parts = append(parts, item.Device)
	}
	return parts
}
