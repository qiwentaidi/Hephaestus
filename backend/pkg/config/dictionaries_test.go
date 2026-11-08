package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagedDictionaryFilesKeepCustomCopiesOutsideBundle(t *testing.T) {
	previous := GetConfigBaseDir()
	root := t.TempDir()
	ForceSetConfigBaseDir(root)
	t.Cleanup(func() { ForceSetConfigBaseDir(previous) })

	bundled := filepath.Join(GetConfigResourceRoot(), "crack")
	custom := GetCustomDictDir("crack")
	for _, dir := range []string{bundled, custom} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(bundled, "ssh.txt"):  "bundled",
		filepath.Join(custom, "ssh.txt"):   "custom",
		filepath.Join(custom, "extra.txt"): "extra",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := ListManagedDictFiles("crack")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Name != "extra.txt" || files[1].Name != "ssh.txt" || !files[1].Custom {
		t.Fatalf("unexpected merged files: %+v", files)
	}
	file, err := ResolveManagedDictFile("crack", "ssh.txt")
	if err != nil || file.Path != filepath.Join(custom, "ssh.txt") {
		t.Fatalf("custom copy did not win: %+v, %v", file, err)
	}
	if err := os.Remove(file.Path); err != nil {
		t.Fatal(err)
	}
	file, err = ResolveManagedDictFile("crack", "ssh.txt")
	if err != nil || file.Path != filepath.Join(bundled, "ssh.txt") {
		t.Fatalf("bundled fallback missing: %+v, %v", file, err)
	}
}
