package asset

import (
	"context"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

func TestNextLifecycleRunUsesLocalThreeAM(t *testing.T) {
	now := time.Date(2026, 10, 20, 2, 0, 0, 0, time.Local)
	next := nextLifecycleRun(now)
	if next.Hour() != 3 || next.Day() != 20 {
		t.Fatalf("next run = %s, want 2026-10-20 03:00", next)
	}
	now = time.Date(2026, 10, 20, 4, 0, 0, 0, time.Local)
	next = nextLifecycleRun(now)
	if next.Hour() != 3 || next.Day() != 21 {
		t.Fatalf("next run = %s, want 2026-10-21 03:00", next)
	}
}

func TestRunLifecycleReconcileSkipsDeclaredAndArchived(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-48 * time.Hour)
	if _, _, err := store.Upsert(ctx, Observation{Type: TypeDomain, Value: "manual.example.com", Source: "manual", ObservedAt: old}); err != nil {
		t.Fatal(err)
	}
	observed, _, err := store.Upsert(ctx, Observation{Type: TypeDomain, Value: "observed.example.com", Source: "portscan", ObservedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateAsset(ctx, observed.ID, AssetUpdate{Archived: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	active, _, err := store.Upsert(ctx, Observation{Type: TypeIP, Value: "192.0.2.44", Source: "portscan", ObservedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	changed, ran, err := RunLifecycleReconcileExclusive(ctx, store.DB, time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("expected lifecycle reconcile to acquire the process lock")
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want only the non-archived observed asset", changed)
	}
	var manual, archived, current database.UnifiedAsset
	if err := store.DB.Where("canonical_value = ?", "manual.example.com").First(&manual).Error; err != nil {
		t.Fatal(err)
	}
	if manual.Status != "active" {
		t.Fatalf("declared asset status = %q, want active", manual.Status)
	}
	if err := store.DB.Where("canonical_value = ?", "observed.example.com").First(&archived).Error; err != nil {
		t.Fatal(err)
	}
	if archived.Status != "active" {
		t.Fatalf("archived asset status = %q, want unchanged active", archived.Status)
	}
	if err := store.DB.Where("id = ?", active.ID).First(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != "retired" {
		t.Fatalf("observed asset status = %q, want retired", current.Status)
	}
}

func TestReconcileLifecycleAfterRowQueryUsesCleanSession(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-4 * time.Hour)
	if _, _, err := store.Upsert(ctx, Observation{Type: TypeDomain, Value: "row-session.example.com", Source: "portscan", ObservedAt: old}); err != nil {
		t.Fatal(err)
	}
	err := store.DB.Connection(func(conn *gorm.DB) error {
		var value int
		if err := conn.Raw("SELECT 1").Row().Scan(&value); err != nil {
			return err
		}
		changed, err := (&Store{DB: conn.Session(&gorm.Session{})}).ReconcileLifecycle(ctx, time.Now().UTC(), time.Hour, 2*time.Hour)
		if err != nil {
			return err
		}
		if changed != 1 {
			t.Fatalf("changed = %d, want 1", changed)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
