package zoomeye

import (
	"testing"

	"hephaestus-backend/pkg/types"
)

func TestBuildHostListCollectsUniqueHosts(t *testing.T) {
	got := buildHostList([]types.ZoomeyeDataItem{
		{
			Domain:   "API.Example.com",
			Hostname: "api.example.com",
			URL:      "https://www.example.com/login",
		},
		{
			Domain:   "console.example.com.",
			Hostname: "",
			URL:      "not a url",
		},
	})

	want := []string{"api.example.com", "www.example.com", "console.example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %d hosts, want %d: %#v", len(got), len(want), got)
	}
	for i, host := range want {
		if got[i].Name != host {
			t.Fatalf("host[%d] = %q, want %q", i, got[i].Name, host)
		}
	}
}
