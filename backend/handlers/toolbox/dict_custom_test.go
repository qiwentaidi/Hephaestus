package toolbox

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/pkg/config"
)

func callDictionaryHandler(method, target, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	handler(ctx)
	return recorder
}

func TestCustomDictionaryLifecyclePreservesBundledFile(t *testing.T) {
	previous := config.GetConfigBaseDir()
	root := t.TempDir()
	config.ForceSetConfigBaseDir(root)
	t.Cleanup(func() { config.ForceSetConfigBaseDir(previous) })

	bundledDir := filepath.Join(config.GetConfigResourceRoot(), "crack")
	customDir := config.GetCustomDictDir("crack")
	for _, dir := range []string{bundledDir, customDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bundledPath := filepath.Join(bundledDir, "ssh.txt")
	if err := os.WriteFile(bundledPath, []byte("bundled:entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	add := callDictionaryHandler(http.MethodPost, "/dict/crack/content?create=true",
		`{"name":"extra.txt","source":"crack","content":"custom:entry\n","protocol":"SSH","group":"crack"}`,
		SaveCrackDictContentHandler)
	if add.Code != http.StatusOK {
		t.Fatalf("create failed: %d %s", add.Code, add.Body.String())
	}
	if _, err := os.Stat(filepath.Join(customDir, "extra.txt")); err != nil {
		t.Fatalf("custom file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundledDir, "extra.txt")); !os.IsNotExist(err) {
		t.Fatalf("create wrote into updateable bundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(customDir, "_meta.json")); err != nil {
		t.Fatalf("custom metadata missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundledDir, "_meta.json")); !os.IsNotExist(err) {
		t.Fatalf("metadata was written into updateable bundle: %v", err)
	}

	duplicate := callDictionaryHandler(http.MethodPost, "/dict/crack/content?create=true",
		`{"name":"ssh.txt","source":"crack","content":"wrong:entry\n","protocol":"SSH"}`,
		SaveCrackDictContentHandler)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("bundled name collision should fail: %d %s", duplicate.Code, duplicate.Body.String())
	}

	edit := callDictionaryHandler(http.MethodPost, "/dict/crack/content",
		`{"name":"ssh.txt","source":"crack","content":"edited:entry\n","protocol":"SSH"}`,
		SaveCrackDictContentHandler)
	if edit.Code != http.StatusOK {
		t.Fatalf("edit failed: %d %s", edit.Code, edit.Body.String())
	}
	if content, err := os.ReadFile(bundledPath); err != nil || string(content) != "bundled:entry\n" {
		t.Fatalf("bundled file was modified: %q, %v", content, err)
	}

	list := callDictionaryHandler(http.MethodGet, "/dict/crack", "", ListCrackDictHandler)
	var payload struct {
		Data struct {
			Dicts []DictionaryInfo `json:"dicts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Dicts) != 2 || payload.Data.Dicts[1].Name != "ssh.txt" || payload.Data.Dicts[1].Origin != "custom" {
		t.Fatalf("unexpected dictionary list: %+v", payload.Data.Dicts)
	}

	removed := callDictionaryHandler(http.MethodDelete, "/dict/crack/content?source=crack&name=ssh.txt", "", DeleteCrackDictContentHandler)
	if removed.Code != http.StatusOK {
		t.Fatalf("custom override deletion failed: %d %s", removed.Code, removed.Body.String())
	}
	read := callDictionaryHandler(http.MethodGet, "/dict/crack/content?source=crack&name=ssh.txt", "", GetCrackDictContentHandler)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), "bundled:entry") {
		t.Fatalf("bundled fallback missing: %d %s", read.Code, read.Body.String())
	}
	blocked := callDictionaryHandler(http.MethodDelete, "/dict/crack/content?source=crack&name=ssh.txt", "", DeleteCrackDictContentHandler)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("bundled deletion should be forbidden: %d %s", blocked.Code, blocked.Body.String())
	}
}

func TestEnsureCrackDictDirDoesNotImportLegacyFiles(t *testing.T) {
	previous := config.GetConfigBaseDir()
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	config.ForceSetConfigBaseDir(root)
	t.Cleanup(func() { config.ForceSetConfigBaseDir(previous) })

	legacyDir := filepath.Join(home, ".config", "TscanPlus", "config", "CrackDict")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "ssh-extra.txt"), []byte("legacy:entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for run := 0; run < 2; run++ {
		if err := ensureCrackDictDir(); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(config.GetCustomDictDir("crack"))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("legacy files imported on run %d: %v", run, entries)
		}
		if err := os.Remove(config.GetCustomDictDir("crack")); err != nil {
			t.Fatal(err)
		}
	}
}
