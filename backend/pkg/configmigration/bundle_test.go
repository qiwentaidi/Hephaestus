package configmigration

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestNormalizeModulesDefaultsToAll(t *testing.T) {
	got, err := normalizeModules(nil)
	if err != nil {
		t.Fatalf("normalizeModules(nil) error = %v", err)
	}
	if !reflect.DeepEqual(got, moduleOrder) {
		t.Fatalf("normalizeModules(nil) = %v, want %v", got, moduleOrder)
	}
}

func TestNormalizeModulesDeduplicatesAndTrims(t *testing.T) {
	got, err := normalizeModules([]string{" ai ", "apiKeys", "ai", ""})
	if err != nil {
		t.Fatalf("normalizeModules() error = %v", err)
	}
	want := []string{ModuleAI, ModuleAPIKeys}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeModules() = %v, want %v", got, want)
	}
}

func TestNormalizeModulesRejectsUnknownModule(t *testing.T) {
	_, err := normalizeModules([]string{ModuleAI, "database"})
	if err == nil {
		t.Fatal("normalizeModules() expected error for unknown module")
	}
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
}

func TestParseModules(t *testing.T) {
	got := ParseModules(" ai, apiKeys ,, notifications ")
	want := []string{ModuleAI, ModuleAPIKeys, ModuleNotifications}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseModules() = %v, want %v", got, want)
	}
	if modules := ParseModules("   "); len(modules) != 0 {
		t.Fatalf("ParseModules(blank) = %v, want empty", modules)
	}
}

func TestNormalizeScanTemplateDefaultsTaskTypeAndValidatesAIDialogue(t *testing.T) {
	template, err := normalizeScanTemplate(exportedScanTemplate{Name: "  basic  "})
	if err != nil {
		t.Fatalf("normalizeScanTemplate() error = %v", err)
	}
	if template.Name != "basic" || template.TaskType != "webscan" {
		t.Fatalf("unexpected normalization: %#v", template)
	}

	if _, err := normalizeScanTemplate(exportedScanTemplate{
		Name:     "dialogue",
		TaskType: "ai_dialogue",
		Config:   json.RawMessage(`{"prompt":"do it"}`),
	}); err == nil {
		t.Fatal("expected validation error for prompt without {{.goal}}")
	}

	valid, err := normalizeScanTemplate(exportedScanTemplate{
		Name:     "dialogue",
		TaskType: "ai_dialogue",
		Config:   json.RawMessage(`{"prompt":"goal: {{.goal}}"}`),
	})
	if err != nil {
		t.Fatalf("normalizeScanTemplate() error = %v", err)
	}
	if string(valid.Config) != `{"prompt":"goal: {{.goal}}"}` {
		t.Fatalf("config was not preserved: %s", valid.Config)
	}
}

func TestNormalizeScanTemplateRejectsInvalidConfigJSON(t *testing.T) {
	if _, err := normalizeScanTemplate(exportedScanTemplate{
		Name:     "broken",
		TaskType: "webscan",
		Config:   json.RawMessage(`{not-json`),
	}); err == nil {
		t.Fatal("expected validation error for invalid JSON")
	}
}
