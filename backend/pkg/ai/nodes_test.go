package ai

import (
	"testing"
	"time"
)

func TestBuildWorkbenchNodeCatalogIncludesLocalAndAgentNodes(t *testing.T) {
	now := time.Now()
	nodes := BuildWorkbenchNodeCatalog([]WorkbenchAgentNodeSnapshot{
		{
			AgentID:            "agent-1",
			DisplayName:        "Agent 1",
			AgentVersion:       "1.2.3",
			Enabled:            true,
			Status:             "online",
			LastHeartbeat:      now,
			LastSeenIP:         "10.0.0.2",
			CPUUsagePercent:    10,
			MemoryUsagePercent: 20,
			DiskUsagePercent:   30,
		},
	})

	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %#v", nodes)
	}
	assertNode(t, nodes, "ai.orchestrator.local", "orchestrator", "online")
	assertNode(t, nodes, "hephaestus.local", "tool_runtime", "online")
	assertNode(t, nodes, "scan-agent.agent-1", "scan_agent", "online")
}

func TestBuildWorkbenchNodeCatalogMarksDisabledAgent(t *testing.T) {
	nodes := BuildWorkbenchNodeCatalog([]WorkbenchAgentNodeSnapshot{
		{
			AgentID:       "agent-1",
			Enabled:       false,
			Status:        "online",
			LastHeartbeat: time.Now(),
		},
	})

	assertNode(t, nodes, "scan-agent.agent-1", "scan_agent", "disabled")
}

func TestSelectWorkbenchNodeForToolUsesLocalCapabilityNode(t *testing.T) {
	nodes := BuildWorkbenchNodeCatalog([]WorkbenchAgentNodeSnapshot{
		{
			AgentID:       "agent-1",
			Enabled:       true,
			Status:        "online",
			LastHeartbeat: time.Now(),
		},
	})
	assignment := SelectWorkbenchNodeForTool("web.fingerprint", nodes)
	if assignment.NodeID != "hephaestus.local" || assignment.NodeType != "tool_runtime" {
		t.Fatalf("unexpected assignment: %#v", assignment)
	}
	if assignment.DispatchMode != "local" || assignment.RemoteCandidateNodeID != "scan-agent.agent-1" {
		t.Fatalf("expected local dispatch with remote candidate, got %#v", assignment)
	}
}

func TestSelectWorkbenchNodeForToolFallsBackToOrchestrator(t *testing.T) {
	nodes := BuildWorkbenchNodeCatalog(nil)
	assignment := SelectWorkbenchNodeForTool("unknown.tool", nodes)
	if assignment.NodeID != "ai.orchestrator.local" || assignment.NodeType != "orchestrator" {
		t.Fatalf("unexpected fallback assignment: %#v", assignment)
	}
}

func TestSelectWorkbenchNodeForToolUsesRemoteWhenLocalRuntimeUnavailable(t *testing.T) {
	nodes := []WorkbenchNodeDescriptor{
		{
			ID:           "ai.orchestrator.local",
			NodeType:     "orchestrator",
			Status:       "online",
			Capabilities: []string{"planning"},
		},
		{
			ID:           "scan-agent.agent-1",
			NodeType:     "scan_agent",
			Status:       "online",
			Capabilities: []string{"jsscan"},
		},
	}

	assignment := SelectWorkbenchNodeForTool("web.jsscan", nodes)
	if assignment.NodeID != "scan-agent.agent-1" || assignment.DispatchMode != "remote" {
		t.Fatalf("expected remote assignment, got %#v", assignment)
	}
}

func TestBuildWorkbenchNodeDispatchAudit(t *testing.T) {
	audit := BuildWorkbenchNodeDispatchAudit("task/1", "step 1", "exec-1", "web.jsscan", WorkbenchNodeAssignment{
		NodeID:                  "hephaestus.local",
		NodeType:                "tool_runtime",
		Status:                  "online",
		DispatchMode:            "local",
		RemoteCandidateNodeID:   "scan-agent.agent-1",
		RemoteCandidateNodeType: "scan_agent",
		Reason:                  "本地执行，远程候选可用。",
	})

	if audit.DispatchMode != "local" || audit.Status != "local_dispatched_remote_candidate_available" {
		t.Fatalf("unexpected audit: %#v", audit)
	}
	if audit.MessageRef != "ai://tasks/task_1/steps/step_1/executions/exec-1/nodes/hephaestus.local" {
		t.Fatalf("unexpected message ref: %s", audit.MessageRef)
	}
}

func TestBuildWorkbenchScanDispatchDecisionKeepsLocalByDefault(t *testing.T) {
	plan := WorkbenchScanExecutionPlan{ScanMode: "local", ToolName: "web.jsscan"}
	assignment := WorkbenchNodeAssignment{
		NodeID:                  "hephaestus.local",
		NodeType:                "tool_runtime",
		Status:                  "online",
		DispatchMode:            "local",
		RemoteCandidateNodeID:   "scan-agent.agent-1",
		RemoteCandidateNodeType: "scan_agent",
		RemoteCandidateStatus:   "online",
	}

	decision := BuildWorkbenchScanDispatchDecision(plan, assignment, false)
	if decision.Plan.ScanMode != "local" || decision.AssignedAgentID != "" || decision.RemotePromoted {
		t.Fatalf("expected local dispatch by default, got %#v", decision)
	}
	if decision.Assignment.NodeID != "hephaestus.local" {
		t.Fatalf("expected local assignment to remain selected, got %#v", decision.Assignment)
	}
}

func TestBuildWorkbenchScanDispatchDecisionPromotesAllowedRemoteCandidate(t *testing.T) {
	plan := WorkbenchScanExecutionPlan{ScanMode: "local", ToolName: "web.jsscan"}
	assignment := WorkbenchNodeAssignment{
		NodeID:                  "hephaestus.local",
		NodeType:                "tool_runtime",
		Status:                  "online",
		DispatchMode:            "local",
		RemoteCandidateNodeID:   "scan-agent.agent-1",
		RemoteCandidateNodeType: "scan_agent",
		RemoteCandidateStatus:   "online",
	}

	decision := BuildWorkbenchScanDispatchDecision(plan, assignment, true)
	if decision.Plan.ScanMode != "agent" || decision.AssignedAgentID != "agent-1" || !decision.RemotePromoted {
		t.Fatalf("expected promoted remote dispatch, got %#v", decision)
	}
	if decision.Assignment.NodeID != "scan-agent.agent-1" || decision.Assignment.DispatchMode != "remote" {
		t.Fatalf("expected remote assignment, got %#v", decision.Assignment)
	}
}

func TestBuildWorkbenchScanDispatchDecisionUsesSelectedRemoteNode(t *testing.T) {
	plan := WorkbenchScanExecutionPlan{ScanMode: "local", ToolName: "web.jsscan"}
	assignment := WorkbenchNodeAssignment{
		NodeID:       "scan-agent.agent-2",
		NodeType:     "scan_agent",
		Status:       "online",
		DispatchMode: "remote",
	}

	decision := BuildWorkbenchScanDispatchDecision(plan, assignment, false)
	if decision.Plan.ScanMode != "agent" || decision.AssignedAgentID != "agent-2" {
		t.Fatalf("expected selected remote node to use agent mode, got %#v", decision)
	}
}

func TestBuildWorkbenchScanDispatchDecisionDowngradesAgentPlanWithoutCandidate(t *testing.T) {
	decision := BuildWorkbenchScanDispatchDecision(
		WorkbenchScanExecutionPlan{ScanMode: "agent", ToolName: "web.jsscan"},
		WorkbenchNodeAssignment{NodeID: "hephaestus.local", NodeType: "tool_runtime", DispatchMode: "local"},
		false,
	)

	if decision.Plan.ScanMode != "local" || decision.AssignedAgentID != "" {
		t.Fatalf("expected agent plan without candidate to downgrade local, got %#v", decision)
	}
}

func assertNode(t *testing.T, nodes []WorkbenchNodeDescriptor, id string, nodeType string, status string) {
	t.Helper()
	for _, node := range nodes {
		if node.ID != id {
			continue
		}
		if node.NodeType != nodeType || node.Status != status {
			t.Fatalf("node %s = %#v, want type=%s status=%s", id, node, nodeType, status)
		}
		return
	}
	t.Fatalf("node %s not found in %#v", id, nodes)
}
