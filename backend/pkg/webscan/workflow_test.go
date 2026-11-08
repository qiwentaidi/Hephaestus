package webscan

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func TestTemplateTimesUsePoCMetadataFields(t *testing.T) {
	content := []byte(`
id: CVE-2026-0001
info:
  name: Test Template
  author: tester
  severity: high
  metadata:
    create-at: "2026-08-26"
    update-at: "2026-08-27"
`)

	var template Template
	if err := yaml.Unmarshal(content, &template); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}

	if got := template.CreatedTime(); got != "2026-08-26" {
		t.Fatalf("expected created time from metadata.create-at, got %q", got)
	}
	if got := template.UpdatedTime(); got != "2026-08-27" {
		t.Fatalf("expected updated time from metadata.update-at, got %q", got)
	}
}

func TestTemplateTimesFallBackToMetadataAliases(t *testing.T) {
	content := []byte(`
id: CVE-2026-0002
info:
  name: Test Template
  author: tester
  severity: medium
  metadata:
    created_at: 2026-08-20
    updated-at: 2026-08-21
`)

	var template Template
	if err := yaml.Unmarshal(content, &template); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}

	if got := template.CreatedTime(); got != "2026-08-20" {
		t.Fatalf("expected created time from metadata.created_at, got %q", got)
	}
	if got := template.UpdatedTime(); got != "2026-08-21" {
		t.Fatalf("expected updated time from metadata.updated-at, got %q", got)
	}
}
