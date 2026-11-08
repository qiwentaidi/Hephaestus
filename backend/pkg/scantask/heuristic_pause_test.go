package scantask

import (
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scanexec"
	"hephaestus-backend/pkg/taskstate"

	"github.com/qiwentaidi/gonmap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Real gonmap connections run under the real local task lifecycle. A stalled
// SOCKS handshake provides deterministic in-flight work without remote traffic.
func TestHeuristicPauseCancelsConnectionsBeforeAcknowledgementAndRestart(t *testing.T) {
	for _, subnet := range []bool{false, true} {
		name := "host"
		if subnet {
			name = "subnet"
		}
		t.Run(name, func(t *testing.T) { testDiscoveryPause(t, subnet) })
	}
}

func testDiscoveryPause(t *testing.T, subnet bool) {
	path := filepath.Join(t.TempDir(), "heuristic.db")
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000"), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, _ := db.DB()
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		return db
	}
	db := open()
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	task := database.VulnScanTask{ID: "heuristic-pause", Version: 1, ScanMode: "local", ScanType: "portscan", Status: taskstate.StatusRunning, Targets: `["remote.invalid"]`, Config: `{"tcpHeuristicEnabled":true,"aliveDetection":false}`}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	started, socketClosed := make(chan struct{}), make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		greeting := make([]byte, 3)
		if _, err := io.ReadFull(conn, greeting); err != nil {
			return
		}
		close(started)
		_, _ = io.Copy(io.Discard, conn)
		close(socketClosed)
	}()
	probeExited, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	old := scanexec.Handlers["portscan"]
	calls := 0
	scanexec.Handlers["portscan"] = func(ctx context.Context, id string, targets []string, _ map[string]interface{}) error {
		calls++
		stage := "portscan:alive"
		if subnet {
			stage = "portscan:subnets"
		}
		if err := taskstate.SaveCheckpoint(db, id, 1, stage, 0, nil); err != nil {
			return err
		}
		options := gonmap.HostDiscoveryOptions{Ports: []int{80}, Timeout: time.Minute, Concurrency: 1, ProxyURL: "socks5://" + listener.Addr().String()}
		var err error
		if subnet {
			_, err = gonmap.DiscoverSubnetsWithContext(ctx, []string{"192.0.2.0/23"}, &gonmap.SubnetDiscoveryOptions{TCP: options, HostSuffixes: []int{1}})
		} else {
			_, err = gonmap.DiscoverLiveHostsWithContext(ctx, targets, &options)
		}
		close(probeExited)
		<-release // Local parent must still wait for module cleanup acknowledgement.
		return err
	}
	exited := make(chan error, 1)
	finished := make(chan struct{})
	go func() { defer close(finished); exited <- RunLocalTaskWithDB(db, task.ID) }()
	t.Cleanup(func() {
		_ = control.GetTaskController().CancelTask(task.ID)
		unblock()
		<-finished
		scanexec.Handlers["portscan"] = old
	})
	wait := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("heuristic lifecycle did not acknowledge transition")
		}
	}
	read := func(db *gorm.DB) database.VulnScanTask {
		t.Helper()
		var result database.VulnScanTask
		if err := db.Where("id = ?", task.ID).First(&result).Error; err != nil {
			t.Fatal(err)
		}
		return result
	}
	wait(started)
	if err := db.Model(&database.VulnScanTask{}).Where("id = ?", task.ID).Update("pause_requested", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := control.GetTaskController().CancelTask(task.ID); err != nil {
		t.Fatal(err)
	}
	wait(probeExited)
	wait(socketClosed)
	if got := read(db); got.Status == taskstate.StatusPaused || !got.PauseRequested {
		t.Fatalf("parent paused before module exit: %+v", got)
	}
	unblock()
	select {
	case err := <-exited:
		if err != nil && !errors.Is(err, taskstate.ErrTaskPaused) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parent did not acknowledge module exit")
	}
	if got := read(db); got.Status != taskstate.StatusPaused || got.PauseRequested {
		t.Fatalf("pause was not finalized: %+v", got)
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	restarted := open()
	if err := ReconcilePausedTasksAfterRestart(restarted, nil); err != nil {
		t.Fatal(err)
	}
	if err := RunLocalTaskWithDB(restarted, task.ID); err != nil {
		t.Fatal(err)
	}
	if read(restarted).Status != taskstate.StatusPaused || calls != 1 {
		t.Fatal("restart reran a paused heuristic task")
	}
	// Persisted pause intent must also win when the process died before ack.
	if err := restarted.Model(&database.VulnScanTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{"status": taskstate.StatusRunning, "pause_requested": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReconcilePausedTasksAfterRestart(restarted, nil); err != nil {
		t.Fatal(err)
	}
	if err := RunLocalTaskWithDB(restarted, task.ID); err != nil {
		t.Fatal(err)
	}
	if read(restarted).Status != taskstate.StatusPaused || calls != 1 {
		t.Fatal("restart ignored persisted pause intent")
	}
}
