package agent

import (
	"context"
	"time"

	"hephaestus-backend/pkg/agentruntime"
	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

type AgentTaskPayload = agentruntime.TaskPayload
type AgentNodeUpsertOption = agentruntime.NodeUpsertOption
type AgentCommandPayload = agentruntime.CommandPayload

func UpsertAgentNode(agentID string, supportedTypes []string, opt AgentNodeUpsertOption) error {
	return agentruntime.UpsertAgentNode(agentID, supportedTypes, opt)
}

func SetAgentNodeOffline(agentID string) error {
	return agentruntime.SetAgentNodeOffline(agentID)
}

func HeartbeatAgent(agentID string, supportedTypes []string, leaseTTL time.Duration, opt AgentNodeUpsertOption) error {
	return agentruntime.HeartbeatAgent(agentID, supportedTypes, leaseTTL, opt)
}

func ClaimNextAgentTask(agentID string, supportedTypes []string, leaseTTL time.Duration) (*AgentTaskPayload, error) {
	return agentruntime.ClaimNextAgentTask(agentID, supportedTypes, leaseTTL)
}

func ClaimPendingAgentCommands(agentID string, limit int) ([]AgentCommandPayload, error) {
	return agentruntime.ClaimPendingAgentCommands(agentID, limit)
}

func FinishAgentCommand(commandID uint, status, resultMsg string) error {
	return agentruntime.FinishAgentCommand(commandID, status, resultMsg)
}

func ExecuteAgentTask(payload *AgentTaskPayload) error {
	return agentruntime.ExecuteAgentTask(payload)
}

func ExecuteAgentTaskWithContext(ctx context.Context, payload *AgentTaskPayload) error {
	return agentruntime.ExecuteAgentTaskWithContext(ctx, payload)
}

func RequeueRunningAgentTasks(reason string) (int64, error) {
	return agentruntime.RequeueRunningAgentTasks(reason)
}

func EnsureAgentTaskItems(db *gorm.DB, task *database.VulnScanTask) error {
	return agentruntime.EnsureAgentTaskItems(db, task)
}

func ResetAgentTaskItems(db *gorm.DB, task *database.VulnScanTask) error {
	return agentruntime.ResetAgentTaskItems(db, task)
}
