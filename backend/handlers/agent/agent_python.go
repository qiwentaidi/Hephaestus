package agent

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/nodepython"
)

func AgentPythonHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	serveAgentPython(c, db)
}

func serveAgentPython(c *gin.Context, db *gorm.DB) {
	var node database.ScanAgentNode
	if err := db.First(&node, "agent_id=?", c.Param("id")).Error; err != nil {
		c.JSON(404, gin.H{"success": false, "error": "Agent not found"})
		return
	}
	key := nodepython.Key(node)
	if c.Request.Method == http.MethodGet {
		state := database.AgentPythonConfig{Mode: "auto", Status: "unchecked"}
		if err := db.First(&state, "node_key=?", key).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(500, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"success": true, "data": state})
		return
	}
	if node.Status != "online" || time.Since(node.LastHeartbeat) > 45*time.Second {
		c.JSON(409, gin.H{"success": false, "error": "Agent is offline; reconnect it before checking Python"})
		return
	}
	var req struct {
		Mode string `json:"mode"`
		Path string `json:"path"`
		Save bool   `json:"save"`
	}
	if c.ShouldBindJSON(&req) != nil || (req.Mode != "auto" && req.Mode != "manual") {
		c.JSON(400, gin.H{"success": false, "error": "Invalid Python mode"})
		return
	}
	req.Path = strings.TrimSpace(req.Path)
	// The target may use another OS. Accept POSIX or Windows absolute paths;
	// executable validation belongs to that target node.
	absolute := filepath.IsAbs(req.Path) || (len(req.Path) > 3 && req.Path[1] == ':' && (req.Path[2] == '\\' || req.Path[2] == '/'))
	if req.Mode == "manual" && (!absolute || len(req.Path) > 4096 || strings.ContainsAny(req.Path, "\x00\n\r")) {
		c.JSON(400, gin.H{"success": false, "error": "Specify an absolute Python interpreter path on this node"})
		return
	}
	if req.Mode == "auto" {
		req.Path = ""
	}
	id := uuid.NewString()
	err := db.Transaction(func(tx *gorm.DB) error {
		initial := database.AgentPythonConfig{NodeKey: key, Mode: "auto"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
			return err
		}
		return tx.Model(&database.AgentPythonConfig{}).Where("node_key=?", key).Updates(map[string]any{"request_id": id, "request_mode": req.Mode, "request_path": req.Path, "save": req.Save, "status": "pending", "detected_path": "", "detected_version": "", "error": "", "checked_at": nil}).Error
	})
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": gin.H{"requestId": id}})
}
