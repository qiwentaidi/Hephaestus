package ai

import (
	"context"
	"os"
	"testing"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcpclient"
	"hephaestus-backend/pkg/mcptools"
)

// emptyProvider 是测试用的 MCP 侧桩：没有外部服务，只验证原生工具路径。
type emptyProvider struct{}

func (emptyProvider) LoadServers() ([]mcpclient.Server, error) { return nil, nil }

func (emptyProvider) ExternalSchemas([]mcpclient.Server) []pentest.AgentToolSchema { return nil }

func (emptyProvider) Authorize(string) error { return nil }

func (emptyProvider) RefreshTools(*pentest.AgentLoopConfig) error { return nil }

func (emptyProvider) RunExternal(context.Context, pentest.AgentLoopConfig, pentest.AgentToolCall) (any, bool, error) {
	return nil, true, nil
}

func TestMain(m *testing.M) {
	RegisterMCPBridge()
	mcptools.RegisterServerProvider(emptyProvider{})
	os.Exit(m.Run())
}
