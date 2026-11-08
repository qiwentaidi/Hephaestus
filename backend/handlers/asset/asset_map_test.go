package asset

import (
	"testing"
	"time"
)

func TestBuildAssetMapOverviewRollsDomainRiskToResolvedIP(t *testing.T) {
	items := []AssetCenterItem{
		{
			Type:      "ip",
			Value:     "203.0.113.20",
			Location:  "中国/浙江省/杭州市",
			TaskCount: 1,
		},
		{
			Type:      "domain",
			Value:     "api.example.com",
			IPs:       []string{"203.0.113.20"},
			TaskCount: 1,
			riskFindings: map[string]struct{}{
				"high\x00CVE-2026-0001|https://api.example.com/admin": {},
			},
		},
	}

	overview := buildAssetMapOverview(items)
	if len(overview.Regions) != 1 {
		t.Fatalf("expected one region, got %#v", overview.Regions)
	}
	region := overview.Regions[0]
	if region.HighCount != 1 || region.CriticalCount != 0 {
		t.Fatalf("expected domain high risk rolled up to IP, got %#v", region)
	}
	if overview.TotalHigh != 1 || overview.TotalCritical != 0 {
		t.Fatalf("expected overview totals to include rolled up risk, got %#v", overview)
	}

	assets := buildAssetMapAssets(items, assetMapAssetQuery{Scope: "located", Province: "浙江"})
	if assets.Total != 1 || assets.Items[0].HighCount != 1 {
		t.Fatalf("expected asset list to expose rolled up risk, got %#v", assets)
	}
}

func TestBuildAssetMapOverviewDeduplicatesRiskFindings(t *testing.T) {
	items := []AssetCenterItem{
		{
			Type:          "ip",
			Value:         "203.0.113.30",
			Location:      "中国/广东省/深圳市",
			CriticalCount: 9,
			HighCount:     9,
			riskFindings: map[string]struct{}{
				"critical\x00CVE-2026-0002|https://203.0.113.30/": {},
				"high\x00CVE-2026-0003|https://203.0.113.30/":     {},
			},
		},
	}
	overview := buildAssetMapOverview(items)
	region := overview.Regions[0]
	if region.CriticalCount != 1 || region.HighCount != 1 {
		t.Fatalf("expected distinct findings only, got %#v", region)
	}
}

func TestBuildAssetMapAssetsFiltersProvinceAndPaginates(t *testing.T) {
	now := time.Now().UTC()
	items := []AssetCenterItem{
		{Type: "ip", Value: "203.0.113.41", Location: "中国/浙江省/杭州市", Operator: "阿里云计算有限公司", ASN: "AS45102", TaskCount: 1, LastSeenAt: now, UpdatedAt: now,
			riskFindings: map[string]struct{}{"critical\x00t1|https://203.0.113.41/": {}}},
		{Type: "ip", Value: "203.0.113.42", Location: "中国/浙江省/宁波市", Operator: "中国电信", ASN: "AS4134", TaskCount: 1, LastSeenAt: now, UpdatedAt: now},
		{Type: "ip", Value: "203.0.113.43", Location: "中国/广东省/深圳市", Operator: "中国电信", ASN: "AS4134", TaskCount: 1, LastSeenAt: now, UpdatedAt: now},
		{Type: "ip", Value: "10.0.0.8", Location: "", TaskCount: 1, LastSeenAt: now, UpdatedAt: now},
		{Type: "endpoint", Value: "https://203.0.113.41/login", EndpointHost: "203.0.113.41", Port: 443, Protocol: "tcp", ServiceLabels: []string{"nginx"}},
	}

	page := buildAssetMapAssets(items, assetMapAssetQuery{Scope: "located", Province: "浙江", Page: 1, PageSize: 1})
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].Value != "203.0.113.41" {
		t.Fatalf("unexpected province page: %#v", page)
	}
	if page.Items[0].City != "杭州" || page.Items[0].Operator != "阿里云" {
		t.Fatalf("expected normalized city/operator, got %#v", page.Items[0])
	}
	if len(page.Items[0].Ports) != 1 || page.Items[0].Ports[0] != "443/tcp" {
		t.Fatalf("expected endpoint port on IP row, got %#v", page.Items[0].Ports)
	}

	internal := buildAssetMapAssets(items, assetMapAssetQuery{Scope: "internal", Page: 1, PageSize: 20})
	if internal.Total != 1 || internal.Items[0].Value != "10.0.0.8" {
		t.Fatalf("unexpected internal scope: %#v", internal)
	}

	risky := buildAssetMapAssets(items, assetMapAssetQuery{Scope: "located", Risk: "any", Page: 1, PageSize: 20})
	if risky.Total != 1 || risky.Items[0].Value != "203.0.113.41" {
		t.Fatalf("unexpected risk filter: %#v", risky)
	}
}

func TestNormalizeAssetRiskSeverity(t *testing.T) {
	cases := map[string]string{
		"CRITICAL": "critical",
		"严重":       "critical",
		"HIGH":     "high",
		"高危":       "high",
		"MEDIUM":   "",
		"low":      "",
	}
	for input, expected := range cases {
		if actual := normalizeAssetRiskSeverity(input); actual != expected {
			t.Fatalf("severity %q: expected %q, got %q", input, expected, actual)
		}
	}
}

func TestBuildAssetMapOverviewSkipsProvincePseudoCity(t *testing.T) {
	items := []AssetCenterItem{
		{Type: "ip", Value: "203.0.113.60", Location: "中国/浙江省", TaskCount: 1},
		{Type: "ip", Value: "203.0.113.61", Location: "中国/浙江省/浙江省", TaskCount: 1},
		{Type: "ip", Value: "203.0.113.62", Location: "中国/浙江省/杭州市", TaskCount: 1},
	}
	overview := buildAssetMapOverview(items)
	region := overview.Regions[0]
	if len(region.Cities) != 1 || region.Cities[0].Name != "杭州" || region.Cities[0].Count != 1 {
		t.Fatalf("expected only the real city, got %#v", region.Cities)
	}

	assets := buildAssetMapAssets(items, assetMapAssetQuery{Scope: "located", Province: "浙江"})
	for _, entry := range assets.Items {
		if entry.Value != "203.0.113.62" && entry.City != "" {
			t.Fatalf("expected province-level assets to have no city, got %#v", entry)
		}
	}
}

func TestBuildAssetMapUsesProjectedRisk(t *testing.T) {
	now := time.Now().UTC()
	items := []AssetCenterItem{{
		Type:       "ip",
		Value:      "203.0.113.70",
		Location:   "中国/浙江省/杭州市",
		LastSeenAt: now,
		UpdatedAt:  now,
	}}
	index := newFindingRiskIndex()
	index.active = true
	index.assets[findingAssetKey("ip", "203.0.113.70")] = findingAssetCounts{
		DirectCritical:  1,
		InheritedHigh:   2,
		InheritedMedium: 3,
	}

	overview := buildAssetMapOverviewWithRisk(items, time.Time{}, index)
	if len(overview.Regions) != 1 {
		t.Fatalf("unexpected regions: %#v", overview.Regions)
	}
	region := overview.Regions[0]
	if region.CriticalCount != 1 || region.HighCount != 2 || region.MediumCount != 3 {
		t.Fatalf("unexpected region risk: %#v", region)
	}
	if region.DirectCritical != 1 || region.InheritedHigh != 2 || region.InheritedMedium != 3 {
		t.Fatalf("unexpected region split: %#v", region)
	}

	list := buildAssetMapAssetsWithRisk(items, assetMapAssetQuery{Scope: "located", Page: 1, PageSize: 20}, index)
	if len(list.Items) != 1 {
		t.Fatalf("unexpected list: %#v", list)
	}
	entry := list.Items[0]
	if entry.DirectCritical != 1 || entry.InheritedHigh != 2 || entry.InheritedMedium != 3 {
		t.Fatalf("unexpected entry split: %#v", entry)
	}
	if entry.CriticalCount != 1 || entry.HighCount != 2 || entry.MediumCount != 3 {
		t.Fatalf("unexpected entry totals: %#v", entry)
	}
}

func TestAssetHostTypeClassifiesIPv6Variants(t *testing.T) {
	for _, value := range []string{
		"2603:1046:c01:2006:0:0:0:1",
		"2603:1046:c01:2006:",
		"[2603:1046:c01:2006:0:0:0:1]:443",
	} {
		if got := assetHostType(value); got != "ip" {
			t.Fatalf("assetHostType(%q) = %q, want ip", value, got)
		}
	}
	if got := assetHostType("app.example.com"); got != "domain" {
		t.Fatalf("domain classified as %q", got)
	}
}

func TestBuildAssetTopologyEntriesTrustsCatalogType(t *testing.T) {
	items := []AssetCenterItem{{
		Type:  "ip",
		Value: "2603:1046:c01:2006:",
	}}
	entries := buildAssetTopologyEntries(items)
	if len(entries) != 1 {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	if entries[0].Type != "ip" {
		t.Fatalf("topology should trust catalog type: %#v", entries[0])
	}
}
