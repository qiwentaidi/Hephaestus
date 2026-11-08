package airun

import (
	"context"
	"strings"

	"hephaestus-backend/lib/pentest"
)

// NormalizeConversationStatus 把持久化的会话状态收敛到已知枚举；未知状态按 running
// 处理，避免把未确认的结果误判为终态。
func NormalizeConversationStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "cancelling", "recovering", "pending", "running", "draining", "waiting", "waiting_tool", "waiting_approval", "completed", "failed", "cancelled", "blocked":
		return strings.ToLower(strings.TrimSpace(status))
	case "active":
		return "running"
	default:
		return "running"
	}
}

// ConversationStatusIsActive 判断会话是否仍可能产生工作（含取消中/恢复中）。
func ConversationStatusIsActive(status string) bool {
	switch NormalizeConversationStatus(status) {
	case "queued", "cancelling", "recovering", "pending", "running", "draining", "waiting_tool", "waiting_approval":
		return true
	default:
		return false
	}
}

// MarkConversationStatus 更新会话状态并回写存储。
func MarkConversationStatus(ctx context.Context, store *pentest.SQLStore, conversationID, status string) error {
	conv, err := store.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	conv.Status = NormalizeConversationStatus(status)
	_, err = store.UpsertConversation(ctx, conv)
	return err
}
