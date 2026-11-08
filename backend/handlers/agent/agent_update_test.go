package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"hephaestus-backend/pkg/agentrelease"
	"hephaestus-backend/pkg/agentupdate"
	"hephaestus-backend/pkg/database"
)

func updateAPIFixture(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	dsn := os.Getenv("HEPHAESTUS_AGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL fixture required")
	}
	admin, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "agent_update_api_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = admin.Exec("CREATE SCHEMA " + schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	db, e := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	t.Cleanup(func() { pool.Close() })
	if e = db.AutoMigrate(&database.ScanAgentNode{}, &database.AgentUpdate{}); e != nil {
		t.Fatal(e)
	}
	nodes := []database.ScanAgentNode{
		{AgentID: "linux", DisplayName: "Linux 测试节点", AgentVersion: "v0.4.5", Enabled: true, Status: "online", LastHeartbeat: time.Now(), UpdateProtocol: 1, UpdateOS: "linux", UpdateArch: "amd64"},
		{AgentID: "windows", DisplayName: "Windows 测试节点", AgentVersion: "v0.4.5", Enabled: true, Status: "online", LastHeartbeat: time.Now(), UpdateProtocol: 1, UpdateOS: "windows", UpdateArch: "amd64"},
		{AgentID: "legacy", DisplayName: "旧版节点", AgentVersion: "v0.4.5", Enabled: true, Status: "online", LastHeartbeat: time.Now()},
		{AgentID: "local", DisplayName: "内置 Worker", AgentVersion: agentrelease.Current().AgentVersion, Enabled: true, Status: "online", LastHeartbeat: time.Now(), LocalWorker: true},
	}
	if e = db.Create(&nodes).Error; e != nil {
		t.Fatal(e)
	}
	r := gin.New()
	handler := func(c *gin.Context) { serveAgentUpdates(c, db) }
	r.GET("/api/v1/webscan/agent-updates", handler)
	r.POST("/api/v1/webscan/agent-updates", handler)
	r.POST("/api/v1/webscan/agent-updates/:id/cancel", handler)
	return db, r
}

func TestAgentUpdateAPIValidatesWholeBatchAndStaleCancel(t *testing.T) {
	db, r := updateAPIFixture(t)
	request := func(path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	if w := request("/api/v1/webscan/agent-updates", `{"agentIds":["linux","legacy"]}`); w.Code != 409 {
		t.Fatal(w.Body.String())
	}
	var count int64
	db.Model(&database.AgentUpdate{}).Count(&count)
	if count != 0 {
		t.Fatal("partial batch")
	}
	if w := request("/api/v1/webscan/agent-updates", `{"agentIds":["linux","windows"]}`); w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	var job database.AgentUpdate
	db.First(&job, "agent_id='linux'")
	if w := request("/api/v1/webscan/agent-updates/linux/cancel", `{"requestId":"stale"}`); w.Code != 409 {
		t.Fatal("stale cancellation accepted")
	}
	if w := request("/api/v1/webscan/agent-updates/linux/cancel", `{"requestId":"`+job.RequestID+`"}`); w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/webscan/agent-updates", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "cancelled") {
		t.Fatal(w.Body.String())
	}
}

// Opt-in fixture runs real API handlers against a disposable schema, allowing
// browser verification without issuing updates to the user's live nodes.
func TestAgentUpdateBrowserFixture(t *testing.T) {
	if os.Getenv("HEPHAESTUS_AGENT_UPDATE_BROWSER") == "" {
		t.Skip("opt-in browser fixture")
	}
	db, r := updateAPIFixture(t)
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "http://localhost:3000")
		c.Header("Access-Control-Allow-Headers", "*")
		c.Header("Access-Control-Allow-Methods", "*")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
		}
	})
	// The pre-existing routes also need CORS for browser requests.
	r.OPTIONS("/*path", func(c *gin.Context) { c.Status(204) })
	r.GET("/api/v1/webscan/agents", func(c *gin.Context) {
		db.Model(&database.ScanAgentNode{}).Where("status='online'").Update("last_heartbeat", time.Now())
		var nodes []database.ScanAgentNode
		db.Find(&nodes)
		rows := []gin.H{}
		for _, n := range nodes {
			data, _ := json.Marshal(n)
			row := gin.H{}
			json.Unmarshal(data, &row)
			row["isAlive"] = true
			row["aiRunning"] = 0
			row["scanRunning"] = 0
			rows = append(rows, row)
		}
		c.JSON(200, gin.H{"success": true, "data": rows})
	})
	r.POST("/__fixture/wait", func(c *gin.Context) {
		var jobs []database.AgentUpdate
		db.Where("status='queued'").Order("requested_at").Find(&jobs)
		if len(jobs) > 0 {
			_ = agentupdate.SetState(db, jobs[0], "waiting", 0)
		}
		c.JSON(200, gin.H{"success": true})
	})
	r.POST("/__fixture/ack", func(c *gin.Context) {
		var jobs []database.AgentUpdate
		db.Where("cancel_requested=true").Find(&jobs)
		for _, job := range jobs {
			_ = agentupdate.Finish(db, job, "cancelled", "")
		}
		c.JSON(200, gin.H{"success": true})
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if req.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		r.ServeHTTP(w, req)
	}))
	defer server.Close()
	t.Log("BROWSER_FIXTURE=" + server.URL)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()
}
