package asset

import (
	"context"
	"hephaestus-backend/pkg/database"
	"testing"
	"time"
)

func TestWebsiteOrigin(t *testing.T) {
	for raw, want := range map[string]string{
		"HTTPS://EXAMPLE.com:443/a?q=1#frag":         "https://example.com",
		"http://example.com:080/a":                   "http://example.com",
		"https://example.com:8443/a":                 "https://example.com:8443",
		"http://example.com/a":                       "http://example.com",
		"https://user:password@[2001:0db8::1]:443/a": "https://[2001:db8::1]",
		"http://[2001:db8::1]:8080/a":                "http://[2001:db8::1]:8080",
	} {
		got, err := WebsiteOrigin(raw)
		if err != nil || got != want {
			t.Errorf("%q => %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"mysql://example.com:3306", "accesskey", "https://", "https://example.com:0", "https://example.com:65536", "https://exa mple.com", "https://{host}/a", "https://example.com（说明）", "https://example.com,note"} {
		if _, err := WebsiteOrigin(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	n, err := Normalize("", TypeURL, "https://[2001:db8::1]:443/a")
	if err != nil || n.CanonicalValue != "https://[2001:db8::1]/a" {
		t.Fatalf("IPv6 normalization: %+v %v", n, err)
	}
}

func TestArchiveWebsiteAllMembersWithinScope(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	var selected string
	for _, v := range []struct{ scope, url string }{{"global", "https://example.com/a"}, {"global", "https://example.com:443/b"}, {"global", "http://example.com/a"}, {"other", "https://example.com/c"}} {
		a, _, err := s.Upsert(ctx, Observation{ScopeKey: v.scope, Type: TypeURL, Value: v.url, Source: "manual", ObservedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		if selected == "" {
			selected = a.ID
		}
	}
	if _, err := s.ArchiveWebsite(ctx, selected, true); err != nil {
		t.Fatal(err)
	}
	var count int64
	s.DB.Model(&database.UnifiedAsset{}).Where("archived_at IS NOT NULL").Count(&count)
	if count != 2 {
		t.Fatalf("archived %d, want 2", count)
	}
	if _, err := s.ArchiveWebsite(ctx, selected, false); err != nil {
		t.Fatal(err)
	}
	s.DB.Model(&database.UnifiedAsset{}).Where("archived_at IS NOT NULL").Count(&count)
	if count != 0 {
		t.Fatal("restore left archived members")
	}
}
