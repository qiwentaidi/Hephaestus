package mcp

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gin-gonic/gin"
)

//go:embed tools.json
var embeddedToolCatalog []byte

var (
	toolCatalogOnce        sync.Once
	toolCatalogDefinitions []mcpToolDefinition
	toolCatalogErr         error
)

func mcpCatalogTools() []mcpTool {
	definitions := mcpCatalogToolDefinitions()
	tools := make([]mcpTool, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, definition.Tool)
	}
	return tools
}

func mcpToolsForProfile(profile mcpServerProfile) []mcpTool {
	definitions := mcpToolDefinitionsForProfile(profile)
	tools := make([]mcpTool, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, definition.Tool)
	}
	return tools
}

func mcpToolDefinitionsForProfile(profile mcpServerProfile) []mcpToolDefinition {
	definitions := mcpCatalogToolDefinitions()
	filtered := make([]mcpToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if profile.Key == mcpSpaceEngineProfile.Key && definition.Tool.ModuleKey != "space_engine" {
			continue
		}
		filtered = append(filtered, definition)
	}
	return filtered
}

func mcpCatalogToolDefinitions() []mcpToolDefinition {
	toolCatalogOnce.Do(func() {
		toolCatalogDefinitions, toolCatalogErr = loadMCPToolDefinitions()
	})
	if toolCatalogErr != nil {
		panic(toolCatalogErr)
	}
	return toolCatalogDefinitions
}

func loadMCPToolDefinitions() ([]mcpToolDefinition, error) {
	var tools []mcpTool
	if err := json.Unmarshal(embeddedToolCatalog, &tools); err != nil {
		return nil, fmt.Errorf("load MCP tool catalog: %w", err)
	}

	handlers := map[string]mcpToolHandler{
		"space_engine_search": func(c *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpSpaceSearchArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallSpaceEngineSearch(c, args)
		},
		"space_engine_native_search": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpSpaceNativeSearchArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallSpaceEngineNativeSearch(args)
		},
		"space_engine_get_quota": func(_ *gin.Context, _ json.RawMessage) (mcpToolCallResult, error) {
			return mcpCallSpaceEngineQuota()
		},
		"space_engine_get_config_status": func(_ *gin.Context, _ json.RawMessage) (mcpToolCallResult, error) {
			return mcpCallSpaceEngineConfigStatus()
		},
		"webscan_create_task":                 mcpCreateScanTaskHandler(""),
		"attack_surface_create_webscan_task":  mcpCreateScanTaskHandler("webscan"),
		"attack_surface_create_portscan_task": mcpCreateScanTaskHandler("portscan"),
		"attack_surface_create_dirscan_task":  mcpCreateScanTaskHandler("dirscan"),
		"attack_surface_create_crack_task":    mcpCreateScanTaskHandler("crack"),
		"attack_surface_create_jsscan_task":   mcpCreateScanTaskHandler("jsscan"),
		"jwt_bruteforce": func(c *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpJWTBruteforceArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallJWTBruteforce(c.Request.Context(), args)
		},
		"jwt_list_dictionaries": func(_ *gin.Context, _ json.RawMessage) (mcpToolCallResult, error) {
			return mcpCallJWTListDictionaries()
		},
		"webscan_get_task": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpTaskIDArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWebScanGetTask(args)
		},
		"webscan_list_vulnerabilities": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWebScanVulnerabilityArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWebScanListVulnerabilities(args)
		},
		"webscan_list_results": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWebScanResultsArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWebScanListResults(args)
		},
		"subdomain_create_task": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpSubdomainCreateTaskArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallSubdomainCreateTask(args)
		},
		"subdomain_get_task": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpTaskIDArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallSubdomainGetTask(args)
		},
		"subdomain_list_results": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpSubdomainResultsArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallSubdomainListResults(args)
		},
		"company_create_task": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpCompanyCreateTaskArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallCompanyCreateTask(args)
		},
		"company_get_task": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpTaskIDArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallCompanyGetTask(args)
		},
		"company_get_results": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpTaskIDArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallCompanyGetResults(args)
		},
	}

	definitions := make([]mcpToolDefinition, 0, len(tools))
	for _, tool := range tools {
		handler, ok := handlers[tool.Name]
		if !ok {
			return nil, fmt.Errorf("missing MCP handler for tool %q", tool.Name)
		}
		definitions = append(definitions, mcpToolDefinition{
			Tool:    tool,
			Handler: handler,
		})
	}

	return definitions, nil
}

func mcpCreateScanTaskHandler(scanType string) mcpToolHandler {
	return func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
		var args mcpWebScanCreateTaskArgs
		if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
			return mcpToolCallResult{}, err
		}
		if scanType != "" {
			args.ScanType = scanType
		}
		return mcpCallWebScanCreateTask(args)
	}
}
