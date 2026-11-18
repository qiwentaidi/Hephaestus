package config

import (
	"encoding/json"
	"testing"
)

func TestNormalizeAIProviderHeadersCanonicalizesDropsAndKeepsLastValue(t *testing.T) {
	headers := []AIProviderHeader{
		{Name: " x-opencode-session ", Value: " ses_1 "},
		{Name: "X-OpenCode-Session", Value: "ses_2"},
		{Name: "Content-Type", Value: "text/plain"},
		{Name: "Bad Header", Value: "value"},
		{Name: "", Value: "no-name"},
		{Name: "X-Empty", Value: "   "},
		{Name: "X-Multiline", Value: "a\r\nb"},
		{Name: "X-OK", Value: " v "},
	}
	got := NormalizeAIProviderHeaders(headers)
	want := []AIProviderHeader{
		{Name: "X-Opencode-Session", Value: "ses_2"},
		{Name: "X-Ok", Value: "v"},
	}
	if len(got) != len(want) {
		t.Fatalf("normalized headers = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("header %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestNormalizeAIProviderHeadersCapsCount(t *testing.T) {
	headers := make([]AIProviderHeader, 0, maxAIProviderHeaders+8)
	for i := 0; i < maxAIProviderHeaders+8; i++ {
		headers = append(headers, AIProviderHeader{
			Name:  "X-Header-" + string(rune('A'+i%26)) + "-" + string(rune('0'+i/26)),
			Value: "value",
		})
	}
	if got := len(NormalizeAIProviderHeaders(headers)); got != maxAIProviderHeaders {
		t.Fatalf("normalized count = %d, want %d", got, maxAIProviderHeaders)
	}
}

func TestValidateAIProviderHeaders(t *testing.T) {
	base := func(headers []AIProviderHeader) AIConfig {
		return AIConfig{
			Providers: []AIProviderConfig{{
				ID:      "primary",
				Name:    "Primary",
				Headers: headers,
			}},
		}
	}
	if err := ValidateAIProviderHeaders(base([]AIProviderHeader{
		{Name: "x-opencode-session", Value: "ses_1"},
		{Name: "", Value: ""},
	})); err != nil {
		t.Fatalf("valid headers rejected: %v", err)
	}
	cases := map[string][]AIProviderHeader{
		"missing name": {{Name: "  ", Value: "value"}},
		"invalid name": {{Name: "Bad Header", Value: "value"}},
		"forbidden":    {{Name: "Content-Length", Value: "1"}},
		"newline":      {{Name: "X-Test", Value: "a\nb"}},
		"duplicate":    {{Name: "X-Test", Value: "1"}, {Name: "x-test", Value: "2"}},
	}
	for name, headers := range cases {
		if err := ValidateAIProviderHeaders(base(headers)); err == nil {
			t.Fatalf("%s: invalid headers accepted", name)
		}
	}
}

func TestIsOpenCodeAPI(t *testing.T) {
	for _, raw := range []string{
		"https://opencode.ai/inference/go/openai/v1",
		"https://opencode.ai",
		"https://zen.opencode.ai/inference",
	} {
		if !IsOpenCodeAPI(raw) {
			t.Fatalf("%q should be recognized as OpenCode", raw)
		}
	}
	for _, raw := range []string{
		"https://notopencode.ai/v1",
		"https://opencode.ai.evil.example/v1",
		"https://example.com/opencode.ai",
		"",
	} {
		if IsOpenCodeAPI(raw) {
			t.Fatalf("%q must not be recognized as OpenCode", raw)
		}
	}
}

func TestLegacyHeadersMigrateIntoProviderAndResolveByRoute(t *testing.T) {
	legacy := NormalizeAIConfig(AIConfig{
		API:     "https://legacy.example.com/v1",
		Key:     "sk-legacy",
		Model:   "legacy-model",
		Headers: []AIProviderHeader{{Name: "X-Gateway", Value: "gw"}},
	})
	if len(legacy.Providers) != 1 || len(legacy.Providers[0].Headers) != 1 || legacy.Providers[0].Headers[0].Value != "gw" {
		t.Fatalf("legacy headers were not migrated: %#v", legacy.Providers)
	}
	if len(legacy.Headers) != 1 || legacy.Headers[0].Name != "X-Gateway" {
		t.Fatalf("top-level headers were not synced: %#v", legacy.Headers)
	}

	cfg := NormalizeAIConfig(AIConfig{
		Enabled:           true,
		DefaultProviderID: "primary",
		Providers: []AIProviderConfig{
			{ID: "primary", Name: "Primary", Enabled: true, API: "https://primary.example.com/v1", Key: "sk-primary", Model: "primary",
				Headers: []AIProviderHeader{{Name: "X-Route", Value: "primary"}}},
			{ID: "backup", Name: "Backup", Enabled: true, API: "https://backup.example.com/v1", Key: "sk-backup", Model: "backup",
				Headers: []AIProviderHeader{{Name: "X-Route", Value: "backup"}}},
		},
		Routing: AIRoutingConfig{Workbench: "backup"},
	})
	resolved := ResolveAIRequestConfigForRoute(cfg, AIRouteWorkbench)
	if len(resolved.Headers) != 1 || resolved.Headers[0].Value != "backup" {
		t.Fatalf("route headers = %#v, want backup header", resolved.Headers)
	}
	if got := ResolveAIRequestConfig(cfg); len(got.Headers) != 1 || got.Headers[0].Value != "primary" {
		t.Fatalf("default headers = %#v, want primary header", got.Headers)
	}
}

func TestLiveAIProviderHeadersFollowSavedChannels(t *testing.T) {
	oldGet, oldSet := getSettingFunc, setSettingFunc
	t.Cleanup(func() { getSettingFunc, setSettingFunc = oldGet, oldSet })
	var stored any
	setSettingFunc = func(_ string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &stored)
	}
	getSettingFunc = func(string) (any, error) { return stored, nil }

	cfg := AIConfig{Enabled: true, DefaultProviderID: "primary", Providers: []AIProviderConfig{
		{ID: "primary", Enabled: true, API: "http://primary/v1", Key: "k", Model: "m",
			Headers: []AIProviderHeader{{Name: "X-Live", Value: "primary"}}},
		{ID: "fallback", Enabled: true, API: "http://fallback/v1", Key: "k2", Model: "m2",
			Headers: []AIProviderHeader{{Name: "X-Live", Value: "fallback"}}},
	}}
	if err := UpdateAIConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if got := LiveAIProviderHeaders("primary", "", "", ""); len(got) != 1 || got[0].Value != "primary" {
		t.Fatalf("live primary headers = %#v", got)
	}
	if got := LiveAIProviderHeaders("", "http://fallback/v1", "k2", "m2"); len(got) != 1 || got[0].Value != "fallback" {
		t.Fatalf("live endpoint headers = %#v", got)
	}
	if got := LiveAIProviderHeaders("missing", "", "", ""); len(got) != 0 {
		t.Fatalf("unknown provider headers = %#v", got)
	}
}
