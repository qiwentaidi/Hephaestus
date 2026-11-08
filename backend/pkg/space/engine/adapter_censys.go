package engine

import (
	"fmt"

	"hephaestus-backend/pkg/space-engine/censys"
	"hephaestus-backend/pkg/types"
)

// CensysAdapter Censys 引擎适配器
type CensysAdapter struct {
	client     *censys.CensysClient
	config     *CensysConfig
	transpiler *Transpiler
}

// CensysConfig Censys 配置
type CensysConfig struct {
	Token          string
	OrganizationID string
}

// Validate 验证配置
func (c *CensysConfig) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("Censys token is required")
	}
	return nil
}

// NewCensysAdapter 创建 Censys 适配器
func NewCensysAdapter(config *CensysConfig) (*CensysAdapter, error) {
	if config == nil {
		return &CensysAdapter{}, nil
	}

	client := censys.NewCensysClient(&types.CensysAuth{
		Token:          config.Token,
		OrganizationID: config.OrganizationID,
	})

	return &CensysAdapter{
		client:     client,
		config:     config,
		transpiler: DefaultTranspiler,
	}, nil
}

// Name 返回引擎名称
func (a *CensysAdapter) Name() string {
	return "censys"
}

// IsConfigured 检查是否已配置
func (a *CensysAdapter) IsConfigured() bool {
	if a.config == nil {
		return false
	}
	return a.config.Token != ""
}

// SupportedQueryTypes 返回支持的查询类型
func (a *CensysAdapter) SupportedQueryTypes() []string {
	return []string{
		string(QueryTypeIP),
		string(QueryTypeDomain),
		string(QueryTypeTitle),
		string(QueryTypePort),
		string(QueryTypeServer),
		string(QueryTypeCert),
		string(QueryTypeApp),
		string(QueryTypeProduct),
		string(QueryTypeCountry),
	}
}

// Search 执行搜索
func (a *CensysAdapter) Search(req *SearchRequest) *SearchResponse {
	resp := &SearchResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	if !a.IsConfigured() {
		resp.Error = fmt.Errorf("Censys is not configured")
		resp.Message = "Censys 未配置"
		return resp
	}

	// 构建 Censys 请求
	censysReq := &types.CensysSearchRequest{
		Query:    req.Query,
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	result := a.client.CensysApiSearch(censysReq)

	if result.Code != 0 {
		resp.Error = fmt.Errorf("%s", result.Message)
		resp.Message = result.Message
		return resp
	}

	resp.Total = result.Total
	resp.Message = result.Message

	// 转换为统一格式
	resp.Results = a.transformResults(result.Data)

	return resp
}

// transformResults 将 Censys 结果转换为统一格式
func (a *CensysAdapter) transformResults(censysResults []types.CensysResultData) []UnifiedResult {
	results := make([]UnifiedResult, 0, len(censysResults))

	for _, r := range censysResults {
		results = append(results, UnifiedResult{
			URL:        r.URL,
			IP:         r.IP,
			Port:       r.Port,
			Protocol:   r.Protocol,
			Domain:     r.Domain,
			Title:      r.Title,
			Components: r.Components,
			Source:     "censys",
		})
	}

	return results
}

// GetUserInfo 获取用户信息
// Censys 暂不支持通过 API 获取用户信息
func (a *CensysAdapter) GetUserInfo() *UserInfo {
	return &UserInfo{
		Message: "Censys 暂不支持通过 API 获取用户信息",
	}
}

// CensysEngineFactory Censys 引擎工厂函数
func CensysEngineFactory(config interface{}) (Engine, error) {
	if config == nil {
		return NewCensysAdapter(nil)
	}

	switch cfg := config.(type) {
	case *CensysConfig:
		return NewCensysAdapter(cfg)
	case CensysConfig:
		return NewCensysAdapter(&cfg)
	case map[string]interface{}:
		censysConfig := &CensysConfig{}
		if v, ok := cfg["token"].(string); ok {
			censysConfig.Token = v
		}
		if v, ok := cfg["organizationId"].(string); ok {
			censysConfig.OrganizationID = v
		}
		return NewCensysAdapter(censysConfig)
	case *types.SpaceOption:
		censysConfig := &CensysConfig{
			Token:          cfg.CensysToken,
			OrganizationID: cfg.CensysOrgID,
		}
		return NewCensysAdapter(censysConfig)
	default:
		return nil, fmt.Errorf("unsupported config type: %T", config)
	}
}
