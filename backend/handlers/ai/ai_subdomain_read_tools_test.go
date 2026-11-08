package ai

import (
	"context"
	"strings"
	"testing"

	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/pkg/database"
)

func TestPentestSubdomainReaderUsesUnifiedTaskStorage(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	if err := db.AutoMigrate(&database.SubdomainScanResult{}); err != nil {
		t.Fatal(err)
	}
	for _, task := range []database.VulnScanTask{
		{ID: "unified-subdomain", Version: 1, ScanType: "subdomain", Status: "completed", Progress: 100},
		{ID: "unified-subdomain", Version: 2, ScanType: "subdomain", Status: "running", Progress: 45},
	} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, result := range []database.SubdomainScanResult{
		{ID: "result-a", TaskID: "unified-subdomain", Domain: "example.test", Subdomain: "api.example.test", IPs: `["192.0.2.10"]`, Source: "crtsh", Sources: `["crtsh","dns"]`},
		{ID: "result-b", TaskID: "unified-subdomain", Domain: "example.test", Subdomain: "www.example.test", IPs: `["192.0.2.20"]`, Source: "dns", Sources: `["dns"]`},
	} {
		if err := db.Create(&result).Error; err != nil {
			t.Fatal(err)
		}
	}

	state, err := readPentestSubdomainWithDB(context.Background(), db, "subdomain_get_task", pentestSubdomainReadArgs{TaskID: "unified-subdomain"})
	if err != nil {
		t.Fatal(err)
	}
	if state["version"] != 2 || state["scanType"] != "subdomain" || state["status"] != "running" || state["progress"] != 45 || state["resultCount"] != int64(2) {
		t.Fatalf("unexpected unified state: %#v", state)
	}
	legacyRefState, err := readPentestSubdomainWithDB(context.Background(), db, "subdomain_get_task", pentestSubdomainReadArgs{TaskID: "unified-subdomain/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if legacyRefState["taskId"] != "unified-subdomain" || legacyRefState["version"] != 1 {
		t.Fatalf("legacy task/version reference was not normalized: %#v", legacyRefState)
	}
	legacyLatestState, err := readPentestSubdomainWithDB(context.Background(), db, "subdomain_get_task", pentestSubdomainReadArgs{TaskID: "unified-subdomain/v0"})
	if err != nil {
		t.Fatal(err)
	}
	if legacyLatestState["taskId"] != "unified-subdomain" || legacyLatestState["version"] != 2 {
		t.Fatalf("legacy latest reference was not normalized: %#v", legacyLatestState)
	}

	page, err := readPentestSubdomainWithDB(context.Background(), db, "subdomain_list_results", pentestSubdomainReadArgs{TaskID: "unified-subdomain", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	results, ok := page["results"].([]pentestSubdomainReadResult)
	if !ok || len(results) != 1 || page["total"] != int64(2) || page["nextOffset"] != 1 || page["hasMore"] != true {
		t.Fatalf("unexpected unified result page: %#v", page)
	}
	if len(results[0].Sources) == 0 || len(results[0].IPs) != 1 {
		t.Fatalf("result fields were not decoded: %#v", results[0])
	}
}

func TestPentestSubdomainReaderRejectsWrongTaskAndHonorsCancellation(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	if err := db.Create(&database.VulnScanTask{ID: "web-task", Version: 1, ScanType: "webscan", Status: "running"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := readPentestSubdomainWithDB(context.Background(), db, "subdomain_get_task", pentestSubdomainReadArgs{TaskID: "web-task"}); err == nil || !strings.Contains(err.Error(), "类型为 webscan") {
		t.Fatalf("wrong scan type accepted: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readPentestSubdomainWithDB(cancelled, db, "subdomain_get_task", pentestSubdomainReadArgs{TaskID: "missing"}); err == nil {
		t.Fatal("cancelled context was ignored")
	}
}
