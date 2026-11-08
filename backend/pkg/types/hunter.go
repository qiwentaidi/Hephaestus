package types

// HunterAuth Hunter 认证信息
type HunterAuth struct {
	API   string `json:"api"`
	Key   string `json:"key"`
	Proxy string `json:"proxy,omitempty"`
}

// HunterSearchRequest Hunter 搜索请求
type HunterSearchRequest struct {
	Query         string `json:"query" binding:"required"`
	Page          int    `json:"page"`
	PageSize      int    `json:"pageSize"`
	TimeFilter    string `json:"timeFilter"`    // 时间筛选：0-最近一个月，1-最近半年，2-最近一年
	IsWeb         int    `json:"isWeb"`         // 1-WEB服务资产，2-非WEB服务资产，3-全部资产
	Deduplication bool   `json:"deduplication"` // 是否去重
}

// HunterSearchResponse Hunter 搜索响应
type HunterSearchResponse struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Data    struct {
		AccountType  string          `json:"account_type"`
		Arr          []HunterDataArr `json:"arr"`
		ConsumeQuota string          `json:"consume_quota"`
		RestQuota    string          `json:"rest_quota"`
		SyntaxPrompt string          `json:"syntax_prompt"`
		Time         int64           `json:"time"`
		Total        int64           `json:"total"`
	} `json:"data"`
}

// HunterDataArr Hunter 数据项
type HunterDataArr struct {
	AsOrg          string            `json:"as_org"`
	Banner         string            `json:"banner"`
	BaseProtocol   string            `json:"base_protocol"`
	City           string            `json:"city"`
	Company        string            `json:"company"`
	Component      []HunterComponent `json:"component"`
	Country        string            `json:"country"`
	Domain         string            `json:"domain"`
	IP             string            `json:"ip"`
	IsRisk         string            `json:"is_risk"`
	IsRiskProtocol string            `json:"is_risk_protocol"`
	IsWeb          string            `json:"is_web"`
	Isp            string            `json:"isp"`
	Number         string            `json:"number"`
	Os             string            `json:"os"`
	Port           int64             `json:"port"`
	Protocol       string            `json:"protocol"`
	Province       string            `json:"province"`
	StatusCode     int64             `json:"status_code"`
	UpdatedAt      string            `json:"updated_at"`
	URL            string            `json:"url"`
	WebTitle       string            `json:"web_title"`
}

// HunterComponent Hunter 组件信息
type HunterComponent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// HunterUserInfo Hunter 用户信息
type HunterUserInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Type            string `json:"type"`              // 账号类型
		RestEquityPoint int    `json:"rest_equity_point"` // 剩余权益积分
		RestFreePoint   int    `json:"rest_free_point"`   // 当日剩余免费积分
		RestExportQuota int    `json:"rest_export_quota"` // 当日剩余导出额度
		DayFreePoint    int    `json:"day_free_point"`    // 当日免费积分上限
		DayExportQuota  int    `json:"day_export_quota"`  // 当日导出额度上限
		OnceExportQuota int    `json:"once_export_quota"` // 单次导出额度上限
		PersonalInfo    struct {
			Username string `json:"username"`  // 用户昵称
			Phone    string `json:"phone"`     // 用户手机号
			IsCharge bool   `json:"is_charge"` // 是否为充值用户
		} `json:"personal_info"`
	} `json:"data"`
}

// HunterTips Hunter 提示响应
type HunterTips struct {
	Code    int            `json:"code"`
	Data    HunterTipsData `json:"data"`
	Message string         `json:"message"`
}

// HunterTipsData Hunter 提示数据
type HunterTipsData struct {
	App     []HunterTipsApp `json:"app"`
	Collect []interface{}   `json:"collect"`
}

// HunterTipsApp Hunter 提示应用
type HunterTipsApp struct {
	Name     string   `json:"name"`
	AssetNum int      `json:"asset_num"`
	Tags     []string `json:"tags"`
}
