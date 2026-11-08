package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/aibatch"
	"hephaestus-backend/pkg/airun"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"
)

// Batch queues orchestrate input and acknowledgement only; the same leased AI
// Worker used by interactive dialogue owns the Agent loop and workspace.
func runDistributedAIBatchItem(ctx context.Context, db *gorm.DB, store *pentest.SQLStore, q aibatch.Queue, item aibatch.Item, save aibatch.Checkpoint) (result string, runErr error) {
	if item.ConversationID != "" {
		if _, err := store.GetConversation(ctx, item.ConversationID); errors.Is(err, sql.ErrNoRows) {
			item.ConversationID = ""
		} else if err != nil {
			return "", err
		}
	}
	if item.ConversationID == "" {
		project, err := store.GetProject(ctx, q.ProjectID)
		if err != nil {
			return "", err
		}
		conv, err := store.UpsertConversation(ctx, pentest.Conversation{ID: uuid.NewString(), ProjectID: project.ID, Mode: pentest.SoloPentestMode, Title: compactPentestProgressText(item.Instruction, 90), Target: firstTargetFromText(item.Instruction), Status: "completed"})
		if err != nil {
			return "", err
		}
		item.ConversationID = conv.ID
		if err = save(item); err != nil {
			return "", err
		}
	} else if err := resumeAIBatchModules(ctx, db, item); err != nil {
		return "", err
	}
	input := item.Instruction
	if messages, err := store.ListMessages(ctx, item.ConversationID, 1); err != nil {
		return "", err
	} else if len(messages) > 0 {
		input = "继续上次任务，保留已有证据和已完成步骤。原始任务：" + input
	}
	if _, err := store.EnqueueWorkerMessage(ctx, pentest.Message{ConversationID: item.ConversationID, Content: input}, q.ProviderID, q.Model, "", batchItemGeneration(item)); err != nil {
		return "", err
	}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stopCtx, done := context.WithTimeout(context.Background(), 12*time.Second)
			defer done()
			if err := store.RequestWorkerStop(stopCtx, item.ConversationID, false, "pause"); err != nil {
				return result, fmt.Errorf("%w: %v", aibatch.ErrUnacknowledged, err)
			}
			if err := airun.WaitWorkerStoppedWith(stopCtx, store, item.ConversationID, func(ctx context.Context, s *pentest.SQLStore, id string) error {
				return airun.FinalizeDistributedWorkerStopWithDB(ctx, s, db, id)
			}); err != nil {
				return result, fmt.Errorf("%w: %v", aibatch.ErrUnacknowledged, err)
			}
			return result, ctx.Err()
		case <-ticker.C:
			turn, err := store.LatestWorkerTurn(ctx, item.ConversationID)
			if err != nil {
				return "", err
			}
			switch turn.Status {
			case "completed":
				messages, err := store.ListMessages(ctx, item.ConversationID, -1)
				if err != nil {
					return "", err
				}
				result = latestAssistantText(messages)
				if err = waitAIBatchModules(ctx, db, item); err != nil {
					return result, err
				}
				return result, nil
			case "failed", "blocked", "cancelled":
				return result, fmt.Errorf("AI Worker %s: %s", turn.Status, turn.Error)
			}
		}
	}
}

func reconcileAIBatchPauses(ctx context.Context, store *pentest.SQLStore, db *gorm.DB) error {
	aiBatchRuntime.Lock()
	manager := aiBatchRuntime.manager
	aiBatchRuntime.Unlock()
	if manager == nil {
		return nil
	}
	return manager.ReconcilePaused(func(item aibatch.Item) error {
		if item.ConversationID != "" {
			turn, err := store.LatestWorkerTurn(ctx, item.ConversationID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err == nil && turn.Status != "cancelled" {
				if !turn.CancelRequested {
					if err = store.RequestWorkerStop(ctx, item.ConversationID, false, "pause"); err != nil {
						return err
					}
				}
				if err = airun.FinalizeDistributedWorkerStopWithDB(ctx, store, db, item.ConversationID); err != nil {
					return err
				}
			}
		}
		modules, err := batchModules(db, item)
		if err != nil {
			return err
		}
		for _, task := range modules {
			switch task.Status {
			case "paused", "completed", "failed", "cancelled":
				continue
			}
			if err = scantask.RequestPause(db, task.ID); err != nil {
				return err
			}
			var running int64
			if err = db.Model(&database.VulnScanTaskItem{}).Where("task_id=? AND version=? AND status='running'", task.ID, task.Version).Count(&running).Error; err != nil {
				return err
			}
			if running > 0 {
				return scantask.ErrExitPending
			}
		}
		return nil
	})
}
