package ai

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/airun"
)

// Browser clients use a query cursor so resuming also works through servers and
// proxies that have not yet allowed Last-Event-ID in their CORS configuration.
func pentestStreamResumeCursor(c *gin.Context) int64 {
	value := c.GetHeader("Last-Event-ID")
	if value == "" {
		value = c.Query("lastEventId")
	}
	cursor, err := strconv.ParseInt(value, 10, 64)
	if err != nil || cursor < 0 {
		return 0
	}
	return cursor
}

func streamDistributedPentestSession(c *gin.Context, store *pentest.SQLStore, id string) {
	cursor := pentestStreamResumeCursor(c)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	snapshotEvery := time.NewTicker(2 * time.Second)
	defer snapshotEvery.Stop()
	snapshot, err := buildPentestLiveSessionSnapshot(c.Request.Context(), store, id, "", "")
	if err != nil {
		writeSSE(c, "error", gin.H{"error": err.Error()})
		return
	}
	writeSSE(c, "session_snapshot", gin.H{"session": snapshot, "updatedAt": time.Now().Format(time.RFC3339)})
	flush := func() bool {
		for {
			events, err := store.ListWorkerEvents(c.Request.Context(), id, cursor, 500)
			if err != nil {
				writeSSE(c, "error", gin.H{"error": err.Error()})
				return false
			}
			for _, event := range events {
				var update pentestProcessUpdate
				if err = json.Unmarshal([]byte(event.Payload), &update); err != nil {
					writeSSE(c, "error", gin.H{"error": "invalid persisted progress event"})
					return false
				}
				_, _ = c.Writer.WriteString("id: " + strconv.FormatInt(event.Cursor, 10) + "\n")
				writeSSE(c, "process_update", gin.H{"process": update, "cursor": event.Cursor})
				cursor = event.Cursor
			}
			if len(events) < 500 {
				return true
			}
		}
	}
	for {
		if !flush() {
			return
		}
		task, _ := snapshot["task"].(gin.H)
		status, _ := task["status"].(string)
		if !airun.ConversationStatusIsActive(status) {
			writeSSE(c, "done", gin.H{"session": snapshot})
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		case <-snapshotEvery.C:
			snapshot, err = buildPentestLiveSessionSnapshot(c.Request.Context(), store, id, "", "")
			if err != nil {
				writeSSE(c, "error", gin.H{"error": err.Error()})
				return
			}
			writeSSE(c, "session_snapshot", gin.H{"session": snapshot, "updatedAt": time.Now().Format(time.RFC3339)})
		}
	}
}
