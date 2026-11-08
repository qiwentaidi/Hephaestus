package ai

import (
	"context"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcptools"
)

// aiPentestBridge 把 AI 原生工具实现暴露给 mcptools 注册表。
type aiPentestBridge struct{}

func (aiPentestBridge) NativeSchemas() []pentest.AgentToolSchema {
	return PentestNativeMCPToolSchemas()
}

func (aiPentestBridge) RunNative(ctx context.Context, cfg pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error) {
	return RunPentestNativeMCPTool(ctx, cfg, call)
}

func (aiPentestBridge) ConversationHasJSScanAsset(conversationID string) bool {
	return ConversationHasJSScanAssetReference(conversationID)
}

// RegisterMCPBridge 在装配阶段把 AI 侧实现注册到 mcptools。
func RegisterMCPBridge() {
	mcptools.RegisterPentestBridge(aiPentestBridge{})
}
