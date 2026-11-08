export type ScheduleKind = 'once' | 'daily' | 'weekdays' | 'weekly' | 'monthly' | 'cron'

// Matches scantask.ScheduleSpec; the backend owns all calendar math.
export interface ScheduleSpec {
	kind: ScheduleKind
	times?: string[]
	date?: string
	weekdays?: number[]
	dayOfMonth?: number
	cron?: string[]
}

// A nil spec means manual triggering. enabled=false keeps the spec paused.
export interface ScheduleConfig {
	enabled: boolean
	spec: ScheduleSpec | null
}

// 网站扫描相关类型定义

// 扫描阶段事件（来自 checkpoint meta.stageEvents，悬停进度条展示扫描时间线）
export interface StageEvent {
	time: string
	stage: string
	label: string
}

export interface WebScanTask {
	id: string
	taskName?: string // 任务名称（存储在数据库的单独列中）
	createdByAI?: boolean
	scanType: ScanType // 扫描类型
	scanMode?: 'auto' | 'local' | 'agent'
	scheduleEnabled?: boolean | null
	scheduleSpec?: ScheduleSpec | null
	nextRunAt?: string
	status: 'pending' | 'running' | 'paused' | 'recovering' | 'completed' | 'failed' | 'cancelled' | 'deleting'
	progress: number
	totalCount: number
	currentCount: number
	resultCount: number
	errorCount: number
	version?: number // 扫描版本号（第几次扫描）
	// `config` 现在为针对不同扫描类型的联合配置（详见下面的具体配置类型）
	config: SubdomainScanConfig | WebScanConfig | PortScanConfig | CrackScanConfig | DirScanConfig | JsScanConfig
	errorMsg?: string
	currentStage?: string
	// 扫描阶段事件时间线（悬停进度条展示：主机存活探测 → 指纹扫描 → 漏洞扫描）
	stageEvents?: StageEvent[]
	checkpointJson?: string
	pauseRequested?: boolean
	heartbeatAt?: string
	startedAt?: string
	completedAt?: string
	createdAt: string
	updatedAt: string
	// 资产信息统计
	fingerprintCount?: number // 指纹数量
	portCount?: number // 端口数量
	subdomainCount?: number // 子域名数量
	// 漏洞统计
	vulnCount?: number // 总漏洞数
	criticalCount?: number // 严重漏洞数
	highCount?: number // 高危漏洞数
	mediumCount?: number // 中危漏洞数
	lowCount?: number // 低危漏洞数
	infoCount?: number // 信息漏洞数
}

export type WebScanTaskSummary = Omit<WebScanTask, 'config'>

export interface TaskScheduleLog {
	id: number
	taskId: string
	version: number
	scanType: string
	scheduleMode: string
	scheduleCron?: string
	scheduledAt?: string
	triggeredAt: string
	status: 'success' | 'failed' | 'dispatching' | 'skipped' | 'expired'
	executionVersion?: number
	errorMsg?: string
	nextRunAt?: string
	createdAt: string
}

export enum ScanType {
	Subdomain = 'subdomain',
	WebScan = 'webscan',
	PortScan = 'portscan',
	Crack = 'crack',
	DirScan = 'dirscan',
	JsScan = 'jsscan',
	Combination = 'combination',
}

export interface SubdomainScanConfig {
	targets: string[]
	thread: number
	timeout: number
	appendEngines: string[]
	dnsServers?: string[]
}

// 独立的各类扫描配置
export interface WebScanConfig {
	targets: string[]
	threads: number
	nucleiThreads?: number
	proxy?: string // 代理地址
	// 网站扫描特有配置
	webscanOption: 1 | 2 | 3 | 4 // 1: 指纹扫描, 2: 全指纹扫描, 3: 指纹漏洞扫描, 4: 专项扫描
	customHeaders: string // 自定义请求头
	customTags: string[] // 指定指纹
	customTemplate: string[] // 指定漏洞
	rootPathScan: boolean // 根路径扫描
	skipNucleiWithoutTags: boolean // 无指纹目标跳过漏扫
	skipNucleiWithWAF: boolean // 识别到 WAF 时跳过漏扫
	screenshot: boolean // 网站截图
	activeTimeoutLimit: number // 主动指纹最多失败次数
	disableMaxHostErr?: boolean // 不限制主机最大失败数
	oobEnabled: boolean // 是否启用 OOB
	interactshURL?: string // 自定义 Interactsh URL
	interactshUrl?: string // 兼容字段：自定义 Interactsh URL
	interactshToken?: string // 自定义 Interactsh Token（可选）
}

export interface CrackScanConfig {
	targets: string[]
	threads: number
	proxy?: string
	// 密码暴破特有配置
	username: string // 用户字典
	password: string // 密码字典
	defaultDict: boolean // 使用默认字典
}


export interface DirScanConfig {
	targets: string[]
	// HTTP 方法（若目录探测需要使用特定方法）
	method: 'GET' | 'POST' | 'HEAD' | 'OPTIONS'
	// 字典路径列表（可以指向仓库内的字典文件路径）
	dictPaths?: string[]
	// 自定义路径列表（直接提供路径字符串）
	paths?: string[]
	// 扩展名白名单，例如 ["php","aspx","asp","jsp","html","js"]
	exts: string[]
	// 并发工作数（与旧参数 workers/threads 对应）
	workers: number
	// 网络/扫描控制
	timeout: number
	bodyExclude?: string
	bodyLengthExcludeTimes?: number
	statusCodeExclude: number[]
	redirect: boolean
	interval?: number
	customHeader?: string
	// 是否启用备份扫描（尝试常见备份文件名等）
	backupscan: boolean
	proxy?: string
	// 目录扫描可扩展选项（例如字典、递归深度等）
	wordlist?: string
	// 递归深度：0/1/... 或者布尔值表示是否递归
	recursive?: number | boolean
}

export interface JsScanConfig {
	targets: string[]
	proxy?: string
	blackDomain: string[]
	highRiskRouter: string[]
	authentication: string[]
	placeholder: Record<string, string>
	vulnDetection: {
		enabled: boolean
	}
	// AI 检测配置：apiKey/baseURL/model 为可选，启用时后端从系统配置获取
	openai: { enabled: boolean; apiKey?: string; baseURL?: string; model?: string }
}



// SQL注入规则类型
interface SQLRule {
	payloads: string[]
	type: 'error-based'
	bodyContains: string[]
}

// LFI规则类型
interface LFIRule {
	payloads: string[]
	matchType: 'regex' | 'word'
	regex?: string[]
	words?: string[]
	condition?: 'and' | 'or'
}

// XSS规则类型
interface XSSRule {
	payloads: string[]
	statusEquals?: number
}

export interface AssetTag {
	productName: string // CNAME对应的产品名称
	assetType: string // 标签类型 WAF/CDN
	source: string // 来源
}

export interface FingerprintInfo {
	id: string
	taskId: string
	version?: number
	url: string
	status: number
	verificationStatus?: 'pending' | 'verified' | 'false_positive' | 'ignored'
	length: number
	title: string
	detect: string
	fingerprints: string // 普通指纹列表（JSON数组字符串）
	highRiskFingerprints?: string // 高危指纹列表（JSON数组字符串）
	assetTags?: string // 资产标签列表（JSON数组字符串）
	screenshot?: string
	favicon?: string // favicon图片路径
	iconHash?: string // FOFA 兼容的 favicon MMH3 哈希
	host: string
	scheme: string
	port: number
	createdAt: string
}

export interface VulnerabilityInfo {
	id: string
	taskId: string
	version?: number
	templateId: string
	vulnName: string
	protocol: string
	severity: 'critical' | 'high' | 'medium' | 'low' | 'info'
	disposalStatus?: 'open' | 'in_progress' | 'fixed' | 'false_positive' | 'ignored'
	vulnUrl: string
	source?: string
	extract?: string
	request?: string
	response?: string
	description?: string
	impact?: string
	remediation?: string
	reference?: string
	responseTime?: string
	principDetection?: boolean
	extraInfo?: string
}

export interface Activity {
	content: string
	timestamp: string
	type: 'success' | 'warning' | 'danger' | 'info'
	icon?: string | null
}

export interface PortInfo {
	id: string
	taskId: string
	host: string
	assets?: PortScanAsset[]
	subdomains?: string[]
	port: number
	transport?: string
	protocol: string
	discoveryStatus?: 'pending' | 'verified'
	product?: string
	service?: string
	version?: string
	state: 'open' | 'closed' | 'filtered'
	banner?: string
	rawResponse?: string
	createdAt: string
}

export interface PortScanAsset {
	domain?: string
	subdomain: string
	assetType?: string
	assetName?: string
	sourceModule?: string
}

export interface PortScanTask {
	id: string
	taskName?: string
	status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
	progress: number
	version: number
	config: PortScanConfig
	portCount?: number
	errorMsg?: string
	startedAt?: string
	completedAt?: string
	createdAt: string
	updatedAt: string
}

export interface SubdomainScanTask {
	id: string
	taskName?: string
	status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
	progress: number
	totalCount: number
	currentCount: number
	resultCount: number
	progressDetail?: string
	config: string
	errorMsg?: string
	startedAt?: string
	completedAt?: string
	createdAt: string
	updatedAt: string
}

export interface PortScanConfig {
	targets: string[]
	threads?: number
	timeout?: number
	portRange?: string
	proxy?: string
	aliveDetection?: boolean // 是否启用存活探测
	subnetDiscoveryEnabled?: boolean
	subnetDiscoveryMode?: 'fast' | 'full'
	subnetDiscoveryHostSuffixes?: string
	subnetDiscoveryRandomSamples?: number
	tcpHeuristicEnabled?: boolean
	tcpHeuristicPorts?: string
	tcpHeuristicTimeoutMs?: number
	tcpDiscoveryMode?: TCPDiscoveryMode
	portDiscoveryMode?: TCPDiscoveryMode
	synRate?: number
	portDiscoveryRate?: number
	discoveryRate?: number
}

export type TCPDiscoveryMode = 'syn' | 'connect'

export interface SYNCapabilityStatus {
	platform: string
	runtimeMode: string
	supported: boolean
	authorized: boolean
	authorizationRequired: boolean
	canFallbackToConnect: boolean
	currentProcessPrivileged: boolean
	helperInstalled: boolean
	helperRunning: boolean
	helperVersion?: string
	status: 'authorized' | 'needs_authorization' | 'unsupported' | string
	message: string
}

export interface PortScanResult {
	host: string
	assets?: PortScanAsset[]
	subdomains?: string[]
	port: number
	transport?: string
	protocol: string
	product?: string
	version?: string
	rawResponse: string
	banner: string
	createdAt: string
}

export type DictionaryMeta = {
	id: string
	name: string
	path: string
	description: string
	size?: number
	entries?: string[]
}
