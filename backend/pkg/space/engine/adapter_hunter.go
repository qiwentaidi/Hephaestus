package engine

import (
	"fmt"

	"hephaestus-backend/pkg/space-engine/hunter"
	"hephaestus-backend/pkg/types"
)

// HunterAdapter Hunter 引擎适配器
type HunterAdapter struct {
	client     *hunter.HunterClient
	config     *HunterConfig
	transpiler *Transpiler
}

// HunterConfig Hunter 配置
type HunterConfig struct {
	API string
	Key string
}

// Validate 验证配置
func (c *HunterConfig) Validate() error {
	if c.Key == "" {
		return fmt.Errorf("Hunter key is required")
	}
	return nil
}

// NewHunterAdapter 创建 Hunter 适配器
func NewHunterAdapter(config *HunterConfig) (*HunterAdapter, error) {
	if config == nil {
		return &HunterAdapter{}, nil
	}

	client := hunter.NewHunterClient(&types.HunterAuth{
		API: config.API,
		Key: config.Key,
	})

	return &HunterAdapter{
		client:     client,
		config:     config,
		transpiler: DefaultTranspiler,
	}, nil
}

// Name 返回引擎名称
func (a *HunterAdapter) Name() string {
	return "hunter"
}

// IsConfigured 检查是否已配置
func (a *HunterAdapter) IsConfigured() bool {
	return a.config != nil && a.config.Key != ""
}

// SupportedQueryTypes 返回支持的查询类型
func (a *HunterAdapter) SupportedQueryTypes() []string {
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
		string(QueryTypeCountry),
	}
}

// Search 执行搜索
func (a *HunterAdapter) Search(req *SearchRequest) *SearchResponse {
	resp := &SearchResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	if !a.IsConfigured() {
		resp.Error = fmt.Errorf("Hunter is not configured")
		resp.Message = "Hunter 未配置"
		return resp
	}

	// 构建 Hunter 请求
	hunterReq := &types.HunterSearchRequest{
		Query:         req.Query,
		Page:          req.Page,
		PageSize:      req.PageSize,
		TimeFilter:    "0", // 聚合搜索固定默认最近一个月
		IsWeb:         3,   // 聚合搜索固定默认全部资产
		Deduplication: false,
	}
	result := a.client.HunterApiSearch(hunterReq)

	if result.Code != 200 {
		resp.Error = fmt.Errorf("%s", result.Message)
		resp.Message = result.Message
		return resp
	}

	resp.Total = int(result.Data.Total)
	resp.Message = result.Message

	// 转换为统一格式
	resp.Results = a.transformResults(result.Data.Arr)

	return resp
}

// transformResults 将 Hunter 结果转换为统一格式
func (a *HunterAdapter) transformResults(hunterResults []types.HunterDataArr) []UnifiedResult {
	results := make([]UnifiedResult, 0, len(hunterResults))

	for _, r := range hunterResults {
		// 提取组件名称
		components := make([]string, 0, len(r.Component))
		for _, c := range r.Component {
			name := c.Name
			if c.Version != "" {
				name += "/" + c.Version
			}
			components = append(components, name)
		}

		results = append(results, UnifiedResult{
			URL:        r.URL,
			IP:         r.IP,
			Port:       int(r.Port),
			Protocol:   r.Protocol,
			Domain:     r.Domain,
			Title:      r.WebTitle,
			Banner:     r.Banner,
			ICP:        r.Number,
			ICPName:    r.Company,
			ISP:        r.Isp,
			Country:    r.Country,
			Region:     r.Province,
			City:       r.City,
			Components: components,
			Source:     "hunter",
		})
	}

	return results
}

// GetUserInfo 获取用户信息
func (a *HunterAdapter) GetUserInfo() *UserInfo {
	if !a.IsConfigured() {
		return &UserInfo{
			Error:   fmt.Errorf("Hunter is not configured"),
			Message: "Hunter 未配置",
		}
	}

	user := a.client.HunterApiGetUserInfo()
	if user.Code != 200 {
		return &UserInfo{
			Error:   fmt.Errorf("failed to get user info: %s", user.Message),
			Message: user.Message,
		}
	}

	// 计算总积分
	totalPoints := user.Data.RestEquityPoint + user.Data.RestFreePoint

	return &UserInfo{
		Username:     user.Data.PersonalInfo.Username,
		IsVIP:        user.Data.PersonalInfo.IsCharge,
		Credits:      int64(user.Data.RestEquityPoint),
		UsedCredits:  int64(user.Data.DayFreePoint - user.Data.RestFreePoint),
		TotalCredits: int64(totalPoints),
		Message:      fmt.Sprintf("账户类型: %s, 当日免费积分: %d/%d", user.Data.Type, user.Data.RestFreePoint, user.Data.DayFreePoint),
	}
}

// HunterEngineFactory Hunter 引擎工厂函数
func HunterEngineFactory(config interface{}) (Engine, error) {
	if config == nil {
		return NewHunterAdapter(nil)
	}

	switch cfg := config.(type) {
	case *HunterConfig:
		return NewHunterAdapter(cfg)
	case HunterConfig:
		return NewHunterAdapter(&cfg)
	case map[string]interface{}:
		hunterConfig := &HunterConfig{}
		if v, ok := cfg["api"].(string); ok {
			hunterConfig.API = v
		}
		if v, ok := cfg["key"].(string); ok {
			hunterConfig.Key = v
		}
		return NewHunterAdapter(hunterConfig)
	case *types.SpaceOption:
		hunterConfig := &HunterConfig{
			Key: cfg.HunterKey,
		}
		return NewHunterAdapter(hunterConfig)
	default:
		return nil, fmt.Errorf("unsupported config type: %T", config)
	}
}
