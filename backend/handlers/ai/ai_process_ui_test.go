package ai

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/airun"
)

// Opt-in visual fixture: all data is isolated, and no model or scan is executed.
func TestAIProcessUISmoke(t *testing.T) {
	if os.Getenv("AI_PROCESS_UI_SMOKE") != "1" {
		t.Skip("opt-in UI fixture")
	}
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	store := pentest.NewSQLStore(db)
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, _ := store.UpsertProject(ctx, pentest.Project{ID: "process-ui-project", Name: "过程展示验证"})
	conv, _ := store.UpsertConversation(ctx, pentest.Conversation{ID: "process-ui-fixture", ProjectID: project.ID, Title: "分析过程展示", Status: "running"})
	_, _ = store.AddMessage(ctx, pentest.Message{ConversationID: conv.ID, Role: "user", Content: "请汇总测试环境的已有证据，并说明下一步验证目的。"})
	record := recordPentestAgentProgress(store, conv.ID)
	record(pentest.AgentLoopEvent{RunID: "fixture-run", Type: "reasoning", BlockID: "earliest", Message: "早期分析：先核对已有证据，避免重复验证。", Status: "completed", Iteration: 1})
	for i := 1; i <= 24; i++ {
		record(pentest.AgentLoopEvent{RunID: "fixture-run", Type: "tool_done", ToolName: "fixture-read", ToolCallID: time.Now().String(), Message: "读取测试证据完成", Iteration: i})
	}
	body := "### 当前发现\n\n测试响应包含默认页面，现有证据不足以判断具体业务。\n\n- **已知**：响应已经成功记录。\n- **待验证**：不同入口是否返回相同内容。\n\n下一步将比较已有响应，避免把默认页面直接判定为漏洞。"
	record(pentest.AgentLoopEvent{RunID: "fixture-run", Type: "reasoning", BlockID: "live-reasoning", Message: body, Status: "running", Iteration: 25})
	record(pentest.AgentLoopEvent{RunID: "fixture-run", Type: "assistant_commentary", BlockID: "live-commentary", Message: "准备比较已有响应的状态码和页面特征，以确认这些入口是否属于同一服务。", Status: "running", Iteration: 25})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "http://localhost:3000")
		c.Header("Access-Control-Allow-Headers", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
	snapshot := func() gin.H {
		s, e := buildPentestSessionSnapshot(ctx, store, conv.ID, "fixture", "")
		if e != nil {
			t.Error(e)
		}
		return s
	}
	r.GET("/snapshot", func(c *gin.Context) { c.JSON(200, gin.H{"success": true, "data": snapshot()}) })
	r.GET("/stream", func(c *gin.Context) { streamPentestSession(c, store, conv.ID) })
	r.POST("/animate", func(c *gin.Context) {
		go func() {
			blockID := "smooth-" + time.Now().Format("150405.000000000")
			text := []rune("### 流式分析验证\n\n这里逐步输出已收到的测试证据，用于验证文本持续更新，不需要等待完整会话刷新。当前仅运行隔离测试，不调用任何模型或扫描目标。\n\n**检查完成**：分片、历史恢复和最终 Markdown 格式。")
			for i := 1; i <= len(text); i += 3 {
				record(pentest.AgentLoopEvent{RunID: "fixture-run", Type: "assistant_commentary", BlockID: blockID, Message: string(text[:i]), Status: "running", Sequence: 100 + i, Iteration: 26})
				time.Sleep(100 * time.Millisecond)
			}
			record(pentest.AgentLoopEvent{RunID: "fixture-run", Type: "assistant_commentary", BlockID: blockID, Message: string(text), Status: "completed", Sequence: 1000, Iteration: 26})
		}()
		c.JSON(200, gin.H{"success": true})
	})
	r.POST("/append", func(c *gin.Context) {
		record(pentest.AgentLoopEvent{RunID: "fixture-run", Type: "reasoning", BlockID: "live-reasoning", Message: body + "\n\n补充证据：两份响应的页面特征一致，后续按同一服务归组。", Status: "running", Iteration: 25})
		c.JSON(200, gin.H{"success": true})
	})
	r.POST("/pause", func(c *gin.Context) {
		record(pentest.AgentLoopEvent{RunID: "fixture-run", Type: "assistant_commentary", BlockID: "live-commentary", Message: "准备比较已有响应的状态码和页面特征，以确认这些入口是否属于同一服务。", Status: "cancelled", Iteration: 25})
		_ = airun.MarkConversationStatus(ctx, store, conv.ID, "cancelled")
		c.JSON(200, gin.H{"success": true})
	})
	t.Log("process UI fixture http://127.0.0.1:18087")
	server := &http.Server{Addr: "127.0.0.1:18087", Handler: r, ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		t.Fatal(err)
	}
}
