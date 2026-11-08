package toolbox

import "testing"

func TestParseNucleiTemplatePolishResultHandlesMarkdownJSON(t *testing.T) {
	result, err := parseNucleiTemplatePolishResult("```json\n{\"id\":\"test-id\",\"name\":\"Test\",\"severity\":\"high\"}\n```")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.ID != "test-id" {
		t.Fatalf("unexpected id: %q", result.ID)
	}
	if result.Severity != "high" {
		t.Fatalf("unexpected severity: %q", result.Severity)
	}
}

func TestSanitizeNucleiTemplateID(t *testing.T) {
	got := sanitizeNucleiTemplateID("  Admin Login Exposure!! ")
	if got != "admin-login-exposure" {
		t.Fatalf("unexpected id: %q", got)
	}
}

func TestNormalizeNucleiSeverityFallback(t *testing.T) {
	if got := normalizeNucleiSeverity("dangerous"); got != "info" {
		t.Fatalf("unexpected severity fallback: %q", got)
	}
}
