package ai

import (
	"context"
	"path/filepath"
	"strconv"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/capabilities"
	"hephaestus-backend/pkg/config"
)

func init() {
	taskengine.HostCapabilityContext = artexCapabilityHost
}

// artexCapabilityHost maps a native task run to the host context used by
// platform scan/discovery tools and external MCP tools.
func artexCapabilityHost(ctx context.Context, taskID, intentID int64, agentKey string) (capabilities.Host, bool) {
	store, err := aipkg.StoreFromAppDB(ctx)
	if err != nil {
		return capabilities.Host{}, false
	}
	run, err := store.GetPlanRun(ctx, pentest.OrchestrationRunPrefix+strconv.FormatInt(taskID, 10))
	if err != nil {
		return capabilities.Host{}, false
	}
	workDir := filepath.Join(config.GetDataDir(), "ai-pentest", "tasks", strconv.FormatInt(taskID, 10))
	conversationID := run.PlannerConversationID
	if capabilities.NormalizeRole(agentKey) == "controller" {
		conversationID = nativeConversation(run.ID, pentest.AgentRoleController)
	} else if intentID > 0 {
		candidate := run.ID + ":intent:" + strconv.FormatInt(intentID, 10)
		if _, err := store.GetConversation(ctx, candidate); err != nil {
			// Tools may be assembled before the UI's next projection pass. Never
			// attribute a new Worker's writes to the Planner in that window.
			rt, runtimeErr := taskengine.Get(ctx)
			if runtimeErr != nil || syncARTEXRun(ctx, store, rt, run) != nil {
				return capabilities.Host{}, false
			}
			if _, err := store.GetConversation(ctx, candidate); err != nil {
				return capabilities.Host{}, false
			}
		}
		conversationID = candidate
	}
	return capabilities.Host{
		TaskID:         taskID,
		AgentKey:       agentKey,
		ConversationID: conversationID,
		ProjectID:      run.ProjectID,
		WorkDir:        workDir,
		Store:          store,
	}, true
}
