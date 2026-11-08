package asset

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testStore(t *testing.T) Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "asset.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.UnifiedAsset{}, &database.UnifiedAssetObservation{}, &database.UnifiedAssetRelation{}, &database.UnifiedAssetChange{}); err != nil {
		t.Fatal(err)
	}
	return Store{DB: db}
}

func TestNormalizeCanonicalValues(t *testing.T) {
	tests := []struct {
		name, typ, value, wantType, want string
	}{
		{"domain url", TypeDomain, "HTTPS://WWW.Example.COM./login", TypeDomain, "www.example.com"},
		{"wildcard domain", TypeDomain, "*.API.Example.COM.", TypeDomain, "api.example.com"},
		{"internal host", TypeDomain, "Intranet", TypeDomain, "intranet"},
		{"domain with adjacent hyphens", TypeDomain, "ab--cd.example.com", TypeDomain, "ab--cd.example.com"},
		{"internationalized domain", TypeDomain, "例子.中国", TypeDomain, "例子.中国"},
		{"DNS record name", TypeDomain, "_dmarc.Example.COM.", TypeDomain, "_dmarc.example.com"},
		{"ipv6", TypeIP, "[2001:0DB8:0:0:0:0:0:1]", TypeIP, "2001:db8::1"},
		{"url default port", TypeURL, "HTTPS://Example.COM:443/#fragment", TypeURL, "https://example.com"},
		{"service", TypeService, "  Nginx   1.24 ", TypeService, "nginx 1.24"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize("", tt.typ, tt.value)
			if err != nil {
				t.Fatal(err)
			}
			if got.ScopeKey != DefaultScope || got.Type != tt.wantType || got.CanonicalValue != tt.want {
				t.Fatalf("unexpected normalized value: %#v", got)
			}
		})
	}
}

func TestNormalizeRejectsInvalidValues(t *testing.T) {
	for _, input := range [][2]string{
		{TypeIP, "not-an-ip"}, {TypeDomain, "https://"}, {TypeURL, "example.com"}, {"unknown", "value"},
		{TypeDomain, "username='+Ext.myCtx.LoginInfo.loginname;j+='"},
		{TypeDomain, "username=’+Ext.myCtx.LoginInfo.loginname;j+=’"},
		{TypeDomain, "user@example.com"},
		{TypeDomain, "api..example.com"},
		{TypeDomain, "api.example.com.."},
		{TypeDomain, "-api.example.com"},
		{TypeDomain, "https://api+token.example.com/login"},
	} {
		if _, err := Normalize("global", input[0], input[1]); err == nil {
			t.Fatalf("expected %s %q to be rejected", input[0], input[1])
		}
	}
}

func TestUpsertIsIdempotentAndPreservesProvenance(t *testing.T) {
	store := testStore(t)
	first := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	input := Observation{ScopeKey: "project-a", Type: TypeDomain, Value: "Example.COM.", DisplayValue: "Example.COM.", Source: "subdomain", SourceID: "task-1", SourceVersion: 2, Confidence: "high", ObservedAt: first, EvidenceRef: "evidence://one", Attributes: map[string]any{"source": "crtsh"}}
	assetRow, observationRow, err := store.Upsert(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if assetRow.CanonicalValue != "example.com" || observationRow.AssetID != assetRow.ID {
		t.Fatalf("unexpected first upsert: %#v %#v", assetRow, observationRow)
	}
	second := input
	second.ObservedAt = first.Add(24 * time.Hour)
	assetAgain, observationAgain, err := store.Upsert(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if assetAgain.ID != assetRow.ID || observationAgain.ID != observationRow.ID {
		t.Fatalf("retry created duplicate identity: %s/%s vs %s/%s", assetAgain.ID, observationAgain.ID, assetRow.ID, observationRow.ID)
	}
	var assetCount, observationCount int64
	store.DB.Model(&database.UnifiedAsset{}).Count(&assetCount)
	store.DB.Model(&database.UnifiedAssetObservation{}).Count(&observationCount)
	if assetCount != 1 || observationCount != 1 {
		t.Fatalf("unexpected counts: assets=%d observations=%d", assetCount, observationCount)
	}
	if !assetAgain.FirstSeenAt.Equal(first) || !assetAgain.LastSeenAt.Equal(second.ObservedAt) || assetAgain.Confidence != "high" {
		t.Fatalf("observation window or confidence lost: %#v", assetAgain)
	}

	other := input
	other.SourceID = "task-2"
	other.ObservedAt = first.Add(48 * time.Hour)
	if _, _, err := store.Upsert(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	store.DB.Model(&database.UnifiedAssetObservation{}).Count(&observationCount)
	if observationCount != 2 {
		t.Fatalf("distinct source observation was deduplicated: %d", observationCount)
	}
}

func TestListFiltersUnifiedAssets(t *testing.T) {
	store := testStore(t)
	for _, input := range []Observation{
		{Type: TypeDomain, Value: "example.com", Source: "manual", Confidence: "high"},
		{Type: TypeIP, Value: "192.0.2.10", Source: "portscan", Confidence: "medium"},
	} {
		if _, _, err := store.Upsert(context.Background(), input); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := store.List(context.Background(), ListFilter{ScopeKey: DefaultScope, Type: TypeDomain, Keyword: "example", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].CanonicalValue != "example.com" {
		t.Fatalf("unexpected filtered result: total=%d items=%#v", total, items)
	}
}

func TestUpsertManyIsAtomic(t *testing.T) {
	store := testStore(t)
	_, _, err := store.UpsertMany(context.Background(), []Observation{
		{Type: TypeDomain, Value: "example.com", Source: "manual"},
		{Type: TypeIP, Value: "not-an-ip", Source: "manual"},
	})
	if err == nil {
		t.Fatal("expected invalid batch item to fail")
	}
	var count int64
	store.DB.Model(&database.UnifiedAsset{}).Count(&count)
	if count != 0 {
		t.Fatalf("partial batch write left %d assets", count)
	}
}

func TestSyncLegacyImportsSourcesIntoUnifiedCatalog(t *testing.T) {
	store := testStore(t)
	if err := store.DB.AutoMigrate(&database.VulnScanTask{}, &database.PortScanResult{}, &database.FingerprintInfo{}, &database.SubdomainScanResult{}); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	if err := store.DB.Create(&database.VulnScanTask{ID: "task-1", Version: 2, ScanType: "portscan"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB.Create(&database.PortScanResult{ID: 7, TaskID: "task-1", Version: 2, Host: "192.0.2.10", Port: 443, Transport: "tcp", Protocol: "https", Product: "nginx", ServiceVersion: "1.24", DiscoveryStatus: "verified", CreatedAt: created}).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB.Create(&database.FingerprintInfo{TaskID: "task-1", Version: 2, URL: "https://Example.com:443/login#fragment", Status: 200, Fingerprints: `["nginx"]`, CreatedAt: created}).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB.Create(&database.SubdomainScanResult{ID: "sub-1", TaskID: "task-1", Domain: "example.com", Subdomain: "api.example.com.", IPs: `["192.0.2.10"]`, Source: "crtsh", CreatedAt: created}).Error; err != nil {
		t.Fatal(err)
	}
	summary, err := store.SyncLegacy(context.Background(), LegacySyncOptions{ScopeKey: "project-a"})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Observations != 6 || summary.BySource["portscan"] != 2 || summary.BySource["fingerprint"] != 2 || summary.BySource["subdomain"] != 2 {
		t.Fatalf("unexpected sync summary: %#v", summary)
	}
	items, total, err := store.List(context.Background(), ListFilter{ScopeKey: "project-a", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(items) != 5 {
		t.Fatalf("unexpected unified asset count: total=%d items=%#v", total, items)
	}
	var relationCount, changeCount int64
	store.DB.Model(&database.UnifiedAssetRelation{}).Count(&relationCount)
	store.DB.Model(&database.UnifiedAssetChange{}).Count(&changeCount)
	if relationCount != 3 || changeCount != 6 {
		t.Fatalf("expected relations and change timeline, relations=%d changes=%d", relationCount, changeCount)
	}
	if _, total, err = store.List(context.Background(), ListFilter{ScopeKey: "project-a", Type: TypeIP, Limit: 20}); err != nil || total != 1 {
		t.Fatalf("expected one normalized IP asset, total=%d err=%v", total, err)
	}
}

func TestLifecycleReconcileAndReactivation(t *testing.T) {
	store := testStore(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if _, _, err := store.Upsert(context.Background(), Observation{Type: TypeDomain, Value: "stale.example.com", Source: "portscan", SourceID: "initial", ObservedAt: base}); err != nil {
		t.Fatal(err)
	}
	changed, err := store.ReconcileLifecycle(context.Background(), base.Add(2*24*time.Hour), 24*time.Hour, 72*time.Hour)
	if err != nil || changed != 1 {
		t.Fatalf("expected stale transition, changed=%d err=%v", changed, err)
	}
	var current database.UnifiedAsset
	if err := store.DB.Where("canonical_value = ?", "stale.example.com").First(&current).Error; err != nil || current.Status != "stale" {
		t.Fatalf("asset did not become stale: %#v err=%v", current, err)
	}
	changed, err = store.ReconcileLifecycle(context.Background(), base.Add(4*24*time.Hour), 24*time.Hour, 72*time.Hour)
	if err != nil || changed != 1 {
		t.Fatalf("expected retired transition, changed=%d err=%v", changed, err)
	}
	if err := store.DB.Where("id = ?", current.ID).First(&current).Error; err != nil || current.Status != "retired" {
		t.Fatalf("asset did not become retired: %#v err=%v", current, err)
	}
	if _, _, err := store.Upsert(context.Background(), Observation{Type: TypeDomain, Value: "stale.example.com", Source: "portscan", SourceID: "reactivated", ObservedAt: base.Add(5 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.Where("id = ?", current.ID).First(&current).Error; err != nil || current.Status != "active" {
		t.Fatalf("asset did not reactivate: %#v err=%v", current, err)
	}
	var reactivated int64
	store.DB.Model(&database.UnifiedAssetChange{}).Where("asset_id = ? AND change_type = ?", current.ID, "reactivated").Count(&reactivated)
	if reactivated != 1 {
		t.Fatalf("reactivation change missing: %d", reactivated)
	}
}

func TestParseIPLikeNormalizesScannerIPv6Variants(t *testing.T) {
	for _, value := range []string{
		"2603:1046:c01:2006:0:0:0:1",
		"2603:1046:c01:2006:0:0:0:1:",
		"[2603:1046:c01:2006:0:0:0:1]",
		"[2603:1046:c01:2006:0:0:0:1]:443",
		"2603:1046:c01:2006:0:0:0:1%eth0",
	} {
		if ip, ok := ParseIPLike(value); !ok || ip != "2603:1046:c01:2006::1" {
			t.Fatalf("ParseIPLike(%q) = %q, %v", value, ip, ok)
		}
	}
	if !LooksLikeIPv6("2603:1046:c01:2006:") {
		t.Fatal("truncated IPv6 should be recognized as IPv6-shaped")
	}
	if _, ok := ParseIPLike("app.example.com"); ok {
		t.Fatal("domain must not parse as IP")
	}
}

func TestClassifyHostUsesSingleRule(t *testing.T) {
	if got := ClassifyHost("2603:1046:c01:2006:"); got != TypeIP {
		t.Fatalf("truncated IPv6 classified as %s", got)
	}
	if got := ClassifyHost("2603:1046:c01:2006:0:0:0:1"); got != TypeIP {
		t.Fatalf("IPv6 classified as %s", got)
	}
	if got := ClassifyHost("app.example.com"); got != TypeDomain {
		t.Fatalf("domain classified as %s", got)
	}
}

func TestReclassifyMisclassifiedIPAssets(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.UnifiedAsset{},
		&database.UnifiedAssetObservation{},
		&database.UnifiedAssetRelation{},
		&database.UnifiedAssetChange{},
		&database.SecurityFinding{},
		&database.FindingAssetLink{},
		&database.AssetRisk{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Now().UTC()
	validCanonical := "2603:1046:c01:2006:0:0:0:1"
	validIP := "2603:1046:c01:2006::1"
	old := database.UnifiedAsset{
		ID:             "asset_old_domain",
		ScopeKey:       DefaultScope,
		Type:           TypeDomain,
		CanonicalValue: validCanonical,
		DisplayValue:   validCanonical,
		Status:         "active",
		Confidence:     "medium",
		FirstSeenAt:    now,
		LastSeenAt:     now,
	}
	if err := db.Create(&old).Error; err != nil {
		t.Fatalf("create old asset: %v", err)
	}
	if err := db.Create(&database.UnifiedAssetObservation{
		ID: "obs_old", AssetID: old.ID, ScopeKey: DefaultScope, Type: TypeDomain,
		CanonicalValue: validCanonical, Source: "subdomain", ObservationKey: "obs-key",
		Confidence: "medium", ObservedAt: now,
	}).Error; err != nil {
		t.Fatalf("create observation: %v", err)
	}
	if err := db.Create(&database.UnifiedAssetRelation{
		ID: "rel_old", ScopeKey: DefaultScope, FromAssetID: "asset_parent", ToAssetID: old.ID,
		RelationType: "resolves_to", Source: "subdomain", FirstSeenAt: now, LastSeenAt: now, Active: true,
	}).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}
	if err := db.Create(&database.AssetRisk{AssetID: old.ID, DirectHigh: 1, OpenFindings: 1}).Error; err != nil {
		t.Fatalf("create risk: %v", err)
	}

	invalid := database.UnifiedAsset{
		ID: "asset_bad_domain", ScopeKey: DefaultScope, Type: TypeDomain,
		CanonicalValue: "2603:1046:c01:2006:", DisplayValue: "2603:1046:c01:2006:",
		Status: "active", Confidence: "medium", FirstSeenAt: now, LastSeenAt: now,
	}
	if err := db.Create(&invalid).Error; err != nil {
		t.Fatalf("create invalid asset: %v", err)
	}

	summary, err := ReclassifyMisclassifiedIPAssets(context.Background(), db)
	if err != nil {
		t.Fatalf("reclassify: %v", err)
	}
	if summary.Reclassified != 1 || summary.Archived != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	var target database.UnifiedAsset
	if err := db.Where("type = ? AND canonical_value = ?", TypeIP, validIP).First(&target).Error; err != nil {
		t.Fatalf("load reclassified asset: %v", err)
	}
	if target.ID != AssetID(DefaultScope, TypeIP, validIP) {
		t.Fatalf("unexpected target id: %#v", target)
	}
	var oldCount int64
	if err := db.Model(&database.UnifiedAsset{}).Where("id = ?", old.ID).Count(&oldCount).Error; err != nil || oldCount != 0 {
		t.Fatalf("old asset still present: %d, %v", oldCount, err)
	}
	var observation database.UnifiedAssetObservation
	if err := db.Where("id = ?", "obs_old").First(&observation).Error; err != nil {
		t.Fatalf("load observation: %v", err)
	}
	if observation.AssetID != target.ID || observation.Type != TypeIP {
		t.Fatalf("observation not moved: %#v", observation)
	}
	var relation database.UnifiedAssetRelation
	if err := db.Where("id = ?", "rel_old").First(&relation).Error; err != nil {
		t.Fatalf("load relation: %v", err)
	}
	if relation.ToAssetID != target.ID {
		t.Fatalf("relation not moved: %#v", relation)
	}
	var risk database.AssetRisk
	if err := db.Where("asset_id = ?", target.ID).First(&risk).Error; err != nil || risk.DirectHigh != 1 {
		t.Fatalf("risk not moved: %#v, %v", risk, err)
	}
	var archived database.UnifiedAsset
	if err := db.Where("id = ?", invalid.ID).First(&archived).Error; err != nil {
		t.Fatalf("load invalid asset: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("invalid IPv6 asset was not archived: %#v", archived)
	}
}
