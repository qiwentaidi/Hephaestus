package asset

import (
	"context"
	"strings"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"
)

func TestSyncLegacyImportsDirectoryJSFindingsAndAliases(t *testing.T) {
	store := testStore(t)
	if err := store.DB.AutoMigrate(
		&database.VulnScanTask{},
		&database.PortScanResult{},
		&database.FingerprintInfo{},
		&database.SubdomainScanResult{},
		&database.VulnScanDirResult{},
		&database.VulnScanJSAsset{},
		&database.VulnerabilityInfo{},
		&database.PortScanResultAsset{},
	); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	if err := store.DB.Create(&database.VulnScanTask{ID: "task-1", Version: 1, ScanType: "webscan", TaskName: "Extended", UpdatedAt: created}).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB.Create(&database.VulnScanDirResult{ID: "dir-1", TaskID: "task-1", Version: 1, URL: "https://app.example.com/admin", Status: 200, CreatedAt: created}).Error; err != nil {
		t.Fatal(err)
	}
	const scriptFragment = "username='+Ext.myCtx.LoginInfo.loginname;j+='"
	const dottedSecret = "auth.token.invalid"
	const webhookSecret = "https://hooks.example.net/services/example-secret"
	for _, asset := range []database.VulnScanJSAsset{
		{ID: "js-1", Type: "apiRoot", Value: "https://api.example.com"},
		{ID: "js-2", Type: "sensitive", Value: scriptFragment},
		{ID: "js-3", Type: "sensitive", Value: dottedSecret},
		{ID: "js-4", Type: "sensitive", Value: webhookSecret},
		{ID: "js-5", Type: "ipUrl", Value: "192.0.2.21"},
		{ID: "js-6", Type: "domain", Value: "legacy.example.com"},
		{ID: "js-7", Type: "apiRoute", Value: "/api/v1/users"},
	} {
		asset.TaskID, asset.Version, asset.CreatedAt = "task-1", 1, created
		if err := store.DB.Create(&asset).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DB.Create(&database.VulnerabilityInfo{TaskID: "task-1", Version: 1, VulnURL: "https://app.example.com/login", Severity: "high", VulnName: "SQL injection"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB.Create(&database.PortScanResultAsset{TaskID: "task-1", Version: 1, ResultID: 7, IP: "192.0.2.20", Port: 443, Transport: "tcp", Domain: "example.com", Subdomain: "app.example.com", CreatedAt: created}).Error; err != nil {
		t.Fatal(err)
	}
	summary, err := store.SyncLegacy(context.Background(), LegacySyncOptions{TaskID: "task-1", Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if summary.BySource["dirscan"] == 0 || summary.BySource["jsscan"] == 0 || summary.BySource["webscan-vulnerability"] == 0 || summary.BySource["portscan"] == 0 {
		t.Fatalf("new sources missing from summary: %#v", summary.BySource)
	}
	items, _, err := store.List(context.Background(), ListFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]database.UnifiedAsset{}
	for _, item := range items {
		values[item.CanonicalValue] = item
	}
	for _, unwanted := range []string{strings.ToLower(scriptFragment), dottedSecret, webhookSecret, "hooks.example.net", "/api/v1/users"} {
		if _, exists := values[unwanted]; exists {
			t.Errorf("unexpected JS scan value in network catalog: %q", unwanted)
		}
	}
	var sensitiveCount int64
	if err := store.DB.Model(&database.VulnScanJSAsset{}).Where("type = ?", "sensitive").Count(&sensitiveCount).Error; err != nil || sensitiveCount != 3 {
		t.Fatalf("original JS scan evidence was not retained: count=%d err=%v", sensitiveCount, err)
	}
	for _, want := range []string{"https://app.example.com/admin", "https://api.example.com", "https://app.example.com/login", "app.example.com", "192.0.2.20", "192.0.2.21", "legacy.example.com"} {
		if _, ok := values[want]; !ok {
			t.Fatalf("expected %s in unified catalog, got %#v", want, values)
		}
	}
	var observations []database.UnifiedAssetObservation
	if err := store.DB.Where("source IN ?", []string{"dirscan", "jsscan", "webscan-vulnerability"}).Find(&observations).Error; err != nil {
		t.Fatal(err)
	}
	if len(observations) == 0 {
		t.Fatal("new observations were not persisted")
	}
	for _, observation := range observations {
		if observation.TaskID != "task-1" || observation.TaskVersion != 1 || observation.Origin != "scan" {
			t.Fatalf("new observation lost task provenance: %#v", observation)
		}
	}
	var aliases []database.UnifiedAssetObservation
	if err := store.DB.Where("source = ? AND canonical_value = ?", "portscan", "app.example.com").Find(&aliases).Error; err != nil {
		t.Fatal(err)
	}
	if len(aliases) == 0 || aliases[0].TaskID != "task-1" {
		t.Fatalf("port alias host was not imported with provenance: %#v", aliases)
	}
	var relations []database.UnifiedAssetRelation
	if err := store.DB.Where("relation_type = ?", "resolves_to").Find(&relations).Error; err != nil {
		t.Fatal(err)
	}
	if len(relations) == 0 {
		t.Fatal("port alias resolve relation was not created")
	}
}
