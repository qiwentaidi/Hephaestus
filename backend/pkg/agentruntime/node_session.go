package agentruntime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v3/host"
	"gorm.io/gorm"
	"hephaestus-backend/pkg/database"
)

// NodeSession owns a dedicated PostgreSQL connection. A machine-wide advisory
// lock excludes both embedded and standalone nodes, even across config paths
// and OS accounts. Connection death releases the lock; task generations still
// fence unfinished work until its existing lease expires.
type NodeSession struct {
	ID, HostIdentity string
	conn             *sql.Conn
	lockKey          int64
	locked           bool
	closeOnce        sync.Once
	closed           atomic.Bool
	platformToken    string
}

func NodeHostIdentity(ctx context.Context) (string, error) {
	id, err := host.HostIDWithContext(ctx)
	if err != nil || strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("cannot identify Agent host: %v", err)
	}
	digest := sha256.Sum256([]byte("hephaestus-agent:" + strings.TrimSpace(id)))
	return hex.EncodeToString(digest[:]), nil
}

func AcquireNodeSession(ctx context.Context, db *gorm.DB, hostID, requestedID, hostname, ip string, embedded bool) (*NodeSession, error) {
	if db.Dialector.Name() != "postgres" {
		return nil, fmt.Errorf("node sessions require PostgreSQL")
	}
	if strings.TrimSpace(hostID) == "" {
		return nil, fmt.Errorf("Agent host identity is empty")
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, err
	}
	session := &NodeSession{HostIdentity: hostID, conn: conn}
	digest := sha256.Sum256([]byte("hephaestus-agent-lock:" + hostID))
	session.lockKey = int64(binary.BigEndian.Uint64(digest[:8]))
	var acquired bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", session.lockKey).Scan(&acquired); err != nil {
		session.Close()
		return nil, err
	}
	if !acquired {
		session.Close()
		return nil, fmt.Errorf("this server already has a running Agent/embedded Worker")
	}
	session.locked = true
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var node database.ScanAgentNode
		err := tx.Where("host_identity = ?", hostID).First(&node).Error
		if err == nil {
			session.ID = node.AgentID
			return tx.Model(&node).Update("retired", false).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// Adopt the newest legacy embedded node so its name and enabled setting
		// survive the first upgrade. Host/IP matching does not merge other servers.
		if embedded {
			err = tx.Where("local_worker = ? AND last_seen_host = ? AND (last_seen_ip = ? OR last_seen_ip = '') AND host_identity = ''", true, hostname, ip).Order("last_heartbeat DESC, created_at DESC").First(&node).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil && node.Status == "online" && time.Since(node.LastHeartbeat) < 45*time.Second {
				return fmt.Errorf("legacy embedded Worker is still online; stop it before upgrading")
			}
		}
		if node.AgentID == "" {
			requestedID = strings.TrimSpace(requestedID)
			if requestedID == "" {
				requestedID = "node:" + uuid.NewString()
			}
			err = tx.Where("agent_id = ?", requestedID).First(&node).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil && node.HostIdentity != "" && node.HostIdentity != hostID {
				return fmt.Errorf("Agent ID %s belongs to another server", requestedID)
			}
			if err == nil && node.HostIdentity == "" && node.Status == "online" && time.Since(node.LastHeartbeat) < 45*time.Second {
				return fmt.Errorf("legacy Agent ID %s is still online; stop it before upgrading", requestedID)
			}
			if node.AgentID == "" {
				node = database.ScanAgentNode{AgentID: requestedID, Enabled: true, Status: "offline"}
				if err = tx.Create(&node).Error; err != nil {
					return err
				}
			}
		}
		session.ID = node.AgentID
		return tx.Model(&database.ScanAgentNode{}).Where("agent_id = ?", node.AgentID).Updates(map[string]any{"host_identity": hostID, "retired": false}).Error
	})
	if err != nil {
		session.Close()
		return nil, err
	}
	if err = session.registerPlatformCredential(ctx); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

func (session *NodeSession) Check(ctx context.Context) error {
	if session == nil {
		return nil
	}
	if session.closed.Load() {
		return fmt.Errorf("Agent node session is closed")
	}
	var n int
	return session.conn.QueryRowContext(ctx, "SELECT 1").Scan(&n)
}
func (session *NodeSession) Close() {
	if session == nil || session.conn == nil {
		return
	}
	session.closeOnce.Do(func() {
		session.closed.Store(true)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if session.locked {
			session.revokePlatformCredential(ctx)
			_, _ = session.conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", session.lockKey)
		}
		cancel()
		// Discard the pinned connection instead of returning a still-locked session
		// to the pool, including when the database vanished during shutdown.
		_ = session.conn.Raw(func(any) error { return driver.ErrBadConn })
		_ = session.conn.Close()
	})
}

// Retire only stale legacy cards. Rows and all historic execution references
// remain intact; disabled archived IDs cannot claim work or poll commands.
func RetireLegacyLocalNodes(db *gorm.DB, id, hostname, ip string) error {
	return db.Model(&database.ScanAgentNode{}).
		Where("agent_id <> ? AND local_worker = ? AND host_identity = '' AND last_seen_host = ? AND (last_seen_ip = ? OR last_seen_ip = '')", id, true, hostname, ip).
		Where("status = 'offline' OR last_heartbeat < ?", time.Now().Add(-45*time.Second)).
		Updates(map[string]any{"retired": true, "enabled": false, "status": "offline"}).Error
}
