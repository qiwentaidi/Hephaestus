package database

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBackfillUnifiedAssetLedgerFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ledger.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&UnifiedAsset{}, &UnifiedAssetObservation{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	assets := []UnifiedAsset{
		{ID: "scan-asset", Type: "domain", CanonicalValue: "scan.example.com", DisplayValue: "scan.example.com", Status: "active", FirstSeenAt: now, LastSeenAt: now},
		{ID: "manual-asset", Type: "ip", CanonicalValue: "192.0.2.10", DisplayValue: "192.0.2.10", Status: "active", FirstSeenAt: now, LastSeenAt: now},
	}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	observations := []UnifiedAssetObservation{
		{ID: "scan-obs", AssetID: "scan-asset", Type: "domain", CanonicalValue: "scan.example.com", Source: "portscan", ObservationKey: "scan-key", ObservedAt: now},
		{ID: "manual-obs", AssetID: "manual-asset", Type: "ip", CanonicalValue: "192.0.2.10", Source: "manual", ObservationKey: "manual-key", ObservedAt: now},
	}
	if err := db.Create(&observations).Error; err != nil {
		t.Fatal(err)
	}
	if err := backfillUnifiedAssetLedgerFields(db); err != nil {
		t.Fatal(err)
	}
	var scanAsset, manualAsset UnifiedAsset
	if err := db.Where("id = ?", "scan-asset").First(&scanAsset).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("id = ?", "manual-asset").First(&manualAsset).Error; err != nil {
		t.Fatal(err)
	}
	if scanAsset.Verification != "observed" {
		t.Fatalf("scan asset verification = %q, want observed", scanAsset.Verification)
	}
	if manualAsset.Verification != "declared" {
		t.Fatalf("manual asset verification = %q, want declared", manualAsset.Verification)
	}
	var scanObs, manualObs UnifiedAssetObservation
	if err := db.Where("id = ?", "scan-obs").First(&scanObs).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("id = ?", "manual-obs").First(&manualObs).Error; err != nil {
		t.Fatal(err)
	}
	if scanObs.Origin != "scan" || manualObs.Origin != "manual" {
		t.Fatalf("unexpected origins: scan=%q manual=%q", scanObs.Origin, manualObs.Origin)
	}
}
