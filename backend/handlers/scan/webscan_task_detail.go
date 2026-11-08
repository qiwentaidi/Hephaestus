package scan

import (
	"errors"
	"fmt"
	"hephaestus-backend/pkg/airun"
	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetWebScanTaskItemsHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "任务ID不能为空"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	version, _ := strconv.Atoi(c.DefaultQuery("version", "0"))
	if version <= 0 {
		var task database.VulnScanTask
		if err := db.Select("version").Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "任务不存在"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询任务失败: " + err.Error()})
			return
		}
		version = task.Version
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}

	query := db.Model(&database.VulnScanTaskItem{}).Where("task_id = ? AND version = ?", taskID, version)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if agentID := strings.TrimSpace(c.Query("agentId")); agentID != "" {
		query = query.Where("worker_agent_id = ?", agentID)
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		query = query.Where("target LIKE ?", "%"+search+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "统计分片失败: " + err.Error()})
		return
	}

	var items []database.VulnScanTaskItem
	if err := query.Order("id ASC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询分片失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"list":     items,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
			"version":  version,
		},
	})
}

// GetWebScanTaskFingerprintsHandler 获取任务指纹结果
func GetWebScanTaskFingerprintsHandler(c *gin.Context) {
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
	statusFilter := c.DefaultQuery("status", "")
	highRiskFilter := strings.TrimSpace(c.DefaultQuery("highRisk", ""))
	sortBy := strings.TrimSpace(c.DefaultQuery("sortBy", ""))
	sortOrder := strings.TrimSpace(c.DefaultQuery("sortOrder", ""))

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

	// 构建查询条件
	query := db.Model(&database.FingerprintInfo{}).Where("version = ?", version)
	if task.ScanType == "combination" && strings.TrimSpace(task.ParentCombinationID) != "" {
		resultTaskIDs, err := combinationResultTaskIDs(db, task, version)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "查询子任务失败: " + err.Error(),
			})
			return
		}
		query = query.Where("task_id IN ?", resultTaskIDs)
	} else {
		query = query.Where("task_id = ?", taskID)
	}

	// 添加搜索条件（支持 URL、标题、主机、指纹和 Icon Hash）
	if searchQuery != "" {
		searchPattern := "%" + strings.ToLower(searchQuery) + "%"
		query = query.Where("LOWER(url) LIKE ? OR LOWER(title) LIKE ? OR LOWER(host) LIKE ? OR LOWER(fingerprints) LIKE ? OR LOWER(icon_hash) LIKE ?",
			searchPattern, searchPattern, searchPattern, searchPattern, searchPattern)
	}

	// 添加状态过滤条件
	query = assetstore.ApplyFingerprintStatusFilter(query, statusFilter)

	if highRiskFilter == "with_high_risk" {
		query = query.Where("high_risk_fingerprints IS NOT NULL AND TRIM(high_risk_fingerprints) != '' AND high_risk_fingerprints != '[]' AND LOWER(high_risk_fingerprints) != 'null'")
	} else if highRiskFilter == "without_high_risk" {
		query = query.Where("(high_risk_fingerprints IS NULL OR TRIM(high_risk_fingerprints) = '' OR high_risk_fingerprints = '[]' OR LOWER(high_risk_fingerprints) = 'null')")
	}

	verificationStatusFilter := strings.TrimSpace(c.Query("verificationStatus"))
	if verificationStatusFilter != "" && verificationStatusFilter != "all" {
		if normalizedStatus, ok := normalizeFingerprintVerificationStatus(verificationStatusFilter); ok {
			query = query.Where("verification_status = ?", normalizedStatus)
		}
	}

	// 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询指纹总数失败: " + err.Error(),
		})
		return
	}

	var fingerprints []database.FingerprintInfo
	orderBy := "created_at DESC"
	if sortBy == "length" {
		if strings.EqualFold(sortOrder, "asc") {
			orderBy = "length ASC, created_at DESC"
		} else if strings.EqualFold(sortOrder, "desc") {
			orderBy = "length DESC, created_at DESC"
		}
	}
	// 查询指定版本的数据
	if err := query.Order(orderBy).Offset(offset).Limit(pageSize).Find(&fingerprints).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询指纹结果失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"fingerprints": fingerprints,
			"total":        total,
			"page":         page,
			"pageSize":     pageSize,
		},
	})
}

type fingerprintIconAggregate struct {
	IconHash string `json:"iconHash"`
	Favicon  string `json:"favicon"`
	Count    int64  `json:"count"`
}

// GetWebScanTaskFingerprintIconsHandler returns the most frequently observed
// favicon hashes for a scan task. It is intentionally scoped to the selected
// task and version, so the hover summary remains useful in the task detail.
func GetWebScanTaskFingerprintIconsHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "任务ID不能为空"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	versionStr := c.DefaultQuery("version", "")
	var version int
	var task database.VulnScanTask
	if versionStr != "" {
		if parsed, parseErr := strconv.Atoi(versionStr); parseErr == nil && parsed > 0 {
			version = parsed
		}
	}
	if version == 0 {
		if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "任务不存在"})
			return
		}
		version = task.Version
	}
	if task.ID == "" {
		if err := db.Where("id = ? AND version = ?", taskID, version).First(&task).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "任务不存在"})
			return
		}
	}

	query := db.Model(&database.FingerprintInfo{}).
		Where("version = ? AND icon_hash IS NOT NULL AND TRIM(icon_hash) <> ''", version)
	if task.ScanType == "combination" && strings.TrimSpace(task.ParentCombinationID) != "" {
		resultTaskIDs, err := combinationResultTaskIDs(db, task, version)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询子任务失败: " + err.Error()})
			return
		}
		query = query.Where("task_id IN ?", resultTaskIDs)
	} else {
		query = query.Where("task_id = ?", taskID)
	}

	var icons []fingerprintIconAggregate
	if err := query.
		Select("icon_hash, MIN(favicon) AS favicon, COUNT(*) AS count").
		Group("icon_hash").
		Order("count DESC, icon_hash ASC").
		Limit(10).
		Scan(&icons).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询图标聚合失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"icons": icons}})
}

type updateFingerprintVerificationStatusRequest struct {
	TaskID             string `json:"taskId"`
	Version            int    `json:"version"`
	URL                string `json:"url"`
	VerificationStatus string `json:"verificationStatus"`
}

func normalizeFingerprintVerificationStatus(raw string) (string, bool) {
	status := strings.ToLower(strings.TrimSpace(raw))
	switch status {
	case "pending", "verified", "false_positive", "ignored":
		return status, true
	default:
		return "", false
	}
}

// PauseWebScanTaskHandler 请求暂停任务
func PauseWebScanTaskHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "任务ID不能为空"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "任务不存在"})
		return
	}

	if db.Dialector.Name() != "postgres" && strings.EqualFold(strings.TrimSpace(task.ScanMode), "agent") {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Agent 模式任务暂不支持暂停"})
		return
	}
	// An AI-created scan can be the reason its dialogue is waiting. Stop that
	// producer before changing the scan to paused; otherwise the scan wakeup can
	// requeue the dialogue and it may create another task immediately.
	switch task.Status {
	case taskstate.StatusPending, taskstate.StatusRunning, taskstate.StatusRecovering, "pausing":
		if airun.ConversationIDFromTask(task) != "" || task.CreatedByAI {
			if err := airun.StopConversationForOwnedScan(db, task); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "停止 AI 对话失败: " + err.Error()})
				return
			}
		}
	}

	if strings.EqualFold(strings.TrimSpace(task.ScanType), "combination") {
		if err := pauseCombinationTask(db, &task); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "暂停组合任务失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "暂停请求已提交"})
		return
	}
	if scantask.UsePersistentWorkers(db, task.ScanMode) && db.Dialector.Name() == "postgres" {
		if task.Status != "pending" && task.Status != "running" && task.Status != "recovering" && task.Status != "pausing" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "当前任务不能暂停"})
			return
		}
		if err := scantask.RequestPause(db, task.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "暂停请求已提交"})
		return
	}

	switch task.Status {
	case taskstate.StatusPending:
		if err := taskstate.MarkPaused(db, task.ID, task.Version, "任务已暂停，等待继续"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "暂停任务失败: " + err.Error()})
			return
		}
		logger.Info("暂停待执行任务成功: id=%s version=%d type=%s", task.ID, task.Version, task.ScanType)
	case taskstate.StatusRunning, taskstate.StatusRecovering:
		if err := db.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", task.ID, task.Version).
			Updates(map[string]interface{}{
				"pause_requested": true,
				"updated_at":      time.Now(),
			}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "暂停任务失败: " + err.Error()})
			return
		}
		logger.Info("已提交任务暂停请求: id=%s version=%d type=%s status=%s", task.ID, task.Version, task.ScanType, task.Status)
		if err := control.GetTaskController().CancelTask(task.ID); err != nil {
			logger.Debug("暂停任务时取消任务 context 失败: task=%s err=%v", task.ID, err)
		} else {
			logger.Info("已取消运行中的任务 context: id=%s version=%d type=%s", task.ID, task.Version, task.ScanType)
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": fmt.Sprintf("任务状态为 %s，无法暂停", task.Status)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "暂停请求已提交"})
}

func pauseCombinationTask(db *gorm.DB, task *database.VulnScanTask) error {
	combo, _, _, err := getCombinationByParentTask(db, task)
	if err != nil {
		return err
	}

	now := time.Now()
	switch task.Status {
	case taskstate.StatusPending:
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&database.CombinationTask{}).
				Where("id = ?", combo.ID).
				Updates(map[string]interface{}{
					"status":          taskstate.StatusPaused,
					"pause_requested": false,
					"updated_at":      now,
				}).Error; err != nil {
				return err
			}
			return tx.Model(&database.VulnScanTask{}).
				Where("id = ? AND version = ?", task.ID, task.Version).
				Updates(map[string]interface{}{
					"status":          taskstate.StatusPaused,
					"pause_requested": false,
					"updated_at":      now,
				}).Error
		})
	case taskstate.StatusRunning, taskstate.StatusRecovering:
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&database.CombinationTask{}).
				Where("id = ?", combo.ID).
				Updates(map[string]interface{}{
					"pause_requested": true,
					"updated_at":      now,
				}).Error; err != nil {
				return err
			}
			return tx.Model(&database.VulnScanTask{}).
				Where("id = ? AND version = ?", task.ID, task.Version).
				Updates(map[string]interface{}{
					"pause_requested": true,
					"updated_at":      now,
				}).Error
		})
		if err != nil {
			return err
		}

		// 组合任务的子模块使用独立的 context。仅设置 pause_requested 会让
		// 未及时轮询数据库的扫描器一直运行，父任务也就会长期停在“暂停中”。
		// 状态写入成功后主动取消该任务下全部子 context；运行器会识别已提交的
		// 暂停请求并将组合任务收敛为 paused。
		if err := control.GetTaskController().CancelTask(task.ID); err != nil {
			logger.Debug("暂停组合任务 context 失败: %v", err)
		}
		return nil
	default:
		return fmt.Errorf("任务状态为 %s，无法暂停", task.Status)
	}
}

// ResumeWebScanTaskHandler 恢复已暂停任务
func ResumeWebScanTaskHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "任务ID不能为空"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "任务不存在"})
		return
	}

	if task.Status != taskstate.StatusPaused && task.Status != taskstate.StatusRecovering {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": fmt.Sprintf("任务状态为 %s，无法继续", task.Status)})
		return
	}

	if err := scantask.AgentResumeUnsupported(task); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	if strings.EqualFold(strings.TrimSpace(task.ScanType), "combination") {
		if err := resumeCombinationTask(db, &task); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "恢复组合任务失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "组合任务已恢复"})
		return
	}

	if err := scantask.ResetForResume(db, &task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "恢复任务失败: " + err.Error()})
		return
	}

	if err := scantask.EnqueueLocalTask(task.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "恢复任务入队失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "任务已恢复"})
}

func resumeCombinationTask(db *gorm.DB, task *database.VulnScanTask) error {
	combo, nodes, ctxState, err := getCombinationByParentTask(db, task)
	if err != nil {
		return err
	}
	if len(ctxState.Running) > 0 {
		return fmt.Errorf("组合任务仍在等待子模块完成暂停")
	}

	now := time.Now()
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.CombinationTask{}).
			Where("id = ?", combo.ID).
			Updates(map[string]interface{}{
				"status":          taskstate.StatusRunning,
				"pause_requested": false,
				"error_msg":       "",
				"updated_at":      now,
			}).Error; err != nil {
			return err
		}
		return scantask.ResetForResume(tx, task)
	}); err != nil {
		return err
	}

	combo.Status = taskstate.StatusRunning
	combo.PauseRequested = false
	_, err = scheduleNextCombinationNodes(db, combo, nodes, ctxState)
	return err
}

// UpdateFingerprintVerificationStatusHandler 更新指纹验证状态
func UpdateFingerprintVerificationStatusHandler(c *gin.Context) {
	var req updateFingerprintVerificationStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数无效: " + err.Error(),
		})
		return
	}

	status, ok := normalizeFingerprintVerificationStatus(req.VerificationStatus)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "验证状态无效，仅支持: pending, verified, false_positive, ignored",
		})
		return
	}

	taskID := strings.TrimSpace(req.TaskID)
	url := strings.TrimSpace(req.URL)
	if taskID == "" || url == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "缺少指纹定位参数，需提供 taskId/url",
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

	buildLocateQuery := func(tx *gorm.DB) *gorm.DB {
		q := tx.Where("task_id = ? AND url = ?", taskID, url)
		if req.Version > 0 {
			q = q.Where("version = ?", req.Version)
		}
		return q
	}

	var fingerprint database.FingerprintInfo
	if err := buildLocateQuery(db.Model(&database.FingerprintInfo{})).First(&fingerprint).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "指纹记录不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询指纹记录失败: " + err.Error(),
		})
		return
	}

	if err := buildLocateQuery(db.Model(&database.FingerprintInfo{})).Update("verification_status", status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新指纹验证状态失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"verificationStatus": status,
		},
	})
}

// GetWebScanTaskPortsHandler 获取任务端口结果
func GetWebScanTaskPortsHandler(c *gin.Context) {
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

	limitStr := c.DefaultQuery("limit", "1000")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}

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

	// 从端口扫描结果表中查询（查询指定版本）
	var portResults []database.PortScanResult
	query := db.Where("version = ?", version)
	if task.ScanType == "combination" && strings.TrimSpace(task.ParentCombinationID) != "" {
		resultTaskIDs, err := combinationResultTaskIDs(db, task, version)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "查询子任务失败: " + err.Error(),
			})
			return
		}
		query = query.Where("task_id IN ?", resultTaskIDs)
	} else {
		query = query.Where("task_id = ?", taskID)
	}

	if err := query.Order("created_at DESC").Limit(limit).Find(&portResults).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询端口结果失败: " + err.Error(),
		})
		return
	}

	// 转换为前端需要的格式
	var ports []map[string]interface{}
	for _, result := range portResults {
		portData := map[string]interface{}{
			"host":        result.Host,
			"port":        result.Port,
			"transport":   result.Transport,
			"protocol":    result.Protocol,
			"rawResponse": result.RawResponse,
			"banner":      result.RawResponse, // 兼容前端字段名
			"createdAt":   result.CreatedAt.Format(time.RFC3339),
		}
		ports = append(ports, portData)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"ports": ports,
		},
	})
}
