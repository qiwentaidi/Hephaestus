package types

// VulnerabilityInfo 漏洞信息
type VulnerabilityInfo struct {
	TaskId             string // 任务ID
	ID                 string
	Name               string
	Description        string
	Impact             string
	Remediation        string
	Reference          string
	Type               string
	Severity           string
	URL                string
	Request            string
	Response           string
	ResponseTime       string
	Extract            string
	PrincipleDetection bool // 原理检测标记
}

// NucleiOptions Nuclei扫描选项
type NucleiOptions struct {
	Proxy               string            // 扫描请求使用的代理（由任务配置决定）
	RequireTemplateTags bool              // 如果没有扫描到指纹，是否需要扫描全漏洞还是直接跳过
	DisableMaxHostErr   bool              // 不限制主机最大失败数
	Threads             int               // Nuclei 漏洞扫描线程数
	Headers             []string          // 自定义请求头
	Interactsh          InteractshOptions // OOB / Interactsh 配置
	Jobs                []NucleiScanJob
	LogProgressOffset   int // 日志进度偏移量，用于分批扫描时显示全局进度
	LogProgressTotal    int // 日志进度总数，用于分批扫描时显示全局进度
}

type NucleiScanJob struct {
	URL      string   `json:"url"`
	Tags     []string `json:"tags,omitempty"`     // 指纹识别到的标签（用于 include tags）
	Template []string `json:"template,omitempty"` // 手动指定模板/目录（优先级可定义）
}

type InteractshOptions struct {
	OOBEnabled      bool   // 是否启用 OOB（显式配置时生效）
	InteractshURL   string // 自定义 Interactsh 服务 URL
	InteractshToken string // 自定义 Interactsh Token（可选）
}

// Tag 资产标签
type Tag struct {
	ProductName string // CNAME对应的产品名称
	AssetType   string // 标签类型 WAF/CDN
	Source      string // 来源
}

// VulnFingerprint 漏洞指纹结构（用于指纹识别到的漏洞）
type VulnFingerprint struct {
	Name        string // 指纹名称（漏洞名称）
	Description string // 漏洞描述
	MatchedRule string // 匹配的规则
}

// InfoResult 指纹识别结果
type InfoResult struct {
	TaskId               string // 任务ID
	URL                  string // 网站链接
	Scheme               string // 协议
	Host                 string // 域名 或者 IP
	Port                 int
	StatusCode           int
	Length               int
	Title                string
	Fingerprints         []string          // 普通指纹列表
	HighRiskFingerprints []string          // 高危指纹列表
	VulnFingerprints     []VulnFingerprint // ⚠️ 漏洞指纹列表（需要保存到漏洞表）
	AssetTags            Tag               // 资产标签列表
	Detect               string
	Screenshot           string // 截图图片路径
	Favicon              string // favicon图片路径
}

// WebReport Web扫描报告
type WebReport struct {
	Targets      string
	Fingerprints []InfoResult
	POCs         []VulnerabilityInfo
}

// WebscanOptions Web扫描选项
type WebscanOptions struct {
	Target                []string            // 扫描目标
	TcpTarget             map[string][]string // tcp层的目标，兼容nuclei可以扫描
	Thread                int                 // 指纹扫描线程数
	NucleiThreads         int                 // Nuclei 漏洞扫描线程数
	Timeout               float64             // 超时时间
	Headers               []string            // 自定义请求头
	Template              []string            // 模板文件
	TemplateFiles         []string            // 模板文件列表
	NoPoc                 bool                // 不扫描漏洞
	SkipNucleiWithoutTags bool                // 如果没有指纹则跳过Nuclei扫描
	SkipNucleiWithWAF     bool                // 如果识别到WAF则跳过Nuclei扫描
	Debug                 bool                // 调试模式
	DeepScan              bool                // 深度扫描（主动指纹）
	RootPath              bool                // 主动指纹是否使用根路径
	Proxy                 string              // 代理地址
	ActiveTimeoutLimit    int                 // 主动指纹超时限制
	DisableMaxHostErr     bool                // 不限制主机最大失败数
	Screenshot            bool                // 是否截图
	CallNuclei            bool                // 是否调用Nuclei扫描
	Tags                  []string            // 自定义标签
	AppendTemplateFolder  string              // 追加模板文件夹
	NetworkCard           string              // 指定扫描网卡
	CustomHeaders         string              // 自定义请求头（字符串格式）
	WebscanOption         int                 // 网站扫描模式
	// OOB / Interactsh
	OOBEnabled      bool   // 是否启用 OOB（显式配置时生效）
	InteractshURL   string // 自定义 Interactsh 服务 URL
	InteractshToken string // 自定义 Interactsh Token（可选）
}
