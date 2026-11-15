package asset

import (
	"context"
	"net/url"
	"testing"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func scopeArchiveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/asset-scope-archive.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&database.UnifiedAsset{},
		&database.UnifiedAssetObservation{},
		&database.UnifiedAssetRelation{},
		&database.VulnScanTask{},
		&database.AssetScopeArchive{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedScopeArchiveFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	seen := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	if err := db.Create(&database.VulnScanTask{ID: "task-1", Version: 1, TaskName: "Scope fixture", ScanType: "portscan", UpdatedAt: seen}).Error; err != nil {
		t.Fatal(err)
	}
	assets := []database.UnifiedAsset{
		{ID: "root", ScopeKey: "global", Type: "domain", CanonicalValue: "example.com", DisplayValue: "example.com", Status: "active", Verification: "observed", FirstSeenAt: seen, LastSeenAt: seen},
		{ID: "sub", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com", DisplayValue: "api.example.com", Status: "active", Verification: "observed", FirstSeenAt: seen, LastSeenAt: seen},
		{ID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", DisplayValue: "192.0.2.10", Status: "active", Verification: "observed", FirstSeenAt: seen, LastSeenAt: seen},
		{ID: "url", ScopeKey: "global", Type: "url", CanonicalValue: "https://api.example.com/login", DisplayValue: "https://api.example.com/login", Status: "active", Verification: "observed", FirstSeenAt: seen, LastSeenAt: seen},
	}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	observations := []database.UnifiedAssetObservation{
		{ID: "o-root", AssetID: "root", ScopeKey: "global", Type: "domain", CanonicalValue: "example.com", Source: "subdomain", TaskID: "task-1", TaskVersion: 1, Origin: "scan", ObservationKey: "k-root", Confidence: "high", ObservedAt: seen, Attributes: map[string]any{"taskId": "task-1", "version": 1}},
		{ID: "o-sub", AssetID: "sub", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com", Source: "subdomain", TaskID: "task-1", TaskVersion: 1, Origin: "scan", ObservationKey: "k-sub", Confidence: "high", ObservedAt: seen, Attributes: map[string]any{"taskId": "task-1", "version": 1, "subdomain": "api.example.com"}},
		{ID: "o-ip", AssetID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", Source: "subdomain", TaskID: "task-1", TaskVersion: 1, Origin: "scan", ObservationKey: "k-ip", Confidence: "high", ObservedAt: seen, Attributes: map[string]any{"taskId": "task-1", "version": 1, "subdomain": "api.example.com"}},
		{ID: "o-port", AssetID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", Source: "portscan", TaskID: "task-1", TaskVersion: 1, Origin: "scan", ObservationKey: "k-port", Confidence: "high", ObservedAt: seen, Attributes: map[string]any{"taskId": "task-1", "version": 1, "port": 443, "transport": "tcp", "protocol": "https", "product": "nginx"}},
		{ID: "o-url", AssetID: "url", ScopeKey: "global", Type: "url", CanonicalValue: "https://api.example.com/login", Source: "fingerprint", TaskID: "task-1", TaskVersion: 1, Origin: "scan", ObservationKey: "k-url", Confidence: "high", ObservedAt: seen, Attributes: map[string]any{"taskId": "task-1", "version": 1, "status": 200, "title": "Login", "fingerprints": []string{"nginx"}}},
	}
	if err := db.Create(&observations).Error; err != nil {
		t.Fatal(err)
	}
}

func TestAssetScopeArchiveHidesRootTreeAndRestores(t *testing.T) {
	db := scopeArchiveTestDB(t)
	seedScopeArchiveFixture(t, db)

	baseline, err := buildLedgerEntitySet(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline.roots) != 1 || len(baseline.subdomains) != 1 || len(baseline.ips) != 1 || len(baseline.ports) != 2 || len(baseline.websites) != 1 {
		t.Fatalf("unexpected baseline: roots=%d subdomains=%d ips=%d ports=%d websites=%d",
			len(baseline.roots), len(baseline.subdomains), len(baseline.ips), len(baseline.ports), len(baseline.websites))
	}
	rootRow := baseline.roots[0]
	if rootRow.ArchiveSubdomainCount != 1 || rootRow.ArchivePortCount != 1 || rootRow.ArchiveWebsiteCount != 1 {
		t.Fatalf("unexpected root scope counts: %#v", rootRow)
	}

	if err := setAssetScopeArchive(context.Background(), db, database.AssetScopeCategoryRoot, "example.com", true); err != nil {
		t.Fatal(err)
	}

	hidden, err := buildLedgerEntitySet(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden.roots) != 0 || len(hidden.subdomains) != 0 || len(hidden.websites) != 0 {
		t.Fatalf("root scope did not hide its tree: roots=%d subdomains=%d websites=%d",
			len(hidden.roots), len(hidden.subdomains), len(hidden.websites))
	}
	if len(hidden.ips) != 1 {
		t.Fatalf("shared IP asset must stay visible, got %d", len(hidden.ips))
	}
	if len(hidden.ports) != 1 || hidden.ports[0].Host != "192.0.2.10" {
		t.Fatalf("only the IP-hosted port should remain, got %#v", hidden.ports)
	}

	archived, err := buildLedgerEntitySet(db, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived.roots) != 1 {
		t.Fatalf("archived view must expose one scope row, got %d", len(archived.roots))
	}
	archivedRoot := archived.roots[0]
	if !archivedRoot.ScopeArchived || !archivedRoot.Archived || archivedRoot.AssetID != "" {
		t.Fatalf("archived root row did not carry scope state: %#v", archivedRoot)
	}
	if archivedRoot.SubdomainCount != 1 || archivedRoot.PortCount != 1 || archivedRoot.WebsiteCount != 1 {
		t.Fatalf("archived root counts must reflect hidden rows: %#v", archivedRoot)
	}
	if len(archived.subdomains) != 0 || len(archived.websites) != 0 || len(archived.ips) != 1 || len(archived.ports) != 1 {
		t.Fatalf("archived view must not expand the hidden subtree: subs=%d sites=%d ips=%d ports=%d",
			len(archived.subdomains), len(archived.websites), len(archived.ips), len(archived.ports))
	}

	if err := setAssetScopeArchive(context.Background(), db, database.AssetScopeCategoryRoot, "example.com", false); err != nil {
		t.Fatal(err)
	}
	restored, err := buildLedgerEntitySet(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.roots) != 1 || len(restored.subdomains) != 1 || len(restored.websites) != 1 || len(restored.ports) != 2 {
		t.Fatalf("restore did not bring the tree back: %#v", restored)
	}
}

func TestAssetScopeArchiveSynthesizesRootWithoutAssetRow(t *testing.T) {
	db := scopeArchiveTestDB(t)
	seen := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	if err := db.Create(&database.UnifiedAsset{
		ID: "sub", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com", DisplayValue: "api.example.com",
		Status: "active", Verification: "observed", FirstSeenAt: seen, LastSeenAt: seen,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.UnifiedAssetObservation{
		ID: "o-sub", AssetID: "sub", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com",
		Source: "subdomain", Origin: "scan", ObservationKey: "k-sub", Confidence: "high", ObservedAt: seen,
		Attributes: map[string]any{"subdomain": "api.example.com"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	baseline, err := buildLedgerEntitySet(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline.roots) != 1 || baseline.roots[0].AssetID != "" {
		t.Fatalf("expected an inferred root without asset id: %#v", baseline.roots)
	}
	if err := setAssetScopeArchive(context.Background(), db, database.AssetScopeCategoryRoot, "example.com", true); err != nil {
		t.Fatal(err)
	}
	archived, err := buildLedgerEntitySet(db, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived.roots) != 1 || !archived.roots[0].ScopeArchived || archived.roots[0].SubdomainCount != 1 {
		t.Fatalf("inferred root scope row was not synthesized: %#v", archived.roots)
	}
}

func TestAssetScopeArchivePortScopeKeepsHost(t *testing.T) {
	db := scopeArchiveTestDB(t)
	seedScopeArchiveFixture(t, db)
	value := assetScopePortValue("192.0.2.10", 443, "tcp")
	if value == "" {
		t.Fatal("empty port scope value")
	}
	if err := setAssetScopeArchive(context.Background(), db, database.AssetScopeCategoryPort, value, true); err != nil {
		t.Fatal(err)
	}
	set, err := buildLedgerEntitySet(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.ips) != 1 || len(set.ports) != 1 {
		t.Fatalf("port scope must keep the IP host and other ports: ips=%d ports=%d", len(set.ips), len(set.ports))
	}
	for _, port := range set.ports {
		if port.Host == "192.0.2.10" && port.Port == 443 {
			t.Fatalf("archived port row is still visible: %#v", port)
		}
	}
	var ipRow database.UnifiedAsset
	if err := db.First(&ipRow, "id = ?", "ip").Error; err != nil {
		t.Fatal(err)
	}
	if ipRow.ArchivedAt != nil {
		t.Fatal("port scope must not archive the host asset")
	}
	archived, err := buildLedgerEntitySet(db, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, port := range archived.ports {
		if port.ScopeArchived && port.Host == "192.0.2.10" && port.Port == 443 {
			found = true
		}
	}
	if !found {
		t.Fatalf("archived view did not expose the port scope row: %#v", archived.ports)
	}
}

func TestApplyLedgerEntityArchiveHandlesScopeSelections(t *testing.T) {
	db := scopeArchiveTestDB(t)
	seedScopeArchiveFixture(t, db)
	store := assetstore.Store{DB: db}
	ctx := context.Background()

	result, err := applyLedgerEntityArchive(ctx, db, store, []ledgerEntitySelection{
		{ID: "root:example.com", Category: "root", Value: "example.com", Scope: true},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || result.Skipped != 0 || len(result.Failed) != 0 {
		t.Fatalf("unexpected root archive result: %#v", result)
	}
	var count int64
	if err := db.Model(&database.AssetScopeArchive{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("root scope row not stored: count=%d err=%v", count, err)
	}

	result, err = applyLedgerEntityArchive(ctx, db, store, []ledgerEntitySelection{
		{ID: "port:192.0.2.10:443:tcp", Category: "port", Value: "192.0.2.10:443", Host: "192.0.2.10", Port: 443, Transport: "tcp", Scope: true},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || len(result.Failed) != 0 {
		t.Fatalf("unexpected port archive result: %#v", result)
	}
	var ipRow database.UnifiedAsset
	if err := db.First(&ipRow, "id = ?", "ip").Error; err != nil {
		t.Fatal(err)
	}
	if ipRow.ArchivedAt != nil {
		t.Fatal("port scope must not archive the host asset")
	}

	result, err = applyLedgerEntityArchive(ctx, db, store, []ledgerEntitySelection{
		{ID: "root:example.com", Category: "root", Value: "example.com", Scope: true},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 {
		t.Fatalf("unexpected root restore result: %#v", result)
	}
	if err := db.Model(&database.AssetScopeArchive{}).Where("category = ?", "root").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("root scope row not restored: count=%d err=%v", count, err)
	}
}

func TestAssetCenterAggregatesHideArchivedRootTree(t *testing.T) {
	db := scopeArchiveTestDB(t)
	seedScopeArchiveFixture(t, db)
	baseline, used, err := collectUnifiedAssetCenterAggregatesWithDB(db)
	if err != nil || !used {
		t.Fatalf("projection failed: used=%v err=%v", used, err)
	}
	if len(baseline) == 0 {
		t.Fatal("expected aggregate items")
	}
	if err := setAssetScopeArchive(context.Background(), db, database.AssetScopeCategoryRoot, "example.com", true); err != nil {
		t.Fatal(err)
	}
	items, used, err := collectUnifiedAssetCenterAggregatesWithDB(db)
	if err != nil || !used {
		t.Fatalf("projection failed after archive: used=%v err=%v", used, err)
	}
	var ipVisible bool
	for _, item := range items {
		if item.Type == "ip" && item.Value == "192.0.2.10" {
			ipVisible = true
		}
		if item.Type == "domain" && item.Value == "api.example.com" {
			t.Fatalf("archived root domain still projected: %#v", item)
		}
		if item.Type == "endpoint" {
			parsed, parseErr := url.Parse(item.Value)
			if parseErr == nil && parsed.Hostname() == "api.example.com" {
				t.Fatalf("archived root endpoint still projected: %#v", item)
			}
		}
	}
	if !ipVisible {
		t.Fatal("shared IP asset must remain in the projection")
	}
}

func TestAssetScopeArchiveHidesNewObservationsAndSurvivesTaskDeletion(t *testing.T) {
	db := scopeArchiveTestDB(t)
	seedScopeArchiveFixture(t, db)
	if err := setAssetScopeArchive(context.Background(), db, database.AssetScopeCategoryRoot, "example.com", true); err != nil {
		t.Fatal(err)
	}
	seen := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	if err := db.Create(&database.UnifiedAsset{
		ID: "new-sub", ScopeKey: "global", Type: "domain", CanonicalValue: "new.example.com", DisplayValue: "new.example.com",
		Status: "active", Verification: "observed", FirstSeenAt: seen, LastSeenAt: seen,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.UnifiedAssetObservation{
		ID: "o-new-sub", AssetID: "new-sub", ScopeKey: "global", Type: "domain", CanonicalValue: "new.example.com",
		Source: "subdomain", Origin: "scan", ObservationKey: "k-new-sub", Confidence: "high", ObservedAt: seen,
		Attributes: map[string]any{"subdomain": "new.example.com"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("id = ?", "task-1").Delete(&database.VulnScanTask{}).Error; err != nil {
		t.Fatal(err)
	}
	normal, err := buildLedgerEntitySet(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(normal.roots) != 0 || len(normal.subdomains) != 0 {
		t.Fatalf("new observations under an archived root should stay hidden: %#v", normal)
	}
	archived, err := buildLedgerEntitySet(db, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived.roots) != 1 || archived.roots[0].SubdomainCount != 2 {
		t.Fatalf("archived root should count both hidden subdomains: %#v", archived.roots)
	}
}
