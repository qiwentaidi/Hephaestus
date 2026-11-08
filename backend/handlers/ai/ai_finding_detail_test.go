package ai

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hephaestus-backend/lib/pentest"
)

func TestFindingDetailUsesRecordedUpstreamEvidenceOnly(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "detail"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.UpsertProject(ctx, pentest.Project{Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "source", Mode: pentest.SoloPentestMode})
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: other.ID, Title: "unrelated", Mode: pentest.SoloPentestMode})
	if err != nil {
		t.Fatal(err)
	}
	executionIDs := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for i, id := range executionIDs {
		owner := conv.ID
		if i == 2 {
			owner = outsider.ID
		}
		if _, err := store.SaveToolExecution(ctx, pentest.ToolExecution{ID: id, ConversationID: owner, ToolName: "fixture", ArgumentsJSON: "{}", ResultJSON: `{"evidence":"fixture"}`, Status: "completed", StartTime: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	finding, err := store.RecordFinding(ctx, pentest.Finding{ProjectID: project.ID, ConversationID: conv.ID, Title: "fixture", SourceFactKey: "finding.source", Evidence: "tool_call:" + executionIDs[2], ReproductionSteps: "recorded steps", Severity: "high"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertFact(ctx, pentest.Fact{ProjectID: project.ID, FactKey: "auto.clue.origin", Summary: "original evidence", Category: "observation", SourceConversationID: conv.ID, SourceMessageID: executionIDs[0]}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertFact(ctx, pentest.Fact{ProjectID: project.ID, FactKey: "finding.source", Summary: "confirmed source", Category: "observation", SourceConversationID: conv.ID}, []pentest.FactLinkInput{{From: "auto.clue.origin", Type: "supports"}}); err != nil {
		t.Fatal(err)
	}
	data, err := buildPentestFindingDetail(ctx, store, finding.ID)
	if err != nil {
		t.Fatal(err)
	}
	facts := data["facts"].([]gin.H)
	executions := data["executionRefs"].([]gin.H)
	if len(facts) != 2 || len(data["edges"].([]gin.H)) != 1 || len(executions) != 1 || executions[0]["id"] != executionIDs[0] {
		t.Fatalf("incorrect evidence links: facts=%d executions=%v", len(facts), executions)
	}
	if len(data["missingEvidenceRefs"].([]string)) != 1 {
		t.Fatal("cross-project reference not reported unavailable")
	}
	if _, err := buildPentestFindingDetail(ctx, store, "not-found"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing finding: %v", err)
	}
	updated, err := store.UpdateFindingStatus(ctx, finding.ID, "in_progress")
	if err != nil || updated.Status != "confirmed" || updated.Evidence != finding.Evidence || updated.ReproductionSteps != finding.ReproductionSteps {
		t.Fatalf("status update altered evidence: %+v %v", updated, err)
	}
	if _, err := store.UpdateFindingStatus(ctx, finding.ID, "invalid"); err == nil {
		t.Fatal("invalid status accepted")
	}
	if _, err := store.UpdateFindingStatus(ctx, "not-found", "fixed"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing update: %v", err)
	}
}
