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

// These capabilities stay available to the in-product AI workbench, but must
// never be advertised by the public Hephaestus MCP endpoint.
var externallyBlockedMCPTools = map[string]struct{}{
	"exec_run":       {},
	"python_execute": {},
}

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
		if profile.Key == mcpHephaestusProfile.Key {
			if _, blocked := externallyBlockedMCPTools[definition.Tool.Name]; blocked {
				continue
			}
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
		"workbench_task_update": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchTaskUpdateArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchTaskUpdate(args)
		},
		"workbench_step_add": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchStepAddArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchStepAdd(args)
		},
		"workbench_fact_upsert": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchFactUpsertArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchFactUpsert(args)
		},
		"workbench_fact_search": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchFactSearchArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchFactSearch(args)
		},
		"workbench_evidence_add": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchEvidenceAddArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchEvidenceAdd(args)
		},
		"workbench_hypothesis_update": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchHypothesisUpdateArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchHypothesisUpdate(args)
		},
		"workbench_scope_check": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchScopeCheckArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchScopeCheck(args)
		},
		"workbench_execution_list": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchExecutionListArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchExecutionList(args)
		},
		"workbench_execution_get": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchExecutionGetArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchExecutionGet(args)
		},
		"workbench_execution_wait": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchExecutionWaitArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchExecutionWait(args)
		},
		"workbench_execution_cancel": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchExecutionCancelArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchExecutionCancel(args)
		},
		"http_request": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchHTTPRequestArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchHTTPRequest(args)
		},
		"http_sequence_run": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchHTTPSequenceRunArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchHTTPSequenceRun(args)
		},
		"http_extract_variable": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchHTTPExtractVariableArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchHTTPExtractVariable(args)
		},
		"http_compare_response": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchHTTPCompareResponseArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchHTTPCompareResponse(args)
		},
		"api_endpoint_record": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchAPIEndpointRecordArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchAPIEndpointRecord(args)
		},
		"api_relation_record": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchAPIRelationRecordArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchAPIRelationRecord(args)
		},
		"api_schema_analyze": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchAPISchemaAnalyzeArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchAPISchemaAnalyze(args)
		},
		"api_doc_test": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchAPIDocTestArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchAPIDocTest(args)
		},
		"exec_run": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchExecRunArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchExecRun(args)
		},
		"python_execute": func(_ *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
			var args mcpWorkbenchPythonExecuteArgs
			if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
				return mcpToolCallResult{}, err
			}
			return mcpCallWorkbenchPythonExecute(args)
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
