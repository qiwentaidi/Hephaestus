package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// syncCancelledCombinationStatus repairs combinations whose latest parent scan
// task is already cancelled while the orchestration row is still non-terminal.
// PostgreSQL cancellation used to update only vul_scan_task, leaving the
// invisible combination_task row in a running state forever.
func syncCancelledCombinationStatus(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable("combination_task") || !db.Migrator().HasTable("vul_scan_task") {
		return nil
	}

	var ids []string
	if err := db.Raw(`
SELECT c.id
FROM combination_task c
JOIN vul_scan_task t ON t.parent_combination_id = c.id
JOIN (
	SELECT id, MAX(version) AS max_version
	FROM vul_scan_task
	GROUP BY id
) latest ON latest.id = t.id AND latest.max_version = t.version
WHERE t.scan_type = 'combination'
  AND t.status = 'cancelled'
  AND c.status NOT IN ('cancelled', 'completed', 'failed', 'paused')
`).Scan(&ids).Error; err != nil {
		return fmt.Errorf("查询状态不一致的组合任务失败: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	now := time.Now()
	if err := db.Model(&CombinationTask{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"status":          "cancelled",
			"pause_requested": false,
			"error_msg":       "任务已取消",
			"completed_at":    now,
			"updated_at":      now,
		}).Error; err != nil {
		return fmt.Errorf("同步已取消组合任务状态失败: %w", err)
	}
	return nil
}
