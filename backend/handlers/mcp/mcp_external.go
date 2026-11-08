package mcphandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/capabilities"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/mcpclient"
	"hephaestus-backend/pkg/mcptools"
)

const externalMCPSettingKey = "ai.external_mcp_servers"

var externalMCPMu sync.Mutex

var externalMCPReadSetting = database.GetSetting
var externalMCPWriteSetting = database.SetSetting

// Capability preferences apply to the platform AI, independently of outward MCP.
const aiToolsSettingKey = "ai.tool_preferences"

func loadAIToolPreferences() (map[string]bool, error) {
	raw, err := externalMCPReadSetting(aiToolsSettingKey)
	if err != nil {
		return nil, err
	}
	preferences := map[string]bool{}
	if raw == nil {
		return preferences, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, &preferences)
	return preferences, err
}

func aiCapabilityCatalog() ([]gin.H, error) {
	catalog, err := pentest.LoadSystemTools("")
	if err != nil {
		return nil, err
	}
	// JSScan is intentionally not part of the global AI capability catalog.
	// It is exposed to a dialogue only when that conversation contains an
	// explicit user @-reference to JSScan assets.
	native := mcptools.RemoveJSScanTools(mcptools.NativeSchemas())
	native = append(native, pentest.BlackboardToolSchemas()...)
	native = append(native, pentest.ToolOutputSchema())
	seen := map[string]bool{}
	items := []gin.H{}
	for _, tool := range native {
		seen[tool.Function.Name] = true
		items = append(items, gin.H{"name": tool.Function.Name, "source": "platform", "command": "", "description": tool.Function.Description, "details": tool.Function.Description, "parameters": tool.Function.Parameters})
	}
	for _, tool := range catalog.ListTools() {
		if seen[tool.Name] {
			continue
		}
		seen[tool.Name] = true
		items = append(items, gin.H{"name": tool.Name, "source": "system", "command": tool.Command, "description": mcptools.FirstNonEmpty(tool.ShortDescription, mcptools.FirstLine(tool.Description)), "details": tool.Description, "parameters": tool.Parameters})
	}
	if !seen["exec"] {
		items = append(items, gin.H{"name": "exec", "source": "system", "command": "sh", "description": "Execute a local command"})
	}
	return items, nil
}

func ListSystemToolsHandler(c *gin.Context) {
	items, err := aiCapabilityCatalog()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	preferences, err := loadAIToolPreferences()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	defaults := map[string]bool{}
	for _, name := range pentest.DefaultPentestToolAllowlist() {
		defaults[name] = true
	}
	for _, item := range items {
		name := item["name"].(string)
		enabled, ok := preferences[name]
		if !ok {
			enabled = defaults[name]
		}
		item["enabled"] = enabled
		item["availableToAI"] = enabled
	}
	c.JSON(200, gin.H{"success": true, "data": items})
}

func SetAIToolEnabledHandler(c *gin.Context) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Enabled == nil {
		c.JSON(400, gin.H{"success": false, "error": "enabled is required"})
		return
	}
	items, err := aiCapabilityCatalog()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	found := false
	for _, item := range items {
		if item["name"] == c.Param("name") {
			found = true
			break
		}
	}
	// Goal progress is injected only for goal-mode runs, so it is intentionally
	// absent from the general callable catalog but remains a valid preference key.
	if c.Param("name") == "update_goal_progress" {
		found = true
	}
	if !found {
		c.JSON(404, gin.H{"success": false, "error": "AI capability not found"})
		return
	}
	externalMCPMu.Lock()
	defer externalMCPMu.Unlock()
	preferences, err := loadAIToolPreferences()
	if err == nil {
		preferences[c.Param("name")] = *req.Enabled
		err = externalMCPWriteSetting(aiToolsSettingKey, preferences)
	}
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true})
}

func SetAIToolsEnabledBulkHandler(c *gin.Context) {
	var req struct {
		Names   []string `json:"names"`
		Enabled *bool    `json:"enabled"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Enabled == nil || len(req.Names) == 0 || len(req.Names) > 500 {
		c.JSON(400, gin.H{"success": false, "error": "names and enabled are required (maximum 500 tools)"})
		return
	}
	items, err := aiCapabilityCatalog()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	available := make(map[string]bool, len(items))
	for _, item := range items {
		available[item["name"].(string)] = true
	}
	names := make([]string, 0, len(req.Names))
	seen := make(map[string]bool, len(req.Names))
	for _, name := range req.Names {
		if !available[name] {
			c.JSON(400, gin.H{"success": false, "error": "unknown AI capability"})
			return
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	externalMCPMu.Lock()
	defer externalMCPMu.Unlock()
	preferences, err := loadAIToolPreferences()
	if err == nil {
		for _, name := range names {
			preferences[name] = *req.Enabled
		}
		err = externalMCPWriteSetting(aiToolsSettingKey, preferences)
	}
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"updated": len(names)}})
}

func RefreshPentestTools(cfg *pentest.AgentLoopConfig) error {
	preferences, err := loadAIToolPreferences()
	if err != nil {
		return err
	}
	servers, err := mcptools.LoadServers()
	if err != nil {
		return err
	}
	disabled := capabilities.DisabledTools(context.Background(), cfg.PromptRole)
	allowed := map[string]bool{}
	for _, name := range pentest.DefaultPentestToolAllowlist() {
		allowed[name] = true
	}
	for name, enabled := range preferences {
		allowed[name] = enabled
	}
	for name := range disabled {
		delete(allowed, name)
	}
	cfg.NativeTools = mcptools.NativeSchemas()
	if !mcptools.ConversationHasJSScanAsset(cfg.Conversation.ID) {
		cfg.NativeTools = mcptools.RemoveJSScanTools(cfg.NativeTools)
	} else {
		cfg.NativeTools = mcptools.RemoveJSScanCreateTool(cfg.NativeTools)
	}
	if len(disabled) > 0 {
		filtered := cfg.NativeTools[:0]
		for _, tool := range cfg.NativeTools {
			if !disabled[tool.Function.Name] {
				filtered = append(filtered, tool)
			}
		}
		cfg.NativeTools = filtered
	}
	for _, tool := range mcptools.ExternalSchemas(servers) {
		if disabled[tool.Function.Name] {
			continue
		}
		cfg.NativeTools = append(cfg.NativeTools, tool)
		allowed[tool.Function.Name] = true
	}
	// Empty allowlists mean unrestricted to the standalone kernel.
	cfg.ToolAllowlist = []string{"__no_enabled_capabilities__"}
	for name, enabled := range allowed {
		if enabled {
			cfg.ToolAllowlist = append(cfg.ToolAllowlist, name)
		}
	}
	// Orchestration roles are intentionally narrower than the default solo
	// tool set. Refreshing must not widen a planner or worker back to all tools.
	switch strings.ToLower(strings.TrimSpace(cfg.PromptRole)) {
	case pentest.AgentRoleController:
		cfg.ToolAllowlist = pentest.ControllerToolAllowlist()
	case pentest.AgentRolePlanner:
		cfg.ToolAllowlist = pentest.PlannerToolAllowlist()
	case pentest.AgentRoleWorker:
		cfg.ToolAllowlist = pentest.WorkerToolAllowlist()
	}
	if len(disabled) > 0 {
		filtered := cfg.ToolAllowlist[:0]
		for _, name := range cfg.ToolAllowlist {
			if !disabled[name] {
				filtered = append(filtered, name)
			}
		}
		cfg.ToolAllowlist = filtered
	}
	return nil
}

func AuthorizePentestTool(name string) error {
	if strings.HasPrefix(name, "external_mcp_") {
		return nil
	} // external dispatcher rechecks service and tool state.
	if pentest.IsPlanTool(name) {
		// Planner-only tools are still gated by the conversation role and the
		// planner allowlist at execution time.
		return nil
	}
	preferences, err := loadAIToolPreferences()
	if err != nil {
		return err
	}
	if enabled, ok := preferences[name]; ok {
		if enabled {
			return nil
		}
		return fmt.Errorf("AI capability %s is disabled", name)
	}
	for _, allowed := range pentest.DefaultPentestToolAllowlist() {
		if name == allowed {
			return nil
		}
	}
	return fmt.Errorf("AI capability %s is not enabled", name)
}

func boolToolArgument(args map[string]any, key string) bool {
	value, _ := args[key].(bool)
	return value
}

func explicitlyFalseToolArgument(args map[string]any, key string) bool {
	value, ok := args[key].(bool)
	return ok && !value
}

func intToolArgument(args map[string]any, key string) int {
	switch value := args[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	}
	return 0
}

func connectionFingerprint(s mcpclient.Server) string {
	data, _ := json.Marshal([]any{s.Mode(), s.URL, s.Headers, s.Insecure, s.Command, s.Args, s.Env, s.WorkDir})
	return string(data)
}

func LoadExternalMCPServers() ([]mcpclient.Server, error) {
	raw, err := externalMCPReadSetting(externalMCPSettingKey)
	if err != nil {
		return nil, err
	}
	servers := []mcpclient.Server{}
	if raw == nil {
		return servers, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, &servers)
	return servers, err
}

func externalMCPView(s mcpclient.Server) gin.H {
	return gin.H{"transport": s.Mode(), "command": s.Command, "args": s.Args, "workDir": s.WorkDir, "hasEnv": len(s.Env) > 0, "disabledTools": s.DisabledTools, "id": s.ID, "name": s.Name, "url": s.URL, "enabled": s.Enabled, "insecure": s.Insecure, "hasHeaders": len(s.Headers) > 0, "tools": s.Tools, "checkedAt": s.CheckedAt, "lastError": s.LastError}
}

func ListExternalMCPServersHandler(c *gin.Context) {
	servers, err := mcptools.LoadServers()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	views := make([]gin.H, 0, len(servers))
	for _, s := range servers {
		views = append(views, externalMCPView(s))
	}
	c.JSON(200, gin.H{"success": true, "data": views})
}

func SaveExternalMCPServerHandler(c *gin.Context) {
	var req struct {
		Name          string             `json:"name"`
		Transport     *string            `json:"transport"`
		Command       *string            `json:"command"`
		Args          *[]string          `json:"args"`
		Env           *map[string]string `json:"env"`
		WorkDir       *string            `json:"workDir"`
		DisabledTools *[]string          `json:"disabledTools"`
		URL           string             `json:"url"`
		Enabled       bool               `json:"enabled"`
		Insecure      *bool              `json:"insecure"`
		Headers       *map[string]string `json:"headers"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "Invalid MCP configuration"})
		return
	}
	externalMCPMu.Lock()
	defer externalMCPMu.Unlock()
	servers, err := mcptools.LoadServers()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	id := c.Param("id")
	index := -1
	s := mcpclient.Server{ID: uuid.NewString(), Tools: []mcpclient.Tool{}}
	if id != "" {
		for i, item := range servers {
			if item.ID == id {
				index = i
				s = item
				break
			}
		}
		if index < 0 {
			c.JSON(404, gin.H{"success": false, "error": "MCP service not found"})
			return
		}
	} else if len(servers) >= 50 {
		c.JSON(400, gin.H{"success": false, "error": "Maximum 50 external MCP services"})
		return
	}
	oldConnection := connectionFingerprint(s)
	s.Name, s.URL, s.Enabled = strings.TrimSpace(req.Name), strings.TrimSpace(req.URL), req.Enabled
	if req.Insecure != nil {
		s.Insecure = *req.Insecure
	}
	if req.Headers != nil {
		s.Headers = *req.Headers
	}
	if req.Transport != nil {
		s.Transport = strings.TrimSpace(*req.Transport)
	}
	if req.Command != nil {
		s.Command = strings.TrimSpace(*req.Command)
	}
	if req.Args != nil {
		s.Args = *req.Args
	}
	if req.Env != nil {
		s.Env = *req.Env
	}
	if req.WorkDir != nil {
		s.WorkDir = strings.TrimSpace(*req.WorkDir)
	}
	if req.DisabledTools != nil {
		s.DisabledTools = *req.DisabledTools
	}
	if s.Mode() == "stdio" {
		s.URL = ""
		s.Headers = nil
		s.Insecure = false
	} else {
		s.Command = ""
		s.Args = nil
		s.Env = nil
		s.WorkDir = ""
	}
	if err := mcpclient.Validate(s); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	newConnection := connectionFingerprint(s)
	if string(oldConnection) != string(newConnection) {
		s.Tools = []mcpclient.Tool{}
		s.CheckedAt = ""
		s.LastError = ""
		s.Enabled = false
	}
	if s.Enabled && s.CheckedAt == "" {
		c.JSON(400, gin.H{"success": false, "error": "Discover tools before enabling this service"})
		return
	}
	if index < 0 {
		servers = append(servers, s)
	} else {
		servers[index] = s
	}
	if err := externalMCPWriteSetting(externalMCPSettingKey, servers); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": externalMCPView(s)})
}

func DeleteExternalMCPServerHandler(c *gin.Context) {
	externalMCPMu.Lock()
	defer externalMCPMu.Unlock()
	servers, err := mcptools.LoadServers()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	for i, s := range servers {
		if s.ID != c.Param("id") {
			continue
		}
		if err := externalMCPWriteSetting(externalMCPSettingKey, append(servers[:i], servers[i+1:]...)); err != nil {
			c.JSON(500, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"success": true})
		return
	}
	c.JSON(404, gin.H{"success": false, "error": "MCP service not found"})
}

func DiscoverExternalMCPServerHandler(c *gin.Context) {
	servers, err := mcptools.LoadServers()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	var selected *mcpclient.Server
	for _, s := range servers {
		if s.ID == c.Param("id") {
			copy := s
			selected = &copy
			break
		}
	}
	if selected == nil {
		c.JSON(404, gin.H{"success": false, "error": "MCP service not found"})
		return
	}
	tools, discoveryErr := mcpclient.Discover(c.Request.Context(), *selected)
	externalMCPMu.Lock()
	defer externalMCPMu.Unlock()
	servers, err = mcptools.LoadServers()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	for i, s := range servers {
		if s.ID != selected.ID {
			continue
		}
		before := connectionFingerprint(*selected)
		after := connectionFingerprint(s)
		if string(before) != string(after) {
			c.JSON(409, gin.H{"success": false, "error": "Connection changed; discover tools again"})
			return
		}
		if discoveryErr != nil {
			s.LastError = discoveryErr.Error()
		} else {
			s.Tools = tools
			s.CheckedAt = time.Now().UTC().Format(time.RFC3339)
			s.LastError = ""
		}
		servers[i] = s
		if err := externalMCPWriteSetting(externalMCPSettingKey, servers); err != nil {
			c.JSON(500, gin.H{"success": false, "error": err.Error()})
			return
		}
		if discoveryErr != nil {
			c.JSON(502, gin.H{"success": false, "error": s.LastError})
			return
		}
		c.JSON(200, gin.H{"success": true, "data": externalMCPView(s)})
		return
	}
	c.JSON(404, gin.H{"success": false, "error": "MCP service was deleted"})
}

func ExternalMCPToolSchemas(servers []mcpclient.Server) []pentest.AgentToolSchema {
	tools := []pentest.AgentToolSchema{}
	for _, s := range servers {
		if !s.Enabled {
			continue
		}
		for _, tool := range s.Tools {
			if !s.ToolEnabled(tool.Name) {
				continue
			}
			tools = append(tools, pentest.AgentToolSchema{Type: "function", Function: pentest.AgentToolFunction{
				Name: mcpclient.Alias(s.ID, tool.Name), Description: fmt.Sprintf("External MCP / %s / %s: %s", s.Name, tool.Name, tool.Description), Parameters: tool.InputSchema,
			}})
		}
	}
	return tools
}

func RunPentestMCPTool(ctx context.Context, cfg pentest.AgentLoopConfig, call pentest.AgentToolCall) (any, bool, error) {
	if !strings.HasPrefix(call.Function.Name, "external_mcp_") {
		return mcptools.RunNative(ctx, cfg, call)
	}
	// Re-read before each invocation: disabling/deleting a service takes effect
	// in existing conversations too, without requiring an AI restart.
	servers, err := mcptools.LoadServers()
	if err != nil {
		return nil, true, err
	}
	for _, s := range servers {
		if !s.Enabled {
			continue
		}
		for _, tool := range s.Tools {
			if !s.ToolEnabled(tool.Name) {
				continue
			}
			if mcpclient.Alias(s.ID, tool.Name) == call.Function.Name {
				result, err := mcpclient.Call(ctx, s, tool.Name, call.Function.Arguments)
				return result, true, err
			}
		}
	}
	return nil, true, fmt.Errorf("external MCP tool is disabled, removed, or no longer available")
}

var loadAIToolCallCounts = func(ctx context.Context) (map[string]int64, error) {
	store, err := ai.StoreFromAppDB(ctx)
	if err != nil {
		return nil, err
	}
	return store.ToolCallCounts(ctx)
}

// This endpoint never reads the outward-facing MCP audit log. Discovery and
// manual connection tests do not enter the AI execution store either.
func AIToolCallCountsHandler(c *gin.Context) {
	counts, err := loadAIToolCallCounts(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": "Failed to load AI tool call counts"})
		return
	}
	servers, err := mcptools.LoadServers()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": "Failed to load MCP services"})
		return
	}
	external := map[string]map[string]int64{}
	for _, server := range servers {
		tools := map[string]int64{}
		for _, tool := range server.Tools {
			tools[tool.Name] = counts[mcpclient.Alias(server.ID, tool.Name)]
		}
		external[server.ID] = tools
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"tools": counts, "external": external}})
}
