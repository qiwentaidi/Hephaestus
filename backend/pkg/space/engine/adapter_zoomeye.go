package engine

import (
	"fmt"
	"strings"

	"hephaestus-backend/pkg/space-engine/zoomeye"
	"hephaestus-backend/pkg/types"
)

type ZoomeyeAdapter struct {
	client     *zoomeye.ZoomeyeClient
	config     *ZoomeyeConfig
	transpiler *Transpiler
}

type ZoomeyeConfig struct {
	Address string
	Key     string
}

func (c *ZoomeyeConfig) Validate() error {
	if c.Key == "" {
		return fmt.Errorf("ZoomEye key is required")
	}
	return nil
}

func NewZoomeyeAdapter(config *ZoomeyeConfig) (*ZoomeyeAdapter, error) {
	if config == nil {
		return &ZoomeyeAdapter{}, nil
	}
	client := zoomeye.NewZoomeyeClient(&types.ZoomeyeAuth{
		Address: config.Address,
		Key:     config.Key,
	})
	return &ZoomeyeAdapter{
		client:     client,
		config:     config,
		transpiler: DefaultTranspiler,
	}, nil
}

func (a *ZoomeyeAdapter) Name() string {
	return "zoomeye"
}

func (a *ZoomeyeAdapter) IsConfigured() bool {
	return a.config != nil && a.config.Key != ""
}

func (a *ZoomeyeAdapter) SupportedQueryTypes() []string {
	return []string{
		string(QueryTypeIP),
		string(QueryTypePort),
		string(QueryTypeDomain),
		string(QueryTypeHost),
		string(QueryTypeTitle),
		string(QueryTypeBody),
		string(QueryTypeCountry),
		string(QueryTypeRegion),
		string(QueryTypeCity),
		string(QueryTypeOrg),
		string(QueryTypeApp),
		string(QueryTypeProduct),
		string(QueryTypeOS),
	}
}

func (a *ZoomeyeAdapter) Search(req *SearchRequest) *SearchResponse {
	resp := &SearchResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
	}
	if !a.IsConfigured() {
		resp.Error = fmt.Errorf("ZoomEye is not configured")
		resp.Message = "ZoomEye 未配置"
		return resp
	}

	subType := "v4"
	fields := "ip,port,domain,url,service,title,product,device,version,hostname,update_time,country.name,province.name,city.name,isp.name,organization.name,protocol"
	if subType == "v4" {
		subType = inferZoomeyeSubType(req.Query)
	}

	result := a.client.ZoomeyeApiSearch(&types.ZoomeyeSearchRequest{
		Query:    req.Query,
		Page:     req.Page,
		PageSize: req.PageSize,
		SubType:  subType,
		Fields:   fields,
	})
	if result.Code != 60000 {
		resp.Error = fmt.Errorf("%s", result.Message)
		resp.Message = result.Message
		return resp
	}
	resp.Total = result.Total
	resp.Message = result.Message
	resp.Results = a.transformResults(result.Data)
	return resp
}

func (a *ZoomeyeAdapter) transformResults(rows []types.ZoomeyeDataItem) []UnifiedResult {
	results := make([]UnifiedResult, 0, len(rows))
	for _, r := range rows {
		title := ""
		if len(r.Title) > 0 {
			title = strings.Join(r.Title, " ")
		}
		protocol := strings.TrimSpace(r.Service)
		if protocol == "" {
			protocol = strings.TrimSpace(r.Protocol)
		}
		domain := strings.TrimSpace(r.Domain)
		if domain == "" {
			domain = strings.TrimSpace(r.Hostname)
		}
		results = append(results, UnifiedResult{
			URL:        r.URL,
			IP:         r.IP,
			Port:       r.Port,
			Protocol:   protocol,
			Domain:     domain,
			Title:      title,
			Banner:     r.Banner,
			Components: zoomeye.BuildComponentList(r),
			ISP:        r.ISPName,
			Country:    r.CountryName,
			Region:     r.ProvinceName,
			City:       r.CityName,
			Source:     "zoomeye",
		})
	}
	return results
}

func inferZoomeyeSubType(query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return "v4"
	}

	webKeywords := []string{
		`title:`, `body:`, `header:`, `server:`, `banner:`, `domain:`, `host:`, `app:`, `product:`,
		`icon_hash:`, `cert:`, `ssl:`, `site:`, `url:`,
	}
	for _, keyword := range webKeywords {
		if strings.Contains(q, keyword) {
			return "web"
		}
	}
	return "v4"
}

func (a *ZoomeyeAdapter) GetUserInfo() *UserInfo {
	if !a.IsConfigured() {
		return &UserInfo{
			Error:   fmt.Errorf("ZoomEye is not configured"),
			Message: "ZoomEye 未配置",
		}
	}
	info := a.client.ZoomeyeApiGetUserInfo()
	if info.Code != 60000 {
		return &UserInfo{
			Error:   fmt.Errorf("%s", info.Message),
			Message: info.Message,
		}
	}
	return &UserInfo{
		Username:     info.Data.Username,
		Email:        info.Data.Email,
		Credits:      zoomeye.ParseZoomeyeInt(info.Data.Subscription.Points),
		TotalCredits: zoomeye.ParseZoomeyeInt(info.Data.Subscription.Points) + zoomeye.ParseZoomeyeInt(info.Data.Subscription.ZoomeyePoints),
		ExpireTime:   info.Data.Subscription.EndDate,
		Message:      fmt.Sprintf("订阅计划: %s", info.Data.Subscription.Plan),
	}
}

func ZoomeyeEngineFactory(config interface{}) (Engine, error) {
	if config == nil {
		return NewZoomeyeAdapter(nil)
	}
	switch cfg := config.(type) {
	case *ZoomeyeConfig:
		return NewZoomeyeAdapter(cfg)
	case ZoomeyeConfig:
		return NewZoomeyeAdapter(&cfg)
	case map[string]interface{}:
		zoomeyeConfig := &ZoomeyeConfig{}
		if v, ok := cfg["api"].(string); ok {
			zoomeyeConfig.Address = v
		}
		if v, ok := cfg["address"].(string); ok && zoomeyeConfig.Address == "" {
			zoomeyeConfig.Address = v
		}
		if v, ok := cfg["key"].(string); ok {
			zoomeyeConfig.Key = v
		}
		return NewZoomeyeAdapter(zoomeyeConfig)
	case *types.SpaceOption:
		return NewZoomeyeAdapter(&ZoomeyeConfig{Address: cfg.ZoomeyeApi, Key: cfg.ZoomeyeKey})
	default:
		return nil, fmt.Errorf("unsupported config type: %T", config)
	}
}
