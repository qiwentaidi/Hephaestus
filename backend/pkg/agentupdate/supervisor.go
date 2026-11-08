package agentupdate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gorm.io/gorm"
	"hephaestus-backend/pkg/database"
)

const batchLock int64 = 0x4855504454

type child struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	stop *os.File
}

func startChild(binaryPath string) (*child, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), "HEPHAESTUS_AGENT_SUPERVISED=1")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = r, os.Stdout, os.Stderr
	p := &child{cmd: cmd, done: make(chan struct{}), stop: w}
	err = cmd.Start()
	r.Close()
	if err != nil {
		w.Close()
		return nil, err
	}
	go func() { p.err = cmd.Wait(); w.Close(); close(p.done) }()
	return p, nil
}
func (p *child) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}
func (p *child) shutdown() { p.stop.Close(); <-p.done }

// Supervisor executes from an independent copy, so Windows can replace the
// worker executable. Only the child owns task leases and the node session.
type Supervisor struct {
	DB                           *gorm.DB
	Binary, HostID, ReleaseBase  string
	PollInterval, ConfirmTimeout time.Duration
	worker                       *child
	ResolveTarget                func(context.Context) (string, error)
}

func (s *Supervisor) Run(ctx context.Context) error {
	if s.PollInterval <= 0 {
		s.PollInterval = 2 * time.Second
	}
	if s.ConfirmTimeout <= 0 {
		s.ConfirmTimeout = 120 * time.Second
	}
	path, err := filepath.Abs(s.Binary)
	if err != nil {
		return err
	}
	s.Binary = path
	self, _ := os.Executable()
	self, _ = filepath.Abs(self)
	if self == path {
		return fmt.Errorf("launch supervise from an independent executable copy")
	}
	if s.DB.Dialector.Name() != "postgres" {
		return fmt.Errorf("managed updates require PostgreSQL")
	}
	pool, err := s.DB.DB()
	if err != nil {
		return err
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Raw(func(any) error { return driver.ErrBadConn }); conn.Close() }()
	hash := sha256.Sum256([]byte("hephaestus-update-supervisor:" + s.HostID))
	key := int64(binary.BigEndian.Uint64(hash[:8]))
	var locked bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("an update supervisor is already running on this host")
	}
	defer conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", key)
	// A restarted child must not immediately drain again while recovering a
	// crash between exit acknowledgement and the file replacement.
	var existingNode database.ScanAgentNode
	if e := s.DB.First(&existingNode, "host_identity=? AND local_worker=false", s.HostID).Error; e == nil {
		var interrupted database.AgentUpdate
		if e = s.DB.First(&interrupted, "agent_id=? AND status='restarting'", existingNode.AgentID).Error; e == nil {
			if _, e = os.Stat(s.Binary); os.IsNotExist(e) {
				_ = Restore(s.Binary)
			}
			if e = SetState(s.DB, interrupted, "confirming", 90); e != nil {
				return e
			}
		}
	}
	s.worker, err = startChild(s.Binary)
	if err != nil {
		return err
	}
	defer func() {
		if s.worker != nil && !s.worker.exited() {
			s.worker.shutdown()
		}
	}()
	ticker := time.NewTicker(s.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.worker.done:
			var node database.ScanAgentNode
			if s.DB.First(&node, "host_identity=?", s.HostID).Error == nil {
				var job database.AgentUpdate
				if s.DB.First(&job, "agent_id=?", node.AgentID).Error == nil && Active(job.Status) {
					_ = Finish(s.DB, job, "failed", "worker exited unexpectedly; retry after reconnecting")
				}
			}
			return s.worker.err
		case <-ticker.C:
			var node database.ScanAgentNode
			if err = s.DB.First(&node, "host_identity=? AND retired=false AND local_worker=false", s.HostID).Error; err != nil {
				continue
			}
			// Reconnect/resume this node's durable request after supervisor failure.
			var job database.AgentUpdate
			if err = s.DB.First(&job, "agent_id=?", node.AgentID).Error; err != nil || !Active(job.Status) {
				continue
			}
			if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", batchLock).Scan(&locked); err != nil {
				return err
			}
			if !locked {
				continue
			}
			var first database.AgentUpdate
			err = s.DB.Where("status IN ?", []string{"queued", "waiting", "downloading", "verifying", "restarting", "confirming", "rolling_back"}).Order("requested_at, agent_id").First(&first).Error
			if err == nil && first.RequestID == job.RequestID {
				if job.Status == "queued" {
					err = s.perform(ctx, conn, job)
				} else {
					err = s.recover(ctx, job)
				}
				if err != nil {
					log.Printf("[agent-update] %s: %v", job.AgentID, err)
				}
			}
			_, unlockErr := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", batchLock)
			if unlockErr != nil {
				return unlockErr
			}
			if s.worker == nil {
				return fmt.Errorf("worker could not restart: %w", err)
			}
		}
	}
}

func (s *Supervisor) confirm(ctx context.Context, job database.AgentUpdate, version string, started time.Time) error {
	deadline := time.NewTimer(s.ConfirmTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(s.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.worker.done:
			return fmt.Errorf("new worker exited: %v", s.worker.err)
		case <-deadline.C:
			return fmt.Errorf("new version heartbeat was not confirmed")
		case <-ticker.C:
			var node database.ScanAgentNode
			if err := s.DB.First(&node, "agent_id=?", job.AgentID).Error; err != nil {
				continue
			}
			if node.Status == "online" && node.AgentVersion == version && node.PlatformSessionPID == s.worker.cmd.Process.Pid && node.LastHeartbeat.After(started.Add(5*time.Second)) {
				return nil
			}
		}
	}
}

func lockInstallation(binaryPath string) (func(), error) {
	path := filepath.Join(filepath.Dir(binaryPath), ".install-lock")
	if runtime.GOOS == "windows" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		return func() { f.Close() }, nil
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, err
	}
	return func() { os.Remove(path) }, nil
}

func (s *Supervisor) perform(ctx context.Context, conn *sql.Conn, job database.AgentUpdate) (result error) {
	unlock, err := lockInstallation(s.Binary)
	if err != nil {
		return Finish(s.DB, job, "failed", "another installer is running: "+err.Error())
	}
	defer unlock()
	journal := filepath.Join(filepath.Dir(s.Binary), ".platform-update-request")
	if err = os.WriteFile(journal, []byte(job.RequestID), 0600); err != nil {
		return Finish(s.DB, job, "failed", err.Error())
	}
	defer func() {
		var current database.AgentUpdate
		if s.DB.First(&current, "agent_id=? AND request_id=?", job.AgentID, job.RequestID).Error == nil && !Active(current.Status) {
			os.Remove(journal)
		}
	}()
	if err = s.checkTarget(ctx, job); err != nil {
		return err
	}
	defer func() {
		if result != nil {
			status := "failed"
			if errors.Is(result, ErrCancelled) {
				status = "cancelled"
			}
			if err := Finish(s.DB, job, status, result.Error()); err != nil {
				log.Printf("[agent-update] persist failure: %v", err)
			}
		}
	}()
	if err = SetState(s.DB, job, "waiting", 0); err != nil {
		return err
	}
	for {
		if err = Check(s.DB, job); err != nil {
			return err
		}
		if s.worker.exited() {
			return fmt.Errorf("worker exited before acknowledging update")
		}
		running, e := Running(s.DB, job.AgentID)
		if e != nil {
			return e
		}
		if running == 0 {
			break
		}
		if err = SetState(s.DB, job, "waiting", 0); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.PollInterval):
		}
	}
	if err = SetState(s.DB, job, "downloading", 10); err != nil {
		return err
	}
	// Cancellation interrupts network I/O too. It never signals active tasks.
	downloadCtx, cancel := context.WithCancel(ctx)
	downloadDone := make(chan struct{})
	go func() {
		defer close(downloadDone)
		tick := time.NewTicker(s.PollInterval)
		defer tick.Stop()
		for {
			select {
			case <-downloadCtx.Done():
				return
			case <-tick.C:
				if Check(s.DB, job) != nil {
					cancel()
					return
				}
			}
		}
	}()
	staged, err := Stage(downloadCtx, s.Binary, s.ReleaseBase, job.TargetVersion, runtime.GOOS, runtime.GOARCH, func(mib int) {
		s.DB.Model(&database.AgentUpdate{}).Where("agent_id=? AND request_id=? AND status='downloading'", job.AgentID, job.RequestID).Updates(map[string]any{"downloaded_bytes": int64(mib) << 20, "updated_at": time.Now()})
	})
	cancel()
	<-downloadDone
	if err != nil {
		if Check(s.DB, job) == ErrCancelled {
			return ErrCancelled
		}
		return err
	}
	defer os.RemoveAll(filepath.Dir(staged))
	if err = SetState(s.DB, job, "verifying", 70); err != nil {
		return err
	}
	if err = SetState(s.DB, job, "restarting", 80); err != nil {
		return err
	}
	// The child drains actual scan and AI goroutines, releases its host lock,
	// then exits. Wait without a forced timeout/kill before replacing any file.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.worker.done:
	}
	if s.worker.err != nil {
		return fmt.Errorf("worker shutdown failed: %v", s.worker.err)
	}
	var live int
	if err = conn.QueryRowContext(ctx, "SELECT 1").Scan(&live); err != nil {
		return err
	}
	if err = s.checkTarget(ctx, job); err != nil {
		return s.restartPrevious(ctx, job, err, false)
	}
	if err = Replace(s.Binary, staged); err != nil {
		return s.restartPrevious(ctx, job, err, false)
	}
	if err = SetState(s.DB, job, "confirming", 90); err != nil {
		return s.restartPrevious(ctx, job, err, true)
	}
	started := time.Now()
	s.worker, err = startChild(s.Binary)
	if err == nil {
		err = s.confirm(ctx, job, job.TargetVersion, started)
	}
	if err != nil {
		return s.restartPrevious(ctx, job, err, true)
	}
	if err = Finish(s.DB, job, "completed", ""); err != nil {
		return err
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(s.Binary), ".installed-version"), []byte(job.TargetVersion+"\n"), 0600)
	return nil
}

func (s *Supervisor) checkTarget(ctx context.Context, job database.AgentUpdate) error {
	if s.ResolveTarget == nil {
		return nil
	}
	v, err := s.ResolveTarget(ctx)
	if err != nil {
		return err
	}
	if v != job.TargetVersion {
		return fmt.Errorf("server now requires %s; refresh the update request", v)
	}
	return nil
}

func (s *Supervisor) restartPrevious(ctx context.Context, job database.AgentUpdate, cause error, restore bool) error {
	if s.worker != nil && !s.worker.exited() {
		s.worker.shutdown()
	}
	if restore {
		if err := Restore(s.Binary); err != nil {
			return fmt.Errorf("%v; rollback failed: %w", cause, err)
		}
	}
	if err := SetState(s.DB, job, "rolling_back", 90); err != nil {
		return fmt.Errorf("%v; cannot coordinate rollback: %w", cause, err)
	}
	started := time.Now()
	var err error
	s.worker, err = startChild(s.Binary)
	if err == nil {
		err = s.confirm(ctx, job, job.PreviousVersion, started)
	}
	if err != nil {
		return fmt.Errorf("%v; previous worker could not reconnect: %w", cause, err)
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(s.Binary), ".installed-version"), []byte(job.PreviousVersion+"\n"), 0600)
	return Finish(s.DB, job, "rolled_back", cause.Error())
}

func (s *Supervisor) recover(ctx context.Context, job database.AgentUpdate) error {
	journal := filepath.Join(filepath.Dir(s.Binary), ".platform-update-request")
	data, err := os.ReadFile(journal)
	if err == nil && strings.TrimSpace(string(data)) == job.RequestID {
		defer os.Remove(journal)
		if runtime.GOOS != "windows" {
			defer os.Remove(filepath.Join(filepath.Dir(s.Binary), ".install-lock"))
		}
		if job.Status == "restarting" || job.Status == "confirming" {
			if VerifyBinary(ctx, s.Binary, job.TargetVersion) == nil {
				err = s.confirm(ctx, job, job.TargetVersion, time.Now().Add(-5*time.Second))
				if err == nil {
					return Finish(s.DB, job, "completed", "")
				}
				return s.restartPrevious(ctx, job, err, true)
			}
		}
	}
	return Finish(s.DB, job, "failed", "supervisor restarted before the update completed; retry the update")
}
