package ai

import (
	"errors"
	"hephaestus-backend/lib/pentest/taskengine/agent"
	"net/http"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine"
	aipkg "hephaestus-backend/pkg/ai"

	"github.com/gin-gonic/gin"
)

func ListAIAgentPromptsHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	items, err := store.ListAgentPrompts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	rt, nativeErr := taskengine.Get(c.Request.Context())
	for i := range items {
		if nativePromptKey(items[i].Role) != "" {
			if nativeErr != nil {
				items[i].Prompt = agent.BuiltinPromptSeeds()[nativePromptKey(items[i].Role)]
				items[i].DefaultPrompt = items[i].Prompt
				items[i].Customized = false
				items[i].Version = 0
				continue
			}
			items[i], err = loadARTEXPrompt(c.Request.Context(), rt, items[i].Role)
			if err != nil {
				nativeError(c, 500, err)
				return
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

func SaveAIAgentPromptHandler(c *gin.Context) {
	if nativePromptKey(c.Param("role")) != "" {
		saveARTEXPrompt(c)
		return
	}
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	saveAIAgentPrompt(c, store)
}

func saveAIAgentPrompt(c *gin.Context, store *pentest.SQLStore) {
	var req struct {
		Prompt  string `json:"prompt"`
		Version int    `json:"version"`
		Reset   bool   `json:"reset"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024)
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "invalid_prompt"})
		return
	}
	item, err := store.SaveAgentPrompt(c.Request.Context(), c.Param("role"), req.Prompt, req.Version, req.Reset)
	if errors.Is(err, pentest.ErrInvalidAgentPrompt) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "invalid_prompt"})
		return
	}
	if errors.Is(err, pentest.ErrAgentPromptConflict) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "code": "version_conflict"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": item})
}
