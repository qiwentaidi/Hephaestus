package engine

import (
	"fmt"
	"strconv"
	"strings"

	"hephaestus-backend/pkg/space-engine/fofa"
	"hephaestus-backend/pkg/types"
)

// FofaAdapter FOFA 引擎适配器
type FofaAdapter struct {
	client     *fofa.FofaClient
	config     *FofaConfig
	transpiler *Transpiler
}

// FofaConfig FOFA 配置
type FofaConfig struct {
	Address string
	Email   string
	Key     string
}

// Validate 验证配置
func (c *FofaConfig) Validate() error {
	if c.Email == "" {
		return fmt.Errorf("FOFA email is required")
	}
	if c.Key == "" {
		return fmt.Errorf("FOFA key is required")
	}
	return nil
}

// NewFofaAdapter 创建 FOFA 适配器
func NewFofaAdapter(config *FofaConfig) (*FofaAdapter, error) {
	if config == nil {
		return &FofaAdapter{}, nil
	}

	client := fofa.NewFofaClient(&types.FofaAuth{
		Address: config.Address,
		Email:   config.Email,
		Key:     config.Key,
	})

	return &FofaAdapter{
		client:     client,
		config:     config,
		transpiler: DefaultTranspiler,
	}, nil
}

// Name 返回引擎名称
func (a *FofaAdapter) Name() string {
	return "fofa"
}

// IsConfigured 检查是否已配置
func (a *FofaAdapter) IsConfigured() bool {
	return a.config != nil && a.config.Key != "" && a.config.Email != ""
}

// SupportedQueryTypes 返回支持的查询类型
func (a *FofaAdapter) SupportedQueryTypes() []string {
	return []string{
		string(QueryTypeIP),
		string(QueryTypeDomain),
		string(QueryTypeTitle),
		string(QueryTypeBody),
		string(QueryTypeICP),
		string(QueryTypePort),
		string(QueryTypeServer),
		string(QueryTypeCert),
		string(QueryTypeApp),
		string(QueryTypeProduct),
		string(QueryTypeOrg),
		string(QueryTypeCountry),
	}
}

// Search 执行搜索
func (a *FofaAdapter) Search(req *SearchRequest) *SearchResponse {
	resp := &SearchResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	if !a.IsConfigured() {
		resp.Error = fmt.Errorf("FOFA is not configured")
		resp.Message = "FOFA 未配置"
		return resp
	}

	result := a.client.FofaApiSearch(
		req.Query,
		strconv.Itoa(req.PageSize),
		strconv.Itoa(req.Page),
		false,
		false,
	)

	if result.Error {
		resp.Error = fmt.Errorf("%s", result.Message)
		resp.Message = result.Message
		return resp
	}

	resp.Total = result.Size
	resp.Message = result.Message

	// 转换为统一格式
	resp.Results = a.transformResults(result.Results)

	return resp
}

// transformResults 将 FOFA 结果转换为统一格式
func (a *FofaAdapter) transformResults(fofaResults []types.FofaResult) []UnifiedResult {
	results := make([]UnifiedResult, 0, len(fofaResults))

	for _, r := range fofaResults {
		port, _ := strconv.Atoi(r.Port)

		// 解析地区信息
		country, region, city := parseRegion(r.Country)

		// 解析组件
		var components []string
		if r.Product != "" {
			components = strings.Split(r.Product, ",")
			for i := range components {
				components[i] = strings.TrimSpace(components[i])
			}
		}

		results = append(results, UnifiedResult{
			URL:        r.URL,
			IP:         r.IP,
			Port:       port,
			Protocol:   r.Protocol,
			Domain:     r.Domain,
			Title:      r.Title,
			ICP:        r.ICP,
			Country:    country,
			Region:     region,
			City:       city,
			Components: components,
			Source:     "fofa",
		})
	}

	return results
}

// parseRegion 解析地区字符串（格式：国家/省份/城市）
func parseRegion(region string) (country, province, city string) {
	parts := strings.Split(region, "/")
	if len(parts) >= 1 {
		country = parts[0]
	}
	if len(parts) >= 2 {
		province = parts[1]
	}
	if len(parts) >= 3 {
		city = parts[2]
	}
	return
}

// GetUserInfo 获取用户信息
func (a *FofaAdapter) GetUserInfo() *UserInfo {
	if !a.IsConfigured() {
		return &UserInfo{
			Error:   fmt.Errorf("FOFA is not configured"),
			Message: "FOFA 未配置",
		}
	}

	user := a.client.FofaApiSearchByUserInfo()
	if user.Error {
		return &UserInfo{
			Error:   fmt.Errorf("failed to get user info"),
			Message: "获取用户信息失败",
		}
	}

	return &UserInfo{
		Username:     user.Username,
		Email:        user.Email,
		IsVIP:        user.Isvip,
		Credits:      int64(user.RemainApiQuery),
		UsedCredits:  int64(user.FofaPoint - user.RemainApiQuery),
		TotalCredits: int64(user.FofaPoint),
	}
}

// FofaEngineFactory FOFA 引擎工厂函数
func FofaEngineFactory(config interface{}) (Engine, error) {
	if config == nil {
		return NewFofaAdapter(nil)
	}

	// 支持多种配置类型
	switch cfg := config.(type) {
	case *FofaConfig:
		return NewFofaAdapter(cfg)
	case FofaConfig:
		return NewFofaAdapter(&cfg)
	case map[string]interface{}:
		fofaConfig := &FofaConfig{}
		if v, ok := cfg["address"].(string); ok {
			fofaConfig.Address = v
		}
		if v, ok := cfg["email"].(string); ok {
			fofaConfig.Email = v
		}
		if v, ok := cfg["key"].(string); ok {
			fofaConfig.Key = v
		}
		return NewFofaAdapter(fofaConfig)
	case *types.SpaceOption:
		fofaConfig := &FofaConfig{
			Address: cfg.FofaApi,
			Email:   cfg.FofaEmail,
			Key:     cfg.FofaKey,
		}
		return NewFofaAdapter(fofaConfig)
	default:
		return nil, fmt.Errorf("unsupported config type: %T", config)
	}
}

func init() {
	// 自动注册工厂（如果有全局注册表的话）
}
