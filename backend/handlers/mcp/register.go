package mcphandlers

import (
	"context"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcpclient"
	"hephaestus-backend/pkg/mcptools"
)

// serverProvider 把本包实现适配成 mcptools.ServerProvider。
type serverProvider struct{}

func (serverProvider) LoadServers() ([]mcpclient.Server, error) {
	return LoadExternalMCPServers()
}

func (serverProvider) ExternalSchemas(servers []mcpclient.Server) []pentest.AgentToolSchema {
	return ExternalMCPToolSchemas(servers)
}

func (serverProvider) Authorize(tool string) error {
	return AuthorizePentestTool(tool)
}

func (serverProvider) RefreshTools(cfg *pentest.AgentLoopConfig) error {
	return RefreshPentestTools(cfg)
}

func (serverProvider) RunExternal(ctx context.Context, cfg pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error) {
	return RunPentestMCPTool(ctx, cfg, call)
}

// RegisterServerProvider 在装配阶段把 MCP 侧实现注册到 mcptools。
func RegisterServerProvider() {
	mcptools.RegisterServerProvider(serverProvider{})
}
