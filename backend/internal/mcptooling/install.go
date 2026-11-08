// Package mcptooling 装配 AI 与 MCP 两侧的工具实现。
package mcptooling

import (
	"hephaestus-backend/handlers/ai"
	mcphandlers "hephaestus-backend/handlers/mcp"
)

// Install 把 AI 的 PentestBridge 与 MCP 的 ServerProvider 注册到 pkg/mcptools。
// 必须在启动任何 AI worker / MCP 工具调用之前调用。
func Install() {
	mcphandlers.RegisterServerProvider()
	ai.RegisterMCPBridge()
}
