package asset

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"

	"gorm.io/gorm"
)

const (
	lifecycleAdvisoryLockKey int64 = 0x41535345544c4346 // "ASSETLCF"
	lifecycleRunTimeout            = 10 * time.Minute
	lifecycleScheduleHour          = 3
)

var (
	lifecycleProcessMu     sync.Mutex
	lifecycleSchedulerOnce sync.Once
)

// RunLifecycleReconcile applies the lifecycle rules under a single-instance
// lock. It is safe to call from both the scheduler and the manual endpoint.
func RunLifecycleReconcile(ctx context.Context, db *gorm.DB, staleAfter, retireAfter time.Duration) (int, error) {
	changed, _, err := RunLifecycleReconcileExclusive(ctx, db, staleAfter, retireAfter)
	return changed, err
}

// RunLifecycleReconcileExclusive also reports whether this process actually
// acquired the run lock. A false "ran" means another instance is already
// reconciling the catalog and this call was skipped.
func RunLifecycleReconcileExclusive(ctx context.Context, db *gorm.DB, staleAfter, retireAfter time.Duration) (int, bool, error) {
	if db == nil {
		return 0, false, errors.New("asset store database is nil")
	}
	if db.Dialector != nil && db.Dialector.Name() == "postgres" {
		changed := 0
		ran := false
		err := db.Connection(func(conn *gorm.DB) error {
			var locked bool
			lockRow := conn.Raw("SELECT pg_try_advisory_lock(?)", lifecycleAdvisoryLockKey).Row()
			if lockRow == nil {
				return errors.New("获取资产活跃度收敛锁失败: empty row")
			}
			if err := lockRow.Scan(&locked); err != nil {
				return fmt.Errorf("获取资产活跃度收敛锁失败: %w", err)
			}
			if !locked {
				return nil
			}
			ran = true
			defer func() {
				var unlocked bool
				unlockRow := conn.Session(&gorm.Session{}).Raw("SELECT pg_advisory_unlock(?)", lifecycleAdvisoryLockKey).Row()
				if unlockRow == nil {
					logger.Warning("释放资产活跃度收敛锁失败: empty row")
					return
				}
				if err := unlockRow.Scan(&unlocked); err != nil {
					logger.Warning("释放资产活跃度收敛锁失败: %v", err)
				}
			}()
			var err error
			changed, err = (&Store{DB: conn.Session(&gorm.Session{})}).ReconcileLifecycle(ctx, time.Now().UTC(), staleAfter, retireAfter)
			return err
		})
		return changed, ran, err
	}
	if !lifecycleProcessMu.TryLock() {
		return 0, false, nil
	}
	defer lifecycleProcessMu.Unlock()
	changed, err := (&Store{DB: db}).ReconcileLifecycle(ctx, time.Now().UTC(), staleAfter, retireAfter)
	return changed, true, err
}

// StartLifecycleScheduler starts the daily 03:00 lifecycle reconciliation. It
// also catches up on startup when the last successful run is older than 24h.
// Only the primary application should call this.
func StartLifecycleScheduler() {
	lifecycleSchedulerOnce.Do(func() {
		go runLifecycleScheduler()
	})
}

func runLifecycleScheduler() {
	db, err := database.GetDB()
	if err != nil {
		logger.Warning("启动资产活跃度调度失败: %v", err)
		return
	}
	if lastRun, err := loadLifecycleLastRun(); err != nil {
		logger.Warning("读取资产活跃度上次执行时间失败: %v", err)
	} else if lastRun.IsZero() || time.Since(lastRun) >= 24*time.Hour {
		runLifecycleOnce(db, "startup-catchup")
	}

	for {
		next := nextLifecycleRun(time.Now())
		timer := time.NewTimer(time.Until(next))
		<-timer.C
		timer.Stop()
		db, err := database.GetDB()
		if err != nil {
			logger.Warning("资产活跃度调度获取数据库失败: %v", err)
			continue
		}
		runLifecycleOnce(db, "daily")
	}
}

func runLifecycleOnce(db *gorm.DB, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), lifecycleRunTimeout)
	defer cancel()
	started := time.Now()
	changed, ran, err := RunLifecycleReconcileExclusive(ctx, db, 0, 0)
	if err != nil {
		logger.Warning("资产活跃度收敛失败(%s): %v", reason, err)
		return
	}
	if !ran {
		logger.Info("资产活跃度收敛已由其他实例执行(%s)", reason)
		return
	}
	if err := database.SetSetting(database.SettingKeyAssetLifecycleLastRun, time.Now().UTC().Format(time.RFC3339)); err != nil {
		logger.Warning("记录资产活跃度执行时间失败: %v", err)
	}
	logger.Info("资产活跃度收敛完成(%s): changed=%d elapsed=%s", reason, changed, time.Since(started))
}

func loadLifecycleLastRun() (time.Time, error) {
	value, err := database.GetSetting(database.SettingKeyAssetLifecycleLastRun)
	if err != nil {
		return time.Time{}, err
	}
	text, ok := value.(string)
	if !ok || text == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析资产活跃度执行时间失败: %w", err)
	}
	return parsed, nil
}

func nextLifecycleRun(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), lifecycleScheduleHour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
