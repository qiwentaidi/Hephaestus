package scan

import (
	"encoding/json"
	"strings"
	"testing"

	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/subdomain"
)

func TestListUnifiedSubdomainTasksUsesLatestVersionAndResultTable(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	if err := db.AutoMigrate(&database.SubdomainScanResult{}); err != nil {
		t.Fatal(err)
	}
	for _, task := range []database.VulnScanTask{
		{ID: "sub", Version: 1, ScanType: "subdomain", TaskName: "old", Status: "completed"},
		{ID: "sub", Version: 2, ScanType: "subdomain", TaskName: "latest", Status: "running", CurrentCount: 4, ProgressDetail: `{"overallProgress":50}`},
		{ID: "web", Version: 1, ScanType: "webscan", TaskName: "ignore", Status: "completed"},
	} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, result := range []database.SubdomainScanResult{
		{ID: "one", TaskID: "sub", Domain: "example.test", Subdomain: "a.example.test", IPs: `[]`},
		{ID: "two", TaskID: "sub", Domain: "example.test", Subdomain: "b.example.test", IPs: `[]`},
	} {
		if err := db.Create(&result).Error; err != nil {
			t.Fatal(err)
		}
	}

	tasks, total, err := listUnifiedSubdomainTasks(db, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(tasks) != 1 || tasks[0].Version != 2 || tasks[0].TaskName != "latest" || tasks[0].CurrentCount != 4 || tasks[0].ResultCount != 2 {
		t.Fatalf("unexpected unified subdomain tasks: total=%d tasks=%#v", total, tasks)
	}
}

func TestBuildSubdomainCopyTextSingleIPEmpty(t *testing.T) {
	results := []subdomain.SubdomainResult{
		{Subdomain: "multi.example.com", IPs: []string{"192.0.2.1", "192.0.2.2"}},
		{Subdomain: "empty.example.com", IPs: nil},
		{Subdomain: "blank.example.com", IPs: []string{""}},
	}

	text, count := buildSubdomainCopyText(results, "single-ip")
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
	if text != "" {
		t.Fatalf("expected empty text, got %q", text)
	}
}

func TestBuildSubdomainExportContentWritesMultipleSources(t *testing.T) {
	results := []subdomain.SubdomainResult{
		{
			Domain:    "example.com",
			Subdomain: "api.example.com",
			IPs:       []string{"192.0.2.1"},
			Source:    "FOFA",
			Sources:   []string{"FOFA", "Quake", "AI"},
		},
	}

	content, _, err := buildSubdomainExportContent(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(content), "FOFA;Quake;AI") {
		t.Fatalf("expected CSV to include all sources, got %q", string(content))
	}
}

func TestBuildSubdomainCopyTextSingleIPDeduplicates(t *testing.T) {
	results := []subdomain.SubdomainResult{
		{Subdomain: "a.example.com", IPs: []string{"192.0.2.1"}},
		{Subdomain: "b.example.com", IPs: []string{"192.0.2.1"}},
		{Subdomain: "c.example.com", IPs: []string{"192.0.2.2"}},
		{Subdomain: "multi.example.com", IPs: []string{"192.0.2.3", "192.0.2.4"}},
	}

	text, count := buildSubdomainCopyText(results, "single-ip")
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}
	if text != "192.0.2.1\n192.0.2.2" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestBuildSubdomainPortScanTargetsFiltersLoopbackAndPreservesCollection(t *testing.T) {
	results := []subdomain.SubdomainResult{
		{Subdomain: "local.example.com", IPs: []string{"127.0.0.1"}},
		{Subdomain: "other-local.example.com", IPs: []string{"127.42.0.1"}},
		{Subdomain: "ipv6-local.example.com", IPs: []string{"::1", "::ffff:127.0.0.1"}},
		{Subdomain: "real.example.com", IPs: []string{"203.0.113.10"}},
		{Subdomain: "mixed.example.com", IPs: []string{"127.0.0.1", "203.0.113.20", "invalid", "[::1]"}},
		{Subdomain: "ipv6-mixed.example.com", IPs: []string{"::1", "2001:db8::1", "203.0.113.20"}},
		{Subdomain: "unresolved.example.com"},
		{Subdomain: "internal.example.com", IPs: []string{"192.168.1.10"}},
	}
	before, _ := json.Marshal(results)
	text, count := buildSubdomainCopyText(results, "portscan-targets")
	want := "real.example.com\n203.0.113.20\n2001:db8::1\nunresolved.example.com\ninternal.example.com"
	if text != want || count != 5 {
		t.Fatalf("import targets = %q (%d), want %q (5)", text, count, want)
	}
	after, _ := json.Marshal(results)
	if string(before) != string(after) {
		t.Fatal("portscan import changed the original collection results")
	}
	// Ordinary copying and exporting retain the loopback records as evidence.
	text, count = buildSubdomainCopyText(results, "subdomain")
	if count != len(results) || !strings.Contains(text, "local.example.com") || !strings.Contains(text, "mixed.example.com") {
		t.Fatalf("ordinary hostname copying changed: %q (%d)", text, count)
	}
	text, _ = buildSubdomainCopyText(results, "single-ip")
	if !strings.Contains(text, "127.0.0.1") {
		t.Fatal("ordinary IP copying lost loopback evidence")
	}
	content, _, err := buildSubdomainExportContent(results)
	if err != nil || !strings.Contains(string(content), "127.0.0.1") {
		t.Fatalf("CSV export lost loopback evidence: %v", err)
	}
}

func TestBuildSubdomainPortScanTargetsHandlesDuplicateMixedHosts(t *testing.T) {
	results := []subdomain.SubdomainResult{
		{Subdomain: "mixed.example.com", IPs: []string{"203.0.113.10"}},
		{Subdomain: "MIXED.example.com.", IPs: []string{"127.0.0.1", "203.0.113.20"}},
		{Subdomain: "mixed.example.com", IPs: []string{"203.0.113.10"}},
	}
	text, count := buildSubdomainCopyText(results, "portscan-targets")
	if text != "203.0.113.10\n203.0.113.20" || count != 2 {
		t.Fatalf("duplicate mixed hostname escaped filtering: %q (%d)", text, count)
	}
}

func TestBuildSubdomainPortScanTargetsOnlyLoopbackIsEmpty(t *testing.T) {
	text, count := buildSubdomainCopyText([]subdomain.SubdomainResult{
		{Subdomain: "local.example.com", IPs: []string{"127.0.0.1", "127.255.255.254", "::1", "::ffff:127.0.0.1"}},
	}, "portscan-targets")
	if text != "" || count != 0 {
		t.Fatalf("expected no importable targets, got %q (%d)", text, count)
	}
}
