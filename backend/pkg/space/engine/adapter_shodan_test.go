package engine

import (
	"strings"
	"testing"

	"hephaestus-backend/pkg/types"
)

func TestShodanAdapterTransformResults(t *testing.T) {
	adapter, err := NewShodanAdapter(&ShodanConfig{API: "http://shodan.test", Key: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	results := adapter.transformResults([]types.ShodanMatch{
		{
			IP:        "192.0.2.60",
			Port:      443,
			Domains:   []string{"Example.com"},
			Hostnames: []string{"www.example.com"},
			Product:   "nginx",
			Version:   "1.24",
			Transport: "tcp",
			Data:      "HTTP/1.1 200 OK",
			ISP:       "Aliyun",
			HTTP:      types.ShodanHTTP{Title: "Example", Status: 200},
			Location:  types.ShodanLocation{CountryName: "China", RegionCode: "ZJ", City: "Hangzhou"},
			SSL:       map[string]any{"cert": map[string]any{}},
			Shodan:    types.ShodanMatchModule{Module: "https"},
		},
		{
			IP:        "192.0.2.60",
			Port:      22,
			Product:   "OpenSSH",
			Version:   "9.0",
			Transport: "tcp",
			Shodan:    types.ShodanMatchModule{Module: "ssh"},
		},
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	https := results[0]
	if https.URL != "https://example.com" || https.Protocol != "https" || https.Domain != "example.com" || https.Title != "Example" {
		t.Fatalf("unexpected https result: %#v", https)
	}
	if len(https.Components) != 2 || https.Components[0] != "nginx" || https.Components[1] != "1.24" {
		t.Fatalf("unexpected components: %#v", https.Components)
	}
	if https.Country != "China" || https.Region != "ZJ" || https.City != "Hangzhou" || https.ISP != "Aliyun" {
		t.Fatalf("unexpected location metadata: %#v", https)
	}
	ssh := results[1]
	if ssh.Protocol != "ssh" || ssh.URL != "" || ssh.Domain != "" {
		t.Fatalf("unexpected ssh result: %#v", ssh)
	}
}

func TestShodanTranspile(t *testing.T) {
	query := DefaultTranspiler.TranslateQuery(`ip:1.2.3.4 AND domain:"example.com" AND title:"login" AND port:443`, "shodan", QueryTypeDefault)
	normalized := strings.Join(strings.Fields(query), " ")
	want := `ip:1.2.3.4 hostname:"example.com" http.title:"login" port:443`
	if normalized != want {
		t.Fatalf("TranslateQuery = %q, want %q", normalized, want)
	}
}

func TestShodanTypedQuery(t *testing.T) {
	ipQuery, err := DefaultTranspiler.TranslateQueryChecked("1.1.1.1", "shodan", QueryTypeIP)
	if err != nil || ipQuery != "ip:1.1.1.1" {
		t.Fatalf("typed ip query = %q, err=%v", ipQuery, err)
	}
	domainQuery, err := DefaultTranspiler.TranslateQueryChecked("example.com", "shodan", QueryTypeDomain)
	if err != nil || domainQuery != `hostname:"example.com"` {
		t.Fatalf("typed domain query = %q, err=%v", domainQuery, err)
	}
}
