package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
)

func TestPentestSessionSnapshotExcludesOtherURLFacts(t *testing.T) {
	store := openAIPentestHandlerTestStore(t)
	ctx := context.Background()
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "multi URL"})
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Target: "https://192.0.2.10:443", Title: "current"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Target: "https://198.51.100.20", Title: "other"})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		conv pentest.Conversation
		key  string
	}{{current, "js/current"}, {other, "js/other"}} {
		if _, err := store.UpsertFact(ctx, pentest.Fact{ProjectID: project.ID, FactKey: item.key, Category: pentest.CategoryNote, Summary: "JS task " + item.key, SourceConversationID: item.conv.ID, Confidence: pentest.ConfidenceConfirmed}, nil); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := buildPentestSessionSnapshot(ctx, store, current.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	report := snapshot["reportPreview"].(gin.H)
	content := report["content"].(string)
	if !strings.Contains(content, "js/current") || strings.Contains(content, "js/other") {
		t.Fatalf("session report mixed target facts: %s", content)
	}
}
