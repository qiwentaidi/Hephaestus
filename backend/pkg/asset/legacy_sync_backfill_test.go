package asset

import (
	"context"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"
)

func TestBackfillMissingSubdomainObservationsReplaysOnlyMissingTasks(t *testing.T) {
	store := testStore(t)
	if err := store.DB.AutoMigrate(&database.VulnScanTask{}, &database.PortScanResult{}, &database.FingerprintInfo{}, &database.SubdomainScanResult{}); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	if err := store.DB.Create(&database.VulnScanTask{ID: "sub-task", Version: 1, ScanType: "subdomain", TaskName: "Subdomain collection", UpdatedAt: created}).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB.Create(&database.SubdomainScanResult{ID: "sub-1", TaskID: "sub-task", Domain: "example.com", Subdomain: "api.example.com", IPs: `["192.0.2.30"]`, Source: "crtsh", CreatedAt: created}).Error; err != nil {
		t.Fatal(err)
	}
	count, err := BackfillMissingSubdomainObservations(context.Background(), store.DB)
	if err != nil || count != 1 {
		t.Fatalf("first backfill = %d, err=%v", count, err)
	}
	var observations []database.UnifiedAssetObservation
	if err := store.DB.Where("source = ?", "subdomain").Find(&observations).Error; err != nil {
		t.Fatal(err)
	}
	if len(observations) != 2 {
		t.Fatalf("expected domain and IP observations, got %#v", observations)
	}
	for _, observation := range observations {
		if observation.TaskID != "sub-task" || observation.Origin != "scan" {
			t.Fatalf("subdomain observation lost task provenance: %#v", observation)
		}
	}
	count, err = BackfillMissingSubdomainObservations(context.Background(), store.DB)
	if err != nil || count != 0 {
		t.Fatalf("second backfill should be a no-op, count=%d err=%v", count, err)
	}
}
