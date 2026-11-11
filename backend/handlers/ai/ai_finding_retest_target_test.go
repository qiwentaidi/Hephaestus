package ai

import (
	"context"
	"testing"

	"hephaestus-backend/lib/pentest"
)

func TestFindingRetestDestination(t *testing.T) {
	for _, tc := range []struct {
		name, role, runID, status, want                     string
		missingSource, missingController, foreignController bool
	}{
		{name: "standalone", want: "conversation"},
		{name: "worker routes to main", role: "worker", runID: "orchestration:1", status: "running", want: "orchestration"},
		{name: "completed run supports a one-shot retest", role: "worker", runID: "orchestration:1", status: "completed", want: "orchestration"},
		{name: "planner routes to main", role: "planner", runID: "orchestration:1", status: "running", want: "orchestration"},
		{name: "legacy archive", role: "worker", runID: "legacy-run", want: "independent"},
		{name: "paused run stays paused", role: "worker", runID: "orchestration:1", status: "paused", want: "independent"},
		{name: "pausing run", role: "worker", runID: "orchestration:1", status: "pausing", want: "independent"},
		{name: "cancelled run", role: "worker", runID: "orchestration:1", status: "cancelled", want: "independent"},
		{name: "missing controller", role: "worker", runID: "orchestration:1", missingController: true, want: "independent"},
		{name: "cross-project controller", role: "worker", runID: "orchestration:1", foreignController: true, want: "independent"},
		{name: "missing source", missingSource: true, want: "independent"},
		{name: "unattached worker", role: "worker", want: "independent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store := openAIPentestHandlerTestStore(t)
			project, err := store.UpsertProject(ctx, pentest.Project{Name: "retest"})
			if err != nil {
				t.Fatal(err)
			}
			if !tc.missingSource {
				if _, err = store.UpsertConversation(ctx, pentest.Conversation{ID: "source", ProjectID: project.ID, Mode: pentest.SoloPentestMode}); err != nil {
					t.Fatal(err)
				}
				if tc.role != "" {
					if err = store.SetAgentRole(ctx, "source", tc.role, "", tc.runID); err != nil {
						t.Fatal(err)
					}
				}
			}
			if tc.runID != "" {
				if _, err = store.CreatePlanRun(ctx, pentest.PlanRun{ID: tc.runID, ProjectID: project.ID, PlannerConversationID: "source", Status: tc.status, Objective: "fixture"}); err != nil {
					t.Fatal(err)
				}
				if !tc.missingController {
					controllerProject := project.ID
					if tc.foreignController {
						other, err := store.UpsertProject(ctx, pentest.Project{Name: "other"})
						if err != nil {
							t.Fatal(err)
						}
						controllerProject = other.ID
					}
					if _, err = store.UpsertConversation(ctx, pentest.Conversation{ID: "main", ProjectID: controllerProject, Mode: pentest.SoloPentestMode}); err != nil {
						t.Fatal(err)
					}
					if err = store.SetAgentRole(ctx, "main", pentest.AgentRoleController, "", tc.runID); err != nil {
						t.Fatal(err)
					}
				}
			}
			finding := pentest.Finding{ProjectID: project.ID, ConversationID: "source"}
			target, err := resolveFindingRetestTarget(ctx, store, finding)
			if err != nil || target.Mode != tc.want {
				t.Fatalf("target=%+v err=%v", target, err)
			}
			if tc.want == "orchestration" && (target.ConversationID != "main" || target.PlanRunID != tc.runID) {
				t.Fatalf("wrong main: %+v", target)
			}
			if tc.want == "conversation" && target.ConversationID != "source" {
				t.Fatalf("lost standalone session: %+v", target)
			}
			if tc.want == "independent" && target.ConversationID != "" {
				t.Fatalf("fallback still has a source session: %+v", target)
			}
			if tc.runID != "" && tc.status != "" {
				run, err := store.GetPlanRun(ctx, tc.runID)
				if err != nil || run.Status != tc.status {
					t.Fatalf("routing changed task state: %+v %v", run, err)
				}
			}
		})
	}
}
