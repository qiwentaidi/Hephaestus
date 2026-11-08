package asset

import (
	"reflect"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"
)

func TestAssetCenterEndpointKeepsPortServiceSeparateFromWebFingerprints(t *testing.T) {
	aggregates := make(map[string]*assetCenterAggregate)
	now := time.Now()
	addAssetCenterHostAndEndpoint(aggregates, "198.51.100.10", "http://198.51.100.10:8080", "port-task", "port scan", "portscan", 1, now, now, true, "http", 8080, "", "nginx 1.28.0")
	addAssetCenterEndpointServices(aggregates, "http://198.51.100.10:8080", []string{"GitLab", "Rails-Framework"})

	aggregate := aggregates["endpoint\x00http://198.51.100.10:8080"]
	if aggregate == nil {
		t.Fatal("expected endpoint aggregate")
	}
	if got := assetCenterPrimaryPortService(aggregate.portServices); got != "nginx 1.28.0" {
		t.Fatalf("unexpected port service: %q", got)
	}
	if _, exists := aggregate.services["GitLab"]; !exists {
		t.Fatal("expected web fingerprint to remain available to the technology stack")
	}
}

func TestAssetFingerprintServicesUsesStoredFingerprintLabels(t *testing.T) {
	fingerprint := database.FingerprintInfo{
		Detect:               "http",
		Fingerprints:         `["GitLab", "Rails-Framework", "Nginx", "GitLab"]`,
		HighRiskFingerprints: `["Nginx"]`,
	}

	got := assetFingerprintServices(fingerprint)
	want := []string{"Nginx", "GitLab", "Rails-Framework"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected fingerprint services: got %#v, want %#v", got, want)
	}
}

func TestAssetFingerprintServicesIgnoresDetect(t *testing.T) {
	fingerprint := database.FingerprintInfo{
		Detect:       "http",
		Fingerprints: `[]`,
	}

	got := assetFingerprintServices(fingerprint)
	want := []string{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected services for a detect-only result: got %#v, want %#v", got, want)
	}
}

func TestBuildAssetTopologyFromDomain(t *testing.T) {
	items := []AssetCenterItem{
		{Type: "domain", Value: "api.example.com", IPs: []string{"203.0.113.12"}},
		{Type: "ip", Value: "203.0.113.12", Operator: "Example Net", ASN: "AS64500"},
		{
			Type:          "endpoint",
			Value:         "https://203.0.113.12:443",
			EndpointHost:  "203.0.113.12",
			Port:          443,
			Protocol:      "https",
			Service:       "nginx",
			ServiceLabels: []string{"nginx", "TLS 1.3"},
		},
	}

	topology, ok := buildAssetTopology(items, "domain", "api.example.com")
	if !ok {
		t.Fatal("expected the domain root to exist")
	}
	if topology.Root.Value != "api.example.com" {
		t.Fatalf("unexpected root: %q", topology.Root.Value)
	}

	nodes := map[string]AssetTopologyNode{}
	for _, node := range topology.Nodes {
		nodes[node.ID] = node
	}
	for _, id := range []string{
		"domain:api.example.com",
		"ip:203.0.113.12",
		"endpoint:https://203.0.113.12:443",
		"technology:endpoint:https://203.0.113.12:443:nginx",
	} {
		if _, exists := nodes[id]; !exists {
			t.Fatalf("missing topology node %q", id)
		}
	}

	edges := map[string]bool{}
	for _, edge := range topology.Edges {
		edges[edge.Source+"|"+edge.Target+"|"+edge.Kind] = true
	}
	for _, edge := range []string{
		"domain:api.example.com|ip:203.0.113.12|resolves",
		"ip:203.0.113.12|endpoint:https://203.0.113.12:443|serves",
		"endpoint:https://203.0.113.12:443|technology:endpoint:https://203.0.113.12:443:nginx|identifies",
	} {
		if !edges[edge] {
			t.Fatalf("missing topology edge %q", edge)
		}
	}
}

func TestBuildAssetTopologyEntriesIncludeRegistrableRoot(t *testing.T) {
	items := []AssetCenterItem{
		{Type: "domain", Value: "api.example.co.uk", IPs: []string{"203.0.113.12"}, SourceTaskIDs: []string{"task-a"}},
		{Type: "domain", Value: "www.example.co.uk", IPs: []string{"203.0.113.18"}, SourceTaskIDs: []string{"task-b"}},
	}

	entries := buildAssetTopologyEntries(items)
	var root *AssetCenterItem
	for index := range entries {
		if entries[index].Type == "root" && entries[index].Value == "example.co.uk" {
			root = &entries[index]
			break
		}
	}
	if root == nil {
		t.Fatal("expected the registrable root domain to be a selectable entry")
	}
	if root.TaskCount != 2 || len(root.IPs) != 2 {
		t.Fatalf("unexpected root aggregate: %#v", root)
	}

	topology, ok := buildAssetTopology(items, "root", "example.co.uk")
	if !ok {
		t.Fatal("expected the root domain topology to be available")
	}
	containsEdge := false
	for _, edge := range topology.Edges {
		if edge.Source == "root:example.co.uk" && edge.Target == "domain:api.example.co.uk" && edge.Kind == "contains" {
			containsEdge = true
			break
		}
	}
	if !containsEdge {
		t.Fatal("expected the root domain to contain the observed subdomain")
	}
}

func TestAssetTopologyRootDomainSkipsIPAddressLikeValues(t *testing.T) {
	if root := assetTopologyRootDomain("2408:8642:afe"); root != "" {
		t.Fatalf("expected an address-like value to be excluded from root domains, got %q", root)
	}
}

func TestParseAssetMapLocationFromMergedIPMetadata(t *testing.T) {
	location, ok := parseAssetMapLocation("中国/北京市/朝阳区")
	if !ok {
		t.Fatal("expected Beijing location to be parsed")
	}
	if location.Province != "北京" || location.ProvinceLabel != "北京市" || location.City != "北京市" {
		t.Fatalf("unexpected parsed location: %#v", location)
	}

	location, ok = parseAssetMapLocation("中国/四川省/成都市")
	if !ok {
		t.Fatal("expected Sichuan location to be parsed")
	}
	if location.Province != "四川" || location.City != "成都市" {
		t.Fatalf("unexpected parsed location: %#v", location)
	}

	if _, ok = parseAssetMapLocation("美国/加利福尼亚州/洛杉矶"); ok {
		t.Fatal("expected overseas location to be excluded from China province map")
	}
}

func TestBuildAssetMapOverviewAggregatesProvinceStats(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	items := []AssetCenterItem{
		{Type: "ip", Value: "203.0.113.10", Location: "中国/北京市/朝阳区", Operator: "中国电信", ASN: "AS4134", TaskCount: 1, SourceTaskIDs: []string{"task-a"}, HighCount: 1, UpdatedAt: now},
		{Type: "ip", Value: "203.0.113.11", Location: "中国/北京市/海淀区", Operator: "中国联通", ASN: "AS4837", TaskCount: 1, SourceTaskIDs: []string{"task-b"}, CriticalCount: 1, UpdatedAt: now},
		{Type: "ip", Value: "203.0.113.12", Location: "中国/广东省/深圳市", Operator: "中国电信", ASN: "AS4134", TaskCount: 1, SourceTaskIDs: []string{"task-a"}, UpdatedAt: now},
		{Type: "ip", Value: "8.8.8.8", Location: "美国/加利福尼亚州/山景城", Operator: "Google", ASN: "AS15169", TaskCount: 1, SourceTaskIDs: []string{"task-c"}, UpdatedAt: now},
		{Type: "ip", Value: "10.0.0.1", Location: "", Operator: "", ASN: "", TaskCount: 1, SourceTaskIDs: []string{"task-d"}, UpdatedAt: now},
		{Type: "ip", Value: "1.1.1.1", Location: "", Operator: "Cloudflare", ASN: "AS13335", TaskCount: 1, SourceTaskIDs: []string{"task-e"}, UpdatedAt: now},
		{Type: "domain", Value: "api.example.com"},
	}

	overview := buildAssetMapOverview(items)
	if overview.TotalIPAssets != 6 || overview.LocatedIPAssets != 3 || overview.UnknownIPAssets != 3 {
		t.Fatalf("unexpected overview counts: %#v", overview)
	}
	if overview.OverseasIPAssets != 1 || overview.NonPublicIPAssets != 1 || overview.UnresolvedIPAssets != 1 {
		t.Fatalf("unexpected unmapped breakdown: %#v", overview)
	}
	if len(overview.Regions) != 2 {
		t.Fatalf("unexpected region count: %#v", overview.Regions)
	}
	beijing := overview.Regions[0]
	if beijing.Province != "北京" || beijing.AssetCount != 2 || beijing.TaskCount != 2 || beijing.CriticalCount != 1 || beijing.HighCount != 1 {
		t.Fatalf("unexpected Beijing aggregate: %#v", beijing)
	}
	if len(overview.Operators) == 0 || overview.Operators[0].Name != "中国电信" || overview.Operators[0].Count != 2 {
		t.Fatalf("unexpected operator stats: %#v", overview.Operators)
	}
	if len(overview.Countries) == 0 || overview.Countries[0].Name != "美国" || overview.Countries[0].Count != 1 {
		t.Fatalf("unexpected overseas country stats: %#v", overview.Countries)
	}
	usa := overview.Countries[0]
	if usa.AssetCount != 1 || usa.TaskCount != 1 || len(usa.ASNs) != 1 {
		t.Fatalf("unexpected overseas country detail: %#v", usa)
	}
}
