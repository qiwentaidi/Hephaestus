package ai

import (
	"testing"

	"hephaestus-backend/pkg/database"
)

func TestPentestScanReadersValidateTaskTypeWithoutConversationTarget(t *testing.T) {
	for _, item := range []struct {
		name     string
		scanType string
		allowed  bool
	}{
		{"attack_surface_get_scan_task", "webscan", true},
		{"attack_surface_list_scan_results", "dirscan", true},
		{"attack_surface_get_scan_task", "portscan", true},
		{"subdomain_list_results", "subdomain", true},
		{"attack_surface_get_jsscan_overview", "jsscan", true},
		{"subdomain_get_task", "webscan", false},
		{"attack_surface_get_jsscan_overview", "portscan", false},
	} {
		if !isPentestTaskReadTool(item.name) {
			t.Fatalf("%s is not registered as a task read tool", item.name)
		}
		task := database.VulnScanTask{ID: "scan", ScanType: item.scanType, Targets: `["https://other.example.test"]`}
		err := validatePentestTaskReadType(item.name, task)
		if (err == nil) != item.allowed {
			t.Fatalf("%s %s: allowed=%v error=%v", item.name, item.scanType, item.allowed, err)
		}
	}
}
