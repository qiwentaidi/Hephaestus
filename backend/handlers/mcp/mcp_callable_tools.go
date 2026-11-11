package mcphandlers

import (
	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/mcpclient"
	"hephaestus-backend/pkg/mcptools"
	"hephaestus-backend/pkg/toolruntime"
)

type callableAITool struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Source      string `json:"source"`
	SourceName  string `json:"sourceName,omitempty"`
}

// Use the same preferences and external aliases as the dialogue runtime.
func callableAITools(catalog []gin.H, preferences map[string]bool, servers []mcpclient.Server) []callableAITool {
	defaults := map[string]bool{}
	for _, name := range pentest.DefaultPentestToolAllowlist() {
		defaults[name] = true
	}
	items := make([]callableAITool, 0, len(catalog))
	for _, tool := range catalog {
		name, _ := tool["name"].(string)
		enabled, ok := preferences[name]
		if !ok {
			enabled = defaults[name]
		}
		bashDisabled := toolruntime.IsCommand(name) && preferences["Bash"] == false
		_, bashConfigured := preferences["Bash"]
		if !enabled || name == "" || !toolruntime.Available(name) || (bashConfigured && bashDisabled) {
			continue
		}
		source, _ := tool["source"].(string)
		description, _ := tool["description"].(string)
		sourceName, _ := tool["sourceName"].(string)
		items = append(items, callableAITool{Name: name, DisplayName: name, Description: description, Source: source, SourceName: sourceName})
	}
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		for _, tool := range server.Tools {
			if !server.ToolEnabled(tool.Name) {
				continue
			}
			items = append(items, callableAITool{
				Name: mcpclient.Alias(server.ID, tool.Name), DisplayName: tool.Name,
				Description: tool.Description, Source: "external", SourceName: server.Name,
			})
		}
	}
	return items
}

func ListCallableAIToolsHandler(c *gin.Context) {
	catalog, err := aiCapabilityCatalog()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	preferences, err := loadAIToolPreferences()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	servers, err := mcptools.LoadServers()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": callableAITools(catalog, preferences, servers)})
}
