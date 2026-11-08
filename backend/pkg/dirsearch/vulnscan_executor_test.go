package dirsearch

import (
	"os"
	"path/filepath"
	"testing"

	"hephaestus-backend/pkg/config"
)

func TestParseConfigExpandsPercentExtPlaceholders(t *testing.T) {
	tempDir := t.TempDir()
	dictPath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(dictPath, []byte("admin.%EXT%\nstatic\n"), 0o600); err != nil {
		t.Fatalf("write dict: %v", err)
	}

	options, err := parseConfig(map[string]interface{}{
		"dictPaths": []interface{}{dictPath},
		"exts":      []interface{}{"php", "jsp"},
	})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}

	expected := map[string]bool{
		"admin.php": true,
		"admin.jsp": true,
		"static":    true,
	}

	if len(options.Paths) != len(expected) {
		t.Fatalf("unexpected path count: got %d want %d; paths=%v", len(options.Paths), len(expected), options.Paths)
	}

	for _, path := range options.Paths {
		if !expected[path] {
			t.Fatalf("unexpected path %q in %v", path, options.Paths)
		}
		delete(expected, path)
	}

	if len(expected) != 0 {
		t.Fatalf("missing expected paths: %v", expected)
	}
}

// useIsolatedConfigRoot 将配置根目录切换到临时目录，避免测试依赖真实托管字典目录。
func useIsolatedConfigRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	config.ForceSetConfigBaseDir(root)
	return root
}

func TestParseConfigFallsBackToDefaultDict(t *testing.T) {
	useIsolatedConfigRoot(t)

	options, err := parseConfig(map[string]interface{}{})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if len(options.Paths) < 500 || len(options.Paths) > 1200 {
		t.Fatalf("default dict path count out of expected range: got %d", len(options.Paths))
	}

	joined := make(map[string]bool, len(options.Paths))
	for _, path := range options.Paths {
		joined[path] = true
	}
	for _, want := range []string{"actuator/env", "api/v1", ".git/HEAD", "graphql", "swagger-ui.html"} {
		if !joined[want] {
			t.Fatalf("default dict missing expected modern path %q", want)
		}
	}
}

func TestParseConfigDefaultDictDisabledKeepsEmpty(t *testing.T) {
	useIsolatedConfigRoot(t)

	options, err := parseConfig(map[string]interface{}{
		"defaultDict": false,
	})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if len(options.Paths) != 0 {
		t.Fatalf("expected no paths when defaultDict is disabled and nothing else configured, got %d", len(options.Paths))
	}
}

func TestParseConfigExplicitDefaultDict(t *testing.T) {
	useIsolatedConfigRoot(t)

	options, err := parseConfig(map[string]interface{}{
		"defaultDict": true,
		"paths":       []interface{}{"custom-probe"},
	})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	found := false
	for _, path := range options.Paths {
		if path == "custom-probe" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("custom path missing from merged paths")
	}
	if len(options.Paths) < 500 {
		t.Fatalf("default dict not merged, only got %d paths", len(options.Paths))
	}
}

func TestParseConfigCustomPathsDoNotImplicitlyLoadDefaultDict(t *testing.T) {
	useIsolatedConfigRoot(t)

	options, err := parseConfig(map[string]interface{}{
		"paths": []interface{}{"/gongshu-web.zip"},
	})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if len(options.Paths) != 1 || options.Paths[0] != "/gongshu-web.zip" {
		t.Fatalf("expected only the explicitly selected custom path, got %v", options.Paths)
	}
}

func TestParseConfigDictNamesResolvesManagedDicts(t *testing.T) {
	root := useIsolatedConfigRoot(t)
	dictDir := filepath.Join(root, "config", "dirsearch")
	if err := os.MkdirAll(dictDir, 0o755); err != nil {
		t.Fatalf("mkdir dict dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dictDir, "mini.txt"), []byte("alpha\nbeta\n"), 0o600); err != nil {
		t.Fatalf("write managed dict: %v", err)
	}

	options, err := parseConfig(map[string]interface{}{
		"dictNames": []interface{}{"mini", "not-exist", "../escape"},
	})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}

	got := map[string]bool{}
	for _, path := range options.Paths {
		got[path] = true
	}
	if !got["alpha"] || !got["beta"] {
		t.Fatalf("managed dict entries missing, paths=%v", options.Paths)
	}
	// not-exist 与 ../escape 应被跳过；mini 加载成功时不触发默认字典兜底
	if len(options.Paths) != 2 {
		t.Fatalf("expected exactly 2 paths from managed dict, got %d: %v", len(options.Paths), options.Paths)
	}
}

func TestResolveDirsearchDictPathsRejectsTraversal(t *testing.T) {
	root := useIsolatedConfigRoot(t)
	dictDir := filepath.Join(root, "config", "dirsearch")
	if err := os.MkdirAll(dictDir, 0o755); err != nil {
		t.Fatalf("mkdir dict dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dictDir, "ok.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write managed dict: %v", err)
	}

	paths, missing := ResolveDirsearchDictPaths([]string{"ok", "ok.txt", "../../etc/passwd", "..", "a/b"})
	if len(paths) != 2 {
		t.Fatalf("expected 2 resolved paths for ok/ok.txt, got %v", paths)
	}
	if len(missing) != 3 {
		t.Fatalf("expected 3 rejected names, got %v", missing)
	}
	for _, path := range paths {
		if filepath.Dir(path) != dictDir {
			t.Fatalf("resolved path escapes dict dir: %s", path)
		}
	}
}

func TestCustomDirsearchDictionaryOverridesBundled(t *testing.T) {
	root := useIsolatedConfigRoot(t)
	bundled := filepath.Join(root, "config", "dirsearch")
	custom := filepath.Join(root, "custom-config", "dirsearch")
	for _, dir := range []string{bundled, custom} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(bundled, "mini.txt"): "bundled\n",
		filepath.Join(custom, "mini.txt"):  "custom\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths, missing := ResolveDirsearchDictPaths([]string{"mini"})
	if len(missing) != 0 || len(paths) != 1 || paths[0] != filepath.Join(custom, "mini.txt") {
		t.Fatalf("custom dictionary did not override bundle: paths=%v missing=%v", paths, missing)
	}
}
