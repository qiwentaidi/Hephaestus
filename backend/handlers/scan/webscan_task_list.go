package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"hephaestus-backend/handlers/agent"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/datapurge"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListWebScanTasksHandler 获取任务列表
func ListWebScanTasksHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "20")
	limitStr := c.DefaultQuery("limit", "")

	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(pageSizeStr)
	if limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			pageSize = limit
		}
	}
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	scanModeStr := strings.TrimSpace(c.DefaultQuery("scanMode", ""))
	status := strings.TrimSpace(c.DefaultQuery("status", ""))
	search := strings.TrimSpace(c.DefaultQuery("search", ""))

	scanTypeMap := map[string]string{
		"subdomain":   "subdomain",
		"webscan":     "webscan",
		"portscan":    "portscan",
		"dirscan":     "dirscan",
		"jsscan":      "jsscan",
		"crack":       "crack",
		"combination": "combination",
	}

	normalizedScanMode := ""
	if scanModeStr != "" {
		lower := strings.ToLower(scanModeStr)
		if scanType, ok := scanTypeMap[lower]; ok {
			normalizedScanMode = scanType
		} else if lower == "combination" {
			normalizedScanMode = "combination"
		} else {
			normalizedScanMode = lower
		}
	}

	listSQL, listArgs, countSQL, countArgs := buildCombinedTasksQueries(normalizedScanMode, status, search, pageSize, offset)

	var rows []combinedTaskRow
	if err := db.Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询任务列表失败: " + err.Error(),
		})
		return
	}

	taskIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		taskIDs = append(taskIDs, row.ID)
	}
	schedulesByTask := make(map[string]*database.ScanSchedule, len(rows))
	if len(taskIDs) > 0 {
		var scheduleRows []database.ScanSchedule
		if err := db.Where("task_id IN ?", taskIDs).Find(&scheduleRows).Error; err == nil {
			for i := range scheduleRows {
				schedulesByTask[scheduleRows[i].TaskID] = &scheduleRows[i]
			}
		}
	}

	var total int64
	if err := db.Raw(countSQL, countArgs...).Scan(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询任务总数失败: " + err.Error(),
		})
		return
	}

	result := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		scanType := row.ScanType

		var fingerprintCount int64
		var portCount int64
		var vulnerabilityCount int64
		var criticalCount int64
		var highCount int64
		var mediumCount int64
		var lowCount int64
		var infoCount int64
		var subdomainCount int64

		taskVersion := row.Version
		db.Model(&database.FingerprintInfo{}).Where("task_id = ? AND version = ?", row.ID, taskVersion).Count(&fingerprintCount)
		db.Model(&database.PortScanResult{}).Where("task_id = ? AND version = ?", row.ID, taskVersion).Count(&portCount)

		if scanType == "subdomain" {
			db.Model(&database.SubdomainScanResult{}).Where("task_id = ?", row.ID).Count(&subdomainCount)
		} else if scanType == "jsscan" {
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ?", row.ID, taskVersion).Count(&vulnerabilityCount)
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", row.ID, taskVersion, "critical").Count(&criticalCount)
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", row.ID, taskVersion, "high").Count(&highCount)
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", row.ID, taskVersion, "medium").Count(&mediumCount)
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", row.ID, taskVersion, "low").Count(&lowCount)
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", row.ID, taskVersion, "info").Count(&infoCount)
		} else {
			db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ?", row.ID, taskVersion).Count(&vulnerabilityCount)
			db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", row.ID, taskVersion, "critical").Count(&criticalCount)
			db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", row.ID, taskVersion, "high").Count(&highCount)
			db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", row.ID, taskVersion, "medium").Count(&mediumCount)
			db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", row.ID, taskVersion, "low").Count(&lowCount)
			db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", row.ID, taskVersion, "info").Count(&infoCount)
		}

		resultCount := int(fingerprintCount + portCount + vulnerabilityCount + subdomainCount)

		taskName := ""
		if row.TaskName.Valid {
			taskName = row.TaskName.String
		}

		errorMsg := ""
		if row.ErrorMsg.Valid {
			errorMsg = row.ErrorMsg.String
		}

		scheduleConfig := scantask.ScheduleConfig{}
		var nextRunAt *time.Time
		if schedule := schedulesByTask[row.ID]; schedule != nil {
			scheduleConfig, _ = scantask.ScheduleConfigFromScanSchedule(schedule)
			nextRunAt = schedule.NextRunAt
		}
		taskData := map[string]interface{}{
			"id":               row.ID,
			"taskName":         taskName,
			"createdByAI":      row.CreatedByAI,
			"scheduleEnabled":  scheduleConfig.Enabled,
			"scheduleSpec":     scheduleConfig.Spec,
			"status":           row.Status,
			"pauseRequested":   row.PauseRequested,
			"progress":         row.Progress,
			"currentStage":     row.CurrentStage.String,
			"stageEvents":      extractStageEvents(row.CheckpointJSON.String),
			"totalCount":       row.TotalCount,
			"currentCount":     row.CurrentCount,
			"resultCount":      resultCount,
			"errorCount":       0,
			"version":          row.Version,
			"scanType":         scanType,
			"errorMsg":         errorMsg,
			"createdAt":        row.CreatedAt.Format(time.RFC3339),
			"updatedAt":        row.UpdatedAt.Format(time.RFC3339),
			"fingerprintCount": int(fingerprintCount),
			"portCount":        int(portCount),
			"subdomainCount":   int(subdomainCount),
			"vulnCount":        int(vulnerabilityCount),
			"criticalCount":    int(criticalCount),
			"highCount":        int(highCount),
			"mediumCount":      int(mediumCount),
			"lowCount":         int(lowCount),
			"infoCount":        int(infoCount),
		}

		if row.CompletedAt != nil {
			taskData["completedAt"] = row.CompletedAt.Format(time.RFC3339)
		}
		if nextRunAt != nil {
			taskData["nextRunAt"] = nextRunAt.Format(time.RFC3339)
		}

		result = append(result, taskData)
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"tasks":      result,
			"total":      total,
			"page":       page,
			"pageSize":   pageSize,
			"totalPages": totalPages,
		},
	})
}

func buildCombinedTasksQueries(scanMode, status, search string, pageSize, offset int) (string, []interface{}, string, []interface{}) {
	latestTasksCTE := `
SELECT t1.* FROM vul_scan_task t1
INNER JOIN (
	SELECT id, MAX(version) as max_version
	FROM vul_scan_task
	GROUP BY id
) t2 ON t1.id = t2.id AND t1.version = t2.max_version
`

	normalClauses := make([]string, 0)
	normalArgs := make([]interface{}, 0)

	if scanMode != "" {
		normalClauses = append(normalClauses, "t1.scan_type = ?")
		normalArgs = append(normalArgs, scanMode)
	}
	if status != "" {
		normalClauses = append(normalClauses, "t1.status = ?")
		normalArgs = append(normalArgs, status)
	}
	if search != "" {
		pattern := "%" + search + "%"
		normalClauses = append(normalClauses, buildTaskSearchClause())
		normalArgs = append(normalArgs, pattern, pattern)
	}

	normalWhere := ""
	if len(normalClauses) > 0 {
		normalWhere = "WHERE " + strings.Join(normalClauses, " AND ")
	}

	normalSelect := fmt.Sprintf(`
SELECT 
	t1.id,
	t1.task_name,
	t1.created_by_ai,
	t1.scan_type,
	t1.status,
	t1.pause_requested,
	t1.progress,
	t1.current_stage,
	t1.checkpoint_json,
	t1.total_count,
	CASE 
		WHEN t1.total_count <= 0 THEN 0
		ELSE (t1.progress * t1.total_count / 100)
	END AS current_count,
	t1.error_msg,
	t1.created_at,
	t1.updated_at,
	t1.completed_at,
	t1.version
FROM latest_tasks t1
%s`, normalWhere)

	unionSubquery := normalSelect

	listSQL := fmt.Sprintf(`
WITH latest_tasks AS (
%s
),
combined AS (
%s
)
SELECT * FROM combined
ORDER BY created_at DESC
LIMIT ? OFFSET ?`, latestTasksCTE, unionSubquery)

	countSQL := fmt.Sprintf(`
WITH latest_tasks AS (
%s
),
combined AS (
%s
)
SELECT COUNT(*) FROM combined`, latestTasksCTE, unionSubquery)

	listArgs := append([]interface{}{}, normalArgs...)
	listArgs = append(listArgs, pageSize, offset)
	countArgs := append([]interface{}{}, normalArgs...)

	return listSQL, listArgs, countSQL, countArgs
}

func buildTaskSearchClause() string {
	db, err := database.GetDB()
	if err == nil && db != nil && db.Dialector != nil && db.Dialector.Name() == "postgres" {
		return "(t1.task_name ILIKE ? OR t1.id ILIKE ?)"
	}
	return "(LOWER(t1.task_name) LIKE LOWER(?) OR LOWER(t1.id) LIKE LOWER(?))"
}

// GetWebScanTaskHandler 获取任务详情（支持版本参数）
func GetWebScanTaskHandler(c *gin.Context) {
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

	// 获取版本号（如果指定了 version 参数，使用指定版本；否则使用最新版本）
	versionStr := c.DefaultQuery("version", "")
	var task database.VulnScanTask
	query := db.Where("id = ?", taskID)
	requestedVersion := 0
	if parsed, parseErr := strconv.Atoi(versionStr); parseErr == nil && parsed > 0 {
		requestedVersion = parsed
		query = query.Where("version = ?", parsed)
	} else {
		query = query.Order("version DESC")
	}
	if err := query.First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			message := "任务不存在"
			if requestedVersion > 0 {
				message = fmt.Sprintf("任务版本 %d 不存在", requestedVersion)
			}
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询任务失败: " + err.Error()})
		return
	}

	// 解析 targets
	var targets []string
	json.Unmarshal([]byte(task.Targets), &targets)

	// 解析 config
	var config map[string]interface{}
	json.Unmarshal([]byte(task.Config), &config)
	// 确保 config 和 targets
	if config == nil {
		config = make(map[string]interface{})
	}
	config["targets"] = targets

	// 统计结果数量（统计指定版本或最新版本）
	var fingerprintCount int64
	var portCount int64
	var vulnerabilityCount int64
	var criticalCount int64
	var highCount int64
	var mediumCount int64
	var lowCount int64
	var infoCount int64
	var subdomainCount int64

	// 使用当前任务的版本号进行统计
	taskVersion := task.Version
	db.Model(&database.FingerprintInfo{}).Where("task_id = ? AND version = ?", task.ID, taskVersion).Count(&fingerprintCount)
	db.Model(&database.PortScanResult{}).Where("task_id = ? AND version = ?", task.ID, taskVersion).Count(&portCount)

	if task.ScanType == "subdomain" {
		db.Model(&database.SubdomainScanResult{}).Where("task_id = ?", task.ID).Count(&subdomainCount)
	} else if task.ScanType == "jsscan" {
		db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ?", task.ID, taskVersion).Count(&vulnerabilityCount)
		db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", task.ID, taskVersion, "critical").Count(&criticalCount)
		db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", task.ID, taskVersion, "high").Count(&highCount)
		db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", task.ID, taskVersion, "medium").Count(&mediumCount)
		db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", task.ID, taskVersion, "low").Count(&lowCount)
		db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", task.ID, taskVersion, "info").Count(&infoCount)
	} else {
		db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ?", task.ID, taskVersion).Count(&vulnerabilityCount)
		db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", task.ID, taskVersion, "critical").Count(&criticalCount)
		db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", task.ID, taskVersion, "high").Count(&highCount)
		db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", task.ID, taskVersion, "medium").Count(&mediumCount)
		db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", task.ID, taskVersion, "low").Count(&lowCount)
		db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", task.ID, taskVersion, "info").Count(&infoCount)
	}

	if task.ScanType == "combination" && strings.TrimSpace(task.ParentCombinationID) != "" {
		resultTaskIDs, idsErr := combinationResultTaskIDs(db, task, taskVersion)
		if idsErr == nil {
			fingerprintCount = 0
			portCount = 0
			vulnerabilityCount = 0
			criticalCount = 0
			highCount = 0
			mediumCount = 0
			lowCount = 0
			infoCount = 0

			var webVulnerabilityCount int64
			var jsVulnerabilityCount int64

			db.Model(&database.FingerprintInfo{}).Where("task_id IN ? AND version = ?", resultTaskIDs, taskVersion).Count(&fingerprintCount)
			db.Model(&database.PortScanResult{}).Where("task_id IN ? AND version = ?", resultTaskIDs, taskVersion).Count(&portCount)
			db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ?", resultTaskIDs, taskVersion).Count(&webVulnerabilityCount)
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ?", resultTaskIDs, taskVersion).Count(&jsVulnerabilityCount)
			vulnerabilityCount = webVulnerabilityCount + jsVulnerabilityCount

			db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ? AND LOWER(severity) = ?", resultTaskIDs, taskVersion, "critical").Count(&criticalCount)
			db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ? AND LOWER(severity) = ?", resultTaskIDs, taskVersion, "high").Count(&highCount)
			db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ? AND LOWER(severity) = ?", resultTaskIDs, taskVersion, "medium").Count(&mediumCount)
			db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ? AND LOWER(severity) = ?", resultTaskIDs, taskVersion, "low").Count(&lowCount)
			db.Model(&database.VulnerabilityInfo{}).Where("task_id IN ? AND version = ? AND LOWER(severity) = ?", resultTaskIDs, taskVersion, "info").Count(&infoCount)

			var jsCriticalCount int64
			var jsHighCount int64
			var jsMediumCount int64
			var jsLowCount int64
			var jsInfoCount int64
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ? AND LOWER(level) = ?", resultTaskIDs, taskVersion, "critical").Count(&jsCriticalCount)
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ? AND LOWER(level) = ?", resultTaskIDs, taskVersion, "high").Count(&jsHighCount)
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ? AND LOWER(level) = ?", resultTaskIDs, taskVersion, "medium").Count(&jsMediumCount)
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ? AND LOWER(level) = ?", resultTaskIDs, taskVersion, "low").Count(&jsLowCount)
			db.Model(&database.VulnScanJSVulnerability{}).Where("task_id IN ? AND version = ? AND LOWER(level) = ?", resultTaskIDs, taskVersion, "info").Count(&jsInfoCount)
			criticalCount += jsCriticalCount
			highCount += jsHighCount
			mediumCount += jsMediumCount
			lowCount += jsLowCount
			infoCount += jsInfoCount
		}
	}

	resultCount := int(fingerprintCount + portCount + vulnerabilityCount + subdomainCount)

	// 将 taskName 添加到 config 中（为了兼容前端，前端可能从 config.taskName 读取）
	if task.TaskName != "" {
		config["taskName"] = task.TaskName
	}

	// 确定扫描类型（优先使用数据库字段，如果没有则从 config 推断）
	scanType := task.ScanType

	scheduleConfig := scantask.ScheduleConfig{}
	var nextRunAt *time.Time
	if schedule, err := scantask.GetScanSchedule(db, task.ID); err == nil && schedule != nil {
		scheduleConfig, _ = scantask.ScheduleConfigFromScanSchedule(schedule)
		nextRunAt = schedule.NextRunAt
	}

	taskData := map[string]interface{}{
		"id":                  task.ID,
		"taskName":            task.TaskName, // 任务名称作为顶层字段
		"scheduleEnabled":     scheduleConfig.Enabled,
		"scheduleSpec":        scheduleConfig.Spec,
		"createdByAI":         task.CreatedByAI,
		"status":              task.Status,
		"progress":            task.Progress,
		"totalCount":          task.TotalCount,
		"currentCount":        task.Progress * task.TotalCount / 100, // 根据进度计算当前完成数
		"resultCount":         resultCount,
		"errorCount":          0,
		"version":             task.Version, // 当前扫描版本号
		"scanType":            scanType,     // 扫描类型：webscan 或 crack
		"currentStage":        task.CurrentStage,
		"stageEvents":         extractStageEvents(task.CheckpointJSON),
		"stepIndex":           task.StepIndex,
		"stepModule":          task.StepModule,
		"parentCombinationId": task.ParentCombinationID,
		"config":              config,
		"errorMsg":            task.ErrorMsg,
		"createdAt":           task.CreatedAt.Format(time.RFC3339),
		"updatedAt":           task.UpdatedAt.Format(time.RFC3339),
		"fingerprintCount":    int(fingerprintCount),
		"portCount":           int(portCount),
		"subdomainCount":      int(subdomainCount),
		"vulnCount":           int(vulnerabilityCount),
		"criticalCount":       int(criticalCount),
		"highCount":           int(highCount),
		"mediumCount":         int(mediumCount),
		"lowCount":            int(lowCount),
		"infoCount":           int(infoCount),
	}

	if task.CompletedAt != nil {
		taskData["completedAt"] = task.CompletedAt.Format(time.RFC3339)
	}
	if nextRunAt != nil {
		taskData["nextRunAt"] = nextRunAt.Format(time.RFC3339)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    taskData,
	})
}

// StartWebScanTaskHandler 开始任务（将 pending 状态的任务改为 running）
func startScanTaskByID(taskID string) (string, error) {
	if datapurge.IsRunning() {
		return "", fmt.Errorf("数据清理进行中，任务暂不可启动")
	}
	if taskID == "" {
		return "", fmt.Errorf("任务ID不能为空")
	}
	db, err := database.GetDB()
	if err != nil {
		return "", fmt.Errorf("获取数据库连接失败: %w", err)
	}

	return startScanTaskByIDWithDB(db, taskID)
}

func startScanTaskByIDWithDB(db *gorm.DB, taskID string) (string, error) {
	if db == nil || taskID == "" {
		return "", fmt.Errorf("数据库或任务ID不能为空")
	}
	// 获取任务信息（获取最新版本）
	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("任务不存在")
		}
		return "", fmt.Errorf("查询任务失败: %w", err)
	}

	// pending、paused、recovering、cancelled、completed 或 failed 状态的任务可以开始
	// completed/failed 状态的任务重新开始时会重置状态并重新扫描
	if task.Status != taskstate.StatusPending &&
		task.Status != taskstate.StatusPaused &&
		task.Status != taskstate.StatusRecovering &&
		task.Status != taskstate.StatusCancelled &&
		task.Status != taskstate.StatusCompleted &&
		task.Status != taskstate.StatusFailed {
		if task.Status == "running" {
			return "", fmt.Errorf("任务正在运行中，无法重复开始")
		}
		return "", fmt.Errorf("任务状态为 %s，无法开始", task.Status)
	}

	if strings.EqualFold(strings.TrimSpace(task.ScanType), "combination") {
		if err := startCombinationTask(db, &task); err != nil {
			return "", fmt.Errorf("启动组合任务失败: %w", err)
		}
		return "组合任务已开始", nil
	}

	if db.Dialector.Name() == "postgres" && scantask.UsePersistentWorkers(db, task.ScanMode) && (task.Status == taskstate.StatusPaused || task.Status == taskstate.StatusRecovering) {
		if err := scantask.ResetForResume(db, &task); err != nil {
			return "", err
		}
		if err := agent.EnsureAgentTaskItems(db, &task); err != nil {
			return "", err
		}
		return "任务已加入 Worker 队列，等待继续", nil
	}
	if strings.EqualFold(strings.TrimSpace(task.ScanMode), "agent") {
		if task.Status == taskstate.StatusPaused || task.Status == taskstate.StatusRecovering {
			return "", fmt.Errorf("Agent 模式任务暂不支持暂停后继续")
		}
		if task.Status == taskstate.StatusCompleted || task.Status == taskstate.StatusCancelled || task.Status == taskstate.StatusFailed {
			var maxVersion int
			if err := db.Raw(`
				SELECT COALESCE(MAX(version), 0) 
				FROM vul_scan_task 
				WHERE id = ?
			`, taskID).Scan(&maxVersion).Error; err != nil {
				return "", fmt.Errorf("获取最大版本号失败: %w", err)
			}

			newTask := &database.VulnScanTask{
				ID:              taskID,
				TaskName:        task.TaskName,
				CreatedByAI:     task.CreatedByAI,
				ScanMode:        task.ScanMode,
				ScanType:        task.ScanType,
				AssignedAgentID: task.AssignedAgentID,
				WorkerAgentID:   "",
				LeaseExpiresAt:  nil,
				Targets:         task.Targets,
				Status:          taskstate.StatusRunning,
				Progress:        0,
				TotalCount:      task.TotalCount,
				Version:         maxVersion + 1,
				Config:          task.Config,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			if err := db.Create(newTask).Error; err != nil {
				return "", fmt.Errorf("创建新版本记录失败: %w", err)
			}
			if err := agent.EnsureAgentTaskItems(db, newTask); err != nil {
				return "", fmt.Errorf("创建 Agent 任务分片失败: %w", err)
			}
		} else {
			result := db.Model(&database.VulnScanTask{}).
				Where("id = ? AND version = ?", taskID, task.Version).
				Updates(map[string]interface{}{
					"status":           "running",
					"current_stage":    "",
					"checkpoint_json":  "",
					"pause_requested":  false,
					"progress":         0,
					"error_msg":        "",
					"worker_agent_id":  "",
					"started_at":       nil,
					"completed_at":     nil,
					"lease_expires_at": nil,
					"heartbeat_at":     nil,
					"updated_at":       time.Now(),
				})
			if result.Error != nil {
				return "", fmt.Errorf("更新任务失败: %w", result.Error)
			}
			task.Status = taskstate.StatusRunning
			task.Progress = 0
			task.WorkerAgentID = ""
			task.LeaseExpiresAt = nil
			task.StartedAt = nil
			task.CompletedAt = nil
			task.HeartbeatAt = nil
			if err := agent.ResetAgentTaskItems(db, &task); err != nil {
				return "", fmt.Errorf("重置 Agent 任务分片失败: %w", err)
			}
		}
		return "任务已加入 Agent 队列，等待执行", nil
	}

	// 如果是 completed/cancelled/failed 状态的任务重新开始，创建新版本记录
	if task.Status == taskstate.StatusCompleted || task.Status == taskstate.StatusCancelled || task.Status == taskstate.StatusFailed {
		// 获取当前最大版本号（使用 Raw SQL 查询更可靠）
		var maxVersion int
		if err := db.Raw(`
			SELECT COALESCE(MAX(version), 0) 
			FROM vul_scan_task 
			WHERE id = ?
		`, taskID).Scan(&maxVersion).Error; err != nil {
			return "", fmt.Errorf("获取最大版本号失败: %w", err)
		}

		newVersion := maxVersion + 1

		// 创建新版本记录（相同的 task_id，version 递增）
		newTask := &database.VulnScanTask{
			ID:              taskID, // 相同的 task_id
			TaskName:        task.TaskName,
			CreatedByAI:     task.CreatedByAI,
			ScanMode:        task.ScanMode,
			ScanType:        task.ScanType, // 保持相同的扫描类型
			AssignedAgentID: task.AssignedAgentID,
			WorkerAgentID:   "",
			LeaseExpiresAt:  nil,
			Targets:         task.Targets,
			Status:          taskstate.StatusRunning,
			Progress:        0,
			TotalCount:      task.TotalCount,
			Version:         newVersion,
			Config:          task.Config,
			CreatedAt:       time.Now(), // CreatedAt 作为新版本的开始时间
			UpdatedAt:       time.Now(),
		}

		if err := db.Create(newTask).Error; err != nil {
			// 如果是主键冲突错误，可能是并发问题，再次尝试获取最大版本号
			if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "23505") {
				// 重新获取最大版本号
				if err2 := db.Raw(`
					SELECT COALESCE(MAX(version), 0) 
					FROM vul_scan_task 
					WHERE id = ?
				`, taskID).Scan(&maxVersion).Error; err2 == nil {
					newVersion = maxVersion + 1
					newTask.Version = newVersion
					// 再次尝试创建
					if err3 := db.Create(newTask).Error; err3 != nil {
						return "", fmt.Errorf("创建新版本记录失败（重试后）: %w", err3)
					}
				} else {
					return "", fmt.Errorf("创建新版本记录失败: %w", err)
				}
			} else {
				return "", fmt.Errorf("创建新版本记录失败: %w", err)
			}
		}
	} else {
		// pending/paused/recovering 状态的任务，直接恢复当前版本
		if err := scantask.ResetForResume(db, &task); err != nil {
			return "", fmt.Errorf("开始任务失败: %w", err)
		}
	}

	if err := scantask.EnqueueLocalTaskWithDB(db, taskID); err != nil {
		return "", fmt.Errorf("提交任务到队列失败: %w", err)
	}

	return "任务已开始", nil
}

func StartWebScanTaskHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	message, err := startScanTaskByID(taskID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "任务不存在") {
			statusCode = http.StatusNotFound
		} else if strings.Contains(err.Error(), "无法开始") || strings.Contains(err.Error(), "正在运行中") {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
	})
}

func startCombinationTask(db *gorm.DB, task *database.VulnScanTask) error {
	if task == nil {
		return fmt.Errorf("任务不存在")
	}

	if task.Status == taskstate.StatusPaused || task.Status == taskstate.StatusRecovering {
		return resumeCombinationTask(db, task)
	}

	combo, nodes, _, err := getCombinationByParentTask(db, task)
	if err != nil {
		return err
	}

	if strings.EqualFold(strings.TrimSpace(task.ScanMode), "agent") &&
		(task.Status == taskstate.StatusPaused || task.Status == taskstate.StatusRecovering) {
		return fmt.Errorf("Agent 模式组合任务暂不支持暂停后继续")
	}

	var activeTask *database.VulnScanTask
	shouldRebuild := true
	if task.Status == taskstate.StatusCompleted || task.Status == taskstate.StatusCancelled || task.Status == taskstate.StatusFailed {
		activeTask, err = createNextCombinationTaskVersion(db, task)
		if err != nil {
			return err
		}
	} else {
		activeTask = task
		shouldRebuild = task.Status == taskstate.StatusPending
	}

	var ctxState *combinationContext
	if shouldRebuild {
		ctxState, err = rebuildCombinationContext(combo, nodes, activeTask.ID)
		if err != nil {
			return err
		}
	} else {
		ctxState, err = loadCombinationContext(combo)
		if err != nil {
			return err
		}
		ctxState.ensureStepLogs(nodes, activeTask.ID)
	}

	now := time.Now()
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.CombinationTask{}).
			Where("id = ?", combo.ID).
			Updates(map[string]interface{}{
				"context":         ctxState.serialize(),
				"status":          taskstate.StatusPending,
				"pause_requested": false,
				"progress":        0,
				"current_step":    0,
				"total_steps":     len(nodes),
				"error_msg":       "",
				"started_at":      nil,
				"completed_at":    nil,
				"updated_at":      now,
			}).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{
			"status":           taskstate.StatusRunning,
			"scan_type":        "combination",
			"pause_requested":  false,
			"current_stage":    "",
			"checkpoint_json":  "",
			"progress":         0,
			"error_msg":        "",
			"worker_agent_id":  "",
			"lease_expires_at": nil,
			"heartbeat_at":     nil,
			"step_index":       0,
			"step_module":      "",
			"started_at":       nil,
			"completed_at":     nil,
			"updated_at":       now,
		}
		return tx.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", activeTask.ID, activeTask.Version).
			Updates(updates).Error
	}); err != nil {
		return err
	}

	combo.Status = taskstate.StatusPending
	combo.PauseRequested = false
	combo.Progress = 0
	combo.CurrentStep = 0
	combo.TotalSteps = len(nodes)
	combo.Context = ctxState.serialize()
	combo.ErrorMsg = ""
	combo.StartedAt = nil
	combo.CompletedAt = nil
	combo.UpdatedAt = now

	_, err = scheduleNextCombinationNodes(db, combo, nodes, ctxState)
	return err
}

// UpdateWebScanTaskNameHandler 更新任务名称
func UpdateWebScanTaskNameHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	var req struct {
		TaskName string `json:"taskName" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
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

	// 检查任务是否存在（检查是否有任何版本）
	var count int64
	if err := db.Model(&database.VulnScanTask{}).Where("id = ?", taskID).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询任务失败: " + err.Error(),
		})
		return
	}
	if count == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "任务不存在",
		})
		return
	}

	// 更新任务名称（存储在 task_name 列中）
	// 更新所有版本的任务名称（因为 task_id 相同）
	result := db.Model(&database.VulnScanTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"task_name":  req.TaskName,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新任务名称失败: " + result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "任务名称已更新",
	})
}
