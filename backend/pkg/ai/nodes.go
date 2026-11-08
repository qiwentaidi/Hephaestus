package ai

import (
	"strings"
	"time"
)

// WorkbenchAgentNodeSnapshot 是 AI 节点目录对扫描 Agent 的最小依赖视图。
type WorkbenchAgentNodeSnapshot struct {
	AgentID            string
	DisplayName        string
	AgentVersion       string
	Enabled            bool
	Status             string
	LastHeartbeat      time.Time
	LastSeenIP         string
	LastSeenHost       string
	CPUUsagePercent    float64
	MemoryUsagePercent float64
	DiskUsagePercent   float64
}

// WorkbenchNodeDescriptor 描述 AI 工作台可协作或可调度的节点。
type WorkbenchNodeDescriptor struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	NodeType     string         `json:"nodeType"`
	Status       string         `json:"status"`
	Capabilities []string       `json:"capabilities"`
	LastSeenAt   *time.Time     `json:"lastSeenAt,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// WorkbenchNodeAssignment 记录某次工具调度被分派给哪个 AI/执行节点。
type WorkbenchNodeAssignment struct {
	NodeID                  string `json:"nodeId"`
	NodeType                string `json:"nodeType"`
	Status                  string `json:"status"`
	DispatchMode            string `json:"dispatchMode"`
	RemoteCandidateNodeID   string `json:"remoteCandidateNodeId,omitempty"`
	RemoteCandidateNodeType string `json:"remoteCandidateNodeType,omitempty"`
	RemoteCandidateStatus   string `json:"remoteCandidateStatus,omitempty"`
	Reason                  string `json:"reason"`
}

// WorkbenchNodeDispatchAudit 是一次 AI 工具调用的节点间调度审计引用。
type WorkbenchNodeDispatchAudit struct {
	MessageRef              string `json:"messageRef"`
	ToolName                string `json:"toolName"`
	SelectedNodeID          string `json:"selectedNodeId"`
	SelectedNodeType        string `json:"selectedNodeType"`
	DispatchMode            string `json:"dispatchMode"`
	RemoteCandidateNodeID   string `json:"remoteCandidateNodeId,omitempty"`
	RemoteCandidateNodeType string `json:"remoteCandidateNodeType,omitempty"`
	Status                  string `json:"status"`
	Reason                  string `json:"reason"`
}

// WorkbenchScanDispatchDecision 描述扫描执行计划最终使用本地还是 Scan Agent 分派。
type WorkbenchScanDispatchDecision struct {
	Plan            WorkbenchScanExecutionPlan `json:"plan"`
	Assignment      WorkbenchNodeAssignment    `json:"assignment"`
	AssignedAgentID string                     `json:"assignedAgentId,omitempty"`
	RemotePromoted  bool                       `json:"remotePromoted"`
	Reason          string                     `json:"reason"`
}

// BuildWorkbenchNodeCatalog 把本地编排器、Hephaestus 本地能力和 Scan Agent 统一成 AI 节点目录。
func BuildWorkbenchNodeCatalog(agentNodes []WorkbenchAgentNodeSnapshot) []WorkbenchNodeDescriptor {
	nodes := []WorkbenchNodeDescriptor{
		{
			ID:       "ai.orchestrator.local",
			Title:    "AI 智脑编排器",
			NodeType: "orchestrator",
			Status:   "online",
			Capabilities: []string{
				"planning",
				"hypothesis_generation",
				"strategy_packs",
				"goal_evaluation",
				"reporting",
			},
		},
		{
			ID:       "hephaestus.local",
			Title:    "Hephaestus 本地能力",
			NodeType: "tool_runtime",
			Status:   "online",
			Capabilities: []string{
				"asset.subdomain",
				"asset.portscan",
				"space.search",
				"web.fingerprint",
				"web.dirscan",
				"web.jsscan",
				"web.vuln.verify",
				"report.markdown",
			},
		},
	}

	for _, agent := range agentNodes {
		status := agent.Status
		if !agent.Enabled {
			status = "disabled"
		}
		lastSeenAt := agent.LastHeartbeat
		title := agent.DisplayName
		if title == "" {
			title = agent.AgentID
		}
		nodes = append(nodes, WorkbenchNodeDescriptor{
			ID:       "scan-agent." + agent.AgentID,
			Title:    title,
			NodeType: "scan_agent",
			Status:   status,
			Capabilities: []string{
				"distributed_scan",
				"webscan",
				"dirscan",
				"jsscan",
				"portscan",
			},
			LastSeenAt: &lastSeenAt,
			Metadata: map[string]any{
				"agentId":            agent.AgentID,
				"agentVersion":       agent.AgentVersion,
				"enabled":            agent.Enabled,
				"lastSeenIp":         agent.LastSeenIP,
				"lastSeenHost":       agent.LastSeenHost,
				"cpuUsagePercent":    agent.CPUUsagePercent,
				"memoryUsagePercent": agent.MemoryUsagePercent,
				"diskUsagePercent":   agent.DiskUsagePercent,
			},
		})
	}
	return nodes
}

// SelectWorkbenchNodeForTool 基于节点能力为工具调用选择审计节点。
func SelectWorkbenchNodeForTool(toolName string, nodes []WorkbenchNodeDescriptor) WorkbenchNodeAssignment {
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if toolName == "" {
		return WorkbenchNodeAssignment{
			NodeID:       "ai.orchestrator.local",
			NodeType:     "orchestrator",
			Status:       "online",
			DispatchMode: "local",
			Reason:       "工具名为空，由本地编排器保底记录。",
		}
	}

	for _, node := range nodes {
		if node.ID != "hephaestus.local" || node.Status != "online" {
			continue
		}
		if nodeHasCapability(node, toolName) {
			return attachWorkbenchRemoteCandidate(toolName, nodes, WorkbenchNodeAssignment{
				NodeID:       node.ID,
				NodeType:     node.NodeType,
				Status:       node.Status,
				DispatchMode: "local",
				Reason:       "工具能力由 Hephaestus 本地执行层承载。",
			})
		}
	}

	for _, node := range nodes {
		if node.NodeType != "scan_agent" || node.Status != "online" {
			continue
		}
		if nodeHasCapability(node, toolName) || nodeHasCapability(node, workbenchRemoteCapabilityForTool(toolName)) {
			return WorkbenchNodeAssignment{
				NodeID:       node.ID,
				NodeType:     node.NodeType,
				Status:       node.Status,
				DispatchMode: "remote",
				Reason:       "工具能力可由在线 Scan Agent 远程执行，已记录远程调度节点。",
			}
		}
	}

	for _, node := range nodes {
		if node.ID != "ai.orchestrator.local" || node.Status != "online" {
			continue
		}
		return attachWorkbenchRemoteCandidate(toolName, nodes, WorkbenchNodeAssignment{
			NodeID:       node.ID,
			NodeType:     node.NodeType,
			Status:       node.Status,
			DispatchMode: "local",
			Reason:       "未找到匹配工具运行节点，由本地编排器保底记录。",
		})
	}

	return WorkbenchNodeAssignment{
		NodeID:       "ai.orchestrator.local",
		NodeType:     "orchestrator",
		Status:       "unknown",
		DispatchMode: "local",
		Reason:       "节点目录为空，由本地编排器保底记录。",
	}
}

// BuildWorkbenchScanDispatchDecision 将节点审计选择转换成扫描创建入口可执行的 scanMode/agentId。
func BuildWorkbenchScanDispatchDecision(plan WorkbenchScanExecutionPlan, assignment WorkbenchNodeAssignment, allowRemoteCandidate bool) WorkbenchScanDispatchDecision {
	plan.ScanMode = strings.ToLower(strings.TrimSpace(plan.ScanMode))
	if plan.ScanMode != "agent" {
		plan.ScanMode = "local"
	}
	decision := WorkbenchScanDispatchDecision{
		Plan:       plan,
		Assignment: assignment,
		Reason:     "保持本地扫描执行。",
	}

	if assignment.DispatchMode == "remote" && assignment.NodeType == "scan_agent" {
		if agentID := WorkbenchAgentIDFromNodeID(assignment.NodeID); agentID != "" {
			decision.Plan.ScanMode = "agent"
			decision.AssignedAgentID = agentID
			decision.Reason = "节点选择结果要求远程 Scan Agent 执行。"
			return decision
		}
	}

	if plan.ScanMode == "agent" {
		if agentID, nodeID := workbenchRemoteAgentCandidate(assignment); agentID != "" {
			decision.Plan.ScanMode = "agent"
			decision.AssignedAgentID = agentID
			decision.Assignment = promoteWorkbenchRemoteAssignment(assignment, nodeID, "执行计划请求 agent 模式，已使用匹配的在线 Scan Agent。")
			decision.RemotePromoted = decision.Assignment.NodeID != assignment.NodeID
			decision.Reason = decision.Assignment.Reason
			return decision
		}
		decision.Plan.ScanMode = "local"
		decision.Reason = "执行计划请求 agent 模式，但没有可用 Scan Agent，已降级本地执行。"
		return decision
	}

	if allowRemoteCandidate && assignment.RemoteCandidateNodeID != "" {
		if agentID := WorkbenchAgentIDFromNodeID(assignment.RemoteCandidateNodeID); agentID != "" {
			decision.Plan.ScanMode = "agent"
			decision.AssignedAgentID = agentID
			decision.Assignment = promoteWorkbenchRemoteAssignment(assignment, assignment.RemoteCandidateNodeID, "远程分派已开启，在线 Scan Agent 候选被提升为实际执行节点。")
			decision.RemotePromoted = true
			decision.Reason = decision.Assignment.Reason
			return decision
		}
	}

	return decision
}

func BuildWorkbenchNodeDispatchAudit(taskID string, stepID string, executionID string, toolName string, assignment WorkbenchNodeAssignment) WorkbenchNodeDispatchAudit {
	dispatchMode := strings.TrimSpace(assignment.DispatchMode)
	if dispatchMode == "" {
		dispatchMode = "local"
	}
	status := "local_dispatched"
	if dispatchMode == "remote" {
		status = "remote_dispatched"
	} else if assignment.RemoteCandidateNodeID != "" {
		status = "local_dispatched_remote_candidate_available"
	}
	return WorkbenchNodeDispatchAudit{
		MessageRef:              buildWorkbenchNodeMessageRef(taskID, stepID, executionID, assignment.NodeID),
		ToolName:                strings.ToLower(strings.TrimSpace(toolName)),
		SelectedNodeID:          assignment.NodeID,
		SelectedNodeType:        assignment.NodeType,
		DispatchMode:            dispatchMode,
		RemoteCandidateNodeID:   assignment.RemoteCandidateNodeID,
		RemoteCandidateNodeType: assignment.RemoteCandidateNodeType,
		Status:                  status,
		Reason:                  assignment.Reason,
	}
}

func nodeHasCapability(node WorkbenchNodeDescriptor, capability string) bool {
	for _, item := range node.Capabilities {
		if strings.EqualFold(strings.TrimSpace(item), capability) {
			return true
		}
	}
	return false
}

func attachWorkbenchRemoteCandidate(toolName string, nodes []WorkbenchNodeDescriptor, assignment WorkbenchNodeAssignment) WorkbenchNodeAssignment {
	if assignment.DispatchMode == "" {
		assignment.DispatchMode = "local"
	}
	if assignment.NodeType == "scan_agent" {
		return assignment
	}
	capability := workbenchRemoteCapabilityForTool(toolName)
	for _, node := range nodes {
		if node.NodeType != "scan_agent" || node.Status != "online" {
			continue
		}
		if !nodeHasCapability(node, toolName) && !nodeHasCapability(node, capability) {
			continue
		}
		assignment.RemoteCandidateNodeID = node.ID
		assignment.RemoteCandidateNodeType = node.NodeType
		assignment.RemoteCandidateStatus = node.Status
		return assignment
	}
	return assignment
}

func WorkbenchAgentIDFromNodeID(nodeID string) string {
	nodeID = strings.TrimSpace(nodeID)
	const prefix = "scan-agent."
	if !strings.HasPrefix(nodeID, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(nodeID, prefix))
}

func workbenchRemoteAgentCandidate(assignment WorkbenchNodeAssignment) (string, string) {
	if assignment.NodeType == "scan_agent" {
		if agentID := WorkbenchAgentIDFromNodeID(assignment.NodeID); agentID != "" {
			return agentID, assignment.NodeID
		}
	}
	if assignment.RemoteCandidateNodeID != "" {
		if agentID := WorkbenchAgentIDFromNodeID(assignment.RemoteCandidateNodeID); agentID != "" {
			return agentID, assignment.RemoteCandidateNodeID
		}
	}
	return "", ""
}

func promoteWorkbenchRemoteAssignment(assignment WorkbenchNodeAssignment, nodeID string, reason string) WorkbenchNodeAssignment {
	assignment.NodeID = nodeID
	assignment.NodeType = "scan_agent"
	if assignment.RemoteCandidateStatus != "" {
		assignment.Status = assignment.RemoteCandidateStatus
	}
	if assignment.Status == "" {
		assignment.Status = "online"
	}
	assignment.DispatchMode = "remote"
	assignment.RemoteCandidateNodeID = ""
	assignment.RemoteCandidateNodeType = ""
	assignment.RemoteCandidateStatus = ""
	assignment.Reason = reason
	return assignment
}

func workbenchRemoteCapabilityForTool(toolName string) string {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "web.fingerprint", "web.vuln.verify":
		return "webscan"
	case "web.dirscan":
		return "dirscan"
	case "web.jsscan":
		return "jsscan"
	case "asset.portscan":
		return "portscan"
	default:
		return strings.ToLower(strings.TrimSpace(toolName))
	}
}

func buildWorkbenchNodeMessageRef(taskID string, stepID string, executionID string, nodeID string) string {
	segments := []string{"ai://tasks", taskID, "steps", stepID, "executions", executionID, "nodes", nodeID}
	for i, segment := range segments {
		if i%2 == 0 {
			continue
		}
		segments[i] = sanitizeWorkbenchNodeRefSegment(segment)
	}
	return strings.Join(segments, "/")
}

func sanitizeWorkbenchNodeRefSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", "\n", "_", "\r", "_", "\t", "_")
	return replacer.Replace(value)
}
