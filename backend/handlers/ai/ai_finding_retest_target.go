package ai

import (
	"context"
	"database/sql"
	"errors"

	"hephaestus-backend/lib/pentest"
)

// The source conversation is evidence provenance. The retest destination is a
// separate, writable conversation and must stay within the finding's project.
type findingRetestTarget struct {
	Mode           string `json:"mode"`
	ConversationID string `json:"conversationId,omitempty"`
	PlanRunID      string `json:"planRunId,omitempty"`
}

func resolveFindingRetestTarget(ctx context.Context, store *pentest.SQLStore, finding pentest.Finding) (findingRetestTarget, error) {
	fallback := findingRetestTarget{Mode: "independent"}
	if finding.ConversationID == "" {
		return fallback, nil
	}
	source, err := store.GetConversation(ctx, finding.ConversationID)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return fallback, err
	}
	if source.ProjectID != finding.ProjectID {
		return fallback, nil
	}
	role, err := store.GetAgentRole(ctx, source.ID)
	if err != nil {
		return fallback, err
	}
	if role.PlanRunID == "" {
		if role.Role == "" {
			return findingRetestTarget{Mode: "conversation", ConversationID: source.ID}, nil
		}
		return fallback, nil
	}
	if !pentest.IsOrchestrationRun(role.PlanRunID) {
		return fallback, nil
	}
	run, err := store.GetPlanRun(ctx, role.PlanRunID)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return fallback, err
	}
	if run.ProjectID != finding.ProjectID {
		return fallback, nil
	}
	// A retest must not implicitly resume a paused/stopping/failed assessment.
	switch run.Status {
	case "paused", "pausing", "cancelling", "cancelled", "failed", "blocked":
		return fallback, nil
	}
	controller, ok := planControllerConversation(ctx, store, run)
	if !ok || controller.ProjectID != finding.ProjectID {
		return fallback, nil
	}
	return findingRetestTarget{Mode: "orchestration", ConversationID: controller.ID, PlanRunID: run.ID}, nil
}
