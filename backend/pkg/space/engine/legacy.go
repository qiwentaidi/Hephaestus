package engine

import (
	"context"
	"strconv"
	"strings"

	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/types"
)

// LegacyUncover 使用新架构实现的聚合搜索（兼容旧接口）
// 这个函数提供与旧 Uncover 函数相同的接口，但内部使用新的适配器模式
// 特性：并发查询所有引擎、自动语法转换、结果去重
func LegacyUncover(query, queryType string, o types.SpaceOption) []types.SpaceResult {
	// 创建聚合器
	agg, err := NewAggregatorFromSpaceOption(&o)
	if err != nil {
		logger.Error("[uncover] failed to create aggregator: %v", err)
		return nil
	}

	// 转换查询类型
	qt := convertLegacyQueryType(queryType)

	logger.Info("[uncover] starting aggregated search: query=%s, type=%s", query, qt)

	// 执行聚合搜索
	ctx := context.Background()
	req := &AggregatedSearchRequest{
		Query:     query,
		QueryType: qt,
		Page:      1,
		PageSize:  100,
	}

	// 获取最大页数
	maxPages := o.MaxPages
	if maxPages <= 0 {
		maxPages = 1
	}
	if maxPages > 10 {
		maxPages = 10
	}

	// 执行并发多页搜索
	resp := agg.MultiPageSearch(ctx, req, maxPages)

	logger.Info("[uncover] search completed: %d results from %d engines in %v",
		len(resp.Results), len(resp.EngineStatus), resp.Duration)

	// 转换为旧格式（包含去重）
	return convertToLegacyResults(resp.Results)
}

// convertLegacyQueryType 将旧的查询类型转换为新的 QueryType
func convertLegacyQueryType(legacyType string) QueryType {
	switch legacyType {
	case "标题":
		return QueryTypeTitle
	case "备案号":
		return QueryTypeICPNumber
	case "备案名称":
		return QueryTypeICPName
	case "域名":
		return QueryTypeDomain
	case "IP":
		return QueryTypeIP
	case "Body":
		return QueryTypeBody
	case "应用", "组件", "App", "app":
		return QueryTypeApp
	case "产品", "Product", "product":
		return QueryTypeProduct
	default:
		return QueryTypeDefault
	}
}

// convertToLegacyResults 将新格式结果转换为旧格式
func convertToLegacyResults(results []UnifiedResult) []types.SpaceResult {
	uniqueResults := DeduplicateResults(results)
	legacyResults := make([]types.SpaceResult, 0, len(uniqueResults))

	for _, r := range uniqueResults {
		// 转换组件为逗号分隔的字符串
		components := strings.Join(r.Components, ",")

		// 转换 Source 为大写首字母格式
		source := strings.Title(r.Source)

		legacyResults = append(legacyResults, types.SpaceResult{
			URL:        r.URL,
			IP:         r.IP,
			Domain:     r.Domain,
			Port:       strconv.Itoa(r.Port),
			Protocol:   r.Protocol,
			Title:      r.Title,
			Components: components,
			ICP:        r.ICP,
			ICPName:    r.ICPName,
			Source:     source,
		})
	}

	return legacyResults
}

// UnifiedToSpaceResult 将单个 UnifiedResult 转换为 SpaceResult
func UnifiedToSpaceResult(r UnifiedResult) types.SpaceResult {
	return types.SpaceResult{
		URL:        r.URL,
		IP:         r.IP,
		Domain:     r.Domain,
		Port:       strconv.Itoa(r.Port),
		Protocol:   r.Protocol,
		Title:      r.Title,
		Components: strings.Join(r.Components, ","),
		ICP:        r.ICP,
		ICPName:    r.ICPName,
		Source:     strings.Title(r.Source),
	}
}

// SpaceResultToUnified 将 SpaceResult 转换为 UnifiedResult
func SpaceResultToUnified(r types.SpaceResult) UnifiedResult {
	port, _ := strconv.Atoi(r.Port)
	var components []string
	if r.Components != "" {
		components = strings.Split(r.Components, ",")
	}

	return UnifiedResult{
		URL:        r.URL,
		IP:         r.IP,
		Port:       port,
		Protocol:   r.Protocol,
		Domain:     r.Domain,
		Title:      r.Title,
		Components: components,
		ICP:        r.ICP,
		ICPName:    r.ICPName,
		Source:     strings.ToLower(r.Source),
	}
}
