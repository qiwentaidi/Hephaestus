package ai

import (
	"context"
	"testing"

	"hephaestus-backend/lib/pentest"
)

// A settled worker turn must not keep the conversation running just because the
// exploration frontier still holds durable context for a later message.
func TestSettleWorkerConversationStatusDoesNotKeepFrontierOnlyRunning(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, conversation, err := createPentestConversationWithUserMessage(ctx, store, pentest.Project{Name: "settle frontier"}, pentest.Conversation{Mode: pentest.SoloPentestMode, Status: "running"}, "检查目标")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveExplorationGoal(ctx, project.ID, conversation.ID, pentest.ExplorationGoal{Title: "verify the target", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddExplorationIntents(ctx, project.ID, conversation.ID, []pentest.IntentInput{{Summary: "verify the alternate route", Priority: 8}}); err != nil {
		t.Fatal(err)
	}
	conversation.Status = "completed"
	if _, err = store.UpsertConversation(ctx, conversation); err != nil {
		t.Fatal(err)
	}

	settleWorkerConversationStatus(ctx, store, pentest.WorkerTurn{ID: "settled-turn", ConversationID: conversation.ID})

	current, err := store.GetConversation(ctx, conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "completed" {
		t.Fatalf("a frontier-only decision kept the settled turn running: %q", current.Status)
	}
}
