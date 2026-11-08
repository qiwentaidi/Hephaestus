package agentupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hephaestus-backend/pkg/database"
)

func fixture(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dsn := os.Getenv("HEPHAESTUS_AGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL fixture")
	}
	admin, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "agent_update_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = admin.Exec("CREATE SCHEMA " + schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	dsn += " search_path=" + schema
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	t.Cleanup(func() { pool.Close() })
	if e = db.AutoMigrate(&database.ScanAgentNode{}, &database.AgentUpdate{}, &database.VulnScanTaskItem{}); e != nil {
		t.Fatal(e)
	}
	if e = db.Exec("CREATE TABLE pentest_worker_turns (id text PRIMARY KEY, node_id text, status text, lease_expires_at timestamptz)").Error; e != nil {
		t.Fatal(e)
	}
	node := database.ScanAgentNode{AgentID: "fixture", HostIdentity: "fixture-host", AgentVersion: "v9.0.0", Status: "online", LastHeartbeat: time.Now(), Enabled: true, UpdateProtocol: 1, UpdateOS: runtime.GOOS, UpdateArch: runtime.GOARCH}
	if e = db.Create(&node).Error; e != nil {
		t.Fatal(e)
	}
	return db, dsn
}

func wait(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out")
}
func readJob(t *testing.T, db *gorm.DB) database.AgentUpdate {
	t.Helper()
	var job database.AgentUpdate
	if e := db.First(&job, "agent_id='fixture'").Error; e != nil {
		t.Fatal(e)
	}
	return job
}

func TestUpdateBatchCancellationAndEnabledPreference(t *testing.T) {
	db, _ := fixture(t)
	if e := Request(db, []string{"fixture", "missing"}, "v9.1.0"); e == nil {
		t.Fatal("invalid batch accepted")
	}
	var count int64
	db.Model(&database.AgentUpdate{}).Count(&count)
	if count != 0 {
		t.Fatal("partial batch committed")
	}
	if e := Request(db, []string{"fixture"}, "v9.1.0"); e != nil {
		t.Fatal(e)
	}
	job := readJob(t, db)
	if e := Request(db, []string{"fixture"}, "v9.1.0"); e == nil {
		t.Fatal("duplicate accepted")
	}
	db.Model(&database.ScanAgentNode{}).Where("agent_id='fixture'").Update("enabled", false)
	if e := SetState(db, job, "waiting", 0); e != nil {
		t.Fatal(e)
	}
	if e := Cancel(db, job.AgentID, "wrong-request"); e == nil {
		t.Fatal("stale cancel accepted")
	}
	if e := Cancel(db, job.AgentID, job.RequestID); e != nil {
		t.Fatal(e)
	}
	if e := SetState(db, job, "restarting", 80); e != ErrCancelled {
		t.Fatalf("cancelled request restarted: %v", e)
	}
	if e := Finish(db, job, "cancelled", ""); e != nil {
		t.Fatal(e)
	}
	var node database.ScanAgentNode
	db.First(&node, "agent_id='fixture'")
	if node.Enabled || node.UpdatePending {
		t.Fatal("cancel overwrote enabled preference or retained drain")
	}
	db.Model(&node).Update("update_protocol", 0)
	if e := Request(db, []string{"fixture"}, "v9.1.0"); e == nil {
		t.Fatal("unsupported bootstrap accepted")
	}
}

// A real subprocess simulates a Worker, including heartbeat, delayed exit
// acknowledgement and a new-version startup failure. No executable replacement
// assertion relies on a mocked process cancellation result.
func TestUpdateChildHelper(t *testing.T) {
	v := os.Getenv("HEPHAESTUS_UPDATE_HELPER_VERSION")
	if v == "" {
		return
	}
	if os.Getenv("HEPHAESTUS_UPDATE_HELPER_BAD_VERSION") == v {
		os.Exit(12)
	}
	db, e := gorm.Open(postgres.Open(os.Getenv("HEPHAESTUS_UPDATE_HELPER_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		os.Exit(13)
	}
	id := os.Getenv("HEPHAESTUS_UPDATE_HELPER_NODE")
	stop := make(chan struct{})
	go func() { io.Copy(io.Discard, os.Stdin); close(stop) }()
	for {
		db.Model(&database.ScanAgentNode{}).Where("agent_id=?", id).Updates(map[string]any{"agent_version": v, "last_heartbeat": time.Now(), "status": "online", "platform_session_pid": os.Getpid()})
		if StopRequested(db, id) {
			time.Sleep(350 * time.Millisecond)
			os.Exit(0)
		}
		select {
		case <-stop:
			os.Exit(0)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func helperBinary(t *testing.T, dir, v string) string {
	t.Helper()
	name := "hephaestus-scan-agent"
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper fixture; native Windows acceptance required")
	}
	body := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\nversion) echo 'Hephaestus scan-agent %s (cli mode)'; exit 0;;\nhelp) echo '  supervise'; exit 0;;\nesac\nexport HEPHAESTUS_UPDATE_HELPER_VERSION='%s'\nexport HEPHAESTUS_UPDATE_HELPER_NODE='fixture'\nexec '%s' -test.run '^TestUpdateChildHelper$'\n", v, v, strings.ReplaceAll(os.Args[0], "'", "'\\''"))
	p := filepath.Join(dir, name)
	if e := os.WriteFile(p, []byte(body), 0755); e != nil {
		t.Fatal(e)
	}
	return p
}

func releaseServer(t *testing.T, binaryPath string, badChecksum bool) *httptest.Server {
	t.Helper()
	content, e := os.ReadFile(binaryPath)
	if e != nil {
		t.Fatal(e)
	}
	var buffer bytes.Buffer
	z := zip.NewWriter(&buffer)
	header := &zip.FileHeader{Name: "hephaestus-scan-agent", Method: zip.Deflate}
	header.SetMode(0755)
	f, _ := z.CreateHeader(header)
	f.Write(content)
	z.Close()
	data := buffer.Bytes()
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if badChecksum {
		hash = strings.Repeat("0", 64)
	}
	name := "hephaestus-agent-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "SHA256SUMS.txt") {
			fmt.Fprintf(w, "%s  %s\n", hash, name)
		} else {
			w.Write(data)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestSupervisedUpdateWaitsForProcessThenConfirmsOrRollsBack(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint("new-start-fails=", fail), func(t *testing.T) {
			db, dsn := fixture(t)
			t.Setenv("HEPHAESTUS_UPDATE_HELPER_DSN", dsn)
			if fail {
				t.Setenv("HEPHAESTUS_UPDATE_HELPER_BAD_VERSION", "v9.1.0")
			}
			dir := t.TempDir()
			installed := helperBinary(t, dir, "v9.0.0")
			old, _ := os.ReadFile(installed)
			os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("node-config"), 0600)
			newBinary := helperBinary(t, t.TempDir(), "v9.1.0")
			server := releaseServer(t, newBinary, false)
			s := &Supervisor{DB: db, Binary: installed, HostID: "fixture-host", ReleaseBase: server.URL, PollInterval: 20 * time.Millisecond, ConfirmTimeout: 10 * time.Second}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- s.Run(ctx) }()
			t.Cleanup(func() {
				cancel()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
					t.Error("supervisor did not exit")
				}
			})
			wait(t, func() bool {
				var n database.ScanAgentNode
				db.First(&n, "agent_id='fixture'")
				return n.PlatformSessionPID > 0
			})
			if e := Request(db, []string{"fixture"}, "v9.1.0"); e != nil {
				t.Fatal(e)
			}
			wait(t, func() bool { return readJob(t, db).Status == "restarting" })
			current, _ := os.ReadFile(installed)
			if !bytes.Equal(old, current) {
				t.Fatal("binary replaced before subprocess acknowledged exit")
			}
			if e := Cancel(db, "fixture", readJob(t, db).RequestID); e == nil {
				t.Fatal("restart was cancellable")
			}
			wait(t, func() bool { return !Active(readJob(t, db).Status) })
			job := readJob(t, db)
			expected := "completed"
			if fail {
				expected = "rolled_back"
			}
			if job.Status != expected {
				t.Fatalf("%+v", job)
			}
			var node database.ScanAgentNode
			db.First(&node, "agent_id='fixture'")
			if node.UpdatePending || node.Status != "online" {
				t.Fatalf("node %+v", node)
			}
			config, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
			if string(config) != "node-config" {
				t.Fatal("config overwritten")
			}
			if fail {
				current, _ = os.ReadFile(installed)
				if !bytes.Equal(old, current) {
					t.Fatal("previous executable not restored")
				}
			}
		})
	}
}

func TestStageRejectsChecksumAndVersionWithoutTouchingInstallation(t *testing.T) {
	for _, mode := range []string{"checksum", "version"} {
		t.Run(mode, func(t *testing.T) {
			v := "v9.1.0"
			if mode == "version" {
				v = "v9.2.0"
			}
			binaryPath := helperBinary(t, t.TempDir(), v)
			server := releaseServer(t, binaryPath, mode == "checksum")
			dir := t.TempDir()
			installed := helperBinary(t, dir, "v9.0.0")
			before, _ := os.ReadFile(installed)
			if _, e := Stage(context.Background(), installed, server.URL, "v9.1.0", runtime.GOOS, runtime.GOARCH, nil); e == nil {
				t.Fatal("invalid package accepted")
			}
			after, _ := os.ReadFile(installed)
			if !bytes.Equal(before, after) {
				t.Fatal("installed binary modified")
			}
		})
	}
}

func TestWaitingCancellationLeavesActiveWorkerRunning(t *testing.T) {
	db, dsn := fixture(t)
	t.Setenv("HEPHAESTUS_UPDATE_HELPER_DSN", dsn)
	db.Exec("INSERT INTO pentest_worker_turns VALUES ('busy','fixture','running',CURRENT_TIMESTAMP+INTERVAL '1 hour')")
	binaryPath := helperBinary(t, t.TempDir(), "v9.0.0")
	s := &Supervisor{DB: db, Binary: binaryPath, HostID: "fixture-host", ReleaseBase: "http://127.0.0.1:1", PollInterval: 20 * time.Millisecond, ConfirmTimeout: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	if e := Request(db, []string{"fixture"}, "v9.1.0"); e != nil {
		t.Fatal(e)
	}
	wait(t, func() bool { return readJob(t, db).Status == "waiting" })
	var before database.ScanAgentNode
	db.First(&before, "agent_id='fixture'")
	job := readJob(t, db)
	if e := Cancel(db, "fixture", job.RequestID); e != nil {
		t.Fatal(e)
	}
	wait(t, func() bool { return readJob(t, db).Status == "cancelled" })
	var after database.ScanAgentNode
	db.First(&after, "agent_id='fixture'")
	if after.UpdatePending || before.PlatformSessionPID != after.PlatformSessionPID {
		t.Fatal("cancellation restarted or left worker drained")
	}
	var status string
	db.Raw("SELECT status FROM pentest_worker_turns WHERE id='busy'").Scan(&status)
	if status != "running" {
		t.Fatal("active task cancelled")
	}
}

func TestSupervisorRecoversInstalledVersionAfterInterruption(t *testing.T) {
	db, dsn := fixture(t)
	t.Setenv("HEPHAESTUS_UPDATE_HELPER_DSN", dsn)
	if err := Request(db, []string{"fixture"}, "v9.1.0"); err != nil {
		t.Fatal(err)
	}
	job := readJob(t, db)
	for _, state := range []string{"waiting", "downloading", "verifying", "restarting", "confirming"} {
		if err := SetState(db, job, state, 90); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	binaryPath := helperBinary(t, dir, "v9.1.0")
	os.WriteFile(filepath.Join(dir, ".platform-update-request"), []byte(job.RequestID), 0600)
	os.Mkdir(filepath.Join(dir, ".install-lock"), 0700)
	s := &Supervisor{DB: db, Binary: binaryPath, HostID: "fixture-host", PollInterval: 20 * time.Millisecond, ConfirmTimeout: 10 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	wait(t, func() bool { return readJob(t, db).Status == "completed" })
	wait(t, func() bool { _, e := os.Stat(filepath.Join(dir, ".install-lock")); return os.IsNotExist(e) })
}

func TestBatchSupervisorsDoNotDrainSecondNodeUntilFirstFinishes(t *testing.T) {
	db, dsn := fixture(t)
	t.Setenv("HEPHAESTUS_UPDATE_HELPER_DSN", dsn)
	var n database.ScanAgentNode
	db.First(&n, "agent_id='fixture'")
	n.AgentID = "fixture2"
	n.HostIdentity = "fixture-host2"
	if e := db.Create(&n).Error; e != nil {
		t.Fatal(e)
	}
	db.Exec("INSERT INTO pentest_worker_turns VALUES('busy','fixture','running',CURRENT_TIMESTAMP+INTERVAL '1 hour')")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 2)
	for _, id := range []string{"fixture", "fixture2"} {
		binaryPath := helperBinary(t, t.TempDir(), "v9.0.0")
		content, _ := os.ReadFile(binaryPath)
		content = bytes.ReplaceAll(content, []byte("NODE='fixture'"), []byte("NODE='"+id+"'"))
		os.WriteFile(binaryPath, content, 0755)
		host := "fixture-host"
		if id == "fixture2" {
			host += "2"
		}
		s := &Supervisor{DB: db, Binary: binaryPath, HostID: host, ReleaseBase: "http://127.0.0.1:1", PollInterval: 20 * time.Millisecond, ConfirmTimeout: time.Second}
		go func() { done <- s.Run(ctx) }()
	}
	t.Cleanup(func() { cancel(); <-done; <-done })
	if e := Request(db, []string{"fixture2", "fixture"}, "v9.1.0"); e != nil {
		t.Fatal(e)
	}
	wait(t, func() bool { return readJob(t, db).Status == "waiting" })
	var second database.AgentUpdate
	db.First(&second, "agent_id='fixture2'")
	db.First(&n, "agent_id='fixture2'")
	if second.Status != "queued" || n.UpdatePending {
		t.Fatalf("second node drained early: %+v %+v", second, n)
	}
	job := readJob(t, db)
	if e := Cancel(db, "fixture", job.RequestID); e != nil {
		t.Fatal(e)
	}
	wait(t, func() bool { db.First(&second, "agent_id='fixture2'"); return second.Status == "failed" })
}
