package logger

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestFileFirstWriterWritesFileWhenConsoleFails(t *testing.T) {
	var file bytes.Buffer
	writer := fileFirstWriter{
		file:    &file,
		console: failingWriter{},
	}

	n, err := writer.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("expected file write to succeed: %v", err)
	}
	if n != len("hello\n") {
		t.Fatalf("write size mismatch: got %d want %d", n, len("hello\n"))
	}
	if got := file.String(); got != "hello\n" {
		t.Fatalf("file content mismatch: got %q", got)
	}
}

func TestFileFirstWriterReportsFileWriteFailure(t *testing.T) {
	var console bytes.Buffer
	writer := fileFirstWriter{
		file:    failingWriter{},
		console: &console,
	}

	if _, err := writer.Write([]byte("hello\n")); err == nil {
		t.Fatalf("expected file write failure")
	}
	if got := console.String(); got != "hello\n" {
		t.Fatalf("console should still receive best-effort output, got %q", got)
	}
}

func TestCleanupExpiredLogsRemovesOnlyLogsOlderThanSevenDays(t *testing.T) {
	logsDir := t.TempDir()
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)

	files := map[string]string{
		"2026-06-16.log": "today",
		"2026-06-09.log": "keep cutoff day",
		"2026-06-08.log": "delete old",
		"README.txt":     "ignore non-log",
		"latest.log":     "ignore malformed log name",
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(logsDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	cleanupExpiredLogs(logsDir, now)

	assertFileExists(t, filepath.Join(logsDir, "2026-06-16.log"))
	assertFileExists(t, filepath.Join(logsDir, "2026-06-09.log"))
	assertFileNotExists(t, filepath.Join(logsDir, "2026-06-08.log"))
	assertFileExists(t, filepath.Join(logsDir, "README.txt"))
	assertFileExists(t, filepath.Join(logsDir, "latest.log"))
}

func TestParseLogDate(t *testing.T) {
	logDate, ok := parseLogDate("2026-06-16.log")
	if !ok {
		t.Fatalf("expected dated log file to parse")
	}
	if got, want := logDate.Format("2006-01-02"), "2026-06-16"; got != want {
		t.Fatalf("parsed date = %q, want %q", got, want)
	}

	if _, ok := parseLogDate("2026-06-16.migrated.1.log"); ok {
		t.Fatalf("migrated log should not be parsed as dated runtime log")
	}
	if _, ok := parseLogDate("notes.txt"); ok {
		t.Fatalf("non-log file should not be parsed")
	}
}

func TestCapturedWritesSourceTaggedLogLine(t *testing.T) {
	var out bytes.Buffer
	logger := &Logger{
		date:   time.Now().Format("2006-01-02"),
		writer: &out,
	}

	logger.captured(INFO, "STDOUT", "\x1b[34mscanner output\x1b[0m\n")

	got := out.String()
	if !strings.Contains(got, "[INFO] [STDOUT] scanner output\n") {
		t.Fatalf("captured log line missing source tag: %q", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("captured log line should strip ANSI escape sequences: %q", got)
	}
}

func TestNewSourceWriterRoutesExternalOutput(t *testing.T) {
	var out bytes.Buffer
	logger := &Logger{
		date:   time.Now().Format("2006-01-02"),
		writer: &out,
	}

	writer := logger.SourceWriter("FINGERS", INFO)
	if n, err := writer.Write([]byte("\x1b[34mfingerprint output\x1b[0m\n")); err != nil || n == 0 {
		t.Fatalf("Write() = %d, %v; want non-zero nil error", n, err)
	}

	got := out.String()
	if !strings.Contains(got, "[INFO] [FINGERS] fingerprint output\n") {
		t.Fatalf("external log line missing source tag: %q", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("external log line should strip ANSI escape sequences: %q", got)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s to exist: %v", path, err)
	}
}

func assertFileNotExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file %s to be removed, stat err = %v", path, err)
	}
}
