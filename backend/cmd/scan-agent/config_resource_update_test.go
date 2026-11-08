package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestResourceArchiveRemainsReadableByWorker(t *testing.T) {
	root := t.TempDir()
	archive, err := os.Create(filepath.Join(root, "config.zip"))
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archive)
	entries := []struct {
		name    string
		mode    os.FileMode
		content string
	}{
		{"pocs/", os.ModeDir | 0o400, ""},
		{"pocs/missing-mode.yaml", 0, "id: missing-mode"},
		{"pocs/readonly.yaml", 0o400, "id: original"},
		{"pocs/readonly.yaml", 0o400, "id: updated"},
	}
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name}
		header.SetMode(entry.mode)
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(entry.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	extracted := filepath.Join(root, "extracted")
	if err := unzipAgentConfigResource(archive.Name(), extracted); err != nil {
		t.Fatal(err)
	}
	// Exercise the actual next stage, not just extraction's write operation.
	copied := filepath.Join(root, "copied")
	if err := copyAgentConfigResourceDir(filepath.Join(extracted, "pocs"), copied); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"missing-mode.yaml": "id: missing-mode", "readonly.yaml": "id: updated"} {
		data, err := os.ReadFile(filepath.Join(copied, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Fatalf("%s: got %q, want %q", name, data, want)
		}
	}
}
