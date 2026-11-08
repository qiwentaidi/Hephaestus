package asset

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBuildLedgerEntitySetGroupsRootIPPortAndWebsite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/entities.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.UnifiedAsset{}, &database.UnifiedAssetObservation{}, &database.UnifiedAssetRelation{}, &database.VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	const invalidDomain = "username='+ext.myctx.logininfo.loginname;j+='"
	const sensitiveDomain = "auth.token.invalid"
	const sensitiveURL = "https://auth.token.invalid/services/example-secret"
	assets := []database.UnifiedAsset{
		{ID: "root", ScopeKey: "global", Type: "domain", CanonicalValue: "example.com", DisplayValue: "example.com", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "sub", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com", DisplayValue: "api.example.com", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", DisplayValue: "192.0.2.10", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "url", ScopeKey: "global", Type: "url", CanonicalValue: "https://api.example.com/login", DisplayValue: "https://api.example.com/login", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "legacy-invalid-domain", ScopeKey: "global", Type: "domain", CanonicalValue: invalidDomain, DisplayValue: invalidDomain,
			Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "sensitive-domain", ScopeKey: "global", Type: "domain", CanonicalValue: sensitiveDomain, DisplayValue: sensitiveDomain, Status: "active", FirstSeenAt: now, LastSeenAt: now},
		{ID: "sensitive-url", ScopeKey: "global", Type: "url", CanonicalValue: sensitiveURL, DisplayValue: sensitiveURL, Status: "active", FirstSeenAt: now, LastSeenAt: now},
	}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	observations := []database.UnifiedAssetObservation{
		{ID: "o-root", AssetID: "root", ScopeKey: "global", Type: "domain", CanonicalValue: "example.com", Source: "subdomain", TaskID: "task-1", TaskVersion: 1, Origin: "scan", ObservationKey: "k-root", Confidence: "medium", ObservedAt: now, Attributes: map[string]any{"taskId": "task-1", "version": 1}},
		{ID: "o-sub", AssetID: "sub", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com", Source: "subdomain", TaskID: "task-1", TaskVersion: 1, Origin: "scan", ObservationKey: "k-sub", Confidence: "medium", ObservedAt: now, Attributes: map[string]any{"taskId": "task-1", "version": 1, "subdomain": "api.example.com"}},
		{ID: "o-ip", AssetID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", Source: "subdomain", TaskID: "task-1", TaskVersion: 1, Origin: "scan", ObservationKey: "k-ip", Confidence: "medium", ObservedAt: now, Attributes: map[string]any{"taskId": "task-1", "version": 1, "subdomain": "api.example.com"}},
		{ID: "o-port", AssetID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", Source: "portscan", TaskID: "task-1", TaskVersion: 1, Origin: "scan", ObservationKey: "k-port", Confidence: "high", ObservedAt: now, Attributes: map[string]any{"taskId": "task-1", "version": 1, "port": 443, "transport": "tcp", "protocol": "https", "product": "nginx", "serviceVersion": "1.24"}},
		{ID: "o-url", AssetID: "url", ScopeKey: "global", Type: "url", CanonicalValue: "https://api.example.com/login", Source: "fingerprint", TaskID: "task-1", TaskVersion: 1, Origin: "scan", ObservationKey: "k-url", Confidence: "high", ObservedAt: now, Attributes: map[string]any{"taskId": "task-1", "version": 1, "status": 200, "title": "Login", "fingerprints": []string{"nginx", "Vue"}, "highRiskFingerprints": []string{"GitLab"}}},
		{ID: "o-invalid-domain", AssetID: "legacy-invalid-domain", ScopeKey: "global", Type: "domain", CanonicalValue: invalidDomain, Source: "jsscan", Origin: "scan", ObservationKey: "k-invalid-domain", ObservedAt: now, Attributes: map[string]any{"port": 443}},
		{ID: "o-invalid-alias", AssetID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", Source: "subdomain", Origin: "scan", ObservationKey: "k-invalid-alias", ObservedAt: now, Attributes: map[string]any{"subdomain": invalidDomain}},
		{ID: "o-sensitive-domain", AssetID: "sensitive-domain", ScopeKey: "global", Type: "domain", CanonicalValue: sensitiveDomain, Source: "jsscan", Origin: "scan", ObservationKey: "k-sensitive-domain", ObservedAt: now, Attributes: map[string]any{"assetType": "sensitive"}},
		{ID: "o-sensitive-url", AssetID: "sensitive-url", ScopeKey: "global", Type: "url", CanonicalValue: sensitiveURL, Source: "jsscan", Origin: "scan", ObservationKey: "k-sensitive-url", ObservedAt: now, Attributes: map[string]any{"assetType": "sensitive"}},
		{ID: "o-sensitive-overlap", AssetID: "sub", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com", Source: "jsscan", TaskID: "task-sensitive", Origin: "scan", ObservationKey: "k-sensitive-overlap", ObservedAt: now, Attributes: map[string]any{"assetType": "sensitive"}},
	}
	if err := db.Create(&observations).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.UnifiedAssetRelation{
		ID: "rel-1", ScopeKey: "global", FromAssetID: "sub", ToAssetID: "ip", RelationType: "resolves_to", Source: "subdomain", SourceID: "sub", Active: true, FirstSeenAt: now, LastSeenAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.VulnScanTask{ID: "task-1", Version: 1, TaskName: "Asset discovery", ScanType: "subdomain", UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	set, err := buildLedgerEntitySet(db, false)
	if err != nil {
		t.Fatal(err)
	}
	counts := set.counts()
	if counts.Root != 1 || counts.Subdomain != 1 || counts.IP != 1 || counts.Port != 2 || counts.Website != 1 {
		t.Fatalf("unexpected entity counts: %#v", counts)
	}
	var legacyAsset database.UnifiedAsset
	if err := db.First(&legacyAsset, "id = ?", "legacy-invalid-domain").Error; err != nil {
		t.Fatalf("ledger projection removed historical evidence: %v", err)
	}
	root := set.roots[0]
	if root.Value != "example.com" || root.SubdomainCount != 1 || root.IPCount != 1 || root.PortCount != 2 || root.WebsiteCount != 1 {
		t.Fatalf("unexpected root entity: %#v", root)
	}
	subdomain := set.subdomains[0]
	if subdomain.Value != "api.example.com" || subdomain.RootDomain != "example.com" || len(subdomain.IPs) != 1 || subdomain.IPs[0] != "192.0.2.10" || subdomain.PortCount != 1 || subdomain.WebsiteCount != 1 {
		t.Fatalf("unexpected subdomain entity: %#v", subdomain)
	}
	if len(subdomain.Sources) != 1 || subdomain.Sources[0] != "subdomain" || len(subdomain.SourceTaskIDs) != 1 || subdomain.SourceTaskIDs[0] != "task-1" {
		t.Fatalf("sensitive observation was treated as asset evidence: %#v", subdomain)
	}
	ip := set.ips[0]
	if len(ip.Domains) != 1 || ip.Domains[0] != "api.example.com" {
		t.Fatalf("unexpected resolved domains on IP entity: %#v", ip.Domains)
	}
	var port, websitePort *LedgerEntity
	for index := range set.ports {
		switch set.ports[index].Value {
		case "192.0.2.10:443":
			port = &set.ports[index]
		case "api.example.com:443":
			websitePort = &set.ports[index]
		}
	}
	if port == nil || port.Port != 443 || port.Transport != "tcp" || port.Service != "nginx 1.24" {
		t.Fatalf("unexpected IP port entity: %#v", port)
	}
	if websitePort == nil || websitePort.WebsiteCount != 1 || !containsAny(websitePort.Fingerprints, "nginx") {
		t.Fatalf("unexpected website port entity: %#v", websitePort)
	}
	website := set.websites[0]
	if website.Value != "https://api.example.com" || website.Host != "api.example.com" || website.Port != 443 || website.Title != "Login" || website.HTTPStatus != 200 {
		t.Fatalf("unexpected website entity: %#v", website)
	}
	if !containsAny(website.Fingerprints, "nginx") || !containsAny(website.Fingerprints, "Vue") {
		t.Fatalf("website fingerprints missing: %#v", website.Fingerprints)
	}
	if err := attachLedgerTaskNames(db, set.websites); err != nil {
		t.Fatal(err)
	}
	if set.websites[0].LastTaskID != "task-1" || set.websites[0].LastTaskName != "Asset discovery" {
		t.Fatalf("website task provenance missing: %#v", set.websites[0])
	}

	center, used, err := collectUnifiedAssetCenterAggregatesWithDB(db)
	if err != nil || !used {
		t.Fatalf("could not load asset topology data: used=%v err=%v", used, err)
	}
	for _, item := range center {
		if item.Value == sensitiveDomain || item.Value == sensitiveURL || item.EndpointHost == sensitiveDomain {
			t.Errorf("asset topology included sensitive evidence: %#v", item)
		}
	}
	inventory, used, err := collectUnifiedAssetInventoryWithDB(db, "")
	if err != nil || !used {
		t.Fatalf("could not load inventory: used=%v err=%v", used, err)
	}
	for _, item := range inventory {
		if item.Value == sensitiveDomain {
			t.Errorf("inventory included sensitive evidence: %#v", item)
		}
	}
}

func TestNormalizePortProtocolDropsTransportLabel(t *testing.T) {
	cases := []struct {
		protocol, transport, want string
	}{
		{"http", "tcp", "http"},
		{"http / tcp", "tcp", "http"},
		{"tcp", "tcp", ""},
		{"", "tcp", ""},
		{"ssh", "", "ssh"},
	}
	for _, tc := range cases {
		if got := normalizePortProtocol(tc.protocol, tc.transport); got != tc.want {
			t.Fatalf("normalizePortProtocol(%q, %q) = %q, want %q", tc.protocol, tc.transport, got, tc.want)
		}
	}
}

func TestBuildLedgerEntitySetHidesArchivedRootAggregate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/archived-root.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.UnifiedAsset{}, &database.UnifiedAssetObservation{}, &database.UnifiedAssetRelation{}, &database.VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	archivedAt := now
	assets := []database.UnifiedAsset{
		{ID: "root", ScopeKey: "global", Type: "domain", CanonicalValue: "example.com", DisplayValue: "example.com", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now, ArchivedAt: &archivedAt},
		{ID: "sub", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com", DisplayValue: "api.example.com", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
	}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	set, err := buildLedgerEntitySet(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.roots) != 0 || len(set.subdomains) != 1 {
		t.Fatalf("archived root should be hidden without archived rows: roots=%#v subdomains=%#v", set.roots, set.subdomains)
	}
	set, err = buildLedgerEntitySet(db, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.roots) != 1 || !set.roots[0].Archived {
		t.Fatalf("archived root should appear with archived rows: %#v", set.roots)
	}
}

func TestBuildLedgerEntitySetReadsSpaceEngineURLsAndPorts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/space-engine.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.UnifiedAsset{}, &database.UnifiedAssetObservation{}, &database.UnifiedAssetRelation{}, &database.VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 24, 10, 30, 0, 0, time.UTC)
	assets := []database.UnifiedAsset{
		{ID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.60", DisplayValue: "192.0.2.60", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "url-http-443", ScopeKey: "global", Type: "url", CanonicalValue: "http://192.0.2.60:443", DisplayValue: "http://192.0.2.60:443", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "url-https-443", ScopeKey: "global", Type: "url", CanonicalValue: "https://192.0.2.60", DisplayValue: "https://192.0.2.60", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "url-http-80", ScopeKey: "global", Type: "url", CanonicalValue: "http://192.0.2.60", DisplayValue: "http://192.0.2.60", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "url-hunter", ScopeKey: "global", Type: "url", CanonicalValue: "https://admin.example.com", DisplayValue: "https://admin.example.com", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
	}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	ipAttributes := func(port, protocol string) map[string]any {
		return map[string]any{"port": port, "protocol": protocol, "title": "404 Not Found", "engine": "uncover"}
	}
	urlAttributes := func(port, protocol, title string) map[string]any {
		return map[string]any{"port": port, "protocol": protocol, "title": title, "engine": "uncover"}
	}
	observations := []database.UnifiedAssetObservation{
		{ID: "o-ip-http-443", AssetID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.60", Source: "uncover", ObservationKey: "k-ip-http-443", Confidence: "medium", ObservedAt: now, Attributes: ipAttributes("443", "http")},
		{ID: "o-ip-https-443", AssetID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.60", Source: "uncover", ObservationKey: "k-ip-https-443", Confidence: "medium", ObservedAt: now, Attributes: ipAttributes("443", "https")},
		{ID: "o-ip-http-80", AssetID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.60", Source: "uncover", ObservationKey: "k-ip-http-80", Confidence: "medium", ObservedAt: now, Attributes: ipAttributes("80", "http")},
		{ID: "o-url-http-443", AssetID: "url-http-443", ScopeKey: "global", Type: "url", CanonicalValue: "http://192.0.2.60:443", Source: "uncover", ObservationKey: "k-url-http-443", Confidence: "medium", ObservedAt: now, Attributes: urlAttributes("443", "http", "404 Not Found")},
		{ID: "o-url-https-443", AssetID: "url-https-443", ScopeKey: "global", Type: "url", CanonicalValue: "https://192.0.2.60", Source: "uncover", ObservationKey: "k-url-https-443", Confidence: "medium", ObservedAt: now, Attributes: urlAttributes("443", "https", "404 Not Found")},
		{ID: "o-url-http-80", AssetID: "url-http-80", ScopeKey: "global", Type: "url", CanonicalValue: "http://192.0.2.60", Source: "uncover", ObservationKey: "k-url-http-80", Confidence: "medium", ObservedAt: now, Attributes: urlAttributes("80", "http", "404 Not Found")},
		{ID: "o-url-hunter", AssetID: "url-hunter", ScopeKey: "global", Type: "url", CanonicalValue: "https://admin.example.com", Source: "hunter", ObservationKey: "k-url-hunter", Confidence: "medium", ObservedAt: now, Attributes: map[string]any{"port": "443", "protocol": "https", "title": "Admin", "status": "403", "engine": "hunter"}},
	}
	if err := db.Create(&observations).Error; err != nil {
		t.Fatal(err)
	}
	set, err := buildLedgerEntitySet(db, false)
	if err != nil {
		t.Fatal(err)
	}
	counts := set.counts()
	if counts.Port != 3 || counts.Website != 4 {
		t.Fatalf("unexpected entity counts: %#v", counts)
	}
	var port443 *LedgerEntity
	for index := range set.ports {
		if set.ports[index].Value == "192.0.2.60:443" {
			port443 = &set.ports[index]
		}
	}
	if port443 == nil || !strings.Contains(port443.Protocol, "http") || !strings.Contains(port443.Protocol, "https") {
		t.Fatalf("space engine port protocols were not merged: %#v", port443)
	}
	websites := map[string]LedgerEntity{}
	for _, website := range set.websites {
		websites[website.Value] = website
	}
	if website := websites["https://192.0.2.60"]; website.Title != "404 Not Found" || website.Port != 443 || website.Protocol != "https" {
		t.Fatalf("space engine website missing title or port: %#v", website)
	}
	if website := websites["https://admin.example.com"]; website.Title != "Admin" || website.HTTPStatus != 403 {
		t.Fatalf("hunter website missing title or status: %#v", website)
	}
}

func TestWebsiteLedgerGroupsAIAndScannerURLsByOrigin(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/website.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.UnifiedAsset{}, &database.UnifiedAssetObservation{}, &database.UnifiedAssetRelation{}, &database.UnifiedAssetChange{}, &database.VulnScanTask{}); err != nil {
		t.Fatal(err)
	}
	store := assetstore.Store{DB: db}
	now := time.Now().UTC()
	ctx := context.Background()
	for i, input := range []struct {
		url, source string
		attrs       map[string]any
	}{
		{"https://example.com/a", "ai", nil},
		{"https://example.com:443/b?x=1", "findings", nil},
		{"https://example.com/c", "fingerprint", map[string]any{"title": "Example", "status": 200, "fingerprints": []string{"nginx"}}},
		{"http://example.com/a", "ai", nil},
		{"https://example.com:8443/a", "ai", nil},
		{"https://[2001:db8::1]:443/a", "ai", nil},
		{"https://[2001:db8::1]/b", "ai", nil},
		{"ftp://example.com/file", "manual", nil},
	} {
		if _, _, err := store.Upsert(ctx, assetstore.Observation{Type: "url", Value: input.url, Source: input.source, SourceID: fmt.Sprint(i), ObservedAt: now.Add(time.Duration(i) * time.Second), Attributes: input.attrs}); err != nil {
			t.Fatal(err)
		}
	}
	set, err := buildLedgerEntitySet(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.websites) != 4 {
		t.Fatalf("want 4 origins, got %+v", set.websites)
	}
	var site LedgerEntity
	for _, row := range set.websites {
		if row.Value == "https://example.com" {
			site = row
		}
	}
	if site.Title != "Example" || !containsAny(site.Fingerprints, "nginx") || !containsAny(site.Sources, "ai") || !containsAny(site.Sources, "findings") {
		t.Fatalf("lost aggregated fields %+v", site)
	}
	if _, err := store.ArchiveWebsite(ctx, site.AssetID, true); err != nil {
		t.Fatal(err)
	}
	set, err = buildLedgerEntitySet(db, false)
	if err != nil || len(set.websites) != 3 {
		t.Fatalf("archive: %d %v", len(set.websites), err)
	}
	set, err = buildLedgerEntitySet(db, true)
	if err != nil || len(set.websites) != 4 {
		t.Fatalf("include archived: %d %v", len(set.websites), err)
	}
}
