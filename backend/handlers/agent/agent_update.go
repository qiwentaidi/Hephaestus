package agent

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"hephaestus-backend/pkg/agentrelease"
	"hephaestus-backend/pkg/agentupdate"
	"hephaestus-backend/pkg/database"
	"net/http"
)

func AgentUpdatesHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	serveAgentUpdates(c, db)
}

func serveAgentUpdates(c *gin.Context, db *gorm.DB) {
	if c.Request.Method == http.MethodGet {
		var jobs []database.AgentUpdate
		if err := db.Order("requested_at DESC").Find(&jobs).Error; err != nil {
			c.JSON(500, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"success": true, "data": gin.H{"targetVersion": agentrelease.Current().AgentVersion, "updates": jobs}})
		return
	}
	if id := c.Param("id"); id != "" {
		var body struct {
			RequestID string `json:"requestId"`
		}
		if c.ShouldBindJSON(&body) != nil || body.RequestID == "" {
			c.JSON(400, gin.H{"success": false, "error": "requestId is required"})
			return
		}
		if err := agentupdate.Cancel(db, id, body.RequestID); err != nil {
			c.JSON(409, gin.H{"success": false, "error": err.Error()})
			return
		}
	} else {
		var body struct {
			AgentIDs []string `json:"agentIds"`
		}
		if c.ShouldBindJSON(&body) != nil {
			c.JSON(400, gin.H{"success": false, "error": "invalid request"})
			return
		}
		if err := agentupdate.Request(db, body.AgentIDs, agentrelease.Current().AgentVersion); err != nil {
			c.JSON(409, gin.H{"success": false, "error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true})
}
