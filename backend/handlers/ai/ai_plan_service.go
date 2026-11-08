package ai

import (
	"context"
	"encoding/json"
	"hephaestus-backend/lib/pentest"
	"strings"
)

// Historical views only. New orchestration runs use the vendored ARTEX engine.
func marshalPlanScope(values []string) string {
	if len(values) == 0 {
		return ""
	}
	data, _ := json.Marshal(values)
	return string(data)
}

func planControllerConversation(ctx context.Context, store *pentest.SQLStore, run pentest.PlanRun) (pentest.Conversation, bool) {
	if store == nil || strings.TrimSpace(run.ID) == "" {
		return pentest.Conversation{}, false
	}
	roles, err := store.ListAgentRolesByPlanRun(ctx, run.ID)
	if err != nil {
		return pentest.Conversation{}, false
	}
	for _, role := range roles {
		if role.Role != pentest.AgentRoleController {
			continue
		}
		conv, err := store.GetConversation(ctx, role.ConversationID)
		if err == nil {
			return conv, true
		}
	}
	return pentest.Conversation{}, false
}

func planRunTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "cancelled", "failed", "blocked":
		return true
	default:
		return false
	}
}

func planConversationDTO(conv pentest.Conversation) map[string]any {
	return map[string]any{
		"id": conv.ID, "projectId": conv.ProjectID, "title": conv.Title, "target": conv.Target,
		"mode": conv.Mode, "status": conv.Status,
	}
}

func planProjectDTO(project pentest.Project) map[string]any {
	return map[string]any{
		"id": project.ID, "name": project.Name, "description": project.Description,
		"status": project.Status,
	}
}

func planRunSnapshot(ctx context.Context, store *pentest.SQLStore, run pentest.PlanRun) (map[string]any, error) {
	items, err := store.ListPlanItems(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	planner, _ := store.GetConversation(ctx, run.PlannerConversationID)
	project, _ := store.GetProject(ctx, run.ProjectID)
	roles, _ := store.ListAgentRolesByPlanRun(ctx, run.ID)
	controller, hasController := planControllerConversation(ctx, store, run)
	if hasController {
		run.ControllerConversationID = controller.ID
	}
	directives, _ := store.ListPlanDirectives(ctx, run.ID, false)
	return map[string]any{
		"run":                    run,
		"items":                  items,
		"plannerConversation":    planConversationDTO(planner),
		"controllerConversation": planConversationDTO(controller),
		"directives":             directives,
		"project":                planProjectDTO(project),
		"agentRoles":             roles,
	}, nil
}
