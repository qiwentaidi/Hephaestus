package scan

import (
	"context"
	"hephaestus-backend/handlers/dashboard"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/airun"
	"hephaestus-backend/pkg/database"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetWebScanTrendHandler 获取扫描趋势数据（按天汇总漏洞严重等级）
func GetWebScanTrendHandler(c *gin.Context) {
	daysStr := c.DefaultQuery("days", "14")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 || days > 60 {
		days = 14
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	// Initialize the shared AI finding schema even before the first AI session.
	if _, err := ai.StoreFromAppDB(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询 AI 会话漏洞失败: " + err.Error()})
		return
	}
	trend, err := loadVulnerabilityTrend(c.Request.Context(), db, time.Now().AddDate(0, 0, -days))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询趋势数据失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"trend": trend,
		},
	})
}

type vulnerabilityTrendItem struct {
	Date     string `json:"date"`
	Critical int    `json:"critical"`
	High     int    `json:"high"`
	Medium   int    `json:"medium"`
	Low      int    `json:"low"`
}

func loadVulnerabilityTrend(ctx context.Context, db *gorm.DB, startDate time.Time) ([]vulnerabilityTrendItem, error) {
	// 先查询按日期和严重等级的统计（通过关联任务表获取创建时间）
	type RawItem struct {
		Date     string `json:"date"`
		Severity string `json:"severity"`
		Count    int    `json:"count"`
	}
	var raw []RawItem
	// 扫描漏洞只取最新任务版本；AI 会话漏洞按发现记录的创建时间汇总。
	if err := db.WithContext(ctx).Raw(`
		SELECT CAST(date AS TEXT) AS date, severity, SUM(count) AS count
		FROM (
			SELECT DATE(t.created_at) AS date, LOWER(v.severity) AS severity, COUNT(*) AS count
			FROM vulnerability_info v
			INNER JOIN vul_scan_task t ON v.task_id = t.id AND v.version = t.version
			INNER JOIN (
				SELECT id, MAX(version) AS max_version
				FROM vul_scan_task
				GROUP BY id
			) latest ON t.id = latest.id AND t.version = latest.max_version
			WHERE t.created_at >= ?
			GROUP BY DATE(t.created_at), LOWER(v.severity)
			UNION ALL
			SELECT DATE(t.created_at) AS date, LOWER(v.level) AS severity, COUNT(*) AS count
			FROM vuln_scan_js_vulnerability v
			INNER JOIN vul_scan_task t ON v.task_id = t.id AND v.version = t.version
			INNER JOIN (
				SELECT id, MAX(version) AS max_version
				FROM vul_scan_task
				GROUP BY id
			) latest ON t.id = latest.id AND t.version = latest.max_version
			WHERE t.created_at >= ?
			GROUP BY DATE(t.created_at), LOWER(v.level)
			UNION ALL
			SELECT DATE(f.created_at) AS date, LOWER(f.severity) AS severity, COUNT(*) AS count
			FROM pentest_findings f
			WHERE f.created_at >= ?
			GROUP BY DATE(f.created_at), LOWER(f.severity)
		) AS merged
		GROUP BY date, severity
		ORDER BY date ASC
	`, startDate, startDate, startDate).Scan(&raw).Error; err != nil {
		return nil, err
	}

	// 聚合为按日期的结构
	trendMap := make(map[string]*vulnerabilityTrendItem)
	for _, r := range raw {
		item, ok := trendMap[r.Date]
		if !ok {
			item = &vulnerabilityTrendItem{Date: r.Date}
			trendMap[r.Date] = item
		}
		switch strings.ToLower(r.Severity) {
		case "critical":
			item.Critical += r.Count
		case "high":
			item.High += r.Count
		case "medium":
			item.Medium += r.Count
		case "low", "info", "information":
			item.Low += r.Count
		default:
			// 未知等级归到low
			item.Low += r.Count
		}
	}

	// 转为有序切片
	trend := make([]vulnerabilityTrendItem, 0, len(trendMap))
	for _, v := range trendMap {
		trend = append(trend, *v)
	}
	sort.Slice(trend, func(i, j int) bool {
		return trend[i].Date < trend[j].Date
	})

	return trend, nil
}

// GetVulnSeverityStatsHandler 获取漏洞等级分布
// days=0 表示获取全部时间的统计，days>0 表示获取最近N天的统计
func GetVulnSeverityStatsHandler(c *gin.Context) {
	daysStr := c.DefaultQuery("days", "0") // 默认获取全部
	days, err := strconv.Atoi(daysStr)
	if err != nil || days < 0 || days > 365 {
		days = 0
	}

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
	var startDate *time.Time
	if days > 0 {
		start := time.Now().AddDate(0, 0, -days)
		startDate = &start
	}
	stats, err := dashboard.LoadDashboardVulnerabilityStats(c.Request.Context(), db, startDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询漏洞分布失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats.Severity,
	})
}

// GetRecentVulnerabilitiesHandler 获取最近发现的漏洞列表
func GetRecentVulnerabilitiesHandler(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 10
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	store, err := ai.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询 AI 会话漏洞失败: " + err.Error()})
		return
	}
	vulns, err := loadRecentVulnerabilities(c.Request.Context(), db, store, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询漏洞失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"vulns": vulns,
			"total": len(vulns),
		},
	})
}

type recentVulnerabilityItem struct {
	ID         string    `json:"id"`
	TaskID     string    `json:"taskId"`
	VulnName   string    `json:"vulnName"`
	TemplateID string    `json:"templateId"`
	Target     string    `json:"target"`
	Severity   string    `json:"severity"`
	Confidence string    `json:"confidence"`
	AIVerified bool      `json:"aiVerified"`
	SourceType string    `json:"sourceType"`
	CreatedAt  time.Time `json:"createdAt"`
}

// loadRecentVulnerabilities merges each source's newest records before applying
// the dashboard limit. AI provenance does not imply aiVerified.
func loadRecentVulnerabilities(ctx context.Context, db *gorm.DB, store *pentest.SQLStore, limit int) ([]recentVulnerabilityItem, error) {
	vulns := make([]recentVulnerabilityItem, 0, limit)
	// 获取最近的漏洞，只取每个任务最新版本的数据，并合并普通漏洞与 JS 漏洞
	if err := db.WithContext(ctx).Raw(`
		SELECT *
		FROM (
			SELECT
				(v.task_id || '-' || COALESCE(v.template_id, '')) AS id,
				v.task_id AS task_id,
				COALESCE(v.vuln_name, v.template_id, '') AS vuln_name,
				COALESCE(v.template_id, '') AS template_id,
				COALESCE(v.vuln_url, '') AS target,
				LOWER(COALESCE(v.severity, 'info')) AS severity,
				'' AS confidence,
				FALSE AS ai_verified,
				'webscan' AS source_type,
				t.created_at AS created_at
			FROM vulnerability_info v
			INNER JOIN vul_scan_task t ON v.task_id = t.id AND v.version = t.version
			INNER JOIN (
				SELECT id, MAX(version) AS max_version
				FROM vul_scan_task
				GROUP BY id
			) latest ON t.id = latest.id AND t.version = latest.max_version

			UNION ALL

			SELECT
				(v.id) AS id,
				v.task_id AS task_id,
				COALESCE(v.title, v.type, '') AS vuln_name,
				COALESCE(v.type, '') AS template_id,
				COALESCE(v.url, '') AS target,
				LOWER(COALESCE(v.level, 'info')) AS severity,
				COALESCE(v.confidence, '') AS confidence,
				COALESCE(v.ai_verified, FALSE) AS ai_verified,
				'jsscan' AS source_type,
				v.created_at AS created_at
			FROM vuln_scan_js_vulnerability v
			INNER JOIN vul_scan_task t ON v.task_id = t.id AND v.version = t.version
			INNER JOIN (
				SELECT id, MAX(version) AS max_version
				FROM vul_scan_task
				GROUP BY id
			) latest ON t.id = latest.id AND t.version = latest.max_version
		) merged
		ORDER BY created_at DESC, source_type DESC, id DESC, target DESC
		LIMIT ?
	`, limit).Scan(&vulns).Error; err != nil {
		return nil, err
	}
	findings, err := store.ListFindings(ctx, "", limit)
	if err != nil {
		return nil, err
	}
	for _, finding := range findings {
		vulns = append(vulns, recentVulnerabilityItem{
			ID: finding.ID, TaskID: finding.ConversationID,
			VulnName: finding.Title, Target: airun.CompactFindingTarget(finding.Target),
			Severity: strings.ToLower(finding.Severity), SourceType: "pentest_blackboard",
			CreatedAt: finding.CreatedAt,
		})
	}
	sort.Slice(vulns, func(i, j int) bool {
		if !vulns[i].CreatedAt.Equal(vulns[j].CreatedAt) {
			return vulns[i].CreatedAt.After(vulns[j].CreatedAt)
		}
		if vulns[i].SourceType != vulns[j].SourceType {
			return vulns[i].SourceType > vulns[j].SourceType
		}
		if vulns[i].ID != vulns[j].ID {
			return vulns[i].ID > vulns[j].ID
		}
		return vulns[i].Target > vulns[j].Target
	})
	if len(vulns) > limit {
		vulns = vulns[:limit]
	}
	return vulns, nil
}

// GetPortAndIPStatsHandler 获取端口数和IP数量统计
func GetPortAndIPStatsHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	// 统计端口总数
	var portCount int64
	if err := db.Model(&database.PortScanResult{}).Count(&portCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询端口数失败: " + err.Error(),
		})
		return
	}

	// 统计唯一IP数量
	var ipCount int64
	if err := db.Model(&database.PortScanResult{}).
		Distinct("host").
		Count(&ipCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询IP数量失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"portCount": portCount,
			"ipCount":   ipCount,
		},
	})
}
