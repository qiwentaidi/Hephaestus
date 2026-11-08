package mcphandlers

import (
	"os"
	"testing"

	"hephaestus-backend/handlers/ai"
)

// TestMain 装配 MCP provider 与 AI bridge：MCP 测试会走完整的工具刷新/授权路径。
func TestMain(m *testing.M) {
	RegisterServerProvider()
	ai.RegisterMCPBridge()
	os.Exit(m.Run())
}
