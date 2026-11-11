package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/nanoid"
	"hephaestus-backend/pkg/space/engine"
	"hephaestus-backend/pkg/toolguard"
	"hephaestus-backend/pkg/types"
	"hephaestus-backend/pkg/version"

	"github.com/gin-gonic/gin"
)

var hunterOrOperatorPattern = regexp.MustCompile(`(?i)\bOR\b`)

const (
	mcpProtocolVersion       = "2025-06-18"
	mcpSessionTTL            = 30 * time.Minute
	mcpSearchDefaultPageSize = 20
	mcpSearchMaxPageSize     = 100
	mcpSearchMaxPages        = 5
)

const (
	mcpCodeParseError     = -32700
	mcpCodeInvalidRequest = -32600
	mcpCodeMethodNotFound = -32601
	mcpCodeInvalidParams  = -32602
	mcpCodeInternalError  = -32603
)

type mcpServerProfile struct {
	Key          string
	ServerName   string
	AuditServer  string
	Instructions string
}

var (
	mcpSpaceEngineProfile = mcpServerProfile{
		Key:          "space-engine",
		ServerName:   "hephaestus-space-engine-mcp",
		AuditServer:  "hephaestus-space-engine",
		Instructions: "Use the provided space engine tools for multi-engine search and runtime status inspection.",
	}
	mcpHephaestusProfile = mcpServerProfile{
		Key:          "hephaestus",
		ServerName:   "hephaestus-mcp",
		AuditServer:  "hephaestus",
		Instructions: "Use the Hephaestus tools for security platform workflows including space engine search, attack surface assessment scan tasks, subdomain discovery, and company intelligence tasks.",
	}
)

type mcpJSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpJSONRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      interface{}      `json:"id,omitempty"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *mcpJSONRPCError `json:"error,omitempty"`
}

type mcpJSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type mcpTool struct {
	Name            string                            `json:"name"`
	ModuleKey       string                            `json:"moduleKey,omitempty"`
	ModuleName      string                            `json:"moduleName,omitempty"`
	ModuleNameI18n  map[string]string                 `json:"moduleNameI18n,omitempty"`
	Title           string                            `json:"title,omitempty"`
	TitleI18n       map[string]string                 `json:"titleI18n,omitempty"`
	Version         string                            `json:"version,omitempty"`
	Description     string                            `json:"description,omitempty"`
	DescriptionI18n map[string]string                 `json:"descriptionI18n,omitempty"`
	Permission      string                            `json:"permission,omitempty"`
	Annotations     map[string]interface{}            `json:"annotations,omitempty"`
	InputSchema     map[string]interface{}            `json:"inputSchema"`
	InputSchemaI18n map[string]map[string]interface{} `json:"inputSchemaI18n,omitempty"`
}

type mcpToolHandler func(*gin.Context, json.RawMessage) (mcpToolCallResult, error)

type mcpToolDefinition struct {
	Tool    mcpTool
	Handler mcpToolHandler
}

type mcpToolTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolCallResult struct {
	Content           []mcpToolTextContent `json:"content"`
	StructuredContent interface{}          `json:"structuredContent,omitempty"`
	IsError           bool                 `json:"isError,omitempty"`
	Blocked           bool                 `json:"blocked,omitempty"`
}

type mcpSessionState struct {
	ProtocolVersion string
	Initialized     bool
	ClientName      string
	ClientVersion   string
	UpdatedAt       time.Time
}

type mcpSpaceSearchArgs struct {
	Query     string   `json:"query"`
	QueryType string   `json:"queryType"`
	Engines   []string `json:"engines"`
	Page      int      `json:"page"`
	PageSize  int      `json:"pageSize"`
	MaxPages  int      `json:"maxPages"`
}

var mcpSessions = struct {
	sync.Mutex
	items map[string]*mcpSessionState
}{
	items: make(map[string]*mcpSessionState),
}

func MCPToolSpaceEngineHandler(c *gin.Context) {
	handleMCP(c, mcpSpaceEngineProfile)
}

func MCPToolHephaestusHandler(c *gin.Context) {
	handleMCP(c, mcpHephaestusProfile)
}

func handleMCP(c *gin.Context, profile mcpServerProfile) {
	audit := startMCPAudit(c, profile.AuditServer)
	defer finishMCPAudit(c, audit)

	switch c.Request.Method {
	case http.MethodPost:
		handleMCPPost(c, profile)
	case http.MethodDelete:
		handleMCPDelete(c)
	default:
		markMCPAuditFailure(c, http.StatusMethodNotAllowed, "仅支持 POST 和 DELETE")
		c.Header("Allow", "POST, DELETE")
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"error": "仅支持 POST 和 DELETE",
		})
	}
}

func handleMCPPost(c *gin.Context, profile mcpServerProfile) {
	var req mcpJSONRPCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		markMCPAuditRequest(c, "parse-error")
		writeMCPError(c, nil, mcpCodeParseError, "无效的 JSON-RPC 请求", err.Error())
		return
	}
	markMCPAuditRequest(c, req.Method)

	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		writeMCPError(c, req.ID, mcpCodeInvalidRequest, "无效的 JSON-RPC 请求", nil)
		return
	}

	if req.Method != "initialize" {
		sessionID, session, err := requireMCPSession(c)
		if err != nil {
			writeMCPError(c, req.ID, mcpCodeInvalidRequest, err.Error(), nil)
			return
		}
		markMCPAuditSession(c, sessionID, session)
		if protocolHeader := strings.TrimSpace(c.GetHeader("MCP-Protocol-Version")); protocolHeader != "" && protocolHeader != session.ProtocolVersion {
			writeMCPError(c, req.ID, mcpCodeInvalidRequest, "MCP 协议版本不匹配", gin.H{
				"expected": session.ProtocolVersion,
				"actual":   protocolHeader,
			})
			return
		}
		touchMCPSession(sessionID)
	}

	switch req.Method {
	case "initialize":
		handleMCPInitialize(c, &req, profile)
	case "notifications/initialized":
		handleMCPInitialized(c)
	case "ping":
		writeMCPResult(c, req.ID, gin.H{})
	case "tools/list":
		if !ensureMCPInitialized(c, req.ID) {
			return
		}
		writeMCPResult(c, req.ID, gin.H{
			"tools": mcpToolsForProfile(profile),
		})
	case "tools/call":
		if !ensureMCPInitialized(c, req.ID) {
			return
		}
		handleMCPToolCall(c, &req, profile)
	default:
		writeMCPError(c, req.ID, mcpCodeMethodNotFound, "不支持的 MCP 方法", req.Method)
	}
}

func handleMCPDelete(c *gin.Context) {
	markMCPAuditRequest(c, "session/delete")
	sessionID := strings.TrimSpace(c.GetHeader("Mcp-Session-Id"))
	if sessionID == "" {
		c.Status(http.StatusNoContent)
		return
	}

	if session := getMCPSession(sessionID); session != nil {
		markMCPAuditSession(c, sessionID, session)
	}
	deleteMCPSession(sessionID)
	c.Status(http.StatusNoContent)
}

func handleMCPInitialize(c *gin.Context, req *mcpJSONRPCRequest, profile mcpServerProfile) {
	var params struct {
		ProtocolVersion string                 `json:"protocolVersion"`
		ClientInfo      map[string]interface{} `json:"clientInfo"`
	}
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeMCPError(c, req.ID, mcpCodeInvalidParams, "initialize 参数无效", err.Error())
			return
		}
	}

	clientName, clientVersion := parseMCPClientInfo(params.ClientInfo)
	markMCPAuditClient(c, clientName, clientVersion)

	sessionID := createMCPSession(mcpProtocolVersion, clientName, clientVersion)
	markMCPAuditSession(c, sessionID, &mcpSessionState{
		ProtocolVersion: mcpProtocolVersion,
		ClientName:      clientName,
		ClientVersion:   clientVersion,
	})
	c.Header("Mcp-Session-Id", sessionID)

	writeMCPResult(c, req.ID, gin.H{
		"protocolVersion": mcpProtocolVersion,
		"capabilities": gin.H{
			"tools": gin.H{
				"listChanged": false,
			},
		},
		"serverInfo": gin.H{
			"name":    profile.ServerName,
			"version": version.Version,
		},
		"instructions": profile.Instructions,
	})
}

func handleMCPInitialized(c *gin.Context) {
	sessionID, session, err := requireMCPSession(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session.Initialized = true
	session.UpdatedAt = time.Now()
	storeMCPSession(sessionID, session)
	c.Status(http.StatusAccepted)
}

func handleMCPToolCall(c *gin.Context, req *mcpJSONRPCRequest, profile mcpServerProfile) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeMCPError(c, req.ID, mcpCodeInvalidParams, "tools/call 参数无效", err.Error())
		return
	}

	toolName := strings.TrimSpace(params.Name)
	markMCPAuditTool(c, toolName)

	result, err := callMCPTool(c, toolName, params.Arguments, profile)
	if err != nil {
		markMCPAuditFailure(c, 0, err.Error())
		writeMCPResult(c, req.ID, mcpToolCallResult{
			Content: []mcpToolTextContent{{
				Type: "text",
				Text: err.Error(),
			}},
			IsError: true,
		})
		return
	}

	writeMCPResult(c, req.ID, result)
}

func callMCPTool(c *gin.Context, name string, rawArgs json.RawMessage, profile mcpServerProfile) (mcpToolCallResult, error) {
	for _, tool := range mcpToolDefinitionsForProfile(profile) {
		if tool.Tool.Name == name {
			if err := authorizeMCPToolCall(c, tool.Tool); err != nil {
				return mcpToolCallResult{}, err
			}
			if match := toolguard.CheckToolCall(name, rawArgs); match != nil {
				message := toolguard.FormatBlockedMessage(match)
				return mcpToolCallResult{
					Content: []mcpToolTextContent{{Type: "text", Text: message}},
					StructuredContent: gin.H{
						"blocked": true,
						"match":   match,
					},
					IsError: true,
					Blocked: true,
				}, nil
			}
			return tool.Handler(c, rawArgs)
		}
	}
	return mcpToolCallResult{}, fmt.Errorf("不支持的工具: %s", name)
}

func authorizeMCPToolCall(c *gin.Context, tool mcpTool) error {
	permission := strings.ToLower(strings.TrimSpace(tool.Permission))
	if permission == "" {
		permission = "read"
	}

	role := strings.ToLower(strings.TrimSpace(resolveMCPUserRole(c)))
	if role == "" {
		role = "user"
	}

	switch permission {
	case "read", "write":
		return nil
	case "search":
		if role == "admin" {
			return nil
		}
		return fmt.Errorf("MCP 工具 %s 需要 admin 角色", tool.Name)
	case "admin":
		if role == "admin" {
			return nil
		}
		return fmt.Errorf("MCP 工具 %s 的 %s 权限未授权", tool.Name, permission)
	default:
		return fmt.Errorf("MCP 工具 %s 使用了未知权限级别: %s", tool.Name, permission)
	}
}

func resolveMCPUserRole(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if role := strings.TrimSpace(c.GetString("role")); role != "" {
		return role
	}

	username := strings.TrimSpace(c.GetString("username"))
	if username == "" {
		return ""
	}

	db, err := database.GetDB()
	if err != nil {
		return ""
	}
	var user database.User
	if err := db.Select("role").Where("username = ?", username).First(&user).Error; err != nil {
		return ""
	}
	return user.Role
}

func mcpCallSpaceEngineSearch(c *gin.Context, args mcpSpaceSearchArgs) (mcpToolCallResult, error) {
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return mcpToolCallResult{}, fmt.Errorf("query 不能为空")
	}

	option := buildSpaceOptionForEngines(args.Engines, args.MaxPages)
	if err := validateSpaceQueryConstraints(args.Query, &option); err != nil {
		return mcpToolCallResult{}, err
	}

	aggregator, err := createAggregatorFromRequest(&option)
	if err != nil {
		return mcpToolCallResult{}, err
	}

	page := args.Page
	if page <= 0 {
		page = 1
	}
	pageSize := args.PageSize
	if pageSize <= 0 {
		pageSize = mcpSearchDefaultPageSize
	}
	if pageSize > mcpSearchMaxPageSize {
		return mcpToolCallResult{}, fmt.Errorf("pageSize 不能超过 %d", mcpSearchMaxPageSize)
	}

	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}

	resp := aggregator.Search(ctx, &engine.AggregatedSearchRequest{
		Query:     args.Query,
		QueryType: engine.QueryTypeFromString(args.QueryType),
		Page:      page,
		PageSize:  pageSize,
		Engines:   normalizeSpaceEngineNames(args.Engines),
	})

	payload := gin.H{
		"query":        args.Query,
		"queryType":    string(engine.QueryTypeFromString(args.QueryType)),
		"page":         page,
		"pageSize":     pageSize,
		"total":        resp.Total,
		"results":      convertToLegacyResults(resp.Results),
		"engineStatus": resp.EngineStatus,
		"durationMs":   resp.Duration.Milliseconds(),
	}
	return newMCPToolResult(payload), nil
}

func mcpCallSpaceEngineQuota() (mcpToolCallResult, error) {
	quota, err := buildSpaceQuotaResponse()
	if err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(quota), nil
}

func buildSpaceQuotaResponse() (*SpaceQuotaResponse, error) {
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	response := &SpaceQuotaResponse{}
	type quotaResult struct {
		engine string
		quota  *EngineQuota
	}

	results := make(chan quotaResult, 5)

	go func() {
		quota := &EngineQuota{Configured: false}
		if spaceConfig.FOFA.API != "" && spaceConfig.FOFA.Email != "" && spaceConfig.FOFA.Key != "" {
			quota.Configured = true
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

	go func() {
		quota := &EngineQuota{Configured: false}
		if spaceConfig.Hunter.Key != "" {
			quota.Configured = true
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

	go func() {
		quota := &EngineQuota{Configured: false}
		if spaceConfig.Quake.Key != "" {
			quota.Configured = true
			req, _ := http.NewRequest(http.MethodGet, "https://quake.360.net/api/v3/user/info", nil)
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

	go func() {
		quota := &EngineQuota{Configured: false}
		if spaceConfig.Censys.Token != "" {
			quota.Configured = true
			quota.Username = maskKey(spaceConfig.Censys.Token, 12)
		}
		results <- quotaResult{engine: "censys", quota: quota}
	}()

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

	for i := 0; i < 5; i++ {
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
		}
	}

	return response, nil
}

func mcpCallSpaceEngineConfigStatus() (mcpToolCallResult, error) {
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		return mcpToolCallResult{}, fmt.Errorf("加载配置失败: %w", err)
	}

	payload := gin.H{
		"fofa": gin.H{
			"configured": spaceConfig.FOFA.Email != "" && spaceConfig.FOFA.Key != "",
			"api":        strings.TrimSpace(spaceConfig.FOFA.API) != "",
			"email":      strings.TrimSpace(spaceConfig.FOFA.Email) != "",
			"key":        strings.TrimSpace(spaceConfig.FOFA.Key) != "",
		},
		"hunter": gin.H{
			"configured": spaceConfig.Hunter.Key != "",
			"api":        strings.TrimSpace(spaceConfig.Hunter.API) != "",
			"key":        strings.TrimSpace(spaceConfig.Hunter.Key) != "",
		},
		"quake": gin.H{
			"configured": spaceConfig.Quake.Key != "",
			"key":        strings.TrimSpace(spaceConfig.Quake.Key) != "",
		},
		"censys": gin.H{
			"configured": spaceConfig.Censys.Token != "",
			"token":      strings.TrimSpace(spaceConfig.Censys.Token) != "",
			"orgId":      strings.TrimSpace(spaceConfig.Censys.OrganizationID) != "",
		},
		"zoomeye": gin.H{
			"configured": spaceConfig.Zoomeye.Key != "",
			"api":        strings.TrimSpace(spaceConfig.Zoomeye.API) != "",
			"key":        strings.TrimSpace(spaceConfig.Zoomeye.Key) != "",
		},
		"shodan": gin.H{
			"configured": spaceConfig.Shodan.Key != "",
			"api":        strings.TrimSpace(spaceConfig.Shodan.API) != "",
			"key":        strings.TrimSpace(spaceConfig.Shodan.Key) != "",
		},
		"github": gin.H{
			"configured": spaceConfig.Github.Token != "",
		},
		"chaos": gin.H{
			"configured": spaceConfig.Chaos.Key != "",
		},
		"bevigil": gin.H{
			"configured": spaceConfig.Bevigil.Key != "",
		},
		"securitytrails": gin.H{
			"configured": spaceConfig.Securitytrails.Key != "",
		},
		"ctRadar": gin.H{
			"configured": spaceConfig.CTRadar.Key != "",
		},
	}
	return newMCPToolResult(payload), nil
}

func decodeMCPToolArgs(raw json.RawMessage, dest interface{}) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("arguments 无效: %w", err)
	}
	return nil
}

func newMCPToolResult(payload interface{}) mcpToolCallResult {
	return mcpToolCallResult{
		Content: []mcpToolTextContent{{
			Type: "text",
			Text: marshalMCPText(payload),
		}},
		StructuredContent: payload,
	}
}

func marshalMCPText(payload interface{}) string {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", payload)
	}
	return string(data)
}

func buildSpaceOptionForEngines(engines []string, maxPages int) types.SpaceOption {
	option := types.SpaceOption{}
	if maxPages > mcpSearchMaxPages {
		maxPages = mcpSearchMaxPages
	}
	option.MaxPages = maxPages

	normalized := normalizeSpaceEngineNames(engines)
	for _, name := range normalized {
		switch name {
		case "fofa":
			option.EnableFofa = true
		case "hunter":
			option.EnableHunter = true
		case "quake":
			option.EnableQuake = true
		case "censys":
			option.EnableCensys = true
		case "zoomeye":
			option.EnableZoomeye = true
		case "shodan":
			option.EnableShodan = true
		}
	}

	return option
}

func createAggregatorFromRequest(opt *types.SpaceOption) (*engine.Aggregator, error) {
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %v", err)
	}

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
	if option.ShodanKey == "" {
		option.ShodanKey = spaceConfig.Shodan.Key
	}
	if option.ShodanApi == "" {
		option.ShodanApi = spaceConfig.Shodan.API
	}

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

func validateSpaceQueryConstraints(query string, opt *types.SpaceOption) error {
	if opt == nil || !opt.EnableHunter {
		return nil
	}

	orCount := len(hunterOrOperatorPattern.FindAllString(query, -1))
	if orCount > 5 {
		return fmt.Errorf("Hunter 最多只支持 5 个 OR 连接符，当前为 %d 个", orCount)
	}

	return nil
}

func convertToLegacyResults(results []engine.UnifiedResult) []types.SpaceResult {
	legacyResults := make([]types.SpaceResult, 0, len(results))
	for _, result := range results {
		legacyResults = append(legacyResults, engine.UnifiedToSpaceResult(result))
	}
	return legacyResults
}

type EngineQuota struct {
	Configured       bool   `json:"configured"`
	Username         string `json:"username"`
	Fcoin            *int   `json:"fcoin,omitempty"`
	FofaPoint        *int   `json:"fofaPoint,omitempty"`
	RemainFreePoint  *int   `json:"remainFreePoint,omitempty"`
	RemainApiQuery   *int   `json:"remainApiQuery,omitempty"`
	RestEquityPoint  *int   `json:"restEquityPoint,omitempty"`
	RestFreePoint    *int   `json:"restFreePoint,omitempty"`
	DayFreePoint     *int   `json:"dayFreePoint,omitempty"`
	Credit           *int   `json:"credit,omitempty"`
	PersistentCredit *int   `json:"persistentCredit,omitempty"`
	Points           *int   `json:"points,omitempty"`
	ZoomeyePoints    *int   `json:"zoomeyePoints,omitempty"`
	Error            string `json:"error,omitempty"`
}

type SpaceQuotaResponse struct {
	Fofa    *EngineQuota `json:"fofa,omitempty"`
	Hunter  *EngineQuota `json:"hunter,omitempty"`
	Quake   *EngineQuota `json:"quake,omitempty"`
	Zoomeye *EngineQuota `json:"zoomeye,omitempty"`
	Censys  *EngineQuota `json:"censys,omitempty"`
}

func normalizeSpaceEngineNames(engines []string) []string {
	if len(engines) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(engines))
	normalized := make([]string, 0, len(engines))
	for _, item := range engines {
		name := strings.TrimSpace(strings.ToLower(item))
		switch name {
		case "fofa", "hunter", "quake", "censys", "zoomeye", "shodan":
		default:
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	return normalized
}

func intPtr(v int) *int {
	return &v
}

func maskKey(v string, keep int) string {
	if len(v) <= keep {
		return v
	}
	return v[:keep] + strings.Repeat("*", len(v)-keep)
}

func decodeJSON(r io.Reader, v interface{}) error {
	decoder := json.NewDecoder(r)
	return decoder.Decode(v)
}

func requireMCPSession(c *gin.Context) (string, *mcpSessionState, error) {
	sessionID := strings.TrimSpace(c.GetHeader("Mcp-Session-Id"))
	if sessionID == "" {
		return "", nil, fmt.Errorf("缺少 Mcp-Session-Id")
	}

	session := getMCPSession(sessionID)
	if session == nil {
		return "", nil, fmt.Errorf("MCP session 不存在或已过期")
	}
	return sessionID, session, nil
}

func ensureMCPInitialized(c *gin.Context, id interface{}) bool {
	_, session, err := requireMCPSession(c)
	if err != nil {
		writeMCPError(c, id, mcpCodeInvalidRequest, err.Error(), nil)
		return false
	}
	if !session.Initialized {
		writeMCPError(c, id, mcpCodeInvalidRequest, "MCP session 尚未完成 initialized 通知", nil)
		return false
	}
	return true
}

func createMCPSession(protocolVersion string, clientName string, clientVersion string) string {
	mcpSessions.Lock()
	defer mcpSessions.Unlock()

	cleanupExpiredMCPSessionsLocked()
	sessionID := nanoid.Generate()
	mcpSessions.items[sessionID] = &mcpSessionState{
		ProtocolVersion: protocolVersion,
		ClientName:      strings.TrimSpace(clientName),
		ClientVersion:   strings.TrimSpace(clientVersion),
		UpdatedAt:       time.Now(),
	}
	return sessionID
}

func getMCPSession(sessionID string) *mcpSessionState {
	mcpSessions.Lock()
	defer mcpSessions.Unlock()

	cleanupExpiredMCPSessionsLocked()
	session := mcpSessions.items[sessionID]
	if session == nil {
		return nil
	}

	cloned := *session
	return &cloned
}

func storeMCPSession(sessionID string, session *mcpSessionState) {
	mcpSessions.Lock()
	defer mcpSessions.Unlock()

	mcpSessions.items[sessionID] = session
}

func touchMCPSession(sessionID string) {
	mcpSessions.Lock()
	defer mcpSessions.Unlock()

	session := mcpSessions.items[sessionID]
	if session == nil {
		return
	}
	session.UpdatedAt = time.Now()
}

func deleteMCPSession(sessionID string) {
	mcpSessions.Lock()
	defer mcpSessions.Unlock()

	delete(mcpSessions.items, sessionID)
}

func cleanupExpiredMCPSessionsLocked() {
	now := time.Now()
	for sessionID, session := range mcpSessions.items {
		if now.Sub(session.UpdatedAt) > mcpSessionTTL {
			delete(mcpSessions.items, sessionID)
		}
	}
}

func writeMCPResult(c *gin.Context, id interface{}, result interface{}) {
	if toolResult, ok := result.(mcpToolCallResult); ok && toolResult.Blocked {
		message := "工具调用已被安全规则拦截"
		if len(toolResult.Content) > 0 {
			message = toolResult.Content[0].Text
		}
		markMCPAuditBlocked(c, message)
	}
	if failed, message := isMCPToolCallError(result); failed {
		markMCPAuditFailure(c, 0, message)
	}
	c.JSON(http.StatusOK, mcpJSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func writeMCPError(c *gin.Context, id interface{}, code int, message string, data interface{}) {
	markMCPAuditFailure(c, code, message)
	c.JSON(http.StatusOK, mcpJSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &mcpJSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	})
}
