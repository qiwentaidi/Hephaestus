// Package capabilities is the host-side registry shared by the standalone AI
// dialogue and the embedded ARTEX task engine. It exposes one entry per
// capability and keeps MCP/native tool details out of the runtimes: a new MCP
// server is configuration, not a code change in either runtime.
package capabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcpclient"
	"hephaestus-backend/pkg/mcptools"
)

// Runtime identifies the consumer that will present the tool to a model.
type Runtime string

const (
	RuntimeEngine     Runtime = "engine"
	RuntimeStandalone Runtime = "standalone"
)

// Kind identifies where the capability comes from.
type Kind string

const (
	KindNative Kind = "native"
	KindMCP    Kind = "mcp"
)

// ToolDisabled resolves the tool names hidden from a role. The second result is
// false when no per-role policy is configured (every tool visible). It is
// installed by the host MCP layer; nil keeps the default.
var ToolDisabled func(ctx context.Context, role string) (map[string]bool, bool, error)

// NormalizeRole maps engine and standalone role names onto the four agent
// management roles. An empty role is the standalone dialogue.
func NormalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "planner":
		return "planner"
	case "worker":
		return "worker"
	case "controller", "mainagent":
		return "controller"
	default:
		return "pentest"
	}
}

// DisabledTools returns the names a role must not see, or nil when the default
// (all tools visible) applies.
func DisabledTools(ctx context.Context, role string) map[string]bool {
	if ToolDisabled == nil {
		return nil
	}
	disabled, restricted, err := ToolDisabled(ctx, NormalizeRole(role))
	if err != nil || !restricted {
		return nil
	}
	return disabled
}

// Host carries the execution context a capability needs. Missing fields are
// tolerated by capabilities that do not use them.
type Host struct {
	TaskID         int64
	AgentKey       string
	ConversationID string
	ProjectID      string
	WorkDir        string
	Store          *pentest.SQLStore
}

// Tool is one model-facing capability.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Kind        Kind
	ServerID    string
	ServerName  string
	RemoteName  string
	ReadOnly    bool
	Concurrent  bool
	Deferred    bool
}

// Native tools that the engine may use. The list is deliberately curated:
// platform scan/discovery and finding retest tools, without the standalone shell tools
// (Bash already covers execution in the engine) and without platform
// shell_open/shell_send (removed).
var engineNativeTools = map[string]bool{
	"retest_vulnerability":                         true,
	"record_vulnerability_retest":                  true,
	"subdomain_create_task":                        true,
	"subdomain_get_task":                           true,
	"subdomain_list_results":                       true,
	"space_engine_search":                          true,
	"waybackurls":                                  true,
	"attack_surface_create_webscan_task":           true,
	"attack_surface_create_portscan_task":          true,
	"attack_surface_create_dirscan_task":           true,
	"attack_surface_create_crack_task":             true,
	"attack_surface_create_jsscan_task":            true,
	"attack_surface_get_scan_task":                 true,
	"attack_surface_list_scan_results":             true,
	"attack_surface_read_reference":                true,
	"attack_surface_get_jsscan_overview":           true,
	"attack_surface_search_jsscan_operations":      true,
	"attack_surface_search_jsscan_resources":       true,
	"attack_surface_search_jsscan_vulnerabilities": true,
	"attack_surface_get_jsscan_evidence":           true,
	"attack_surface_export_jsscan_openapi":         true,
	"nuclei_list_templates":                        true,
	"nuclei_read_template":                         true,
	"nuclei":                                       true,
	"dns_lookup":                                   true,
}

// plannerNativeTools are read/discovery capabilities the Planner may use. The
// Planner must not launch scans itself; that stays with Workers.
var plannerNativeTools = map[string]bool{
	"retest_vulnerability":                         true,
	"subdomain_create_task":                        true,
	"subdomain_get_task":                           true,
	"subdomain_list_results":                       true,
	"space_engine_search":                          true,
	"waybackurls":                                  true,
	"attack_surface_get_scan_task":                 true,
	"attack_surface_list_scan_results":             true,
	"attack_surface_get_jsscan_overview":           true,
	"attack_surface_search_jsscan_operations":      true,
	"attack_surface_search_jsscan_resources":       true,
	"attack_surface_search_jsscan_vulnerabilities": true,
	"attack_surface_get_jsscan_evidence":           true,
	"nuclei_list_templates":                        true,
	"nuclei_read_template":                         true,
	"dns_lookup":                                   true,
}

// Tools returns the capabilities visible to one runtime role. The engine's
// Planner gets discovery/read tools; Workers and the MainAgent get the full
// engine set. MCP tools are execution capabilities and are offered to Workers
// and the MainAgent, not the Planner.
func Tools(ctx context.Context, runtime Runtime, agentKey string) []Tool {
	disabled := DisabledTools(ctx, agentKey)
	native := nativeTools(agentKey, disabled)
	mcp := mcpTools(agentKey, disabled)
	out := append(native, mcp...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func nativeTools(agentKey string, disabled map[string]bool) []Tool {
	schemas := mcptools.NativeSchemas()
	out := make([]Tool, 0, len(schemas))
	role := strings.ToLower(strings.TrimSpace(agentKey))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Function.Name)
		if name == "" || !engineNativeTools[name] {
			continue
		}
		if role == "planner" && !plannerNativeTools[name] {
			continue
		}
		if disabled[name] {
			continue
		}
		if err := mcptools.AuthorizeTool(name); err != nil {
			continue
		}
		out = append(out, Tool{
			Name:        name,
			Description: schema.Function.Description,
			Schema:      schema.Function.Parameters,
			Kind:        KindNative,
			ReadOnly:    nativeReadOnly(name),
			Concurrent:  nativeReadOnly(name),
		})
	}
	return out
}

func nativeReadOnly(name string) bool {
	switch name {
	case "retest_vulnerability", "subdomain_get_task", "subdomain_list_results", "attack_surface_get_scan_task",
		"attack_surface_read_reference",
		"attack_surface_list_scan_results", "attack_surface_get_jsscan_overview",
		"attack_surface_search_jsscan_operations", "attack_surface_search_jsscan_resources",
		"attack_surface_search_jsscan_vulnerabilities", "attack_surface_get_jsscan_evidence",
		"attack_surface_export_jsscan_openapi", "nuclei_list_templates", "nuclei_read_template",
		"dns_lookup":
		return true
	}
	return false
}

func mcpTools(agentKey string, disabled map[string]bool) []Tool {
	servers, err := mcptools.LoadServers()
	if err != nil {
		return nil
	}
	out := []Tool{}
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		for _, remote := range server.Tools {
			if !server.ToolEnabled(remote.Name) {
				continue
			}
			name := mcpclient.Alias(server.ID, remote.Name)
			if disabled[name] {
				continue
			}
			out = append(out, Tool{
				Name:        name,
				Description: fmt.Sprintf("External MCP / %s / %s: %s", server.Name, remote.Name, remote.Description),
				Schema:      nonNilSchema(remote.InputSchema),
				Kind:        KindMCP,
				ServerID:    server.ID,
				ServerName:  server.Name,
				RemoteName:  remote.Name,
				Concurrent:  false,
			})
		}
	}
	return out
}

func nonNilSchema(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return schema
}

// Call executes one capability. MCP services are re-read on every call so a
// disabled server or tool takes effect immediately in running tasks.
func Call(ctx context.Context, host Host, tool Tool, args map[string]any) (any, error) {
	switch tool.Kind {
	case KindMCP:
		return callMCP(ctx, tool, args)
	case KindNative:
		return callNative(ctx, host, tool, args)
	default:
		return nil, fmt.Errorf("unsupported capability kind %q", tool.Kind)
	}
}

// callMCPServer is a seam for tests; production always uses mcpclient.Call.
var callMCPServer = mcpclient.Call

func callMCP(ctx context.Context, tool Tool, args map[string]any) (any, error) {
	servers, err := mcptools.LoadServers()
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		if !server.Enabled || server.ID != tool.ServerID {
			continue
		}
		for _, remote := range server.Tools {
			if remote.Name != tool.RemoteName || !server.ToolEnabled(remote.Name) {
				continue
			}
			return callMCPServer(ctx, server, remote.Name, args)
		}
		return nil, fmt.Errorf("MCP tool %s/%s is disabled or unavailable", tool.ServerName, tool.RemoteName)
	}
	return nil, fmt.Errorf("MCP server %s is disabled or unavailable", tool.ServerName)
}

func callNative(ctx context.Context, host Host, tool Tool, args map[string]any) (any, error) {
	if err := mcptools.AuthorizeTool(tool.Name); err != nil {
		return nil, err
	}
	cfg := pentest.AgentLoopConfig{
		WorkDir:      host.WorkDir,
		Store:        host.Store,
		Conversation: pentest.Conversation{ID: host.ConversationID, ProjectID: host.ProjectID, Mode: pentest.SoloPentestMode},
		Project:      pentest.Project{ID: host.ProjectID},
	}
	if host.Store != nil {
		if host.ProjectID != "" {
			if project, err := host.Store.GetProject(ctx, host.ProjectID); err == nil {
				cfg.Project = project
			}
		}
		if host.ConversationID != "" {
			if conversation, err := host.Store.GetConversation(ctx, host.ConversationID); err == nil {
				cfg.Conversation = conversation
			}
		}
	}
	call := pentest.AgentToolCall{
		Type:     "function",
		Function: pentest.AgentFunctionCall{Name: tool.Name, Arguments: args},
	}
	out, _, err := mcptools.RunNative(ctx, cfg, call)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// EncodeResult converts an arbitrary tool result into text for the engine's
// actool.Result and for standalone observations.
func EncodeResult(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(raw)
	}
}
