package types

// QuakeAuth Quake 认证信息
type QuakeAuth struct {
	Token string `json:"token"`
}

// QuakeRequestOptions Quake 搜索请求选项
type QuakeRequestOptions struct {
	Query    string
	IpList   []string // 判断 IpList 是否为空决定是否为批量查询
	PageNum  int
	PageSize int
	Latest   bool
}

// 原始数据中有用的字段
type QuakeRawResult struct {
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
	Credit  interface{} `json:"credit"`
	Data    []struct {
		Components []Component `json:"components"`
		Hostname   string      `json:"hostname"`
		Org        string      `json:"org"`
		Port       int         `json:"port"`
		Service    Service     `json:"service"`
		Version    string      `json:"version"`
		IP         string      `json:"ip"`
		Location   Location    `json:"location"`
		IsIPv6     bool        `json:"is_ipv6"`
		Transport  string      `json:"transport"`
		Time       string      `json:"time"`
		ASN        int         `json:"asn"`
		ID         string      `json:"id"`
	}
	Meta struct {
		Pagination struct {
			Count     int `json:"count"`
			PageIndex int `json:"page_index"`
			PageSize  int `json:"page_size"`
			Total     int `json:"total"`
		} `json:"pagination"`
	} `json:"meta"`
}

type Component struct {
	ProductLevel   string   `json:"product_level"`
	ProductType    []string `json:"product_type"`
	ProductVendor  string   `json:"product_vendor"`
	ProductNameCN  string   `json:"product_name_cn"`
	ProductNameEN  string   `json:"product_name_en"`
	ID             string   `json:"id"`
	ProductCatalog []string `json:"product_catalog"`
	Version        string   `json:"version"`
}

type Service struct {
	Response string `json:"response"`
	Name     string `json:"name"`
	HTTP     HTTP   `json:"http"`
	Cert     string `json:"cert"`
}

type HTTP struct {
	XPoweredBy      string      `json:"x_powered_by"`
	Server          string      `json:"server"`
	Path            string      `json:"path"`
	HTMLHash        interface{} `json:"html_hash"`
	ResponseHeaders interface{} `json:"response_headers"`
	StatusCode      interface{} `json:"status_code"`
	Favicon         interface{} `json:"favicon"`
	Host            string      `json:"host"`
	Body            interface{} `json:"body"`
	MetaKeywords    interface{} `json:"meta_keywords"`
	Title           string      `json:"title"`
	ICP             struct {
		Leader_name  string `json:"leader_name"`
		Domain       string `json:"domain"`
		Main_licence struct {
			Unit    string `json:"unit"`
			Nature  string `json:"nature"`
			Licence string `json:"licence"`
		} `json:"main_licence"`
		Content_type_name string `json:"content_type_name"`
		Limit_access      bool   `json:"limit_access"`
		Licence           string `json:"licence"`
	} `json:"icp"`
}

type Favicon struct {
	Data     string `json:"data"`
	Location string `json:"location"`
	Hash     string `json:"hash"`
	S3URL    string `json:"s3_url"`
}

type Location struct {
	Owner       string    `json:"owner"`
	ProvinceCN  string    `json:"province_cn"`
	ISP         string    `json:"isp"`
	ProvinceEN  string    `json:"province_en"`
	CountryEN   string    `json:"country_en"`
	DistrictCN  string    `json:"district_cn"`
	GPS         []float64 `json:"gps"`
	StreetCN    string    `json:"street_cn"`
	CityEN      string    `json:"city_en"`
	DistrictEN  string    `json:"district_en"`
	CountryCN   string    `json:"country_cn"`
	StreetEN    string    `json:"street_en"`
	CityCN      string    `json:"city_cn"`
	CountryCode string    `json:"country_code"`
	ASName      string    `json:"asname"`
	SceneCN     string    `json:"scene_cn"`
	SceneEN     string    `json:"scene_en"`
	Radius      float64   `json:"radius"`
}

type QuakeResult struct {
	Code    int         `json:"code"`    // 响应状态信息，正常是0
	Message string      `json:"message"` // 提示信息
	Data    []QuakeData `json:"data"`
	Total   int         `json:"total"`
	Credit  int         `json:"credit"` // Quake 搜索接口返回的积分字段（具体语义以官方接口为准）
}

type QuakeData struct {
	URL        string   `json:"url"`
	Components []string `json:"components"`
	Port       int      `json:"port"`
	Protocol   string   `json:"protocol"` // 协议类型
	Host       string   `json:"host"`
	Title      string   `json:"title"`
	IcpName    string   `json:"icpName"`    // 备案单位名称
	IcpNumber  string   `json:"icpNumber"`  // 备案单位编号
	CertName   string   `json:"certName"`   // 证书申请单位
	FaviconURL string   `json:"faviconURL"` // 图标地址
	IP         string   `json:"ip"`
	Isp        string   `json:"isp"`
	Position   string   `json:"position"`
}

// QuakeUserInfo Quake 用户信息
type QuakeUserInfo struct {
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
	Data    struct {
		ID   string `json:"id"`
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Fullname string `json:"fullname"`
			Email    string `json:"email"`
		} `json:"user"`
		Baned            bool   `json:"baned"`
		BanStatus        string `json:"ban_status"`
		Credit           int    `json:"credit"`
		PersistentCredit int    `json:"persistent_credit"`
		Token            string `json:"token"`
		MobilePhone      string `json:"mobile_phone"`
		Source           string `json:"source"`
		PrivacyLog       struct {
			Status bool        `json:"status"`
			Time   interface{} `json:"time"`
		} `json:"privacy_log"`
		EnterpriseInformation struct {
			Name   interface{} `json:"name"`
			Email  interface{} `json:"email"`
			Status string      `json:"status"`
		} `json:"enterprise_information"`
		PersonalInformationStatus bool `json:"personal_information_status"`
		Role                      []struct {
			Fullname string `json:"fullname"`
			Priority int    `json:"priority"`
			Credit   int    `json:"credit"`
		} `json:"role"`
	} `json:"data"`
	Meta struct {
	} `json:"meta"`
}

// QuakeTipsResult Quake 提示结果
type QuakeTipsResult struct {
	Code    float64         `json:"code"`
	Message string          `json:"message"`
	Data    []QuakeTipsData `json:"data"`
}

// QuakeTipsData Quake 提示数据
type QuakeTipsData struct {
	Product_name string  `json:"product_name"`
	Vul_count    float64 `json:"vul_count"`
	Vendor_name  string  `json:"vendor_name"`
	Ip_count     float64 `json:"ip_count"`
}

type Position struct {
	Country   string
	Province  string
	City      string
	District  string
	Connector string // 连接符
}

type ShortcutsMeta struct {
	Meta    interface{} `json:"meta"`
	Code    float64     `json:"code"`
	Message string      `json:"message"`
	Data    []struct {
		Published      bool    `json:"published"`
		Order          float64 `json:"order"`
		Is_new         bool    `json:"is_new"`
		Put_more_tools bool    `json:"put_more_tools"`
		Id             string  `json:"id"`
		Name           string  `json:"name"`
		Description    string  `json:"description"`
		Index          string  `json:"index"`
	} `json:"data"`
}
