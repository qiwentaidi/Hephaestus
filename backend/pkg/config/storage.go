package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
	"hephaestus-backend/pkg/nanoid"
	"hephaestus-backend/pkg/toolguard"
)

// 延迟导入 database 包，避免循环依赖
// 这些函数在数据库初始化后才可用
var (
	getSettingFunc          func(string) (interface{}, error)
	setSettingFunc          func(string, interface{}) error
	settingKeySpaceEngines  string
	settingKeyDingTalk      string
	dingTalkConfigMu        sync.RWMutex
	dingTalkConfigCache     *DingTalkConfig // 内存缓存
	settingKeyWeChat        string
	weChatConfigMu          sync.RWMutex
	weChatConfigCache       *WeChatConfig // 内存缓存
	settingKeyFeishu        string
	feishuConfigMu          sync.RWMutex
	feishuConfigCache       *FeishuConfig // 内存缓存
	settingKeyNotifications string
	notificationConfigMu    sync.RWMutex
	notificationConfigCache *NotificationConfig
	settingKeyAI            string
	aiConfigMu              sync.RWMutex
	settingKeyToolGuard     string
	toolGuardConfigMu       sync.RWMutex
)

var (
	mu          sync.RWMutex
	runtimeMu   sync.RWMutex
	runtimeMode = RuntimeModeServer
)

const (
	RuntimeModeServer          = "server"
	RuntimeModeDesktop         = "desktop"
	RuntimeModeAgent           = "agent"
	serverConfigFile           = "config.yaml"
	desktopConfigFile          = "config-desktop.yaml"
	defaultAgentID             = "scan-agent"
	defaultAgentName           = "Scan Agent"
	defaultZoomeyeAPIAddr      = "https://api.zoomeye.ai/"
	defaultShodanAPIAddr       = "https://api.shodan.io"
	AIProtocolOpenAICompatible = "openai-compatible"
	AIProtocolAnthropic        = "anthropic"
	AIRouteWorkbench           = "workbench"
	AIRouteJSScan              = "jsscan"
	AIRouteRepeaterPolish      = "repeaterPolish"
	AIRouteSubdomainAssist     = "subdomainAssist"
	defaultAIProviderID        = "dashscope-default"
	defaultAIProviderName      = "DashScope Compatible"
	defaultAIAPIAddr           = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	defaultAIModel             = "qwen-plus"
	defaultAnthropicAPIAddr    = "https://api.anthropic.com/v1"
)

// getConfigDir 获取配置目录（使用配置基础目录）
func getConfigDir() string {
	return GetConfigBaseDir()
}

func isAgentProcess() bool {
	return GetRuntimeMode() == RuntimeModeAgent
}

func SetRuntimeMode(mode string) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()

	switch mode {
	case RuntimeModeDesktop:
		runtimeMode = RuntimeModeDesktop
	case RuntimeModeAgent:
		runtimeMode = RuntimeModeAgent
	default:
		runtimeMode = RuntimeModeServer
	}
}

func GetRuntimeMode() string {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	return runtimeMode
}

func AllowedDatabaseTypes() []string {
	if GetRuntimeMode() == RuntimeModeAgent {
		return []string{DatabaseTypePostgres}
	}
	if GetRuntimeMode() == RuntimeModeDesktop {
		return []string{DatabaseTypePostgres, DatabaseTypeSQLite}
	}
	return []string{DatabaseTypePostgres, DatabaseTypeSQLite}
}

func IsDatabaseTypeAllowed(dbType string) bool {
	normalized := dbType
	if normalized == "" {
		normalized = DatabaseTypePostgres
	}
	for _, allowed := range AllowedDatabaseTypes() {
		if allowed == normalized {
			return true
		}
	}
	return false
}

// Config 系统配置结构
type Config struct {
	Web          Web                     `yaml:"web"`
	Database     DatabaseConfig          `yaml:"database"`
	Agent        AgentConfig             `yaml:"agent,omitempty"`
	SpaceEngines SpaceEngineConfig       `yaml:"spaceEngines,omitempty"` // 现在存储在数据库中
	Scan         ScanConfig              `yaml:"scan,omitempty"`         // 可选配置，可在代码中使用默认值
	Proxy        ProxyConfig             `yaml:"proxy,omitempty"`        // 可选配置，可在代码中使用默认值
	Plugins      map[string]PluginConfig `yaml:"plugins,omitempty"`      // 现在存储在数据库中
	DingTalk     DingTalkConfig          `yaml:"dingtalk,omitempty"`     // 钉钉 Webhook 配置（存储在数据库中）
	WeChat       WeChatConfig            `yaml:"wechat,omitempty"`       // 企业微信 Webhook 配置（存储在数据库中）
	Feishu       FeishuConfig            `yaml:"feishu,omitempty"`       // 飞书 Webhook 配置（存储在数据库中）
}

// SpaceEngineConfig 空间引擎配置
type SpaceEngineConfig struct {
	FOFA           FOFAConfig           `yaml:"fofa" json:"fofa"`
	Hunter         HunterConfig         `yaml:"hunter" json:"hunter"`
	Quake          QuakeConfig          `yaml:"quake" json:"quake"`
	Censys         CensysConfig         `yaml:"censys" json:"censys"`
	Zoomeye        ZoomeyeConfig        `yaml:"zoomeye" json:"zoomeye"`
	Shodan         ShodanConfig         `yaml:"shodan" json:"shodan"`
	Github         GithubConfig         `yaml:"github" json:"github"`
	Chaos          ChaosConfig          `yaml:"chaos" json:"chaos"`
	Bevigil        BevigilConfig        `yaml:"bevigil" json:"bevigil"`
	Securitytrails SecuritytrailsConfig `yaml:"securitytrails" json:"securitytrails"`
	CTRadar        CTRadarConfig        `yaml:"ctRadar" json:"ctRadar"`
}

type AgentConfig struct {
	Roles           []string          `yaml:"roles,omitempty"`
	AIConcurrency   int               `yaml:"aiConcurrency,omitempty"`
	ScanConcurrency int               `yaml:"scanConcurrency,omitempty"`
	ID              string            `yaml:"id"`
	DisplayName     string            `yaml:"displayName,omitempty"`
	Server          AgentServerConfig `yaml:"server,omitempty"`
}

type AgentServerConfig struct {
	Address string `yaml:"address"`
}

// ResolveAgentIdentity returns a persistent unique identity for the agent.
func ResolveAgentIdentity(id, displayName string) (string, string) {
	resolvedID := strings.TrimSpace(id)
	resolvedName := strings.TrimSpace(displayName)

	if resolvedID == "" || strings.EqualFold(resolvedID, defaultAgentID) {
		resolvedID = nanoid.Generate()
	}
	if resolvedName == "" || strings.EqualFold(resolvedName, defaultAgentName) {
		resolvedName = defaultAgentName
	}

	return resolvedID, resolvedName
}

// FOFAConfig FOFA配置
type FOFAConfig struct {
	API   string `yaml:"api" json:"api"`
	Email string `yaml:"email" json:"email"`
	Key   string `yaml:"key" json:"key"`
}

// HunterConfig 鹰图配置
type HunterConfig struct {
	API string `yaml:"api" json:"api"`
	Key string `yaml:"key" json:"key"`
}

// QuakeConfig Quake配置
type QuakeConfig struct {
	Key string `yaml:"key" json:"key"`
}

// CensysConfig Censys配置
type CensysConfig struct {
	Token          string `yaml:"token" json:"token"`
	OrganizationID string `yaml:"organizationId" json:"organizationId"`
}

// ZoomeyeConfig Zoomeye配置
type ZoomeyeConfig struct {
	API string `yaml:"api" json:"api"`
	Key string `yaml:"key" json:"key"`
}

// ShodanConfig Shodan配置
type ShodanConfig struct {
	API string `yaml:"api" json:"api"`
	Key string `yaml:"key" json:"key"`
}

// GithubConfig GitHub配置
type GithubConfig struct {
	Token string `yaml:"token" json:"token"`
}

// ChaosConfig Chaos配置
type ChaosConfig struct {
	Key string `yaml:"key" json:"key"`
}

// BevigilConfig Bevigil配置
type BevigilConfig struct {
	Key string `yaml:"key" json:"key"`
}

// SecuritytrailsConfig Securitytrails配置
type SecuritytrailsConfig struct {
	Key string `yaml:"key" json:"key"`
}

// CTRadarConfig ct-radar 配置
type CTRadarConfig struct {
	Key string `yaml:"key" json:"key"`
}

// AIConfig AI配置
type AIConfig struct {
	RequestTimeoutMinutes   int                `yaml:"requestTimeoutMinutes,omitempty" json:"requestTimeoutMinutes,omitempty"`
	Enabled                 bool               `yaml:"enabled" json:"enabled"`
	Protocol                string             `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	API                     string             `yaml:"api" json:"api"`
	Key                     string             `yaml:"key" json:"key"`
	Model                   string             `yaml:"model" json:"model"`
	DefaultProviderID       string             `yaml:"defaultProviderId,omitempty" json:"defaultProviderId,omitempty"`
	Providers               []AIProviderConfig `yaml:"providers,omitempty" json:"providers,omitempty"`
	Routing                 AIRoutingConfig    `yaml:"routing,omitempty" json:"routing,omitempty"`
	WorkbenchRemoteDispatch bool               `yaml:"workbenchRemoteDispatch" json:"workbenchRemoteDispatch"`
}

// AIProviderConfig AI供应商配置
type AIProviderConfig struct {
	// RequestTimeoutMinutes limits one model request, including the response body.
	// Zero/missing uses 30 minutes for compatibility with existing channels.
	RequestTimeoutMinutes      int    `yaml:"requestTimeoutMinutes,omitempty" json:"requestTimeoutMinutes,omitempty"`
	WorkbenchRequestRateUnit   string `yaml:"workbenchRequestRateUnit,omitempty" json:"workbenchRequestRateUnit,omitempty"`
	WorkbenchMaxConcurrent     int    `yaml:"workbenchMaxConcurrent,omitempty" json:"workbenchMaxConcurrent,omitempty"`
	WorkbenchRequestsPerMinute int    `yaml:"workbenchRequestsPerMinute,omitempty" json:"workbenchRequestsPerMinute,omitempty"`
	// MaxTotalTokens 单次模型调用的上下文窗口软预算（粗略估算，超出时按回合裁剪最早的历史消息）；
	// 0 表示不限制。
	MaxTotalTokens int `yaml:"maxTotalTokens,omitempty" json:"maxTotalTokens,omitempty"`
	// MaxOutputTokens 单次模型回复的输出上限（max_tokens）；
	// 0 使用运行时默认 4096。
	MaxOutputTokens   int    `yaml:"maxOutputTokens,omitempty" json:"maxOutputTokens,omitempty"`
	ID                string `yaml:"id" json:"id"`
	Name              string `yaml:"name" json:"name"`
	Protocol          string `yaml:"protocol" json:"protocol"`
	Enabled           bool   `yaml:"enabled" json:"enabled"`
	API               string `yaml:"api" json:"api"`
	Key               string `yaml:"key" json:"key"`
	Model             string `yaml:"model" json:"model"`
	SupportsTools     bool   `yaml:"supportsTools" json:"supportsTools"`
	SupportsStreaming bool   `yaml:"supportsStreaming" json:"supportsStreaming"`
}

// AIRoutingConfig 按功能选择AI供应商
type AIRoutingConfig struct {
	Workbench                string `yaml:"workbench,omitempty" json:"workbench,omitempty"`
	WorkbenchFallbackEnabled bool   `yaml:"workbenchFallbackEnabled,omitempty" json:"workbenchFallbackEnabled,omitempty"`
	WorkbenchFallback        string `yaml:"workbenchFallback,omitempty" json:"workbenchFallback,omitempty"`
	JSScan                   string `yaml:"jsscan,omitempty" json:"jsscan,omitempty"`
	RepeaterPolish           string `yaml:"repeaterPolish,omitempty" json:"repeaterPolish,omitempty"`
	SubdomainAssist          string `yaml:"subdomainAssist,omitempty" json:"subdomainAssist,omitempty"`
}

// DingTalkConfig 钉钉 Webhook 配置
type DingTalkConfig struct {
	WebhookURL           string `yaml:"webhookUrl" json:"webhookUrl"`                     // 钉钉 Webhook URL
	Secret               string `yaml:"secret" json:"secret"`                             // 签名密钥（如果启用了加签）
	Enabled              bool   `yaml:"enabled" json:"enabled"`                           // 是否启用通知
	NotifyOnVulnFound    bool   `yaml:"notifyOnVulnFound" json:"notifyOnVulnFound"`       // 发现漏洞时通知
	NotifyOnScanComplete bool   `yaml:"notifyOnScanComplete" json:"notifyOnScanComplete"` // 扫描完成时通知
}

// WeChatConfig 企业微信 Webhook 配置
type WeChatConfig struct {
	WebhookURL           string `yaml:"webhookUrl" json:"webhookUrl"`                     // 企业微信 Webhook URL
	Enabled              bool   `yaml:"enabled" json:"enabled"`                           // 是否启用通知
	NotifyOnVulnFound    bool   `yaml:"notifyOnVulnFound" json:"notifyOnVulnFound"`       // 发现漏洞时通知
	NotifyOnScanComplete bool   `yaml:"notifyOnScanComplete" json:"notifyOnScanComplete"` // 扫描完成时通知
}

// FeishuConfig 飞书 Webhook 配置
type FeishuConfig struct {
	WebhookURL           string `yaml:"webhookUrl" json:"webhookUrl"`                     // 飞书 Webhook URL
	Secret               string `yaml:"secret" json:"secret"`                             // 签名密钥（如果启用了加签）
	Enabled              bool   `yaml:"enabled" json:"enabled"`                           // 是否启用通知
	NotifyOnVulnFound    bool   `yaml:"notifyOnVulnFound" json:"notifyOnVulnFound"`       // 发现漏洞时通知
	NotifyOnScanComplete bool   `yaml:"notifyOnScanComplete" json:"notifyOnScanComplete"` // 扫描完成时通知
}

const (
	NotificationEventVulnerabilityFound = "vulnerability_found"
	NotificationEventScanComplete       = "scan_complete"

	NotificationChannelDingTalk = "dingtalk"
	NotificationChannelWeChat   = "wechat"
	NotificationChannelFeishu   = "feishu"
	NotificationChannelWebhook  = "webhook"
	NotificationChannelJira     = "jira"
	NotificationChannelEmail    = "email"
)

// NotificationChannelConfig describes one notification destination and the
// events it subscribes to. The provider-specific fields intentionally live in
// one model so dispatchers can fan out the same event to every channel.
type NotificationChannelConfig struct {
	ID           string   `yaml:"id" json:"id"`
	Type         string   `yaml:"type" json:"type"`
	Name         string   `yaml:"name" json:"name"`
	Enabled      bool     `yaml:"enabled" json:"enabled"`
	Events       []string `yaml:"events" json:"events"`
	WebhookURL   string   `yaml:"webhookUrl,omitempty" json:"webhookUrl,omitempty"`
	Secret       string   `yaml:"secret,omitempty" json:"secret,omitempty"`
	BaseURL      string   `yaml:"baseUrl,omitempty" json:"baseUrl,omitempty"`
	ProjectKey   string   `yaml:"projectKey,omitempty" json:"projectKey,omitempty"`
	Username     string   `yaml:"username,omitempty" json:"username,omitempty"`
	Token        string   `yaml:"token,omitempty" json:"token,omitempty"`
	IssueType    string   `yaml:"issueType,omitempty" json:"issueType,omitempty"`
	SMTPHost     string   `yaml:"smtpHost,omitempty" json:"smtpHost,omitempty"`
	SMTPPort     int      `yaml:"smtpPort,omitempty" json:"smtpPort,omitempty"`
	SMTPUsername string   `yaml:"smtpUsername,omitempty" json:"smtpUsername,omitempty"`
	SMTPPassword string   `yaml:"smtpPassword,omitempty" json:"smtpPassword,omitempty"`
	From         string   `yaml:"from,omitempty" json:"from,omitempty"`
	Recipients   string   `yaml:"recipients,omitempty" json:"recipients,omitempty"`
}

type NotificationConfig struct {
	Version  int                         `yaml:"version" json:"version"`
	Channels []NotificationChannelConfig `yaml:"channels" json:"channels"`
}

// ScanConfig 扫描配置
type ScanConfig struct {
	WebThread      int    `yaml:"webThread"`
	CrackThread    int    `yaml:"crackThread"`
	PortThread     int    `yaml:"portThread"`
	PortTimeout    int    `yaml:"portTimeout"`
	AliveModule    string `yaml:"aliveModule"`
	DefaultNetwork string `yaml:"defaultNetwork"`
}

// ProxyConfig 代理配置
type ProxyConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Mode     string `yaml:"mode"`
	Address  string `yaml:"address"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

const (
	DatabaseTypePostgres = "postgres"
	DatabaseTypeSQLite   = "sqlite"
)

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Type     string `yaml:"type" json:"type"`
	Path     string `yaml:"path" json:"path"`
	Host     string `yaml:"host" json:"host"`
	Port     string `yaml:"port" json:"port"`
	User     string `yaml:"user" json:"user"`
	Password string `yaml:"password" json:"password"`
	DBName   string `yaml:"dbname" json:"dbname"`
	SSLMode  string `yaml:"sslmode" json:"sslmode"`
}

// Web 配置
type Web struct {
	Port          int    `yaml:"port"` // 服务端口，默认 18181
	MCPListenHost string `yaml:"mcpListenHost,omitempty" json:"mcpListenHost,omitempty"`
	MCPListenPort string `yaml:"mcpListenPort,omitempty" json:"mcpListenPort,omitempty"`
}

// GetWebPort 获取 Web 服务端口
func GetWebPort() int {
	config, err := LoadConfig()
	if err != nil || config.Web.Port <= 0 {
		return 18181 // 默认端口
	}
	return config.Web.Port
}

// GetMCPListenConfig 获取 MCP 专用监听配置，未配置或读取失败时返回空字符串
func GetMCPListenConfig() (host, port string) {
	config, err := LoadConfig()
	if err != nil || config == nil {
		return "", ""
	}
	return strings.TrimSpace(config.Web.MCPListenHost), strings.TrimSpace(config.Web.MCPListenPort)
}

// SetMCPListenConfig 保存 MCP 专用监听配置
func SetMCPListenConfig(host, port string) error {
	config, err := LoadConfig()
	if err != nil {
		return err
	}
	config.Web.MCPListenHost = strings.TrimSpace(host)
	config.Web.MCPListenPort = strings.TrimSpace(port)
	return SaveConfig(config)
}

// PluginConfig 插件配置
type PluginConfig map[string]interface{}

// GetConfigPath 获取配置文件路径
func GetConfigPath() string {
	return GetConfigPathForMode(GetRuntimeMode())
}

func GetConfigPathForMode(mode string) string {
	return filepath.Join(getConfigDir(), getConfigFileNameForMode(mode))
}

// GetResolvedConfigPath 获取当前实际读取的配置文件路径。
// 若尚未命中任何可读配置，则回退到主配置写入路径。
func GetResolvedConfigPath() string {
	if path := getReadableConfigPath(); path != "" {
		return path
	}
	return GetConfigPath()
}

func getConfigLoadPaths() []string {
	paths := []string{GetConfigPath()}
	if wd, err := os.Getwd(); err == nil {
		cwdConfigPath := filepath.Join(wd, getPrimaryConfigFileName())
		if cwdConfigPath != paths[0] {
			paths = append(paths, cwdConfigPath)
		}
	}
	return paths
}

func getReadableConfigPath() string {
	for _, path := range getConfigLoadPaths() {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// LoadConfig 加载配置
func LoadConfig() (*Config, error) {
	mu.RLock()
	defer mu.RUnlock()

	configPath := getReadableConfigPath()

	// 如果文件不存在，生成模式对应的配置文件并返回默认配置
	if configPath == "" {
		configPath = GetConfigPath()
		if err := generateInitialConfigFile(configPath); err != nil {
			return GetDefaultConfig(), nil
		}
		return GetDefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if isAgentProcess() {
		resolvedID, _ := ResolveAgentIdentity(config.Agent.ID, "")
		legacyDisplayName := strings.TrimSpace(config.Agent.DisplayName)
		legacyServerAddress := strings.TrimSpace(config.Agent.Server.Address)
		if resolvedID != strings.TrimSpace(config.Agent.ID) || legacyDisplayName != "" || legacyServerAddress != "" {
			config.Agent.ID = resolvedID
			config.Agent.DisplayName = ""
			config.Agent.Server = AgentServerConfig{}
			if err := writeConfigFile(configPath, &config); err != nil {
				return nil, fmt.Errorf("迁移 Agent 标识失败: %w", err)
			}
		}
	}

	// 不再自动填充默认值，要求用户明确配置数据库信息

	return &config, nil
}

// SaveConfig 保存配置
func SaveConfig(config *Config) error {
	mu.Lock()
	defer mu.Unlock()

	// 确保目录存在
	configDir := getConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	configPath := GetConfigPath()
	return writeConfigFile(configPath, config)
}

// GetDefaultConfig 获取默认配置
func GetDefaultConfig() *Config {
	dbConfig := getRuntimeDefaultDatabaseConfig()
	agentID, _ := ResolveAgentIdentity(defaultAgentID, "")

	return &Config{
		Database: dbConfig,
		Agent: AgentConfig{
			ID: agentID,
		},
		SpaceEngines: SpaceEngineConfig{
			FOFA: FOFAConfig{
				API:   "https://fofa.info",
				Email: "",
				Key:   "",
			},
			Hunter: HunterConfig{
				API: "https://hunter.qianxin.com",
				Key: "",
			},
			Quake: QuakeConfig{
				Key: "",
			},
			Censys: CensysConfig{
				Token:          "",
				OrganizationID: "",
			},
			Zoomeye: ZoomeyeConfig{
				API: defaultZoomeyeAPIAddr,
				Key: "",
			},
			Shodan: ShodanConfig{
				API: defaultShodanAPIAddr,
				Key: "",
			},
			Github: GithubConfig{
				Token: "",
			},
			Chaos: ChaosConfig{
				Key: "",
			},
			Bevigil: BevigilConfig{
				Key: "",
			},
			Securitytrails: SecuritytrailsConfig{
				Key: "",
			},
			CTRadar: CTRadarConfig{
				Key: "",
			},
		},
		Scan: ScanConfig{
			WebThread:      50,
			CrackThread:    20,
			PortThread:     1000,
			PortTimeout:    3,
			AliveModule:    "icmp",
			DefaultNetwork: "auto",
		},
		Proxy: ProxyConfig{
			Enabled:  false,
			Mode:     "HTTP",
			Address:  "",
			Port:     8080,
			Username: "",
			Password: "",
		},
		DingTalk: DingTalkConfig{
			WebhookURL:           "",
			Secret:               "",
			Enabled:              false,
			NotifyOnVulnFound:    true,
			NotifyOnScanComplete: true,
		},
		Plugins: make(map[string]PluginConfig),
	}
}

func writeConfigFile(configPath string, config *Config) error {
	data, err := yaml.Marshal(prepareConfigForPersistence(config))
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	content := string(data)
	if isAgentConfigContent(config) {
		content = injectAgentIDComment(content)
	}

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

func isAgentConfigContent(config *Config) bool {
	if config == nil {
		return false
	}
	return strings.TrimSpace(config.Agent.ID) != ""
}

func prepareConfigForPersistence(config *Config) *Config {
	if config == nil {
		return nil
	}

	cloned := *config
	if isAgentProcess() {
		cloned.Agent.DisplayName = ""
		cloned.Agent.Server = AgentServerConfig{}
	}
	return &cloned
}

func injectAgentIDComment(content string) string {
	replaced := strings.Replace(
		content,
		"agent:\n    id:",
		"agent:\n    # agent.id 为节点唯一标识，首次生成后不可修改，否则会导致节点被识别为新的 Agent\n    id:",
		1,
	)
	if replaced != content {
		return replaced
	}
	return strings.Replace(
		content,
		"agent:\n  id:",
		"agent:\n  # agent.id 为节点唯一标识，首次生成后不可修改，否则会导致节点被识别为新的 Agent\n  id:",
		1,
	)
}

func getRuntimeDefaultDatabaseConfig() DatabaseConfig {
	if GetRuntimeMode() == RuntimeModeDesktop {
		return DatabaseConfig{
			Type: DatabaseTypeSQLite,
			Path: GetDefaultSQLitePath(),
		}
	}

	return DatabaseConfig{
		Type:    DatabaseTypePostgres,
		Host:    "127.0.0.1",
		Port:    "5432",
		User:    "postgres",
		DBName:  "hephaestus",
		SSLMode: "disable",
	}
}

// InitSpaceEngineConfigDB 初始化空间引擎配置的数据库操作（在数据库初始化后调用）
// 这个函数由 main.go 在数据库初始化后调用，避免循环依赖
func InitSpaceEngineConfigDB(
	getSetting func(string) (interface{}, error),
	setSetting func(string, interface{}) error,
	key string,
) {
	getSettingFunc = getSetting
	setSettingFunc = setSetting
	settingKeySpaceEngines = key
}

// InitDingTalkConfigDB 初始化钉钉配置的数据库操作（在数据库初始化后调用）
func InitDingTalkConfigDB(
	getSetting func(string) (interface{}, error),
	setSetting func(string, interface{}) error,
	key string,
) {
	getSettingFunc = getSetting
	setSettingFunc = setSetting
	settingKeyDingTalk = key
}

// InitWeChatConfigDB 初始化企业微信配置的数据库操作（在数据库初始化后调用）
func InitWeChatConfigDB(
	getSetting func(string) (interface{}, error),
	setSetting func(string, interface{}) error,
	key string,
) {
	getSettingFunc = getSetting
	setSettingFunc = setSetting
	settingKeyWeChat = key
}

// InitFeishuConfigDB 初始化飞书配置的数据库操作（在数据库初始化后调用）
func InitFeishuConfigDB(
	getSetting func(string) (interface{}, error),
	setSetting func(string, interface{}) error,
	key string,
) {
	getSettingFunc = getSetting
	setSettingFunc = setSetting
	settingKeyFeishu = key
}

// InitNotificationConfigDB initializes the unified notification configuration
// binding. It is separate from the legacy per-platform bindings so existing
// installations can be migrated lazily when the notification settings open.
func InitNotificationConfigDB(
	getSetting func(string) (interface{}, error),
	setSetting func(string, interface{}) error,
	key string,
) {
	getSettingFunc = getSetting
	setSettingFunc = setSetting
	settingKeyNotifications = key
}

// InitAIConfigDB 初始化AI配置的数据库操作（在数据库初始化后调用）
func InitAIConfigDB(
	getSetting func(string) (interface{}, error),
	setSetting func(string, interface{}) error,
	key string,
) {
	getSettingFunc = getSetting
	setSettingFunc = setSetting
	settingKeyAI = key
}

// InitToolGuardConfigDB 初始化调用拦截配置的数据库操作。
func InitToolGuardConfigDB(
	getSetting func(string) (interface{}, error),
	setSetting func(string, interface{}) error,
	key string,
) {
	getSettingFunc = getSetting
	setSettingFunc = setSetting
	settingKeyToolGuard = key
}

// GetSpaceEngineConfig 获取空间引擎配置（优先从数据库读取，如果数据库未初始化则从配置文件读取）
func GetSpaceEngineConfig() (SpaceEngineConfig, error) {
	// 如果数据库操作函数已初始化，优先从数据库读取
	if getSettingFunc != nil && settingKeySpaceEngines != "" {
		value, err := getSettingFunc(settingKeySpaceEngines)
		if err == nil && value != nil {
			// 将 JSON 值转换为 SpaceEngineConfig
			valueJSON, err := json.Marshal(value)
			if err == nil {
				var spaceConfig SpaceEngineConfig
				if err := json.Unmarshal(valueJSON, &spaceConfig); err == nil {
					return normalizeSpaceEngineConfig(spaceConfig), nil
				}
			}
		}
		// 如果从数据库读取失败（包括配置不存在的情况），回退到配置文件
	}

	// 从配置文件读取（向后兼容）
	config, err := LoadConfig()
	if err != nil {
		return SpaceEngineConfig{}, err
	}
	return normalizeSpaceEngineConfig(config.SpaceEngines), nil
}

// UpdateSpaceEngineConfig 更新空间引擎配置（优先保存到数据库，如果数据库未初始化则保存到配置文件）
func UpdateSpaceEngineConfig(spaceConfig SpaceEngineConfig) error {
	spaceConfig = normalizeSpaceEngineConfig(spaceConfig)

	// 如果数据库操作函数已初始化，优先保存到数据库
	if setSettingFunc != nil && settingKeySpaceEngines != "" {
		if err := setSettingFunc(settingKeySpaceEngines, spaceConfig); err == nil {
			return nil
		}
		// 如果保存到数据库失败，回退到配置文件
	}

	// 保存到配置文件（向后兼容）
	config, err := LoadConfig()
	if err != nil {
		return err
	}
	config.SpaceEngines = spaceConfig
	return SaveConfig(config)
}

func normalizeSpaceEngineConfig(spaceConfig SpaceEngineConfig) SpaceEngineConfig {
	if strings.TrimSpace(spaceConfig.FOFA.API) == "" {
		spaceConfig.FOFA.API = "https://fofa.info"
	}
	if strings.TrimSpace(spaceConfig.Hunter.API) == "" {
		spaceConfig.Hunter.API = "https://hunter.qianxin.com"
	}
	if strings.TrimSpace(spaceConfig.Zoomeye.API) == "" {
		spaceConfig.Zoomeye.API = defaultZoomeyeAPIAddr
	}
	if strings.TrimSpace(spaceConfig.Shodan.API) == "" {
		spaceConfig.Shodan.API = defaultShodanAPIAddr
	}
	return spaceConfig
}

// GetDingTalkConfig 获取钉钉配置（仅从数据库读取）
func GetDingTalkConfig() (DingTalkConfig, error) {
	dingTalkConfigMu.RLock()
	// 如果缓存存在，直接返回
	if dingTalkConfigCache != nil {
		defer dingTalkConfigMu.RUnlock()
		return *dingTalkConfigCache, nil
	}
	dingTalkConfigMu.RUnlock()

	// 如果缓存不存在，加载配置并缓存
	dingTalkConfigMu.Lock()
	defer dingTalkConfigMu.Unlock()

	// 双重检查，防止多个 goroutine 同时加载
	if dingTalkConfigCache != nil {
		return *dingTalkConfigCache, nil
	}

	defaultConfig := DingTalkConfig{
		WebhookURL:           "",
		Secret:               "",
		Enabled:              false,
		NotifyOnVulnFound:    true,
		NotifyOnScanComplete: true,
	}

	if getSettingFunc == nil || settingKeyDingTalk == "" {
		dingTalkConfigCache = &defaultConfig
		return defaultConfig, nil
	}

	value, err := getSettingFunc(settingKeyDingTalk)
	if err != nil {
		return DingTalkConfig{}, err
	}
	if value == nil {
		dingTalkConfigCache = &defaultConfig
		return defaultConfig, nil
	}

	valueJSON, err := json.Marshal(value)
	if err != nil {
		return DingTalkConfig{}, err
	}

	var config DingTalkConfig
	if err := json.Unmarshal(valueJSON, &config); err != nil {
		return DingTalkConfig{}, err
	}

	dingTalkConfigCache = &config
	return config, nil
}

// UpdateDingTalkConfig 更新钉钉配置（仅保存到数据库）
func UpdateDingTalkConfig(dingTalkConfig DingTalkConfig) error {
	dingTalkConfigMu.Lock()
	defer dingTalkConfigMu.Unlock()

	// 清除缓存
	dingTalkConfigCache = nil

	if setSettingFunc == nil || settingKeyDingTalk == "" {
		return fmt.Errorf("钉钉配置数据库未初始化")
	}

	return setSettingFunc(settingKeyDingTalk, dingTalkConfig)
}

// GetWeChatConfig 获取企业微信配置（仅从数据库读取）
func GetWeChatConfig() (WeChatConfig, error) {
	weChatConfigMu.RLock()
	// 如果缓存存在，直接返回
	if weChatConfigCache != nil {
		defer weChatConfigMu.RUnlock()
		return *weChatConfigCache, nil
	}
	weChatConfigMu.RUnlock()

	// 如果缓存不存在，加载配置并缓存
	weChatConfigMu.Lock()
	defer weChatConfigMu.Unlock()

	// 双重检查
	if weChatConfigCache != nil {
		return *weChatConfigCache, nil
	}

	defaultConfig := WeChatConfig{
		WebhookURL:           "",
		Enabled:              false,
		NotifyOnVulnFound:    true,
		NotifyOnScanComplete: true,
	}

	if getSettingFunc == nil || settingKeyWeChat == "" {
		weChatConfigCache = &defaultConfig
		return defaultConfig, nil
	}

	value, err := getSettingFunc(settingKeyWeChat)
	if err != nil {
		return WeChatConfig{}, err
	}
	if value == nil {
		weChatConfigCache = &defaultConfig
		return defaultConfig, nil
	}

	valueJSON, err := json.Marshal(value)
	if err != nil {
		return WeChatConfig{}, err
	}

	var config WeChatConfig
	if err := json.Unmarshal(valueJSON, &config); err != nil {
		return WeChatConfig{}, err
	}

	weChatConfigCache = &config
	return config, nil
}

// UpdateWeChatConfig 更新企业微信配置（仅保存到数据库）
func UpdateWeChatConfig(weChatConfig WeChatConfig) error {
	weChatConfigMu.Lock()
	defer weChatConfigMu.Unlock()

	// 清除缓存
	weChatConfigCache = nil

	if setSettingFunc == nil || settingKeyWeChat == "" {
		return fmt.Errorf("企业微信配置数据库未初始化")
	}

	return setSettingFunc(settingKeyWeChat, weChatConfig)
}

// GetFeishuConfig 获取飞书配置（仅从数据库读取）
func GetFeishuConfig() (FeishuConfig, error) {
	feishuConfigMu.RLock()
	// 如果缓存存在，直接返回
	if feishuConfigCache != nil {
		defer feishuConfigMu.RUnlock()
		return *feishuConfigCache, nil
	}
	feishuConfigMu.RUnlock()

	// 如果缓存不存在，加载配置并缓存
	feishuConfigMu.Lock()
	defer feishuConfigMu.Unlock()

	// 双重检查
	if feishuConfigCache != nil {
		return *feishuConfigCache, nil
	}

	defaultConfig := FeishuConfig{
		WebhookURL:           "",
		Secret:               "",
		Enabled:              false,
		NotifyOnVulnFound:    true,
		NotifyOnScanComplete: true,
	}

	if getSettingFunc == nil || settingKeyFeishu == "" {
		feishuConfigCache = &defaultConfig
		return defaultConfig, nil
	}

	value, err := getSettingFunc(settingKeyFeishu)
	if err != nil {
		return FeishuConfig{}, err
	}
	if value == nil {
		feishuConfigCache = &defaultConfig
		return defaultConfig, nil
	}

	valueJSON, err := json.Marshal(value)
	if err != nil {
		return FeishuConfig{}, err
	}

	var config FeishuConfig
	if err := json.Unmarshal(valueJSON, &config); err != nil {
		return FeishuConfig{}, err
	}

	feishuConfigCache = &config
	return config, nil
}

// UpdateFeishuConfig 更新飞书配置（仅保存到数据库）
func UpdateFeishuConfig(feishuConfig FeishuConfig) error {
	feishuConfigMu.Lock()
	defer feishuConfigMu.Unlock()

	// 清除缓存
	feishuConfigCache = nil

	if setSettingFunc == nil || settingKeyFeishu == "" {
		return fmt.Errorf("飞书配置数据库未初始化")
	}

	return setSettingFunc(settingKeyFeishu, feishuConfig)
}

func defaultNotificationEvents() []string {
	return []string{NotificationEventVulnerabilityFound, NotificationEventScanComplete}
}

func defaultNotificationConfig() NotificationConfig {
	return NotificationConfig{
		Version: 1,
		Channels: []NotificationChannelConfig{
			{ID: NotificationChannelDingTalk, Type: NotificationChannelDingTalk, Events: defaultNotificationEvents()},
			{ID: NotificationChannelWeChat, Type: NotificationChannelWeChat, Events: defaultNotificationEvents()},
			{ID: NotificationChannelFeishu, Type: NotificationChannelFeishu, Events: defaultNotificationEvents()},
			{ID: NotificationChannelWebhook, Type: NotificationChannelWebhook, Events: defaultNotificationEvents()},
			{ID: NotificationChannelJira, Type: NotificationChannelJira, Events: defaultNotificationEvents(), IssueType: "Task"},
			{ID: NotificationChannelEmail, Type: NotificationChannelEmail, Events: defaultNotificationEvents(), SMTPPort: 587},
		},
	}
}

func legacyNotificationConfig() (NotificationConfig, error) {
	dingTalk, err := GetDingTalkConfig()
	if err != nil {
		return NotificationConfig{}, err
	}
	wechat, err := GetWeChatConfig()
	if err != nil {
		return NotificationConfig{}, err
	}
	feishu, err := GetFeishuConfig()
	if err != nil {
		return NotificationConfig{}, err
	}
	result := defaultNotificationConfig()
	result.Channels[0].Enabled = dingTalk.Enabled
	result.Channels[0].WebhookURL = dingTalk.WebhookURL
	result.Channels[0].Secret = dingTalk.Secret
	result.Channels[0].Events = nil
	if dingTalk.NotifyOnVulnFound {
		result.Channels[0].Events = append(result.Channels[0].Events, NotificationEventVulnerabilityFound)
	}
	if dingTalk.NotifyOnScanComplete {
		result.Channels[0].Events = append(result.Channels[0].Events, NotificationEventScanComplete)
	}
	result.Channels[1].Enabled = wechat.Enabled
	result.Channels[1].WebhookURL = wechat.WebhookURL
	result.Channels[1].Events = nil
	if wechat.NotifyOnVulnFound {
		result.Channels[1].Events = append(result.Channels[1].Events, NotificationEventVulnerabilityFound)
	}
	if wechat.NotifyOnScanComplete {
		result.Channels[1].Events = append(result.Channels[1].Events, NotificationEventScanComplete)
	}
	result.Channels[2].Enabled = feishu.Enabled
	result.Channels[2].WebhookURL = feishu.WebhookURL
	result.Channels[2].Secret = feishu.Secret
	result.Channels[2].Events = nil
	if feishu.NotifyOnVulnFound {
		result.Channels[2].Events = append(result.Channels[2].Events, NotificationEventVulnerabilityFound)
	}
	if feishu.NotifyOnScanComplete {
		result.Channels[2].Events = append(result.Channels[2].Events, NotificationEventScanComplete)
	}
	return result, nil
}

// GetNotificationConfig returns the unified model. Existing installations
// without this setting are projected from the legacy three-platform settings.
func GetNotificationConfig() (NotificationConfig, error) {
	notificationConfigMu.RLock()
	if notificationConfigCache != nil {
		defer notificationConfigMu.RUnlock()
		return *notificationConfigCache, nil
	}
	notificationConfigMu.RUnlock()

	if getSettingFunc == nil || settingKeyNotifications == "" {
		return legacyNotificationConfig()
	}
	value, err := getSettingFunc(settingKeyNotifications)
	if err != nil {
		return NotificationConfig{}, err
	}
	if value == nil {
		return legacyNotificationConfig()
	}
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return NotificationConfig{}, err
	}
	var result NotificationConfig
	if err := json.Unmarshal(valueJSON, &result); err != nil {
		return NotificationConfig{}, err
	}
	if result.Version == 0 {
		result.Version = 1
	}
	notificationConfigMu.Lock()
	notificationConfigCache = &result
	notificationConfigMu.Unlock()
	return result, nil
}

func UpdateNotificationConfig(notificationConfig NotificationConfig) error {
	if notificationConfig.Version == 0 {
		notificationConfig.Version = 1
	}
	if setSettingFunc == nil || settingKeyNotifications == "" {
		return fmt.Errorf("统一通知配置数据库未初始化")
	}
	if err := setSettingFunc(settingKeyNotifications, notificationConfig); err != nil {
		return err
	}
	notificationConfigMu.Lock()
	notificationConfigCache = &notificationConfig
	notificationConfigMu.Unlock()
	return nil
}

// GetAIConfig 获取AI配置
func GetAIConfig() (AIConfig, error) {
	aiConfigMu.RLock()
	defer aiConfigMu.RUnlock()

	if getSettingFunc == nil {
		return AIConfig{}, fmt.Errorf("配置系统未初始化")
	}

	value, err := getSettingFunc(settingKeyAI)
	if err != nil {
		return AIConfig{}, fmt.Errorf("获取AI配置失败: %w", err)
	}

	if value == nil {
		// 返回默认配置
		return DefaultAIConfig(), nil
	}

	// 将 map 转换为 AIConfig
	var config AIConfig
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return AIConfig{}, fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, &config); err != nil {
		return AIConfig{}, fmt.Errorf("反序列化配置失败: %w", err)
	}

	return NormalizeAIConfig(config), nil
}

// UpdateAIConfig 更新AI配置
func UpdateAIConfig(aiConfig AIConfig) error {
	if err := ValidateAIRequestTimeouts(aiConfig); err != nil {
		return err
	}
	aiConfigMu.Lock()
	defer aiConfigMu.Unlock()

	if setSettingFunc == nil {
		return fmt.Errorf("配置系统未初始化")
	}

	if err := setSettingFunc(settingKeyAI, NormalizeAIConfig(aiConfig)); err != nil {
		return fmt.Errorf("保存AI配置失败: %w", err)
	}

	return nil
}

// GetToolGuardConfig 获取调用拦截配置，并一次性迁移可编辑的预置规则。
func GetToolGuardConfig() (toolguard.Config, error) {
	toolGuardConfigMu.Lock()
	defer toolGuardConfigMu.Unlock()

	if getSettingFunc == nil || settingKeyToolGuard == "" {
		return toolguard.Config{}, fmt.Errorf("配置系统未初始化")
	}
	value, err := getSettingFunc(settingKeyToolGuard)
	if err != nil {
		return toolguard.Config{}, fmt.Errorf("获取调用拦截配置失败: %w", err)
	}
	if value == nil {
		guardConfig := toolguard.DefaultConfig()
		if err := setSettingFunc(settingKeyToolGuard, guardConfig); err != nil {
			return toolguard.Config{}, fmt.Errorf("保存默认调用拦截配置失败: %w", err)
		}
		return guardConfig, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return toolguard.Config{}, fmt.Errorf("序列化调用拦截配置失败: %w", err)
	}
	var guardConfig toolguard.Config
	if err := json.Unmarshal(raw, &guardConfig); err != nil {
		return toolguard.Config{}, fmt.Errorf("反序列化调用拦截配置失败: %w", err)
	}
	guardConfigWithIDs := toolguard.EnsureRuleIDs(guardConfig)
	configChanged := !guardConfig.Enabled
	for i := range guardConfig.Rules {
		if guardConfig.Rules[i].ID != guardConfigWithIDs.Rules[i].ID {
			configChanged = true
			break
		}
	}
	guardConfig = guardConfigWithIDs
	if migrated, changed := toolguard.MigratePresetRules(guardConfig); changed {
		guardConfig = migrated
		configChanged = true
	}
	guardConfig.Enabled = true
	if _, err := toolguard.Compile(guardConfig); err != nil {
		return toolguard.Config{}, err
	}
	if configChanged {
		if err := setSettingFunc(settingKeyToolGuard, guardConfig); err != nil {
			return toolguard.Config{}, fmt.Errorf("迁移调用拦截配置失败: %w", err)
		}
	}
	return guardConfig, nil
}

// UpdateToolGuardConfig 保存已验证的调用拦截配置。
func UpdateToolGuardConfig(guardConfig toolguard.Config) error {
	toolGuardConfigMu.Lock()
	defer toolGuardConfigMu.Unlock()

	if setSettingFunc == nil || settingKeyToolGuard == "" {
		return fmt.Errorf("配置系统未初始化")
	}
	guardConfig.Enabled = true
	guardConfig.PresetVersion = toolguard.CurrentPresetVersion
	if _, err := toolguard.Compile(guardConfig); err != nil {
		return err
	}
	if err := setSettingFunc(settingKeyToolGuard, guardConfig); err != nil {
		return fmt.Errorf("保存调用拦截配置失败: %w", err)
	}
	return nil
}

func DefaultAIConfig() AIConfig {
	return NormalizeAIConfig(AIConfig{
		Enabled:                 false,
		Protocol:                AIProtocolOpenAICompatible,
		API:                     defaultAIAPIAddr,
		Key:                     "",
		Model:                   defaultAIModel,
		WorkbenchRemoteDispatch: false,
	})
}

func NormalizeAIConfig(aiConfig AIConfig) AIConfig {
	seen := make(map[string]bool, len(aiConfig.Providers))
	if len(aiConfig.Providers) == 0 {
		provider := AIProviderConfig{
			RequestTimeoutMinutes: aiConfig.RequestTimeoutMinutes,
			ID:                    defaultAIProviderID,
			Name:                  defaultAIProviderName,
			Protocol:              firstNonEmpty(aiConfig.Protocol, AIProtocolOpenAICompatible),
			Enabled:               aiConfig.Enabled,
			API:                   firstNonEmpty(aiConfig.API, defaultAIAPIAddr),
			Key:                   aiConfig.Key,
			Model:                 firstNonEmpty(aiConfig.Model, defaultAIModel),
			SupportsTools:         true,
			SupportsStreaming:     true,
		}
		aiConfig.Providers = []AIProviderConfig{normalizeAIProvider(provider, 0)}
		aiConfig.DefaultProviderID = aiConfig.Providers[0].ID
		seen[aiConfig.DefaultProviderID] = true
	} else {
		for idx := range aiConfig.Providers {
			provider := normalizeAIProvider(aiConfig.Providers[idx], idx)
			for seen[provider.ID] {
				provider.ID = fmt.Sprintf("%s-%d", provider.ID, idx+1)
			}
			seen[provider.ID] = true
			aiConfig.Providers[idx] = provider
		}
		if strings.TrimSpace(aiConfig.DefaultProviderID) == "" || !seen[aiConfig.DefaultProviderID] {
			aiConfig.DefaultProviderID = chooseDefaultAIProviderID(aiConfig.Providers)
		}
	}

	if strings.TrimSpace(aiConfig.DefaultProviderID) == "" && len(aiConfig.Providers) > 0 {
		aiConfig.DefaultProviderID = aiConfig.Providers[0].ID
	}
	aiConfig.Routing.Workbench = strings.TrimSpace(aiConfig.Routing.Workbench)
	aiConfig.Routing.JSScan = strings.TrimSpace(aiConfig.Routing.JSScan)
	aiConfig.Routing.RepeaterPolish = strings.TrimSpace(aiConfig.Routing.RepeaterPolish)
	aiConfig.Routing.SubdomainAssist = strings.TrimSpace(aiConfig.Routing.SubdomainAssist)
	aiConfig.Routing.WorkbenchFallback = strings.TrimSpace(aiConfig.Routing.WorkbenchFallback)
	workbenchProviderID := firstNonEmpty(aiConfig.Routing.Workbench, aiConfig.DefaultProviderID)
	if aiConfig.Routing.WorkbenchFallback == "" ||
		aiConfig.Routing.WorkbenchFallback == workbenchProviderID ||
		!seen[aiConfig.Routing.WorkbenchFallback] {
		aiConfig.Routing.WorkbenchFallback = ""
		aiConfig.Routing.WorkbenchFallbackEnabled = false
	}

	if provider, ok := aiConfig.DefaultProvider(); ok {
		aiConfig.RequestTimeoutMinutes = provider.RequestTimeoutMinutes
		aiConfig.Protocol = provider.Protocol
		aiConfig.API = provider.API
		aiConfig.Key = provider.Key
		aiConfig.Model = provider.Model
	} else {
		aiConfig.Protocol = AIProtocolOpenAICompatible
		aiConfig.API = defaultAIAPIAddr
		aiConfig.Model = defaultAIModel
	}

	return aiConfig
}

func (aiConfig AIConfig) DefaultProvider() (AIProviderConfig, bool) {
	for _, provider := range aiConfig.Providers {
		if provider.ID == aiConfig.DefaultProviderID {
			return provider, true
		}
	}
	if len(aiConfig.Providers) > 0 {
		return aiConfig.Providers[0], true
	}
	return AIProviderConfig{}, false
}

func ResolveAIRequestConfig(aiConfig AIConfig) AIConfig {
	aiConfig = NormalizeAIConfig(aiConfig)
	provider, ok := aiConfig.DefaultProvider()
	if !ok {
		return aiConfig
	}
	aiConfig.Enabled = aiConfig.Enabled && provider.Enabled
	aiConfig.RequestTimeoutMinutes = provider.RequestTimeoutMinutes
	aiConfig.Protocol = provider.Protocol
	aiConfig.API = provider.API
	aiConfig.Key = provider.Key
	aiConfig.Model = provider.Model
	return aiConfig
}

func ResolveAIRequestConfigForRoute(aiConfig AIConfig, route string) AIConfig {
	aiConfig = NormalizeAIConfig(aiConfig)
	providerID := strings.TrimSpace(aiConfig.DefaultProviderID)
	switch route {
	case AIRouteWorkbench:
		providerID = firstNonEmpty(aiConfig.Routing.Workbench, providerID)
	case AIRouteJSScan:
		providerID = firstNonEmpty(aiConfig.Routing.JSScan, providerID)
	case AIRouteRepeaterPolish:
		providerID = firstNonEmpty(aiConfig.Routing.RepeaterPolish, providerID)
	case AIRouteSubdomainAssist:
		providerID = firstNonEmpty(aiConfig.Routing.SubdomainAssist, providerID)
	}

	for _, provider := range aiConfig.Providers {
		if provider.ID != providerID {
			continue
		}
		aiConfig.DefaultProviderID = provider.ID
		aiConfig.Enabled = aiConfig.Enabled && provider.Enabled
		aiConfig.RequestTimeoutMinutes = provider.RequestTimeoutMinutes
		aiConfig.Protocol = provider.Protocol
		aiConfig.API = provider.API
		aiConfig.Key = provider.Key
		aiConfig.Model = provider.Model
		return aiConfig
	}

	return ResolveAIRequestConfig(aiConfig)
}

func normalizeAIProvider(provider AIProviderConfig, index int) AIProviderConfig {
	provider.RequestTimeoutMinutes = EffectiveAIRequestTimeoutMinutes(provider.RequestTimeoutMinutes)
	provider.WorkbenchRequestsPerMinute = max(0, provider.WorkbenchRequestsPerMinute)
	// RPM stays canonical for backwards compatibility; the unit controls display.
	if provider.WorkbenchRequestRateUnit != "second" && provider.WorkbenchRequestRateUnit != "minute" {
		provider.WorkbenchRequestRateUnit = "second"
		if provider.WorkbenchRequestsPerMinute > 0 {
			provider.WorkbenchRequestRateUnit = "minute"
		}
	}
	if provider.WorkbenchRequestRateUnit == "second" && provider.WorkbenchRequestsPerMinute%60 != 0 {
		provider.WorkbenchRequestRateUnit = "minute"
	}
	provider.ID = strings.TrimSpace(provider.ID)
	if provider.ID == "" {
		provider.ID = fmt.Sprintf("provider-%d", index+1)
	}
	provider.Name = strings.TrimSpace(provider.Name)
	if provider.Name == "" {
		provider.Name = fmt.Sprintf("AI Provider %d", index+1)
	}
	provider.Protocol = normalizeAIProtocol(provider.Protocol)
	provider.API = strings.TrimSpace(provider.API)
	if provider.API == "" {
		if provider.Protocol == AIProtocolAnthropic {
			provider.API = defaultAnthropicAPIAddr
		} else {
			provider.API = defaultAIAPIAddr
		}
	}
	provider.Key = strings.TrimSpace(provider.Key)
	provider.Model = strings.TrimSpace(provider.Model)
	if provider.Model == "" {
		if provider.Protocol == AIProtocolAnthropic {
			provider.Model = "claude-3-5-sonnet-latest"
		} else {
			provider.Model = defaultAIModel
		}
	}
	if provider.Protocol == AIProtocolAnthropic {
		provider.SupportsTools = true
		provider.SupportsStreaming = false
	} else {
		provider.SupportsTools = true
		provider.SupportsStreaming = true
	}
	return provider
}

func normalizeAIProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case AIProtocolAnthropic, "claude":
		return AIProtocolAnthropic
	default:
		return AIProtocolOpenAICompatible
	}
}

func chooseDefaultAIProviderID(providers []AIProviderConfig) string {
	for _, provider := range providers {
		if provider.Enabled && strings.TrimSpace(provider.Key) != "" && strings.TrimSpace(provider.Model) != "" {
			return provider.ID
		}
	}
	if len(providers) > 0 {
		return providers[0].ID
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// GetDatabaseConfig 获取数据库配置
func GetDatabaseConfig() (DatabaseConfig, error) {
	config, err := LoadConfig()
	if err != nil {
		return DatabaseConfig{}, err
	}

	dbConfig := config.Database
	if dbConfig.Type == "" {
		defaultConfig := getRuntimeDefaultDatabaseConfig()
		dbConfig.Type = defaultConfig.Type
		if dbConfig.Path == "" {
			dbConfig.Path = defaultConfig.Path
		}
		if dbConfig.Host == "" {
			dbConfig.Host = defaultConfig.Host
		}
		if dbConfig.Port == "" {
			dbConfig.Port = defaultConfig.Port
		}
		if dbConfig.User == "" {
			dbConfig.User = defaultConfig.User
		}
		if dbConfig.Password == "" {
			dbConfig.Password = defaultConfig.Password
		}
		if dbConfig.DBName == "" {
			dbConfig.DBName = defaultConfig.DBName
		}
		if dbConfig.SSLMode == "" {
			dbConfig.SSLMode = defaultConfig.SSLMode
		}
	}
	if dbConfig.Type == DatabaseTypeSQLite {
		normalized, shouldPersist := normalizeSQLiteDatabaseConfig(dbConfig)
		dbConfig = normalized
		if shouldPersist {
			updatedConfig := *config
			updatedConfig.Database = dbConfig
			if err := SaveConfig(&updatedConfig); err != nil {
				return DatabaseConfig{}, err
			}
		}
	}

	return dbConfig, nil
}

// UpdateDatabaseConfig 更新数据库配置
func UpdateDatabaseConfig(dbConfig DatabaseConfig) error {
	config, err := LoadConfig()
	if err != nil {
		return err
	}

	config.Database = dbConfig
	return SaveConfig(config)
}

// GetDefaultSQLitePath 获取 SQLite 默认数据库文件路径。
func GetDefaultSQLitePath() string {
	dataDir := GetDataDir()
	if err := os.MkdirAll(dataDir, 0755); err == nil {
		return filepath.Join(dataDir, "hephaestus.db")
	}

	wd, _ := os.Getwd()
	fallbackDir := filepath.Join(wd, ".config", "hephaestus", "data")
	_ = os.MkdirAll(fallbackDir, 0755)
	return filepath.Join(fallbackDir, "hephaestus.db")
}

func normalizeSQLiteDatabaseConfig(dbConfig DatabaseConfig) (DatabaseConfig, bool) {
	if dbConfig.Path == "" {
		dbConfig.Path = GetDefaultSQLitePath()
		return dbConfig, false
	}

	if GetRuntimeMode() == RuntimeModeDesktop && isLegacyDesktopSQLitePath(dbConfig.Path) {
		dbConfig.Path = GetDefaultSQLitePath()
		return dbConfig, true
	}

	return dbConfig, false
}

func isLegacyDesktopSQLitePath(path string) bool {
	normalized := strings.ToLower(strings.TrimSpace(path))
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	normalized = strings.TrimRight(normalized, "/")
	return normalized == "c:/sqlite" || strings.HasPrefix(normalized, "c:/sqlite/")
}

// generateInitialConfigFile 生成初始配置文件
func generateInitialConfigFile(configPath string) error {
	// 确保目录存在
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	var configContent string
	if isAgentProcess() {
		agentID, _ := ResolveAgentIdentity(defaultAgentID, "")
		initialConfig := &Config{
			Database: DatabaseConfig{
				Type:    DatabaseTypePostgres,
				Host:    "localhost",
				Port:    "5432",
				User:    "postgres",
				DBName:  "hephaestus",
				SSLMode: "disable",
			},
			Agent: AgentConfig{
				ID: agentID,
			},
		}

		data, err := yaml.Marshal(initialConfig)
		if err != nil {
			return fmt.Errorf("序列化配置失败: %w", err)
		}

		configContent = `# Hephaestus Scan Agent 配置文件
# 当前版本 Agent 仍通过共享数据库领取任务，因此部署时仍需补全 database 配置
# agent.id 使用首次生成的 nanoid 作为唯一标识，不可修改，否则会导致节点被识别为新的 Agent
# 显示名称请在 Web 端「Agent 管理」中修改，服务端地址请在 Web 端「Agent 管理 -> 设置」中统一配置
# 配置文件路径: ` + configPath + `

` + injectAgentIDComment(string(data))
	} else {
		dbConfig := getRuntimeDefaultDatabaseConfig()
		configNotice := "# database.type 支持 postgres / sqlite\n# sqlite 默认数据库文件: " + GetDefaultSQLitePath() + "\n"
		if GetRuntimeMode() == RuntimeModeDesktop {
			configNotice = "# 桌面端默认使用 sqlite，本地数据库文件: " + GetDefaultSQLitePath() + "\n# 如需接入共享 PostgreSQL，可将 database.type 改为 postgres 并补全连接信息\n"
		}

		initialConfig := &Config{
			Web: Web{
				Port: 18181,
			},
			Database: dbConfig,
		}

		data, err := yaml.Marshal(initialConfig)
		if err != nil {
			return fmt.Errorf("序列化配置失败: %w", err)
		}

		configContent = `# Hephaestus 配置文件
` + configNotice + `
# web.port: 服务端口，默认 18181
# 其他配置（空间引擎、插件、扫描、代理）存储在数据库中或使用代码默认值
# 配置文件路径: ` + configPath + `

` + string(data)
	}

	// 写入文件
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

func getPrimaryConfigFileName() string {
	return getConfigFileNameForMode(GetRuntimeMode())
}

func getConfigFileNameForMode(mode string) string {
	if mode == RuntimeModeDesktop {
		return desktopConfigFile
	}
	return serverConfigFile
}
