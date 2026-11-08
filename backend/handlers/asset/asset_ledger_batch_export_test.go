package asset

import (
	"bytes"
	"context"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newLedgerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ledger.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&database.UnifiedAsset{},
		&database.UnifiedAssetObservation{},
		&database.UnifiedAssetChange{},
		&database.UnifiedAssetRelation{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func parseLedgerCSV(t *testing.T, content []byte) [][]string {
	t.Helper()
	if !bytes.HasPrefix(content, []byte("\xEF\xBB\xBF")) {
		t.Fatalf("CSV export is missing the UTF-8 BOM")
	}
	reader := csv.NewReader(bytes.NewReader(content[3:]))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func TestBuildLedgerEntityCSVWritesLocalizedHeadersAndRows(t *testing.T) {
	now := time.Date(2026, 10, 30, 9, 30, 0, 0, time.UTC)
	entities := []LedgerEntity{
		{
			ID: "root:example.com", Category: "root", Value: "example.com", Name: "Example, Inc.",
			RootDomain: "example.com", Host: "example.com", IPs: []string{"192.0.2.10", "192.0.2.11"},
			SubdomainCount: 3, IPCount: 2, PortCount: 4, WebsiteCount: 1,
			Sources: []string{"portscan", "subdomain"}, Status: "active", Verification: "observed",
			FirstSeenAt: now, LastSeenAt: now,
		},
		{
			ID: "website:example", Category: "website", Value: "https://example.com", Host: "example.com",
			Port: 443, Transport: "tcp", Protocol: "https", Title: `Admin "panel"`, HTTPStatus: 200,
			Fingerprints: []string{"nginx", "Vue"}, HighRiskFingerprints: []string{"GitLab"},
			Sources: []string{"fingerprint"}, Status: "active", Verification: "verified",
			Archived: true, FirstSeenAt: now,
		},
	}

	content, err := buildLedgerEntityCSV(entities, "zh")
	if err != nil {
		t.Fatal(err)
	}
	records := parseLedgerCSV(t, content)
	if len(records) != 3 {
		t.Fatalf("want header plus two rows, got %d records", len(records))
	}
	if len(records[0]) != len(ledgerCSVHeadersZH) {
		t.Fatalf("header column count = %d, want %d", len(records[0]), len(ledgerCSVHeadersZH))
	}
	if records[0][0] != "类型" || records[0][21] != "是否归档" {
		t.Fatalf("unexpected zh header: %#v", records[0])
	}
	for index, record := range records {
		if len(record) != len(ledgerCSVHeadersZH) {
			t.Fatalf("row %d has %d columns, want %d", index, len(record), len(ledgerCSVHeadersZH))
		}
	}
	root := records[1]
	if root[0] != "root" || root[1] != "example.com" || root[2] != "Example, Inc." {
		t.Fatalf("unexpected root row: %#v", root)
	}
	if root[5] != "192.0.2.10;192.0.2.11" || root[6] != "" {
		t.Fatalf("unexpected root IP or port columns: %#v", root)
	}
	if root[14] != "3" || root[15] != "2" || root[16] != "4" || root[17] != "1" {
		t.Fatalf("unexpected root counters: %#v", root)
	}
	if root[18] != "portscan;subdomain" || root[21] != "false" || root[22] != "2026-10-30T09:30:00Z" {
		t.Fatalf("unexpected root metadata: %#v", root)
	}
	website := records[2]
	if website[0] != "website" || website[6] != "443" || website[11] != "200" {
		t.Fatalf("unexpected website row: %#v", website)
	}
	if website[12] != "nginx;Vue" || website[13] != "GitLab" || website[21] != "true" {
		t.Fatalf("unexpected website fingerprints or archive state: %#v", website)
	}

	english, err := buildLedgerEntityCSV(entities[:1], "en")
	if err != nil {
		t.Fatal(err)
	}
	englishRecords := parseLedgerCSV(t, english)
	if englishRecords[0][0] != "Category" || englishRecords[0][21] != "Archived" {
		t.Fatalf("unexpected en header: %#v", englishRecords[0])
	}
}

func TestFilterLedgerEntitiesCombinesKeywordStatusAndSource(t *testing.T) {
	entities := []LedgerEntity{
		{ID: "root:alpha", Category: "root", Value: "alpha.example.com", DisplayValue: "alpha.example.com", Sources: []string{"subdomain"}, Status: "active", Verification: "observed"},
		{ID: "root:beta", Category: "root", Value: "beta.example.com", DisplayValue: "beta.example.com", Sources: []string{"manual"}, Status: "retired", Verification: "declared"},
	}
	cases := []struct {
		name   string
		filter assetstore.LedgerFilter
		want   string
	}{
		{"keyword is case insensitive", assetstore.LedgerFilter{Keyword: "ALPHA"}, "alpha.example.com"},
		{"scan source groups scanner sources", assetstore.LedgerFilter{Source: "scan"}, "alpha.example.com"},
		{"status filter", assetstore.LedgerFilter{Status: "retired"}, "beta.example.com"},
		{"verification filter", assetstore.LedgerFilter{Verification: "declared"}, "beta.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterLedgerEntities(entities, tc.filter)
			if len(got) != 1 || got[0].Value != tc.want {
				t.Fatalf("filterLedgerEntities(%#v) = %#v, want %q", tc.filter, got, tc.want)
			}
		})
	}
	if got := filterLedgerEntities(entities, assetstore.LedgerFilter{}); len(got) != 2 {
		t.Fatalf("empty filter should keep every row, got %#v", got)
	}
}

func TestSelectLedgerEntitiesResolvesSelectionAgainstEntitySet(t *testing.T) {
	db := newLedgerTestDB(t)
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	archivedAt := now
	assets := []database.UnifiedAsset{
		{ID: "root", ScopeKey: "global", Type: "domain", CanonicalValue: "example.com", DisplayValue: "example.com", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "sub", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com", DisplayValue: "api.example.com", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "url", ScopeKey: "global", Type: "url", CanonicalValue: "https://api.example.com/login", DisplayValue: "https://api.example.com/login", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now, ArchivedAt: &archivedAt},
		{ID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", DisplayValue: "192.0.2.10", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
	}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	set, err := buildLedgerEntitySet(db, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.websites) != 1 || !set.websites[0].Archived {
		t.Fatalf("expected one archived website row, got %#v", set.websites)
	}
	items := []ledgerEntitySelection{
		{ID: set.subdomains[0].ID, Category: "subdomain"},
		{ID: set.websites[0].ID, Category: "website"},
		{ID: set.subdomains[0].ID, Category: "subdomain"},
		{ID: "root:missing.example.com", Category: "root"},
	}
	selected := selectLedgerEntities(set, items)
	if len(selected) != 2 {
		t.Fatalf("want two resolved rows, got %#v", selected)
	}
	if selected[0].ID != set.subdomains[0].ID || selected[1].ID != set.websites[0].ID {
		t.Fatalf("selection order or identity changed: %#v", selected)
	}
}

func TestApplyLedgerEntityArchiveArchivesWebsiteOriginAndRestores(t *testing.T) {
	db := newLedgerTestDB(t)
	store := assetstore.Store{DB: db}
	now := time.Now().UTC()
	assets := []database.UnifiedAsset{
		{ID: "root", ScopeKey: "global", Type: "domain", CanonicalValue: "example.com", DisplayValue: "example.com", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "sub", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com", DisplayValue: "api.example.com", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "url-a", ScopeKey: "global", Type: "url", CanonicalValue: "https://example.com/a", DisplayValue: "https://example.com/a", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "url-b", ScopeKey: "global", Type: "url", CanonicalValue: "https://example.com/b", DisplayValue: "https://example.com/b", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", DisplayValue: "192.0.2.10", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
	}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	items := []ledgerEntitySelection{
		{ID: "root:example.com", Category: "root", AssetID: "root", Value: "example.com"},
		{ID: "website:example", Category: "website", AssetID: "url-a", Value: "https://example.com"},
		{ID: "port:example:443", Category: "port", AssetID: "root"},
		{ID: "root:aggregate.example.com", Category: "root", Value: "aggregate.example.com"},
		{ID: "ip:198.51.100.9", Category: "ip", AssetID: "missing", Value: "198.51.100.9"},
	}
	result, err := applyLedgerEntityArchive(context.Background(), store, items, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "archive" || result.Updated != 2 || result.Skipped != 1 || len(result.Failed) != 1 {
		t.Fatalf("unexpected archive result: %#v", result)
	}
	if result.Failed[0].ID != "198.51.100.9" || result.Failed[0].Error == "" {
		t.Fatalf("unexpected failure detail: %#v", result.Failed)
	}
	archived := func(id string) bool {
		var assetRow database.UnifiedAsset
		if err := db.First(&assetRow, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		return assetRow.ArchivedAt != nil
	}
	for _, id := range []string{"root", "url-a", "url-b"} {
		if !archived(id) {
			t.Fatalf("asset %q should be archived", id)
		}
	}
	for _, id := range []string{"sub", "ip"} {
		if archived(id) {
			t.Fatalf("asset %q should stay active", id)
		}
	}

	restore, err := applyLedgerEntityArchive(context.Background(), store, items[:2], false)
	if err != nil {
		t.Fatal(err)
	}
	if restore.Action != "restore" || restore.Updated != 2 {
		t.Fatalf("unexpected restore result: %#v", restore)
	}
	for _, id := range []string{"root", "url-a", "url-b"} {
		if archived(id) {
			t.Fatalf("asset %q should be restored", id)
		}
	}
}

func TestWriteLedgerEntityCSVServesAttachment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 10, 30, 9, 30, 0, 0, time.UTC)
	entities := []LedgerEntity{
		{ID: "root:example.com", Category: "root", Value: "example.com", RootDomain: "example.com", SubdomainCount: 2, Sources: []string{"subdomain"}, Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assets/ledger/entities/export?category=root", nil)

	writeLedgerEntityCSV(context, "root", entities, "zh")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/csv") {
		t.Fatalf("content type = %q, want text/csv", contentType)
	}
	disposition := recorder.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, "asset-ledger-root-") || !strings.Contains(disposition, "filename*=UTF-8''") {
		t.Fatalf("unexpected content disposition: %q", disposition)
	}
	if count := recorder.Header().Get("X-Total-Count"); count != "1" {
		t.Fatalf("X-Total-Count = %q, want 1", count)
	}
	records := parseLedgerCSV(t, recorder.Body.Bytes())
	if len(records) != 2 || records[1][1] != "example.com" {
		t.Fatalf("unexpected CSV records: %#v", records)
	}

	emptyRecorder := httptest.NewRecorder()
	emptyContext, _ := gin.CreateTestContext(emptyRecorder)
	emptyContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assets/ledger/entities/export?category=root", nil)
	writeLedgerEntityCSV(emptyContext, "root", nil, "zh")
	if emptyRecorder.Code != http.StatusBadRequest {
		t.Fatalf("empty export status = %d, want 400", emptyRecorder.Code)
	}
	if !strings.Contains(emptyRecorder.Body.String(), "没有可导出的资产") {
		t.Fatalf("unexpected empty export body: %s", emptyRecorder.Body.String())
	}
}

func TestLedgerExportCategoriesCoversAllTabs(t *testing.T) {
	categories, label, ok := ledgerExportCategories("ALL")
	if !ok || label != "all" {
		t.Fatalf("all scope = %#v %q %v", categories, label, ok)
	}
	want := []LedgerEntityCategory{LedgerEntityRoot, LedgerEntityIP, LedgerEntitySubdomain, LedgerEntityPort, LedgerEntityWebsite}
	if len(categories) != len(want) {
		t.Fatalf("all scope categories = %#v, want %#v", categories, want)
	}
	for index := range want {
		if categories[index] != want[index] {
			t.Fatalf("all scope order = %#v, want %#v", categories, want)
		}
	}
	single, label, ok := ledgerExportCategories("ip")
	if !ok || label != "ip" || len(single) != 1 || single[0] != LedgerEntityIP {
		t.Fatalf("ip scope = %#v %q %v", single, label, ok)
	}
	if _, _, ok := ledgerExportCategories("bogus"); ok {
		t.Fatal("unknown category should be rejected")
	}
	defaultScope, _, ok := ledgerExportCategories("")
	if !ok || len(defaultScope) != 1 || defaultScope[0] != LedgerEntityRoot {
		t.Fatalf("default scope = %#v", defaultScope)
	}
}

func TestFilteredCountsMatchActiveFilters(t *testing.T) {
	db := newLedgerTestDB(t)
	now := time.Date(2026, 10, 30, 11, 0, 0, 0, time.UTC)
	assets := []database.UnifiedAsset{
		{ID: "alpha", ScopeKey: "global", Type: "domain", CanonicalValue: "alpha.com", DisplayValue: "alpha.com", Status: "active", Verification: "observed", FirstSeenAt: now, LastSeenAt: now},
		{ID: "beta", ScopeKey: "global", Type: "domain", CanonicalValue: "beta.net", DisplayValue: "beta.net", Status: "retired", Verification: "declared", FirstSeenAt: now, LastSeenAt: now},
	}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	set, err := buildLedgerEntitySet(db, false)
	if err != nil {
		t.Fatal(err)
	}
	base := set.counts()
	if base.Root != 2 {
		t.Fatalf("unexpected base counts: %#v", base)
	}
	byKeyword := set.filteredCounts(assetstore.LedgerFilter{Keyword: "alpha"})
	if byKeyword.Root != 1 || byKeyword.IP != 0 || byKeyword.Subdomain != 0 || byKeyword.Port != 0 || byKeyword.Website != 0 {
		t.Fatalf("unexpected keyword filtered counts: %#v", byKeyword)
	}
	byStatus := set.filteredCounts(assetstore.LedgerFilter{Status: "retired"})
	if byStatus.Root != 1 {
		t.Fatalf("unexpected status filtered counts: %#v", byStatus)
	}
	empty := set.filteredCounts(assetstore.LedgerFilter{Keyword: "no-such-asset"})
	if empty.Root != 0 || empty.IP != 0 || empty.Subdomain != 0 || empty.Port != 0 || empty.Website != 0 {
		t.Fatalf("unexpected empty filtered counts: %#v", empty)
	}
}
