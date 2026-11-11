package scantask

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"gorm.io/gorm"

	"hephaestus-backend/pkg/agentruntime"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scanexec"
	"hephaestus-backend/pkg/subdomain"
	"hephaestus-backend/pkg/taskstate"
)

func CreateAndEnqueue(scanType, taskName, scanMode, assignedAgentID string, targets []string, config map[string]interface{}, schedule ScheduleConfig) (string, error) {
	db, err := database.GetDB()
	if err != nil {
		return "", fmt.Errorf("获取数据库连接失败: %w", err)
	}
	return CreateAndEnqueueWithDB(db, scanType, taskName, scanMode, assignedAgentID, targets, config, schedule, false)
}

func CreateAndEnqueueWithDB(db *gorm.DB, scanType, taskName, scanMode, assignedAgentID string, targets []string, config map[string]interface{}, schedule ScheduleConfig, createdByAI bool) (string, error) {
	if db == nil {
		return "", fmt.Errorf("数据库连接不能为空")
	}
	if handler, ok := scanexec.Handlers[scanType]; !ok || handler == nil {
		return "", fmt.Errorf("scanType is not supported or not implemented")
	}

	if len(targets) == 0 {
		return "", fmt.Errorf("targets 不能为空")
	}

	configCopy := make(map[string]interface{})
	if config != nil {
		maps.Copy(configCopy, config)
	}
	configCopy["targets"] = targets
	if scanType == "subdomain" {
		// Resolve defaults before persistence so task details and dispatched work
		// use the same configuration, including for AI-created tasks.
		options, err := subdomain.BuildScanOption(targets, configCopy)
		if err != nil {
			return "", fmt.Errorf("解析子域名配置失败: %w", err)
		}
		configCopy["domains"] = options.Domains
		configCopy["thread"] = options.Thread
		configCopy["timeout"] = options.Timeout
		configCopy["mode"] = options.Mode
		configCopy["appendEngines"] = options.AppendEngines
		if len(options.DnsServers) == 0 {
			options.DnsServers = append([]string(nil), subdomain.DefaultDnsServers...)
		}
		configCopy["dnsServers"] = options.DnsServers
		// Deliberately persist only public scan options, never resolved API keys.
	}
	if scanType == "portscan" {
		if err := ApplyPortScanPortGroupDefaultsWithDB(db, configCopy); err != nil {
			return "", fmt.Errorf("解析端口组失败: %w", err)
		}
	}

	actualScanMode, err := NormalizeModeForDB(db, scanMode)
	if err != nil {
		return "", err
	}
	assignedAgentID = strings.TrimSpace(assignedAgentID)
	if actualScanMode == "local" {
		assignedAgentID = ""
	} else if assignedAgentID != "" {
		var node database.ScanAgentNode
		if err := db.Where("agent_id = ?", assignedAgentID).First(&node).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", fmt.Errorf("指定的 agentId 不存在: %s", assignedAgentID)
			}
			return "", fmt.Errorf("查询指定 agent 失败: %w", err)
		}
		if !node.Enabled {
			return "", fmt.Errorf("指定的 agent 已禁用: %s", assignedAgentID)
		}
		configCopy["agentId"] = assignedAgentID
	}

	if taskName == "" {
		taskName = fmt.Sprintf("%s-%s", scanType, time.Now().Format("20060102150405"))
	}

	scheduleConfig, err := NormalizeScheduleConfig(schedule)
	if err != nil {
		return "", err
	}
	now := time.Now()
	if err := ValidateScheduleConfig(scheduleConfig, now); err != nil {
		return "", err
	}
	taskID := database.GenerateNanoid()

	configJSON, _ := json.Marshal(configCopy)
	targetsJSON, _ := json.Marshal(targets)

	task := &database.VulnScanTask{
		ID:              taskID,
		TaskName:        taskName,
		CreatedByAI:     createdByAI,
		ScanMode:        actualScanMode,
		ScanType:        scanType,
		AssignedAgentID: assignedAgentID,
		Targets:         string(targetsJSON),
		Status:          "pending",
		Progress:        0,
		TotalCount:      len(targets),
		Version:         1,
		Config:          string(configJSON),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Publish the parent and its complete work queue together. If any shard
	// batch fails, no pending task should remain visible to workers or the UI.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return fmt.Errorf("创建任务失败: %w", err)
		}
		// A brand-new task cannot have an existing schedule row, so manual tasks
		// do not need the schedule table at all.
		if scheduleConfig.Spec != nil {
			if err := UpsertScanSchedule(tx, taskID, scheduleConfig, now); err != nil {
				return fmt.Errorf("创建任务计划失败: %w", err)
			}
		}
		if UsePersistentWorkers(tx, actualScanMode) {
			if err := agentruntime.EnsureAgentTaskItems(tx, task); err != nil {
				return fmt.Errorf("创建 Agent 任务分片失败: %w", err)
			}
		}
		return nil
	}); err != nil {
		return "", err
	}

	if scheduleConfig.Spec != nil {
		return taskID, nil
	}

	if UsePersistentWorkers(db, actualScanMode) {
		return taskID, nil
	}

	if err := db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ?", task.ID, task.Version).
		Updates(map[string]interface{}{
			"status":       taskstate.StatusRunning,
			"updated_at":   time.Now(),
			"started_at":   time.Now(),
			"heartbeat_at": time.Now(),
		}).Error; err != nil {
		return "", fmt.Errorf("更新任务状态失败: %w", err)
	}

	if err := EnqueueLocalTaskWithDB(db, taskID); err != nil {
		return "", fmt.Errorf("提交任务到队列失败: %w", err)
	}

	return taskID, nil
}

func CreateExternalScanTask(scanType, taskName, scanMode, assignedAgentID string, targets []string, taskConfig map[string]interface{}) (string, string, string, string, error) {
	db, err := database.GetDB()
	if err != nil {
		return "", "", "", "", fmt.Errorf("获取数据库连接失败: %w", err)
	}
	return CreateExternalScanTaskWithDB(db, scanType, taskName, scanMode, assignedAgentID, targets, taskConfig)
}

func CreateExternalScanTaskWithDB(db *gorm.DB, scanType, taskName, scanMode, assignedAgentID string, targets []string, taskConfig map[string]interface{}) (string, string, string, string, error) {
	return createExternalScanTaskWithDB(db, scanType, taskName, scanMode, assignedAgentID, targets, taskConfig, false)
}

func CreateAIScanTask(scanType, taskName, scanMode, assignedAgentID string, targets []string, taskConfig map[string]interface{}) (string, string, string, string, error) {
	db, err := database.GetDB()
	if err != nil {
		return "", "", "", "", err
	}
	return CreateAIScanTaskWithDB(db, scanType, taskName, scanMode, assignedAgentID, targets, taskConfig)
}

func CreateAIScanTaskWithDB(db *gorm.DB, scanType, taskName, scanMode, assignedAgentID string, targets []string, taskConfig map[string]interface{}) (string, string, string, string, error) {
	return createExternalScanTaskWithDB(db, scanType, taskName, scanMode, assignedAgentID, targets, taskConfig, true)
}

func createExternalScanTaskWithDB(db *gorm.DB, scanType, taskName, scanMode, assignedAgentID string, targets []string, taskConfig map[string]interface{}, createdByAI bool) (string, string, string, string, error) {
	if db == nil {
		return "", "", "", "", fmt.Errorf("数据库连接不能为空")
	}
	actualScanType := strings.ToLower(strings.TrimSpace(scanType))
	if actualScanType == "" {
		actualScanType = "webscan"
	}

	actualScanMode, err := NormalizeModeForDB(db, scanMode)
	if err != nil {
		return "", "", "", "", err
	}
	actualAgentID := strings.TrimSpace(assignedAgentID)
	if actualScanMode == "local" {
		actualAgentID = ""
	}
	taskID, err := CreateAndEnqueueWithDB(db, actualScanType, taskName, actualScanMode, actualAgentID, targets, taskConfig, ScheduleConfig{}, createdByAI)
	if err != nil {
		return "", "", "", "", err
	}
	return taskID, actualScanType, actualScanMode, actualAgentID, nil
}

func CreateExternalWebScanTask(taskName, scanMode, assignedAgentID string, targets []string, taskConfig map[string]interface{}) (string, string, string, error) {
	taskID, _, actualScanMode, actualAgentID, err := CreateExternalScanTask("webscan", taskName, scanMode, assignedAgentID, targets, taskConfig)
	return taskID, actualScanMode, actualAgentID, err
}
