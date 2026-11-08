package scan

import (
	"testing"

	"hephaestus-backend/pkg/database"
)

func TestInterfacesFromOpenAPI(t *testing.T) {
	document := database.VulnScanJSOpenAPIDocument{
		TargetURL: "https://example.test/app",
		Document: `{
			"openapi":"3.1.0",
			"servers":[{"url":"https://example.test/api"}],
			"paths":{
				"/users/{id}":{
					"parameters":[{"name":"id","in":"path"}],
					"post":{
						"parameters":[{"name":"page","in":"query"}],
						"requestBody":{"content":{"application/json":{"schema":{"properties":{"name":{"type":"string"},"role":{"type":"string"}}}}}}
					}
				}
			}
		}`,
	}
	items, err := interfacesFromOpenAPI(document)
	if err != nil {
		t.Fatalf("parse interfaces: %v", err)
	}
	if len(items) != 1 || items[0].Method != "POST" || items[0].URL != "https://example.test/api/users/{id}" {
		t.Fatalf("unexpected interfaces: %+v", items)
	}
	want := map[jsScanInterfaceParameter]bool{
		{Name: "id", In: "path"}:    true,
		{Name: "page", In: "query"}: true,
		{Name: "name", In: "body"}:  true,
		{Name: "role", In: "body"}:  true,
	}
	if len(items[0].Parameters) != len(want) {
		t.Fatalf("parameters: %+v", items[0].Parameters)
	}
	for _, parameter := range items[0].Parameters {
		if !want[parameter] {
			t.Fatalf("unexpected parameter: %+v", parameter)
		}
	}
}
