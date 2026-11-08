package hunter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"hephaestus-backend/pkg/types"

	"github.com/qiwentaidi/clients"
)

const defaultHunterAPI = "https://hunter.qianxin.com/"

type HunterClient struct {
	Auth *types.HunterAuth
}

func NewHunterClient(auth *types.HunterAuth) *HunterClient {
	if auth.API == "" {
		auth.API = defaultHunterAPI
	}
	return &HunterClient{
		Auth: auth,
	}
}

// HunterBaseEncode hunter base64加密接口
func HunterBaseEncode(str string) string {
	return base64.URLEncoding.EncodeToString([]byte(str))
}

// HunterApiSearch 执行搜索
func (h *HunterClient) HunterApiSearch(req *types.HunterSearchRequest) *types.HunterSearchResponse {
	var result types.HunterSearchResponse

	// 计算时间范围
	var beforeTime string
	currentTime := time.Now().Format("2006-01-02")

	switch req.TimeFilter {
	case "0":
		// 最近一个月
		beforeTime = time.Now().AddDate(0, 0, -29).Format("2006-01-02")
	case "1":
		// 最近半年
		beforeTime = time.Now().AddDate(0, 0, -179).Format("2006-01-02")
	case "2":
		// 最近一年
		beforeTime = time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	default:
		// 默认最近一个月
		beforeTime = time.Now().AddDate(0, 0, -29).Format("2006-01-02")
	}

	// 确保 API URL 以 / 结尾
	apiBase := h.Auth.API
	if apiBase != "" && apiBase[len(apiBase)-1] != '/' {
		apiBase += "/"
	}

	// 构建API地址
	apiURL := fmt.Sprintf("%sopenApi/search?api-key=%s&search=%s&page=%s&page_size=%s&is_web=%s&port_filter=%s&start_time=%s&end_time=%s",
		apiBase,
		h.Auth.Key,
		HunterBaseEncode(req.Query),
		strconv.Itoa(req.Page),
		strconv.Itoa(req.PageSize),
		strconv.Itoa(req.IsWeb),
		strconv.FormatBool(req.Deduplication),
		beforeTime,
		currentTime,
	)

	client := clients.NewRestyClientWithProxy(nil, true, h.Auth.Proxy)
	resp, err := clients.SimpleGet(apiURL, client)

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

// SearchHunterTips 搜索提示
func (h *HunterClient) SearchHunterTips(keyword string) *types.HunterTips {
	var result types.HunterTips

	apiURL := fmt.Sprintf("https://hunter.qianxin.com/api/recommend?keyword=%s", HunterBaseEncode(keyword))

	client := clients.NewRestyClientWithProxy(nil, true, h.Auth.Proxy)
	resp, err := clients.SimpleGet(apiURL, client)
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

// HunterApiGetUserInfo 获取用户信息
func (h *HunterClient) HunterApiGetUserInfo() *types.HunterUserInfo {
	var result types.HunterUserInfo

	// 确保 API URL 以 / 结尾
	apiBase := h.Auth.API
	if apiBase != "" && apiBase[len(apiBase)-1] != '/' {
		apiBase += "/"
	}

	apiURL := fmt.Sprintf("%sopenApi/userInfo?api-key=%s", apiBase, h.Auth.Key)

	client := clients.NewRestyClientWithProxy(nil, true, h.Auth.Proxy)
	resp, err := clients.SimpleGet(apiURL, client)

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
