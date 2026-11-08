package config

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAIRequestTimeoutPersistenceAndLiveChannels(t *testing.T) {
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
	// Existing single-provider and multi-provider configurations both default to 30.
	for _, legacy := range []AIConfig{{API: "http://local/v1", Key: "fixture", Model: "local"}, {Providers: []AIProviderConfig{{ID: "legacy"}}}} {
		if err := UpdateAIConfig(legacy); err != nil {
			t.Fatal(err)
		}
		got, err := GetAIConfig()
		if err != nil || got.Providers[0].RequestTimeoutMinutes != 30 || got.RequestTimeoutMinutes != 30 {
			t.Fatalf("legacy default: %+v, %v", got, err)
		}
	}
	cfg := AIConfig{Enabled: true, DefaultProviderID: "primary", Providers: []AIProviderConfig{
		{ID: "primary", Enabled: true, API: "http://local/v1", Key: "fixture", Model: "local", RequestTimeoutMinutes: 1},
		{ID: "fallback", Enabled: true, API: "http://local/v1", Key: "fixture", Model: "local", RequestTimeoutMinutes: 120},
	}, Routing: AIRoutingConfig{Workbench: "fallback"}}
	if err := UpdateAIConfig(cfg); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]time.Duration{"primary": time.Minute, "fallback": 120 * time.Minute} {
		if got := LiveAIRequestTimeout(id, "", "", "", 30); got != want {
			t.Fatalf("%s timeout = %s, want %s", id, got, want)
		}
	}
	// Resolving a route and then resolving its request must retain the channel.
	routed := ResolveAIRequestConfig(ResolveAIRequestConfigForRoute(cfg, AIRouteWorkbench))
	if routed.DefaultProviderID != "fallback" || routed.RequestTimeoutMinutes != 120 {
		t.Fatalf("route timeout lost: %+v", routed)
	}
	cfg.Providers[0].RequestTimeoutMinutes = 45
	if err := UpdateAIConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if got := LiveAIRequestTimeout("primary", "", "", "", 1); got != 45*time.Minute {
		t.Fatalf("saved timeout not refreshed: %s", got)
	}
	for _, invalid := range []int{-1, 121} {
		cfg.Providers[0].RequestTimeoutMinutes = invalid
		if err := UpdateAIConfig(cfg); err == nil {
			t.Fatalf("accepted invalid timeout %d", invalid)
		}
		if got := LiveAIRequestTimeout("primary", "", "", "", 1); got != 45*time.Minute {
			t.Fatalf("invalid update changed storage: %s", got)
		}
	}
}
