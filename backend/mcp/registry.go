package mcp

import "github.com/gin-gonic/gin"

type MCPServerDescriptor struct {
	Key             string            `json:"key"`
	Name            string            `json:"name"`
	NameI18n        map[string]string `json:"nameI18n,omitempty"`
	Description     string            `json:"description"`
	DescriptionI18n map[string]string `json:"descriptionI18n,omitempty"`
	Endpoint        string            `json:"endpoint"`
	LegacyEndpoint  string            `json:"legacyEndpoint,omitempty"`
	ToolCount       int               `json:"toolCount"`
	Permissions     []string          `json:"permissions"`
	Aggregate       bool              `json:"aggregate"`
	Enabled         bool              `json:"enabled"`
}

func ListMCPServersHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"success": true,
		"data":    MCPServerDescriptors(),
	})
}

func MCPServerDescriptors() []MCPServerDescriptor {
	aggregateTools := mcpToolsForProfile(mcpHephaestusProfile)
	aggregatePermissions := collectMCPToolPermissions(aggregateTools)
	spaceTools := mcpToolsForProfile(mcpSpaceEngineProfile)
	spacePermissions := collectMCPToolPermissions(spaceTools)

	return []MCPServerDescriptor{
		{
			Key:         mcpHephaestusProfile.Key,
			Name:        "Hephaestus MCP",
			NameI18n:    map[string]string{"en": "Hephaestus MCP", "zh": "Hephaestus MCP 聚合服务"},
			Description: "Aggregate MCP endpoint for Hephaestus platform tools. Configure this endpoint when the AI client should see all allowed platform capabilities.",
			DescriptionI18n: map[string]string{
				"en": "Aggregate MCP endpoint for Hephaestus platform tools. Configure this endpoint when the AI client should see all allowed platform capabilities.",
				"zh": "Hephaestus 平台工具聚合入口。AI 客户端需要看到所有已授权平台能力时配置该入口。",
			},
			Endpoint:    "/api/v1/mcp/hephaestus",
			ToolCount:   len(aggregateTools),
			Permissions: aggregatePermissions,
			Aggregate:   true,
			Enabled:     true,
		},
		{
			Key:         mcpSpaceEngineProfile.Key,
			Name:        "Space Engine MCP",
			NameI18n:    map[string]string{"en": "Space Engine MCP", "zh": "空间引擎 MCP 服务"},
			Description: "Dedicated MCP endpoint for external space engine search and quota/status inspection.",
			DescriptionI18n: map[string]string{
				"en": "Dedicated MCP endpoint for external space engine search and quota/status inspection.",
				"zh": "空间引擎专用 MCP 入口，用于外部空间引擎搜索和配额/配置状态查询。",
			},
			Endpoint:       "/api/v1/mcp/space-engine",
			LegacyEndpoint: "/api/v1/mcp/space-engine",
			ToolCount:      len(spaceTools),
			Permissions:    spacePermissions,
			Aggregate:      false,
			Enabled:        true,
		},
	}
}

func collectMCPToolPermissions(tools []mcpTool) []string {
	seen := make(map[string]bool)
	permissions := make([]string, 0, len(tools))
	for _, tool := range tools {
		permission := tool.Permission
		if permission == "" {
			permission = "read"
		}
		if seen[permission] {
			continue
		}
		seen[permission] = true
		permissions = append(permissions, permission)
	}
	return permissions
}
