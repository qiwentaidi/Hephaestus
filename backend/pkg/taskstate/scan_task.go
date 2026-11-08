package taskstate

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"

	"gorm.io/gorm"
)

const (
	StatusPending    = "pending"
	StatusRunning    = "running"
	StatusPaused     = "paused"
	StatusRecovering = "recovering"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
	StatusDeleting   = "deleting"

	HeartbeatInterval = 5 * time.Second
	HeartbeatTimeout  = 20 * time.Second
)

var ErrTaskPaused = errors.New("task paused")

type Checkpoint struct {
	Stage     string                 `json:"stage"`
	Progress  int                    `json:"progress,omitempty"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
	UpdatedAt time.Time              `json:"updatedAt"`
}

func ParseCheckpoint(raw string) Checkpoint {
	if raw == "" {
		return Checkpoint{}
	}
	var checkpoint Checkpoint
	if err := json.Unmarshal([]byte(raw), &checkpoint); err != nil {
		return Checkpoint{}
	}
	return checkpoint
}

func MarshalCheckpoint(stage string, progress int, meta map[string]interface{}) string {
	payload, _ := json.Marshal(Checkpoint{
		Stage:     stage,
		Progress:  progress,
		Meta:      meta,
		UpdatedAt: time.Now(),
	})
	return string(payload)
}

func completionCheckpointMeta(db *gorm.DB, taskID string, version int) map[string]interface{} {
	var task struct {
		CheckpointJSON string `gorm:"column:checkpoint_json"`
	}
	if err := db.Model(&database.VulnScanTask{}).
		Select("checkpoint_json").
		Where("id = ? AND version = ?", taskID, version).
		First(&task).Error; err != nil {
		return nil
	}
	checkpoint := ParseCheckpoint(task.CheckpointJSON)
	if checkpoint.Meta == nil {
		return nil
	}
	if _, ok := checkpoint.Meta[moduleCheckpointsKey]; ok {
		return checkpoint.Meta
	}
	stageEvents, ok := checkpoint.Meta["stageEvents"]
	if !ok {
		return nil
	}
	return map[string]interface{}{"stageEvents": stageEvents}
}

func SaveCheckpoint(db *gorm.DB, taskID string, version int, stage string, progress int, meta map[string]interface{}) error {
	now := time.Now()
	return db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ?", taskID, version).
		Updates(map[string]interface{}{
			"current_stage":   stage,
			"checkpoint_json": MarshalCheckpoint(stage, progress, meta),
			"progress":        progress,
			"heartbeat_at":    now,
			"updated_at":      now,
		}).Error
}

func SaveCheckpointWithoutProgress(db *gorm.DB, taskID string, version int, stage string, progress int, meta map[string]interface{}) error {
	now := time.Now()
	return db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ?", taskID, version).
		Updates(map[string]interface{}{
			"current_stage":   stage,
			"checkpoint_json": MarshalCheckpoint(stage, progress, meta),
			"heartbeat_at":    now,
			"updated_at":      now,
		}).Error
}

func StartHeartbeat(ctx context.Context, db *gorm.DB, taskID string, version int) func() {
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(HeartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				now := time.Now()
				if err := db.Model(&database.VulnScanTask{}).
					Where("id = ? AND version = ? AND status IN ?", taskID, version, []string{StatusRunning, StatusRecovering}).
					Updates(map[string]interface{}{
						"heartbeat_at": now,
						"updated_at":   now,
					}).Error; err != nil {
					logger.Warning("更新任务心跳失败: task=%s version=%d err=%v", taskID, version, err)
				}
			}
		}
	}()

	return func() {
		close(stopCh)
		<-doneCh
	}
}

func MarkRunning(db *gorm.DB, taskID string, version int, stage string) error {
	now := time.Now()
	return db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ? AND status <> ?", taskID, version, StatusDeleting).
		Updates(map[string]interface{}{
			"status":          StatusRunning,
			"pause_requested": false,
			"current_stage":   stage,
			"heartbeat_at":    now,
			"started_at":      now,
			"updated_at":      now,
		}).Error
}

func MarkRecovering(db *gorm.DB, taskID string, version int, message string) error {
	now := time.Now()
	return db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ? AND status <> ?", taskID, version, StatusDeleting).
		Updates(map[string]interface{}{
			"status":       StatusRecovering,
			"error_msg":    message,
			"heartbeat_at": nil,
			"updated_at":   now,
		}).Error
}

func MarkPaused(db *gorm.DB, taskID string, version int, message string) error {
	now := time.Now()
	return db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ? AND status <> ?", taskID, version, StatusDeleting).
		Updates(map[string]interface{}{
			"status":          StatusPaused,
			"pause_requested": false,
			"error_msg":       message,
			"heartbeat_at":    now,
			"updated_at":      now,
		}).Error
}

func MarkCancelled(db *gorm.DB, taskID string, version int, message string) error {
	now := time.Now()
	return db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ? AND status <> ?", taskID, version, StatusDeleting).
		Updates(map[string]interface{}{
			"status":          StatusCancelled,
			"pause_requested": false,
			"error_msg":       message,
			"heartbeat_at":    nil,
			"updated_at":      now,
		}).Error
}

func MarkFailed(db *gorm.DB, taskID string, version int, err error) error {
	now := time.Now()
	message := ""
	if err != nil {
		message = err.Error()
	}
	return db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ? AND status <> ?", taskID, version, StatusDeleting).
		Updates(map[string]interface{}{
			"status":          StatusFailed,
			"pause_requested": false,
			"error_msg":       message,
			"heartbeat_at":    nil,
			"updated_at":      now,
		}).Error
}

func MarkCompleted(db *gorm.DB, taskID string, version int) error {
	now := time.Now()
	meta := completionCheckpointMeta(db, taskID, version)
	return db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ? AND status IN ?", taskID, version, []string{StatusRunning, StatusRecovering}).
		Updates(map[string]interface{}{
			"status":          StatusCompleted,
			"progress":        100,
			"pause_requested": false,
			"current_stage":   "completed",
			"checkpoint_json": MarshalCheckpoint("completed", 100, meta),
			"heartbeat_at":    nil,
			"completed_at":    &now,
			"updated_at":      now,
		}).Error
}

func MarkDeleting(db *gorm.DB, taskID string) error {
	now := time.Now()
	return db.Model(&database.VulnScanTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":           StatusDeleting,
			"pause_requested":  false,
			"lease_expires_at": nil,
			"heartbeat_at":     nil,
			"updated_at":       now,
		}).Error
}

func CheckpointAndPause(ctx context.Context, db *gorm.DB, taskID string, version int, stage string, progress int, meta map[string]interface{}) error {
	return checkpointAndPause(ctx, db, taskID, version, stage, progress, meta, true)
}

func CheckpointAndPauseWithoutProgress(ctx context.Context, db *gorm.DB, taskID string, version int, stage string, progress int, meta map[string]interface{}) error {
	return checkpointAndPause(ctx, db, taskID, version, stage, progress, meta, false)
}

// checkTaskControlState 读取任务控制状态。
// 返回 paused=true 表示任务请求暂停；任务已取消/删除时返回 context.Canceled。
func checkTaskControlState(db *gorm.DB, taskID string, version int) (bool, error) {
	var control struct {
		Status         string
		PauseRequested bool
	}
	if err := db.Model(&database.VulnScanTask{}).
		Select("status", "pause_requested").
		Where("id = ? AND version = ?", taskID, version).
		First(&control).Error; err != nil {
		return false, err
	}

	if control.Status == StatusCancelled || control.Status == StatusDeleting {
		return false, context.Canceled
	}

	return control.PauseRequested, nil
}

// CheckTaskControlState 供高频回调使用的轻量暂停/取消检查（只读、不写 checkpoint）。
// 高频回调可按时间节流写 checkpoint，但每次回调都应调用本函数以保证暂停/取消的响应性。
func CheckTaskControlState(ctx context.Context, db *gorm.DB, taskID string, version int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	paused, err := checkTaskControlState(db, taskID, version)
	if err != nil {
		return err
	}
	if paused {
		return ErrTaskPaused
	}
	return nil
}

func checkpointAndPause(ctx context.Context, db *gorm.DB, taskID string, version int, stage string, progress int, meta map[string]interface{}, updateProgress bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// FIX(BUG 9): 先读取任务控制状态再决定是否写 checkpoint。
	// 此前实现先写 checkpoint 后查状态，导致已取消(cancelled/deleting)的任务
	// 也会先被污染一次进度/阶段数据才发现应当退出。
	paused, err := checkTaskControlState(db, taskID, version)
	if err != nil {
		return err
	}

	saveCheckpointProgress := func() error {
		if updateProgress {
			return SaveCheckpoint(db, taskID, version, stage, progress, meta)
		}
		return SaveCheckpointWithoutProgress(db, taskID, version, stage, progress, meta)
	}

	if paused {
		// 暂停前先落 checkpoint 保存当前进度，再将任务置为 paused
		if err := saveCheckpointProgress(); err != nil {
			return err
		}
		if err := MarkPaused(db, taskID, version, "任务已暂停，等待继续"); err != nil {
			return err
		}
		return ErrTaskPaused
	}

	return saveCheckpointProgress()
}
