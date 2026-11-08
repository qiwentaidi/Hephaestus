package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"database/sql"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/nodepython"
)

func TestAgentPythonPostgresAPI(t *testing.T) {
	db := agentPostgresDB(t)
	if err := db.AutoMigrate(&database.AgentPythonConfig{}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/agents/:id/python", func(c *gin.Context) { serveAgentPython(c, db) })
	router.POST("/agents/:id/python", func(c *gin.Context) { serveAgentPython(c, db) })
	request := func(method string, body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "/agents/fixture-worker/python", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python required")
	}
	if w := request("POST", map[string]any{"mode": "manual", "path": "python3", "save": true}); w.Code != 400 {
		t.Fatalf("relative path status %d", w.Code)
	}
	if w := request("POST", map[string]any{"mode": "manual", "path": python, "save": true}); w.Code != 202 {
		t.Fatalf("enqueue: %d %s", w.Code, w.Body.String())
	}
	// API itself must never execute a remote interpreter or mark a probe ready.
	var pending database.AgentPythonConfig
	db.First(&pending, "node_key=?", "fixture-worker")
	if pending.Status != "pending" || pending.Path != "" {
		t.Fatalf("API ran probe: %+v", pending)
	}
	if err := nodepython.Process(context.Background(), db, "fixture-worker"); err != nil {
		t.Fatal(err)
	}
	w := request("GET", nil)
	var response struct {
		Data database.AgentPythonConfig `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || response.Data.Status != "ready" || response.Data.Path != python {
		t.Fatalf("result: %s", w.Body.String())
	}
	db.Model(&database.ScanAgentNode{}).Where("agent_id=?", "fixture-worker").Update("status", "offline")
	if w := request("POST", map[string]any{"mode": "auto", "save": true}); w.Code != 409 {
		t.Fatalf("offline: %d", w.Code)
	}
}

// Optional browser fixture uses the real API, a real node-side probe and an
// isolated PostgreSQL schema. It never edits live node settings.
func TestAgentPythonBrowserFixture(t *testing.T) {
	address := os.Getenv("HEPHAESTUS_PYTHON_BROWSER_ADDR")
	if address == "" {
		t.Skip("optional browser fixture")
	}
	db := agentPostgresDB(t)
	if err := db.AutoMigrate(&database.AgentPythonConfig{}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "http://localhost:3000")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
	router.OPTIONS("/*path", func(c *gin.Context) { c.Status(204) })
	for _, method := range []string{"GET", "POST"} {
		router.Handle(method, "/api/v1/webscan/agents/:id/python", func(c *gin.Context) {
			c.Params = gin.Params{{Key: "id", Value: "fixture-worker"}}
			serveAgentPython(c, db)
		})
	}
	finished := make(chan struct{}, 1)
	router.POST("/fixture/finish", func(c *gin.Context) {
		select {
		case finished <- struct{}{}:
		default:
		}
		c.Status(204)
	})
	server := &http.Server{Addr: address, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	t.Cleanup(func() { server.Close() })
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.Error(err)
		}
	}()
	t.Log("Python browser fixture ready on", address)
	end := time.After(4 * time.Minute)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-end:
			return
		case <-finished:
			return
		case <-ticker.C:
			db.Model(&database.ScanAgentNode{}).Where("agent_id=?", "fixture-worker").Updates(map[string]any{"status": "online", "last_heartbeat": time.Now()})
			if err := nodepython.Process(context.Background(), db, "fixture-worker"); err != nil {
				t.Error(err)
				return
			}
		}
	}
}

// agentPostgresDB 为 agent 包测试提供隔离的 PostgreSQL schema。
// 未设置 HEPHAESTUS_AGENT_TEST_DSN 时跳过，与其他 handlers 集成测试保持一致。
func agentPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("HEPHAESTUS_AGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL fixture")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "agent_pkg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
	})

	db, err := sql.Open("pgx", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return gdb
}
