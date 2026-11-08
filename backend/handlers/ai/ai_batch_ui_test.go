package ai

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/pkg/aibatch"
)

// Opt-in UI smoke fixture. It uses the real persistent scheduler on a temporary
// database and deliberately never calls an LLM or a scan target.
func TestAIBatchUISmoke(t *testing.T) {
	if os.Getenv("AI_BATCH_UI_SMOKE") != "1" {
		t.Skip("opt-in browser fixture")
	}
	db := testfixture.SQLiteDB(t)
	m, err := aibatch.New(db, func(ctx context.Context, q aibatch.Queue, i aibatch.Item, save aibatch.Checkpoint) (string, error) {
		if strings.HasPrefix(i.Instruction, "fixture:fail") {
			return "isolated provider error", fmt.Errorf("fixture HTTP 403")
		}
		if i.Instruction == "fixture:ok" {
			return "isolated success", nil
		}
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return "UI fixture: retained partial evidence", ctx.Err()
		case <-timer.C:
			return "UI fixture: completed without network scanning", nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	aiBatchRuntime.Lock()
	old := aiBatchRuntime.manager
	aiBatchRuntime.manager = m
	aiBatchRuntime.Unlock()
	defer func() { aiBatchRuntime.Lock(); aiBatchRuntime.manager = old; aiBatchRuntime.Unlock() }()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "http://localhost:3000")
		c.Header("Access-Control-Allow-Headers", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
	g := r.Group("/api/v1/ai/batch-queues")
	g.GET("", listAIBatchQueues)
	g.GET("/:id", getAIBatchQueue)
	g.POST("/actions", actionAIBatchQueues)
	g.DELETE("/:id", deleteAIBatchQueue)
	g.POST("/:id/items/:itemId/retry", retryAIBatchItem)
	// Only creation substitutes project lookup, avoiding the application database.
	g.POST("", func(c *gin.Context) {
		var req struct {
			Name         string   `json:"name"`
			ProjectID    string   `json:"projectId"`
			Instructions []string `json:"instructions"`
			Concurrency  int      `json:"concurrency"`
			ExecuteNow   bool     `json:"executeNow"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			batchError(c, err)
			return
		}
		if req.ProjectID == "" {
			req.ProjectID = "ui-fixture-project"
		}
		q, err := m.Create(aibatch.Queue{Name: req.Name, ProjectID: req.ProjectID, Concurrency: req.Concurrency}, req.Instructions)
		if err != nil {
			batchError(c, err)
			return
		}
		if req.ExecuteNow {
			if err = m.Start(q.ID); err != nil {
				batchError(c, err)
				return
			}
		}
		c.JSON(200, gin.H{"success": true, "data": q})
	})
	t.Log("UI smoke fixture http://127.0.0.1:18086 (temporary database, no scanning)")
	server := &http.Server{Addr: "127.0.0.1:18086", Handler: r, ReadHeaderTimeout: 5 * time.Second}
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}
