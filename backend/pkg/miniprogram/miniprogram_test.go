package miniprogram

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func buildTestWxapkg(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	type fileItem struct {
		name    string
		content []byte
	}
	items := make([]fileItem, 0, len(files))
	for name, content := range files {
		items = append(items, fileItem{name: name, content: content})
	}

	index := &bytes.Buffer{}
	if err := binary.Write(index, binary.BigEndian, uint32(len(items))); err != nil {
		t.Fatal(err)
	}

	offset := 1 + 4 + 4 + 4 + 1 + 4
	for _, item := range items {
		offset += 4 + len(item.name) + 4 + 4
	}

	body := &bytes.Buffer{}
	currentOffset := offset
	for _, item := range items {
		if err := binary.Write(index, binary.BigEndian, uint32(len(item.name))); err != nil {
			t.Fatal(err)
		}
		if _, err := index.WriteString(item.name); err != nil {
			t.Fatal(err)
		}
		if err := binary.Write(index, binary.BigEndian, uint32(currentOffset)); err != nil {
			t.Fatal(err)
		}
		if err := binary.Write(index, binary.BigEndian, uint32(len(item.content))); err != nil {
			t.Fatal(err)
		}
		currentOffset += len(item.content)
		if _, err := body.Write(item.content); err != nil {
			t.Fatal(err)
		}
	}

	result := &bytes.Buffer{}
	result.WriteByte(0xBE)
	if err := binary.Write(result, binary.BigEndian, uint32(0)); err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(result, binary.BigEndian, uint32(offset)); err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(result, binary.BigEndian, uint32(body.Len())); err != nil {
		t.Fatal(err)
	}
	result.WriteByte(0xED)
	if _, err := result.Write(index.Bytes()); err != nil {
		t.Fatal(err)
	}
	if _, err := result.Write(body.Bytes()); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

func TestUnpackWxapkgWritesFilesAndExtractsMatches(t *testing.T) {
	root := t.TempDir()
	service := NewService("", root, []string{`api[_-]?key["']?\s*[:=]\s*["']?[\w-]+`})
	data := buildTestWxapkg(t, map[string][]byte{
		"/app.json":      []byte(`{"name":"demo"}`),
		"/pages/a/a.js":  []byte(`const api_key = "secret123"`),
		"/pages/a/a.png": []byte("png"),
	})

	outputDir := filepath.Join(root, "wx123", "1", "__APP__")
	files, err := service.UnpackWxapkg(data, outputDir)
	if err != nil {
		t.Fatalf("UnpackWxapkg() error = %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	if _, err := os.Stat(filepath.Join(outputDir, "app.json")); err != nil {
		t.Fatalf("expected app.json to be written: %v", err)
	}
	matches := service.ExtractMatches(files)
	if len(matches) != 1 || matches[0] != `api_key = "secret123` {
		t.Fatalf("unexpected matches: %#v", matches)
	}
}

func TestFormatContentSkipsBeautifyingLargeJavaScript(t *testing.T) {
	content := bytes.Repeat([]byte(`const value={key:"example"};`), maxJSBeautifyBytes/28+1)
	formatted, err := formatContent("bundle.js", content)
	if err != nil {
		t.Fatalf("formatContent() error = %v", err)
	}
	if !bytes.Equal(formatted, content) {
		t.Fatal("expected large JavaScript to remain unchanged")
	}
}

func TestDecompileMainPackageWritesAppJSONAtVersionRoot(t *testing.T) {
	root := t.TempDir()
	appID := "wx1234567890abcdef"
	version := "42"
	appletRoot := filepath.Join(root, "packages")
	versionDir := filepath.Join(appletRoot, appID, version)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "__APP__.wxapkg"), buildTestWxapkg(t, map[string][]byte{
		"/app.json":     []byte(`{"name":"demo"}`),
		"/pages/a/a.js": []byte(`const api_key = "secret123"`),
	}), 0644); err != nil {
		t.Fatal(err)
	}

	outputRoot := filepath.Join(root, "out")
	service := NewService(appletRoot, outputRoot, []string{`api[_-]?key["']?\s*[:=]\s*["']?[\w-]+`})
	result, err := service.Decompile(appID, version)
	if err != nil {
		t.Fatalf("Decompile() error = %v", err)
	}
	if result.FileCount != 2 {
		t.Fatalf("FileCount = %d, want 2", result.FileCount)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, appID, version, "app.json")); err != nil {
		t.Fatalf("expected app.json at version root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, appID, version, "__APP__", "app.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no nested __APP__/app.json, got err=%v", err)
	}
}

func TestScanProgramsSupportsRootWxapkgWithoutVersionDir(t *testing.T) {
	root := t.TempDir()
	appID := "wx1234567890abcdef"
	appletRoot := filepath.Join(root, "packages")
	appDir := filepath.Join(appletRoot, appID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "__APP__.wxapkg"), buildTestWxapkg(t, map[string][]byte{
		"/app.json": []byte(`{"name":"demo"}`),
	}), 0644); err != nil {
		t.Fatal(err)
	}

	outputRoot := filepath.Join(root, "out")
	service := NewService(appletRoot, outputRoot, nil)
	programs, err := service.ScanPrograms()
	if err != nil {
		t.Fatalf("ScanPrograms() error = %v", err)
	}
	if len(programs) != 1 || len(programs[0].Versions) != 1 {
		t.Fatalf("unexpected programs: %#v", programs)
	}
	if programs[0].Versions[0].Number != rootVersionName {
		t.Fatalf("version = %q, want %q", programs[0].Versions[0].Number, rootVersionName)
	}

	if _, err := service.Decompile(appID, rootVersionName); err != nil {
		t.Fatalf("Decompile(root version) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, appID, rootVersionName, "app.json")); err != nil {
		t.Fatalf("expected root version app.json: %v", err)
	}
}

func TestDecompileFailsWhenEveryPackageFails(t *testing.T) {
	root := t.TempDir()
	appID := "wx1234567890abcdef"
	version := "42"
	appletRoot := filepath.Join(root, "packages")
	versionDir := filepath.Join(appletRoot, appID, version)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "__APP__.wxapkg"), []byte("bad"), 0644); err != nil {
		t.Fatal(err)
	}

	service := NewService(appletRoot, filepath.Join(root, "out"), nil)
	if _, err := service.Decompile(appID, version); err == nil {
		t.Fatal("expected invalid wxapkg to fail")
	}
}

func TestShouldDecryptNonWxapkgHeaderAcrossPlatforms(t *testing.T) {
	if !shouldDecrypt([]byte{0}) {
		t.Fatal("expected non-0xBE header to require decryption")
	}
	if shouldDecrypt([]byte{0xBE}) {
		t.Fatal("expected 0xBE header to be treated as plaintext wxapkg")
	}
}

func TestUnpackWxapkgRejectsTraversalPath(t *testing.T) {
	service := NewService("", t.TempDir(), nil)
	data := buildTestWxapkg(t, map[string][]byte{
		"../escape.js": []byte(`alert(1)`),
	})

	if _, err := service.UnpackWxapkg(data, t.TempDir()); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}

func TestResolveAppletPathPrefersPackageCacheRoot(t *testing.T) {
	root := t.TempDir()
	wxAppletRoot := filepath.Join(root, ".wxapplet")
	legacyAppletRoot := filepath.Join(wxAppletRoot, "Applet", "profile-hash")
	packageRoot := filepath.Join(wxAppletRoot, "packages")
	versionDir := filepath.Join(packageRoot, "wx1234567890abcdef", "42")

	if err := os.MkdirAll(filepath.Join(legacyAppletRoot, "wxempty", "usr"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "__APP__.wxapkg"), []byte("placeholder"), 0644); err != nil {
		t.Fatal(err)
	}

	resolved := ResolveAppletPath(wxAppletRoot)
	if resolved != packageRoot {
		t.Fatalf("ResolveAppletPath() = %q, want %q", resolved, packageRoot)
	}

	service := NewService(wxAppletRoot, filepath.Join(root, "out"), nil)
	programs, err := service.ScanPrograms()
	if err != nil {
		t.Fatalf("ScanPrograms() error = %v", err)
	}
	if len(programs) != 1 {
		t.Fatalf("expected 1 mini program, got %d: %#v", len(programs), programs)
	}
	if programs[0].AppID != "wx1234567890abcdef" || len(programs[0].Versions) != 1 || programs[0].Versions[0].Number != "42" {
		t.Fatalf("unexpected programs: %#v", programs)
	}
}

func TestResolveAppletPathIgnoresPluginPackageCache(t *testing.T) {
	root := t.TempDir()
	wxAppletRoot := filepath.Join(root, ".wxapplet")
	packageRoot := filepath.Join(wxAppletRoot, "packages")
	appVersionDir := filepath.Join(packageRoot, "wx1234567890abcdef", "42")

	if err := os.MkdirAll(appVersionDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appVersionDir, "__APP__.wxapkg"), []byte("placeholder"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, appID := range []string{"wxplugin000000001", "wxplugin000000002", "wxplugin000000003"} {
		versionDir := filepath.Join(packageRoot, pluginDirName, appID, "1")
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(versionDir, "__PLUGINCODE__.wxapkg"), []byte("placeholder"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if resolved := ResolveAppletPath(wxAppletRoot); resolved != packageRoot {
		t.Fatalf("ResolveAppletPath(.wxapplet) = %q, want %q", resolved, packageRoot)
	}
	if resolved := ResolveAppletPath(filepath.Join(packageRoot, pluginDirName)); resolved != packageRoot {
		t.Fatalf("ResolveAppletPath(__plugin__) = %q, want %q", resolved, packageRoot)
	}
	if resolved := ResolveAppletPath(filepath.Join(packageRoot, pluginDirName, "wxplugin000000001")); resolved != packageRoot {
		t.Fatalf("ResolveAppletPath(plugin app) = %q, want %q", resolved, packageRoot)
	}
}

func TestDefaultRuleGroupsPreserveAllRules(t *testing.T) {
	if len(DefaultRuleGroups) != 4 {
		t.Fatalf("len(DefaultRuleGroups) = %d, want 4", len(DefaultRuleGroups))
	}
	if len(DefaultRules) != 19 {
		t.Fatalf("len(DefaultRules) = %d, want 19", len(DefaultRules))
	}
	for _, group := range DefaultRuleGroups {
		if !group.Enabled {
			t.Fatalf("default group %q is disabled", group.ID)
		}
		if len(group.Rules) == 0 {
			t.Fatalf("default group %q has no rules", group.ID)
		}
	}
}

func TestDefaultRulesExtractChineseIDCard(t *testing.T) {
	matches := NewGroupedMatcher(DefaultRuleGroups).FindAllMatches("身份证号：11010519491231002X")
	for _, match := range matches {
		if match.GroupID == "account-privacy" && match.RuleID == "id-card" && match.Value == "11010519491231002X" {
			return
		}
	}
	t.Fatalf("expected ID card match in account-privacy group, got %#v", matches)
}

func TestGroupedMatcherKeepsCrossGroupAttribution(t *testing.T) {
	matcher := NewGroupedMatcher([]RuleGroup{
		{
			ID:      "first",
			Name:    "First",
			Enabled: true,
			Rules: []ExtractionRule{{
				ID:      "token-a",
				Pattern: `token=[a-z]+`,
				Enabled: true,
			}},
		},
		{
			ID:      "second",
			Name:    "Second",
			Enabled: true,
			Rules: []ExtractionRule{{
				ID:      "token-b",
				Pattern: `token=[a-z]+`,
				Enabled: true,
			}},
		},
	})

	matches := matcher.FindAllMatches("const value = 'token=secret'")
	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2: %#v", len(matches), matches)
	}
	if matches[0].GroupID != "first" || matches[0].RuleID != "token-a" {
		t.Fatalf("unexpected first match: %#v", matches[0])
	}
	if matches[1].GroupID != "second" || matches[1].RuleID != "token-b" {
		t.Fatalf("unexpected second match: %#v", matches[1])
	}
}

func TestDecompileWritesAndReadsGroupedMatches(t *testing.T) {
	root := t.TempDir()
	appID := "wx1234567890abcdef"
	version := "42"
	appletRoot := filepath.Join(root, "packages")
	versionDir := filepath.Join(appletRoot, appID, version)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "__APP__.wxapkg"), buildTestWxapkg(t, map[string][]byte{
		"/app.js": []byte(`const api_key = "secret123"; const mobile = "13800138000"`),
	}), 0644); err != nil {
		t.Fatal(err)
	}

	groups := []RuleGroup{
		{
			ID:      "credentials",
			Name:    "Credentials",
			Enabled: true,
			Rules: []ExtractionRule{{
				ID:      "api-key",
				Pattern: `api[_-]?key["']?\s*[:=]\s*["']?[\w-]+`,
				Enabled: true,
			}},
		},
		{
			ID:      "privacy",
			Name:    "Privacy",
			Enabled: true,
			Rules: []ExtractionRule{{
				ID:      "phone",
				Pattern: `1[3-9]\d{9}`,
				Enabled: true,
			}},
		},
	}
	outputRoot := filepath.Join(root, "out")
	service := NewServiceWithRuleGroups(appletRoot, outputRoot, groups)
	result, err := service.Decompile(appID, version)
	if err != nil {
		t.Fatalf("Decompile() error = %v", err)
	}
	if result.MatchedCount != 2 || len(result.MatchGroups) != 2 {
		t.Fatalf("unexpected grouped result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, appID, version, groupedMatchesFileName)); err != nil {
		t.Fatalf("expected matches.json: %v", err)
	}

	stored, err := service.ReadGroupedMatches(appID, version)
	if err != nil {
		t.Fatalf("ReadGroupedMatches() error = %v", err)
	}
	if stored.Total != 2 || len(stored.Groups) != 2 {
		t.Fatalf("unexpected stored matches: %#v", stored)
	}
}

func TestReadGroupedMatchesFallsBackToLegacyFile(t *testing.T) {
	root := t.TempDir()
	appID := "wx1234567890abcdef"
	version := "42"
	outputDir := filepath.Join(root, appID, version)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, matchesFileName), []byte("legacy-secret\n"), 0644); err != nil {
		t.Fatal(err)
	}

	service := NewServiceWithRuleGroups("", root, DefaultRuleGroups)
	result, err := service.ReadGroupedMatches(appID, version)
	if err != nil {
		t.Fatalf("ReadGroupedMatches() error = %v", err)
	}
	if result.Total != 1 || len(result.Groups) != 1 || result.Groups[0].ID != "unclassified" {
		t.Fatalf("unexpected legacy fallback: %#v", result)
	}
}
