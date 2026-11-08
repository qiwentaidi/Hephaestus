package dashboard

import (
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	assethandler "hephaestus-backend/handlers/asset"
	"hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DashboardTask 统一的任务信息结构
type DashboardTask struct {
	ID        string    `json:"id"`
	TaskName  string    `json:"taskName"`
	TaskType  string    `json:"taskType"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func GetDashboardTokenUsageHandler(c *gin.Context) {
	days := 7
	if value, err := strconv.Atoi(c.DefaultQuery("days", "7")); err == nil && value >= 1 && value <= 30 {
		days = value
	}
	store, err := ai.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Token 用量不可用"})
		return
	}
	usage, err := store.TokenUsage(c.Request.Context(), "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Token 用量读取失败"})
		return
	}
	trend, err := store.TokenUsageTrend(c.Request.Context(), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Token 趋势读取失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"totalTokens": usage.TotalTokens, "inputTokens": usage.InputTokens,
		"outputTokens": usage.OutputTokens, "cacheReadTokens": usage.CacheReadTokens,
		"trend": trend,
	}})
}

// DashboardTasksResponse Dashboard 任务数据响应
type DashboardTasksResponse struct {
	AITotalTokens          *int64          `json:"aiTotalTokens"`
	RecentTasks            []DashboardTask `json:"recentTasks"`
	TaskTypeDistribution   map[string]int  `json:"taskTypeDistribution"`
	TaskStatusDistribution map[string]int  `json:"taskStatusDistribution"`
}

// deduplicateDashboardTasks collapses records that represent the same logical
// task across the unified and legacy task tables.
func deduplicateDashboardTasks(tasks []DashboardTask) []DashboardTask {
	uniqueTasks := make(map[string]DashboardTask, len(tasks))
	for _, task := range tasks {
		key := task.TaskType + "\x00" + task.ID
		existing, ok := uniqueTasks[key]
		if !ok || task.UpdatedAt.After(existing.UpdatedAt) ||
			(task.UpdatedAt.Equal(existing.UpdatedAt) && task.CreatedAt.After(existing.CreatedAt)) {
			uniqueTasks[key] = task
		}
	}

	deduplicated := make([]DashboardTask, 0, len(uniqueTasks))
	for _, task := range uniqueTasks {
		deduplicated = append(deduplicated, task)
	}
	return deduplicated
}

// GetDashboardTasksHandler 获取 Dashboard 任务数据
// @Summary 获取 Dashboard 任务统计数据
// @Description 返回最近扫描活动、任务类型分布、任务状态分布
// @Tags Dashboard
// @Produce json
// @Param limit query int false "最近任务数量限制，默认 10"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/dashboard/tasks [get]
func GetDashboardTasksHandler(c *gin.Context) {
	db, err := database.GetDB()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "数据库连接失败",
		})
		return
	}

	// Usage failures are represented as unavailable, never as a false zero.
	var aiTotalTokens *int64
	if store, usageErr := ai.StoreFromAppDB(c.Request.Context()); usageErr == nil {
		if total, usageErr := store.TotalTokenUsage(c.Request.Context(), ""); usageErr == nil {
			aiTotalTokens = &total
		}
	}
	// 获取 limit 参数，默认 10
	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	var allTasks []DashboardTask
	taskTypeCount := make(map[string]int)
	taskStatusCount := make(map[string]int)

	// 1. 获取所有漏洞扫描任务（只获取每个 ID 的最新版本）
	var vulnScanTasks []database.VulnScanTask
	if err := db.Raw(`
			SELECT t1.id, t1.task_name, t1.scan_type, t1.status, t1.created_at, t1.updated_at
			FROM vul_scan_task t1
			INNER JOIN (
				SELECT id, MAX(version) AS max_version
				FROM vul_scan_task
				GROUP BY id
			) t2 ON t1.id = t2.id AND t1.version = t2.max_version
		`).Scan(&vulnScanTasks).Error; err == nil {
		for _, t := range vulnScanTasks {
			taskName := t.TaskName
			taskType := t.ScanType // 使用 scan_type 字段作为任务类型

			// 如果没有任务名称，根据任务类型设置默认名称
			if taskName == "" {
				switch taskType {
				case "webscan":
					taskName = "漏洞扫描任务"
				case "portscan":
					taskName = "端口扫描任务"
				case "crack":
					taskName = "密码暴破任务"
				case "jsscan":
					taskName = "JS扫描任务"
				case "dirscan":
					taskName = "目录扫描任务"
				default:
					taskName = "扫描任务"
				}
			}

			allTasks = append(allTasks, DashboardTask{
				ID:        t.ID,
				TaskName:  taskName,
				TaskType:  taskType,
				Status:    t.Status,
				CreatedAt: t.CreatedAt,
				UpdatedAt: t.UpdatedAt,
			})
			taskTypeCount[taskType]++
			taskStatusCount[t.Status]++
		}
	}

	// 统一任务表承载攻击面评估中的子域名发现任务；Dashboard 不再暴露旧独立子域名任务入口。
	allTasks = deduplicateDashboardTasks(allTasks)
	taskTypeCount = make(map[string]int)
	taskStatusCount = make(map[string]int)
	for _, task := range allTasks {
		taskTypeCount[task.TaskType]++
		taskStatusCount[task.Status]++
	}

	// 按 CreatedAt 降序排序
	sort.Slice(allTasks, func(i, j int) bool {
		return allTasks[i].CreatedAt.After(allTasks[j].CreatedAt)
	})

	// 限制返回数量
	recentTasks := allTasks
	if len(allTasks) > limit {
		recentTasks = allTasks[:limit]
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": DashboardTasksResponse{
			AITotalTokens:          aiTotalTokens,
			RecentTasks:            recentTasks,
			TaskTypeDistribution:   taskTypeCount,
			TaskStatusDistribution: taskStatusCount,
		},
	})
}

// AssetStatsResponse 资产统计响应
type AssetStatsResponse struct {
	IPCount          int64 `json:"ipCount"`
	PortCount        int64 `json:"portCount"`
	WebsiteCount     int64 `json:"websiteCount"`
	FingerprintCount int64 `json:"fingerprintCount"` // Compatibility alias for older clients.
	SubdomainCount   int64 `json:"subdomainCount"`
}

// VulnDisposalStatsResponse 漏洞处置状态统计响应
type VulnDisposalStatsResponse struct {
	Open          int64 `json:"open"`
	InProgress    int64 `json:"inProgress"`
	Fixed         int64 `json:"fixed"`
	FalsePositive int64 `json:"falsePositive"`
	Ignored       int64 `json:"ignored"`
}

func collectUniqueDashboardIPs(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, nil
	}

	uniqueIPs := make(map[string]struct{}, 2048)
	addIP := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if host, _, err := net.SplitHostPort(value); err == nil {
			value = host
		} else {
			value = strings.Trim(value, "[]")
		}
		if parsed := net.ParseIP(value); parsed != nil {
			uniqueIPs[parsed.String()] = struct{}{}
		}
	}
	addIPsFromText := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}

		var ipList []string
		if err := json.Unmarshal([]byte(raw), &ipList); err == nil {
			for _, ip := range ipList {
				addIP(ip)
			}
			return
		}

		var singleIP string
		if err := json.Unmarshal([]byte(raw), &singleIP); err == nil {
			addIP(singleIP)
			return
		}

		for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == ';'
		}) {
			addIP(token)
		}
	}

	rows, err := db.Model(&database.PortScanResult{}).
		Select("DISTINCT host").
		Where("host IS NOT NULL AND host != ''").
		Rows()
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var host string
		if err := rows.Scan(&host); err != nil {
			return 0, err
		}
		addIP(host)
	}

	subdomainRows, err := db.Model(&database.SubdomainScanResult{}).
		Select("ips").
		Where("ips IS NOT NULL AND ips != ''").
		Rows()
	if err != nil {
		return 0, err
	}
	defer subdomainRows.Close()

	for subdomainRows.Next() {
		var ips string
		if err := subdomainRows.Scan(&ips); err != nil {
			return 0, err
		}
		addIPsFromText(ips)
	}

	fingerprintRows, err := db.Model(&database.FingerprintInfo{}).
		Select("host").
		Where("host IS NOT NULL AND host != ''").
		Rows()
	if err != nil {
		return 0, err
	}
	defer fingerprintRows.Close()

	for fingerprintRows.Next() {
		var host string
		if err := fingerprintRows.Scan(&host); err != nil {
			return 0, err
		}
		addIP(host)
	}

	return int64(len(uniqueIPs)), nil
}

func countLatestDashboardFingerprints(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, nil
	}

	var count int64
	err := db.Table("fingerprint_info AS fp").
		Joins(`
INNER JOIN (
	SELECT id, MAX(version) AS max_version
	FROM vul_scan_task
	GROUP BY id
) latest ON fp.task_id = latest.id AND fp.version = latest.max_version`).
		Count(&count).Error
	return count, err
}

// GetAssetStatsHandler 获取资产统计数据
// @Summary 获取资产统计数据
// @Description 返回与资产中心台账一致的 IP、端口、网站、子域名数量统计
// @Tags Dashboard
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/dashboard/asset-stats [get]
func GetAssetStatsHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	counts, err := assethandler.LoadLedgerEntityCounts(db.WithContext(c.Request.Context()), false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取资产台账统计失败: " + err.Error(),
		})
		return
	}
	stats := AssetStatsResponse{
		IPCount:          int64(counts.IP),
		PortCount:        int64(counts.Port),
		WebsiteCount:     int64(counts.Website),
		FingerprintCount: int64(counts.Website),
		SubdomainCount:   int64(counts.Subdomain),
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// GetVulnDisposalStatsHandler 获取漏洞处置状态统计数据
// @Summary 获取漏洞处置状态统计数据
// @Description 返回漏洞待处置、处理中、已修复、误报、已忽略的数量统计
// @Tags Dashboard
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/dashboard/vuln-disposal-stats [get]
func GetVulnDisposalStatsHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	if _, err := ai.StoreFromAppDB(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询 AI 会话漏洞失败: " + err.Error()})
		return
	}
	stats, err := LoadDashboardVulnerabilityStats(c.Request.Context(), db, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询漏洞处置统计失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats.Disposal,
	})
}
