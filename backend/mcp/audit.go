package mcp

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/nanoid"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	mcpAuditContextKey = "mcp_audit"
)

var mcpAuditServers = []string{"hephaestus", "hephaestus-space-engine"}

type mcpAuditContext struct {
	StartedAt     time.Time
	Server        string
	Method        string
	ToolName      string
	SessionID     string
	ClientName    string
	ClientVersion string
	Status        string
	ErrorCode     int
	ErrorMessage  string
}

func startMCPAudit(c *gin.Context, server string) *mcpAuditContext {
	audit := &mcpAuditContext{
		StartedAt: time.Now(),
		Server:    strings.TrimSpace(server),
		Method:    strings.ToLower(strings.TrimSpace(c.Request.Method)),
	}
	c.Set(mcpAuditContextKey, audit)
	return audit
}

func currentMCPAudit(c *gin.Context) *mcpAuditContext {
	value, ok := c.Get(mcpAuditContextKey)
	if !ok {
		return nil
	}
	audit, _ := value.(*mcpAuditContext)
	return audit
}

func finishMCPAudit(c *gin.Context, audit *mcpAuditContext) {
	if audit == nil {
		return
	}

	status := strings.TrimSpace(audit.Status)
	if status == "" {
		status = "success"
	}

	httpStatus := c.Writer.Status()
	if httpStatus == 0 {
		httpStatus = http.StatusOK
	}
	if httpStatus >= http.StatusBadRequest && status == "success" {
		status = "failed"
	}

	db, err := database.GetDB()
	if err != nil {
		logger.Error("保存 MCP 调用审计失败，数据库不可用: %v", err)
		return
	}

	record := database.MCPSpaceEngineAuditLog{
		ID:            nanoid.Generate(),
		Server:        fallbackAuditValue(audit.Server, "hephaestus-space-engine"),
		Method:        fallbackAuditValue(audit.Method, strings.ToLower(strings.TrimSpace(c.Request.Method))),
		ToolName:      trimAuditValue(audit.ToolName, 255),
		SessionID:     trimAuditValue(audit.SessionID, 255),
		ClientName:    trimAuditValue(audit.ClientName, 255),
		ClientVersion: trimAuditValue(audit.ClientVersion, 100),
		UserAgent:     trimAuditValue(c.GetHeader("User-Agent"), 2048),
		SourceIP:      trimAuditValue(c.ClientIP(), 100),
		Status:        status,
		HTTPStatus:    httpStatus,
		ErrorCode:     audit.ErrorCode,
		ErrorMessage:  trimAuditValue(audit.ErrorMessage, 2048),
		DurationMS:    time.Since(audit.StartedAt).Milliseconds(),
		CreatedAt:     time.Now(),
	}
	if record.Method == "" {
		record.Method = "unknown"
	}

	if err := db.Create(&record).Error; err != nil {
		logger.Error("保存 MCP 调用审计失败: %v", err)
	}
}

func markMCPAuditRequest(c *gin.Context, method string) {
	if audit := currentMCPAudit(c); audit != nil {
		audit.Method = strings.TrimSpace(method)
	}
}

func markMCPAuditSession(c *gin.Context, sessionID string, session *mcpSessionState) {
	audit := currentMCPAudit(c)
	if audit == nil {
		return
	}

	audit.SessionID = strings.TrimSpace(sessionID)
	if session == nil {
		return
	}
	audit.ClientName = session.ClientName
	audit.ClientVersion = session.ClientVersion
}

func markMCPAuditClient(c *gin.Context, name string, version string) {
	if audit := currentMCPAudit(c); audit != nil {
		audit.ClientName = strings.TrimSpace(name)
		audit.ClientVersion = strings.TrimSpace(version)
	}
}

func markMCPAuditTool(c *gin.Context, toolName string) {
	if audit := currentMCPAudit(c); audit != nil {
		audit.ToolName = strings.TrimSpace(toolName)
	}
}

func markMCPAuditFailure(c *gin.Context, code int, message string) {
	if audit := currentMCPAudit(c); audit != nil {
		audit.Status = "failed"
		audit.ErrorCode = code
		audit.ErrorMessage = strings.TrimSpace(message)
	}
}

func markMCPAuditBlocked(c *gin.Context, message string) {
	if audit := currentMCPAudit(c); audit != nil {
		audit.Status = "blocked"
		audit.ErrorCode = 0
		audit.ErrorMessage = strings.TrimSpace(message)
	}
}

func parseMCPClientInfo(info map[string]interface{}) (string, string) {
	if len(info) == 0 {
		return "", ""
	}

	name, _ := info["name"].(string)
	version, _ := info["version"].(string)
	return strings.TrimSpace(name), strings.TrimSpace(version)
}

func isMCPToolCallError(result interface{}) (bool, string) {
	toolResult, ok := result.(mcpToolCallResult)
	if !ok || !toolResult.IsError || toolResult.Blocked {
		return false, ""
	}
	if len(toolResult.Content) > 0 {
		return true, toolResult.Content[0].Text
	}
	return true, "tool call failed"
}

func fallbackAuditValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func trimAuditValue(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

// ListMCPAuditLogsHandler 返回真实 MCP 调用审计记录。
func ListMCPAuditLogsHandler(c *gin.Context) {
	page := parseAuditPositiveInt(c.DefaultQuery("page", "1"), 1, 0)
	pageSizeRaw := c.Query("pageSize")
	if pageSizeRaw == "" {
		pageSizeRaw = c.Query("limit")
	}
	pageSize := parseAuditPositiveInt(pageSizeRaw, 10, 200)
	hours := parseAuditPositiveInt(c.DefaultQuery("hours", "24"), 24, 24*30)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	query := applyMCPAuditLogFilters(
		db.Model(&database.MCPSpaceEngineAuditLog{}).
			Where("server IN ? AND created_at >= ?", mcpAuditServers, since),
		c,
	)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "统计 MCP 调用审计失败: " + err.Error(),
		})
		return
	}

	if total == 0 {
		page = 1
	} else {
		totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
		if page > totalPages {
			page = totalPages
		}
	}

	var logs []database.MCPSpaceEngineAuditLog
	if err := applyMCPAuditLogFilters(
		db.Where("server IN ? AND created_at >= ?", mcpAuditServers, since),
		c,
	).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询 MCP 调用审计失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":    logs,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// ListMCPToolSpaceEngineAuditLogsHandler is kept for older internal callers.
func ListMCPToolSpaceEngineAuditLogsHandler(c *gin.Context) {
	ListMCPAuditLogsHandler(c)
}

func applyMCPAuditLogFilters(query *gorm.DB, c *gin.Context) *gorm.DB {
	kind := strings.ToLower(strings.TrimSpace(c.Query("kind")))
	switch kind {
	case "important":
		query = query.Where("(method = ? OR status IN ?)", "tools/call", []string{"failed", "blocked"})
	case "tool_call":
		query = query.Where("method = ?", "tools/call")
	case "protocol":
		query = query.Where("method <> ? AND status <> ?", "tools/call", "failed")
	}

	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	if status == "success" || status == "failed" || status == "blocked" {
		query = query.Where("status = ?", status)
	}

	keyword := trimAuditValue(c.Query("keyword"), 120)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where(
			"method LIKE ? OR tool_name LIKE ? OR client_name LIKE ? OR client_version LIKE ? OR source_ip LIKE ? OR session_id LIKE ?",
			like,
			like,
			like,
			like,
			like,
			like,
		)
	}

	return query
}

func parseAuditPositiveInt(raw string, fallback int, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	if max > 0 && value > max {
		return max
	}
	return value
}
