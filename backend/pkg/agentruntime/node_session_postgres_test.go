package agentruntime

import (
	"context"
	"github.com/google/uuid"
	"hephaestus-backend/pkg/database"
	"strings"
	"testing"
	"time"
)

func TestNodeSessionPostgresStableIdentityAndLegacyRetirement(t *testing.T) {
	db, _, _ := postgresScanWorkerFixture(t)
	old := []database.ScanAgentNode{
		{AgentID: "legacy-old", LocalWorker: true, LastSeenHost: "fixture-host", LastSeenIP: "192.0.2.1", Status: "offline", Enabled: true, LastHeartbeat: time.Now().Add(-time.Hour)},
		{AgentID: "legacy-current", LocalWorker: true, LastSeenHost: "fixture-host", LastSeenIP: "192.0.2.1", Status: "offline", Enabled: false, DisplayName: "custom local name", LastHeartbeat: time.Now()},
		{AgentID: "other-machine", LocalWorker: true, LastSeenHost: "fixture-host", LastSeenIP: "192.0.2.2", Status: "offline", Enabled: true},
	}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	// GORM's default:true replaces a false bool during struct inserts.
	if err := db.Model(&database.ScanAgentNode{}).Where("agent_id = ?", "legacy-current").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	hostID := uuid.NewString()
	session, err := AcquireNodeSession(ctx, db, hostID, "new-random-id", "fixture-host", "192.0.2.1", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	if session.ID != "legacy-current" {
		t.Fatalf("failed to adopt latest identity: %s", session.ID)
	}
	if duplicate, err := AcquireNodeSession(ctx, db, hostID, "another-config", "fixture-host", "192.0.2.1", false); err == nil {
		duplicate.Close()
		t.Fatal("second Agent on the same server was accepted")
	}
	if err := RetireLegacyLocalNodes(db, session.ID, "fixture-host", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	var archived, other database.ScanAgentNode
	db.Where("agent_id = ?", "legacy-old").First(&archived)
	db.Where("agent_id = ?", "other-machine").First(&other)
	if !archived.Retired || archived.Enabled || other.Retired {
		t.Fatal("retirement removed or changed the wrong host")
	}
	var count int64
	db.Model(&database.ScanAgentNode{}).Where("agent_id = ?", "legacy-old").Count(&count)
	if count != 1 {
		t.Fatal("historical node record deleted")
	}
	session.Close()
	restarted, err := AcquireNodeSession(ctx, db, hostID, "changed-config-id", "fixture-host", "192.0.2.1", false)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if restarted.ID != session.ID {
		t.Fatal("restart registered another node")
	}
	var saved database.ScanAgentNode
	db.Where("agent_id = ?", restarted.ID).First(&saved)
	if saved.DisplayName != "custom local name" || saved.Enabled {
		t.Fatal("node settings were reset")
	}
	saved.AIConcurrency = 1
	saved.ScanConcurrency = 2
	if err := upsertNodeRecord(db, saved); err != nil {
		t.Fatal(err)
	}
	db.Where("agent_id = ?", saved.AgentID).First(&saved)
	if saved.Enabled || saved.AIConcurrency != 1 || saved.ScanConcurrency != 2 {
		t.Fatal("capacity update reset the enabled setting")
	}
}
func TestNodeSessionPostgresRejectsIdentityCopyAndReleasesLostLock(t *testing.T) {
	db, _, _ := postgresScanWorkerFixture(t)
	ctx := context.Background()
	hostID := uuid.NewString()
	session, err := AcquireNodeSession(ctx, db, hostID, "fixture-node", "host", "192.0.2.3", false)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if duplicate, err := AcquireNodeSession(ctx, db, uuid.NewString(), "fixture-node", "other-host", "192.0.2.4", false); err == nil {
		duplicate.Close()
		t.Fatal("copied node ID registered on another machine")
	}
	var pid int
	if err := session.conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("SELECT pg_terminate_backend(?)", pid).Error; err != nil {
		t.Fatal(err)
	}
	if err := session.Check(ctx); err == nil {
		t.Fatal("lost instance lock was not detected")
	}
	session.Close()
	replacement, err := AcquireNodeSession(ctx, db, hostID, "another-id", "host", "192.0.2.3", false)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	if replacement.ID != session.ID || strings.TrimSpace(replacement.ID) == "" {
		t.Fatal("failed to reuse identity after disconnect")
	}
}
