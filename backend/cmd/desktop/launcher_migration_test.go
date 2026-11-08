package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLauncherJSONRecoversInvalidLocalConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "navigation.json")
	if err := os.WriteFile(path, []byte(`[{"name":"bad","children":[{"path":"C:\Windows\System32\cmd.exe"}]}]`), 0644); err != nil {
		t.Fatalf("write invalid launcher config: %v", err)
	}

	var groups []LauncherGroup
	if err := readLauncherJSON(path, &groups); err != nil {
		t.Fatalf("read launcher config: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected invalid config to reset to empty groups, got %d", len(groups))
	}

	matches, err := filepath.Glob(path + ".invalid-*")
	if err != nil {
		t.Fatalf("glob backup: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one invalid config backup, got %d", len(matches))
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read reset launcher config: %v", err)
	}
	if strings.TrimSpace(string(current)) != "[]" {
		t.Fatalf("expected reset launcher config to be [], got %q", strings.TrimSpace(string(current)))
	}
}

func TestEnsureLegacySlackLauncherMigrated(t *testing.T) {
	if _, err := os.Stat(legacySlackLauncherNavigationFile()); err != nil {
		t.Skip("legacy Slack launcher navigation file not present")
	}
	if _, err := os.Stat(legacySlackLauncherJDKFile()); err != nil {
		t.Skip("legacy Slack launcher jdk file not present")
	}

	backupFile := func(path string) []byte {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			t.Fatalf("backup %s: %v", path, err)
		}
		return data
	}
	restoreFile := func(path string, data []byte) {
		if data == nil {
			_ = os.Remove(path)
			return
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("restore %s: %v", path, err)
		}
	}

	navBackup := backupFile(launcherNavigationFile())
	jdkBackup := backupFile(launcherJDKFile())
	markerBackup := backupFile(launcherMigrationMarkerFile())
	defer restoreFile(launcherNavigationFile(), navBackup)
	defer restoreFile(launcherJDKFile(), jdkBackup)
	defer restoreFile(launcherMigrationMarkerFile(), markerBackup)

	if err := resetLauncherMigrationState(); err != nil {
		t.Fatalf("reset launcher migration state: %v", err)
	}

	if err := ensureLegacySlackLauncherMigrated(); err != nil {
		t.Fatalf("migrate launcher config: %v", err)
	}

	groups, err := readLauncherGroupsRaw()
	if err != nil {
		t.Fatalf("read migrated groups: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("expected migrated launcher groups")
	}
	if strings.TrimSpace(groups[0].Name) == "" {
		t.Fatal("expected first launcher group to have a name")
	}

	jdks, err := readLauncherJDKsRaw()
	if err != nil {
		t.Fatalf("read migrated jdks: %v", err)
	}
	if len(jdks) == 0 {
		t.Fatal("expected migrated launcher jdks")
	}

	navigationBytes, err := os.ReadFile(launcherNavigationFile())
	if err != nil {
		t.Fatalf("read migrated navigation file: %v", err)
	}
	if strings.TrimSpace(string(navigationBytes)) == "null" {
		t.Fatal("expected migrated navigation file to not be null")
	}

	jdkBytes, err := os.ReadFile(launcherJDKFile())
	if err != nil {
		t.Fatalf("read migrated jdk file: %v", err)
	}
	if strings.TrimSpace(string(jdkBytes)) == "null" {
		t.Fatal("expected migrated jdk file to not be null")
	}

	markerBytes, err := os.ReadFile(launcherMigrationMarkerFile())
	if err != nil {
		t.Fatalf("read migration marker: %v", err)
	}
	if !strings.Contains(string(markerBytes), launcherMigrationSourceSlack) {
		t.Fatal("expected migration marker to record Slack import source")
	}
}
