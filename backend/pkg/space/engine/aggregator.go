package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/types"
)

// AggregatorConfig 聚合器配置
type AggregatorConfig struct {
	MaxPages       int           // 最大查询页数
	Timeout        time.Duration // 单个引擎超时时间
	MaxConcurrency int           // 最大并发数（0表示不限制）
	EnableFofa     bool
	EnableHunter   bool
	EnableQuake    bool
	EnableCensys   bool
	EnableZoomeye  bool
	EnableShodan   bool
	FofaConfig     *FofaConfig
	HunterConfig   *HunterConfig
	QuakeConfig    *QuakeConfig
	CensysConfig   *CensysConfig
	ZoomeyeConfig  *ZoomeyeConfig
	ShodanConfig   *ShodanConfig
}

// Aggregator 空间引擎聚合器
// 负责管理多个引擎，执行聚合搜索
type Aggregator struct {
	registry   *Registry
	transpiler *Transpiler
	config     *AggregatorConfig
	mu         sync.RWMutex
}

// NewAggregator 创建聚合器
func NewAggregator(config *AggregatorConfig) (*Aggregator, error) {
	agg := &Aggregator{
		registry:   NewRegistry(),
		transpiler: DefaultTranspiler,
		config:     config,
	}

	// 注册工厂
	agg.registry.RegisterFactory("fofa", FofaEngineFactory)
	agg.registry.RegisterFactory("hunter", HunterEngineFactory)
	agg.registry.RegisterFactory("quake", QuakeEngineFactory)
	agg.registry.RegisterFactory("censys", CensysEngineFactory)
	agg.registry.RegisterFactory("zoomeye", ZoomeyeEngineFactory)
	agg.registry.RegisterFactory("shodan", ShodanEngineFactory)

	// 初始化引擎
	if config != nil {
		if config.EnableFofa && config.FofaConfig != nil {
			agg.registry.CreateEngine("fofa", config.FofaConfig)
		}
		if config.EnableHunter && config.HunterConfig != nil {
			agg.registry.CreateEngine("hunter", config.HunterConfig)
		}
		if config.EnableQuake && config.QuakeConfig != nil {
			agg.registry.CreateEngine("quake", config.QuakeConfig)
		}
		if config.EnableCensys && config.CensysConfig != nil {
			agg.registry.CreateEngine("censys", config.CensysConfig)
		}
		if config.EnableZoomeye && config.ZoomeyeConfig != nil {
			agg.registry.CreateEngine("zoomeye", config.ZoomeyeConfig)
		}
		if config.EnableShodan && config.ShodanConfig != nil {
			agg.registry.CreateEngine("shodan", config.ShodanConfig)
		}
	}

	return agg, nil
}

// NewAggregatorFromSpaceOption 从 SpaceOption 创建聚合器
func NewAggregatorFromSpaceOption(opt *types.SpaceOption) (*Aggregator, error) {
	config := &AggregatorConfig{
		MaxPages:      opt.MaxPages,
		Timeout:       time.Second * 30,
		EnableFofa:    opt.EnableFofa,
		EnableHunter:  opt.EnableHunter,
		EnableQuake:   opt.EnableQuake,
		EnableCensys:  opt.EnableCensys,
		EnableZoomeye: opt.EnableZoomeye,
		EnableShodan:  opt.EnableShodan,
	}

	if opt.EnableFofa && opt.FofaKey != "" {
		config.FofaConfig = &FofaConfig{
			Address: opt.FofaApi,
			Email:   opt.FofaEmail,
			Key:     opt.FofaKey,
		}
	}

	if opt.EnableHunter && opt.HunterKey != "" {
		config.HunterConfig = &HunterConfig{
			Key: opt.HunterKey,
		}
	}

	if opt.EnableQuake && opt.QuakeKey != "" {
		config.QuakeConfig = &QuakeConfig{
			Token: opt.QuakeKey,
		}
	}

	if opt.EnableCensys && opt.CensysToken != "" {
		config.CensysConfig = &CensysConfig{
			Token:          opt.CensysToken,
			OrganizationID: opt.CensysOrgID,
		}
	}

	if opt.EnableZoomeye && opt.ZoomeyeKey != "" {
		config.ZoomeyeConfig = &ZoomeyeConfig{
			Address: opt.ZoomeyeApi,
			Key:     opt.ZoomeyeKey,
		}
	}

	if opt.EnableShodan && opt.ShodanKey != "" {
		config.ShodanConfig = &ShodanConfig{
			API: opt.ShodanApi,
			Key: opt.ShodanKey,
		}
	}

	return NewAggregator(config)
}

// AggregatedSearchRequest 聚合搜索请求
type AggregatedSearchRequest struct {
	Query     string    // 元语法查询
	QueryType QueryType // 查询类型
	Page      int       // 页码
	PageSize  int       // 每页数量
	Engines   []string  // 指定引擎列表（空表示使用所有已配置的引擎）
}

// AggregatedSearchResponse 聚合搜索响应
type AggregatedSearchResponse struct {
	Results      []UnifiedResult         `json:"results"`
	Total        int                     `json:"total"`
	EngineStatus map[string]EngineStatus `json:"engineStatus"`
	Duration     time.Duration           `json:"duration"`
}

// EngineStatus 单个引擎的状态
type EngineStatus struct {
	Success bool   `json:"success"`
	Total   int    `json:"total"`
	Count   int    `json:"count"` // 返回的结果数
	Message string `json:"message"`
}

// Search 执行聚合搜索（并发查询所有引擎）
func (a *Aggregator) Search(ctx context.Context, req *AggregatedSearchRequest) *AggregatedSearchResponse {
	startTime := time.Now()
	resp := &AggregatedSearchResponse{
		Results:      make([]UnifiedResult, 0),
		EngineStatus: make(map[string]EngineStatus),
	}

	// 获取要使用的引擎
	engines := a.getEngines(req.Engines)
	if len(engines) == 0 {
		logger.Debug("[aggregator] no engines configured")
		return resp
	}

	// 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 100
	}

	// 设置超时上下文
	timeout := time.Second * 30
	if a.config != nil && a.config.Timeout > 0 {
		timeout = a.config.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 并发搜索所有引擎
	var wg sync.WaitGroup
	resultsChan := make(chan *engineSearchResult, len(engines))

	// 使用信号量控制并发数
	var sem chan struct{}
	if a.config != nil && a.config.MaxConcurrency > 0 {
		sem = make(chan struct{}, a.config.MaxConcurrency)
	}

	logger.Info("[aggregator] starting concurrent search across %d engines: query=%s", len(engines), req.Query)

	for _, eng := range engines {
		wg.Add(1)
		go func(engine Engine) {
			defer wg.Done()

			// 获取信号量（如果有限制）
			if sem != nil {
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					resultsChan <- &engineSearchResult{
						EngineName: engine.Name(),
						Status: EngineStatus{
							Success: false,
							Message: "context cancelled while waiting for semaphore",
						},
					}
					return
				}
			}

			result := a.searchSingleEngine(ctx, engine, req)
			resultsChan <- result
		}(eng)
	}

	// 等待所有搜索完成
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// 收集结果（使用互斥锁保护状态写入）
	var statusMu sync.Mutex
	for result := range resultsChan {
		statusMu.Lock()
		resp.EngineStatus[result.EngineName] = result.Status
		if result.Status.Success {
			resp.Results = append(resp.Results, result.Results...)
			resp.Total += result.Status.Total
			logger.Info("[aggregator] %s returned %d results (total: %d)",
				result.EngineName, result.Status.Count, result.Status.Total)
		} else {
			logger.Warning("[aggregator] %s failed: %s", result.EngineName, result.Status.Message)
		}
		statusMu.Unlock()
	}

	resp.Duration = time.Since(startTime)
	logger.Info("[aggregator] search completed in %v, total results: %d", resp.Duration, len(resp.Results))
	return resp
}

// engineSearchResult 单个引擎的搜索结果
type engineSearchResult struct {
	EngineName string
	Results    []UnifiedResult
	Status     EngineStatus
}

// searchSingleEngine 搜索单个引擎（带超时和错误处理）
func (a *Aggregator) searchSingleEngine(ctx context.Context, engine Engine, req *AggregatedSearchRequest) *engineSearchResult {
	result := &engineSearchResult{
		EngineName: engine.Name(),
		Results:    make([]UnifiedResult, 0),
	}

	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		result.Status = EngineStatus{
			Success: false,
			Message: "request timeout or cancelled",
		}
		return result
	default:
	}

	// 转换查询语法
	query, err := a.transpiler.TranslateQueryChecked(req.Query, engine.Name(), req.QueryType)
	if err != nil {
		result.Status = EngineStatus{Success: false, Message: err.Error()}
		return result
	}
	logger.Debug("[aggregator] %s: translated query: %s", engine.Name(), query)

	// 构建搜索请求
	searchReq := &SearchRequest{
		Query:    query,
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	// 使用 channel 实现超时控制
	type searchResult struct {
		resp *SearchResponse
	}
	done := make(chan searchResult, 1)

	go func() {
		resp := engine.Search(searchReq)
		done <- searchResult{resp: resp}
	}()

	// 等待结果或超时
	select {
	case <-ctx.Done():
		result.Status = EngineStatus{
			Success: false,
			Message: "request timeout",
		}
		return result
	case sr := <-done:
		searchResp := sr.resp
		if searchResp.Error != nil {
			message := strings.TrimSpace(searchResp.Message)
			if message == "" {
				message = searchResp.Error.Error()
			}
			if message == "" {
				message = fmt.Sprintf("%s search failed with unknown error", engine.Name())
			}
			result.Status = EngineStatus{
				Success: false,
				Message: message,
			}
			return result
		}

		result.Results = searchResp.Results
		result.Status = EngineStatus{
			Success: true,
			Total:   searchResp.Total,
			Count:   len(searchResp.Results),
			Message: searchResp.Message,
		}
		return result
	}
}

// getEngines 获取要使用的引擎
func (a *Aggregator) getEngines(engineNames []string) []Engine {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(engineNames) == 0 {
		// 返回所有已配置的引擎
		return a.registry.GetConfiguredEngines()
	}

	engines := make([]Engine, 0, len(engineNames))
	for _, name := range engineNames {
		if engine := a.registry.GetEngine(name); engine != nil && engine.IsConfigured() {
			engines = append(engines, engine)
		}
	}
	return engines
}

// GetEngine 获取指定引擎
func (a *Aggregator) GetEngine(name string) Engine {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.registry.GetEngine(name)
}

// GetAllEngines 获取所有引擎
func (a *Aggregator) GetAllEngines() map[string]Engine {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.registry.GetAllEngines()
}

// GetConfiguredEngines 获取所有已配置的引擎
func (a *Aggregator) GetConfiguredEngines() []Engine {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.registry.GetConfiguredEngines()
}

// GetUserInfo 获取所有引擎的用户信息
func (a *Aggregator) GetUserInfo() map[string]*UserInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[string]*UserInfo)
	for name, engine := range a.registry.GetAllEngines() {
		if engine.IsConfigured() {
			result[name] = engine.GetUserInfo()
		}
	}
	return result
}

// TranslateQuery 转换查询语法
func (a *Aggregator) TranslateQuery(query string, targetEngine string, queryType QueryType) string {
	return a.transpiler.TranslateQuery(query, targetEngine, queryType)
}

// MultiPageSearch 多页搜索（并发获取多页）
// 策略：先获取第一页，根据总数决定是否需要获取更多页
func (a *Aggregator) MultiPageSearch(ctx context.Context, req *AggregatedSearchRequest, maxPages int) *AggregatedSearchResponse {
	if maxPages <= 0 {
		maxPages = 1
	}
	if a.config != nil && a.config.MaxPages > 0 && maxPages > a.config.MaxPages {
		maxPages = a.config.MaxPages
	}

	startTime := time.Now()

	// 先获取第一页，了解总数
	req.Page = 1
	firstPageResp := a.Search(ctx, req)

	if maxPages == 1 || len(firstPageResp.Results) < req.PageSize {
		// 只需要一页或第一页已经是最后一页
		firstPageResp.Duration = time.Since(startTime)
		return firstPageResp
	}

	// 计算需要获取的额外页数
	// 估算每个引擎的总页数，取最大值
	estimatedTotalPages := 1
	for _, status := range firstPageResp.EngineStatus {
		if status.Success && status.Total > 0 {
			enginePages := (status.Total + req.PageSize - 1) / req.PageSize
			if enginePages > estimatedTotalPages {
				estimatedTotalPages = enginePages
			}
		}
	}

	// 限制最大页数
	if estimatedTotalPages > maxPages {
		estimatedTotalPages = maxPages
	}

	logger.Info("[aggregator] multi-page search: fetching pages 2-%d concurrently", estimatedTotalPages)

	// 并发获取剩余页
	allResults := &AggregatedSearchResponse{
		Results:      firstPageResp.Results,
		EngineStatus: firstPageResp.EngineStatus,
	}

	if estimatedTotalPages > 1 {
		var wg sync.WaitGroup
		pageResultsChan := make(chan *AggregatedSearchResponse, estimatedTotalPages-1)

		for page := 2; page <= estimatedTotalPages; page++ {
			wg.Add(1)
			go func(p int) {
				defer wg.Done()

				// 检查上下文
				select {
				case <-ctx.Done():
					return
				default:
				}

				pageReq := &AggregatedSearchRequest{
					Query:     req.Query,
					QueryType: req.QueryType,
					Page:      p,
					PageSize:  req.PageSize,
					Engines:   req.Engines,
				}
				pageResp := a.Search(ctx, pageReq)
				pageResultsChan <- pageResp
			}(page)
		}

		// 等待所有页完成
		go func() {
			wg.Wait()
			close(pageResultsChan)
		}()

		// 收集结果
		var statusMu sync.Mutex
		for pageResp := range pageResultsChan {
			statusMu.Lock()
			allResults.Results = append(allResults.Results, pageResp.Results...)

			// 更新引擎状态（累加计数）
			for name, status := range pageResp.EngineStatus {
				if existing, ok := allResults.EngineStatus[name]; ok {
					existing.Total = status.Total // Total 保持不变
					existing.Count += status.Count
					existing.Success = existing.Success && status.Success
					if !status.Success && status.Message != "" {
						existing.Message = status.Message
					}
					allResults.EngineStatus[name] = existing
				}
			}
			statusMu.Unlock()
		}
	}

	allResults.Total = len(allResults.Results)
	allResults.Duration = time.Since(startTime)

	logger.Info("[aggregator] multi-page search completed: %d results in %v", allResults.Total, allResults.Duration)

	return allResults
}

// MergeResults 合并多个引擎的结果并去重
// 优先保留信息更完整的结果
func MergeResults(results []UnifiedResult) []UnifiedResult {
	if len(results) == 0 {
		return results
	}

	// 使用 map 按 IP:Port 分组
	grouped := make(map[string][]UnifiedResult)

	for _, r := range results {
		key := r.IP + ":" + string(rune(r.Port))
		if r.IP == "" && r.Domain != "" {
			key = r.Domain + ":" + string(rune(r.Port))
		}
		grouped[key] = append(grouped[key], r)
	}

	// 对每组选择最完整的结果
	merged := make([]UnifiedResult, 0, len(grouped))
	for _, group := range grouped {
		best := selectBestResult(group)
		merged = append(merged, best)
	}

	return merged
}

// selectBestResult 从一组结果中选择信息最完整的
func selectBestResult(results []UnifiedResult) UnifiedResult {
	if len(results) == 1 {
		return results[0]
	}

	best := results[0]
	bestScore := scoreResult(best)

	for _, r := range results[1:] {
		score := scoreResult(r)
		if score > bestScore {
			best = r
			bestScore = score
		} else if score == bestScore {
			// 分数相同时，合并信息
			best = mergeResultInfo(best, r)
		}
	}

	return best
}

// scoreResult 计算结果的完整度分数
func scoreResult(r UnifiedResult) int {
	score := 0
	if r.IP != "" {
		score += 10
	}
	if r.Domain != "" {
		score += 5
	}
	if r.Title != "" {
		score += 3
	}
	if r.URL != "" {
		score += 3
	}
	if r.Protocol != "" {
		score += 2
	}
	if len(r.Components) > 0 {
		score += len(r.Components)
	}
	if r.Banner != "" {
		score += 2
	}
	if r.Country != "" {
		score += 1
	}
	if r.ICP != "" {
		score += 2
	}
	if r.ICPName != "" {
		score += 1
	}
	return score
}

// mergeResultInfo 合并两个结果的信息
func mergeResultInfo(a, b UnifiedResult) UnifiedResult {
	result := a

	// 补充缺失的字段
	if result.IP == "" && b.IP != "" {
		result.IP = b.IP
	}
	if result.Domain == "" && b.Domain != "" {
		result.Domain = b.Domain
	}
	if result.Title == "" && b.Title != "" {
		result.Title = b.Title
	}
	if result.URL == "" && b.URL != "" {
		result.URL = b.URL
	}
	if result.Protocol == "" && b.Protocol != "" {
		result.Protocol = b.Protocol
	}
	if result.Banner == "" && b.Banner != "" {
		result.Banner = b.Banner
	}
	if result.ICP == "" && b.ICP != "" {
		result.ICP = b.ICP
	}
	if result.ICPName == "" && b.ICPName != "" {
		result.ICPName = b.ICPName
	}
	if result.Country == "" && b.Country != "" {
		result.Country = b.Country
	}

	// 合并组件列表（去重）
	if len(b.Components) > 0 {
		componentSet := make(map[string]bool)
		for _, c := range result.Components {
			componentSet[c] = true
		}
		for _, c := range b.Components {
			if !componentSet[c] {
				result.Components = append(result.Components, c)
				componentSet[c] = true
			}
		}
	}

	// 记录所有来源
	if result.Source != b.Source && b.Source != "" {
		result.Source = result.Source + "," + b.Source
	}

	return result
}

// SearchWithDeduplication 执行搜索并自动去重
func (a *Aggregator) SearchWithDeduplication(ctx context.Context, req *AggregatedSearchRequest) *AggregatedSearchResponse {
	resp := a.Search(ctx, req)
	resp.Results = DeduplicateResults(resp.Results)
	resp.Total = len(resp.Results)
	return resp
}

// SearchWithMerge 执行搜索并合并相同目标的结果
func (a *Aggregator) SearchWithMerge(ctx context.Context, req *AggregatedSearchRequest) *AggregatedSearchResponse {
	resp := a.Search(ctx, req)
	resp.Results = MergeResults(resp.Results)
	resp.Total = len(resp.Results)
	return resp
}
