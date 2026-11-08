package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hephaestus-backend/pkg/agentasset"
	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/nodepython"
	"hephaestus-backend/pkg/notification"
	"hephaestus-backend/pkg/portscan"
	"hephaestus-backend/pkg/scanexec"
	"hephaestus-backend/pkg/taskstate"
)

type TaskPayload struct {
	Generation int64

	ID       string
	Version  int
	ItemID   uint
	AgentID  string
	TaskName string
	ScanType string
	Targets  []string
	Config   map[string]interface{}
}

type NodeUpsertOption struct {
	UpdateProtocol       int
	UpdateOS, UpdateArch string
	HostIdentity         string
	AIWorker             bool
	ScanWorker           *bool
	LocalWorker          bool
	AIConcurrency        int
	ScanConcurrency      int
	NetworkZones         []string

	DisplayName        string
	AgentVersion       string
	LastSeenIP         string
	LastSeenHost       string
	CPUUsagePercent    float64
	MemoryUsageBytes   uint64
	MemoryTotalBytes   uint64
	MemoryUsagePercent float64
	DiskUsageBytes     uint64
	DiskTotalBytes     uint64
	DiskUsagePercent   float64
}

type CommandPayload struct {
	ID          uint
	TaskID      string
	TaskVersion int
	Command     string
}

var errAgentTaskClaimLost = errors.New("agent task claim lost")

const defaultTaskItemMaxRetries = 2

// Keep each INSERT well below PostgreSQL's 65535 bind-parameter limit.
// A target item contains multiple columns, so the target count alone is not
// a safe statement-size limit.
const taskItemCreateBatchSize = 500

func normalizeLeaseTTL(leaseTTL time.Duration) time.Duration {
	if leaseTTL <= 0 {
		return 30 * time.Second
	}
	return leaseTTL
}

func UpsertAgentNode(agentID string, supportedTypes []string, opt NodeUpsertOption) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("agentID 不能为空")
	}

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	now := time.Now()
	if db.Dialector.Name() == "postgres" {
		if err := db.Raw("SELECT CURRENT_TIMESTAMP").Scan(&now).Error; err != nil {
			return err
		}
	}
	displayName := strings.TrimSpace(opt.DisplayName)

	var existing database.ScanAgentNode
	switch err := db.Where("agent_id = ?", agentID).First(&existing).Error; {
	case err == nil:
		if opt.HostIdentity == "" {
			opt.HostIdentity = existing.HostIdentity
		}
		if strings.TrimSpace(existing.DisplayName) != "" {
			displayName = strings.TrimSpace(existing.DisplayName)
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		if displayName == "" {
			displayName = agentID
		}
	default:
		return fmt.Errorf("查询 Agent 节点失败: %w", err)
	}
	if displayName == "" {
		displayName = agentID
	}

	scanEnabled := opt.ScanWorker == nil || *opt.ScanWorker
	aiCapacity, scanCapacity := opt.AIConcurrency, opt.ScanConcurrency
	if aiCapacity <= 0 {
		aiCapacity = 1
	}
	if scanCapacity <= 0 {
		scanCapacity = 1
	}
	typesJSON, _ := json.Marshal(supportedTypes)
	zonesJSON, _ := json.Marshal(opt.NetworkZones)
	node := database.ScanAgentNode{
		UpdateProtocol: opt.UpdateProtocol, UpdateOS: opt.UpdateOS, UpdateArch: opt.UpdateArch,
		HostIdentity: opt.HostIdentity, Retired: existing.Retired,
		AIWorker: opt.AIWorker, ScanWorker: scanEnabled, LocalWorker: opt.LocalWorker,
		AIConcurrency: aiCapacity, ScanConcurrency: scanCapacity, SupportedTypes: string(typesJSON), NetworkZones: string(zonesJSON),
		AgentID:            agentID,
		DisplayName:        displayName,
		AgentVersion:       strings.TrimSpace(opt.AgentVersion),
		Enabled:            true,
		Status:             "online",
		LastHeartbeat:      now,
		LastSeenIP:         strings.TrimSpace(opt.LastSeenIP),
		LastSeenHost:       strings.TrimSpace(opt.LastSeenHost),
		CPUUsagePercent:    opt.CPUUsagePercent,
		MemoryUsageBytes:   opt.MemoryUsageBytes,
		MemoryTotalBytes:   opt.MemoryTotalBytes,
		MemoryUsagePercent: opt.MemoryUsagePercent,
		DiskUsageBytes:     opt.DiskUsageBytes,
		DiskTotalBytes:     opt.DiskTotalBytes,
		DiskUsagePercent:   opt.DiskUsagePercent,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := upsertNodeRecord(db, node); err != nil {
		return err
	}
	nodepython.Poll(db, node)
	return nil
}

func upsertNodeRecord(db *gorm.DB, node database.ScanAgentNode) error {
	updates := map[string]interface{}{
		"update_protocol": node.UpdateProtocol, "update_os": node.UpdateOS, "update_arch": node.UpdateArch,
		"host_identity": node.HostIdentity, "retired": node.Retired,
		"ai_worker": node.AIWorker, "scan_worker": node.ScanWorker, "local_worker": node.LocalWorker,
		"ai_concurrency": node.AIConcurrency, "scan_concurrency": node.ScanConcurrency,
		"supported_types": node.SupportedTypes, "network_zones": node.NetworkZones,
		"display_name":         node.DisplayName,
		"agent_version":        node.AgentVersion,
		"status":               "online",
		"last_heartbeat":       node.LastHeartbeat,
		"last_seen_ip":         node.LastSeenIP,
		"last_seen_host":       node.LastSeenHost,
		"cpu_usage_percent":    node.CPUUsagePercent,
		"memory_usage_bytes":   node.MemoryUsageBytes,
		"memory_total_bytes":   node.MemoryTotalBytes,
		"memory_usage_percent": node.MemoryUsagePercent,
		"disk_usage_bytes":     node.DiskUsageBytes,
		"disk_total_bytes":     node.DiskTotalBytes,
		"disk_usage_percent":   node.DiskUsagePercent,
		"updated_at":           node.UpdatedAt,
	}
	values := make(map[string]interface{}, len(updates)+3)
	for key, value := range updates {
		values[key] = value
	}
	values["agent_id"], values["enabled"], values["created_at"] = node.AgentID, node.Enabled, node.CreatedAt
	// A map preserves explicit false roles instead of GORM's default:true
	// substituting Scan Worker on the first registration of an AI-only node.
	return db.Model(&database.ScanAgentNode{}).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "agent_id"}}, DoUpdates: clause.Assignments(updates)}).Create(values).Error

}

func SetAgentNodeOffline(agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	return db.Model(&database.ScanAgentNode{}).
		Where("agent_id = ?", agentID).
		Updates(map[string]interface{}{
			"status":     "offline",
			"updated_at": time.Now(),
		}).Error
}

func HeartbeatAgent(agentID string, supportedTypes []string, leaseTTL time.Duration, opt NodeUpsertOption) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("agentID 不能为空")
	}

	leaseTTL = normalizeLeaseTTL(leaseTTL)
	if err := UpsertAgentNode(agentID, supportedTypes, opt); err != nil {
		return err
	}

	db, err := database.GetDB()
	if err != nil {
		return err
	}
	if db.Dialector.Name() == "postgres" {
		return nil
	} // Each running item renews its own generation.
	leaseUntil := time.Now().Add(leaseTTL)
	if err := db.Model(&database.VulnScanTask{}).
		Where("scan_mode = ? AND status = ? AND worker_agent_id = ?", "agent", "running", agentID).
		Updates(map[string]interface{}{
			"lease_expires_at": &leaseUntil,
			"updated_at":       time.Now(),
		}).Error; err != nil {
		return err
	}
	return db.Model(&database.VulnScanTaskItem{}).
		Where("status = ? AND worker_agent_id = ?", "running", agentID).
		Updates(map[string]interface{}{
			"lease_expires_at": &leaseUntil,
			"updated_at":       time.Now(),
		}).Error
}

func EnsureAgentTaskItems(db *gorm.DB, task *database.VulnScanTask) error {
	if db == nil || task == nil {
		return fmt.Errorf("任务不能为空")
	}
	if !usesWorkerScanQueue(db, task.ScanMode) {
		return nil
	}

	var existing int64
	if err := db.Model(&database.VulnScanTaskItem{}).
		Where("task_id = ? AND version = ?", task.ID, task.Version).
		Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}

	var targets []string
	if err := json.Unmarshal([]byte(task.Targets), &targets); err != nil {
		return fmt.Errorf("解析任务目标失败: %w", err)
	}
	now := time.Now()
	items := make([]database.VulnScanTaskItem, 0, len(targets))
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		items = append(items, database.VulnScanTaskItem{
			TaskID:     task.ID,
			Version:    task.Version,
			ScanType:   strings.ToLower(strings.TrimSpace(task.ScanType)),
			Target:     target,
			Status:     "pending",
			MaxRetries: defaultTaskItemMaxRetries,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if len(items) == 0 {
		return fmt.Errorf("任务目标为空")
	}
	// Workers may call this for an existing task outside a creation transaction.
	// Never leave a partial target set that a later retry would treat as complete.
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.CreateInBatches(&items, taskItemCreateBatchSize).Error; err != nil {
			return err
		}
		return tx.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", task.ID, task.Version).
			Updates(map[string]interface{}{
				"total_count": len(items),
				"updated_at":  now,
			}).Error
	})
}

func ResetAgentTaskItems(db *gorm.DB, task *database.VulnScanTask) error {
	if db == nil || task == nil {
		return fmt.Errorf("任务不能为空")
	}
	if !usesWorkerScanQueue(db, task.ScanMode) {
		return nil
	}
	if err := db.Where("task_id = ? AND version = ?", task.ID, task.Version).
		Delete(&database.VulnScanTaskItem{}).Error; err != nil {
		return err
	}
	return EnsureAgentTaskItems(db, task)
}

func ensureNextPendingAgentTaskItems(db *gorm.DB, agentID string, supportedTypes []string) (bool, error) {
	var task database.VulnScanTask
	query := db.Model(&database.VulnScanTask{}).
		Where("scan_mode IN ? AND status = ?", workerScanModes(db, agentID), "pending").
		Where("(COALESCE(assigned_agent_id, '') = '' OR assigned_agent_id = ?)", agentID).
		Where("NOT EXISTS (?)",
			db.Model(&database.VulnScanTaskItem{}).
				Select("1").
				Where("vul_scan_task_item.task_id = vul_scan_task.id").
				Where("vul_scan_task_item.version = vul_scan_task.version"),
		)
	if len(supportedTypes) > 0 {
		query = query.Where("scan_type IN ?", supportedTypes)
	}
	if err := query.Order("created_at ASC").Order("version ASC").First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if err := EnsureAgentTaskItems(db, &task); err != nil {
		return false, err
	}
	return true, nil
}

func ClaimNextAgentTask(agentID string, supportedTypes []string, leaseTTL time.Duration) (*TaskPayload, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agentID 不能为空")
	}

	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	var node database.ScanAgentNode
	if err := db.Where("agent_id = ?", agentID).First(&node).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("查询 Agent 节点失败: %w", err)
	}
	return claimNextAgentTaskWithDB(db, node, supportedTypes, leaseTTL)
}
func claimNextAgentTaskWithDB(db *gorm.DB, node database.ScanAgentNode, supportedTypes []string, leaseTTL time.Duration) (*TaskPayload, error) {
	agentID := node.AgentID
	if !node.ScanWorker {
		return nil, nil
	}
	if len(supportedTypes) == 0 {
		_ = json.Unmarshal([]byte(node.SupportedTypes), &supportedTypes)
	}
	if !node.Enabled || node.UpdatePending {
		return nil, nil
	}

	leaseTTL = normalizeLeaseTTL(leaseTTL)
	now := time.Now()
	leaseUntil := now.Add(leaseTTL)

	type claimedItem struct {
		ItemID     uint
		TaskID     string
		Version    int
		TaskName   string
		Generation int64
		ScanType   string
		Target     string
		Config     string
	}
	var claimed claimedItem
	for {
		txErr := db.Transaction(func(tx *gorm.DB) error {
			if tx.Dialector.Name() == "postgres" {
				if err := tx.Raw("SELECT CURRENT_TIMESTAMP").Scan(&now).Error; err != nil {
					return err
				}
				leaseUntil = now.Add(leaseTTL)
				var current database.ScanAgentNode
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("agent_id=?", agentID).First(&current).Error; err != nil {
					return err
				}
				if !current.Enabled || current.UpdatePending || !current.ScanWorker || current.Status != "online" || current.LastHeartbeat.Before(now.Add(-30*time.Second)) {
					return errWorkerCapacityFull
				}
				var count int64
				if err := tx.Model(&database.VulnScanTaskItem{}).Where("worker_agent_id=? AND status='running' AND lease_expires_at>CURRENT_TIMESTAMP", agentID).Count(&count).Error; err != nil {
					return err
				}
				if count >= int64(current.ScanConcurrency) {
					return errWorkerCapacityFull
				}
			}
			query := tx.Table("vul_scan_task_item AS item").
				Select("item.id AS item_id, task.id AS task_id, task.version, task.task_name, item.scan_type, item.target, task.config, item.worker_generation AS generation").
				Joins("JOIN vul_scan_task AS task ON task.id = item.task_id AND task.version = item.version").
				Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where("task.scan_mode IN ?", workerScanModes(tx, agentID)).
				Where("task.pause_requested = ?", false).
				Where("task.status = ? OR NOT EXISTS (SELECT 1 FROM scan_schedule AS schedule WHERE schedule.task_id = task.id)", "running").
				Where("task.status IN ?", []string{"pending", "running"}).
				Where("(COALESCE(task.assigned_agent_id, '') = '' OR task.assigned_agent_id = ?)", agentID).
				Where(
					"(item.status = ? OR (item.status = ? AND item.lease_expires_at IS NOT NULL AND item.lease_expires_at < ?))",
					"pending", "running", now,
				)

			if len(supportedTypes) > 0 {
				query = query.Where("item.scan_type IN ?", supportedTypes)
			}
			if tx.Dialector.Name() == "postgres" {
				query = query.Where(`COALESCE(task.config::jsonb->>'networkZone','')='' OR jsonb_exists(COALESCE(?::jsonb,'[]'::jsonb), task.config::jsonb->>'networkZone')`, node.NetworkZones)
				if node.LocalWorker {
					// An explicit assignment to this node is a pin, not an auto
					// task: remote availability must not delay a task the user
					// deliberately placed on the local agent.
					query = query.Where(`task.scan_mode='local' OR task.assigned_agent_id = ? OR NOT EXISTS (
 SELECT 1 FROM scan_agent_node remote WHERE remote.local_worker=false AND remote.scan_worker=true AND remote.enabled=true AND remote.status='online' AND remote.last_heartbeat>CURRENT_TIMESTAMP-INTERVAL '30 seconds'
 AND (remote.supported_types IS NULL OR remote.supported_types IN ('[]','null') OR jsonb_exists(remote.supported_types::jsonb,item.scan_type))
 AND (COALESCE(task.config::jsonb->>'networkZone','')='' OR jsonb_exists(remote.network_zones::jsonb,task.config::jsonb->>'networkZone'))
 AND remote.scan_concurrency>(SELECT COUNT(*) FROM vul_scan_task_item occupied WHERE occupied.worker_agent_id=remote.agent_id AND occupied.status='running' AND occupied.lease_expires_at>CURRENT_TIMESTAMP))`, agentID)
				}
			}

			if err := query.
				Order("CASE WHEN item.status = 'pending' THEN 0 ELSE 1 END").
				Order("item.created_at ASC").
				Order("item.id ASC").
				Limit(1).
				Scan(&claimed).Error; err != nil {
				return err
			}
			if claimed.ItemID == 0 {
				return gorm.ErrRecordNotFound
			}

			updateFields := map[string]interface{}{
				"worker_generation": claimed.Generation + 1,
				"status":            "running",
				"worker_agent_id":   agentID,
				"lease_expires_at":  &leaseUntil,
				"error_msg":         "",
				"started_at":        &now,
				"updated_at":        now,
			}
			claimUpdate := tx.Model(&database.VulnScanTaskItem{}).
				Where("id = ?", claimed.ItemID).
				Where(
					"(status = ? OR (status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ?))",
					"pending", "running", now,
				).
				Updates(updateFields)
			if claimUpdate.Error != nil {
				return claimUpdate.Error
			}
			if claimUpdate.RowsAffected == 0 {
				return errAgentTaskClaimLost
			}

			parentUpdates := map[string]interface{}{
				"status":           "running",
				"worker_agent_id":  agentID,
				"lease_expires_at": &leaseUntil,
				"error_msg":        "",
				"started_at":       &now,
				"updated_at":       now,
			}
			return tx.Model(&database.VulnScanTask{}).
				Where("id = ? AND version = ? AND status IN ?", claimed.TaskID, claimed.Version, []string{"pending", "running"}).
				Updates(parentUpdates).Error
		})

		if errors.Is(txErr, errWorkerCapacityFull) {
			return nil, nil
		}
		if txErr == nil {
			break
		}
		if errors.Is(txErr, errAgentTaskClaimLost) {
			continue
		}
		if errors.Is(txErr, gorm.ErrRecordNotFound) {
			seeded, seedErr := ensureNextPendingAgentTaskItems(db, agentID, supportedTypes)
			if seedErr != nil {
				return nil, fmt.Errorf("初始化 Agent 任务分片失败: %w", seedErr)
			}
			if seeded {
				continue
			}
			return nil, nil
		}
		return nil, fmt.Errorf("claim agent 任务失败: %w", txErr)
	}

	config := make(map[string]interface{})
	if strings.TrimSpace(claimed.Config) != "" {
		if err := json.Unmarshal([]byte(claimed.Config), &config); err != nil {
			_ = db.Model(&database.VulnScanTask{}).
				Where("id = ? AND version = ?", claimed.TaskID, claimed.Version).
				Updates(map[string]interface{}{
					"status":           "failed",
					"error_msg":        "解析任务配置失败: " + err.Error(),
					"lease_expires_at": nil,
					"updated_at":       time.Now(),
				}).Error
			return nil, fmt.Errorf("解析任务配置失败: %w", err)
		}
	}

	return &TaskPayload{
		Generation: claimed.Generation + 1,
		ID:         claimed.TaskID,
		Version:    claimed.Version,
		ItemID:     claimed.ItemID,
		AgentID:    agentID,
		TaskName:   claimed.TaskName,
		ScanType:   claimed.ScanType,
		Targets:    []string{claimed.Target},
		Config:     config,
	}, nil
}

func ClaimPendingAgentCommands(agentID string, limit int) ([]CommandPayload, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agentID 不能为空")
	}
	if limit <= 0 {
		limit = 20
	}

	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var commands []database.ScanAgentCommand
	now := time.Now()
	txErr := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.ScanAgentCommand{}).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("agent_id = ? AND status = ?", agentID, "pending").
			Order("created_at ASC").
			Limit(limit).
			Find(&commands).Error; err != nil {
			return err
		}
		if len(commands) == 0 {
			return nil
		}

		ids := make([]uint, 0, len(commands))
		for _, cmd := range commands {
			ids = append(ids, cmd.ID)
		}
		claimUpdate := tx.Model(&database.ScanAgentCommand{}).
			Where("id IN ?", ids).
			Where("agent_id = ? AND status = ?", agentID, "pending").
			Updates(map[string]interface{}{
				"status":        "dispatched",
				"dispatched_at": &now,
				"updated_at":    now,
			})
		if claimUpdate.Error != nil {
			return claimUpdate.Error
		}
		if claimUpdate.RowsAffected == 0 {
			commands = nil
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	out := make([]CommandPayload, 0, len(commands))
	for _, cmd := range commands {
		out = append(out, CommandPayload{
			ID:          cmd.ID,
			TaskID:      cmd.TaskID,
			TaskVersion: cmd.TaskVersion,
			Command:     cmd.Command,
		})
	}
	return out, nil
}

func FinishAgentCommand(commandID uint, status, resultMsg string) error {
	if commandID == 0 {
		return fmt.Errorf("commandID 不能为空")
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = "done"
	}
	if status != "done" && status != "failed" {
		status = "done"
	}
	now := time.Now()

	db, err := database.GetDB()
	if err != nil {
		return err
	}

	return db.Model(&database.ScanAgentCommand{}).
		Where("id = ?", commandID).
		Updates(map[string]interface{}{
			"status":     status,
			"result_msg": strings.TrimSpace(resultMsg),
			"acked_at":   &now,
			"updated_at": now,
		}).Error
}

func AggregateAgentTaskProgress(db *gorm.DB, taskID string, version int) error {
	if db == nil {
		return fmt.Errorf("数据库连接不能为空")
	}

	shouldSync := false
	err := db.Transaction(func(db *gorm.DB) error {
		var current database.VulnScanTask
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND version = ?", taskID, version).First(&current).Error; err != nil {
			return err
		}
		if current.Status == "cancelled" || current.Status == "deleting" {
			return nil
		}

		type statusCount struct {
			Status  string
			Count   int64
			NoHosts int64
		}
		var counts []statusCount
		query := db.Model(&database.VulnScanTaskItem{}).Where("task_id = ? AND version = ?", taskID, version)
		combination := current.ParentCombinationID != ""
		scanType := current.ScanType
		if combination {
			scanType = current.StepModule
			query = query.Where("scan_type = ?", current.StepModule)
		}
		columns := "status, COUNT(*) AS count"
		if scanType == "portscan" {
			columns += ", COUNT(CASE WHEN port_scan_no_hosts THEN 1 END) AS no_hosts"
		}
		if err := query.Select(columns).
			Group("status").
			Scan(&counts).Error; err != nil {
			return err
		}

		var total, completed, failed, cancelled, noHosts int64
		for _, item := range counts {
			total += item.Count
			switch item.Status {
			case "completed":
				completed += item.Count
				noHosts += item.NoHosts
			case "failed":
				failed += item.Count
			case "cancelled":
				cancelled += item.Count
			}
		}
		if total == 0 {
			return nil
		}

		if current.Status == "cancelling" || current.PauseRequested {
			var running int64
			if err := db.Model(&database.VulnScanTaskItem{}).Where("task_id=? AND version=? AND status='running'", taskID, version).Count(&running).Error; err != nil {
				return err
			}
			status := "cancelling"
			if current.PauseRequested {
				status = "pausing"
			}
			if running == 0 {
				status = "cancelled"
				if current.PauseRequested {
					status = "paused"
				}
			}
			return db.Model(&database.VulnScanTask{}).Where("id=? AND version=?", taskID, version).Updates(map[string]any{"status": status, "updated_at": time.Now()}).Error
		}

		terminal := completed + failed + cancelled
		progress := int((terminal * 100) / total)
		if progress > 100 {
			progress = 100
		}
		now := time.Now()
		var finalCheckpoint *taskstate.Checkpoint
		if terminal >= total && scanType == "portscan" {
			checkpoint := taskstate.ScanCheckpoint(current, "portscan", combination)
			if finishWorkerPortScanStages(&checkpoint, noHosts == total, now) {
				finalCheckpoint = &checkpoint
			}
		}
		if combination {
			stage := current.StepModule + ":agent"
			if terminal >= total {
				stage = current.StepModule + ":completed"
			}
			meta := map[string]interface{}{
				"totalCount": total, "currentCount": terminal, "errorCount": failed,
			}
			if finalCheckpoint != nil {
				meta["stageEvents"] = finalCheckpoint.Meta["stageEvents"]
			}
			return taskstate.SaveCombinationCheckpoint(db, taskID, version, current.StepModule, stage, progress, meta)
		}

		updates := map[string]interface{}{
			"status":           "running",
			"progress":         progress,
			"total_count":      total,
			"error_count":      failed,
			"heartbeat_at":     now,
			"lease_expires_at": nil,
			"updated_at":       now,
		}
		if terminal >= total {
			shouldSync = current.Status != "completed"
			updates["status"] = "completed"
			updates["progress"] = 100
			updates["completed_at"] = &now
			updates["heartbeat_at"] = nil
			updates["worker_agent_id"] = ""
			if finalCheckpoint != nil {
				updates["checkpoint_json"] = taskstate.MarshalCheckpoint(taskstate.StatusCompleted, 100, finalCheckpoint.Meta)
			}
		}

		return db.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ? AND status <> ?", taskID, version, "cancelled").
			Updates(updates).Error
	})
	if err != nil {
		return err
	}
	if shouldSync {
		if _, syncErr := assetstore.SyncCompletedTask(context.Background(), db, taskID, version); syncErr != nil {
			logger.Warning("[agent] 统一资产目录同步失败: task=%s version=%d err=%v", taskID, version, syncErr)
		}
	}
	return nil
}

func ExecuteAgentTask(payload *TaskPayload) error {
	return ExecuteAgentTaskWithContext(context.Background(), payload)
}

func ExecuteAgentTaskWithContext(ctx context.Context, payload *TaskPayload) error {
	if payload == nil {
		return fmt.Errorf("payload 不能为空")
	}

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	return executeAgentTaskWithDB(ctx, db, payload)
}

func executeAgentTaskWithDB(ctx context.Context, db *gorm.DB, payload *TaskPayload) error {
	handler, ok := scanexec.GetHandler(payload.ScanType)
	if !ok || handler == nil {
		return fmt.Errorf("scanType 不支持: %s", payload.ScanType)
	}

	if payload.ItemID > 0 && db.Dialector.Name() == "postgres" {
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					pollCtx, stopPoll := context.WithTimeout(runCtx, 2*time.Second)
					pollDB := db.WithContext(pollCtx)
					var parent database.VulnScanTask
					err := pollDB.Select("status", "pause_requested").Where("id=? AND version=?", payload.ID, payload.Version).First(&parent).Error
					if err != nil || parent.PauseRequested || (parent.Status != "running" && parent.Status != "pending") {
						stopPoll()
						cancel()
						return
					}
					result := pollDB.Model(&database.VulnScanTaskItem{}).Where("id=? AND worker_agent_id=? AND worker_generation=? AND status='running' AND lease_expires_at>CURRENT_TIMESTAMP", payload.ItemID, payload.AgentID, payload.Generation).Updates(map[string]any{"lease_expires_at": gorm.Expr("CURRENT_TIMESTAMP + INTERVAL '30 seconds'"), "updated_at": gorm.Expr("CURRENT_TIMESTAMP")})
					stopPoll()
					if result.Error != nil || result.RowsAffected != 1 {
						cancel()
						return
					}
				}
			}
		}()
		defer func() { cancel(); <-done }()
		ctx = runCtx
	}

	if payload.ItemID > 0 {
		itemConfig, execErr := materializeScanEvidence(ctx, db, payload)
		if itemConfig == nil {
			itemConfig = map[string]interface{}{}
		}
		itemConfig["agentItemMode"] = true
		itemConfig["agentItemID"] = payload.ItemID
		portScanNoHosts := false
		if payload.ScanType == "portscan" {
			ctx = portscan.WithStageReporter(ctx, func(ctx context.Context, stage, label string) error {
				if stage == portscan.StageNoHosts {
					portScanNoHosts = true
					return nil
				}
				return saveWorkerPortScanStage(ctx, db, payload, stage, label)
			})
		}
		if execErr == nil {
			execErr = handler(ctx, payload.ID, payload.Targets, itemConfig)
		}
		now := time.Now()
		if execErr != nil {
			if errors.Is(execErr, context.Canceled) || ctx.Err() == context.Canceled {
				result := db.Model(&database.VulnScanTaskItem{}).
					Where("id = ? AND worker_agent_id = ? AND worker_generation = ? AND lease_expires_at>CURRENT_TIMESTAMP", payload.ItemID, payload.AgentID, payload.Generation).
					Updates(map[string]interface{}{
						"status":           cancelledItemStatus(db, payload.ID, payload.Version),
						"error_msg":        "任务已取消",
						"lease_expires_at": nil,
						"completed_at":     &now,
						"updated_at":       now,
					})
				if result.Error == nil && result.RowsAffected > 0 {
					_ = AggregateAgentTaskProgress(db, payload.ID, payload.Version)
				}
				return execErr
			}

			updates := map[string]interface{}{
				"error_msg":        execErr.Error(),
				"lease_expires_at": nil,
				"updated_at":       now,
			}
			var item database.VulnScanTaskItem
			if err := db.Select("retry_count", "max_retries").Where("id = ?", payload.ItemID).First(&item).Error; err == nil && item.RetryCount < item.MaxRetries {
				updates["status"] = "pending"
				updates["worker_agent_id"] = ""
				updates["retry_count"] = item.RetryCount + 1
			} else {
				updates["status"] = "failed"
				updates["completed_at"] = &now
			}
			result := db.Model(&database.VulnScanTaskItem{}).
				Where("id = ? AND worker_agent_id = ? AND worker_generation = ? AND lease_expires_at>CURRENT_TIMESTAMP", payload.ItemID, payload.AgentID, payload.Generation).
				Updates(updates)
			if result.Error == nil && result.RowsAffected > 0 {
				_ = AggregateAgentTaskProgress(db, payload.ID, payload.Version)
			}
			return execErr
		}

		var current database.VulnScanTask
		if err := db.Select("status").Where("id = ? AND version = ?", payload.ID, payload.Version).First(&current).Error; err == nil && current.Status == "cancelled" {
			result := db.Model(&database.VulnScanTaskItem{}).
				Where("id = ? AND worker_agent_id = ? AND worker_generation = ? AND lease_expires_at>CURRENT_TIMESTAMP", payload.ItemID, payload.AgentID, payload.Generation).
				Updates(map[string]interface{}{
					"status":           "cancelled",
					"lease_expires_at": nil,
					"completed_at":     &now,
					"updated_at":       now,
				})
			if result.Error == nil && result.RowsAffected > 0 {
				_ = AggregateAgentTaskProgress(db, payload.ID, payload.Version)
			}
			return context.Canceled
		}

		updates := map[string]interface{}{
			"status":           "completed",
			"error_msg":        "",
			"lease_expires_at": nil,
			"completed_at":     &now,
			"updated_at":       now,
		}
		if payload.ScanType == "portscan" {
			updates["port_scan_no_hosts"] = portScanNoHosts
		}
		result := db.Model(&database.VulnScanTaskItem{}).
			Where("id = ? AND worker_agent_id = ? AND worker_generation = ? AND lease_expires_at>CURRENT_TIMESTAMP", payload.ItemID, payload.AgentID, payload.Generation).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := agentasset.SyncTaskAssetFilesWithContext(ctx, db, payload.ID, payload.Version); err != nil {
			logger.Warning("[agent] 任务产物同步失败: id=%s version=%d item=%d err=%v", payload.ID, payload.Version, payload.ItemID, err)
		}
		_ = AggregateAgentTaskProgress(db, payload.ID, payload.Version)
		return nil
	}

	if payload.Config == nil {
		payload.Config = map[string]interface{}{}
	}
	execErr := handler(ctx, payload.ID, payload.Targets, payload.Config)
	if execErr != nil {
		now := time.Now()
		if errors.Is(execErr, context.Canceled) || ctx.Err() == context.Canceled {
			_ = db.Model(&database.VulnScanTask{}).
				Where("id = ? AND version = ?", payload.ID, payload.Version).
				Updates(map[string]interface{}{
					"status":           "cancelled",
					"error_msg":        "任务已取消",
					"lease_expires_at": nil,
					"updated_at":       now,
				}).Error
			return execErr
		}
		_ = db.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", payload.ID, payload.Version).
			Updates(map[string]interface{}{
				"status":           "failed",
				"error_msg":        execErr.Error(),
				"lease_expires_at": nil,
				"updated_at":       now,
			}).Error
		go notification.NotifyScanComplete(payload.ID, payload.TaskName, 0, "failed")
		return execErr
	}

	var current database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", payload.ID, payload.Version).First(&current).Error; err == nil {
		if current.Status == "cancelled" {
			_ = db.Model(&database.VulnScanTask{}).
				Where("id = ? AND version = ?", payload.ID, payload.Version).
				Updates(map[string]interface{}{
					"lease_expires_at": nil,
					"updated_at":       time.Now(),
				}).Error
			return context.Canceled
		}
	}

	if err := agentasset.SyncTaskAssetFilesWithContext(ctx, db, payload.ID, payload.Version); err != nil {
		logger.Warning("[agent] 任务产物同步失败: id=%s version=%d err=%v", payload.ID, payload.Version, err)
	}
	if current.ParentCombinationID != "" {
		return taskstate.SaveCombinationCheckpoint(db, payload.ID, payload.Version, payload.ScanType, payload.ScanType+":completed", 100, nil)
	}

	now := time.Now()
	_ = db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ?", payload.ID, payload.Version).
		Updates(map[string]interface{}{
			"status":           "completed",
			"progress":         100,
			"completed_at":     &now,
			"lease_expires_at": nil,
			"updated_at":       now,
		}).Error
	if _, syncErr := assetstore.SyncCompletedTask(ctx, db, payload.ID, payload.Version); syncErr != nil {
		logger.Warning("[agent] 统一资产目录同步失败: task=%s version=%d err=%v", payload.ID, payload.Version, syncErr)
	}

	go func() {
		var vulnCount int64
		db.Model(&database.VulnerabilityInfo{}).
			Where("task_id = ? AND version = ?", payload.ID, payload.Version).
			Count(&vulnCount)
		notification.NotifyScanComplete(payload.ID, payload.TaskName, int(vulnCount), "completed")
	}()

	return nil
}

func RequeueRunningAgentTasks(reason string) (int64, error) {
	db, err := database.GetDB()
	if err != nil {
		return 0, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	if db.Dialector.Name() == "postgres" {
		return 0, nil
	} // Expired items are claimed with a new generation, never bulk-reset.

	updateFields := map[string]interface{}{
		"status":           "pending",
		"worker_agent_id":  "",
		"lease_expires_at": nil,
		"updated_at":       time.Now(),
		"started_at":       nil,
		"completed_at":     nil,
	}
	if strings.TrimSpace(reason) != "" {
		updateFields["error_msg"] = reason
	}

	var rowsAffected int64
	txErr := db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&database.VulnScanTaskItem{}).
			Where("status = ?", "running").
			Updates(map[string]interface{}{
				"status":           "pending",
				"worker_agent_id":  "",
				"lease_expires_at": nil,
				"updated_at":       time.Now(),
			})
		if result.Error != nil {
			return result.Error
		}
		rowsAffected += result.RowsAffected

		result = tx.Model(&database.VulnScanTask{}).
			Where("scan_mode = ? AND status = ?", "agent", "running").
			Updates(updateFields)
		if result.Error != nil {
			return result.Error
		}
		rowsAffected += result.RowsAffected
		return nil
	})
	if txErr != nil {
		return 0, txErr
	}
	return rowsAffected, nil
}

// AcknowledgeInterruptedLocalScanStops finalizes scan items that were already
// asked to stop when the embedded worker process was interrupted. Acquiring
// the host-scoped NodeSession is the proof that the previous local worker has
// exited, so its paused items can be acknowledged safely. Running items from
// ordinary scans are deliberately left alone and remain eligible for lease
// based recovery.
func AcknowledgeInterruptedLocalScanStops(agentID string) (int64, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return 0, fmt.Errorf("agentID 不能为空")
	}
	db, err := database.GetDB()
	if err != nil {
		return 0, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	if db.Dialector.Name() != "postgres" {
		return 0, nil
	}

	var itemIDs []uint
	if err := db.Table("vul_scan_task_item AS item").
		Joins("JOIN vul_scan_task AS task ON task.id = item.task_id AND task.version = item.version").
		Where("item.worker_agent_id = ? AND item.status = ? AND task.pause_requested = ? AND task.status IN ?", agentID, "running", true, []string{"running", "recovering", "pausing"}).
		Pluck("item.id", &itemIDs).Error; err != nil {
		return 0, err
	}
	if len(itemIDs) == 0 {
		return 0, nil
	}
	now := time.Now()
	result := db.Model(&database.VulnScanTaskItem{}).
		Where("id IN ? AND worker_agent_id = ? AND status = ?", itemIDs, agentID, "running").
		Updates(map[string]interface{}{
			"status":           "paused",
			"lease_expires_at": nil,
			"completed_at":     &now,
			"updated_at":       now,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

var errWorkerCapacityFull = errors.New("worker capacity full")

func usesWorkerScanQueue(db *gorm.DB, mode string) bool {
	return mode == "agent" || (db.Dialector.Name() == "postgres" && (mode == "auto" || mode == "local"))
}
func workerScanModes(db *gorm.DB, nodeID string) []string {
	if db.Dialector.Name() != "postgres" {
		return []string{"agent"}
	}
	modes := []string{"agent", "auto"}
	var local bool
	if db.Model(&database.ScanAgentNode{}).Select("local_worker").Where("agent_id=?", nodeID).Scan(&local).Error == nil && local {
		modes = append(modes, "local")
	}
	return modes
}

func cancelledItemStatus(db *gorm.DB, id string, version int) string {
	var pause bool
	_ = db.Model(&database.VulnScanTask{}).Select("pause_requested").Where("id=? AND version=?", id, version).Scan(&pause).Error
	if pause {
		return "paused"
	}
	return "cancelled"
}
