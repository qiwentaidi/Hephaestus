package ai

import (
	"context"
	"testing"
	"time"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine/db"
)

func TestARTEXActivityTurnTokenAttribution(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "token fixture"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Mode: pentest.SoloPentestMode})
	if err != nil {
		t.Fatal(err)
	}
	run := pentest.PlanRun{ID: "artex:tokens"}
	start := time.Now().UTC()
	in, out, cache := 100, 10, 40
	activities := []db.Activity{
		{ID: 1, Kind: "thinking", Detail: "reasoning"},
		{ID: 2, Kind: "text", Detail: "first assistant anchors worker turn"},
		{ID: 3, Kind: "usage", InputTokens: &in, OutputTokens: &out},
		{ID: 4, Kind: "result", Detail: "first result", InputTokens: &in, OutputTokens: &out, CacheReadTokens: &cache},
		{ID: 5, Kind: "result", Detail: "automatic continuation, same visible turn", InputTokens: &in, OutputTokens: &out},
		{ID: 6, Kind: "user", Detail: "explicit follow-up starts another turn"},
		{ID: 7, Kind: "text", Detail: "follow-up response"},
		{ID: 8, Kind: "result", Detail: "follow-up result", InputTokens: &in, OutputTokens: &out},
		{ID: 9, Kind: "result", InputTokens: &in, OutputTokens: &out}, // No message: cannot infer attribution.
	}
	for repeat := 0; repeat < 2; repeat++ {
		for _, a := range activities {
			a.CreatedAt = start.Add(time.Duration(a.ID) * time.Second)
			if err := projectARTEXActivity(ctx, store, run, conv.ID, a); err != nil {
				t.Fatal(err)
			}
		}
	}
	totals, err := store.TurnTokenUsageDetail(ctx, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 2 || totals["artex:tokens:activity:2"].TotalTokens != 220 || totals["artex:tokens:activity:6"].TotalTokens != 110 || totals["artex:tokens:activity:2"].CacheReadTokens != 40 {
		t.Fatalf("wrong visible turn attribution: %+v", totals)
	}
	// Simulate an older, cursor-passed activity whose totals were saved without a link.
	historicalID := "artex:tokens:activity:10"
	if _, err = store.AddMessage(ctx, pentest.Message{ID: historicalID, ConversationID: conv.ID, Role: "assistant", Content: "historical result", CreatedAt: start.Add(10 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordTokenUsage(ctx, historicalID, conv.ID, 50); err != nil {
		t.Fatal(err)
	}
	for repeat := 0; repeat < 2; repeat++ {
		messages, err := store.ListMessages(ctx, conv.ID, -1)
		if err != nil {
			t.Fatal(err)
		}
		_, turns := buildPentestChatForStatus(messages, "completed")
		if err = store.BindMessageTokenUsageTurns(ctx, conv.ID, artexMessageTurnIDs(turns)); err != nil {
			t.Fatal(err)
		}
	}
	totals, err = store.TurnTokenUsageDetail(ctx, conv.ID)
	if err != nil || len(totals) != 2 || totals["artex:tokens:activity:6"].TotalTokens != 160 {
		t.Fatalf("historical attribution: %+v %v", totals, err)
	}
	total, err := store.TotalTokenUsage(ctx, conv.ID)
	if err != nil || total != 490 {
		t.Fatalf("lifetime total changed: %d %v", total, err)
	}
}
