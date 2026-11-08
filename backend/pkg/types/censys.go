package types

// CensysAuth Censys 认证信息
type CensysAuth struct {
	Token          string `json:"token"`          // Personal Access Token
	OrganizationID string `json:"organizationId"` // Optional organization ID
}

// CensysSearchRequest Censys 搜索请求
type CensysSearchRequest struct {
	Query    string `json:"query"`    // 查询语句
	Page     int    `json:"page"`     // 页码，从1开始
	PageSize int    `json:"pageSize"` // 每页数量
}

// CensysSearchResult Censys 搜索结果
type CensysSearchResult struct {
	Code    int                `json:"code"`    // 响应状态码
	Message string             `json:"message"` // 提示信息
	Data    []CensysResultData `json:"data"`    // 搜索结果数据
	Total   int                `json:"total"`   // 总结果数
}

// CensysResultData Censys 单个搜索结果
type CensysResultData struct {
	IP         string   `json:"ip"`         // IP 地址
	Port       int      `json:"port"`       // 端口
	Protocol   string   `json:"protocol"`   // 协议
	Domain     string   `json:"domain"`     // 域名
	Title      string   `json:"title"`      // 标题
	Components []string `json:"components"` // 组件列表
	URL        string   `json:"url"`        // URL
}

// CensysAPIResponse Censys API 原始响应
type CensysAPIResponse struct {
	Code   int    `json:"code,omitempty"`
	Status string `json:"status,omitempty"`
	Result struct {
		Query string `json:"query"`
		Total int    `json:"total"`
		Links struct {
			Next string `json:"next"`
			Prev string `json:"prev"`
		} `json:"links"`
		Hits []CensysAPIHit `json:"hits"`
	} `json:"result"`
	Message string `json:"message,omitempty"`
}

// CensysAPIHit Censys API 单个命中结果
type CensysAPIHit struct {
	IP       string `json:"ip"`
	Services []struct {
		Port              int    `json:"port"`
		ServiceName       string `json:"service_name"`
		TransportProtocol string `json:"transport_protocol"`
		HTTP              *struct {
			Response struct {
				Headers struct {
					Server []string `json:"server"`
					Title  string   `json:"title"`
				} `json:"headers"`
			} `json:"response"`
		} `json:"http,omitempty"`
	} `json:"services"`
	Location struct {
		CountryCode string `json:"country_code"`
		Province    string `json:"province"`
		City        string `json:"city"`
	} `json:"location"`
	DNS struct {
		Names []string `json:"names"`
	} `json:"dns"`
}
