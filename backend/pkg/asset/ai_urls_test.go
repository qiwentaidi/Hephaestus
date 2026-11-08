package asset

import (
	"context"
	"hephaestus-backend/pkg/database"
	"reflect"
	"testing"
	"time"
)

func TestBackfillAIFindingURLsIdempotentAndPreservesArchive(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	if err := s.DB.Exec(`CREATE TABLE pentest_findings (id TEXT PRIMARY KEY, project_id TEXT, conversation_id TEXT, target TEXT, created_at DATETIME, superseded_at DATETIME)`).Error; err != nil {
		t.Fatal(err)
	}
	for i, raw := range []string{"https://example.com/a", "https://example.com:443/b", "mysql://example.com:3306", "accesskey", "https://target.test（说明；资源位于 https://static.test/main.js）"} {
		if err := s.DB.Exec(`INSERT INTO pentest_findings(id,project_id,conversation_id,target,created_at) VALUES(?,?,?,?,?)`, string(rune('a'+i)), "project", "conversation", raw, now).Error; err != nil {
			t.Fatal(err)
		}
	}
	s.DB.Exec(`INSERT INTO pentest_findings VALUES('z','project','conversation','https://superseded.example',?,?)`, now, now)
	preview, err := s.BackfillAIFindingURLs(ctx, false)
	if err != nil || preview.Examined != 5 || preview.Eligible != 3 || preview.Imported != 0 {
		t.Fatalf("preview %+v %v", preview, err)
	}
	var count int64
	s.DB.Model(&database.UnifiedAsset{}).Count(&count)
	if count != 0 {
		t.Fatal("preview wrote assets")
	}
	first, err := s.BackfillAIFindingURLs(ctx, true)
	if err != nil || first.Imported != 3 {
		t.Fatalf("backfill %+v %v", first, err)
	}
	var obs database.UnifiedAssetObservation
	if err := s.DB.Where("source_id = ? AND type = ?", "finding:a", TypeURL).First(&obs).Error; err != nil {
		t.Fatal(err)
	}
	if obs.Attributes["conversationId"] != "conversation" || obs.EvidenceRef != "finding:a" || !obs.ObservedAt.Equal(now) {
		t.Fatalf("lost provenance %+v", obs)
	}
	archived := true
	if _, err := s.UpdateAsset(ctx, obs.AssetID, AssetUpdate{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	second, err := s.BackfillAIFindingURLs(ctx, true)
	if err != nil || second.Imported != 0 {
		t.Fatalf("replay %+v %v", second, err)
	}
	var assetRow database.UnifiedAsset
	s.DB.First(&assetRow, "id = ?", obs.AssetID)
	if assetRow.ArchivedAt == nil || !assetRow.LastSeenAt.Equal(now) {
		t.Fatal("replay changed lifecycle")
	}
	s.DB.Model(&database.UnifiedAssetObservation{}).Count(&count)
	if count != 8 {
		t.Fatalf("want URL + host observations, got %d", count)
	}
}

func TestDiscoverAIURLs(t *testing.T) {
	cases := []struct {
		name   string
		tool   string
		args   map[string]any
		result any
		want   []string
	}{
		{"structured", "crawler", nil, map[string]any{"urls": []string{"https://a.test/a", "https://a.test/b", "https://a.test/a"}}, []string{"https://a.test/a", "https://a.test/b"}},
		{"shell JSONL and URL lines", "exec", map[string]any{"command": "curl https://not-executed.test"}, map[string]any{"output": "{\"url\":\"https://a.test/a\"}\nhttps://b.test\nmentioned https://ignore.test\n"}, []string{"https://a.test/a", "https://b.test"}},
		{"failed native", "http", map[string]any{"url": "https://a.test"}, map[string]any{"success": false, "url": "https://a.test", "status": 200}, []string{}},
		{"failed command", "exec", nil, map[string]any{"exitCode": 1, "output": "https://a.test"}, []string{}},
		{"blocked", "http", map[string]any{"url": "https://a.test"}, map[string]any{"error": "blocked"}, []string{}},
		{"HTTP 403 observed", "http_request", map[string]any{"url": "https://a.test"}, map[string]any{"status": 403}, []string{"https://a.test"}},
		{"not a website", "crawler", nil, map[string]any{"urls": []string{"mysql://a.test:3306", "accesskey"}}, []string{}},
		{"invalid hosts", "crawler", nil, map[string]any{"urls": []string{"https://api+token.example.com/login", "https://api..example.com"}}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DiscoverAIURLs(tc.tool, tc.args, tc.result)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestAIFindingURLsFromMixedTarget(t *testing.T) {
	for _, tc := range []struct {
		target string
		want   []string
	}{
		{"https://target.test（说明；脚本位于 https://static.test/main.js）", []string{"https://target.test", "https://static.test/main.js"}},
		{"Targets: https://example.test/a, https://example.test/b.", []string{"https://example.test/a", "https://example.test/b"}},
		{"(https://[2001:db8::1]:8443/a)", []string{"https://[2001:db8::1]:8443/a"}},
		{"https://example.test/a(b)", []string{"https://example.test/a(b)"}},
		{"mysql://db.test:3306 / accesskey", []string{}},
	} {
		if got := AIFindingURLs(tc.target); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %v want %v", tc.target, got, tc.want)
		}
	}
}
