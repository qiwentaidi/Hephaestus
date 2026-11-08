package shodan

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/types"
)

const (
	DefaultAPIAddress = "https://api.shodan.io"
	maxResponseBytes  = 16 << 20
)

// Client 调用 Shodan Search API 和账号信息接口。
type Client struct {
	Address    string
	Key        string
	HTTPClient *http.Client
}

func NewClient(address, key string) *Client {
	address = strings.TrimSpace(address)
	if address == "" {
		address = DefaultAPIAddress
	}
	if !strings.HasPrefix(strings.ToLower(address), "http://") && !strings.HasPrefix(strings.ToLower(address), "https://") {
		address = "https://" + strings.TrimLeft(address, "/")
	}
	return &Client{
		Address: strings.TrimRight(address, "/"),
		Key:     strings.TrimSpace(key),
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Search 调用 /shodan/host/search。Shodan 每页最多返回 100 条。
func (c *Client) Search(query string, page int) *types.ShodanSearchResponse {
	result := &types.ShodanSearchResponse{Matches: make([]types.ShodanMatch, 0)}
	if c == nil || strings.TrimSpace(c.Key) == "" {
		result.Error = "未配置 Shodan API Key"
		return result
	}
	if page <= 0 {
		page = 1
	}
	params := url.Values{}
	params.Set("query", query)
	params.Set("page", strconv.Itoa(page))
	params.Set("minify", "true")
	if err := c.get("/shodan/host/search", params, result); err != nil {
		result.Error = err.Error()
	}
	if result.Matches == nil {
		result.Matches = make([]types.ShodanMatch, 0)
	}
	return result
}

// APIInfo 调用 /api-info 获取账号套餐和额度。
func (c *Client) APIInfo() *types.ShodanUserInfo {
	result := &types.ShodanUserInfo{}
	if c == nil || strings.TrimSpace(c.Key) == "" {
		result.Error = "未配置 Shodan API Key"
		return result
	}
	if err := c.get("/api-info", nil, result); err != nil {
		result.Error = err.Error()
	}
	return result
}

func (c *Client) get(path string, params url.Values, out any) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("key", c.Key)
	endpoint := c.Address + path
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建 Shodan 请求失败: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("请求 Shodan 失败: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("读取 Shodan 响应失败: %w", err)
	}

	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		if response.StatusCode >= http.StatusBadRequest {
			return fmt.Errorf("Shodan 请求失败 (HTTP %d)", response.StatusCode)
		}
		return fmt.Errorf("解析 Shodan 响应失败: %w", err)
	}
	if envelope.Error != "" {
		return fmt.Errorf("%s", humanizeError(envelope.Error, response.StatusCode))
	}
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%s", humanizeError("", response.StatusCode))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析 Shodan 响应失败: %w", err)
	}
	return nil
}

func humanizeError(message string, status int) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(lower, "requires membership"):
		return "Shodan API Key 权限不足：搜索接口需要 Membership 及以上套餐（当前 Key 为 OSS/免费套餐）"
	case strings.Contains(lower, "invalid api key"), strings.Contains(lower, "invalid key"):
		return "Shodan API Key 无效"
	case strings.Contains(lower, "query credits"):
		return "Shodan 查询额度不足，无法执行该查询"
	case strings.Contains(lower, "rate limit"), status == http.StatusTooManyRequests:
		return "Shodan 请求过于频繁，请稍后重试"
	case message != "":
		return "Shodan 查询失败: " + message
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "Shodan API Key 无效或权限不足"
	default:
		return fmt.Sprintf("Shodan 请求失败 (HTTP %d)", status)
	}
}
