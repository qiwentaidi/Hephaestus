package agentruntime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"gorm.io/gorm"
	"hephaestus-backend/pkg/database"
)

const NodeCredentialHeader = "X-Hephaestus-Agent-ID"

var ErrNodeCredential = errors.New("invalid Agent node credential")

func platformCredentialHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

// Register through the pinned connection so losing the node lock cannot leave
// another pool connection publishing a credential for an unowned session.
func (session *NodeSession) registerPlatformCredential(ctx context.Context) error {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return fmt.Errorf("generate Agent credential: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(secret[:])
	result, err := session.conn.ExecContext(ctx, `UPDATE scan_agent_node
SET platform_token_hash=$1, platform_session_pid=pg_backend_pid()
WHERE agent_id=$2 AND host_identity=$3`, platformCredentialHash(token), session.ID, session.HostIdentity)
	if err != nil {
		return fmt.Errorf("register Agent credential (start the updated primary first): %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return fmt.Errorf("Agent credential registration did not update its node")
	}
	session.platformToken = token
	return nil
}

func (session *NodeSession) revokePlatformCredential(ctx context.Context) {
	if session.platformToken == "" {
		return
	}
	// A late Close from an old process must never revoke its replacement.
	_, _ = session.conn.ExecContext(ctx, `UPDATE scan_agent_node
SET platform_token_hash='', platform_session_pid=0
WHERE agent_id=$1 AND platform_token_hash=$2 AND platform_session_pid=pg_backend_pid()`, session.ID, platformCredentialHash(session.platformToken))
}

// AuthorizePlatformRequest never exposes the credential to configuration,
// model context, or logs. Only a live owner may put it on a platform request.
func (session *NodeSession) AuthorizePlatformRequest(ctx context.Context, nodeID string, request *http.Request) error {
	if session == nil || session.ID != nodeID || session.platformToken == "" || request == nil {
		return ErrNodeCredential
	}
	if err := session.Check(ctx); err != nil {
		return fmt.Errorf("Agent node session unavailable: %w", err)
	}
	request.Header.Set(NodeCredentialHeader, session.ID)
	request.Header.Set("Authorization", "Bearer "+session.platformToken)
	return nil
}

// VerifyNodeCredential also checks the exact PostgreSQL advisory lock held by
// the registered process. Stale heartbeats cannot keep a lost process valid.
func VerifyNodeCredential(ctx context.Context, db *gorm.DB, nodeID, token string) error {
	return verifyNodeCredential(ctx, db, nodeID, token, "ai_worker=true")
}

// VerifyScanNodeCredential authorizes scan asset uploads only for scanning nodes.
func VerifyScanNodeCredential(ctx context.Context, db *gorm.DB, nodeID, token string) error {
	return verifyNodeCredential(ctx, db, nodeID, token, "scan_worker=true")
}

// VerifyResourceNodeCredential authorizes resource downloads for either worker role.
func VerifyResourceNodeCredential(ctx context.Context, db *gorm.DB, nodeID, token string) error {
	return verifyNodeCredential(ctx, db, nodeID, token, "(ai_worker=true OR scan_worker=true)")
}

func verifyNodeCredential(ctx context.Context, db *gorm.DB, nodeID, token, rolePredicate string) error {
	if nodeID == "" || len(nodeID) > 100 || len(token) != 43 || strings.TrimSpace(nodeID) != nodeID {
		return ErrNodeCredential
	}
	if db == nil || db.Dialector.Name() != "postgres" {
		return ErrNodeCredential
	}
	var node database.ScanAgentNode
	err := db.WithContext(ctx).Select("host_identity", "platform_token_hash", "platform_session_pid").
		Where("agent_id=? AND enabled=true AND retired=false AND status='online' AND last_heartbeat>CURRENT_TIMESTAMP-INTERVAL '30 seconds'", nodeID).
		Where(rolePredicate).Take(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNodeCredential
	}
	if err != nil {
		return err
	}
	if node.HostIdentity == "" || node.PlatformSessionPID == 0 || subtle.ConstantTimeCompare([]byte(node.PlatformTokenHash), []byte(platformCredentialHash(token))) != 1 {
		return ErrNodeCredential
	}
	digest := sha256.Sum256([]byte("hephaestus-agent-lock:" + node.HostIdentity))
	key := binary.BigEndian.Uint64(digest[:8])
	var ownsLock bool
	err = db.WithContext(ctx).Raw(`SELECT EXISTS (
SELECT 1 FROM pg_locks WHERE locktype='advisory' AND pid=? AND granted
AND classid=?::oid AND objid=?::oid AND objsubid=1
AND database=(SELECT oid FROM pg_database WHERE datname=current_database()))`, node.PlatformSessionPID, uint32(key>>32), uint32(key)).Scan(&ownsLock).Error
	if err != nil {
		return err
	}
	if !ownsLock {
		return ErrNodeCredential
	}
	return nil
}
