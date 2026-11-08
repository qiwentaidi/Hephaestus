package space

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/handlers/platform"
	"hephaestus-backend/pkg/config"
	shodanclient "hephaestus-backend/pkg/space-engine/shodan"
	"hephaestus-backend/pkg/space/engine"
	"hephaestus-backend/pkg/types"
)

var hunterOrOperatorPattern = regexp.MustCompile(`(?i)\bOR\b`)

// SpaceSearchHandler 空间引擎聚合搜索处理
func SpaceSearchHandler(c *gin.Context) {
	var req types.SpaceSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	// 从数据库读取配置
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加载配置失败: " + err.Error()})
		return
	}

	// 构建选项，优先使用请求中的配置，如果没有则使用数据库中的配置
	option := types.SpaceOption{
		FofaApi:       req.Option.FofaApi,
		FofaEmail:     req.Option.FofaEmail,
		FofaKey:       req.Option.FofaKey,
		HunterKey:     req.Option.HunterKey,
		QuakeKey:      req.Option.QuakeKey,
		ZoomeyeApi:    req.Option.ZoomeyeApi,
		ZoomeyeKey:    req.Option.ZoomeyeKey,
		CensysToken:   req.Option.CensysToken,
		CensysOrgID:   req.Option.CensysOrgID,
		ShodanApi:     req.Option.ShodanApi,
		ShodanKey:     req.Option.ShodanKey,
		EnableFofa:    req.Option.EnableFofa,
		EnableHunter:  req.Option.EnableHunter,
		EnableQuake:   req.Option.EnableQuake,
		EnableZoomeye: req.Option.EnableZoomeye,
		EnableCensys:  req.Option.EnableCensys,
		EnableShodan:  req.Option.EnableShodan,
		MaxPages:      req.Option.MaxPages,
	}

	// 如果请求中没有提供配置，则使用数据库中的配置
	if option.FofaApi == "" {
		option.FofaApi = spaceConfig.FOFA.API
	}
	if option.FofaEmail == "" {
		option.FofaEmail = spaceConfig.FOFA.Email
	}
	if option.FofaKey == "" {
		option.FofaKey = spaceConfig.FOFA.Key
	}
	if option.HunterKey == "" {
		option.HunterKey = spaceConfig.Hunter.Key
	}
	if option.QuakeKey == "" {
		option.QuakeKey = spaceConfig.Quake.Key
	}
	if option.ZoomeyeApi == "" {
		option.ZoomeyeApi = spaceConfig.Zoomeye.API
	}
	if option.ZoomeyeKey == "" {
		option.ZoomeyeKey = spaceConfig.Zoomeye.Key
	}
	if option.CensysToken == "" {
		option.CensysToken = spaceConfig.Censys.Token
	}
	if option.CensysOrgID == "" {
		option.CensysOrgID = spaceConfig.Censys.OrganizationID
	}
	if option.ShodanApi == "" {
		option.ShodanApi = spaceConfig.Shodan.API
	}
	if option.ShodanKey == "" {
		option.ShodanKey = spaceConfig.Shodan.Key
	}

	// 如果没有指定启用哪些引擎，默认全部启用（如果配置了的话）
	if !option.EnableFofa && !option.EnableHunter && !option.EnableQuake && !option.EnableZoomeye && !option.EnableCensys && !option.EnableShodan {
		// 根据配置自动启用
		if option.FofaApi != "" && option.FofaEmail != "" && option.FofaKey != "" {
			option.EnableFofa = true
		}
		if option.HunterKey != "" {
			option.EnableHunter = true
		}
		if option.QuakeKey != "" {
			option.EnableQuake = true
		}
		if option.ZoomeyeKey != "" {
			option.EnableZoomeye = true
		}
		if option.CensysToken != "" {
			option.EnableCensys = true
		}
		if option.ShodanKey != "" {
			option.EnableShodan = true
		}
	}

	// 检查是否至少启用了一个引擎
	if !option.EnableFofa && !option.EnableHunter && !option.EnableQuake && !option.EnableZoomeye && !option.EnableCensys && !option.EnableShodan {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请至少启用一个空间引擎，或在设置中配置至少一个空间引擎的 API 密钥"})
		return
	}

	if err := ValidateSpaceQueryConstraints(req.Query, &option); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 执行搜索（使用新的适配器模式）
	results := engine.LegacyUncover(req.Query, req.Type, option)

	response := types.SpaceSearchResult{
		Results: results,
		Total:   len(results),
		Message: fmt.Sprintf("共查询到 %d 条结果", len(results)),
	}

	c.JSON(http.StatusOK, response)
}

// SpaceSessionSearchRequest 会话式搜索请求
type SpaceSessionSearchRequest struct {
	Query    string            `json:"query" binding:"required"` // 查询语句
	Type     string            `json:"type"`                     // 查询类型（可选，默认为 default）
	PageSize int               `json:"pageSize"`                 // 每页数量，默认 100
	Option   types.SpaceOption `json:"option"`                   // 空间引擎选项
}

// SpaceSessionResponse 会话搜索响应
type SpaceSessionResponse struct {
	SessionID       string                         `json:"sessionId"`
	Results         []types.SpaceResult            `json:"results"`
	Total           int                            `json:"total"`        // 当前已获取的总数
	HasMore         bool                           `json:"hasMore"`      // 是否还有更多数据
	EngineStatus    map[string]engine.EngineStatus `json:"engineStatus"` // 各引擎状态
	Message         string                         `json:"message"`
	ReachedMaxLimit bool                           `json:"reachedMaxLimit"` // 是否已达到最大限制
	MaxResults      int                            `json:"maxResults"`      // 最大结果数限制
}

// SpaceSessionSearchHandler 创建搜索会话并获取初始数据
func SpaceSessionSearchHandler(c *gin.Context) {
	var req SpaceSessionSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	if err := ValidateSpaceQueryConstraints(req.Query, &req.Option); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 创建聚合器
	aggregator, err := createAggregatorFromRequest(&req.Option)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 转换查询类型
	queryType := engine.QueryTypeDefault
	if req.Type != "" && req.Type != "default" {
		queryType = engine.QueryType(req.Type)
	}

	// 创建会话
	sessionMgr := engine.GetSessionManager()
	session := sessionMgr.CreateSession(aggregator, req.Query, queryType)

	// 获取初始数据
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result := session.FetchInitial(ctx, pageSize)
	if err := persistSpaceSearchHistory(session, platform.Username(c)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存聚合搜索结果失败: " + err.Error()})
		return
	}

	// 转换结果格式
	legacyResults := convertToLegacyResults(result.Results)

	c.JSON(http.StatusOK, SpaceSessionResponse{
		SessionID:       session.ID,
		Results:         legacyResults,
		Total:           result.Total,
		HasMore:         result.HasMore,
		EngineStatus:    result.EngineStatus,
		Message:         fmt.Sprintf("查询到 %d 条结果，%s", result.Total, getHasMoreText(result.HasMore)),
		ReachedMaxLimit: result.ReachedMaxLimit,
		MaxResults:      result.MaxResults,
	})
}

// SpaceFetchMoreRequest 获取更多数据请求
type SpaceFetchMoreRequest struct {
	SessionID string `json:"sessionId" binding:"required"`
	PageSize  int    `json:"pageSize"` // 每页数量，默认 100
}

// SpaceFetchMoreHandler 获取更多数据
func SpaceFetchMoreHandler(c *gin.Context) {
	var req SpaceFetchMoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	sessionMgr := engine.GetSessionManager()
	session := sessionMgr.GetSession(req.SessionID)
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在或已过期"})
		return
	}

	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result := session.FetchMore(ctx, pageSize)
	if err := persistSpaceSearchHistory(session, platform.Username(c)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存聚合搜索结果失败: " + err.Error()})
		return
	}

	// 转换结果格式
	legacyResults := convertToLegacyResults(result.Results)

	c.JSON(http.StatusOK, SpaceSessionResponse{
		SessionID:       session.ID,
		Results:         legacyResults,
		Total:           len(session.GetAllResults()),
		HasMore:         result.HasMore,
		EngineStatus:    result.EngineStatus,
		Message:         fmt.Sprintf("新增 %d 条结果，总计 %d 条", len(result.Results), len(session.GetAllResults())),
		ReachedMaxLimit: result.ReachedMaxLimit,
		MaxResults:      result.MaxResults,
	})
}

// SpaceGetPageRequest 获取指定页数据请求
type SpaceGetPageRequest struct {
	SessionID string `json:"sessionId" binding:"required"`
	Page      int    `json:"page"`     // 页码，从 1 开始
	PageSize  int    `json:"pageSize"` // 每页数量，默认 20
}

// SpaceGetPageResponse 分页响应
type SpaceGetPageResponse struct {
	SessionID    string                         `json:"sessionId"`
	Results      []types.SpaceResult            `json:"results"`
	Total        int                            `json:"total"`      // 总数据量
	Page         int                            `json:"page"`       // 当前页
	PageSize     int                            `json:"pageSize"`   // 每页数量
	TotalPages   int                            `json:"totalPages"` // 总页数
	HasMore      bool                           `json:"hasMore"`    // 是否还能获取更多数据
	EngineStatus map[string]engine.EngineStatus `json:"engineStatus"`
}

// SpaceGetPageHandler 获取指定页数据（从缓存中）
func SpaceGetPageHandler(c *gin.Context) {
	var req SpaceGetPageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	sessionMgr := engine.GetSessionManager()
	session := sessionMgr.GetSession(req.SessionID)
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在或已过期"})
		return
	}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	result := session.GetResultsPage(page, pageSize)

	// 转换结果格式
	legacyResults := convertToLegacyResults(result.Results)

	c.JSON(http.StatusOK, SpaceGetPageResponse{
		SessionID:    session.ID,
		Results:      legacyResults,
		Total:        result.Total,
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   result.TotalPages,
		HasMore:      result.HasMore,
		EngineStatus: session.EngineStatus,
	})
}

// SpaceExportRequest 导出请求
type SpaceExportRequest struct {
	SessionID string `json:"sessionId" binding:"required"`
	FetchAll  bool   `json:"fetchAll"` // 是否获取所有数据后再导出
	MaxPages  int    `json:"maxPages"` // 最大获取页数，默认 50
}

// SpaceExportHandler 导出数据
func SpaceExportHandler(c *gin.Context) {
	var req SpaceExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	sessionMgr := engine.GetSessionManager()
	session := sessionMgr.GetSession(req.SessionID)
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在或已过期"})
		return
	}

	var results []engine.UnifiedResult

	if req.FetchAll {
		// 获取所有数据
		maxPages := req.MaxPages
		if maxPages <= 0 {
			maxPages = 50
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		result := session.FetchAll(ctx, 100, maxPages)
		results = result.Results
		if err := persistSpaceSearchHistory(session, platform.Username(c)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存聚合搜索结果失败: " + err.Error()})
			return
		}
	} else {
		// 只导出已获取的数据
		results = session.GetAllResults()
	}

	// 转换结果格式
	legacyResults := convertToLegacyResults(results)

	c.JSON(http.StatusOK, gin.H{
		"results": legacyResults,
		"total":   len(legacyResults),
		"message": fmt.Sprintf("导出 %d 条结果", len(legacyResults)),
	})
}

// SpaceCloseSessionHandler 关闭会话
func SpaceCloseSessionHandler(c *gin.Context) {
	sessionID := c.Query("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 sessionId 参数"})
		return
	}

	sessionMgr := engine.GetSessionManager()
	sessionMgr.DeleteSession(sessionID)

	c.JSON(http.StatusOK, gin.H{"message": "会话已关闭"})
}

// createAggregatorFromRequest 从请求创建聚合器
func createAggregatorFromRequest(opt *types.SpaceOption) (*engine.Aggregator, error) {
	// 从数据库读取配置
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %v", err)
	}

	// 合并配置
	option := *opt
	if option.FofaApi == "" {
		option.FofaApi = spaceConfig.FOFA.API
	}
	if option.FofaEmail == "" {
		option.FofaEmail = spaceConfig.FOFA.Email
	}
	if option.FofaKey == "" {
		option.FofaKey = spaceConfig.FOFA.Key
	}
	if option.HunterKey == "" {
		option.HunterKey = spaceConfig.Hunter.Key
	}
	if option.QuakeKey == "" {
		option.QuakeKey = spaceConfig.Quake.Key
	}
	if option.ZoomeyeApi == "" {
		option.ZoomeyeApi = spaceConfig.Zoomeye.API
	}
	if option.ZoomeyeKey == "" {
		option.ZoomeyeKey = spaceConfig.Zoomeye.Key
	}
	if option.CensysToken == "" {
		option.CensysToken = spaceConfig.Censys.Token
	}
	if option.CensysOrgID == "" {
		option.CensysOrgID = spaceConfig.Censys.OrganizationID
	}
	if option.ShodanApi == "" {
		option.ShodanApi = spaceConfig.Shodan.API
	}
	if option.ShodanKey == "" {
		option.ShodanKey = spaceConfig.Shodan.Key
	}

	// 如果没有指定启用哪些引擎，默认全部启用
	if !option.EnableFofa && !option.EnableHunter && !option.EnableQuake && !option.EnableZoomeye && !option.EnableCensys && !option.EnableShodan {
		if option.FofaApi != "" && option.FofaEmail != "" && option.FofaKey != "" {
			option.EnableFofa = true
		}
		if option.HunterKey != "" {
			option.EnableHunter = true
		}
		if option.QuakeKey != "" {
			option.EnableQuake = true
		}
		if option.ZoomeyeKey != "" {
			option.EnableZoomeye = true
		}
		if option.CensysToken != "" {
			option.EnableCensys = true
		}
		if option.ShodanKey != "" {
			option.EnableShodan = true
		}
	}

	if !option.EnableFofa && !option.EnableHunter && !option.EnableQuake && !option.EnableZoomeye && !option.EnableCensys && !option.EnableShodan {
		return nil, fmt.Errorf("请至少启用一个空间引擎，或在设置中配置至少一个空间引擎的 API 密钥")
	}

	return engine.NewAggregatorFromSpaceOption(&option)
}

func ValidateSpaceQueryConstraints(query string, opt *types.SpaceOption) error {
	if opt == nil || !opt.EnableHunter {
		return nil
	}

	orCount := len(hunterOrOperatorPattern.FindAllString(query, -1))
	if orCount > 5 {
		return fmt.Errorf("Hunter 最多只支持 5 个 OR 连接符，当前为 %d 个", orCount)
	}

	return nil
}

// convertToLegacyResults 将 UnifiedResult 转换为 SpaceResult
func convertToLegacyResults(results []engine.UnifiedResult) []types.SpaceResult {
	legacyResults := make([]types.SpaceResult, 0, len(results))
	for _, r := range results {
		legacyResults = append(legacyResults, engine.UnifiedToSpaceResult(r))
	}
	return legacyResults
}

// getHasMoreText 获取是否有更多数据的提示文本
func getHasMoreText(hasMore bool) string {
	if hasMore {
		return "可继续加载更多"
	}
	return "已获取全部数据"
}

// EngineQuota 单个引擎的配额信息
type EngineQuota struct {
	Configured bool   `json:"configured"` // 是否已配置
	Username   string `json:"username"`   // 用户名
	// FOFA
	Fcoin           *int `json:"fcoin,omitempty"`           // F币
	FofaPoint       *int `json:"fofaPoint,omitempty"`       // FOFA积分
	RemainFreePoint *int `json:"remainFreePoint,omitempty"` // 剩余免费积分
	RemainApiQuery  *int `json:"remainApiQuery,omitempty"`  // 剩余API查询次数
	// Hunter
	RestEquityPoint *int `json:"restEquityPoint,omitempty"` // 剩余权益积分
	RestFreePoint   *int `json:"restFreePoint,omitempty"`   // 当日剩余免费积分
	DayFreePoint    *int `json:"dayFreePoint,omitempty"`    // 当日免费积分上限
	// Quake
	Credit           *int `json:"credit,omitempty"`           // 永久积分
	PersistentCredit *int `json:"persistentCredit,omitempty"` // 持久积分
	// ZoomEye
	Points        *int `json:"points,omitempty"`
	ZoomeyePoints *int `json:"zoomeyePoints,omitempty"`
	// Shodan
	QueryCredits *int   `json:"queryCredits,omitempty"`
	ScanCredits  *int   `json:"scanCredits,omitempty"`
	Plan         string `json:"plan,omitempty"`
	// 通用
	Error string `json:"error,omitempty"` // 错误信息
}

// SpaceQuotaResponse 配额查询响应
type SpaceQuotaResponse struct {
	Fofa    *EngineQuota `json:"fofa,omitempty"`
	Hunter  *EngineQuota `json:"hunter,omitempty"`
	Quake   *EngineQuota `json:"quake,omitempty"`
	Zoomeye *EngineQuota `json:"zoomeye,omitempty"`
	Censys  *EngineQuota `json:"censys,omitempty"`
	Shodan  *EngineQuota `json:"shodan,omitempty"`
}

// SpaceQuotaHandler 获取各引擎配额信息
func SpaceQuotaHandler(c *gin.Context) {
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加载配置失败: " + err.Error()})
		return
	}

	response := SpaceQuotaResponse{}

	// 并发查询各引擎配额
	type quotaResult struct {
		engine string
		quota  *EngineQuota
	}
	results := make(chan quotaResult, 6)

	// FOFA
	go func() {
		quota := &EngineQuota{Configured: false}
		if spaceConfig.FOFA.API != "" && spaceConfig.FOFA.Email != "" && spaceConfig.FOFA.Key != "" {
			quota.Configured = true
			// 调用 FOFA 用户信息接口
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/info/my?email=%s&key=%s",
				spaceConfig.FOFA.API, spaceConfig.FOFA.Email, spaceConfig.FOFA.Key))
			if err != nil {
				quota.Error = err.Error()
			} else {
				defer resp.Body.Close()
				var userInfo types.FofaUserInfo
				if err := decodeJSON(resp.Body, &userInfo); err == nil && !userInfo.Error {
					quota.Username = userInfo.Username
					quota.Fcoin = intPtr(userInfo.Fcoin)
					quota.FofaPoint = intPtr(userInfo.FofaPoint)
					quota.RemainFreePoint = intPtr(userInfo.RemainFreePoint)
					quota.RemainApiQuery = intPtr(userInfo.RemainApiQuery)
				} else if userInfo.Error {
					quota.Error = userInfo.Message
				}
			}
		}
		results <- quotaResult{engine: "fofa", quota: quota}
	}()

	// Hunter
	go func() {
		quota := &EngineQuota{Configured: false}
		if spaceConfig.Hunter.Key != "" {
			quota.Configured = true
			// 使用配置中的 API 地址，如果没有则使用默认值
			apiBase := spaceConfig.Hunter.API
			if apiBase == "" {
				apiBase = "https://hunter.qianxin.com/"
			}
			if apiBase[len(apiBase)-1] != '/' {
				apiBase += "/"
			}
			apiURL := fmt.Sprintf("%sopenApi/userInfo?api-key=%s", apiBase, spaceConfig.Hunter.Key)
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(apiURL)
			if err != nil {
				quota.Error = err.Error()
			} else {
				defer resp.Body.Close()
				var userInfo types.HunterUserInfo
				if err := decodeJSON(resp.Body, &userInfo); err == nil && userInfo.Code == 200 {
					quota.Username = userInfo.Data.PersonalInfo.Username
					quota.RestEquityPoint = intPtr(userInfo.Data.RestEquityPoint)
					quota.RestFreePoint = intPtr(userInfo.Data.RestFreePoint)
					quota.DayFreePoint = intPtr(userInfo.Data.DayFreePoint)
				} else if err != nil {
					quota.Error = "解析响应失败"
				} else {
					quota.Error = userInfo.Message
				}
			}
		}
		results <- quotaResult{engine: "hunter", quota: quota}
	}()

	// Quake
	go func() {
		quota := &EngineQuota{Configured: false}
		if spaceConfig.Quake.Key != "" {
			quota.Configured = true
			req, _ := http.NewRequest("GET", "https://quake.360.net/api/v3/user/info", nil)
			req.Header.Set("X-QuakeToken", spaceConfig.Quake.Key)
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				quota.Error = err.Error()
			} else {
				defer resp.Body.Close()
				var userInfo types.QuakeUserInfo
				if err := decodeJSON(resp.Body, &userInfo); err == nil {
					quota.Username = userInfo.Data.User.Username
					quota.Credit = intPtr(userInfo.Data.Credit)
					quota.PersistentCredit = intPtr(userInfo.Data.PersistentCredit)
				} else {
					quota.Error = userInfo.Message
				}
			}
		}
		results <- quotaResult{engine: "quake", quota: quota}
	}()

	// Censys（没有积分概念，只检查配置）
	go func() {
		quota := &EngineQuota{Configured: false}
		if spaceConfig.Censys.Token != "" {
			quota.Configured = true
			quota.Username = maskKey(spaceConfig.Censys.Token, 12)
		}
		results <- quotaResult{engine: "censys", quota: quota}
	}()

	// ZoomEye
	go func() {
		quota := &EngineQuota{Configured: false}
		if spaceConfig.Zoomeye.Key != "" {
			quota.Configured = true
			client, err := engine.NewZoomeyeAdapter(&engine.ZoomeyeConfig{
				Address: spaceConfig.Zoomeye.API,
				Key:     spaceConfig.Zoomeye.Key,
			})
			if err != nil {
				quota.Error = err.Error()
			} else {
				info := client.GetUserInfo()
				if info == nil {
					quota.Error = "获取用户信息失败"
				} else {
					quota.Username = info.Username
					if info.Credits > 0 {
						quota.Points = intPtr(int(info.Credits))
					} else {
						zero := 0
						quota.Points = &zero
					}
					extra := int(info.TotalCredits - info.Credits)
					if extra < 0 {
						extra = 0
					}
					quota.ZoomeyePoints = intPtr(extra)
				}
			}
		}
		results <- quotaResult{engine: "zoomeye", quota: quota}
	}()

	// Shodan
	go func() {
		quota := &EngineQuota{Configured: false}
		if spaceConfig.Shodan.Key != "" {
			quota.Configured = true
			client := shodanclient.NewClient(spaceConfig.Shodan.API, spaceConfig.Shodan.Key)
			info := client.APIInfo()
			if info.Error != "" {
				quota.Error = info.Error
			} else {
				quota.Plan = strings.ToUpper(info.Plan)
				quota.QueryCredits = intPtr(info.QueryCredits)
				quota.ScanCredits = intPtr(info.ScanCredits)
			}
		}
		results <- quotaResult{engine: "shodan", quota: quota}
	}()

	// 收集结果
	for i := 0; i < 6; i++ {
		result := <-results
		switch result.engine {
		case "fofa":
			response.Fofa = result.quota
		case "hunter":
			response.Hunter = result.quota
		case "quake":
			response.Quake = result.quota
		case "zoomeye":
			response.Zoomeye = result.quota
		case "censys":
			response.Censys = result.quota
		case "shodan":
			response.Shodan = result.quota
		}
	}

	c.JSON(http.StatusOK, response)
}

// intPtr 返回 int 的指针，用于 JSON 序列化时区分 0 值和未设置
func intPtr(v int) *int {
	return &v
}

func maskKey(v string, keep int) string {
	if v == "" || keep <= 0 {
		return ""
	}
	if len(v) <= keep {
		return v
	}
	return v[:keep] + "..."
}

// decodeJSON 解码 JSON
func decodeJSON(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}
