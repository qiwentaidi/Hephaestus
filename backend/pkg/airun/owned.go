package airun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/database"
)

// ownedScanCreateTools 是 AI 工具中会创建扫描任务的那一批；只有它们的结果才会被
// 视为“该会话拥有的扫描”。
var ownedScanCreateTools = map[string]struct{}{
	"subdomain_create_task":               {},
	"attack_surface_create_webscan_task":  {},
	"attack_surface_create_portscan_task": {},
	"attack_surface_create_jsscan_task":   {},
	"attack_surface_create_dirscan_task":  {},
	"attack_surface_create_crack_task":    {},
}

// OwnedScanToolNames 返回会创建扫描任务的工具名（供兼容旧任务查询使用）。
func OwnedScanToolNames() []string {
	tools := make([]string, 0, len(ownedScanCreateTools))
	for name := range ownedScanCreateTools {
		tools = append(tools, name)
	}
	return tools
}

// ConversationIDFromTask 读取任务配置里的 AI 会话归属标记。
func ConversationIDFromTask(task database.VulnScanTask) string {
	var config map[string]any
	if json.Unmarshal([]byte(task.Config), &config) != nil {
		return ""
	}
	id, _ := config["aiWorkerConversationId"].(string)
	return strings.TrimSpace(id)
}

// StopConversationForOwnedScan 在暂停/删除扫描前先停掉其 AI 生产者。
// 分布式 worker 通过持久化协议确认退出，进程内 worker 使用 run 取消并等待 done。
func StopConversationForOwnedScan(db *gorm.DB, task database.VulnScanTask) error {
	stopCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	store, err := ai.StoreFromAppDB(stopCtx)
	if err != nil {
		return err
	}

	conversationID := ConversationIDFromTask(task)
	if conversationID == "" && task.CreatedByAI && db != nil {
		// 旧任务没有归属标记，从持久化的工具执行结果里反查生产者。
		var executions []struct {
			ConversationID string
			ResultJSON     string
		}
		query := db.WithContext(stopCtx).Table("pentest_tool_executions").
			Select("conversation_id, result_json").
			Where("tool_name IN ? AND result_json LIKE ?", ownedScanToolNames(), "%"+task.ID+"%").
			Order("created_at DESC").Limit(50)
		if query.Find(&executions).Error == nil {
			for _, execution := range executions {
				result, ok := executionResult(execution.ResultJSON)
				if ok && materialString(result, "taskId", "task_id") == task.ID {
					conversationID = strings.TrimSpace(execution.ConversationID)
					break
				}
			}
		}
	}
	if conversationID == "" {
		return nil
	}

	conversation, err := store.GetConversation(stopCtx, conversationID)
	if err != nil {
		return err
	}
	if !ConversationStatusIsActive(conversation.Status) {
		return nil
	}

	if store.Distributed() || store.LocalWorkerEnabled() {
		if err := store.RequestWorkerStop(stopCtx, conversationID, false, "pause"); err != nil {
			return err
		}
		return WaitWorkerStopped(stopCtx, store, conversationID)
	}

	if run, ok := LoadActiveRun(conversationID); ok {
		run.Cancel()
		select {
		case <-run.Done:
		case <-stopCtx.Done():
			return fmt.Errorf("AI 会话 %s 尚未确认停止", conversationID)
		}
	}
	return MarkConversationStatus(stopCtx, store, conversationID, "cancelled")
}

func ownedScanToolNames() []string { return OwnedScanToolNames() }

func executionResult(raw string) (map[string]any, bool) {
	var observation pentest.AgentToolObservation
	if err := json.Unmarshal([]byte(raw), &observation); err == nil && (observation.ToolName != "" || observation.ExecutionID != "") {
		if !observation.OK || observation.Result == nil {
			return nil, false
		}
		return materialMap(observation.Result), true
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, false
	}
	return result, true
}

func materialMap(value any) map[string]any {
	if out, ok := value.(map[string]any); ok {
		return out
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func materialString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := row[key]; ok && value != nil {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func OwnedScanTaskIDs(executions []pentest.ToolExecution) []string {
	seen := map[string]struct{}{}
	for _, execution := range executions {
		if _, ok := ownedScanCreateTools[strings.TrimSpace(execution.ToolName)]; !ok {
			continue
		}
		if execution.Status == pentest.ToolExecutionStatusFailed || execution.Status == pentest.ToolExecutionStatusCancelled || execution.Status == pentest.ToolExecutionStatusBlocked {
			continue
		}
		result, ok := executionResult(execution.ResultJSON)
		if !ok {
			continue
		}
		if reused, ok := result["reused"].(bool); ok && reused {
			continue
		}
		taskID := strings.TrimSpace(materialString(result, "taskId", "task_id"))
		if taskID == "" {
			continue
		}
		seen[taskID] = struct{}{}
	}
	taskIDs := make([]string, 0, len(seen))
	for id := range seen {
		taskIDs = append(taskIDs, id)
	}
	return taskIDs
}

func CompactFindingTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, scheme := range []string{"https://", "http://"} {
		if start := strings.Index(raw, scheme); start >= 0 {
			target := raw[start:]
			if end := strings.IndexAny(target, " \t\r\n，。；;、（()）[]【】{}<>"); end >= 0 {
				target = target[:end]
			}
			return strings.Trim(target, " \t\r\n，。；;,")
		}
	}

	target := raw
	if end := strings.IndexAny(target, "\r\n（("); end >= 0 {
		target = target[:end]
	}
	return strings.Trim(target, " \t\r\n，。；;,")
}
