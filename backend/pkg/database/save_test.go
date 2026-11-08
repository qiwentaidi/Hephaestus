package database

import (
	"fmt"
	"testing"

	"hephaestus-backend/pkg/types"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestSaveDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&VulnerabilityInfo{}); err != nil {
		t.Fatalf("migrate vulnerability_info: %v", err)
	}
	return db
}

func TestSaveVulnerabilityResultPreservesExistingID(t *testing.T) {
	db := newTestSaveDB(t)

	existing := VulnerabilityInfo{
		TaskID:         "task-1",
		Version:        1,
		TemplateID:     "CVE-2026-39365",
		VulnName:       "Source Map Disclosure",
		Protocol:       "HTTP",
		Severity:       "LOW",
		DisposalStatus: "open",
		VulnURL:        "http://localhost:3003/console/node_modules/.vite/deps/../../../config.production.js.map",
		Extract:        "old extract",
		Description:    "old description",
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("insert existing vulnerability: %v", err)
	}

	SaveVulnerabilityResult(db, "task-1", 1, "webscan", types.VulnerabilityInfo{
		ID:          "CVE-2026-39365",
		Name:        "Source Map Disclosure",
		Type:        "HTTP",
		Severity:    "MEDIUM",
		URL:         existing.VulnURL,
		Extract:     "new extract",
		Description: "new description",
	})

	var stored VulnerabilityInfo
	if err := buildVulnerabilityIdentityQuery(db.Model(&VulnerabilityInfo{}), &existing).First(&stored).Error; err != nil {
		t.Fatalf("load updated vulnerability: %v", err)
	}
	if stored.ID == "" || stored.ID != existing.ID {
		t.Fatalf("ID changed: %q -> %q", existing.ID, stored.ID)
	}
	if stored.Severity != "MEDIUM" {
		t.Fatalf("expected severity MEDIUM, got %s", stored.Severity)
	}
	if stored.Extract != "new extract" {
		t.Fatalf("expected extract updated, got %q", stored.Extract)
	}
	if stored.Description != "new description" {
		t.Fatalf("expected description updated, got %q", stored.Description)
	}

	var count int64
	if err := db.Model(&VulnerabilityInfo{}).Count(&count).Error; err != nil {
		t.Fatalf("count vulnerabilities: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 vulnerability row, got %d", count)
	}
}

func TestSavePortResultPromotesPendingDiscovery(t *testing.T) {
	db := newTestSaveDB(t)
	if err := db.AutoMigrate(&PortScanResult{}); err != nil {
		t.Fatalf("migrate port results: %v", err)
	}
	pending := SavePortResult(db, "scan-1", 1, &types.PortInfo{
		Host: "192.0.2.1", Port: 443, Transport: "tcp", Protocol: "unknown", DiscoveryStatus: "pending",
	})
	if pending == nil || pending.DiscoveryStatus != "pending" {
		t.Fatalf("expected pending candidate, got %#v", pending)
	}
	verified := SavePortResult(db, "scan-1", 1, &types.PortInfo{
		Host: "192.0.2.1", Port: 443, Transport: "tcp", Protocol: "https", Product: "nginx",
	})
	if verified == nil || verified.ID != pending.ID {
		t.Fatalf("expected candidate to be updated in place, got %#v", verified)
	}
	if verified.DiscoveryStatus != "verified" || verified.Protocol != "https" || verified.Product != "nginx" {
		t.Fatalf("expected verified details, got %#v", verified)
	}
	SavePortResult(db, "scan-1", 1, &types.PortInfo{
		Host: "192.0.2.1", Port: 443, Transport: "tcp", Protocol: "unknown", DiscoveryStatus: "pending",
	})
	var stored PortScanResult
	if err := db.First(&stored, verified.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.DiscoveryStatus != "verified" || stored.Protocol != "https" {
		t.Fatalf("retry must not replace verified result, got %#v", stored)
	}
}
