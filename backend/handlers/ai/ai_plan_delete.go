package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/lib/pentest"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
)

var errPlanRunNotDeletable = errors.New("智能编排任务仍在规划、等待或暂停中，请先暂停或取消后再删除")

// DeleteAIPlanRunHandler removes one orchestration task, its projected plan,
// associated conversations and workspace files. The project itself and other
// runs are kept.
func DeleteAIPlanRunHandler(c *gin.Context) {
	ctx := c.Request.Context()
	store, err := aipkg.StoreFromAppDB(ctx)
	if err != nil {
		nativeError(c, http.StatusInternalServerError, err)
		return
	}
	run, err := store.GetPlanRun(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "not found") {
			nativeError(c, http.StatusNotFound, err)
			return
		}
		nativeError(c, http.StatusInternalServerError, err)
		return
	}
	if run.ReadOnly || !pentest.IsOrchestrationRun(run.ID) {
		nativeError(c, http.StatusConflict, pentest.ErrLegacyOrchestrationReadOnly)
		return
	}

	conversationIDs, err := deletePlanRunData(
		ctx, store,
		filepath.Join(config.GetDataDir(), "ai-pentest", "sessions"),
		run, deleteARTEXProjectTasks,
	)
	if err != nil {
		status := http.StatusConflict
		if !errors.Is(err, errPlanRunNotDeletable) {
			status = http.StatusInternalServerError
		}
		nativeError(c, status, err)
		return
	}

	if db, dbErr := database.GetDB(); dbErr == nil && len(conversationIDs) > 0 {
		if err := db.WithContext(ctx).Where("conversation_id IN ?", conversationIDs).Delete(&database.AIWorkbenchSideChat{}).Error; err != nil {
			log.Printf("[plan-delete] delete side chats %s: %v", run.ID, err)
		}
		if err := db.WithContext(ctx).Where("conversation_id IN ?", conversationIDs).Delete(&database.AIAssetReference{}).Error; err != nil {
			log.Printf("[plan-delete] delete asset references %s: %v", run.ID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"id":            run.ID,
		"conversations": len(conversationIDs),
	}})
}

func deletePlanRunData(
	ctx context.Context,
	store *pentest.SQLStore,
	sessionsDir string,
	run pentest.PlanRun,
	deleteTasks func(context.Context, []string) error,
) ([]string, error) {
	if run.ReadOnly || !pentest.IsOrchestrationRun(run.ID) {
		return nil, pentest.ErrLegacyOrchestrationReadOnly
	}
	if planRunDeleteBlocked(run.Status) {
		return nil, errPlanRunNotDeletable
	}
	conversationIDs, err := planRunConversationIDs(ctx, store, run)
	if err != nil {
		return nil, err
	}
	if deleteTasks == nil {
		return nil, fmt.Errorf("删除任务函数不能为空")
	}
	if err := deleteTasks(ctx, []string{run.ID}); err != nil {
		return conversationIDs, err
	}
	for _, conversationID := range conversationIDs {
		if _, err := store.GetConversation(ctx, conversationID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return conversationIDs, err
		}
		if err := deletePentestSession(ctx, store, sessionsDir, conversationID, true); err != nil {
			return conversationIDs, err
		}
	}
	if err := store.DeletePlanRun(ctx, run.ID); err != nil {
		return conversationIDs, err
	}
	return conversationIDs, nil
}

func planRunDeleteBlocked(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "planning", "waiting", "pausing", "running", "pending":
		return true
	default:
		return false
	}
}

func planRunConversationIDs(ctx context.Context, store *pentest.SQLStore, run pentest.PlanRun) ([]string, error) {
	seen := make(map[string]struct{})
	result := make([]string, 0, 4)
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || !safeConversationID(id) {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}

	add(run.PlannerConversationID)
	add(run.ControllerConversationID)
	roles, err := store.ListAgentRolesByPlanRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	for _, role := range roles {
		add(role.ConversationID)
	}
	items, err := store.ListPlanItems(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		add(item.ChildConversationID)
	}
	return result, nil
}
