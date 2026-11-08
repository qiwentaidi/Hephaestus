package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"hephaestus-backend/pkg/logger"
)

// SearchSession 搜索会话
// 用于管理一次搜索的状态，支持按需加载更多数据
type SearchSession struct {
	ID           string                  `json:"id"`
	HistoryID    string                  `json:"historyId"`
	Query        string                  `json:"query"`
	QueryType    QueryType               `json:"queryType"`
	CreatedAt    time.Time               `json:"createdAt"`
	LastAccess   time.Time               `json:"lastAccess"`
	Results      []UnifiedResult         `json:"results"`
	EngineStatus map[string]EngineStatus `json:"engineStatus"`
	// 每个引擎的分页状态
	EnginePages map[string]*EnginePageState `json:"-"`
	// 是否还有更多数据
	HasMore      bool `json:"hasMore"`
	TotalFetched int  `json:"totalFetched"`
	// 内存限制相关
	MaxResults      int  `json:"maxResults"`      // 最大结果数限制
	ReachedMaxLimit bool `json:"reachedMaxLimit"` // 是否已达到最大限制

	mu         sync.RWMutex
	aggregator *Aggregator
}

// 默认配置
const (
	DefaultMaxResultsPerSession = 5000             // 单个会话最大结果数
	DefaultMaxSessions          = 50               // 最大会话数
	DefaultSessionTTL           = 15 * time.Minute // 会话过期时间
)

// EnginePageState 单个引擎的分页状态
type EnginePageState struct {
	CurrentPage int  // 当前已获取的页码
	TotalPages  int  // 估算的总页数
	HasMore     bool // 是否还有更多数据
	LastCount   int  // 上次获取的数量（用于判断是否到末页）
}

// SessionManager 搜索会话管理器
type SessionManager struct {
	sessions    map[string]*SearchSession
	mu          sync.RWMutex
	maxSessions int           // 最大会话数
	sessionTTL  time.Duration // 会话过期时间
}

// NewSessionManager 创建会话管理器
func NewSessionManager(maxSessions int, sessionTTL time.Duration) *SessionManager {
	if maxSessions <= 0 {
		maxSessions = DefaultMaxSessions
	}
	if sessionTTL <= 0 {
		sessionTTL = DefaultSessionTTL
	}

	sm := &SessionManager{
		sessions:    make(map[string]*SearchSession),
		maxSessions: maxSessions,
		sessionTTL:  sessionTTL,
	}

	// 启动清理协程
	go sm.cleanupLoop()

	return sm
}

// 全局会话管理器
var globalSessionManager *SessionManager
var sessionManagerOnce sync.Once

// GetSessionManager 获取全局会话管理器
func GetSessionManager() *SessionManager {
	sessionManagerOnce.Do(func() {
		globalSessionManager = NewSessionManager(DefaultMaxSessions, DefaultSessionTTL)
	})
	return globalSessionManager
}

// CreateSession 创建新的搜索会话
func (sm *SessionManager) CreateSession(aggregator *Aggregator, query string, queryType QueryType) *SearchSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 检查是否需要清理旧会话
	if len(sm.sessions) >= sm.maxSessions {
		sm.evictOldest()
	}

	session := &SearchSession{
		ID:           generateSessionID(),
		HistoryID:    "",
		Query:        query,
		QueryType:    queryType,
		CreatedAt:    time.Now(),
		LastAccess:   time.Now(),
		Results:      make([]UnifiedResult, 0),
		EngineStatus: make(map[string]EngineStatus),
		EnginePages:  make(map[string]*EnginePageState),
		HasMore:      true,
		MaxResults:   DefaultMaxResultsPerSession,
		aggregator:   aggregator,
	}

	// 初始化每个引擎的分页状态
	for name := range aggregator.GetAllEngines() {
		if engine := aggregator.GetEngine(name); engine != nil && engine.IsConfigured() {
			session.EnginePages[name] = &EnginePageState{
				CurrentPage: 0,
				HasMore:     true,
			}
		}
	}

	sm.sessions[session.ID] = session
	logger.Info("[session] created new session: %s, query: %s", session.ID, query)

	return session
}

// GetSession 获取会话
func (sm *SessionManager) GetSession(sessionID string) *SearchSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, ok := sm.sessions[sessionID]
	if !ok {
		return nil
	}

	session.LastAccess = time.Now()
	return session
}

// DeleteSession 删除会话
func (sm *SessionManager) DeleteSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionID)
}

// evictOldest 删除最旧的会话
func (sm *SessionManager) evictOldest() {
	var oldestID string
	var oldestTime time.Time

	for id, session := range sm.sessions {
		if oldestID == "" || session.LastAccess.Before(oldestTime) {
			oldestID = id
			oldestTime = session.LastAccess
		}
	}

	if oldestID != "" {
		delete(sm.sessions, oldestID)
		logger.Debug("[session] evicted oldest session: %s", oldestID)
	}
}

// cleanupLoop 定期清理过期会话
func (sm *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sm.cleanup()
	}
}

// cleanup 清理过期会话
func (sm *SessionManager) cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for id, session := range sm.sessions {
		if now.Sub(session.LastAccess) > sm.sessionTTL {
			delete(sm.sessions, id)
			logger.Debug("[session] cleaned up expired session: %s", id)
		}
	}
}

// generateSessionID 生成会话ID
func generateSessionID() string {
	return fmt.Sprintf("ss_%d_%d", time.Now().UnixNano(), time.Now().UnixNano()%1000)
}

// FetchInitial 获取初始数据（第一页）
func (s *SearchSession) FetchInitial(ctx context.Context, pageSize int) *SessionFetchResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	if pageSize <= 0 {
		pageSize = 100
	}

	req := &AggregatedSearchRequest{
		Query:     s.Query,
		QueryType: s.QueryType,
		Page:      1,
		PageSize:  pageSize,
	}

	resp := s.aggregator.Search(ctx, req)

	// 更新会话状态（同一端点可能被多个引擎返回，只保留一条）
	s.Results = DeduplicateResults(resp.Results)
	s.EngineStatus = resp.EngineStatus
	s.TotalFetched = len(resp.Results)
	s.LastAccess = time.Now()

	// 更新每个引擎的分页状态
	s.HasMore = false
	for name, status := range resp.EngineStatus {
		if pageState, ok := s.EnginePages[name]; ok {
			pageState.CurrentPage = 1
			pageState.LastCount = status.Count

			// 估算总页数
			if status.Total > 0 {
				pageState.TotalPages = (status.Total + pageSize - 1) / pageSize
				pageState.HasMore = status.Count >= pageSize && pageState.CurrentPage < pageState.TotalPages
			} else {
				pageState.HasMore = status.Count >= pageSize
			}

			if pageState.HasMore {
				s.HasMore = true
			}
		}
	}

	return &SessionFetchResult{
		Results:         s.Results,
		Total:           len(s.Results),
		HasMore:         s.HasMore,
		EngineStatus:    s.EngineStatus,
		ReachedMaxLimit: s.ReachedMaxLimit,
		MaxResults:      s.MaxResults,
	}
}

// FetchMore 获取更多数据
func (s *SearchSession) FetchMore(ctx context.Context, pageSize int) *SessionFetchResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已达到最大结果数量限制
	if s.ReachedMaxLimit || len(s.Results) >= s.MaxResults {
		s.ReachedMaxLimit = true
		s.HasMore = false
		return &SessionFetchResult{
			Results:         nil,
			Total:           0,
			HasMore:         false,
			EngineStatus:    s.EngineStatus,
			ReachedMaxLimit: true,
			MaxResults:      s.MaxResults,
		}
	}

	if !s.HasMore {
		return &SessionFetchResult{
			Results:      nil,
			Total:        0,
			HasMore:      false,
			EngineStatus: s.EngineStatus,
		}
	}

	if pageSize <= 0 {
		pageSize = 100
	}

	// 找出还有更多数据的引擎
	enginesWithMore := make([]string, 0)
	for name, pageState := range s.EnginePages {
		if pageState.HasMore {
			enginesWithMore = append(enginesWithMore, name)
		}
	}

	if len(enginesWithMore) == 0 {
		s.HasMore = false
		return &SessionFetchResult{
			Results:      nil,
			Total:        0,
			HasMore:      false,
			EngineStatus: s.EngineStatus,
		}
	}

	// 为每个引擎获取下一页
	var wg sync.WaitGroup
	resultsChan := make(chan *engineFetchResult, len(enginesWithMore))

	for _, engineName := range enginesWithMore {
		pageState := s.EnginePages[engineName]
		nextPage := pageState.CurrentPage + 1

		wg.Add(1)
		go func(name string, page int) {
			defer wg.Done()

			engine := s.aggregator.GetEngine(name)
			if engine == nil {
				return
			}

			// 转换查询语法
			query, err := s.aggregator.transpiler.TranslateQueryChecked(s.Query, name, s.QueryType)
			if err != nil {
				resultsChan <- &engineFetchResult{EngineName: name, Page: page, Error: err}
				return
			}

			searchReq := &SearchRequest{
				Query:    query,
				Page:     page,
				PageSize: pageSize,
			}

			resp := engine.Search(searchReq)
			resultsChan <- &engineFetchResult{
				EngineName: name,
				Page:       page,
				Results:    resp.Results,
				Total:      resp.Total,
				Count:      len(resp.Results),
				Error:      resp.Error,
			}
		}(engineName, nextPage)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// 收集结果
	newResults := make([]UnifiedResult, 0)
	s.HasMore = false

	for result := range resultsChan {
		if result.Error != nil {
			logger.Warning("[session] %s page %d failed: %v", result.EngineName, result.Page, result.Error)
			continue
		}

		newResults = append(newResults, result.Results...)

		// 更新引擎分页状态
		if pageState, ok := s.EnginePages[result.EngineName]; ok {
			pageState.CurrentPage = result.Page
			pageState.LastCount = result.Count

			if result.Total > 0 {
				pageState.TotalPages = (result.Total + pageSize - 1) / pageSize
				pageState.HasMore = pageState.CurrentPage < pageState.TotalPages
			} else {
				pageState.HasMore = result.Count >= pageSize
			}

			if pageState.HasMore {
				s.HasMore = true
			}

			// 更新引擎状态
			if status, ok := s.EngineStatus[result.EngineName]; ok {
				if result.Total > 0 {
					status.Total = result.Total
				}
				status.Count += result.Count
				s.EngineStatus[result.EngineName] = status
			}
		}
	}

	// 跨页、跨引擎去重：已缓存的结果不再重复追加
	newResults = DeduplicateAgainst(s.Results, newResults)

	// 追加到结果集（检查内存限制）
	remainingCapacity := s.MaxResults - len(s.Results)
	if remainingCapacity <= 0 {
		s.ReachedMaxLimit = true
		s.HasMore = false
		return &SessionFetchResult{
			Results:         nil,
			Total:           0,
			HasMore:         false,
			EngineStatus:    s.EngineStatus,
			ReachedMaxLimit: true,
			MaxResults:      s.MaxResults,
		}
	}

	// 限制追加的结果数量
	if len(newResults) > remainingCapacity {
		newResults = newResults[:remainingCapacity]
		s.ReachedMaxLimit = true
		s.HasMore = false
	}

	s.Results = append(s.Results, newResults...)
	s.TotalFetched = len(s.Results)
	s.LastAccess = time.Now()

	return &SessionFetchResult{
		Results:         newResults,
		Total:           len(newResults),
		HasMore:         s.HasMore,
		EngineStatus:    s.EngineStatus,
		ReachedMaxLimit: s.ReachedMaxLimit,
		MaxResults:      s.MaxResults,
	}
}

// GetAllResults 获取所有已缓存的结果
func (s *SearchSession) GetAllResults() []UnifiedResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Results
}

// GetResultsPage 获取指定页的结果（从缓存中）
func (s *SearchSession) GetResultsPage(page, pageSize int) *SessionFetchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	start := (page - 1) * pageSize
	end := start + pageSize

	if start >= len(s.Results) {
		return &SessionFetchResult{
			Results:         nil,
			Total:           len(s.Results),
			HasMore:         s.HasMore,
			Page:            page,
			PageSize:        pageSize,
			TotalPages:      (len(s.Results) + pageSize - 1) / pageSize,
			ReachedMaxLimit: s.ReachedMaxLimit,
			MaxResults:      s.MaxResults,
		}
	}

	if end > len(s.Results) {
		end = len(s.Results)
	}

	return &SessionFetchResult{
		Results:         s.Results[start:end],
		Total:           len(s.Results),
		HasMore:         s.HasMore,
		Page:            page,
		PageSize:        pageSize,
		TotalPages:      (len(s.Results) + pageSize - 1) / pageSize,
		ReachedMaxLimit: s.ReachedMaxLimit,
		MaxResults:      s.MaxResults,
	}
}

// SessionFetchResult 会话获取结果
type SessionFetchResult struct {
	Results         []UnifiedResult         `json:"results"`
	Total           int                     `json:"total"`           // 已获取的总数
	HasMore         bool                    `json:"hasMore"`         // 是否还有更多数据
	Page            int                     `json:"page"`            // 当前页码（用于分页展示）
	PageSize        int                     `json:"pageSize"`        // 每页数量
	TotalPages      int                     `json:"totalPages"`      // 总页数（基于已获取的数据）
	EngineStatus    map[string]EngineStatus `json:"engineStatus"`    // 引擎状态
	ReachedMaxLimit bool                    `json:"reachedMaxLimit"` // 是否已达到最大限制
	MaxResults      int                     `json:"maxResults"`      // 最大结果数限制
}

// engineFetchResult 内部引擎获取结果
type engineFetchResult struct {
	EngineName string
	Page       int
	Results    []UnifiedResult
	Total      int
	Count      int
	Error      error
}

// FetchAll 获取所有数据（用于导出）
func (s *SearchSession) FetchAll(ctx context.Context, pageSize int, maxPages int) *SessionFetchResult {
	if pageSize <= 0 {
		pageSize = 100
	}
	if maxPages <= 0 {
		maxPages = 50 // 默认最多50页
	}

	// 如果还没有初始数据，先获取
	if len(s.Results) == 0 {
		s.FetchInitial(ctx, pageSize)
	}

	// 持续获取直到没有更多数据、达到最大页数或达到内存限制
	fetchedPages := 1
	for s.HasMore && fetchedPages < maxPages && !s.ReachedMaxLimit {
		select {
		case <-ctx.Done():
			logger.Info("[session] fetch all cancelled: %s", s.ID)
			break
		default:
		}

		result := s.FetchMore(ctx, pageSize)
		if len(result.Results) == 0 || result.ReachedMaxLimit {
			break
		}
		fetchedPages++
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return &SessionFetchResult{
		Results:         s.Results,
		Total:           len(s.Results),
		HasMore:         s.HasMore,
		EngineStatus:    s.EngineStatus,
		ReachedMaxLimit: s.ReachedMaxLimit,
		MaxResults:      s.MaxResults,
	}
}
