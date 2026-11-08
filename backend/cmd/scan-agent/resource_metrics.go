package main

import "hephaestus-backend/pkg/agentruntime"

func populateAgentResourceMetrics(opt *agentruntime.NodeUpsertOption) {
	agentruntime.PopulateNodeMetrics(opt)
}
