package asset

import (
	"context"
	"fmt"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func topologyTableFixture() []AssetCenterItem {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	return []AssetCenterItem{
		{Type: "domain", Value: "example.com", SourceTaskIDs: []string{"dns"}, LastSeenAt: now},
		{Type: "domain", Value: "app.example.com", IPs: []string{"192.0.2.1", "192.0.2.2", "192.0.2.1"}, SourceTaskIDs: []string{"dns", "web-app"}, LastSeenAt: now},
		{Type: "domain", Value: "api.example.com", IPs: []string{"192.0.2.1"}, SourceTaskIDs: []string{"dns", "web-api"}, LastSeenAt: now},
		{Type: "domain", Value: "empty.example.com", SourceTaskIDs: []string{"dns"}, LastSeenAt: now},
		{Type: "ip", Value: "192.0.2.1", SourceTaskIDs: []string{"ports"}, LastSeenAt: now},
		{Type: "ip", Value: "192.0.2.2", SourceTaskIDs: []string{"dns"}, LastSeenAt: now},
		{Type: "ip", Value: "192.0.2.9", SourceTaskIDs: []string{"ports"}, LastSeenAt: now},
		{Type: "endpoint", Value: "ssh://192.0.2.1:22", EndpointHost: "192.0.2.1", Port: 22, Protocol: "ssh", Service: "OpenSSH", ServiceLabels: []string{"OpenSSH"}, SourceTaskIDs: []string{"ports"}, LastSeenAt: now},
		{Type: "endpoint", Value: "https://192.0.2.1:443", EndpointHost: "192.0.2.1", Port: 443, Protocol: "https", ServiceLabels: []string{"Nginx"}, SourceTaskIDs: []string{"ip-web"}, LastSeenAt: now},
		{Type: "endpoint", Value: "https://app.example.com:443", EndpointHost: "app.example.com", Port: 443, Protocol: "https", ServiceLabels: []string{"Vue"}, SourceTaskIDs: []string{"web-app"}, LastSeenAt: now},
		{Type: "endpoint", Value: "http://api.example.com:80", EndpointHost: "api.example.com", Port: 80, Protocol: "http", ServiceLabels: []string{"GitLab"}, SourceTaskIDs: []string{"web-api"}, LastSeenAt: now},
	}
}

func TestAssetTopologyTableMatchesObservedGraphPaths(t *testing.T) {
	items := topologyTableFixture()
	rows, stats := buildAssetTopologyTable(items)
	if len(rows) != 10 || stats != (AssetTopologyTableStats{Roots: 1, Domains: 4, IPs: 3, Endpoints: 4}) {
		t.Fatalf("lost, duplicated or inferred paths: rows=%d stats=%#v", len(rows), stats)
	}
	graph, ok := buildAssetTopology(items, "root", "example.com")
	if !ok {
		t.Fatal("reference graph unavailable")
	}
	edges := map[string]bool{}
	for _, edge := range graph.Edges {
		edges[edge.Source+"\x00"+edge.Target] = true
	}
	for _, row := range rows {
		if row.Type == "ip" {
			if row.IP != "192.0.2.9" || row.RootDomain != "" || row.Domain != "" {
				t.Fatalf("wrong standalone path: %#v", row)
			}
			continue
		}
		path := []string{assetTopologyNodeID("root", row.RootDomain)}
		if row.Domain != "" {
			path = append(path, assetTopologyNodeID("domain", row.Domain))
		}
		if row.IP != "" {
			path = append(path, assetTopologyNodeID("ip", row.IP))
		}
		if row.Endpoint != "" {
			path = append(path, assetTopologyNodeID("endpoint", row.Endpoint))
		}
		for i := 1; i < len(path); i++ {
			if !edges[path[i-1]+"\x00"+path[i]] {
				t.Fatalf("row invented a relationship absent from the topology: %#v", row)
			}
		}
		switch row.Endpoint {
		case "https://app.example.com:443":
			if row.IP != "" || !reflect.DeepEqual(row.Technologies, []string{"Vue"}) || !reflect.DeepEqual(row.SourceTaskIDs, []string{"web-app"}) {
				t.Fatalf("domain website was attributed to its resolved IPs: %#v", row)
			}
		case "https://192.0.2.1:443":
			if row.IP != "192.0.2.1" || !reflect.DeepEqual(row.Technologies, []string{"Nginx"}) {
				t.Fatalf("IP endpoint inherited virtual-host fingerprints: %#v", row)
			}
		}
	}
	repeat, _ := buildAssetTopologyTable(items)
	if !reflect.DeepEqual(rows, repeat) {
		t.Fatal("table pagination order is not stable")
	}
}

func TestAssetTopologyTableIncludesEndpointsBeyondGraphLimit(t *testing.T) {
	items := []AssetCenterItem{{Type: "domain", Value: "many.example.com"}}
	for i := range 30 {
		port := 8000 + i
		items = append(items, AssetCenterItem{Type: "endpoint", Value: fmt.Sprintf("http://many.example.com:%d", port), EndpointHost: "many.example.com", Port: port, Protocol: "http", SourceTaskIDs: []string{"scan"}})
	}
	rows, stats := buildAssetTopologyTable(items)
	if len(rows) != 30 || stats.Endpoints != 30 {
		t.Fatalf("table inherited graph display truncation: rows=%d stats=%#v", len(rows), stats)
	}
	graph, ok := buildAssetTopology(items, "domain", "many.example.com")
	if !ok {
		t.Fatal("reference graph unavailable")
	}
	visible := 0
	for _, node := range graph.Nodes {
		if node.Type == "endpoint" {
			visible++
		}
	}
	if visible >= len(rows) {
		t.Fatal("fixture did not exercise the graph display limit")
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/topology/table?page=2&pageSize=20", nil)
	page, _, start, end := inventoryPagination(c, len(rows))
	if page != 2 || end-start != 10 || rows[start].Port != 8020 {
		t.Fatalf("wrong flat-row pagination: %d [%d:%d]", page, start, end)
	}
}

func TestAssetTopologyTablePreservesMissingLayersAndIPv6(t *testing.T) {
	items := []AssetCenterItem{
		{Type: "domain", Value: "dns.example.net", IPs: []string{"2001:db8::1", "not-an-ip"}, SourceTaskIDs: []string{"dns"}},
		{Type: "endpoint", Value: "https://orphan.example.net:9443", SourceTaskIDs: []string{"web"}},
	}
	rows, stats := buildAssetTopologyTable(items)
	if len(rows) != 2 || stats.Roots != 1 || stats.Domains != 2 || stats.IPs != 1 || stats.Endpoints != 1 {
		t.Fatalf("incomplete paths lost: %#v %#v", rows, stats)
	}
	for _, row := range rows {
		if row.Domain == "dns.example.net" {
			if row.IP != "2001:db8::1" || row.Endpoint != "" || !reflect.DeepEqual(row.SourceTaskIDs, []string{"dns"}) {
				t.Fatalf("missing IP node not grounded in DNS evidence: %#v", row)
			}
		} else if row.IP != "" || row.Port != 9443 || row.Domain != "orphan.example.net" {
			t.Fatalf("orphan endpoint invented an IP relationship: %#v", row)
		}
	}
}

func TestAssetTopologyTableFiltersAreScopedToOnePath(t *testing.T) {
	rows, _ := buildAssetTopologyTable(topologyTableFixture())
	for _, tc := range []struct {
		name   string
		filter assetTopologyTableFilter
		want   int
	}{
		{"all", assetTopologyTableFilter{Type: "all"}, 10},
		{"standalone IP", assetTopologyTableFilter{Type: "ip"}, 1},
		{"same endpoint", assetTopologyTableFilter{Type: "all", Port: 443, Service: "HTTPS", Technology: "Vue", TaskID: "web-app"}, 1},
		{"cross endpoint technology", assetTopologyTableFilter{Type: "all", Port: 22, Technology: "Vue"}, 0},
		{"ancestor task is not endpoint source", assetTopologyTableFilter{Type: "all", Port: 22, TaskID: "web-app"}, 0},
		{"task name excluded from keyword", assetTopologyTableFilter{Type: "all", Keyword: "vue website"}, 0},
		{"task ID excluded from keyword", assetTopologyTableFilter{Type: "all", Keyword: "web-app"}, 0},
		{"technology keyword", assetTopologyTableFilter{Type: "all", Keyword: "Vue"}, 1},
		{"root domain", assetTopologyTableFilter{Type: "all", RootDomain: "other.net"}, 0},
		{"shared IP", assetTopologyTableFilter{Type: "all", Keyword: "192.0.2.1", Port: 443}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			for _, row := range rows {
				if matchesAssetTopologyTableRow(row, tc.filter) {
					count++
				}
			}
			if count != tc.want {
				t.Fatalf("matched %d paths, want %d", count, tc.want)
			}
		})
	}
	for _, query := range []string{"port=0", "port=65536", "port=abc", "type=endpoint"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/topology/table?"+query, nil)
		ListAssetTopologyTableHandler(c)
		if c.Writer.Status() != 400 {
			t.Fatalf("invalid query not rejected: %s", query)
		}
	}
}

func TestAssetTopologyTableProjectionHonorsCancellation(t *testing.T) {
	db := inventoryTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := collectAssetCenterAggregatesWithDB(db.WithContext(ctx)); err == nil {
		t.Fatal("cancelled table projection continued querying")
	}
}
