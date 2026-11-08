package scan

import (
	"fmt"
	"github.com/google/uuid"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/findings"
	"hephaestus-backend/pkg/logger"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetWebScanTaskVulnerabilitiesHandler 获取任务漏洞结果
func GetWebScanTaskVulnerabilitiesHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	// 获取分页参数
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "20")
	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// 获取搜索和过滤参数
	searchQuery := c.DefaultQuery("search", "")
	severityFilter := c.DefaultQuery("severity", "")
	disposalStatusFilter := c.DefaultQuery("disposalStatus", "")

	// 获取版本号（如果指定了 version 参数，使用指定版本；否则使用最新版本）
	versionStr := c.DefaultQuery("version", "")
	var version int
	var task database.VulnScanTask

	if versionStr != "" {
		if v, err := strconv.Atoi(versionStr); err == nil && v > 0 {
			version = v
		} else {
			// 版本号无效，获取最新版本
			if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{
					"success": false,
					"error":   "任务不存在",
				})
				return
			}
			version = task.Version
		}
	} else {
		// 没有指定版本，获取最新版本
		if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "任务不存在",
			})
			return
		}
		version = task.Version
	}

	if task.ID == "" {
		if err := db.Where("id = ? AND version = ?", taskID, version).First(&task).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "任务不存在",
			})
			return
		}
	}

	// 构建基础查询条件
	baseQuery := db.Model(&database.VulnerabilityInfo{})
	if task.ScanType == "combination" && strings.TrimSpace(task.ParentCombinationID) != "" {
		resultTaskIDs, err := combinationResultTaskIDs(db, task, version)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "查询子任务失败: " + err.Error(),
			})
			return
		}
		baseQuery = baseQuery.Where("task_id IN ? AND version = ?", resultTaskIDs, version)
	} else {
		baseQuery = baseQuery.Where("task_id = ? AND version = ?", taskID, version)
	}

	// 添加搜索条件（支持漏洞名称、URL、描述）
	if searchQuery != "" {
		searchPattern := "%" + searchQuery + "%"
		baseQuery = baseQuery.Where("vuln_name LIKE ? OR vuln_url LIKE ? OR description LIKE ?",
			searchPattern, searchPattern, searchPattern)
	}

	// 统计信息基于任务/版本/搜索词，不受右侧筛选影响
	statsQuery := baseQuery.Session(&gorm.Session{})

	// 添加严重程度过滤条件（不区分大小写）
	query := baseQuery.Session(&gorm.Session{})
	if severityFilter != "" {
		query = query.Where("LOWER(severity) = LOWER(?)", severityFilter)
	}

	// 添加处置状态过滤条件
	if disposalStatusFilter != "" {
		query = query.Where("disposal_status = ?", disposalStatusFilter)
	}

	// 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询漏洞总数失败: " + err.Error(),
		})
		return
	}

	var statsTotal int64
	if err := statsQuery.Session(&gorm.Session{}).Count(&statsTotal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询漏洞统计失败: " + err.Error(),
		})
		return
	}

	stats := gin.H{
		"total": statsTotal,
		"severity": gin.H{
			"critical": int64(0),
			"high":     int64(0),
			"medium":   int64(0),
			"low":      int64(0),
			"info":     int64(0),
		},
		"disposalStatus": gin.H{
			"open":           int64(0),
			"in_progress":    int64(0),
			"fixed":          int64(0),
			"false_positive": int64(0),
			"ignored":        int64(0),
		},
	}

	countWithQuery := func(q *gorm.DB, condition string, args ...interface{}) int64 {
		var count int64
		if err := q.Session(&gorm.Session{}).Where(condition, args...).Count(&count).Error; err != nil {
			return 0
		}
		return count
	}

	stats["severity"] = gin.H{
		"critical": countWithQuery(statsQuery, "LOWER(severity) = ?", "critical"),
		"high":     countWithQuery(statsQuery, "LOWER(severity) = ?", "high"),
		"medium":   countWithQuery(statsQuery, "LOWER(severity) = ?", "medium"),
		"low":      countWithQuery(statsQuery, "LOWER(severity) = ?", "low"),
		"info":     countWithQuery(statsQuery, "LOWER(severity) = ?", "info"),
	}
	stats["disposalStatus"] = gin.H{
		"open":           countWithQuery(statsQuery, "COALESCE(disposal_status, 'open') = ?", "open"),
		"in_progress":    countWithQuery(statsQuery, "COALESCE(disposal_status, 'open') = ?", "in_progress"),
		"fixed":          countWithQuery(statsQuery, "COALESCE(disposal_status, 'open') = ?", "fixed"),
		"false_positive": countWithQuery(statsQuery, "COALESCE(disposal_status, 'open') = ?", "false_positive"),
		"ignored":        countWithQuery(statsQuery, "COALESCE(disposal_status, 'open') = ?", "ignored"),
	}

	var vulnerabilities []database.VulnerabilityInfo
	// 查询指定版本的数据
	// vulnerability_info 为历史无主键表，使用已有字段进行稳定排序，避免翻页抖动
	if err := query.Order("template_id ASC").
		Order("vuln_url ASC").
		Order("protocol ASC").
		Order("severity DESC").
		Order("vuln_name ASC").
		Offset(offset).
		Limit(pageSize).
		Find(&vulnerabilities).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询漏洞结果失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"vulnerabilities": vulnerabilities,
			"total":           total,
			"page":            page,
			"pageSize":        pageSize,
			"stats":           stats,
		},
	})
}

type versionHistoryStats struct {
	FingerprintCount int64
	PortCount        int64
	DirResultCount   int64
	SubdomainCount   int64
	WebVulnCount     int64
	JSVulnCount      int64
	CriticalCount    int64
	HighCount        int64
	MediumCount      int64
	LowCount         int64
	InfoCount        int64
}

func normalizeHistoryScanType(scanType string) string {
	return strings.ToLower(strings.TrimSpace(scanType))
}

func scanTypeHasAny(scanType string, candidates ...string) bool {
	scanType = normalizeHistoryScanType(scanType)
	for _, candidate := range candidates {
		if scanType == candidate {
			return true
		}
	}
	return false
}

func loadVersionHistoryStats(db *gorm.DB, task database.VulnScanTask, taskIDs []string, version int) (versionHistoryStats, error) {
	var stats versionHistoryStats
	scanType := normalizeHistoryScanType(task.ScanType)
	isCombination := scanType == "combination"

	shouldCountWebScan := isCombination || scanTypeHasAny(scanType, "webscan")
	shouldCountPortScan := isCombination || scanTypeHasAny(scanType, "portscan", "crack")
	shouldCountDirScan := isCombination || scanTypeHasAny(scanType, "dirscan")
	shouldCountSubdomain := isCombination || scanTypeHasAny(scanType, "subdomain")
	shouldCountJSScan := isCombination || scanTypeHasAny(scanType, "jsscan")
	shouldCountWebVuln := isCombination || scanTypeHasAny(scanType, "webscan", "crack")

	if shouldCountWebScan {
		if err := db.Model(&database.FingerprintInfo{}).Where("task_id IN ? AND version = ?", taskIDs, version).Count(&stats.FingerprintCount).Error; err != nil {
			return stats, err
		}
	}

	if shouldCountPortScan {
		if err := db.Model(&database.PortScanResult{}).Where("task_id IN ? AND version = ?", taskIDs, version).Count(&stats.PortCount).Error; err != nil {
			return stats, err
		}
	}

	if shouldCountDirScan {
		if err := db.Model(&database.VulnScanDirResult{}).Where("task_id IN ? AND version = ?", taskIDs, version).Count(&stats.DirResultCount).Error; err != nil {
			return stats, err
		}
	}

	if shouldCountSubdomain {
		if err := db.Model(&database.SubdomainScanResult{}).Where("task_id IN ?", taskIDs).Count(&stats.SubdomainCount).Error; err != nil {
			return stats, err
		}
	}

	if shouldCountWebVuln {
		if err := db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ?", taskIDs, version).Count(&stats.WebVulnCount).Error; err != nil {
			return stats, err
		}
		if err := db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ? AND LOWER(severity) = ?", taskIDs, version, "critical").Count(&stats.CriticalCount).Error; err != nil {
			return stats, err
		}
		if err := db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ? AND LOWER(severity) = ?", taskIDs, version, "high").Count(&stats.HighCount).Error; err != nil {
			return stats, err
		}
		if err := db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ? AND LOWER(severity) = ?", taskIDs, version, "medium").Count(&stats.MediumCount).Error; err != nil {
			return stats, err
		}
		if err := db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ? AND LOWER(severity) = ?", taskIDs, version, "low").Count(&stats.LowCount).Error; err != nil {
			return stats, err
		}
		if err := db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ? AND LOWER(severity) = ?", taskIDs, version, "info").Count(&stats.InfoCount).Error; err != nil {
			return stats, err
		}
	}

	if shouldCountJSScan {
		if err := db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ?", taskIDs, version).Count(&stats.JSVulnCount).Error; err != nil {
			return stats, err
		}
		var jsCriticalCount, jsHighCount, jsMediumCount, jsLowCount, jsInfoCount int64
		if err := db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ? AND LOWER(level) = ?", taskIDs, version, "critical").Count(&jsCriticalCount).Error; err != nil {
			return stats, err
		}
		if err := db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ? AND LOWER(level) = ?", taskIDs, version, "high").Count(&jsHighCount).Error; err != nil {
			return stats, err
		}
		if err := db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ? AND LOWER(level) = ?", taskIDs, version, "medium").Count(&jsMediumCount).Error; err != nil {
			return stats, err
		}
		if err := db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ? AND LOWER(level) = ?", taskIDs, version, "low").Count(&jsLowCount).Error; err != nil {
			return stats, err
		}
		if err := db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ? AND LOWER(level) = ?", taskIDs, version, "info").Count(&jsInfoCount).Error; err != nil {
			return stats, err
		}
		stats.CriticalCount += jsCriticalCount
		stats.HighCount += jsHighCount
		stats.MediumCount += jsMediumCount
		stats.LowCount += jsLowCount
		stats.InfoCount += jsInfoCount
	}

	return stats, nil
}

// GetWebScanTaskVersionsHistoryHandler 获取任务所有版本的历史统计数据
// 按任务类型返回可用于历史趋势展示的结果统计，并兼容组合任务的子任务结果聚合。
func GetWebScanTaskVersionsHistoryHandler(c *gin.Context) {
	taskID := c.Param("id")

	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	// 获取任务的所有版本记录
	var tasks []database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version ASC").Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取任务版本失败: " + err.Error(),
		})
		return
	}

	if len(tasks) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "任务不存在",
		})
		return
	}

	// 获取任务名称和类型（从最新版本）
	latestTask := tasks[len(tasks)-1]
	taskName := latestTask.TaskName
	scanType := latestTask.ScanType

	// 构建版本历史数据
	type VersionHistory struct {
		Version          int    `json:"version"`
		Status           string `json:"status"`
		Progress         int    `json:"progress"`
		CreatedAt        string `json:"createdAt"`
		CompletedAt      string `json:"completedAt,omitempty"`
		FingerprintCount int64  `json:"fingerprintCount"`
		PortCount        int64  `json:"portCount"`
		DirResultCount   int64  `json:"dirResultCount"`
		SubdomainCount   int64  `json:"subdomainCount"`
		VulnCount        int64  `json:"vulnCount"`
		WebVulnCount     int64  `json:"webVulnCount"`
		JSVulnCount      int64  `json:"jsVulnCount"`
		CriticalCount    int64  `json:"criticalCount"`
		HighCount        int64  `json:"highCount"`
		MediumCount      int64  `json:"mediumCount"`
		LowCount         int64  `json:"lowCount"`
		InfoCount        int64  `json:"infoCount"`
		ResultCount      int64  `json:"resultCount"`
	}

	versionsHistory := make([]VersionHistory, 0, len(tasks))

	for _, task := range tasks {
		version := task.Version
		resultTaskIDs, err := combinationResultTaskIDs(db, task, version)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "查询任务结果范围失败: " + err.Error(),
			})
			return
		}
		stats, err := loadVersionHistoryStats(db, task, resultTaskIDs, version)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "统计任务版本结果失败: " + err.Error(),
			})
			return
		}

		totalVulnCount := stats.WebVulnCount + stats.JSVulnCount
		resultCount := stats.FingerprintCount +
			stats.PortCount +
			stats.DirResultCount +
			stats.SubdomainCount +
			totalVulnCount

		// 构建版本历史
		history := VersionHistory{
			Version:          version,
			Status:           task.Status,
			Progress:         task.Progress,
			CreatedAt:        task.CreatedAt.Format(time.RFC3339),
			FingerprintCount: stats.FingerprintCount,
			PortCount:        stats.PortCount,
			DirResultCount:   stats.DirResultCount,
			SubdomainCount:   stats.SubdomainCount,
			VulnCount:        totalVulnCount,
			WebVulnCount:     stats.WebVulnCount,
			JSVulnCount:      stats.JSVulnCount,
			CriticalCount:    stats.CriticalCount,
			HighCount:        stats.HighCount,
			MediumCount:      stats.MediumCount,
			LowCount:         stats.LowCount,
			InfoCount:        stats.InfoCount,
			ResultCount:      resultCount,
		}

		if task.CompletedAt != nil {
			history.CompletedAt = task.CompletedAt.Format(time.RFC3339)
		}

		versionsHistory = append(versionsHistory, history)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"taskId":   taskID,
			"taskName": taskName,
			"scanType": scanType,
			"versions": versionsHistory,
		},
	})
}

// GetWebScanTaskVersionTimeHandler 获取任务指定版本的时间信息（开始时间和结束时间）
func GetWebScanTaskVersionTimeHandler(c *gin.Context) {
	taskID := c.Param("id")
	versionStr := c.Param("version")

	if taskID == "" || versionStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID和版本号不能为空",
		})
		return
	}

	version, err := strconv.Atoi(versionStr)
	if err != nil || version <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "版本号无效",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	// 获取指定版本的任务记录
	var task database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", taskID, version).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "任务版本不存在",
		})
		return
	}

	// 版本开始时间直接使用任务的 CreatedAt（因为每次重新扫描都会创建新记录）
	startTime := task.CreatedAt.Format(time.RFC3339)

	// 版本结束时间：如果任务已完成，使用 completed_at；否则为空
	var endTime string
	if task.CompletedAt != nil {
		endTime = task.CompletedAt.Format(time.RFC3339)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"startTime": startTime,
			"endTime":   endTime,
		},
	})
}

// DeleteVulnerabilityHandler 删除漏洞
func DeleteVulnerabilityHandler(c *gin.Context) {
	vulnID := c.Param("id")
	if vulnID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "漏洞ID不能为空",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	// 检查漏洞是否存在
	var vuln database.VulnerabilityInfo
	if err := db.First(&vuln, "id = ?", vulnID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "漏洞不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询漏洞失败: " + err.Error(),
		})
		return
	}

	// 删除漏洞
	if err := db.Delete(&vuln).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除漏洞失败: " + err.Error(),
		})
		return
	}
	if key, keyErr := findings.ComputeDedupKey(findings.Input{
		Source:      "webscan",
		TaskID:      vuln.TaskID,
		TaskVersion: vuln.Version,
		TemplateID:  vuln.TemplateID,
		Title:       vuln.VulnName,
		Target:      vuln.VulnURL,
		Protocol:    vuln.Protocol,
		Identity:    vuln.TemplateID,
	}); keyErr == nil {
		if statusErr := findings.UpdateStatusByDedupKey(db, key, "ignored"); statusErr != nil && statusErr != gorm.ErrRecordNotFound {
			logger.Error("同步漏洞归因状态失败: %v", statusErr)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "漏洞已删除",
	})
}

type updateVulnerabilityDisposalStatusRequest struct {
	ID             string `json:"id"`
	TaskID         string `json:"taskId"`
	Version        int    `json:"version"`
	TemplateID     string `json:"templateId"`
	VulnURL        string `json:"vulnUrl"`
	Protocol       string `json:"protocol"`
	DisposalStatus string `json:"disposalStatus"`
}

func normalizeVulnerabilityDisposalStatus(raw string) (string, bool) {
	status := strings.ToLower(strings.TrimSpace(raw))
	switch status {
	case "open", "in_progress", "fixed", "false_positive", "ignored":
		return status, true
	default:
		return "", false
	}
}

// UpdateVulnerabilityDisposalStatusHandler 更新漏洞处置状态
func UpdateVulnerabilityDisposalStatusHandler(c *gin.Context) {
	var req updateVulnerabilityDisposalStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数无效: " + err.Error(),
		})
		return
	}

	status, ok := normalizeVulnerabilityDisposalStatus(req.DisposalStatus)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "处置状态无效，仅支持: open, in_progress, fixed, false_positive, ignored",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	locateQuery, err := locateVulnerabilityForStatus(db, req, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	var vuln database.VulnerabilityInfo
	if err := locateQuery.First(&vuln).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "漏洞不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询漏洞失败: " + err.Error(),
		})
		return
	}

	if err := db.Model(&database.VulnerabilityInfo{}).Where("id = ?", vuln.ID).Update("disposal_status", status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新漏洞处置状态失败: " + err.Error(),
		})
		return
	}
	if key, keyErr := findings.ComputeDedupKey(findings.Input{
		Source:      "webscan",
		TaskID:      vuln.TaskID,
		TaskVersion: vuln.Version,
		TemplateID:  vuln.TemplateID,
		Title:       vuln.VulnName,
		Target:      vuln.VulnURL,
		Severity:    vuln.Severity,
		Protocol:    vuln.Protocol,
		Identity:    vuln.TemplateID,
	}); keyErr == nil {
		if statusErr := findings.UpdateStatusByDedupKey(db, key, status); statusErr != nil && statusErr != gorm.ErrRecordNotFound {
			logger.Error("同步漏洞归因状态失败: %v", statusErr)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"disposalStatus": status,
		},
	})
}

// Body IDs are authoritative. Legacy route IDs were template IDs; retain that
// convention only for non-UUID path values accompanied by the old locator.
func locateVulnerabilityForStatus(db *gorm.DB, req updateVulnerabilityDisposalStatusRequest, pathID string) (*gorm.DB, error) {
	id := strings.TrimSpace(req.ID)
	pathID = strings.TrimSpace(pathID)
	if id == "" && pathID != "" {
		if _, err := uuid.Parse(pathID); err == nil {
			id = pathID
		}
	}
	q := db.Model(&database.VulnerabilityInfo{})
	if id != "" {
		q = q.Where("id = ?", id)
		if req.TaskID != "" {
			q = q.Where("task_id = ?", req.TaskID)
		}
		if req.Version > 0 {
			q = q.Where("version = ?", req.Version)
		}
		return q, nil
	}
	templateID := strings.TrimSpace(req.TemplateID)
	if templateID == "" {
		templateID = pathID
	}
	if strings.TrimSpace(req.TaskID) == "" || templateID == "" || strings.TrimSpace(req.VulnURL) == "" {
		return nil, fmt.Errorf("缺少漏洞定位参数，需提供 id 或 taskId/templateId/vulnUrl")
	}
	q = q.Where("task_id = ? AND template_id = ? AND vuln_url = ?", req.TaskID, templateID, req.VulnURL)
	if req.Version > 0 {
		q = q.Where("version = ?", req.Version)
	}
	if req.Protocol != "" {
		q = q.Where("protocol = ?", req.Protocol)
	}
	return q, nil
}
