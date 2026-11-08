package scan

import (
	"encoding/json"
	"reflect"
	"testing"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/subdomain"
)

func TestSubdomainFirstCombinationFiltersLoopbackAcrossHandoffs(t *testing.T) {
	results := []subdomain.SubdomainResult{
		{Domain: "example.com", Subdomain: "example.com", IPs: []string{"127.0.0.1"}},
		{Domain: "example.com", Subdomain: "loopback.example.com", IPs: []string{"127.42.0.1", "::1", "::ffff:127.0.0.1"}},
		{Domain: "example.com", Subdomain: "mixed.example.com", IPs: []string{"127.0.0.1", "203.0.113.10"}},
		{Domain: "example.com", Subdomain: "real.example.com", IPs: []string{"203.0.113.20"}},
		{Domain: "example.com", Subdomain: "internal.example.com", IPs: []string{"192.168.1.10"}},
	}
	before, _ := json.Marshal(results)
	nodes := []combinationPipelineNode{
		{ModuleID: "subdomain", StageIndex: 0},
		{ModuleID: "portscan", StageIndex: 1},
		{ModuleID: "webscan", StageIndex: 2},
	}
	state := newCombinationContext([]string{"example.com"})
	state.applySubdomainResults(results, nodes)
	after, _ := json.Marshal(results)
	if string(before) != string(after) {
		t.Fatal("handoff changed the original subdomain collection results")
	}
	if !reflect.DeepEqual(state.PortTargets, []string{"192.168.1.10", "203.0.113.10", "203.0.113.20"}) {
		t.Fatalf("unexpected port targets: %#v", state.PortTargets)
	}
	if len(state.PortTargetAliases["203.0.113.10"]) != 0 {
		t.Fatal("mixed DNS answer must not become a hostname alias that can resolve to loopback again")
	}
	if len(state.PortTargetAliases["203.0.113.20"]) != 1 {
		t.Fatal("ordinary hostname alias was lost")
	}

	// Checkpoint merges and reloads must retain the filter for subsequent modules.
	state = mergeCombinationContext(newCombinationContext([]string{"example.com"}), state.clone())
	restored, err := loadCombinationContext(&database.CombinationTask{Context: state.serialize()})
	if err != nil {
		t.Fatal(err)
	}
	portTargets, err := restored.targetsFor("portscan")
	if err != nil || !reflect.DeepEqual(portTargets, state.PortTargets) {
		t.Fatalf("restored port targets = %#v, error = %v", portTargets, err)
	}

	// Exercise the persisted port-to-Web/crack handoff, including an old loopback
	// result that must not be dispatched by a resumed combination task.
	db := combinationProgressTestDB(t)
	if err := db.AutoMigrate(&database.PortScanResult{}, &database.PortScanResultAsset{}); err != nil {
		t.Fatal(err)
	}
	task := database.VulnScanTask{ID: "loopback-handoff", Version: 1, ScanType: "combination"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	exported := exportPortTargetAliases(restored.PortTargetAliases)
	for _, host := range append(append([]string(nil), portTargets...), "127.0.0.1") {
		for _, port := range []int{80, 22} {
			row := database.PortScanResult{TaskID: task.ID, Version: 1, Host: host, Port: port}
			if port == 22 {
				row.Protocol = "ssh"
			}
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
	restored.WebTargets, restored.CrackTargets, err = deriveTargetsFromPortScanResults(db, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored.WebTargetsReady, restored.CrackTargetsReady = true, true
	for _, module := range []string{"webscan", "dirscan", "jsscan"} {
		targets, err := restored.targetsFor(module)
		want := []string{"http://203.0.113.10:80", "http://internal.example.com:80", "http://real.example.com:80"}
		if err != nil || !reflect.DeepEqual(targets, want) {
			t.Fatalf("%s targets = %#v, want %#v, error = %v", module, targets, want, err)
		}
	}
	crackTargets, err := restored.targetsFor("crack")
	wantCrack := []string{"ssh://192.168.1.10:22", "ssh://203.0.113.10:22", "ssh://203.0.113.20:22"}
	if err != nil || !reflect.DeepEqual(crackTargets, wantCrack) {
		t.Fatalf("crack targets = %#v, want %#v, error = %v", crackTargets, wantCrack, err)
	}
}

func TestSubdomainFirstCombinationDoesNotFallBackToLoopbackHosts(t *testing.T) {
	state := newCombinationContext([]string{"Example.COM."})
	state.applySubdomainResults([]subdomain.SubdomainResult{
		{Subdomain: "example.com", IPs: []string{"127.0.0.1", "::1"}},
	}, []combinationPipelineNode{{ModuleID: "subdomain", StageIndex: 0}})
	for _, module := range []string{"portscan", "webscan", "dirscan", "jsscan", "crack"} {
		targets, err := state.targetsFor(module)
		if err != nil || len(targets) != 0 {
			t.Fatalf("%s fell back to a loopback host: %#v, error = %v", module, targets, err)
		}
	}
	// Direct Web scans without a portscan stage must also exclude collected hosts.
	state.InitialTargets = []string{"https://EXAMPLE.com.:443/path", "http://safe.example.com"}
	state.WebTargets = []string{
		"http://127.42.0.1:80", "http://[::1]:80", "http://[::ffff:127.0.0.1]:80",
		"https://EXAMPLE.com.:443/path", "http://safe.example.com",
	}
	targets, err := state.targetsFor("webscan")
	if err != nil || !reflect.DeepEqual(targets, []string{"http://safe.example.com"}) {
		t.Fatalf("Web targets = %#v, error = %v", targets, err)
	}
	state.WebTargets = nil
	targets, err = state.targetsFor("webscan")
	if err != nil || !reflect.DeepEqual(targets, []string{"http://safe.example.com"}) {
		t.Fatalf("fallback Web targets = %#v, error = %v", targets, err)
	}
	// Collection itself keeps receiving the original targets.
	targets, err = state.targetsFor("subdomain")
	if err != nil || !reflect.DeepEqual(targets, state.InitialTargets) {
		t.Fatalf("subdomain targets changed: %#v, error = %v", targets, err)
	}
}

func TestCombinationLoopbackFilterRequiresSubdomainFirstStage(t *testing.T) {
	for _, tt := range []struct {
		name  string
		nodes []combinationPipelineNode
	}{
		{"without collection", []combinationPipelineNode{{ModuleID: "portscan", StageIndex: 0}}},
		{"collection later", []combinationPipelineNode{{ModuleID: "webscan", StageIndex: 0}, {ModuleID: "subdomain", StageIndex: 1}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := newCombinationContext([]string{"127.0.0.1"})
			state.applySubdomainResults([]subdomain.SubdomainResult{
				{Subdomain: "local.example.com", IPs: []string{"127.0.0.1", "::1"}},
			}, tt.nodes)
			if state.FilterLoopback || len(state.LoopbackHosts) != 0 {
				t.Fatal("loopback filter enabled outside a subdomain-first combination")
			}
			targets, err := state.targetsFor("portscan")
			if err != nil || !reflect.DeepEqual(targets, []string{"127.0.0.1", "::1"}) {
				t.Fatalf("other combinations changed behavior: %#v, error = %v", targets, err)
			}
		})
	}
	state := newCombinationContext([]string{"http://127.0.0.1:80"})
	targets, err := state.targetsFor("webscan")
	if err != nil || !reflect.DeepEqual(targets, state.InitialTargets) {
		t.Fatalf("combination without collection changed behavior: %#v, error = %v", targets, err)
	}
}
