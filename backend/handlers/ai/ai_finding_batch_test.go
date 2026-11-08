package ai

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
)

func TestBatchDispositionPartialSuccessAndHistory(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "batch"})
	if err != nil {
		t.Fatal(err)
	}
	finding, err := store.RecordFinding(ctx, pentest.Finding{ProjectID: project.ID, Title: "evidence fixture", Evidence: "original proof", RetestNotes: "original retest", Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	input := findingDispositionRequest{FindingIDs: []string{finding.ID, "missing", finding.ID}, Status: "ignored", Note: " Risk approved for internal system "}
	if err := validateFindingDisposition(&input); err != nil {
		t.Fatal(err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("PUT", "/api/v1/ai/findings/status", nil)
	c.Set("username", "reviewer")
	result := applyFindingDispositions(c, store, input)
	if result["total"] != 2 || result["updatedCount"] != 1 || result["failedCount"] != 1 {
		t.Fatalf("unexpected results: %+v", result)
	}
	if result["failed"].([]gin.H)[0]["findingId"] != "missing" {
		t.Fatal("failed item not identified")
	}
	reloaded, err := store.GetFinding(ctx, finding.ID)
	if err != nil || reloaded.Status != "accepted_risk" || reloaded.Evidence != "original proof" || reloaded.RetestNotes != "original retest" {
		t.Fatalf("evidence overwritten: %+v %v", reloaded, err)
	}
	history, err := store.ListFindingDispositions(ctx, finding.ID)
	if err != nil || len(history) != 1 || history[0].Note != strings.TrimSpace(input.Note) || history[0].Actor != "reviewer" || history[0].PreviousStatus != "open" {
		t.Fatalf("history missing/duplicated: %+v %v", history, err)
	}
	detail, err := buildPentestFindingDetail(ctx, store, finding.ID)
	if err != nil || len(detail["dispositions"].([]pentest.FindingDisposition)) != 1 {
		t.Fatalf("detail missing disposition: %v", err)
	}
	input.Status, input.Note, input.FindingIDs = "open", "Reopen for verification", []string{finding.ID}
	result = applyFindingDispositions(c, store, input)
	if result["updatedCount"] != 1 {
		t.Fatal(result)
	}
	history, _ = store.ListFindingDispositions(ctx, finding.ID)
	if len(history) != 2 || history[0].Status != "open" || history[0].PreviousStatus != "accepted_risk" {
		t.Fatalf("reopen lost previous disposition: %+v", history)
	}
}

func TestBatchDispositionValidation(t *testing.T) {
	for _, input := range []findingDispositionRequest{
		{Status: "open"},
		{FindingIDs: []string{" "}, Status: "open"},
		{FindingIDs: []string{"id"}, Status: "invalid"},
		{FindingIDs: []string{"id"}, Status: "fixed", Note: strings.Repeat("字", 2001)},
		{FindingIDs: make([]string, 101), Status: "fixed"},
	} {
		if err := validateFindingDisposition(&input); err == nil {
			t.Fatalf("invalid input accepted: %+v", input)
		}
	}
	for _, status := range []string{"false_positive", "ignored", "fixed", "in_progress"} {
		input := findingDispositionRequest{FindingIDs: []string{"id"}, Status: status}
		if err := validateFindingDisposition(&input); err != nil {
			t.Fatalf("status %s should be accepted without a note: %v", status, err)
		}
	}
}

func TestHighRiskAndAcceptedRiskFilteringBeforePagination(t *testing.T) {
	ctx := context.Background()
	store := openAIPentestHandlerTestStore(t)
	for i, status := range []string{"accepted_risk", "ignored", "open"} {
		severity := "high"
		if i == 2 {
			severity = "low"
		}
		if _, err := store.RecordFinding(ctx, pentest.Finding{ProjectID: "fixture", Title: status, Severity: severity, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/ai/findings?highRiskOnly=true&status=ignored", nil)
	filter := pentestFindingFilterFromQuery(c)
	page, total, err := store.QueryFindings(ctx, filter, 1, 1)
	if err != nil || total != 2 || len(page) != 1 || page[0].Severity != "high" {
		t.Fatalf("incorrect filtered page: %+v total=%d err=%v", page, total, err)
	}
}
