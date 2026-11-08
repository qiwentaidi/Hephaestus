package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	if err := os.WriteFile(src, []byte("dictionary"), 0o644); err != nil {
		t.Fatalf("准备源文件失败: %v", err)
	}
	if err := os.WriteFile(dst, []byte("stale"), 0o644); err != nil {
		t.Fatalf("准备目标文件失败: %v", err)
	}
	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile 失败: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("读取目标文件失败: %v", err)
	}
	if string(got) != "dictionary" {
		t.Fatalf("目标内容期望 dictionary，实际 %q", got)
	}
}

func TestCopyFileMissingSource(t *testing.T) {
	dir := t.TempDir()
	if err := CopyFile(filepath.Join(dir, "missing.txt"), filepath.Join(dir, "dst.txt")); err == nil {
		t.Fatal("源文件不存在时应返回错误")
	}
}
