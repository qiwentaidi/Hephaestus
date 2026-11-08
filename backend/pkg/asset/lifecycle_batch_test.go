package asset

import (
	"context"
	"fmt"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"
)

func TestReconcileLifecycleBatchesManyChanges(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	assets := make([]database.UnifiedAsset, 0, 250)
	for i := 0; i < 250; i++ {
		assets = append(assets, database.UnifiedAsset{
			ID:             fmt.Sprintf("asset_batch_%03d", i),
			ScopeKey:       "global",
			Type:           TypeDomain,
			CanonicalValue: fmt.Sprintf("batch-%03d.example.com", i),
			DisplayValue:   fmt.Sprintf("batch-%03d.example.com", i),
			Status:         "active",
			Verification:   "observed",
			FirstSeenAt:    now.Add(-48 * time.Hour),
			LastSeenAt:     now.Add(-48 * time.Hour),
		})
	}
	if err := store.DB.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	changed, err := store.ReconcileLifecycle(context.Background(), now, time.Hour, 72*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if changed != len(assets) {
		t.Fatalf("changed = %d, want %d", changed, len(assets))
	}
	var changeCount int64
	if err := store.DB.Model(&database.UnifiedAssetChange{}).Where("change_type = ?", "stale").Count(&changeCount).Error; err != nil {
		t.Fatal(err)
	}
	if changeCount != int64(len(assets)) {
		t.Fatalf("change rows = %d, want %d", changeCount, len(assets))
	}
}
