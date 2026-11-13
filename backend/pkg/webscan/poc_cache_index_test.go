package webscan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestTemplate(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write template %s: %v", name, err)
	}
	return path
}

func TestPOCTemplateIndexRoundTrip(t *testing.T) {
	templateDir := t.TempDir()
	writeTestTemplate(t, templateDir, "test-template.yaml", `id: test-template
info:
  name: Test Template
  author: tester
  severity: high
  tags: http,test
  metadata:
    dast: true
    nested:
      vendor: acme
`)
	writeTestTemplate(t, templateDir, "second.yaml", `id: second
info:
  name: Second
  author: tester
  severity: low
  tags: tcp
`)

	folders := normalizePOCTemplateFolderInputs([]string{templateDir})
	original := WorkFlowDB
	WorkFlowDB = nil
	t.Cleanup(func() {
		WorkFlowDB = original
	})

	nextDB, entries, err := buildPOCTemplateCache(folders)
	if err != nil {
		t.Fatalf("build cache: %v", err)
	}
	if len(nextDB) != 2 || len(entries) != 2 {
		t.Fatalf("expected 2 templates, got %d/%d", len(nextDB), len(entries))
	}
	template, ok := nextDB["test-template"]
	if !ok {
		t.Fatalf("missing test-template key, got %v", nextDB)
	}
	if template.Info.Name != "Test Template" {
		t.Fatalf("unexpected template name: %q", template.Info.Name)
	}
	if !strings.Contains(template.RawContent, "id: test-template") {
		t.Fatalf("raw content not preserved: %q", template.RawContent)
	}
	if template.ModifiedTime == "" {
		t.Fatal("expected modified time to be populated")
	}
	if _, ok := template.Info.Metadata["nested"].(map[string]any); !ok {
		t.Fatalf("nested metadata not normalized: %#v", template.Info.Metadata["nested"])
	}

	indexPath := filepath.Join(t.TempDir(), "poc-template-index.json.gz")
	if err := writePOCTemplateIndexTo(indexPath, folders, entries); err != nil {
		t.Fatalf("write index: %v", err)
	}

	WorkFlowDB = nil
	if !loadPOCTemplateIndexFrom(indexPath, folders) {
		t.Fatal("expected index to load")
	}
	loaded := WorkFlowDB
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded templates, got %d", len(loaded))
	}
	loadedTemplate, ok := loaded["test-template"]
	if !ok {
		t.Fatalf("loaded cache missing test-template: %v", loaded)
	}
	if loadedTemplate.Info.Name != "Test Template" || loadedTemplate.RawContent != template.RawContent {
		t.Fatalf("loaded template differs from built template")
	}
	if vendor, ok := loadedTemplate.Info.Metadata["nested"].(map[string]any); !ok || vendor["vendor"] != "acme" {
		t.Fatalf("loaded nested metadata mismatch: %#v", loadedTemplate.Info.Metadata["nested"])
	}

	// 目录集合不一致时索引不可用。
	if loadPOCTemplateIndexFrom(indexPath, normalizePOCTemplateFolderInputs([]string{t.TempDir()})) {
		t.Fatal("expected folder mismatch to invalidate the index")
	}

	// 文件内容变化（mtime 变化）时索引不可用。
	modifiedPath := filepath.Join(templateDir, "second.yaml")
	if err := os.WriteFile(modifiedPath, []byte("id: second\ninfo:\n  name: Changed\n"), 0o644); err != nil {
		t.Fatalf("modify template: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(modifiedPath, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if loadPOCTemplateIndexFrom(indexPath, folders) {
		t.Fatal("expected modified template to invalidate the index")
	}

	// 文件被删除时索引不可用。
	if err := os.Remove(modifiedPath); err != nil {
		t.Fatalf("remove template: %v", err)
	}
	if loadPOCTemplateIndexFrom(indexPath, folders) {
		t.Fatal("expected missing template to invalidate the index")
	}
}

func TestNormalizeTemplateMetadataHandlesNestedInterfaceMaps(t *testing.T) {
	metadata := map[string]any{
		"outer": map[any]any{
			"inner": []any{map[any]any{"vendor": "acme"}},
		},
	}

	normalized := normalizeTemplateMetadata(metadata)
	outer, ok := normalized["outer"].(map[string]any)
	if !ok {
		t.Fatalf("outer map not normalized: %#v", normalized["outer"])
	}
	items, ok := outer["inner"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("inner list not preserved: %#v", outer["inner"])
	}
	inner, ok := items[0].(map[string]any)
	if !ok || inner["vendor"] != "acme" {
		t.Fatalf("nested map not normalized: %#v", items[0])
	}
}
