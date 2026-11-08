package engine

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"hephaestus-backend/pkg/space-engine/quake"
	"hephaestus-backend/pkg/types"
)

// QuakeAdapter Quake 引擎适配器
type QuakeAdapter struct {
	client     *quake.QuakeClient
	config     *QuakeConfig
	transpiler *Transpiler
}

// QuakeConfig Quake 配置
type QuakeConfig struct {
	Token string
}

// Validate 验证配置
func (c *QuakeConfig) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("Quake token is required")
	}
	return nil
}

// NewQuakeAdapter 创建 Quake 适配器
func NewQuakeAdapter(config *QuakeConfig) (*QuakeAdapter, error) {
	if config == nil {
		return &QuakeAdapter{}, nil
	}

	client := quake.NewQuakeClient(&types.QuakeAuth{
		Token: config.Token,
	})

	return &QuakeAdapter{
		client:     client,
		config:     config,
		transpiler: DefaultTranspiler,
	}, nil
}

// Name 返回引擎名称
func (a *QuakeAdapter) Name() string {
	return "quake"
}

// IsConfigured 检查是否已配置
func (a *QuakeAdapter) IsConfigured() bool {
	return a.config != nil && a.config.Token != ""
}

// SupportedQueryTypes 返回支持的查询类型
func (a *QuakeAdapter) SupportedQueryTypes() []string {
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
func (a *QuakeAdapter) Search(req *SearchRequest) *SearchResponse {
	resp := &SearchResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	if !a.IsConfigured() {
		resp.Error = fmt.Errorf("Quake is not configured")
		resp.Message = "Quake 未配置"
		return resp
	}

	// 构建 Quake 请求
	quakeReq := &types.QuakeRequestOptions{
		Query:    req.Query,
		PageNum:  req.Page,
		PageSize: req.PageSize,
		Latest:   true, // 默认只查最新数据
	}

	result := a.client.QuakeApiSearch(quakeReq)

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

// transformResults 将 Quake 结果转换为统一格式
func (a *QuakeAdapter) transformResults(quakeResults []types.QuakeData) []UnifiedResult {
	results := make([]UnifiedResult, 0, len(quakeResults))

	for _, r := range quakeResults {
		// 解析地区信息
		country, region, city := parseQuakePosition(r.Position)

		// Host 可能是服务名（如 "ssh"）或 IP，只在它是域名时使用。
		domain := quakeDomain(r.Host, r.URL)

		results = append(results, UnifiedResult{
			URL:        r.URL,
			IP:         r.IP,
			Port:       r.Port,
			Protocol:   r.Protocol,
			Domain:     domain,
			Title:      r.Title,
			ICP:        r.IcpNumber,
			ICPName:    r.IcpName,
			ISP:        r.Isp,
			Country:    country,
			Region:     region,
			City:       city,
			Components: r.Components,
			Source:     "quake",
		})
	}

	return results
}

// parseQuakePosition 解析 Quake 地区字符串
func parseQuakePosition(position string) (country, region, city string) {
	// Quake 的 Position 格式可能是 "中国/北京/北京" 或 "中国"
	parts := strings.Split(position, "/")
	if len(parts) >= 1 {
		country = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 {
		region = strings.TrimSpace(parts[1])
	}
	if len(parts) >= 3 {
		city = strings.TrimSpace(parts[2])
	}
	return
}

func quakeDomain(host, rawURL string) string {
	if domain := extractDomainFromURL(host); domain != "" {
		return domain
	}
	return extractDomainFromURL(rawURL)
}

// extractDomainFromURL 从任意协议的 URL 中提取域名；IP 和服务名不作为域名返回。
func extractDomainFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "//" + rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return ""
	}
	return host
}

// GetUserInfo 获取用户信息
func (a *QuakeAdapter) GetUserInfo() *UserInfo {
	if !a.IsConfigured() {
		return &UserInfo{
			Error:   fmt.Errorf("Quake is not configured"),
			Message: "Quake 未配置",
		}
	}

	user := a.client.QuakeApiGetUserInfo(context.Background())
	if user == nil {
		return &UserInfo{
			Error:   fmt.Errorf("failed to get user info"),
			Message: "获取用户信息失败",
		}
	}

	// 计算总积分
	totalCredits := user.Data.Credit + user.Data.PersistentCredit

	// 获取角色名称
	var roleName string
	if len(user.Data.Role) > 0 {
		roleName = user.Data.Role[0].Fullname
	}

	return &UserInfo{
		Username:     user.Data.User.Username,
		Email:        user.Data.User.Email,
		Credits:      int64(user.Data.Credit),
		TotalCredits: int64(totalCredits),
		Message:      fmt.Sprintf("会员等级: %s, 永久积分: %d", roleName, user.Data.PersistentCredit),
	}
}

// QuakeEngineFactory Quake 引擎工厂函数
func QuakeEngineFactory(config interface{}) (Engine, error) {
	if config == nil {
		return NewQuakeAdapter(nil)
	}

	switch cfg := config.(type) {
	case *QuakeConfig:
		return NewQuakeAdapter(cfg)
	case QuakeConfig:
		return NewQuakeAdapter(&cfg)
	case map[string]interface{}:
		quakeConfig := &QuakeConfig{}
		if v, ok := cfg["token"].(string); ok {
			quakeConfig.Token = v
		}
		return NewQuakeAdapter(quakeConfig)
	case *types.SpaceOption:
		quakeConfig := &QuakeConfig{
			Token: cfg.QuakeKey,
		}
		return NewQuakeAdapter(quakeConfig)
	default:
		return nil, fmt.Errorf("unsupported config type: %T", config)
	}
}
