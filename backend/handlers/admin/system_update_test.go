package admin

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestUnzipRejectsTraversal(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "update.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	entry, err := zw.Create("../escaped.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("must not be written")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "extracted")
	if err := unzip(archivePath, dest); err == nil {
		t.Fatal("accepted a path traversal archive")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("path traversal created an outside file: %v", err)
	}
}
