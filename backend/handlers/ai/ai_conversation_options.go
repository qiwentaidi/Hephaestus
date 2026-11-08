package ai

import (
	"net/http"

	"github.com/gin-gonic/gin"
	aipkg "hephaestus-backend/pkg/ai"
)

func ListAIPentestProjectConversationOptionsHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	page := max(queryInt(c, "page", 1), 1)
	pageSize := min(max(queryInt(c, "pageSize", 20), 1), 100)
	offset := (page - 1) * pageSize
	options, total, err := store.ListConversationOptions(c.Request.Context(), c.Param("id"), pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"conversations": options, "total": total, "page": page, "pageSize": pageSize,
		"hasMore": offset+len(options) < total,
	}})
}
