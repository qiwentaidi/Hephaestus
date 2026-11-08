package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"hephaestus-backend/lib/pentest"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/aibatch"
	"hephaestus-backend/pkg/airun"
	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/mcptools"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"
)

var aiBatchRuntime struct {
	sync.Mutex
	manager *aibatch.Manager
}

// Initialize before scan schedulers/recovery so a persisted queue pause cannot
// accidentally restart a native scan owned by that queue.
func InitAIBatchQueues() error {
	aiBatchRuntime.Lock()
	defer aiBatchRuntime.Unlock()
	if aiBatchRuntime.manager != nil {
		return nil
	}
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	manager, err := aibatch.New(db, runAIBatchItem)
	if err != nil {
		return err
	}
	if err = manager.Recover(func(item aibatch.Item) error {
		if db.Dialector.Name() == "postgres" && item.ConversationID != "" {
			store, err := aipkg.StoreFromAppDB(context.Background())
			if err != nil {
				return err
			}
			if err = store.RequestWorkerStop(context.Background(), item.ConversationID, false, "pause"); err != nil {
				return err
			}
		}

		if err := stopAIBatchModules(db, item, true); err != nil {
			return err
		}
		if item.ConversationID != "" {
			store, err := aipkg.StoreFromAppDB(context.Background())
			if err != nil {
				return err
			}
			if store.Distributed() {
				if err = airun.WaitWorkerStopped(context.Background(), store, item.ConversationID); err != nil {
					return err
				}
				return nil
			}
			if err = airun.MarkConversationStatus(context.Background(), store, item.ConversationID, "cancelled"); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	aiBatchRuntime.manager = manager
	return nil
}
func getAIBatchManager() (*aibatch.Manager, error) {
	if err := InitAIBatchQueues(); err != nil {
		return nil, err
	}
	aiBatchRuntime.Lock()
	defer aiBatchRuntime.Unlock()
	return aiBatchRuntime.manager, nil
}

func RegisterAIBatchRoutes(ai *gin.RouterGroup) {
	ai.Use(guardAIBatchSession)
	ai.GET("/batch-queues", listAIBatchQueues)
	ai.POST("/batch-queues", createAIBatchQueue)
	ai.POST("/batch-queues/actions", actionAIBatchQueues)
	ai.GET("/batch-queues/:id", getAIBatchQueue)
	ai.DELETE("/batch-queues/:id", deleteAIBatchQueue)
	ai.POST("/batch-queues/:id/items/:itemId/retry", retryAIBatchItem)
}
func batchError(c *gin.Context, err error) {
	code := http.StatusBadRequest
	if errors.Is(err, gorm.ErrRecordNotFound) {
		code = http.StatusNotFound
	}
	c.JSON(code, gin.H{"success": false, "error": err.Error()})
}
func listAIBatchQueues(c *gin.Context) {
	m, err := getAIBatchManager()
	if err != nil {
		batchError(c, err)
		return
	}
	qs, err := m.List()
	if err != nil {
		batchError(c, err)
		return
	}
	if qs == nil {
		qs = []aibatch.Queue{}
	}
	c.JSON(200, gin.H{"success": true, "data": qs})
}
func getAIBatchQueue(c *gin.Context) {
	m, err := getAIBatchManager()
	if err != nil {
		batchError(c, err)
		return
	}
	q, err := m.Get(c.Param("id"))
	if err != nil {
		batchError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": q})
}
func createAIBatchQueue(c *gin.Context) {
	var req struct {
		Name         string   `json:"name"`
		ProjectID    string   `json:"projectId"`
		Instructions []string `json:"instructions"`
		Concurrency  int      `json:"concurrency"`
		ProviderID   string   `json:"providerId"`
		Model        string   `json:"model"`
		ExecuteNow   bool     `json:"executeNow"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		batchError(c, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" || len(req.Instructions) == 0 || len(req.Instructions) > 1000 || req.Concurrency < 1 || req.Concurrency > 8 {
		batchError(c, fmt.Errorf("provide a name, 1–1000 instructions and concurrency 1–8"))
		return
	}
	// Validate all rows before allocating a project.
	for _, s := range req.Instructions {
		if strings.TrimSpace(s) == "" || len(s) > 32000 {
			batchError(c, fmt.Errorf("each instruction must contain 1–32000 bytes"))
			return
		}
	}
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		batchError(c, err)
		return
	}
	project := pentest.Project{}
	if req.ProjectID != "" {
		project, err = store.GetProject(c.Request.Context(), req.ProjectID)
	} else {
		project, err = store.UpsertProject(c.Request.Context(), pentest.Project{Name: strings.TrimSpace(req.Name)})
	}
	if err != nil {
		batchError(c, err)
		return
	}
	m, err := getAIBatchManager()
	if err != nil {
		batchError(c, err)
		return
	}
	q, err := m.Create(aibatch.Queue{Name: strings.TrimSpace(req.Name), ProjectID: project.ID, ProviderID: req.ProviderID, Model: req.Model, Concurrency: req.Concurrency}, req.Instructions)
	if err != nil {
		batchError(c, err)
		return
	}
	if req.ExecuteNow {
		if err = m.Start(q.ID); err != nil {
			batchError(c, err)
			return
		}
		q, err = m.Get(q.ID)
		if err != nil {
			batchError(c, err)
			return
		}
	}
	c.JSON(200, gin.H{"success": true, "data": q})
}
func actionAIBatchQueues(c *gin.Context) {
	var req struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		batchError(c, err)
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > 1000 || (req.Action != "pause" && req.Action != "resume" && req.Action != "retry_failed") {
		batchError(c, fmt.Errorf("select queues and a pause/resume/retry_failed action"))
		return
	}
	m, err := getAIBatchManager()
	if err != nil {
		batchError(c, err)
		return
	}
	result := map[string]string{}
	for _, id := range req.IDs {
		if _, handled := result[id]; handled {
			continue
		}
		var err error
		if req.Action == "pause" {
			err = m.Pause(id)
		} else if req.Action == "retry_failed" {
			_, err = m.RetryFailed(id)
		} else {
			err = m.Start(id)
		}
		if err != nil {
			result[id] = err.Error()
		} else {
			result[id] = ""
		}
	}
	c.JSON(200, gin.H{"success": true, "data": result})
}
func retryAIBatchItem(c *gin.Context) {
	m, err := getAIBatchManager()
	if err == nil {
		err = m.Retry(c.Param("id"), c.Param("itemId"))
	}
	if err != nil {
		batchError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": true})
}
func deleteAIBatchQueue(c *gin.Context) {
	m, err := getAIBatchManager()
	if err == nil {
		err = m.Delete(c.Param("id"))
	}
	if err != nil {
		batchError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": true})
}

// Keep manual messages from replacing a queue-owned active run. Reading the
// conversation is always allowed. Deleting a conversation cascades: its queue
// items are removed so no queue reference orphans. Project deletion must not
// orphan queues.
func guardAIBatchSession(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		c.Next()
		return
	}
	path := c.FullPath()
	id := c.Param("id")
	if id == "" || (!strings.Contains(path, "/sessions/") && !strings.Contains(path, "/projects/")) {
		c.Next()
		return
	}
	db, err := database.GetDB()
	if err != nil {
		batchError(c, err)
		c.Abort()
		return
	}
	if !db.Migrator().HasTable(&aibatch.Queue{}) {
		c.Next()
		return
	}
	if strings.Contains(path, "/sessions/") && c.Request.Method == http.MethodDelete {
		// Deleting a conversation removes its queue items first; queues left
		// empty are deleted by the manager.
		m, err := getAIBatchManager()
		if err == nil {
			err = m.RemoveConversation(id)
		}
		if err != nil {
			batchError(c, err)
			c.Abort()
			return
		}
		c.Next()
		return
	}
	// A batch runner marks its conversation terminal before the queue manager
	// finishes releasing the item (for example while native modules are being
	// acknowledged). That cleanup window must not prevent a user from sending a
	// follow-up message to an already completed conversation.
	if shouldBypassAIBatchQueueGuardForMessage(path, c.Request.Method, sessionStatusForBatchGuard(db, id)) {
		c.Next()
		return
	}
	var count int64
	if strings.Contains(path, "/projects/") && c.Request.Method == http.MethodDelete {
		err = db.Model(&aibatch.Queue{}).Where("project_id = ?", id).Count(&count).Error
	} else if strings.Contains(path, "/sessions/") {
		err = db.Model(&aibatch.Item{}).Where("conversation_id = ? AND status IN ?", id, []string{"running", "paused", "pausing", "pending"}).Count(&count).Error
	}
	if err != nil {
		batchError(c, err)
		c.Abort()
		return
	}
	if count > 0 {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"success": false, "error": "此会话或项目关联任务队列，请在任务队列中操作；删除前请先移除关联队列"})
		return
	}
	c.Next()
}

func sessionStatusForBatchGuard(db *gorm.DB, id string) string {
	if db == nil || strings.TrimSpace(id) == "" {
		return ""
	}
	var status string
	if err := db.Table("pentest_conversations").Select("status").Where("id = ?", id).Scan(&status).Error; err != nil {
		return ""
	}
	return status
}

func shouldBypassAIBatchQueueGuardForMessage(path, method, status string) bool {
	if method != http.MethodPost || !strings.Contains(path, "/sessions/") {
		return false
	}
	if !strings.HasSuffix(path, "/message") && !strings.HasSuffix(path, "/message/stream") {
		return false
	}
	return status != "" && !airun.ConversationStatusIsActive(status)
}

func runAIBatchItem(ctx context.Context, q aibatch.Queue, item aibatch.Item, save aibatch.Checkpoint) (result string, runErr error) {
	db, err := database.GetDB()
	if err != nil {
		return "", err
	}
	// Any detached native modules must acknowledge stop before this runner exits.
	defer func() {
		if err := stopAIBatchModules(db, item, false); err != nil {
			runErr = fmt.Errorf("%w: %v", aibatch.ErrUnacknowledged, err)
		}
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	store, err := aipkg.StoreFromAppDB(ctx)
	if err != nil {
		return "", err
	}
	if store.Distributed() {
		return runDistributedAIBatchItem(ctx, db, store, q, item, save)
	}
	var cfg pentest.AgentLoopConfig
	var history []pentest.AgentMessage
	if item.ConversationID != "" {
		_, err = store.GetConversation(ctx, item.ConversationID)
		if errors.Is(err, sql.ErrNoRows) {
			item.ConversationID = ""
			err = nil
		}
		if err != nil {
			return "", err
		}
	}
	if item.ConversationID != "" {
		if err = resumeAIBatchModules(ctx, db, item); err != nil {
			return "", err
		}
		_, cfg, history, err = preparePentestSessionMessage(ctx, item.ConversationID, "继续上次任务，保留已有证据和已完成步骤。原始任务："+item.Instruction, q.ProviderID, q.Model, "")
	} else {
		conversationID := uuid.NewString()
		cfg, err = buildPentestAgentConfig(ctx, store, q.ProviderID, q.Model, firstTargetFromText(item.Instruction), conversationID)
		if err == nil {
			cfg.Project, err = store.GetProject(ctx, q.ProjectID)
		}
		if err == nil {
			item.ConversationID = conversationID
			if err = save(item); err != nil {
				return "", err
			}
			cfg.Conversation.Title = compactPentestProgressText(item.Instruction, 90)
			cfg.Project, cfg.Conversation, err = createPentestConversationWithUserMessage(ctx, store, cfg.Project, cfg.Conversation, item.Instruction)
		}
		history = []pentest.AgentMessage{{Role: "user", Content: item.Instruction}}
	}
	if err != nil {
		return "", err
	}
	// The queue owns module execution. Remote agent scans cannot yet acknowledge
	// pause, so expose the existing local path and reject explicit remote requests.
	cfg.NativeToolRunner = func(toolCtx context.Context, c pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error) {
		if err := toolCtx.Err(); err != nil {
			return nil, true, err
		}
		name := call.Function.Name
		creates := name == "subdomain_create_task" || name == "attack_surface_create_webscan_task" || name == "attack_surface_create_jsscan_task" || name == "attack_surface_create_dirscan_task" || name == "attack_surface_create_crack_task"
		if creates {
			if call.Function.Arguments == nil {
				call.Function.Arguments = map[string]any{}
			}
			if mode, _ := call.Function.Arguments["scanMode"].(string); mode == "agent" {
				return nil, true, fmt.Errorf("batch queues require local scans with pause acknowledgement")
			}
			call.Function.Arguments["scanMode"] = "local"
			conf, _ := call.Function.Arguments["config"].(map[string]any)
			if conf == nil {
				conf = map[string]any{}
			}
			conf["aiBatchItemId"] = batchItemGeneration(item)
			call.Function.Arguments["config"] = conf
		}
		data, handled, err := mcptools.RunNative(toolCtx, c, call)
		if creates && err == nil {
			if payload, ok := data.(gin.H); ok {
				if id, ok := payload["taskId"].(string); ok {
					item.Modules = append(item.Modules, id)
					if err = save(item); err != nil {
						return nil, true, err
					}
				}
			}
		}
		return data, handled, err
	}
	snapshot, err := runPentestAgentLoopAndSnapshot(ctx, cfg, history, q.ProviderID, q.Model)
	if snapshot != nil {
		result = latestAssistantTextFromBatch(snapshot)
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	// Do not count an independently cancelled session as successful.
	if snapshot != nil {
		if task, ok := snapshot["task"].(gin.H); ok && task["status"] == "cancelled" {
			return result, fmt.Errorf("session was cancelled")
		}
	}
	if err == nil {
		err = waitAIBatchModules(ctx, db, item)
	}
	return result, err
}
func latestAssistantTextFromBatch(snapshot gin.H) string { return latestAssistantContent(snapshot) }

func batchModules(db *gorm.DB, item aibatch.Item) ([]database.VulnScanTask, error) {
	// The marker is persisted as part of native task creation, closing the crash
	// window between creation and checkpointing its ID into the queue item.
	var tasks []database.VulnScanTask
	query := db.Where("config LIKE ?", "%"+batchItemGeneration(item)+"%")
	if len(item.Modules) > 0 {
		query = query.Or("id IN ?", item.Modules)
	}
	if err := query.Order("version DESC").Find(&tasks).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []database.VulnScanTask{}
	for _, t := range tasks {
		if seen[t.ID] {
			continue
		}
		var conf map[string]any
		_ = json.Unmarshal([]byte(t.Config), &conf)
		match := conf["aiBatchItemId"] == batchItemGeneration(item)
		for _, id := range item.Modules {
			match = match || id == t.ID
		}
		if match {
			seen[t.ID] = true
			out = append(out, t)
		}
	}
	return out, nil
}
func stopAIBatchModules(db *gorm.DB, item aibatch.Item, restart bool) error {
	modules, err := batchModules(db, item)
	if err != nil {
		return err
	}
	for _, task := range modules {
		switch task.Status {
		case "completed", "failed", "cancelled", "paused":
			continue
		}
		if scantask.UsePersistentWorkers(db, task.ScanMode) && db.Dialector.Name() == "postgres" {
			if err = scantask.RequestPause(db, task.ID); err != nil {
				return err
			}
			continue
		}
		if task.ScanMode == "agent" {
			return fmt.Errorf("remote scan %s has no pause acknowledgement", task.ID)
		}
		if err = db.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", task.ID, task.Version).Update("pause_requested", true).Error; err != nil {
			return err
		}
		_ = control.GetTaskController().CancelTask(task.ID)
		if restart {
			if err = taskstate.MarkPaused(db, task.ID, task.Version, "AI 批量任务已暂停"); err != nil {
				return err
			}
		} else if task.Status == "pending" {
			// A pending item can be finalized only if it has not been claimed meanwhile.
			if err = db.Model(&database.VulnScanTask{}).Where("id = ? AND version = ? AND status = ?", task.ID, task.Version, "pending").Updates(map[string]any{"status": "paused", "pause_requested": true, "updated_at": time.Now()}).Error; err != nil {
				return err
			}
		}
	}
	if restart && db.Dialector.Name() != "postgres" {
		return nil
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		modules, err = batchModules(db, item)
		if err != nil {
			return err
		}
		active := false
		for _, task := range modules {
			switch task.Status {
			case "completed", "failed", "cancelled", "paused":
			default:
				active = true
				_ = control.GetTaskController().CancelTask(task.ID)
			}
		}
		if !active {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("等待扫描模块退出超时，请检查子任务；队列保持暂停中")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
func resumeAIBatchModules(ctx context.Context, db *gorm.DB, item aibatch.Item) error {
	modules, err := batchModules(db, item)
	if err != nil {
		return err
	}
	for _, task := range modules {
		if err = ctx.Err(); err != nil {
			return err
		}
		if task.Status == "paused" {
			if err = scantask.ResetForResume(db, &task); err != nil {
				return err
			}
			if err = scantask.EnqueueLocalTask(task.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// A successful assistant turn does not imply its detached scans have finished.
func waitAIBatchModules(ctx context.Context, db *gorm.DB, item aibatch.Item) error {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		modules, err := batchModules(db, item)
		if err != nil {
			return err
		}
		active := false
		for _, t := range modules {
			switch t.Status {
			case "completed":
			case "failed", "cancelled", "paused":
				return fmt.Errorf("scan %s ended in %s: %s", t.ID, t.Status, t.ErrorMsg)
			default:
				active = true
			}
		}
		if !active {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func StopAIBatchQueues(ctx context.Context) error {
	aiBatchRuntime.Lock()
	m := aiBatchRuntime.manager
	aiBatchRuntime.Unlock()
	if m == nil {
		return nil
	}
	return m.Shutdown(ctx)
}

func batchItemGeneration(item aibatch.Item) string {
	return fmt.Sprintf("%s/%d", item.ID, item.Generation)
}
