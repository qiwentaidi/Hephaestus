package findings

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"hephaestus-backend/pkg/database"
	center "hephaestus-backend/pkg/vulnerabilitycenter"
)

func centerInt(c *gin.Context, key string, fallback int) int {
	v, err := strconv.Atoi(c.Query(key))
	if err != nil || v < 1 {
		return fallback
	}
	return min(v, 1000000)
}
func centerSource(c *gin.Context) bool {
	switch c.Query("source") {
	case "", "all", "web", "js", "ai":
		return true
	}
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid source"})
	return false
}
func centerValues(c *gin.Context, key string) []string {
	out := []string{}
	for _, raw := range c.QueryArray(key) {
		for _, v := range strings.Split(raw, ",") {
			if v = strings.TrimSpace(v); v != "" && v != "all" {
				out = append(out, v)
			}
		}
	}
	return out
}

func ListCenterHandler(c *gin.Context) {
	if !centerSource(c) {
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	result, err := center.Query(c.Request.Context(), db, center.Filter{Source: c.Query("source"), ProjectID: c.Query("projectId"), ConversationID: c.Query("conversationId"), TaskID: c.Query("taskId"), Version: centerInt(c, "version", 0), Search: c.Query("search"), Severities: centerValues(c, "severity"), Statuses: centerValues(c, "status"), NeedsReview: c.Query("needsReview") == "true", Page: centerInt(c, "page", 1), PageSize: centerInt(c, "pageSize", 20)})
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": result})
}
func CenterScopesHandler(c *gin.Context) {
	if !centerSource(c) {
		return
	}
	if c.Query("parentId") != "" && (c.Query("source") == "" || c.Query("source") == "all") {
		c.JSON(400, gin.H{"success": false, "error": "source required for child scopes"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	result, err := center.Scopes(c.Request.Context(), db, c.Query("source"), c.Query("parentId"), c.Query("search"), centerInt(c, "page", 1))
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": result})
}

// Resolve persistent Web finding IDs within the exact task version, retaining
// composite-identity links created before IDs were introduced.
func CenterWebDetailHandler(c *gin.Context) {
	version, err := strconv.Atoi(c.Query("version"))
	if err != nil || version < 1 || c.Query("taskId") == "" {
		c.JSON(400, gin.H{"success": false, "error": "taskId and version required"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	var record database.VulnerabilityInfo
	query := db.WithContext(c.Request.Context()).Where("task_id=? AND version=?", c.Query("taskId"), version)
	if id := strings.TrimSpace(c.Query("id")); id != "" {
		query = query.Where("id = ?", id)
	} else {
		// Retain links created before persistent Web finding IDs were introduced.
		query = query.Where("COALESCE(template_id,'')=? AND COALESCE(vuln_url,'')=? AND COALESCE(protocol,'')=? AND COALESCE(vuln_name,'')=?", c.Query("templateId"), c.Query("url"), c.Query("protocol"), c.Query("name"))
	}
	err = query.First(&record).Error
	if err != nil {
		status := 500
		if err == gorm.ErrRecordNotFound {
			status = 404
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": record})
}
