package findings

import (
	"fmt"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestFindingsDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
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
	return db
}

func TestIngestAttributionAndProjection(t *testing.T) {
	db := newTestFindingsDB(t)
	now := time.Now().UTC()

	web, err := Ingest(db, Input{
		Source:      "webscan",
		SourceID:    "web-1",
		TaskID:      "task-1",
		TemplateID:  "CVE-2026-0001",
		Title:       "SQL Injection",
		Target:      "https://api.example.com/login",
		Severity:    "high",
		Status:      "open",
		ObservedAt:  now,
		ResolvedIPs: []string{"203.0.113.10"},
	})
	if err != nil {
		t.Fatalf("ingest web finding: %v", err)
	}
	if web.Category != CategoryWeb || web.TargetType != "url" {
		t.Fatalf("unexpected finding: %#v", web)
	}
	var primary database.UnifiedAsset
	if err := db.Where("id = ?", web.PrimaryAssetID).First(&primary).Error; err != nil {
		t.Fatalf("load primary asset: %v", err)
	}
	if primary.Type != "url" {
		t.Fatalf("expected URL primary, got %#v", primary)
	}

	ipRisk := loadRisk(t, db, "ip", "203.0.113.10")
	if ipRisk.InheritedHigh != 1 || ipRisk.DirectHigh != 0 {
		t.Fatalf("expected inherited high on IP, got %#v", ipRisk)
	}
	domainRisk := loadRisk(t, db, "domain", "api.example.com")
	if domainRisk.DirectHigh != 1 || domainRisk.InheritedHigh != 0 {
		t.Fatalf("expected direct high on host domain, got %#v", domainRisk)
	}
	urlRisk := loadRisk(t, db, "url", "https://api.example.com/login")
	if urlRisk.DirectHigh != 1 {
		t.Fatalf("expected direct high on URL, got %#v", urlRisk)
	}

	// Re-observation must update the existing finding rather than add a second
	// one to the projection.
	if _, err := Ingest(db, Input{
		Source:      "webscan",
		SourceID:    "web-1",
		TaskID:      "task-1",
		TemplateID:  "CVE-2026-0001",
		Title:       "SQL Injection",
		Target:      "https://api.example.com/login",
		Severity:    "high",
		Status:      "open",
		ObservedAt:  now.Add(time.Hour),
		ResolvedIPs: []string{"203.0.113.10"},
	}); err != nil {
		t.Fatalf("re-ingest web finding: %v", err)
	}
	var findingCount int64
	if err := db.Model(&database.SecurityFinding{}).Count(&findingCount).Error; err != nil {
		t.Fatalf("count findings: %v", err)
	}
	if findingCount != 1 {
		t.Fatalf("expected one deduplicated finding, got %d", findingCount)
	}
	if risk := loadRisk(t, db, "ip", "203.0.113.10"); risk.InheritedHigh != 1 {
		t.Fatalf("expected inherited high to remain 1, got %#v", risk)
	}
}

func TestServiceFindingIsDirectOnIP(t *testing.T) {
	db := newTestFindingsDB(t)
	now := time.Now().UTC()
	if _, err := Ingest(db, Input{
		Source:     "webscan",
		SourceID:   "svc-1",
		TaskID:     "task-2",
		Title:      "Redis 未授权访问",
		Target:     "http://203.0.113.20:6379",
		Severity:   "critical",
		Status:     "open",
		ObservedAt: now,
	}); err != nil {
		t.Fatalf("ingest service finding: %v", err)
	}
	ipRisk := loadRisk(t, db, "ip", "203.0.113.20")
	if ipRisk.DirectCritical != 1 || ipRisk.InheritedCritical != 0 {
		t.Fatalf("expected direct critical on IP, got %#v", ipRisk)
	}
	serviceRisk := loadRisk(t, db, "service", "203.0.113.20:6379")
	if serviceRisk.DirectCritical != 1 {
		t.Fatalf("expected direct critical on service, got %#v", serviceRisk)
	}

	views, err := FindingsForAsset(db, "ip", "203.0.113.20")
	if err != nil {
		t.Fatalf("list findings: %v", err)
	}
	if len(views) != 1 || !views[0].Direct {
		t.Fatalf("expected one direct finding for the IP, got %#v", views)
	}

	if err := UpdateStatusBySourceID(db, "webscan", "svc-1", "fixed"); err != nil {
		t.Fatalf("update status: %v", err)
	}
	if risk := loadRisk(t, db, "ip", "203.0.113.20"); risk.DirectCritical != 0 {
		t.Fatalf("expected fixed finding to leave the projection, got %#v", risk)
	}
}

func TestSummarizeRiskCountsPrimaryAssets(t *testing.T) {
	db := newTestFindingsDB(t)
	now := time.Now().UTC()
	for index, input := range []Input{
		{Source: "webscan", SourceID: "a", TemplateID: "T-1", Title: "SQL Injection", Target: "https://a.example.com/login", Severity: "high", ObservedAt: now, ResolvedIPs: []string{"203.0.113.30"}},
		{Source: "jsscan", SourceID: "b", TemplateID: "T-2", Title: "敏感信息泄露", Target: "https://a.example.com/app.js", Severity: "medium", ObservedAt: now, ResolvedIPs: []string{"203.0.113.30"}},
		{Source: "webscan", SourceID: "c", TemplateID: "T-3", Title: "Redis 未授权访问", Target: "http://203.0.113.30:6379", Severity: "critical", ObservedAt: now},
		{Source: "webscan", SourceID: "d", TemplateID: "T-4", Title: "内网问题", Target: "http://192.168.1.10/admin", Severity: "critical", ObservedAt: now},
	} {
		input.ScopeKey = "global"
		if _, err := Ingest(db, input); err != nil {
			t.Fatalf("ingest %d: %v", index, err)
		}
	}
	summary, err := SummarizeRisk(db)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Total != 3 || summary.URL != 2 || summary.Service != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.Critical != 1 || summary.High != 1 || summary.Medium != 1 {
		t.Fatalf("unexpected severity summary: %#v", summary)
	}
}

func loadRisk(t *testing.T, db *gorm.DB, assetType, value string) database.AssetRisk {
	t.Helper()
	var asset database.UnifiedAsset
	if err := db.Where("type = ? AND canonical_value = ?", assetType, value).First(&asset).Error; err != nil {
		t.Fatalf("load asset %s/%s: %v", assetType, value, err)
	}
	var risk database.AssetRisk
	if err := db.Where("asset_id = ?", asset.ID).First(&risk).Error; err != nil && err != gorm.ErrRecordNotFound {
		t.Fatalf("load risk %s/%s: %v", assetType, value, err)
	}
	return risk
}

func TestCertificateFindingGetsCertificatePrimary(t *testing.T) {
	db := newTestFindingsDB(t)
	fingerprint := "ab" + "cd" + "ef" + "01" + "23" + "45" + "67" + "89" + "ab" + "cd" + "ef" + "01" + "23" + "45" + "67" + "89" +
		"ab" + "cd" + "ef" + "01" + "23" + "45" + "67" + "89" + "ab" + "cd" + "ef" + "01" + "23" + "45" + "67" + "89"
	finding, err := Ingest(db, Input{
		Source:      "ai",
		SourceID:    "cert-1",
		Title:       "TLS 证书过期",
		Target:      "https://api.example.com",
		Severity:    "high",
		ObservedAt:  time.Now().UTC(),
		Evidence:    "sha256:" + fingerprint,
		ResolvedIPs: []string{"203.0.113.40"},
	})
	if err != nil {
		t.Fatalf("ingest certificate finding: %v", err)
	}
	if finding.Category != CategoryCertificate || finding.PrimaryAssetID == "" {
		t.Fatalf("unexpected certificate finding: %#v", finding)
	}
	var primary database.UnifiedAsset
	if err := db.Where("id = ?", finding.PrimaryAssetID).First(&primary).Error; err != nil {
		t.Fatalf("load primary: %v", err)
	}
	if primary.Type != "certificate" || primary.CanonicalValue != fingerprint {
		t.Fatalf("expected certificate asset, got %#v", primary)
	}
	if risk := loadRisk(t, db, "certificate", fingerprint); risk.DirectHigh != 1 {
		t.Fatalf("expected direct high on certificate, got %#v", risk)
	}
	if risk := loadRisk(t, db, "ip", "203.0.113.40"); risk.InheritedHigh != 1 {
		t.Fatalf("expected inherited high on serving IP, got %#v", risk)
	}
	summary, err := SummarizeRisk(db)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Certificate != 1 {
		t.Fatalf("expected certificate in summary, got %#v", summary)
	}
}

func TestCDNEdgeDoesNotBlameResolvedIP(t *testing.T) {
	db := newTestFindingsDB(t)
	now := time.Now().UTC()
	if _, err := Ingest(db, Input{
		Source:      "jsscan",
		SourceID:    "cdn-1",
		Title:       "敏感信息泄露",
		Target:      "https://cdn.example.com/app.js",
		Severity:    "high",
		ObservedAt:  now,
		ResolvedIPs: []string{"203.0.113.50"},
		CDNEdge:     true,
	}); err != nil {
		t.Fatalf("ingest CDN finding: %v", err)
	}
	var ipAsset database.UnifiedAsset
	if err := db.Where("type = ? AND canonical_value = ?", "ip", "203.0.113.50").First(&ipAsset).Error; err != nil {
		t.Fatalf("expected resolved IP asset to be recorded: %v", err)
	}
	var risk database.AssetRisk
	if err := db.Where("asset_id = ?", ipAsset.ID).First(&risk).Error; err == nil {
		t.Fatalf("CDN edge IP must not carry inherited risk: %#v", risk)
	} else if err != gorm.ErrRecordNotFound {
		t.Fatalf("load CDN IP risk: %v", err)
	}
	if risk := loadRisk(t, db, "url", "https://cdn.example.com/app.js"); risk.DirectHigh != 1 {
		t.Fatalf("expected URL to remain a direct risk, got %#v", risk)
	}
}

func TestIsCDNCNAME(t *testing.T) {
	for _, value := range []string{
		"cdnweb.example.com.m.alikunlun.com",
		"foo.cloudfront.net",
		"bar.saaswaf.com",
	} {
		if !isCDNCNAME(value) {
			t.Fatalf("expected %q to be a CDN CNAME", value)
		}
	}
	if isCDNCNAME("origin.example.com") {
		t.Fatal("origin CNAME should not be treated as CDN")
	}
}

func TestIngestRejectsCredentialTargets(t *testing.T) {
	db := newTestFindingsDB(t)
	for _, input := range []Input{
		{Source: "ai", SourceID: "cred-1", Title: "阿里云 AccessKey 泄露", Target: "aliyuncs.com", Severity: "high"},
		{Source: "ai", SourceID: "cred-2", Title: "凭证泄露", Target: "accesskey (LTAI5t9au6example)", Severity: "high"},
		{Source: "ai", SourceID: "cred-3", Title: "API Key 泄露", Target: "https://accesskey", Severity: "high"},
	} {
		input.ObservedAt = time.Now().UTC()
		if _, err := Ingest(db, input); err == nil {
			t.Fatalf("expected credential target to be rejected: %#v", input)
		}
	}
	var assetCount int64
	if err := db.Model(&database.UnifiedAsset{}).Count(&assetCount).Error; err != nil {
		t.Fatalf("count assets: %v", err)
	}
	if assetCount != 0 {
		t.Fatalf("credential findings must not create assets, got %d", assetCount)
	}
}

func TestCredentialFindingOnRealURLKeepsAttribution(t *testing.T) {
	db := newTestFindingsDB(t)
	finding, err := Ingest(db, Input{
		Source:      "ai",
		SourceID:    "cred-4",
		Title:       "AccessKey 泄露",
		Target:      "https://app.example.com/config",
		Severity:    "high",
		ObservedAt:  time.Now().UTC(),
		ResolvedIPs: []string{"203.0.113.60"},
	})
	if err != nil {
		t.Fatalf("real URL credential finding should be accepted: %v", err)
	}
	var primary database.UnifiedAsset
	if err := db.Where("id = ?", finding.PrimaryAssetID).First(&primary).Error; err != nil {
		t.Fatalf("load primary: %v", err)
	}
	if primary.Type != "url" {
		t.Fatalf("expected URL primary, got %#v", primary)
	}
}

func TestIsValidDomainNameRejectsCredentialLikeValues(t *testing.T) {
	if IsValidDomainName("accesskey") {
		t.Fatal("single-label credential value must not be a domain")
	}
	if IsValidDomainName("accesskey (LTAI5t9au6example)") {
		t.Fatal("free-form credential value must not be a domain")
	}
	if !IsValidDomainName("app.example.com") {
		t.Fatal("valid domain rejected")
	}
}
