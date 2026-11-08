package types

// FofaAuth FOFA 认证信息
type FofaAuth struct {
	Email   string `json:"email"`
	Key     string `json:"key"`
	Address string `json:"address"` // API 地址，默认为 https://fofa.info
}

// FofaResult FOFA 搜索结果
type FofaResult struct {
	URL      string `json:"URL"`
	Host     string `json:"Host"`
	Title    string `json:"Title"`
	IP       string `json:"IP"`
	Port     string `json:"Port"`
	Domain   string `json:"Domain"`
	Protocol string `json:"Protocol"`
	Country  string `json:"Country"`
	ICP      string `json:"ICP"`
	Product  string `json:"Product"`
}

// FofaSearchResult FOFA 搜索结果响应
type FofaSearchResult struct {
	Error   bool         `json:"error"`
	Message string       `json:"message"`
	Size    int          `json:"size"`
	Results []FofaResult `json:"results"`
}

// FofaRawResult FOFA API 原始响应
type FofaRawResult struct {
	Error   bool       `json:"error"`
	Errmsg  string     `json:"errmsg"`
	Size    int        `json:"size"`
	Results [][]string `json:"results"`
}

// FofaUserInfo FOFA 用户信息
type FofaUserInfo struct {
	Error           bool   `json:"error"`
	Email           string `json:"email"`
	Username        string `json:"username"`
	Category        string `json:"category"`
	Fcoin           int    `json:"fcoin"`
	FofaPoint       int    `json:"fofa_point"`
	RemainFreePoint int    `json:"remain_free_point"`
	RemainApiQuery  int    `json:"remain_api_query"`
	RemainApiData   int    `json:"remain_api_data"`
	Isvip           bool   `json:"isvip"`
	VipLevel        int    `json:"vip_level"`
	IsVerified      bool   `json:"is_verified"`
	Avatar          string `json:"avatar"`
	Message         string `json:"message"`
	FofacliVer      string `json:"fofacli_ver"`
	FofaServer      bool   `json:"fofa_server"`
}

// SearchRequest 搜索请求
type SearchRequest struct {
	Query    string `json:"query" binding:"required"`
	PageSize int    `json:"pageSize"`
	PageNum  int    `json:"pageNum"`
	Fraud    bool   `json:"fraud"`
	Cert     bool   `json:"cert"`
}

// IconHashRequest 图标hash请求
type IconHashRequest struct {
	URL       string `json:"url"`       // 图片URL
	ImageData string `json:"imageData"` // base64编码的图片数据（data:image/...;base64,...格式）
}

// StatsRequest 统计聚合请求
type StatsRequest struct {
	Query  string `json:"query" binding:"required"`  // 查询语句
	Fields string `json:"fields" binding:"required"` // 统计字段，多个字段用逗号分隔，如：host,ip,port
}

// StatsItem 统计项
type StatsItem struct {
	Name  string `json:"name"`  // 统计项名称
	Count int    `json:"count"` // 统计数量
}

// StatsResponse 统计聚合响应
type StatsResponse struct {
	Error   bool                   `json:"error"`
	Message string                 `json:"message"`
	Data    map[string][]StatsItem `json:"data"` // key为字段名，value为该字段的统计列表
}

// FofaStatsAPIResponse FOFA 统计聚合 API 原始响应
type FofaStatsAPIResponse struct {
	Error           bool `json:"error"`
	ConsumedFpoint  int  `json:"consumed_fpoint"`
	RequiredFpoints int  `json:"required_fpoints"`
	Size            int  `json:"size"`
	Distinct        struct {
		Domain int `json:"domain"`
		IP     int `json:"ip"`
		Ipc    int `json:"ipc"`
		Server int `json:"server"`
	} `json:"distinct"`
	Aggs struct {
		Countries []interface{} `json:"countries"`
		Domain    []struct {
			Count int    `json:"count"`
			Name  string `json:"name"`
		} `json:"domain"`
		Port []struct {
			Count int    `json:"count"`
			Name  string `json:"name"`
		} `json:"port"`
		Protocol []struct {
			Count int    `json:"count"`
			Name  string `json:"name"`
		} `json:"protocol"`
		Server []struct {
			Count int    `json:"count"`
			Name  string `json:"name"`
		} `json:"server"`
	} `json:"aggs"`
	Query          string `json:"query"`
	Lastupdatetime string `json:"lastupdatetime"`
}
