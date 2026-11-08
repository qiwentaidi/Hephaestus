package ai

import (
	"os"
	"path/filepath"
	"testing"

	"hephaestus-backend/lib/pentest"
)

func TestPersistMessageAttachmentsCannotEscapeWorkspace(t *testing.T) {
	root := t.TempDir()
	files := []pentest.MessageAttachmentUpload{{
		ID:          "attachment-id",
		Name:        "../../etc/passwd",
		ContentType: "text/plain",
		Size:        5,
		Data:        []byte("hello"),
	}}
	paths, err := persistMessageAttachmentsAt(root, "conversation-id", files)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || filepath.IsAbs(paths[0]) {
		t.Fatalf("unexpected attachment path: %#v", paths)
	}
	clean := filepath.Clean(paths[0])
	if clean == ".." || len(clean) >= 3 && clean[:3] == ".."+string(filepath.Separator) {
		t.Fatalf("path escaped workspace: %q", paths[0])
	}
	content, err := os.ReadFile(filepath.Join(root, "ai-pentest", "sessions", "conversation-id", filepath.FromSlash(paths[0])))
	if err != nil || string(content) != "hello" {
		t.Fatalf("stored content = %q, err=%v", content, err)
	}
	if filepath.Base(filepath.FromSlash(paths[0])) != "attachment-id-passwd" {
		t.Fatalf("filename was not sanitized: %q", paths[0])
	}
}
