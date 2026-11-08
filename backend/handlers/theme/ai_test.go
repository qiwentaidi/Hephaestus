package theme

import (
	"encoding/json"
	"strings"
	"testing"
)

func aiThemePayload(t *testing.T, overrides map[string]any) string {
	t.Helper()
	colors := map[string]any{
		"background":            "#101114",
		"foreground":            "#ececf1",
		"card":                  "#181a20",
		"primary":               "#5b8def",
		"primaryForeground":     "#ffffff",
		"muted":                 "#1c1f26",
		"mutedForeground":       "#a0a7b4",
		"border":                "#2a2f3a",
		"sidebar":               "#0c0d10",
		"sidebarForeground":     "#d7dae2",
		"destructive":           "#e5484d",
		"destructiveForeground": "#ffffff",
		"ring":                  "#5b8def",
	}
	for key, value := range overrides {
		colors[key] = value
	}
	payload := map[string]any{
		"name":        "深海蓝",
		"description": "冷静专业的深色主题",
		"definition": map[string]any{
			"schemaVersion": 1,
			"base":          "dark",
			"radius":        "0.5rem",
			"colors":        colors,
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestParseAIThemeResponse(t *testing.T) {
	content := "```json\n" + aiThemePayload(t, nil) + "\n```"
	result, err := parseAIThemeResponse(content, "light")
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "深海蓝" || result.Definition.Base != "dark" {
		t.Fatalf("unexpected generated theme: %+v", result)
	}
	if result.Definition.Colors.Primary != "#5b8def" {
		t.Fatalf("unexpected primary: %s", result.Definition.Colors.Primary)
	}
}

func TestParseAIThemeResponseSupportsTopLevelColors(t *testing.T) {
	content := `{"name":"纸墨","base":"light","radius":"0.25rem","colors":` +
		`{"background":"#fbfbfd","foreground":"#1c2024","card":"#ffffff","primary":"#3e63dd",` +
		`"primaryForeground":"#ffffff","muted":"#f3f4f7","mutedForeground":"#60646c",` +
		`"border":"#d9d9e0","sidebar":"#f6f7f9","sidebarForeground":"#1c2024",` +
		`"destructive":"#e5484d","destructiveForeground":"#ffffff","ring":"#3e63dd"}}`
	result, err := parseAIThemeResponse(content, "dark")
	if err != nil {
		t.Fatal(err)
	}
	if result.Definition.Base != "light" || result.Definition.Radius != "0.25rem" {
		t.Fatalf("unexpected definition: %+v", result.Definition)
	}
}

func TestParseAIThemeResponsePreservesTokenOverrides(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(aiThemePayload(t, nil)), &payload); err != nil {
		t.Fatal(err)
	}
	definition := payload["definition"].(map[string]any)
	definition["overrides"] = map[string]any{
		"--card":           "#1b2638",
		"--sidebar-accent": "#263a4d",
		"--tone-orange":    "#f59e0b",
	}
	content, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	result, err := parseAIThemeResponse(string(content), "dark")
	if err != nil {
		t.Fatal(err)
	}
	if result.Definition.Overrides["--card"] != "#1b2638" {
		t.Fatalf("expected card override to be preserved: %+v", result.Definition.Overrides)
	}
	if result.Definition.Overrides["--sidebar-accent"] != "#263a4d" {
		t.Fatalf("expected sidebar override to be preserved: %+v", result.Definition.Overrides)
	}
}

func TestBuildThemeGenerationMessagesRequestsLayeredOverrides(t *testing.T) {
	messages := buildThemeGenerationMessages("赛博海港，冷暖对比", "dark")
	if len(messages) != 2 {
		t.Fatalf("expected system and user messages, got %d", len(messages))
	}
	for _, needle := range []string{"overrides", "8 到 24", "至少形成 3 个", "辅助色彩关系"} {
		if !strings.Contains(messages[0].Content, needle) {
			t.Fatalf("system prompt missing %q", needle)
		}
	}
}

func TestParseAIThemeResponseRejectsInvalidColors(t *testing.T) {
	content := aiThemePayload(t, map[string]any{"primary": "blue"})
	if _, err := parseAIThemeResponse(content, "dark"); err == nil {
		t.Fatal("expected invalid color to be rejected")
	}
}

func TestParseAIThemeResponseFallsBackToRequestedBase(t *testing.T) {
	content := strings.Replace(aiThemePayload(t, nil), `"base":"dark"`, `"base":""`, 1)
	result, err := parseAIThemeResponse(content, "light")
	if err != nil {
		t.Fatal(err)
	}
	if result.Definition.Base != "light" {
		t.Fatalf("expected fallback base light, got %s", result.Definition.Base)
	}
}

func TestParseAIThemeResponseFallsBackFromInvalidBase(t *testing.T) {
	content := strings.Replace(aiThemePayload(t, nil), `"base":"dark"`, `"base":"auto"`, 1)
	result, err := parseAIThemeResponse(content, "light")
	if err != nil {
		t.Fatal(err)
	}
	if result.Definition.Base != "light" {
		t.Fatalf("expected fallback base light, got %s", result.Definition.Base)
	}
}

func TestExtractJSONObject(t *testing.T) {
	raw, err := extractJSONObject("Here is the theme:\n```json\n{\"name\":\"x\"}\n```\nThanks")
	if err != nil {
		t.Fatal(err)
	}
	if raw != `{"name":"x"}` {
		t.Fatalf("unexpected extracted JSON: %s", raw)
	}
	if _, err := extractJSONObject("no object here"); err == nil {
		t.Fatal("expected non-JSON content to be rejected")
	}
}
