package scan

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/subdomain"
)

func TestWildcardCandidatesDoNotExpandPortScanWebTargets(t *testing.T) {
	results := []subdomain.SubdomainResult{
		{Domain: "example.com", Subdomain: "example.com", IPs: []string{"203.0.113.10"}},
		{Domain: "example.com", Subdomain: "real.example.com", IPs: []string{"203.0.113.10"}},
		{Domain: "example.com", Subdomain: "cdn.example.com", IPs: []string{"203.0.113.30"}, AssetType: "CDN"},
		{Domain: "example.com", Subdomain: "only-wildcard.example.com", IPs: []string{"203.0.113.20"}, IsWildcard: true},
	}
	for i := 0; i < 12868; i++ {
		results = append(results, subdomain.SubdomainResult{
			Domain: "example.com", Subdomain: fmt.Sprintf("candidate-%d.example.com", i),
			IPs: []string{"203.0.113.10"}, IsWildcard: true,
		})
	}
	before, _ := json.Marshal(results)
	portTargets, aliases := derivePortTargetsFromSubdomainResults(results, true)
	if !reflect.DeepEqual(portTargets, []string{"203.0.113.10", "203.0.113.20"}) {
		t.Fatalf("unexpected port targets: %#v", portTargets)
	}
	if len(aliases["203.0.113.10"]) != 2 || len(aliases["203.0.113.20"]) != 0 {
		t.Fatalf("wildcard aliases leaked into port targets: %#v", aliases)
	}
	after, _ := json.Marshal(results)
	if string(before) != string(after) {
		t.Fatal("derivation changed the original collection results")
	}

	// Exercise checkpoint serialization and the persisted port-to-web handoff.
	state := newCombinationContext([]string{"example.com"})
	state.PortTargets, state.PortTargetAliases, state.PortTargetsReady = portTargets, aliases, true
	restored, err := loadCombinationContext(&database.CombinationTask{Context: state.serialize()})
	if err != nil {
		t.Fatal(err)
	}
	exported := exportPortTargetAliases(restored.PortTargetAliases)
	db := combinationProgressTestDB(t)
	if err := db.AutoMigrate(&database.PortScanResult{}, &database.PortScanResultAsset{}); err != nil {
		t.Fatal(err)
	}
	task := database.VulnScanTask{ID: "wildcard-handoff", Version: 1, ScanType: "combination"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	for _, host := range portTargets {
		for _, port := range []int{80, 443} {
			row := database.PortScanResult{TaskID: task.ID, Version: 1, Host: host, Port: port}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			for _, alias := range exported[host] {
				asset := database.PortScanResultAsset{
					TaskID: task.ID, Version: 1, ResultID: row.ID, IP: host, Port: port,
					Domain: alias["domain"], Subdomain: alias["subdomain"],
				}
				if err := db.Create(&asset).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	webTargets, _, err := deriveTargetsFromPortScanResults(db, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"http://203.0.113.20:80", "http://example.com:80", "http://real.example.com:80",
		"https://203.0.113.20:443", "https://example.com:443", "https://real.example.com:443",
	}
	if !reflect.DeepEqual(webTargets, want) {
		t.Fatalf("web targets = %#v, want %#v", webTargets, want)
	}
}
