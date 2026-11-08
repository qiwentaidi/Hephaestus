// Package engine 提供空间引擎的统一接口和适配器模式实现
package engine

import (
	"time"
)

// UnifiedResult 统一的搜索结果数据模型
// 无论上游是哪个引擎，最终都转换为此格式
type UnifiedResult struct {
	URL        string    `json:"url"`                  // 完整 URL
	IP         string    `json:"ip"`                   // IP 地址
	Port       int       `json:"port"`                 // 端口号
	Protocol   string    `json:"protocol"`             // 协议（http/https/ssh 等）
	Domain     string    `json:"domain"`               // 域名
	Title      string    `json:"title"`                // 网页标题
	Banner     string    `json:"banner,omitempty"`     // Banner 信息
	Components []string  `json:"components,omitempty"` // 组件/产品列表
	ICP        string    `json:"icp,omitempty"`        // ICP 备案号
	ICPName    string    `json:"icpName,omitempty"`    // ICP 备案名称/备案单位
	ISP        string    `json:"isp,omitempty"`        // 运营商
	Country    string    `json:"country,omitempty"`    // 国家
	Region     string    `json:"region,omitempty"`     // 地区/省份
	City       string    `json:"city,omitempty"`       // 城市
	Source     string    `json:"source"`               // 数据来源（fofa/hunter/quake/censys）
	UpdateTime time.Time `json:"updateTime,omitempty"` // 更新时间
}

// SearchRequest 统一的搜索请求
type SearchRequest struct {
	Query    string // 原始查询语句（已转换为引擎专用语法）
	Page     int    // 页码，从 1 开始
	PageSize int    // 每页数量
}

// SearchResponse 统一的搜索响应
type SearchResponse struct {
	Results  []UnifiedResult `json:"results"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
	Message  string          `json:"message,omitempty"`
	Error    error           `json:"-"` // 内部错误，不序列化
}

// UserInfo 统一的用户信息
type UserInfo struct {
	Username     string `json:"username"`
	Email        string `json:"email,omitempty"`
	IsVIP        bool   `json:"isVip"`
	Credits      int64  `json:"credits"`      // 积分/额度
	UsedCredits  int64  `json:"usedCredits"`  // 已使用
	TotalCredits int64  `json:"totalCredits"` // 总额度
	ExpireTime   string `json:"expireTime,omitempty"`
	Message      string `json:"message,omitempty"`
	Error        error  `json:"-"`
}

// Engine 空间搜索引擎接口
// 所有引擎适配器必须实现此接口
type Engine interface {
	// Name 返回引擎名称（如 "fofa", "hunter", "quake", "censys"）
	Name() string

	// Search 执行搜索，返回统一格式的结果
	Search(req *SearchRequest) *SearchResponse

	// GetUserInfo 获取用户信息（可选实现，返回 nil 表示不支持）
	GetUserInfo() *UserInfo

	// IsConfigured 检查引擎是否已配置（API Key 等）
	IsConfigured() bool

	// SupportedQueryTypes 返回支持的查询类型
	SupportedQueryTypes() []string
}

// EngineConfig 引擎配置接口
type EngineConfig interface {
	// Validate 验证配置是否有效
	Validate() error
}

// EngineFactory 引擎工厂函数类型
type EngineFactory func(config interface{}) (Engine, error)

// Registry 引擎注册表
type Registry struct {
	engines   map[string]Engine
	factories map[string]EngineFactory
}

// NewRegistry 创建新的引擎注册表
func NewRegistry() *Registry {
	return &Registry{
		engines:   make(map[string]Engine),
		factories: make(map[string]EngineFactory),
	}
}

// RegisterFactory 注册引擎工厂
func (r *Registry) RegisterFactory(name string, factory EngineFactory) {
	r.factories[name] = factory
}

// CreateEngine 使用工厂创建引擎实例
func (r *Registry) CreateEngine(name string, config interface{}) (Engine, error) {
	factory, ok := r.factories[name]
	if !ok {
		return nil, nil // 未注册的引擎
	}
	engine, err := factory(config)
	if err != nil {
		return nil, err
	}
	r.engines[name] = engine
	return engine, nil
}

// GetEngine 获取已创建的引擎
func (r *Registry) GetEngine(name string) Engine {
	return r.engines[name]
}

// GetAllEngines 获取所有已创建的引擎
func (r *Registry) GetAllEngines() map[string]Engine {
	return r.engines
}

// GetConfiguredEngines 获取所有已配置的引擎
func (r *Registry) GetConfiguredEngines() []Engine {
	var configured []Engine
	for _, engine := range r.engines {
		if engine.IsConfigured() {
			configured = append(configured, engine)
		}
	}
	return configured
}
