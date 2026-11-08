package admin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveLogFilePathRejectsTraversal(t *testing.T) {
	logsDir := t.TempDir()

	if _, err := resolveLogFilePath(logsDir, "../app.log"); err == nil {
		t.Fatalf("expected traversal path to be rejected")
	}
	if _, err := resolveLogFilePath(logsDir, "nested/app.log"); err == nil {
		t.Fatalf("expected nested path to be rejected")
	}
}

func TestResolveLogFilePathAllowsBaseFile(t *testing.T) {
	logsDir := t.TempDir()

	path, err := resolveLogFilePath(logsDir, "app.log")
	if err != nil {
		t.Fatalf("resolve should succeed: %v", err)
	}

	expected := filepath.Join(logsDir, "app.log")
	if path != expected {
		t.Fatalf("resolved path mismatch: got %q want %q", path, expected)
	}
}

func TestReadLogTailBoundsLinesAndFilters(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "app.log")
	content := strings.Join([]string{
		"[2026-04-07 10:00:00] [INFO] start",
		"[2026-04-07 10:00:01] [WARN] slow",
		"[2026-04-07 10:00:02] [ERROR] failed",
		"[2026-04-07 10:00:03] [INFO] done",
	}, "\n") + "\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	lines, truncated, err := readLogTail(logFile, 2, "ERROR", "")
	if err != nil {
		t.Fatalf("read tail failed: %v", err)
	}
	if !truncated {
		t.Fatalf("expected truncated=true when requesting fewer than total lines")
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 filtered line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "[ERROR] failed") {
		t.Fatalf("unexpected filtered content: %q", lines[0])
	}
}

func TestTailTextByLinesReturnsLastLines(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "app.log")
	content := strings.Join([]string{
		"[2026-04-07 10:00:00] [INFO] first",
		"[2026-04-07 10:00:01] [INFO] second",
		"[2026-04-07 10:00:02] [INFO] third",
		"[2026-04-07 10:00:03] [INFO] fourth",
	}, "\n") + "\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	text, truncated, err := tailTextByLines(logFile, 2)
	if err != nil {
		t.Fatalf("read tail text failed: %v", err)
	}
	if !truncated {
		t.Fatalf("expected truncated=true when requesting fewer than total lines")
	}

	want := strings.Join([]string{
		"[2026-04-07 10:00:02] [INFO] third",
		"[2026-04-07 10:00:03] [INFO] fourth",
	}, "\n")
	if text != want {
		t.Fatalf("tail text mismatch:\ngot:\n%s\nwant:\n%s", text, want)
	}
}

func TestReadLogTailRejectsInvalidLineCount(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(logFile, []byte("test\n"), 0644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	if _, _, err := readLogTail(logFile, 0, "", ""); err == nil {
		t.Fatalf("expected invalid line count to fail")
	}
	if _, _, err := readLogTail(logFile, maxLogReadLines+1, "", ""); err == nil {
		t.Fatalf("expected line count above max to fail")
	}
}
