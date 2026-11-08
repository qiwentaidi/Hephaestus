package types

// ShodanIpReport Shodan IP 报告
type ShodanIpReport struct {
	Ports     []float64 `json:"ports"`
	Tags      []string  `json:"tags"`
	Vulns     []string  `json:"vulns"`
	Cpes      []string  `json:"cpes"`
	Hostnames []string  `json:"hostnames"`
	IP        string    `json:"ip"`
}

// ShodanSearchResponse Shodan 主机搜索响应
type ShodanSearchResponse struct {
	Matches []ShodanMatch `json:"matches"`
	Total   int           `json:"total"`
	Credits int           `json:"credits"`
	Error   string        `json:"error,omitempty"`
}

// ShodanMatch 单条 Shodan 搜索结果
type ShodanMatch struct {
	IP        string            `json:"ip_str"`
	Port      int               `json:"port"`
	Hostnames []string          `json:"hostnames"`
	Domains   []string          `json:"domains"`
	Product   string            `json:"product"`
	Version   string            `json:"version"`
	Transport string            `json:"transport"`
	Data      string            `json:"data"`
	Org       string            `json:"org"`
	ISP       string            `json:"isp"`
	ASN       string            `json:"asn"`
	Timestamp string            `json:"timestamp"`
	CPE       []string          `json:"cpe"`
	CPE23     []string          `json:"cpe23"`
	HTTP      ShodanHTTP        `json:"http"`
	Location  ShodanLocation    `json:"location"`
	SSL       map[string]any    `json:"ssl"`
	Shodan    ShodanMatchModule `json:"_shodan"`
}

// ShodanHTTP Shodan 结果中的 HTTP 信息
type ShodanHTTP struct {
	Title  string `json:"title"`
	Host   string `json:"host"`
	Server string `json:"server"`
	Status int    `json:"status"`
}

// ShodanLocation Shodan 结果中的地理位置
type ShodanLocation struct {
	CountryName string `json:"country_name"`
	CountryCode string `json:"country_code"`
	RegionCode  string `json:"region_code"`
	City        string `json:"city"`
}

// ShodanMatchModule Shodan 结果的模块信息
type ShodanMatchModule struct {
	Module string `json:"module"`
}

// ShodanUserInfo Shodan API 账号信息
type ShodanUserInfo struct {
	QueryCredits int    `json:"query_credits"`
	ScanCredits  int    `json:"scan_credits"`
	Plan         string `json:"plan"`
	Unlocked     bool   `json:"unlocked"`
	Error        string `json:"error,omitempty"`
}
