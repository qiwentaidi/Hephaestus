package scan

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/subdomain"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	subdomainExportBatchSize = 1000
)

type subdomainScanTaskListItem struct {
	ID             string     `json:"id"`
	TaskName       string     `json:"taskName,omitempty"`
	Version        int        `json:"version"`
	Status         string     `json:"status"`
	Progress       int        `json:"progress"`
	TotalCount     int        `json:"totalCount"`
	CurrentCount   int        `json:"currentCount"`
	ResultCount    int64      `json:"resultCount"`
	ProgressDetail string     `json:"progressDetail,omitempty"`
	Config         string     `json:"config"`
	ErrorMsg       string     `json:"errorMsg,omitempty"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// GetSubdomainDNSServersHandler 获取子域名扫描 DNS 服务器配置
func GetSubdomainDNSServersHandler(c *gin.Context) {
	settings, err := subdomain.LoadDNSServerSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "加载DNS服务器配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": settings})
}

// UpdateSubdomainDNSServersHandler 更新子域名扫描 DNS 服务器配置
func UpdateSubdomainDNSServersHandler(c *gin.Context) {
	var req subdomain.DNSServerSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	if _, err := subdomain.NormalizeDNSServerSettings(req, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	settings, err := subdomain.SaveDNSServerSettings(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存DNS服务器配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": settings, "message": "DNS服务器配置已保存"})
}

// GetSubdomainScanResultsHandler 获取扫描结果
func GetSubdomainScanResultsHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	limit := 100
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// 获取搜索和筛选参数
	searchQuery := c.Query("search")
	sourceFilter := c.Query("source")
	wildcardFilter := c.Query("wildcard")
	if wildcardFilter != "" && wildcardFilter != "all" && wildcardFilter != "exclude" && wildcardFilter != "only" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的泛解析筛选参数"})
		return
	}

	results, total, err := subdomain.GetScanResults(taskID, limit, offset, searchQuery, sourceFilter, wildcardFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取扫描结果失败: " + err.Error(),
		})
		return
	}
	sourceStats, err := subdomain.GetScanResultSourceStats(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取扫描结果来源统计失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"results":     results,
			"total":       total,
			"limit":       limit,
			"offset":      offset,
			"sourceStats": sourceStats,
		},
	})
}

// ExportSubdomainScanResultsHandler 导出子域名扫描结果
func ExportSubdomainScanResultsHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	results, total, err := subdomain.GetAllScanResults(taskID, subdomainExportBatchSize, "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取扫描结果失败: " + err.Error(),
		})
		return
	}

	content, contentType, err := buildSubdomainExportContent(results)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "生成导出文件失败: " + err.Error(),
		})
		return
	}

	filename := fmt.Sprintf("subdomain_%s.csv", time.Now().Format("20060102_150405"))
	quoted := strings.ReplaceAll(filename, `"`, "_")
	escaped := url.PathEscape(filename)
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, quoted, escaped))
	c.Header("X-Total-Count", strconv.FormatInt(total, 10))
	c.Data(http.StatusOK, contentType, content)
}

// CopySubdomainScanResultsHandler 获取全量复制文本
func CopySubdomainScanResultsHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	copyType := strings.ToLower(strings.TrimSpace(c.DefaultQuery("type", "subdomain")))
	if copyType != "subdomain" && copyType != "single-ip" && copyType != "portscan-targets" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "不支持的复制类型: " + copyType,
		})
		return
	}

	results, _, err := subdomain.GetAllScanResults(taskID, subdomainExportBatchSize, "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取扫描结果失败: " + err.Error(),
		})
		return
	}

	text, count := buildSubdomainCopyText(results, copyType)
	if copyType == "single-ip" && (count == 0 || strings.TrimSpace(text) == "") {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    "NO_SINGLE_IP_RESULTS",
			"error":   "没有可复制的单 IP 结果",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"text":  text,
			"count": count,
		},
	})
}

func buildSubdomainExportContent(results []subdomain.SubdomainResult) ([]byte, string, error) {
	var buffer bytes.Buffer
	buffer.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"主域名", "子域名", "IP", "资产类型", "资产名称", "来源", "是否泛解析"}); err != nil {
		return nil, "", err
	}
	for _, result := range results {
		wildcardText := "否"
		if result.IsWildcard {
			wildcardText = "是"
		}
		if err := writer.Write([]string{
			result.Domain,
			result.Subdomain,
			strings.Join(result.IPs, ";"),
			result.AssetType,
			result.AssetName,
			formatSubdomainSources(result),
			wildcardText,
		}); err != nil {
			return nil, "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), "text/csv; charset=utf-8", nil
}

func formatSubdomainSources(result subdomain.SubdomainResult) string {
	if len(result.Sources) > 0 {
		return strings.Join(result.Sources, ";")
	}
	return result.Source
}

func buildSubdomainCopyText(results []subdomain.SubdomainResult, copyType string) (string, int) {
	switch copyType {
	case "portscan-targets":
		return buildSubdomainPortScanTargets(results)
	case "single-ip":
		seen := make(map[string]struct{}, len(results))
		lines := make([]string, 0, len(results))
		for _, result := range results {
			if len(result.IPs) != 1 || result.IPs[0] == "" {
				continue
			}
			ip := result.IPs[0]
			if _, ok := seen[ip]; ok {
				continue
			}
			seen[ip] = struct{}{}
			lines = append(lines, ip)
		}
		return strings.Join(lines, "\n"), len(lines)
	default:
		lines := make([]string, 0, len(results))
		for _, result := range results {
			lines = append(lines, result.Subdomain)
		}
		return strings.Join(lines, "\n"), len(lines)
	}
}

func buildSubdomainPortScanTargets(results []subdomain.SubdomainResult) (string, int) {
	// Check all rows first so a duplicate hostname with a mixed DNS answer
	// cannot bypass the filter through another row containing only remote IPs.
	loopbackHosts := make(map[string]struct{})
	for _, result := range results {
		if subdomainHasLoopbackIP(result) {
			loopbackHosts[normalizeCombinationTargetHost(result.Subdomain)] = struct{}{}
		}
	}
	seen := make(map[string]struct{})
	lines := make([]string, 0, len(results))
	appendTarget := func(target string) {
		if _, exists := seen[target]; !exists {
			seen[target] = struct{}{}
			lines = append(lines, target)
		}
	}
	for _, result := range results {
		host := strings.TrimSpace(result.Subdomain)
		if host == "" {
			continue
		}
		if _, hasLoopback := loopbackHosts[normalizeCombinationTargetHost(host)]; !hasLoopback {
			appendTarget(host)
			continue
		}
		// Import the remaining addresses rather than re-resolving a mixed
		// hostname during port scanning and potentially connecting to loopback.
		for _, rawIP := range result.IPs {
			if ip := net.ParseIP(normalizeCombinationTargetHost(rawIP)); ip != nil && !ip.IsLoopback() {
				appendTarget(ip.String())
			}
		}
	}
	return strings.Join(lines, "\n"), len(lines)
}

// ListSubdomainScanTasksHandler 获取任务列表
func ListSubdomainScanTasksHandler(c *gin.Context) {
	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}
	tasks, total, err := listUnifiedSubdomainTasks(db, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取任务列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"tasks":  tasks,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}

func listUnifiedSubdomainTasks(db *gorm.DB, limit int, offset int) ([]subdomainScanTaskListItem, int64, error) {
	var total int64
	if err := db.Model(&database.VulnScanTask{}).
		Where("scan_type = ?", "subdomain").Distinct("id").Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tasks []database.VulnScanTask
	query := db.Where(`scan_type = ? AND version = (
		SELECT MAX(latest.version) FROM vul_scan_task latest WHERE latest.id = vul_scan_task.id
	)`, "subdomain").Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	if err := query.Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	counts := make(map[string]int64, len(tasks))
	if len(tasks) > 0 {
		ids := make([]string, 0, len(tasks))
		for _, task := range tasks {
			ids = append(ids, task.ID)
		}
		var rows []struct {
			TaskID string `gorm:"column:task_id"`
			Count  int64  `gorm:"column:count"`
		}
		if err := db.Model(&database.SubdomainScanResult{}).
			Select("task_id, COUNT(*) AS count").Where("task_id IN ?", ids).
			Group("task_id").Scan(&rows).Error; err != nil {
			return nil, 0, err
		}
		for _, row := range rows {
			counts[row.TaskID] = row.Count
		}
	}

	items := make([]subdomainScanTaskListItem, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, subdomainScanTaskListItem{
			ID: task.ID, TaskName: task.TaskName, Version: task.Version,
			Status: task.Status, Progress: task.Progress, TotalCount: task.TotalCount,
			CurrentCount: task.CurrentCount, ResultCount: counts[task.ID],
			ProgressDetail: task.ProgressDetail, Config: task.Config, ErrorMsg: task.ErrorMsg,
			StartedAt: task.StartedAt, CompletedAt: task.CompletedAt,
			CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt,
		})
	}
	return items, total, nil
}
