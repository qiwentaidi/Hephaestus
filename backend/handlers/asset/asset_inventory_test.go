package asset

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"path/filepath"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"hephaestus-backend/pkg/database"
)

// assetTestDB 为 asset 包测试提供隔离的 sqlite 库。
func assetTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "asset.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.CombinationTask{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func inventoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := assetTestDB(t)
	if err := db.AutoMigrate(&database.PortScanResult{}, &database.PortScanResultAsset{}, &database.FingerprintInfo{}); err != nil {
		t.Fatal(err)
	}
	return db
}
func inventoryFind(t *testing.T, items []AssetInventoryItem, value string) AssetInventoryItem {
	t.Helper()
	for _, item := range items {
		if item.Value == value {
			return item
		}
	}
	t.Fatalf("missing asset %s: %#v", value, items)
	return AssetInventoryItem{}
}
func TestAssetInventoryHistoricalVersionsAndLiveResults(t *testing.T) {
	db := inventoryTestDB(t)
	before := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	later := before.Add(24 * time.Hour)
	for _, task := range []database.VulnScanTask{{ID: "port", Version: 1, TaskName: "Historic port", ScanType: "portscan"}, {ID: "port", Version: 2, TaskName: "Current port", ScanType: "portscan"}, {ID: "combo", Version: 1, TaskName: "Combined assessment", ScanType: "combination", ScanMode: "agent"}} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, result := range []database.PortScanResult{{TaskID: "port", Version: 1, Host: "EXAMPLE.com.", Port: 443, Transport: "tcp", Protocol: "https", Product: "nginx", ServiceVersion: "1.24", DiscoveryStatus: "verified", CreatedAt: before}, {TaskID: "port", Version: 2, Host: "example.com", Port: 443, Transport: "tcp", DiscoveryStatus: "pending", CreatedAt: later}, {TaskID: "port", Version: 2, Host: "example.com", Port: 443, Transport: "udp", Protocol: "quic", DiscoveryStatus: "pending", CreatedAt: later}} {
		if err := db.Create(&result).Error; err != nil {
			t.Fatal(err)
		}
	}
	fingerprint := database.FingerprintInfo{TaskID: "combo", Version: 1, URL: "https://example.com/login", Status: 200, Fingerprints: `["GitLab", "GitLab"]`, HighRiskFingerprints: `["Rails"]`, Title: "Sign in", CreatedAt: later}
	if err := db.Create(&fingerprint).Error; err != nil {
		t.Fatal(err)
	}
	items, err := collectAssetInventory(db, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("host not merged: %#v", items)
	}
	item := items[0]
	if item.ObservationCount != 4 || item.TaskCount != 2 || len(item.Sources) != 3 || len(item.Observations) != 4 {
		t.Fatalf("lost versions or provenance: %#v", item)
	}
	if len(item.Ports) != 2 || item.OpenPortCount != 1 || item.Ports[0].DiscoveryStatus != "verified" || item.Ports[1].DiscoveryStatus != "pending" {
		t.Fatalf("TCP/UDP or verification incorrect: %#v", item.Ports)
	}
	if !reflect.DeepEqual(item.Fingerprints, []string{"GitLab", "Rails"}) {
		t.Fatalf("fingerprints incorrect: %#v", item.Fingerprints)
	}
	if !reflect.DeepEqual(item.Ports[0].Services, []string{"nginx 1.24"}) {
		t.Fatalf("port service lost: %#v", item.Ports[0])
	}
	if !item.FirstSeenAt.Equal(before) || !item.LastSeenAt.Equal(later) {
		t.Fatalf("wrong observation dates: %#v", item)
	}
	// A later task result is visible immediately, without explicit import/backfill.
	if err := db.Create(&database.FingerprintInfo{TaskID: "combo", Version: 1, URL: "http://example.com:8080/", Status: 200, Fingerprints: `["Nginx"]`, CreatedAt: later.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	fresh, err := collectAssetInventory(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if fresh[0].ObservationCount != 5 || fresh[0].OpenPortCount != 2 || len(fresh[0].Observations) != 0 {
		t.Fatalf("new result not integrated or history leaked into list: %#v", fresh[0])
	}
	repeat, err := collectAssetInventory(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fresh, repeat) {
		t.Fatal("repeated query changed inventory")
	}
}
func TestAssetInventoryDomainAliasesIPv6AndFailedProbe(t *testing.T) {
	db := inventoryTestDB(t)
	now := time.Now().UTC()
	port := database.PortScanResult{TaskID: "scan", Version: 1, Host: "2001:0db8:0:0:0:0:0:1", Port: 22, Transport: "tcp", Protocol: "ssh", DiscoveryStatus: "verified", CreatedAt: now}
	if err := db.Create(&port).Error; err != nil {
		t.Fatal(err)
	}
	alias := database.PortScanResultAsset{TaskID: "scan", Version: 1, ResultID: port.ID, IP: "2001:db8::1", Subdomain: "SSH.Example.com.", Port: 22, Transport: "tcp", CreatedAt: now}
	for range 2 {
		if err := db.Create(&alias).Error; err != nil {
			t.Fatal(err)
		}
		alias.ID = 0
	}
	for _, fingerprint := range []database.FingerprintInfo{{TaskID: "scan", Version: 1, URL: "https://[2001:db8::1]/", Status: 200, Fingerprints: `["Nginx"]`, CreatedAt: now}, {TaskID: "failed", Version: 1, URL: "http://failed.example.com/", Status: 0, CreatedAt: now}} {
		if err := db.Create(&fingerprint).Error; err != nil {
			t.Fatal(err)
		}
	}
	items, err := collectAssetInventory(db, "ssh.example.com")
	if err != nil {
		t.Fatal(err)
	}
	ip := inventoryFind(t, items, "2001:db8::1")
	if ip.Type != "ip" || ip.ObservationCount != 2 || ip.OpenPortCount != 2 {
		t.Fatalf("IPv6 not normalized: %#v", ip)
	}
	domain := inventoryFind(t, items, "ssh.example.com")
	if domain.ObservationCount != 1 || !reflect.DeepEqual(domain.IPs, []string{"2001:db8::1"}) || len(domain.Observations) != 1 {
		t.Fatalf("domain context lost/duplicated: %#v", domain)
	}
	failed := inventoryFind(t, items, "failed.example.com")
	if failed.OpenPortCount != 0 || failed.Ports[0].DiscoveryStatus != "pending" {
		t.Fatalf("failed probe incorrectly counted open: %#v", failed)
	}
}
func TestAssetInventoryFiltersPaginationAndValidation(t *testing.T) {
	item := AssetInventoryItem{Type: "domain", Value: "example.com", IPs: []string{"192.0.2.1"}, Fingerprints: []string{"GitLab"}, Sources: []AssetInventorySource{{TaskID: "task", TaskName: "Historical scan"}}, Ports: []AssetInventoryPort{{Port: 443, Transport: "tcp", Protocols: []string{"https"}, Services: []string{"nginx 1.24"}}}}
	for _, filter := range []assetInventoryFilter{{Type: "all", Keyword: "gitlab"}, {Type: "domain", Port: 443, Service: "NGINX", Fingerprint: "Git", TaskID: "task"}, {Type: "all", Keyword: "historical"}, {Type: "all", Keyword: "192.0.2.1"}} {
		if !matchesAssetInventory(item, filter) {
			t.Fatalf("expected matching filter: %#v", filter)
		}
	}
	for _, filter := range []assetInventoryFilter{{Type: "ip"}, {Type: "all", Port: 80}, {Type: "all", TaskID: "other"}, {Type: "all", Fingerprint: "WordPress"}} {
		if matchesAssetInventory(item, filter) {
			t.Fatalf("unexpected matching filter: %#v", filter)
		}
	}
	for _, query := range []string{"port=0", "port=65536", "port=abc", "type=endpoint"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/inventory?"+query, nil)
		ListAssetInventoryHandler(c)
		if c.Writer.Status() != 400 {
			t.Fatalf("query %s not rejected", query)
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/inventory?page=9999999999999999&pageSize=20", nil)
	page, size, start, end := inventoryPagination(c, 21)
	if page != 2 || size != 20 || start != 20 || end != 21 {
		t.Fatalf("bad bounded pagination: %d %d %d %d", page, size, start, end)
	}
}
func TestAssetInventoryHonorsRequestCancellation(t *testing.T) {
	db := inventoryTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := collectAssetInventory(db.WithContext(ctx), ""); err == nil {
		t.Fatal("cancelled query should fail")
	}
}

func TestAssetInventoryFingerprintPortFallback(t *testing.T) {
	db := inventoryTestDB(t)
	for _, result := range []database.FingerprintInfo{
		{TaskID: "scan", Version: 1, URL: "https://fallback.example.com/path", Port: 8443, Status: 200},
		{TaskID: "scan", Version: 1, URL: "https://explicit.example.com:9443/path", Port: 8443, Status: 200},
	} {
		if err := db.Create(&result).Error; err != nil {
			t.Fatal(err)
		}
	}
	items, err := collectAssetInventory(db, "")
	if err != nil {
		t.Fatal(err)
	}
	for host, expected := range map[string]int{"fallback.example.com": 8443, "explicit.example.com": 9443} {
		item := inventoryFind(t, items, host)
		if len(item.Ports) != 1 || item.Ports[0].Port != expected {
			t.Fatalf("%s: port fallback lost: %#v", host, item.Ports)
		}
	}
}
