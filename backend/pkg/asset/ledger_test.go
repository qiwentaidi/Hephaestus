package asset

import (
	"context"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"
)

func stringPtr(value string) *string { return &value }
func boolPtr(value bool) *bool       { return &value }

func TestLedgerFieldsSurviveObservationIngestion(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	manual, _, err := store.Upsert(ctx, Observation{Type: TypeDomain, Value: "api.example.com", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if manual.Verification != "declared" {
		t.Fatalf("manual asset should be declared, got %q", manual.Verification)
	}
	tags := []string{"prod", "api"}
	updated, err := store.UpdateAsset(ctx, manual.ID, AssetUpdate{
		Name: stringPtr("Production API"), Notes: stringPtr("owner: platform"), Tags: &tags, GroupKey: stringPtr("prod"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Production API" || updated.GroupKey != "prod" || len(updated.Tags) != 2 {
		t.Fatalf("ledger fields were not saved: %#v", updated)
	}
	scanned, _, err := store.Upsert(ctx, Observation{Type: TypeDomain, Value: "api.example.com", Source: "portscan", TaskID: "task-1", TaskVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	if scanned.Verification != "observed" {
		t.Fatalf("scan observation should upgrade verification, got %q", scanned.Verification)
	}
	if scanned.Name != "Production API" || scanned.GroupKey != "prod" || len(scanned.Tags) != 2 {
		t.Fatalf("scan observation overwrote user fields: %#v", scanned)
	}
	archived, err := store.UpdateAsset(ctx, manual.ID, AssetUpdate{Archived: boolPtr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("archive timestamp was not set")
	}
	rescanned, _, err := store.Upsert(ctx, Observation{Type: TypeDomain, Value: "api.example.com", Source: "portscan", SourceID: "another", TaskID: "task-1", TaskVersion: 4})
	if err != nil {
		t.Fatal(err)
	}
	if rescanned.ArchivedAt == nil || rescanned.Name != "Production API" {
		t.Fatalf("scan re-observation cleared archive or name: %#v", rescanned)
	}
}

func TestLedgerListAndSummary(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.DB.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.Create(&database.VulnScanTask{ID: "task-1", Version: 2, TaskName: "External scan", ScanType: "portscan"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, input := range []Observation{
		{Type: TypeDomain, Value: "manual.example.com", Source: "manual", ObservedAt: now.Add(-3 * time.Hour)},
		{Type: TypeIP, Value: "192.0.2.10", Source: "import", Origin: "import", ImportBatchID: "batch-1", ObservedAt: now.Add(-2 * time.Hour)},
		{Type: TypeURL, Value: "https://api.example.com", Source: "fingerprint", TaskID: "task-1", TaskVersion: 2, ObservedAt: now.Add(-time.Hour)},
	} {
		if _, _, err := store.Upsert(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := store.ListLedger(ctx, LedgerFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("unexpected ledger size: total=%d items=%#v", total, items)
	}
	byValue := map[string]LedgerItem{}
	for _, item := range items {
		byValue[item.CanonicalValue] = item
	}
	manual := byValue["manual.example.com"]
	if manual.Verification != "declared" || len(manual.Sources) != 1 || manual.Sources[0] != "manual" {
		t.Fatalf("unexpected manual ledger item: %#v", manual)
	}
	imported := byValue["192.0.2.10"]
	if imported.Verification != "declared" || imported.Sources[0] != "import" {
		t.Fatalf("unexpected import ledger item: %#v", imported)
	}
	scanned := byValue["https://api.example.com"]
	if scanned.Verification != "observed" || scanned.TaskCount != 1 || scanned.LastTaskID != "task-1" || scanned.LastTaskName != "External scan" {
		t.Fatalf("unexpected scan ledger item: %#v", scanned)
	}
	sourceItems, sourceTotal, err := store.ListLedger(ctx, LedgerFilter{Source: "scan", Limit: 20})
	if err != nil || sourceTotal != 1 || len(sourceItems) != 1 || sourceItems[0].CanonicalValue != "https://api.example.com" {
		t.Fatalf("source filter failed: total=%d items=%#v err=%v", sourceTotal, sourceItems, err)
	}
	declared, declaredTotal, err := store.ListLedger(ctx, LedgerFilter{Verification: "declared", Limit: 20})
	if err != nil || declaredTotal != 2 || len(declared) != 2 {
		t.Fatalf("verification filter failed: total=%d items=%#v err=%v", declaredTotal, declared, err)
	}
	summary, err := store.LedgerSummary(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 3 || summary.Domains != 1 || summary.IPs != 1 || summary.URLs != 1 || summary.Declared != 2 || summary.Observed != 1 || summary.Manual != 1 || summary.Imported != 1 || summary.Scanned != 1 {
		t.Fatalf("unexpected ledger summary: %#v", summary)
	}
}

func TestLedgerArchiveHidesAssetByDefault(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	assetRow, _, err := store.Upsert(ctx, Observation{Type: TypeDomain, Value: "archive.example.com", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateAsset(ctx, assetRow.ID, AssetUpdate{Archived: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	items, total, err := store.ListLedger(ctx, LedgerFilter{Limit: 20})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("archived asset should be hidden: total=%d items=%#v err=%v", total, items, err)
	}
	items, total, err = store.ListLedger(ctx, LedgerFilter{IncludeArchived: true, Limit: 20})
	if err != nil || total != 1 || len(items) != 1 || !items[0].Archived {
		t.Fatalf("includeArchived should return the asset: total=%d items=%#v err=%v", total, items, err)
	}
	restored, err := store.UpdateAsset(ctx, assetRow.ID, AssetUpdate{Archived: boolPtr(false)})
	if err != nil || restored.ArchivedAt != nil {
		t.Fatalf("asset was not unarchived: %#v err=%v", restored, err)
	}
	if _, total, err = store.ListLedger(ctx, LedgerFilter{Limit: 20}); err != nil || total != 1 {
		t.Fatalf("restored asset should be visible by default: total=%d err=%v", total, err)
	}
}
