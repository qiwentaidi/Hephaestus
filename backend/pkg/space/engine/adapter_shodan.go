package engine

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	shodanclient "hephaestus-backend/pkg/space-engine/shodan"
	"hephaestus-backend/pkg/types"
)

// ShodanAdapter Shodan 搜索引擎适配器
type ShodanAdapter struct {
	client     *shodanclient.Client
	config     *ShodanConfig
	transpiler *Transpiler
}

type ShodanConfig struct {
	API string
	Key string
}

func (c *ShodanConfig) Validate() error {
	if c == nil || strings.TrimSpace(c.Key) == "" {
		return fmt.Errorf("Shodan key is required")
	}
	return nil
}

func NewShodanAdapter(config *ShodanConfig) (*ShodanAdapter, error) {
	if config == nil {
		return &ShodanAdapter{}, nil
	}
	return &ShodanAdapter{
		client:     shodanclient.NewClient(config.API, config.Key),
		config:     config,
		transpiler: DefaultTranspiler,
	}, nil
}

func (a *ShodanAdapter) Name() string {
	return "shodan"
}

func (a *ShodanAdapter) IsConfigured() bool {
	return a.config != nil && strings.TrimSpace(a.config.Key) != ""
}

func (a *ShodanAdapter) SupportedQueryTypes() []string {
	return []string{
		string(QueryTypeIP),
		string(QueryTypePort),
		string(QueryTypeDomain),
		string(QueryTypeHost),
		string(QueryTypeCountry),
		string(QueryTypeCity),
		string(QueryTypeOrg),
		string(QueryTypeISP),
		string(QueryTypeASN),
		string(QueryTypeTitle),
		string(QueryTypeHeader),
		string(QueryTypeBody),
		string(QueryTypeProduct),
		string(QueryTypeApp),
		string(QueryTypeOS),
		string(QueryTypeCert),
		string(QueryTypeIconHash),
	}
}

func (a *ShodanAdapter) Search(req *SearchRequest) *SearchResponse {
	resp := &SearchResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
	}
	if !a.IsConfigured() {
		resp.Error = fmt.Errorf("Shodan is not configured")
		resp.Message = "Shodan 未配置"
		return resp
	}

	result := a.client.Search(req.Query, req.Page)
	if result.Error != "" {
		resp.Error = errors.New(result.Error)
		resp.Message = result.Error
		return resp
	}
	resp.Total = result.Total
	if result.Credits > 0 {
		resp.Message = fmt.Sprintf("消耗 %d 次查询额度", result.Credits)
	}

	results := a.transformResults(result.Matches)
	if req.PageSize > 0 && len(results) > req.PageSize {
		results = results[:req.PageSize]
	}
	resp.Results = results
	return resp
}

func (a *ShodanAdapter) GetUserInfo() *UserInfo {
	if !a.IsConfigured() {
		return nil
	}
	info := a.client.APIInfo()
	if info.Error != "" {
		err := errors.New(info.Error)
		return &UserInfo{Error: err, Message: info.Error}
	}
	plan := strings.ToLower(strings.TrimSpace(info.Plan))
	return &UserInfo{
		Username:     strings.ToUpper(plan),
		IsVIP:        plan != "" && plan != "oss" && plan != "free",
		Credits:      int64(info.QueryCredits),
		TotalCredits: int64(info.ScanCredits),
		Message:      info.Plan,
	}
}

func (a *ShodanAdapter) transformResults(matches []types.ShodanMatch) []UnifiedResult {
	results := make([]UnifiedResult, 0, len(matches))
	for _, match := range matches {
		ip := strings.TrimSpace(match.IP)
		domain := shodanDomain(match)
		protocol := inferShodanProtocol(match)
		host := domain
		if host == "" {
			host = ip
		}
		url := shodanURL(protocol, host, match.Port)

		updateTime := time.Time{}
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(match.Timestamp)); err == nil {
			updateTime = parsed
		}

		banner := match.Data
		if runes := []rune(banner); len(runes) > 4096 {
			banner = string(runes[:4096])
		}

		isp := strings.TrimSpace(match.ISP)
		if isp == "" {
			isp = strings.TrimSpace(match.Org)
		}

		results = append(results, UnifiedResult{
			URL:        url,
			IP:         ip,
			Port:       match.Port,
			Protocol:   protocol,
			Domain:     domain,
			Title:      strings.TrimSpace(match.HTTP.Title),
			Banner:     banner,
			Components: shodanComponents(match),
			ISP:        isp,
			Country:    strings.TrimSpace(match.Location.CountryName),
			Region:     strings.TrimSpace(match.Location.RegionCode),
			City:       strings.TrimSpace(match.Location.City),
			Source:     "shodan",
			UpdateTime: updateTime,
		})
	}
	return results
}

func shodanDomain(match types.ShodanMatch) string {
	ip := strings.TrimSpace(match.IP)
	for _, values := range [][]string{match.Domains, match.Hostnames} {
		for _, value := range values {
			value = strings.ToLower(strings.TrimSpace(value))
			if value == "" || value == ip || net.ParseIP(value) != nil {
				continue
			}
			return value
		}
	}
	return ""
}

func inferShodanProtocol(match types.ShodanMatch) string {
	module := strings.ToLower(strings.TrimSpace(match.Shodan.Module))
	switch module {
	case "https":
		return "https"
	case "http":
		if len(match.SSL) > 0 {
			return "https"
		}
		return "http"
	}
	if module != "" {
		return module
	}
	if match.HTTP.Title != "" || match.HTTP.Status != 0 {
		if len(match.SSL) > 0 {
			return "https"
		}
		return "http"
	}
	if transport := strings.ToLower(strings.TrimSpace(match.Transport)); transport != "" {
		return transport
	}
	return ""
}

func shodanURL(protocol, host string, port int) string {
	if host == "" || (protocol != "http" && protocol != "https") {
		return ""
	}
	portPart := ""
	if port > 0 && !((protocol == "http" && port == 80) || (protocol == "https" && port == 443)) {
		portPart = fmt.Sprintf(":%d", port)
	}
	return fmt.Sprintf("%s://%s%s", protocol, host, portPart)
}

func shodanComponents(match types.ShodanMatch) []string {
	components := make([]string, 0, 2)
	if product := strings.TrimSpace(match.Product); product != "" {
		components = append(components, product)
	}
	if version := strings.TrimSpace(match.Version); version != "" {
		components = append(components, version)
	}
	return components
}

func ShodanEngineFactory(config interface{}) (Engine, error) {
	shodanConfig, ok := config.(*ShodanConfig)
	if !ok {
		return nil, fmt.Errorf("invalid Shodan config")
	}
	return NewShodanAdapter(shodanConfig)
}
