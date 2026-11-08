package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"hephaestus-backend/pkg/database"
)

func credentialNodeFixture(t *testing.T) (*gorm.DB, *NodeSession, string) {
	t.Helper()
	db, _, _ := postgresScanWorkerFixture(t)
	session, err := AcquireNodeSession(context.Background(), db, uuid.NewString(), "credential-node", "fixture", "192.0.2.1", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	if err := db.Model(&database.ScanAgentNode{}).Where("agent_id=?", session.ID).
		Updates(map[string]any{"ai_worker": true, "status": "online", "last_heartbeat": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPost, "http://fixture.invalid/tools", nil)
	if err := session.AuthorizePlatformRequest(context.Background(), session.ID, request); err != nil {
		t.Fatal(err)
	}
	return db, session, strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
}

func TestNodeCredentialPostgresRegistrationRotationAndRedaction(t *testing.T) {
	db, session, token := credentialNodeFixture(t)
	ctx := context.Background()
	if err := VerifyNodeCredential(ctx, db, session.ID, token); err != nil {
		t.Fatal(err)
	}
	var node database.ScanAgentNode
	if err := db.Where("agent_id=?", session.ID).First(&node).Error; err != nil {
		t.Fatal(err)
	}
	if node.PlatformTokenHash == token || node.PlatformTokenHash != platformCredentialHash(token) {
		t.Fatal("database did not store only the credential hash")
	}
	body, err := json.Marshal(node)
	if err != nil || strings.Contains(string(body), token) || strings.Contains(string(body), node.PlatformTokenHash) || strings.Contains(string(body), "platform_session") {
		t.Fatal("node API representation leaked credential metadata")
	}
	// Registration/heartbeat updates must not clear the session credential.
	if err = upsertNodeRecord(db, node); err != nil {
		t.Fatal(err)
	}
	if err = VerifyNodeCredential(ctx, db, session.ID, token); err != nil {
		t.Fatal(err)
	}
	if duplicate, err := AcquireNodeSession(ctx, db, session.HostIdentity, session.ID, "fixture", "192.0.2.1", false); err == nil {
		duplicate.Close()
		t.Fatal("duplicate session registered")
	}
	if err = VerifyNodeCredential(ctx, db, session.ID, token); err != nil {
		t.Fatal("failed duplicate registration invalidated the active credential")
	}
	session.Close()
	if err = VerifyNodeCredential(ctx, db, session.ID, token); !errors.Is(err, ErrNodeCredential) {
		t.Fatalf("closed credential accepted: %v", err)
	}
	restarted, err := AcquireNodeSession(ctx, db, session.HostIdentity, session.ID, "fixture", "192.0.2.1", false)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if restarted.platformToken == token {
		t.Fatal("credential was reused across restarts")
	}
	if err = VerifyNodeCredential(ctx, db, session.ID, token); !errors.Is(err, ErrNodeCredential) {
		t.Fatalf("previous process credential accepted: %v", err)
	}
	if err = VerifyNodeCredential(ctx, db, restarted.ID, restarted.platformToken); err != nil {
		t.Fatal(err)
	}
}

func TestNodeCredentialPostgresLossRevokesBeforeHeartbeatExpires(t *testing.T) {
	db, session, token := credentialNodeFixture(t)
	ctx := context.Background()
	var pid int
	if err := session.conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("SELECT pg_terminate_backend(?)", pid).Error; err != nil {
		t.Fatal(err)
	}
	if err := VerifyNodeCredential(ctx, db, session.ID, token); !errors.Is(err, ErrNodeCredential) {
		t.Fatalf("credential survived losing registration connection: %v", err)
	}
	replacement, err := AcquireNodeSession(ctx, db, session.HostIdentity, session.ID, "fixture", "192.0.2.1", false)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	session.Close()
	if err = VerifyNodeCredential(ctx, db, replacement.ID, replacement.platformToken); err != nil {
		t.Fatalf("late close revoked replacement credential: %v", err)
	}
	request, _ := http.NewRequest(http.MethodPost, "http://fixture.invalid/tools", nil)
	if err = session.AuthorizePlatformRequest(ctx, session.ID, request); err == nil || request.Header.Get("Authorization") != "" {
		t.Fatal("closed session attached a credential")
	}
}

func TestNodeCredentialPostgresRejectsInvalidAndUnavailableNodes(t *testing.T) {
	for _, tc := range []struct{ name, update string }{
		{"wrong token", ""},
		{"wrong node", ""},
		{"disabled", `UPDATE scan_agent_node SET enabled=false WHERE agent_id='credential-node'`},
		{"retired", `UPDATE scan_agent_node SET retired=true WHERE agent_id='credential-node'`},
		{"offline", `UPDATE scan_agent_node SET status='offline' WHERE agent_id='credential-node'`},
		{"stale", `UPDATE scan_agent_node SET last_heartbeat=CURRENT_TIMESTAMP-INTERVAL '1 minute' WHERE agent_id='credential-node'`},
		{"scan only", `UPDATE scan_agent_node SET ai_worker=false WHERE agent_id='credential-node'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, session, token := credentialNodeFixture(t)
			nodeID := session.ID
			if tc.name == "wrong token" {
				token = strings.Repeat("x", 43)
			} else if tc.name == "wrong node" {
				nodeID = "remote"
			} else if err := db.Exec(tc.update).Error; err != nil {
				t.Fatal(err)
			}
			if err := VerifyNodeCredential(context.Background(), db, nodeID, token); !errors.Is(err, ErrNodeCredential) {
				t.Fatalf("invalid credential accepted: %v", err)
			}
		})
	}
}

func TestScanNodeCredentialPostgresRoleAndSession(t *testing.T) {
	db, session, token := credentialNodeFixture(t)
	ctx := context.Background()
	if err := db.Model(&database.ScanAgentNode{}).Where("agent_id=?", session.ID).
		Updates(map[string]any{"ai_worker": false, "scan_worker": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := VerifyScanNodeCredential(ctx, db, session.ID, token); err != nil {
		t.Fatalf("active scan node could not upload assets: %v", err)
	}
	if err := VerifyResourceNodeCredential(ctx, db, session.ID, token); err != nil {
		t.Fatalf("active scan node could not sync resources: %v", err)
	}
	if err := VerifyNodeCredential(ctx, db, session.ID, token); !errors.Is(err, ErrNodeCredential) {
		t.Fatalf("scan-only node passed AI authorization: %v", err)
	}
	if err := db.Model(&database.ScanAgentNode{}).Where("agent_id=?", session.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := VerifyScanNodeCredential(ctx, db, session.ID, token); !errors.Is(err, ErrNodeCredential) {
		t.Fatalf("disabled scan node could upload: %v", err)
	}
}
