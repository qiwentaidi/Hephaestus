package scan

import (
	"errors"
	"hephaestus-backend/handlers/agent"
	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CancelWebScanTaskHandler 取消任务
func CancelWebScanTaskHandler(c *gin.Context) {
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

	err = scantask.CancelTaskWithDB(db, taskID)
	if err != nil {
		if errors.Is(err, scantask.ErrNotCancelable) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "任务不存在或无法取消",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "取消任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "任务已取消",
	})
}

// DeleteWebScanTaskHandler 删除任务
func DeleteWebScanTaskHandler(c *gin.Context) {
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

	alreadyDeleting, err := markWebScanTaskDeleting(db, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "任务不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "标记任务删除中失败: " + err.Error(),
		})
		return
	}

	if !alreadyDeleting {
		go deleteWebScanTaskAsync(taskID)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"status":  taskstate.StatusDeleting,
		"message": "任务删除中",
	})
}

func markWebScanTaskDeleting(db *gorm.DB, taskID string) (bool, error) {
	alreadyDeleting := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var task database.VulnScanTask
		if err := tx.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
			return err
		}
		if task.Status == taskstate.StatusDeleting {
			alreadyDeleting = true
			return nil
		}

		if err := enqueueAgentTaskCancelCommands(tx, taskID); err != nil {
			return err
		}

		now := time.Now()
		if err := tx.Model(&database.VulnScanTaskItem{}).
			Where("task_id = ? AND status IN ?", taskID, []string{taskstate.StatusPending, taskstate.StatusRunning}).
			Updates(map[string]interface{}{
				"status":           taskstate.StatusCancelled,
				"lease_expires_at": nil,
				"updated_at":       now,
			}).Error; err != nil {
			return err
		}

		return taskstate.MarkDeleting(tx, taskID)
	})
	return alreadyDeleting, err
}

func enqueueAgentTaskCancelCommands(tx *gorm.DB, taskID string) error {
	var runningAgentItems []database.VulnScanTaskItem
	if err := tx.Where(
		"task_id = ? AND status = ? AND worker_agent_id <> ''",
		taskID, taskstate.StatusRunning,
	).Find(&runningAgentItems).Error; err != nil {
		return err
	}

	commandedAgents := make(map[string]struct{})
	for _, item := range runningAgentItems {
		if item.WorkerAgentID == "" {
			continue
		}
		if _, ok := commandedAgents[item.WorkerAgentID]; ok {
			continue
		}
		commandedAgents[item.WorkerAgentID] = struct{}{}
		cmd := database.ScanAgentCommand{
			AgentID:     item.WorkerAgentID,
			TaskID:      item.TaskID,
			TaskVersion: item.Version,
			Command:     "cancel",
			Status:      "pending",
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cmd).Error; err != nil {
			return err
		}
	}
	return nil
}

func deleteWebScanTaskAsync(taskID string) {
	taskController := control.GetTaskController()
	if taskController.IsTaskRunning(taskID) {
		if err := taskController.CancelTask(taskID); err != nil {
			logger.Debug("删除任务时取消任务 context 失败: %v", err)
		}
	}

	db, err := database.GetDB()
	if err != nil {
		logger.Error("删除任务失败: task=%s err=%v", taskID, err)
		return
	}

	if err := deleteWebScanTaskData(db, taskID); err != nil {
		logger.Error("异步删除任务失败: task=%s err=%v", taskID, err)
		now := time.Now()
		if updateErr := db.Model(&database.VulnScanTask{}).
			Where("id = ? AND status = ?", taskID, taskstate.StatusDeleting).
			Updates(map[string]interface{}{
				"status":     taskstate.StatusFailed,
				"error_msg":  "删除任务失败: " + err.Error(),
				"updated_at": now,
			}).Error; updateErr != nil {
			logger.Warning("更新任务删除失败状态失败: task=%s err=%v", taskID, updateErr)
		}
	}
}

func deleteWebScanTaskData(db *gorm.DB, taskID string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var task struct {
			ScanType            string `gorm:"column:scan_type"`
			ParentCombinationID string `gorm:"column:parent_combination_id"`
		}
		if err := tx.Model(&database.VulnScanTask{}).
			Select("scan_type, parent_combination_id").
			Where("id = ?", taskID).
			Order("version DESC").
			First(&task).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// 删除并清理指纹相关的截图、favicon 文件（先查出路径再删除文件）
		var fps []database.FingerprintInfo
		if err := tx.Where("task_id = ?", taskID).Find(&fps).Error; err == nil {
			for _, fp := range fps {
				if fp.Screenshot != "" {
					if err := agent.DeleteUnusedAgentScanAsset(tx, taskID, "screenshot", fp.Screenshot); err != nil {
						logger.Warning("删除截图资产失败: task=%s path=%s err=%v", taskID, fp.Screenshot, err)
					}
				}

				if fp.Favicon != "" {
					if err := agent.DeleteUnusedAgentScanAsset(tx, taskID, "favicon", fp.Favicon); err != nil {
						logger.Warning("删除favicon资产失败: task=%s path=%s err=%v", taskID, fp.Favicon, err)
					}
				}
			}
		}
		// 删除指纹信息
		tx.Where("task_id = ?", taskID).Delete(&database.FingerprintInfo{})
		// 删除漏洞信息
		tx.Where("task_id = ?", taskID).Delete(&database.VulnerabilityInfo{})
		// 删除端口扫描结果
		tx.Where("task_id = ?", taskID).Delete(&database.PortScanResult{})
		tx.Where("task_id = ?", taskID).Delete(&database.PortScanResultAsset{})
		// 删除子域名收集结果；任务统一存储在 vul_scan_task。
		tx.Where("task_id = ?", taskID).Delete(&database.SubdomainScanResult{})
		// 删除 Agent 分片
		tx.Where("task_id = ?", taskID).Delete(&database.VulnScanTaskItem{})
		// 删除计划定义与调度流水，避免孤儿计划再次触发已删除任务
		if err := tx.Where("task_id = ?", taskID).Delete(&database.ScanSchedule{}).Error; err != nil {
			return err
		}
		if err := tx.Where("schedule_id = ?", taskID).Delete(&database.ScanScheduleOccurrence{}).Error; err != nil {
			return err
		}
		// 删除主任务
		if err := tx.Delete(&database.VulnScanTask{}, "id = ?", taskID).Error; err != nil {
			return err
		}
		if task.ScanType == "combination" && task.ParentCombinationID != "" {
			if err := tx.Delete(&database.CombinationTask{}, "id = ?", task.ParentCombinationID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
