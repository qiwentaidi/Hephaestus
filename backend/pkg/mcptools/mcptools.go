// Package mcptools 是 AI 侧与 MCP 侧之间的工具注册表。
//
// 两侧都只依赖本包：AI 侧注册 pentest 工具的实现，MCP 侧注册外部服务与执行器，
// 由 internal/app 在启动时显式装配。这样 ai 与 mcp 不必互相 import。
package mcptools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcpclient"
)

// ErrNotConfigured 表示启动装配缺失，调用方应把它当作初始化错误。
var ErrNotConfigured = errors.New("mcp tool registry is not configured")

// ServerProvider 由 MCP 侧实现：外部 MCP 服务列表、schema、授权与执行。
type ServerProvider interface {
	LoadServers() ([]mcpclient.Server, error)
	ExternalSchemas(servers []mcpclient.Server) []pentest.AgentToolSchema
	Authorize(tool string) error
	RefreshTools(cfg *pentest.AgentLoopConfig) error
	RunExternal(ctx context.Context, cfg pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error)
}

// PentestBridge 由 AI 侧实现：原生 pentest 工具 schema 与执行、JSScan 资产判断。
type PentestBridge interface {
	NativeSchemas() []pentest.AgentToolSchema
	RunNative(ctx context.Context, cfg pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error)
	ConversationHasJSScanAsset(conversationID string) bool
}

var (
	provider ServerProvider
	bridge   PentestBridge
)

// RegisterServerProvider 由 MCP 侧在启动时调用。
func RegisterServerProvider(p ServerProvider) { provider = p }

// RegisterPentestBridge 由 AI 侧在启动时调用。
func RegisterPentestBridge(b PentestBridge) { bridge = b }

// Provider 返回已注册的 MCP 服务提供者。
func Provider() (ServerProvider, error) {
	if provider == nil {
		return nil, ErrNotConfigured
	}
	return provider, nil
}

// Bridge 返回已注册的 AI 工具桥。
func Bridge() (PentestBridge, error) {
	if bridge == nil {
		return nil, ErrNotConfigured
	}
	return bridge, nil
}

// LoadServers 读取配置中的外部 MCP 服务。
func LoadServers() ([]mcpclient.Server, error) {
	p, err := Provider()
	if err != nil {
		return nil, err
	}
	return p.LoadServers()
}

// ExternalSchemas 把外部 MCP 服务转换成 AI 工具 schema。
func ExternalSchemas(servers []mcpclient.Server) []pentest.AgentToolSchema {
	p, err := Provider()
	if err != nil {
		return nil
	}
	return p.ExternalSchemas(servers)
}

// AuthorizeTool 判断工具是否被策略放行。
func AuthorizeTool(tool string) error {
	p, err := Provider()
	if err != nil {
		return err
	}
	return p.Authorize(tool)
}

// RefreshTools 刷新一轮对话可见的工具集合。
func RefreshTools(cfg *pentest.AgentLoopConfig) error {
	p, err := Provider()
	if err != nil {
		return err
	}
	return p.RefreshTools(cfg)
}

// RunTool 执行工具调用：external_mcp_ 前缀走 MCP，其余交给 AI 原生桥。
func RunTool(ctx context.Context, cfg pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error) {
	if strings.HasPrefix(call.Function.Name, "external_mcp_") {
		p, err := Provider()
		if err != nil {
			return nil, true, err
		}
		return p.RunExternal(ctx, cfg, call)
	}
	b, err := Bridge()
	if err != nil {
		return nil, true, err
	}
	return b.RunNative(ctx, cfg, call)
}

// NativeSchemas 返回 AI 原生 pentest 工具 schema。
func NativeSchemas() []pentest.AgentToolSchema {
	b, err := Bridge()
	if err != nil {
		return nil
	}
	return b.NativeSchemas()
}

// RunNative 直接执行 AI 原生工具（供 MCP 侧复用）。
func RunNative(ctx context.Context, cfg pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error) {
	b, err := Bridge()
	if err != nil {
		return nil, true, err
	}
	return b.RunNative(ctx, cfg, call)
}

// ConversationHasJSScanAsset 判断对话是否绑定过 JSScan 资产。
func ConversationHasJSScanAsset(conversationID string) bool {
	b, err := Bridge()
	if err != nil {
		return false
	}
	return b.ConversationHasJSScanAsset(conversationID)
}

const jsScanCreateTool = "attack_surface_create_jsscan_task"

// IsJSScanTool 判断工具名是否属于 JSScan 工具组。
func IsJSScanTool(name string) bool {
	switch strings.TrimSpace(name) {
	case "attack_surface_create_jsscan_task",
		"attack_surface_get_jsscan_overview",
		"attack_surface_search_jsscan_operations",
		"attack_surface_search_jsscan_vulnerabilities",
		"attack_surface_search_jsscan_resources",
		"attack_surface_get_jsscan_evidence",
		"attack_surface_export_jsscan_openapi":
		return true
	default:
		return false
	}
}

// RemoveJSScanTools 过滤掉所有 JSScan 工具。
func RemoveJSScanTools(tools []pentest.AgentToolSchema) []pentest.AgentToolSchema {
	out := make([]pentest.AgentToolSchema, 0, len(tools))
	for _, tool := range tools {
		if IsJSScanTool(tool.Function.Name) {
			continue
		}
		out = append(out, tool)
	}
	return out
}

// RemoveJSScanCreateTool 只过滤“创建 JSScan 任务”这一个工具。
func RemoveJSScanCreateTool(tools []pentest.AgentToolSchema) []pentest.AgentToolSchema {
	out := make([]pentest.AgentToolSchema, 0, len(tools))
	for _, tool := range tools {
		if tool.Function.Name == jsScanCreateTool {
			continue
		}
		out = append(out, tool)
	}
	return out
}

// RemoveJSScanToolNames 过滤工具名列表里的 JSScan 工具。
func RemoveJSScanToolNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if IsJSScanTool(name) {
			continue
		}
		out = append(out, name)
	}
	return out
}

// RemoveJSScanCreateToolName 过滤工具名列表里的 JSScan 创建工具。
func RemoveJSScanCreateToolName(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if name == jsScanCreateTool {
			continue
		}
		out = append(out, name)
	}
	return out
}

// FirstLine 返回文本第一行（去空白）。
func FirstLine(value string) string {
	value = strings.TrimSpace(value)
	if i := strings.IndexByte(value, '\n'); i >= 0 {
		return strings.TrimSpace(value[:i])
	}
	return value
}

// FirstNonEmpty 返回第一个非空字符串。
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// Describe 生成外部 MCP 工具的描述文本。
func Describe(serverName, toolName, description string) string {
	return fmt.Sprintf("External MCP / %s / %s: %s", serverName, toolName, description)
}
