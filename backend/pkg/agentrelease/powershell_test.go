package agentrelease

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Exercises the Windows installer's download/update logic under PowerShell on a
// POSIX host. Windows ACL and console behavior still require a Windows machine.
func TestPowerShellInstallerLifecycle(t *testing.T) {
	pwsh := os.Getenv("HEPHAESTUS_PWSH_TEST_BIN")
	if pwsh == "" || runtime.GOOS == "windows" {
		t.Skip("set HEPHAESTUS_PWSH_TEST_BIN on a POSIX host for PowerShell fixtures")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "node with spaces")
	_ = os.Mkdir(dir, 0700)
	config := []byte("database:\n  type: postgres\nagent:\n  id: stable-powershell-node\n")
	_ = os.WriteFile(filepath.Join(dir, "config.yaml"), config, 0600)
	installer, _ := Installer("install.ps1")
	// Shorten only the fixture deadlines; requests still use real HTTP transport.
	installer = bytes.ReplaceAll(installer, []byte("[int]$TimeoutSec = 30"), []byte("[int]$TimeoutSec = 1"))
	installer = bytes.ReplaceAll(installer, []byte("-TimeoutSec 300"), []byte("-TimeoutSec 1"))
	source := filepath.Join(root, "install.ps1")
	_ = os.WriteFile(source, installer, 0700)
	version, badChecksum, wrongBinary := "v0.4.6", false, false
	failure := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if strings.HasSuffix(req.URL.Path, ".zip") {
			switch failure {
			case "timeout":
				time.Sleep(1200 * time.Millisecond)
				return
			case "not-found":
				http.NotFound(w, req)
				return
			case "forbidden":
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}
		if req.URL.Path == "/api/v1/agent/release" {
			fmt.Fprintln(w, version)
			return
		}
		if req.URL.Path == "/api/v1/agent/install.ps1" {
			_, _ = w.Write(installer)
			return
		}
		var buffer bytes.Buffer
		z := zip.NewWriter(&buffer)
		header := &zip.FileHeader{Name: "hephaestus-scan-agent.exe", Method: zip.Deflate}
		header.SetMode(0755)
		entry, _ := z.CreateHeader(header)
		binaryVersion := version
		if wrongBinary {
			binaryVersion = "v0.3.0"
		}
		fmt.Fprintf(entry, "#!/bin/sh\nprintf 'Hephaestus scan-agent %s (cli mode)\\n'\n", binaryVersion)
		_ = z.Close()
		if strings.HasSuffix(req.URL.Path, "/SHA256SUMS.txt") {
			sum := fmt.Sprintf("%x", sha256.Sum256(buffer.Bytes()))
			if badChecksum {
				sum = strings.Repeat("0", 64)
			}
			fmt.Fprintf(w, "%s  hephaestus-agent-windows-amd64.zip\n", sum)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(buffer.Bytes())
	}))
	defer server.Close()
	run := func(script string, args ...string) (string, error) {
		command := exec.Command(pwsh, append([]string{"-NoLogo", "-NoProfile", "-File", script, "-InstallDir", dir, "-Server", server.URL, "-ReleaseBase", server.URL + "/releases/download", "-NoStart"}, args...)...)
		command.Env = append(os.Environ(), "PROCESSOR_ARCHITECTURE=AMD64", "PROCESSOR_ARCHITEW6432=", "no_proxy=127.0.0.1,localhost", "NO_PROXY=127.0.0.1,localhost")
		out, err := command.CombinedOutput()
		return string(out), err
	}
	if out, err := run(source); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	binary := filepath.Join(dir, "hephaestus-scan-agent.exe")
	initial, _ := os.ReadFile(binary)
	version = "v0.4.7"
	update := filepath.Join(dir, "install.ps1")
	if out, err := run(update, "-Action", "update"); err != nil {
		t.Fatalf("update: %v\n%s", err, out)
	}
	installed, _ := os.ReadFile(binary)
	previous, _ := os.ReadFile(binary + ".previous")
	if !bytes.Equal(initial, previous) {
		t.Fatal("previous binary not retained")
	}
	t.Run("current-directory-default", func(t *testing.T) {
		cwd := filepath.Join(root, "working directory with spaces")
		defaultDir := filepath.Join(cwd, "hephaestus-agent")
		_ = os.MkdirAll(defaultDir, 0700)
		_ = os.WriteFile(filepath.Join(defaultDir, "config.yaml"), config, 0600)
		command := exec.Command(pwsh, "-NoLogo", "-NoProfile", "-File", source, "-Server", server.URL, "-ReleaseBase", server.URL+"/releases/download", "-NoStart")
		command.Dir = cwd
		command.Env = append(os.Environ(), "PROCESSOR_ARCHITECTURE=AMD64", "PROCESSOR_ARCHITEW6432=", "no_proxy=127.0.0.1,localhost", "NO_PROXY=127.0.0.1,localhost")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("default directory installation: %v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(defaultDir, "hephaestus-scan-agent.exe")); err != nil {
			t.Fatalf("default binary was not installed in current directory: %v", err)
		}
		if data, _ := os.ReadFile(filepath.Join(defaultDir, "config.yaml")); !bytes.Equal(data, config) {
			t.Fatal("default installation changed existing configuration")
		}
	})
	for _, scenario := range []string{"checksum", "version", "incompatible"} {
		t.Run(scenario, func(t *testing.T) {
			args := []string{"-Action", "update"}
			switch scenario {
			case "checksum":
				badChecksum = true
				defer func() { badChecksum = false }()
			case "version":
				wrongBinary = true
				defer func() { wrongBinary = false }()
			case "incompatible":
				args = append(args, "-Version", "v0.3.0")
			}
			if out, err := run(update, args...); err == nil {
				t.Fatalf("unsafe update accepted: %s", out)
			}
			if data, _ := os.ReadFile(binary); !bytes.Equal(data, installed) {
				t.Fatal("failed update changed binary")
			}
		})
	}
	for _, scenario := range []struct{ name, message string }{
		{"timeout", "Request timed out"},
		{"not-found", "HTTP 404"},
		{"forbidden", "HTTP 403"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			failure = scenario.name
			defer func() { failure = "" }()
			out, err := run(update, "-Action", "update")
			if err == nil || !strings.Contains(out, scenario.message) || !strings.Contains(out, "package hephaestus-agent-windows-amd64.zip") || !strings.Contains(out, "Original error:") {
				t.Fatalf("incorrect download failure diagnosis: %v\n%s", err, out)
			}
			if scenario.name != "not-found" && strings.Contains(out, "was not found") {
				t.Fatalf("network failure described as missing package: %s", out)
			}
			if data, _ := os.ReadFile(binary); !bytes.Equal(data, installed) {
				t.Fatal("download failure changed the installed binary")
			}
		})
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "config.yaml")); !bytes.Equal(data, config) {
		t.Fatal("configuration/node identity changed")
	}
	for _, name := range []string{"run.bat", "update.bat"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
}
