package ai

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/lib/pentest/taskengine"
)

type webSearchUpdatePayload struct {
	Enabled      *bool   `json:"enabled"`
	Backend      string  `json:"backend"`
	Proxy        string  `json:"proxy"`
	BraveAPIKey  *string `json:"braveApiKey"`
	TavilyAPIKey *string `json:"tavilyApiKey"`
}

type webSearchTestPayload struct {
	Backend      string  `json:"backend"`
	Proxy        string  `json:"proxy"`
	BraveAPIKey  *string `json:"braveApiKey"`
	TavilyAPIKey *string `json:"tavilyApiKey"`
}

func GetAIWebSearchSettingsHandler(c *gin.Context) {
	runtime, err := taskengine.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": runtime.Engine.WebSearchSettings()})
}

func UpdateAIWebSearchSettingsHandler(c *gin.Context) {
	var payload webSearchUpdatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}
	if payload.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "enabled 不能为空"})
		return
	}
	runtime, err := taskengine.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := runtime.Engine.UpdateWebSearchSettings(*payload.Enabled, payload.Backend, payload.Proxy, payload.BraveAPIKey, payload.TavilyAPIKey); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": runtime.Engine.WebSearchSettings()})
}

func TestAIWebSearchHandler(c *gin.Context) {
	var payload webSearchTestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}
	runtime, err := taskengine.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	results, err := runtime.Engine.TestWebSearch(c.Request.Context(), payload.Backend, payload.Proxy, payload.BraveAPIKey, payload.TavilyAPIKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": results})
}
