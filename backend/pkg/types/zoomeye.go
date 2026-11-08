package types

import "encoding/json"

// ZoomeyeAuth ZoomEye 认证信息
type ZoomeyeAuth struct {
	Address string `json:"address"` // API 地址，默认为 https://api.zoomeye.ai/
	Key     string `json:"key"`
	Proxy   string `json:"proxy,omitempty"`
}

// ZoomeyeSearchRequest ZoomEye 搜索请求
type ZoomeyeSearchRequest struct {
	Query       string `json:"query"`
	Page        int    `json:"page"`
	PageSize    int    `json:"pageSize"`
	SubType     string `json:"subType,omitempty"`
	Fields      string `json:"fields,omitempty"`
	IgnoreCache bool   `json:"ignoreCache,omitempty"`
}

// ZoomeyeSearchResponse ZoomEye 搜索响应
type ZoomeyeSearchResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Total   int               `json:"total"`
	Query   string            `json:"query"`
	Data    []ZoomeyeDataItem `json:"data"`
}

// ZoomeyeDataItem ZoomEye 单条资产
type ZoomeyeDataItem struct {
	URL          string   `json:"url"`
	IP           string   `json:"ip"`
	Domain       string   `json:"domain"`
	Hostname     string   `json:"hostname"`
	Port         int      `json:"port"`
	Service      string   `json:"service"`
	Protocol     string   `json:"protocol"`
	Title        []string `json:"title"`
	Version      string   `json:"version"`
	Device       string   `json:"device"`
	Product      string   `json:"product"`
	Header       string   `json:"header"`
	Body         string   `json:"body"`
	Banner       string   `json:"banner"`
	UpdateTime   string   `json:"update_time"`
	CountryName  string   `json:"country.name"`
	ProvinceName string   `json:"province.name"`
	CityName     string   `json:"city.name"`
	ISPName      string   `json:"isp.name"`
	OrgName      string   `json:"organization.name"`
	SSL          string   `json:"ssl"`
	Jarm         string   `json:"ssl.jarm"`
	JA3S         string   `json:"ssl.ja3s"`
	IconHashMD5  string   `json:"iconhash_md5"`
}

// ZoomeyeUserInfo ZoomEye 用户信息
type ZoomeyeUserInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Username     string `json:"username"`
		Email        string `json:"email"`
		Phone        string `json:"phone"`
		CreatedAt    string `json:"created_at"`
		Subscription struct {
			Plan          string      `json:"plan"`
			EndDate       string      `json:"end_date"`
			Points        json.Number `json:"points"`
			ZoomeyePoints json.Number `json:"zoomeye_points"`
		} `json:"subscription"`
	} `json:"data"`
}
