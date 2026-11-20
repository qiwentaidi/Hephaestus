package mcphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/capabilities"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/mcpclient"
	"hephaestus-backend/pkg/mcptools"
	"hephaestus-backend/pkg/toolruntime"

	"github.com/gin-gonic/gin"
)

// agentToolVisibilityKey stores role -> disabled tool names. A missing key means
// every tool is visible to every role (the default policy). Disabled lists are
// used instead of allowlists so newly added tools stay visible by default.
const agentToolVisibilityKey = "ai.agent_tool_visibility"

// agentMCPVisibilityKey is the previous server-level setting. It is read once
// for migration when no tool-level policy exists yet.
const agentMCPVisibilityKey = "ai.agent_mcp_visibility"

var agentMCPRoles = []string{"planner", "worker", "controller", "pentest"}

var agentToolReadSetting = database.GetSetting
var agentToolWriteSetting = database.SetSetting

func init() {
	capabilities.ToolDisabled = func(_ context.Context, role string) (map[string]bool, bool, error) {
		return AgentToolDisabled(role)
	}
	// The per-role binding (the checkboxes on the visibility sheet) is the
	// authorization source for the standalone agent loop: a tool bound to a role
	// is callable once it is enabled globally and available in this runtime.
	capabilities.RoleTools = func(ctx context.Context, role string) ([]string, error) {
		return enabledVisibleToolNames(ctx, role)
	}
}

type agentTool struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Enabled     bool   `json:"enabled"`
	// Available mirrors the runtime feature gate (background tasks / interactive
	// shell). A tool that is enabled but unavailable in this build cannot be
	// granted, so the sheet renders it disabled instead of letting the operator
	// tick a binding that would never take effect.
	Available  bool   `json:"available"`
	ServerID   string `json:"serverId,omitempty"`
	ServerName string `json:"serverName,omitempty"`
}

func loadAgentToolVisibility() (map[string][]string, error) {
	raw, err := agentToolReadSetting(agentToolVisibilityKey)
	if err != nil || raw == nil {
		return nil, err
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	visibility := map[string][]string{}
	if err = json.Unmarshal(data, &visibility); err != nil {
		return nil, err
	}
	changed := false
	for role, names := range visibility {
		migrated, update := toolruntime.MigrateDisabled(names)
		if update {
			visibility[role] = migrated
			changed = true
		}
	}
	if changed {
		if err := agentToolWriteSetting(agentToolVisibilityKey, visibility); err != nil {
			return nil, err
		}
	}
	return visibility, nil
}

func loadLegacyMCPVisibility() (map[string][]string, error) {
	raw, err := agentToolReadSetting(agentMCPVisibilityKey)
	if err != nil || raw == nil {
		return nil, err
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	visibility := map[string][]string{}
	if err = json.Unmarshal(data, &visibility); err != nil {
		return nil, err
	}
	return visibility, nil
}

// AgentToolDisabled returns the tools hidden from one role. The boolean is
// always true: an empty or missing role policy materializes the least-privilege
// default policy, so callers never fall back to "everything visible".
var agentToolCatalogMu sync.Mutex
var agentToolCatalogAt time.Time
var agentToolCatalogValue []agentTool

func agentToolCatalogCached() ([]agentTool, error) {
	agentToolCatalogMu.Lock()
	defer agentToolCatalogMu.Unlock()
	if agentToolCatalogValue != nil && time.Since(agentToolCatalogAt) < 30*time.Second {
		return agentToolCatalogValue, nil
	}
	tools, err := agentToolCatalog()
	if err != nil {
		return nil, err
	}
	agentToolCatalogValue = tools
	agentToolCatalogAt = time.Now()
	return tools, nil
}

func AgentToolDisabled(role string) (map[string]bool, bool, error) {
	normalized := capabilities.NormalizeRole(role)
	visibility, err := loadAgentToolVisibility()
	if err != nil {
		return nil, false, err
	}
	if visibility != nil {
		if names, ok := visibility[normalized]; ok {
			return nameSet(names), true, nil
		}
	}
	tools, err := agentToolCatalogCached()
	if err != nil {
		return nil, false, err
	}
	disabled := defaultAgentToolDisabled(normalized, tools)
	if legacy, legacyErr := loadLegacyMCPVisibility(); legacyErr == nil && legacy != nil {
		if _, ok := legacy[normalized]; ok {
			legacyDisabled, _, err := AgentToolDisabledForLegacy(normalized, legacy, tools)
			if err != nil {
				return nil, false, err
			}
			disabled = append(disabled, legacyDisabled...)
		}
	}
	return nameSet(disabled), true, nil
}

func nameSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			out[trimmed] = true
		}
	}
	return out
}

// enabledVisibleToolNames is the single authorization source for the standalone
// agent loop. A tool is granted to a role when all of the following hold:
//   - it is enabled globally (AI capability switch for built-ins, server + tool
//     switches for external MCP);
//   - it is available in this runtime (background tasks / interactive shell);
//   - the role's binding does not hide it (the per-role checkboxes).
//
// The code-seeded binding (defaultRoleBinding) keeps the historical
// least-privilege defaults, so an upgrade does not silently widen a role; after
// that only the operator's binding decides.
func enabledVisibleToolNames(ctx context.Context, role string) ([]string, error) {
	disabled, _, err := AgentToolDisabled(role)
	if err != nil {
		// A settings read failure must not silently freeze a role's toolset:
		// keep the global enablement half and treat the binding as open, matching
		// DisabledTools' own degrade-to-unrestricted behaviour.
		disabled = map[string]bool{}
	}
	preferences, err := loadAIToolPreferences()
	if err != nil {
		return nil, err
	}
	allowed := nameSet(pentest.DefaultPentestToolAllowlist())
	for name, enabled := range preferences {
		if enabled {
			allowed[name] = true
			continue
		}
		delete(allowed, name)
	}
	// The AI capability page keeps the command carriers behind the Bash switch.
	if !allowed["Bash"] {
		for _, name := range []string{"Monitor", "shell_open", "shell_send"} {
			delete(allowed, name)
		}
	}
	for name := range disabled {
		delete(allowed, name)
	}
	for name := range allowed {
		if !toolruntime.Available(name) {
			delete(allowed, name)
		}
	}
	// External MCP tools are bound per role exactly like built-ins. Server and
	// per-tool switches are the global half of the decision.
	servers, err := mcptools.LoadServers()
	if err != nil && !errors.Is(err, mcptools.ErrNotConfigured) {
		return nil, err
	}
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		for _, tool := range server.Tools {
			if !server.ToolEnabled(tool.Name) {
				continue
			}
			alias := mcpclient.Alias(server.ID, tool.Name)
			if disabled[alias] {
				continue
			}
			allowed[alias] = true
		}
	}
	names := make([]string, 0, len(allowed))
	for name := range allowed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// planningOnlyTools belong to planning, steering, or goal management. Workers
// should execute assigned intent, not create new directions. The engine's own
// base tools reuse these names, so treating them as role-boundary defaults keeps
// the engine's planner/worker sets intact until an operator re-binds them.
var planningOnlyTools = map[string]bool{
	"add_intent": true, "update_intent": true, "get_exploration_graph": true,
	"expand_digest": true, "update_exploration_goal": true, "update_goal_progress": true,
	"delegate_tasks": true, "set_goals": true, "set_constraints": true,
	"steer_work": true, "kill_work": true, "add_hint": true,
	"block_plan": true, "finish_plan": true, "get_plan_status": true,
	"add_plan_directive": true, "archive_plan_directive": true, "steer_plan_item": true,
	"cancel_plan_item": true, "resume_plan_run": true,
}

func isExecutionTool(name string) bool {
	switch name {
	case "Bash", "Monitor", "shell_open", "shell_send", "execute-python-script", "install-python-package",
		"space_engine_search", "waybackurls", "http-framework-test", "dns_lookup",
		"subdomain_create_task", "nuclei", "retest_vulnerability",
		"record_vulnerability_retest":
		return true
	}
	if strings.HasPrefix(name, "attack_surface_create_") {
		return true
	}
	return strings.HasPrefix(name, "jwt_")
}

// defaultRoleAllows is the code-seeded binding a role starts from — the
// equivalent of ARTEX's built-in tool seeding. It applies only until an operator
// saves the visibility sheet; afterwards the stored binding is authoritative and
// can add or drop any tool the runtime can host.
func defaultRoleAllows(role string, tool agentTool) bool {
	// External MCP stays opt-in for every role: enabling a server must never
	// silently widen every agent's toolset.
	if tool.Source == "mcp" {
		return false
	}
	switch capabilities.NormalizeRole(role) {
	case "pentest":
		return true
	case "worker":
		return !planningOnlyTools[tool.Name]
	case "planner", "controller":
		if tool.Source != "platform" {
			return false
		}
		return !isExecutionTool(tool.Name)
	default:
		return true
	}
}

func defaultAgentToolDisabled(role string, tools []agentTool) []string {
	disabled := make([]string, 0, len(tools))
	for _, tool := range tools {
		if !defaultRoleAllows(role, tool) {
			disabled = append(disabled, tool.Name)
		}
	}
	return disabled
}

func agentToolCatalog() ([]agentTool, error) {
	catalog, err := pentest.LoadSystemTools("")
	if err != nil {
		return nil, err
	}
	servers, err := mcptools.LoadServers()
	if err != nil && err != mcptools.ErrNotConfigured {
		return nil, err
	}
	preferences, err := loadAIToolPreferences()
	if err != nil {
		preferences = map[string]bool{}
	}
	defaults := nameSet(pentest.DefaultPentestToolAllowlist())
	globallyEnabled := func(name string) bool {
		if enabled, ok := preferences[name]; ok {
			return enabled
		}
		return defaults[name]
	}
	seen := map[string]bool{}
	items := []agentTool{}
	add := func(tool agentTool) {
		if tool.Name == "" || seen[tool.Name] {
			return
		}
		seen[tool.Name] = true
		tool.Available = toolruntime.Available(tool.Name)
		items = append(items, tool)
	}
	// Match the AI capability page: JSScan tools are only exposed to a dialogue
	// that explicitly references JSScan assets, so they are not part of the
	// global visibility matrix.
	for _, schema := range mcptools.RemoveJSScanTools(mcptools.NativeSchemas()) {
		name := strings.TrimSpace(schema.Function.Name)
		add(agentTool{Name: name, Title: name, Description: schema.Function.Description, Source: "platform", Enabled: globallyEnabled(name)})
	}
	for _, schema := range pentest.BlackboardToolSchemas() {
		name := strings.TrimSpace(schema.Function.Name)
		add(agentTool{Name: name, Title: name, Description: schema.Function.Description, Source: "platform", Enabled: globallyEnabled(name)})
	}
	if schema := pentest.ToolOutputSchema(); schema.Function.Name != "" {
		add(agentTool{Name: schema.Function.Name, Title: schema.Function.Name, Description: schema.Function.Description, Source: "platform", Enabled: globallyEnabled(schema.Function.Name)})
	}
	for _, tool := range catalog.ListTools() {
		if tool.Name == "exec" {
			continue
		}
		add(agentTool{Name: tool.Name, Title: tool.Name, Description: mcptools.FirstNonEmpty(tool.ShortDescription, mcptools.FirstLine(tool.Description)), Source: "system", Enabled: globallyEnabled(tool.Name)})
	}
	for _, schema := range pentest.NormaToolSchemas() {
		name := schema.Function.Name
		add(agentTool{Name: name, Title: name, Description: schema.Function.Description, Source: "system", Enabled: globallyEnabled(name)})
	}
	for _, server := range servers {
		for _, tool := range server.Tools {
			name := mcpclient.Alias(server.ID, tool.Name)
			add(agentTool{
				Name: name, Title: server.Name + " / " + tool.Name,
				Description: tool.Description, Source: "mcp",
				Enabled:  server.Enabled && server.ToolEnabled(tool.Name),
				ServerID: server.ID, ServerName: server.Name,
			})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Source != items[j].Source {
			return sourceRank(items[i].Source) < sourceRank(items[j].Source)
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func sourceRank(source string) int {
	switch source {
	case "platform":
		return 0
	case "system":
		return 1
	default:
		return 2
	}
}

func GetAgentToolVisibilityHandler(c *gin.Context) {
	tools, err := agentToolCatalog()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	visibility, err := loadAgentToolVisibility()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if visibility == nil {
		visibility = map[string][]string{}
	}
	legacy, err := loadLegacyMCPVisibility()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	known := map[string]bool{}
	for _, tool := range tools {
		known[tool.Name] = true
	}
	for _, role := range agentMCPRoles {
		names, ok := visibility[role]
		if !ok {
			names = defaultAgentToolDisabled(role, tools)
			if legacy != nil {
				if _, exists := legacy[role]; exists {
					extra, _, err := AgentToolDisabledForLegacy(role, legacy, tools)
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
						return
					}
					names = append(names, extra...)
				}
			}
		}
		valid := make([]string, 0, len(names))
		for _, name := range dedupeStrings(names) {
			if known[name] {
				valid = append(valid, name)
			}
		}
		visibility[role] = valid
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"roles": agentMCPRoles, "tools": tools, "visibility": visibility,
	}})
}

func AgentToolDisabledForLegacy(role string, legacy map[string][]string, tools []agentTool) ([]string, bool, error) {
	allowed := nameSet(legacy[role])
	disabled := []string{}
	for _, tool := range tools {
		if tool.Source == "mcp" && !allowed[tool.ServerID] {
			disabled = append(disabled, tool.Name)
		}
	}
	return disabled, true, nil
}

func SetAgentToolVisibilityHandler(c *gin.Context) {
	var req struct {
		Visibility map[string][]string `json:"visibility"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	tools, err := agentToolCatalog()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	known := map[string]bool{}
	for _, tool := range tools {
		known[tool.Name] = true
	}
	next := map[string][]string{}
	for _, role := range agentMCPRoles {
		names, ok := req.Visibility[role]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing role " + role})
			return
		}
		valid := make([]string, 0, len(names))
		for _, name := range dedupeStrings(names) {
			if !known[name] {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "unknown tool " + name})
				return
			}
			valid = append(valid, name)
		}
		next[role] = valid
	}
	for role := range req.Visibility {
		if !isAgentMCPRole(role) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "unknown role " + role})
			return
		}
	}
	if err := agentToolWriteSetting(agentToolVisibilityKey, next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"roles": agentMCPRoles, "visibility": next}})
}

func isAgentMCPRole(role string) bool {
	for _, known := range agentMCPRoles {
		if role == known {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
