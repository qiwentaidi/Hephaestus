package ai

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"hephaestus-backend/lib/pentest"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/database"
)

const maxStoredSideChatMessageChars = 16000

type sideChatHistoryRequest struct {
	Messages []database.AIWorkbenchSideChatMessage `json:"messages"`
}

func getSideChatConversation(c *gin.Context, sessionID string) (pentest.Conversation, *pentest.SQLStore, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(sessionID) > 255 {
		return pentest.Conversation{}, nil, errors.New("invalid session id")
	}
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		return pentest.Conversation{}, nil, err
	}
	conversation, err := store.GetConversation(c.Request.Context(), sessionID)
	if err != nil || conversation.Mode != pentest.SoloPentestMode {
		return pentest.Conversation{}, nil, errors.New("conversation not found")
	}
	return conversation, store, nil
}

func GetAIPentestSideChatHistoryHandler(c *gin.Context) {
	_, _, err := getSideChatConversation(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "主会话不存在或不可用"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	var history database.AIWorkbenchSideChat
	err = db.WithContext(c.Request.Context()).First(&history, "conversation_id = ?", c.Param("id")).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []database.AIWorkbenchSideChatMessage{}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if history.Messages == nil {
		history.Messages = []database.AIWorkbenchSideChatMessage{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": history.Messages})
}

func SaveAIPentestSideChatHistoryHandler(c *gin.Context) {
	_, _, err := getSideChatConversation(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "主会话不存在或不可用"})
		return
	}
	var request sideChatHistoryRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Messages) > maxSideChatMessages {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "侧边聊天历史格式无效"})
		return
	}
	for _, message := range request.Messages {
		if (message.Role != "user" && message.Role != "assistant") || len([]rune(message.Content)) > maxStoredSideChatMessageChars {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "侧边聊天消息无效"})
			return
		}
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	history := database.AIWorkbenchSideChat{
		ConversationID: c.Param("id"),
		Messages:       request.Messages,
		UpdatedAt:      time.Now(),
	}
	if err := db.WithContext(c.Request.Context()).Save(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": history.Messages})
}

func DeleteAIPentestSideChatHistoryHandler(c *gin.Context) {
	_, _, err := getSideChatConversation(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "主会话不存在或不可用"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := db.WithContext(c.Request.Context()).Delete(&database.AIWorkbenchSideChat{}, "conversation_id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
}
