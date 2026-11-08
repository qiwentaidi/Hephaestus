package types

// SubdomainOption 子域名扫描选项
type SubdomainOption struct {
	Domains             []string `json:"domains"`             // 要扫描的域名列表
	Subs                []string `json:"subs"`                // 子域名字典
	DnsServers          []string `json:"dnsServers"`          // DNS服务器列表
	Thread              int      `json:"thread"`              // 并发线程数
	Timeout             int      `json:"timeout"`             // 超时时间（秒）
	Proxy               string   `json:"proxy,omitempty"`     // 受控任务出口代理
	Mode                int      `json:"mode"`                // 扫描模式：1-枚举模式，2-API模式，3-仅CDN/WAF识别
	ResolveExcludeTimes int      `json:"resolveExcludeTimes"` // Deprecated: 保留兼容旧配置，不再用于过滤
	AppendEngines       []string `json:"appendEngines"`       // 使用的API引擎列表
	FofaApi             string   `json:"fofaApi"`             // FOFA API Key
	FofaEmail           string   `json:"fofaEmail"`           // FOFA Email
	FofaAddress         string   `json:"fofaAddress"`         // FOFA API地址
	HunterApi           string   `json:"hunterApi"`           // Hunter API Key
	QuakeApi            string   `json:"quakeApi"`            // Quake API Key
	ChaosApi            string   `json:"chaosApi"`            // Chaos API Key
	BevigilApi          string   `json:"bevigilApi"`          // Bevigil API Key
	ZoomeyeAddress      string   `json:"zoomeyeAddress"`      // Zoomeye API地址
	ZoomeyeApi          string   `json:"zoomeyeApi"`          // Zoomeye API Key
	GithubApi           string   `json:"githubApi"`           // Github API Key
	SecuritytrailsApi   string   `json:"securitytrailsApi"`   // Securitytrails API Key	// AI 相关配置
	CTRadarApi          string   `json:"ctRadarApi"`          // ct-radar API Key（可选）
	AIEnabled           bool     `json:"aiEnabled"`           // 是否启用AI分析
	AIApiUrl            string   `json:"aiApiUrl"`            // AI API地址（支持OpenAI兼容）
	AIApiKey            string   `json:"aiApiKey"`            // AI API Key
	AIModel             string   `json:"aiModel"`             // AI模型名称}
}
