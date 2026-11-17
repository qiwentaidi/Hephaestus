package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// syncCancelledCombinationStatus is the 0015 repair for the historical
// PostgreSQL cancellation bug. It covers only the case where the latest parent
// scan task is cancelled and the combination row is still non-terminal (paused
// is treated as terminal here). Orphan rows and other terminal parent states
// are handled by reconcileCombinationStatus (0016).
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

// reconcileCombinationStatus is the 0016 repair for the remaining invisible
// combination states:
//
//   - an active combination row with no combination parent task at all;
//   - an active combination row whose latest parent task is already terminal
//     (cancelled, failed, completed, paused or deleting).
//
// A combination row whose parent is still running/recovering/pausing/cancelling
// is intentionally left alone. Terminal combination rows are also left alone
// so the migration never rewrites finished history.
func reconcileCombinationStatus(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable("combination_task") || !db.Migrator().HasTable("vul_scan_task") {
		return nil
	}
	now := time.Now()

	var orphanIDs []string
	if err := db.Raw(`
SELECT c.id
FROM combination_task c
WHERE c.status NOT IN ('completed', 'failed', 'cancelled', 'paused')
  AND NOT EXISTS (
	SELECT 1 FROM vul_scan_task t
	WHERE t.parent_combination_id = c.id
	  AND t.scan_type = 'combination'
  )
`).Scan(&orphanIDs).Error; err != nil {
		return fmt.Errorf("查询孤儿组合任务失败: %w", err)
	}
	if err := updateCombinationStates(db, orphanIDs, "cancelled", "组合任务缺少父任务，已自动终止", now, true); err != nil {
		return fmt.Errorf("收敛孤儿组合任务失败: %w", err)
	}

	type parentStateRow struct {
		ID           string
		ParentStatus string
	}
	var rows []parentStateRow
	if err := db.Raw(`
SELECT c.id, t.status AS parent_status
FROM combination_task c
JOIN vul_scan_task t
  ON t.parent_combination_id = c.id
 AND t.scan_type = 'combination'
 AND t.version = (
	SELECT MAX(p.version)
	FROM vul_scan_task p
	WHERE p.parent_combination_id = c.id
	  AND p.scan_type = 'combination'
 )
WHERE t.status IN ('cancelled', 'failed', 'completed', 'paused', 'deleting')
  AND c.status NOT IN ('completed', 'failed', 'cancelled', 'paused')
`).Scan(&rows).Error; err != nil {
		return fmt.Errorf("查询组合任务父任务状态失败: %w", err)
	}

	grouped := make(map[string][]string)
	for _, row := range rows {
		grouped[row.ParentStatus] = append(grouped[row.ParentStatus], row.ID)
	}
	for parentStatus, ids := range grouped {
		status, message, completed := combinationTargetState(parentStatus)
		if status == "" {
			continue
		}
		if err := updateCombinationStates(db, ids, status, message, now, completed); err != nil {
			return fmt.Errorf("同步组合任务状态 %s 失败: %w", parentStatus, err)
		}
	}
	return nil
}

func combinationTargetState(parentStatus string) (status, message string, completed bool) {
	switch parentStatus {
	case "cancelled":
		return "cancelled", "任务已取消", true
	case "deleting":
		return "cancelled", "父任务正在删除，组合任务已终止", true
	case "failed":
		return "failed", "父任务已失败，组合任务已同步失败", true
	case "completed":
		return "completed", "", true
	case "paused":
		return "paused", "父任务已暂停，组合任务已同步暂停", false
	default:
		return "", "", false
	}
}

func updateCombinationStates(db *gorm.DB, ids []string, status, message string, now time.Time, completed bool) error {
	if len(ids) == 0 {
		return nil
	}
	updates := map[string]any{
		"status":          status,
		"pause_requested": false,
		"error_msg":       message,
		"updated_at":      now,
	}
	if completed {
		updates["completed_at"] = now
	} else {
		updates["completed_at"] = nil
	}
	return db.Model(&CombinationTask{}).Where("id IN ?", ids).Updates(updates).Error
}
