package airun

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"
)

func FinalizeDistributedWorkerStop(ctx context.Context, store *pentest.SQLStore, id string) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	return FinalizeDistributedWorkerStopWithDB(ctx, store, db, id)
}

func FinalizeDistributedWorkerStopWithDB(ctx context.Context, store *pentest.SQLStore, db *gorm.DB, id string) error {
	t, err := store.LatestWorkerTurn(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		// A reconcile pass may have captured the stop row just before the
		// session deletion transaction cascaded it away. There is nothing left
		// to stop or acknowledge in that case.
		if _, conversationErr := store.GetConversation(ctx, id); errors.Is(conversationErr, sql.ErrNoRows) {
			return nil
		}
	}
	if err != nil {
		return err
	}
	if t.Status != "stopped" && !(t.Status == "cancelling" && t.NodeID == "") {
		return fmt.Errorf("worker exit acknowledgement pending")
	}
	execs, err := store.ListToolExecutionsByConversation(ctx, id, 0)
	if err != nil {
		return err
	}
	ids := OwnedScanTaskIDs(execs)
	// A scan can have been created before an HTTP response/checkpoint was lost.
	// Its server-written ownership marker remains authoritative for cancellation.
	var owned []string
	if db.Dialector.Name() == "postgres" {
		if err = db.Model(&database.VulnScanTask{}).Select("id").Where("config::jsonb->>'aiWorkerConversationId' = ?", id).Distinct().Pluck("id", &owned).Error; err != nil {
			return err
		}
	} else {
		// SQLite builds do not require the JSON1 extension. Cancellation is rare,
		// so filtering the ownership marker in Go is safer than a dialect-specific
		// json_extract query.
		var tasks []database.VulnScanTask
		if err = db.Model(&database.VulnScanTask{}).Select("id", "config").Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			var config map[string]any
			if json.Unmarshal([]byte(task.Config), &config) != nil {
				continue
			}
			if strings.TrimSpace(fmt.Sprint(config["aiWorkerConversationId"])) == id {
				owned = append(owned, task.ID)
			}
		}
	}
	seen := map[string]bool{}
	for _, taskID := range append(ids, owned...) {
		seen[taskID] = true
	}
	ids = ids[:0]
	for taskID := range seen {
		ids = append(ids, taskID)
	}
	for _, taskID := range ids {
		if t.StopReason == "pause" {
			err = scantask.RequestPause(db, taskID)
		} else {
			err = scantask.CancelTaskWithDB(db, taskID)
		}
		if err != nil && !errors.Is(err, scantask.ErrNotCancelable) {
			return err
		}
	}

	for _, taskID := range ids {
		var running int64
		if err = db.Model(&database.VulnScanTaskItem{}).Where("task_id=? AND status='running'", taskID).Count(&running).Error; err != nil {
			return err
		}
		if running != 0 {
			return fmt.Errorf("scan %s exit acknowledgement pending", taskID)
		}
	}
	return store.FinalizeWorkerCancellation(ctx, id)
}

func WaitWorkerStopped(ctx context.Context, store *pentest.SQLStore, id string) error {
	return WaitWorkerStoppedWith(ctx, store, id, FinalizeDistributedWorkerStop)
}

func WaitWorkerStoppedWith(ctx context.Context, store *pentest.SQLStore, id string, finalize func(context.Context, *pentest.SQLStore, string) error) error {
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		t, err := store.LatestWorkerTurn(ctx, id)
		if errors.Is(err, context.Canceled) {
			return err
		}
		if errors.Is(err, sql.ErrNoRows) {
			if _, conversationErr := store.GetConversation(ctx, id); errors.Is(conversationErr, sql.ErrNoRows) {
				return nil
			}
		}
		if err == nil && t.Status != "running" && t.Status != "queued" && t.Status != "cancelling" {
			if t.Status == "stopped" {
				if err = finalize(ctx, store, id); err != nil {
					goto wait
				}
			} else {
				return nil
			}
			return nil
		}
		if err == nil && t.Status == "cancelling" && t.NodeID == "" {
			if err = finalize(ctx, store, id); err == nil {
				return nil
			}
		}
		if err != nil && t.ID == "" {
			return err
		}
	wait:
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("执行尚未确认停止，请稍后重试")
		case <-ticker.C:
		}
	}
}
