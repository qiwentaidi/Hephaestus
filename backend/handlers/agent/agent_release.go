package agent

import (
	"github.com/gin-gonic/gin"
	"hephaestus-backend/pkg/agentrelease"
	"net/http"
)

// Public bootstrap information contains no node configuration or credentials.
func AgentReleaseHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	r := agentrelease.Current()
	if c.Query("format") == "text" {
		c.String(http.StatusOK, "%s\n", r.AgentVersion)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": r})
}

func AgentInstallerHandler(c *gin.Context) {
	name := c.Param("file")
	switch name {
	case "install.sh", "install.bat", "install.ps1":
	default:
		c.Status(http.StatusNotFound)
		return
	}
	data, err := agentrelease.Installer(name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", "attachment; filename="+name)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", data)
}
