package asset

import (
	"testing"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUnifiedAssetCenterProjectionUsesCanonicalObservations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/asset-center.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.UnifiedAsset{}, &database.UnifiedAssetObservation{}); err != nil {
		t.Fatal(err)
	}
	seen := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	if err := db.Create(&database.VulnScanTask{ID: "task-1", Version: 1, TaskName: "Unified scan", ScanType: "portscan", UpdatedAt: seen}).Error; err != nil {
		t.Fatal(err)
	}
	assets := []database.UnifiedAsset{
		{ID: "asset-domain", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com", DisplayValue: "api.example.com", Status: "active", Confidence: "high", FirstSeenAt: seen, LastSeenAt: seen},
		{ID: "asset-ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", DisplayValue: "192.0.2.10", Status: "active", Confidence: "high", FirstSeenAt: seen, LastSeenAt: seen},
		{ID: "asset-url", ScopeKey: "global", Type: "url", CanonicalValue: "https://api.example.com/login", DisplayValue: "https://api.example.com/login", Status: "active", Confidence: "high", FirstSeenAt: seen, LastSeenAt: seen},
	}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	observations := []database.UnifiedAssetObservation{
		{ID: "obs-domain", AssetID: "asset-domain", ScopeKey: "global", Type: "domain", CanonicalValue: "api.example.com", Source: "subdomain", SourceID: "sub-1", SourceVersion: 1, ObservationKey: "obs-domain-key", Confidence: "high", ObservedAt: seen, Attributes: map[string]any{"taskId": "task-1", "version": 1, "subdomain": "api.example.com"}},
		{ID: "obs-ip", AssetID: "asset-ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", Source: "subdomain", SourceID: "sub-1:ip:192.0.2.10", SourceVersion: 1, ObservationKey: "obs-ip-key", Confidence: "high", ObservedAt: seen, Attributes: map[string]any{"taskId": "task-1", "version": 1, "subdomain": "api.example.com"}},
		{ID: "obs-port", AssetID: "asset-ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", Source: "portscan", SourceID: "port-1", SourceVersion: 1, ObservationKey: "obs-port-key", Confidence: "high", ObservedAt: seen, Attributes: map[string]any{"taskId": "task-1", "version": 1, "port": 443, "protocol": "https", "product": "nginx", "serviceVersion": "1.24"}},
		{ID: "obs-url", AssetID: "asset-url", ScopeKey: "global", Type: "url", CanonicalValue: "https://api.example.com/login", Source: "fingerprint", SourceID: "task-1:1:https://api.example.com/login", SourceVersion: 1, ObservationKey: "obs-url-key", Confidence: "high", ObservedAt: seen, Attributes: map[string]any{"taskId": "task-1", "version": 1, "fingerprints": []string{"nginx"}}},
	}
	if err := db.Create(&observations).Error; err != nil {
		t.Fatal(err)
	}
	items, used, err := collectUnifiedAssetCenterAggregatesWithDB(db)
	if err != nil || !used {
		t.Fatalf("unified projection not used: used=%v err=%v", used, err)
	}
	if len(items) != 4 {
		t.Fatalf("unexpected projected item count: %#v", items)
	}
	var domain, ip, endpoint AssetCenterItem
	for _, item := range items {
		switch {
		case item.Type == "domain":
			domain = item
		case item.Type == "ip":
			ip = item
		case item.Type == "endpoint" && item.Value == "https://192.0.2.10:443":
			endpoint = item
		}
	}
	if domain.Value != "api.example.com" || len(domain.IPs) != 1 || domain.IPs[0] != "192.0.2.10" {
		t.Fatalf("domain relation was not projected: %#v", domain)
	}
	if ip.Value != "192.0.2.10" {
		t.Fatalf("IP was not projected: %#v", ip)
	}
	if endpoint.Value == "" || endpoint.EndpointHost != "192.0.2.10" || endpoint.Port != 443 || endpoint.Service != "nginx 1.24" {
		t.Fatalf("endpoint was not projected: %#v", endpoint)
	}
}

func TestUnifiedAssetInventoryProjectionReadsCanonicalPort(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/asset-inventory.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.UnifiedAsset{}, &database.UnifiedAssetObservation{}); err != nil {
		t.Fatal(err)
	}
	seen := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	if err := db.Create(&database.VulnScanTask{ID: "task-1", Version: 1, TaskName: "Unified port", ScanType: "portscan", UpdatedAt: seen}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.UnifiedAsset{ID: "asset-ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", DisplayValue: "192.0.2.10", Status: "active", Confidence: "high", FirstSeenAt: seen, LastSeenAt: seen}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.UnifiedAssetObservation{ID: "obs-port", AssetID: "asset-ip", ScopeKey: "global", Type: "ip", CanonicalValue: "192.0.2.10", Source: "portscan", SourceID: "port-1", SourceVersion: 1, ObservationKey: "obs-port-key", Confidence: "high", ObservedAt: seen, Attributes: map[string]any{"taskId": "task-1", "version": 1, "port": 443, "protocol": "https", "transport": "tcp", "product": "nginx", "serviceVersion": "1.24", "discoveryStatus": "verified"}}).Error; err != nil {
		t.Fatal(err)
	}
	items, used, err := collectUnifiedAssetInventoryWithDB(db, "")
	if err != nil || !used || len(items) != 1 {
		t.Fatalf("unexpected unified inventory projection: used=%v items=%#v err=%v", used, items, err)
	}
	if len(items[0].Ports) != 1 || items[0].Ports[0].Port != 443 || items[0].OpenPortCount != 1 {
		t.Fatalf("port observation was not projected: %#v", items[0])
	}
}
