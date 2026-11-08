package portscan

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	appconfig "hephaestus-backend/pkg/config"
)

func TestParseCrackDictsPrefersCustomProtocolDictionary(t *testing.T) {
	previous := appconfig.GetConfigBaseDir()
	root := t.TempDir()
	appconfig.ForceSetConfigBaseDir(root)
	t.Cleanup(func() { appconfig.ForceSetConfigBaseDir(previous) })

	bundled := filepath.Join(appconfig.GetConfigResourceRoot(), "crack")
	custom := appconfig.GetCustomDictDir("crack")
	for _, dir := range []string{bundled, custom} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(bundled, "ssh.txt"):           "builtin:password\n",
		filepath.Join(bundled, "ftp.txt"):           "ftp:password\n",
		filepath.Join(custom, "my-credentials.txt"): "added:password\n",
		filepath.Join(custom, "_meta.json"):         `{"my-credentials.txt":{"protocol":"SSH"}}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	entries := parseCrackDicts("ssh", map[string]interface{}{"defaultDict": true})
	if !slices.Equal(entries, []string{"added:password"}) {
		t.Fatalf("custom dictionary should replace built-in dictionary for SSH: %v", entries)
	}

	entries = parseCrackDicts("ftp", map[string]interface{}{"defaultDict": true})
	if !slices.Equal(entries, []string{"ftp:password"}) {
		t.Fatalf("built-in dictionary should be used when FTP has no custom dictionary: %v", entries)
	}

	if err := os.WriteFile(filepath.Join(custom, "ssh-extra.txt"), []byte("second:password\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries = parseCrackDicts("ssh", map[string]interface{}{"defaultDict": true})
	if len(entries) != 2 || !slices.Contains(entries, "added:password") || !slices.Contains(entries, "second:password") {
		t.Fatalf("all custom SSH dictionaries should be loaded: %v", entries)
	}
}

func TestParseCrackDictsKeepsSavedDictionaryOrder(t *testing.T) {
	previous := appconfig.GetConfigBaseDir()
	appconfig.ForceSetConfigBaseDir(t.TempDir())
	t.Cleanup(func() { appconfig.ForceSetConfigBaseDir(previous) })

	custom := appconfig.GetCustomDictDir("crack")
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(custom, "ssh.txt")
	if err := os.WriteFile(path, []byte("# SSH\r\nroot:root\r\nroot:2\r\n\r\nroot:123456\r\nroot:2\r\nadmin:admin\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := map[string]interface{}{
		"defaultDict": true,
		"username":    "root\noperator",
		"password":    "2\n",
	}
	want := []string{"root:root", "root:2", "root:123456", "admin:admin", "root:", "operator:2", "operator:"}
	for range 20 {
		if got := parseCrackDicts("ssh", config); !slices.Equal(got, want) {
			t.Fatalf("saved dictionary order must survive merging and deduplication: got %v, want %v", got, want)
		}
	}

	// Editing the dictionary while the service runs must affect the next load.
	if err := os.WriteFile(path, []byte("root:3\nroot:2\nroot:3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := parseCrackDicts("ssh", map[string]interface{}{"defaultDict": true}); !slices.Equal(got, []string{"root:3", "root:2"}) {
		t.Fatalf("next load must read the updated dictionary in order: %v", got)
	}
}
