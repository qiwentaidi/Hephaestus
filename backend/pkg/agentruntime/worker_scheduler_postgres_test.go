package agentruntime

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scanexec"
)

func postgresScanWorkerFixture(t *testing.T) (*gorm.DB, database.ScanAgentNode, database.ScanAgentNode) {
	t.Helper()
	dsn := os.Getenv("HEPHAESTUS_AGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL fixture")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "scan_worker_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	conn, err := sql.Open("pgx", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&database.VulnScanTask{}, &database.VulnScanTaskItem{}, &database.ScanAgentNode{}); err != nil {
		t.Fatal(err)
	}
	local := database.ScanAgentNode{AgentID: "local", Enabled: true, ScanWorker: true, LocalWorker: true, ScanConcurrency: 1, Status: "online", LastHeartbeat: time.Now(), SupportedTypes: `["jsscan"]`, NetworkZones: `["private-a"]`}
	remote := local
	remote.AgentID = "remote"
	remote.LocalWorker = false
	if err = db.Create(&[]database.ScanAgentNode{local, remote}).Error; err != nil {
		t.Fatal(err)
	}
	return db, local, remote
}

func TestScanWorkerUpdatePendingBlocksClaimUnderNodeLock(t *testing.T) {
	db, _, remote := postgresScanWorkerFixture(t)
	pendingScanFixture(t, db, "agent", remote.AgentID, "{}")
	if err := db.Model(&database.ScanAgentNode{}).Where("agent_id=?", remote.AgentID).Update("update_pending", true).Error; err != nil {
		t.Fatal(err)
	}
	// Pass the deliberately stale pre-update node snapshot to exercise the
	// transaction's current-node check rather than only the early guard.
	if task, err := claimNextAgentTaskWithDB(db, remote, nil, 30*time.Second); err != nil || task != nil {
		t.Fatalf("draining node claimed task: %v %v", task, err)
	}
	db.Model(&database.ScanAgentNode{}).Where("agent_id=?", remote.AgentID).Update("update_pending", false)
	if task, err := claimNextAgentTaskWithDB(db, remote, nil, 30*time.Second); err != nil || task == nil {
		t.Fatalf("node did not resume: %v %v", task, err)
	}
}

func TestScanWorkerPostgresAIOnlyRegistrationPreservesFalseScanRole(t *testing.T) {
	db, _, _ := postgresScanWorkerFixture(t)
	node := database.ScanAgentNode{AgentID: "ai-only", Enabled: true, AIWorker: true, ScanWorker: false, AIConcurrency: 1, ScanConcurrency: 1, Status: "online", LastHeartbeat: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := upsertNodeRecord(db, node); err != nil {
		t.Fatal(err)
	}
	var saved database.ScanAgentNode
	if err := db.Where("agent_id=?", node.AgentID).First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if !saved.AIWorker || saved.ScanWorker {
		t.Fatalf("wrong roles on first registration: %+v", saved)
	}
	id := pendingScanFixture(t, db, "agent", node.AgentID, "{}")
	if payload, err := claimNextAgentTaskWithDB(db, saved, nil, 30*time.Second); err != nil || payload != nil {
		t.Fatalf("AI-only node claimed scan %s: %v %v", id, payload, err)
	}
}

func TestScanWorkerPostgresPauseCancelsExecutionBeforeParentStops(t *testing.T) {
	db, _, remote := postgresScanWorkerFixture(t)
	id := pendingScanFixture(t, db, "auto", "", "{}")
	payload, err := claimNextAgentTaskWithDB(db, remote, nil, 30*time.Second)
	if err != nil || payload == nil {
		t.Fatalf("claim %v %v", payload, err)
	}
	entered, requestExited := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
		close(requestExited)
	}))
	defer server.Close()
	defer server.CloseClientConnections()
	old := scanexec.Handlers["jsscan"]
	scanexec.Handlers["jsscan"] = func(ctx context.Context, _ string, _ []string, _ map[string]interface{}) error {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if e != nil {
			return e
		}
		response, e := server.Client().Do(req)
		if response != nil {
			response.Body.Close()
		}
		return e
	}
	t.Cleanup(func() { scanexec.Handlers["jsscan"] = old })
	exited := make(chan error, 1)
	go func() { exited <- executeAgentTaskWithDB(context.Background(), db, payload) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not start")
	}
	if err = db.Model(&database.VulnScanTask{}).Where("id=?", id).Updates(map[string]any{"pause_requested": true, "status": "pausing"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = AggregateAgentTaskProgress(db, id, 1); err != nil {
		t.Fatal(err)
	}
	var parent database.VulnScanTask
	db.Where("id=?", id).First(&parent)
	if parent.Status != "pausing" {
		t.Fatal("parent reported stopped before child acknowledgement")
	}
	select {
	case err = <-exited:
		if err == nil {
			t.Fatal("scan ignored cancellation")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("scan never acknowledged exit")
	}
	select {
	case <-requestExited:
	case <-time.After(3 * time.Second):
		t.Fatal("request context still running")
	}
	db.Where("id=?", id).First(&parent)
	if parent.Status != "paused" || !parent.PauseRequested {
		t.Fatalf("parent final state %+v", parent)
	}
	var item database.VulnScanTaskItem
	db.First(&item, payload.ItemID)
	if item.Status != "paused" {
		t.Fatalf("child state %+v", item)
	}
	if next, e := claimNextAgentTaskWithDB(db, remote, nil, 30*time.Second); e != nil || next != nil {
		t.Fatalf("restart claim ignored pause %v %v", next, e)
	}
}

func TestAcknowledgeInterruptedLocalScanStopsOnlyPausedItems(t *testing.T) {
	db, local, _ := postgresScanWorkerFixture(t)
	pausedID := pendingScanFixture(t, db, "local", "", "{}")
	runningID := pendingScanFixture(t, db, "local", "", "{}")
	if _, err := claimNextAgentTaskWithDB(db, local, nil, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	// The fixture has one local slot; make both rows look like interrupted
	// local work so the recovery filter, rather than the scheduler, is tested.
	var pausedItem database.VulnScanTaskItem
	if err := db.Where("task_id=?", pausedID).First(&pausedItem).Error; err != nil {
		t.Fatal(err)
	}
	var runningItem database.VulnScanTaskItem
	if err := db.Where("task_id=?", runningID).First(&runningItem).Error; err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := db.Model(&database.VulnScanTaskItem{}).Where("id IN ?", []uint{pausedItem.ID, runningItem.ID}).Updates(map[string]interface{}{
		"status": "running", "worker_agent_id": local.AgentID, "lease_expires_at": &future,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.VulnScanTask{}).Where("id=?", pausedID).Updates(map[string]interface{}{"status": "pausing", "pause_requested": true}).Error; err != nil {
		t.Fatal(err)
	}
	count, err := AcknowledgeInterruptedLocalScanStops(local.AgentID)
	if err != nil || count != 1 {
		t.Fatalf("acknowledged %d interrupted scans: %v", count, err)
	}
	if err := db.First(&pausedItem, pausedItem.ID).Error; err != nil {
		t.Fatal(err)
	}
	if pausedItem.Status != "paused" || pausedItem.LeaseExpiresAt != nil {
		t.Fatalf("paused item was not finalized: %+v", pausedItem)
	}
	if err := db.First(&runningItem, runningItem.ID).Error; err != nil {
		t.Fatal(err)
	}
	if runningItem.Status != "running" {
		t.Fatalf("ordinary running item was changed: %+v", runningItem)
	}
}

func pendingScanFixture(t *testing.T, db *gorm.DB, mode, assigned, conf string) string {
	t.Helper()
	task := database.VulnScanTask{ID: uuid.NewString(), Version: 1, ScanMode: mode, AssignedAgentID: assigned, ScanType: "jsscan", Status: "pending", Targets: `["fixture"]`, Config: conf}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureAgentTaskItems(db, &task); err != nil {
		t.Fatal(err)
	}
	return task.ID
}
func TestScanWorkerPostgresRemotePriorityCapacityAndLocalFallback(t *testing.T) {
	db, local, remote := postgresScanWorkerFixture(t)
	id := pendingScanFixture(t, db, "auto", "", "{}")
	if task, err := claimNextAgentTaskWithDB(db, local, nil, 30*time.Second); err != nil || task != nil {
		t.Fatalf("local preempted idle remote: %v %v", task, err)
	}
	task, err := claimNextAgentTaskWithDB(db, remote, nil, 30*time.Second)
	if err != nil || task == nil || task.ID != id {
		t.Fatalf("remote claim %v %v", task, err)
	}
	second := pendingScanFixture(t, db, "auto", "", "{}")
	if task, err := claimNextAgentTaskWithDB(db, remote, nil, 30*time.Second); err != nil || task != nil {
		t.Fatalf("remote over capacity %v %v", task, err)
	}
	task, err = claimNextAgentTaskWithDB(db, local, nil, 30*time.Second)
	if err != nil || task == nil || task.ID != second {
		t.Fatalf("local fallback %v %v", task, err)
	}
	pendingScanFixture(t, db, "auto", "", "{}")
	if task, err := claimNextAgentTaskWithDB(db, local, nil, 30*time.Second); err != nil || task != nil {
		t.Fatalf("all busy must queue %v %v", task, err)
	}
}
func TestScanWorkerPostgresExplicitLocationCapabilityAndNetwork(t *testing.T) {
	db, local, remote := postgresScanWorkerFixture(t)
	id := pendingScanFixture(t, db, "local", "", "{}")
	if task, err := claimNextAgentTaskWithDB(db, remote, nil, 30*time.Second); err != nil || task != nil {
		t.Fatalf("remote claimed explicit local %v %v", task, err)
	}
	task, err := claimNextAgentTaskWithDB(db, local, nil, 30*time.Second)
	if err != nil || task == nil || task.ID != id {
		t.Fatalf("explicit local %v %v", task, err)
	}
	if err = db.Model(&database.VulnScanTaskItem{}).Where("task_id=?", id).Update("status", "completed").Error; err != nil {
		t.Fatal(err)
	}
	privateID := pendingScanFixture(t, db, "auto", "", `{"networkZone":"private-b"}`)
	if task, err := claimNextAgentTaskWithDB(db, remote, nil, 30*time.Second); err != nil || task != nil {
		t.Fatalf("network mismatch %v %v", task, err)
	}
	remote.NetworkZones = `["private-b"]`
	if err = db.Model(&remote).Update("network_zones", remote.NetworkZones).Error; err != nil {
		t.Fatal(err)
	}
	if task, err := claimNextAgentTaskWithDB(db, remote, []string{"webscan"}, 30*time.Second); err != nil || task != nil {
		t.Fatalf("capability mismatch %v %v", task, err)
	}
	task, err = claimNextAgentTaskWithDB(db, remote, nil, 30*time.Second)
	if err != nil || task == nil || task.ID != privateID {
		t.Fatalf("qualified network claim %v %v", task, err)
	}
}

func TestScanWorkerPostgresExplicitLocalAssignmentBypassesRemotePriority(t *testing.T) {
	db, local, _ := postgresScanWorkerFixture(t)
	// The remote fixture is online, capable and idle. Auto tasks still prefer it,
	// but a task explicitly pinned to the local node must start on that node.
	id := pendingScanFixture(t, db, "agent", local.AgentID, "{}")
	task, err := claimNextAgentTaskWithDB(db, local, nil, 30*time.Second)
	if err != nil || task == nil || task.ID != id {
		t.Fatalf("pinned local claim %v %v", task, err)
	}
}
