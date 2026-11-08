package ai

import (
	"context"
	"strings"
	"testing"

	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/pkg/database"
)

func TestAIScanReadersVersionPaginationAndPausedState(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	if err := db.AutoMigrate(&database.PortScanResult{}, &database.VulnScanDirResult{}, &database.FingerprintInfo{}, &database.VulnerabilityInfo{}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"webscan", "portscan", "dirscan"} {
		for _, version := range []int{1, 2} {
			task := database.VulnScanTask{ID: kind, Version: version, ScanType: kind, Status: "paused", PauseRequested: true, Config: `{"token":"never return"}`}
			if err := db.Create(&task).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, version := range []int{1, 2} {
		for _, port := range []int{80, 443, 8080} {
			if err := db.Create(&database.PortScanResult{TaskID: "portscan", Version: version, Host: "example.test", Port: port}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx := context.Background()
	state, err := readPentestScanWithDB(ctx, db, "attack_surface_get_scan_task", pentestScanReadArgs{TaskID: "portscan"})
	if err != nil || state["version"] != 2 || state["status"] != "paused" || state["pauseRequested"] != true || state["config"] != nil {
		t.Fatal(state, err)
	}
	for offset := 0; offset < 3; offset++ {
		page, err := readPentestScanWithDB(ctx, db, "attack_surface_list_scan_results", pentestScanReadArgs{TaskID: "portscan", Version: 1, Offset: offset, Limit: 1})
		if err != nil || page["total"] != int64(3) || page["nextOffset"] != offset+1 || page["hasMore"] != (offset < 2) {
			t.Fatal(page, err)
		}
	}
	if _, err = readPentestScanWithDB(ctx, db, "attack_surface_list_scan_results", pentestScanReadArgs{TaskID: "portscan", ResultType: "vulnerabilities"}); err == nil {
		t.Fatal("wrong type accepted")
	}
	for _, kind := range []string{"webscan", "dirscan"} {
		got, err := readPentestScanWithDB(ctx, db, "attack_surface_list_scan_results", pentestScanReadArgs{TaskID: kind})
		if err != nil || got["hasMore"] != false || got["status"] != "paused" {
			t.Fatal(got, err)
		}
	}
	if err := db.Create(&database.VulnerabilityInfo{TaskID: "webscan", Version: 2, VulnURL: "https://example.test", Response: strings.Repeat("a", 3000)}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := readPentestScanWithDB(ctx, db, "attack_surface_list_scan_results", pentestScanReadArgs{TaskID: "webscan", ResultType: "vulnerabilities"})
	if err != nil {
		t.Fatal(err)
	}
	row := got["results"].([]map[string]any)[0]
	if row["response_truncated"] != true {
		t.Fatal(row)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = readPentestScanWithDB(cancelled, db, "attack_surface_get_scan_task", pentestScanReadArgs{TaskID: "portscan"}); err == nil {
		t.Fatal("ignored cancellation")
	}
	var task database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", "portscan", 2).First(&task).Error; err != nil || task.Status != "paused" {
		t.Fatal("reader changed task", task, err)
	}
}
