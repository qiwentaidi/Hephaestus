package agentrelease

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
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

	"gopkg.in/yaml.v3"
	"hephaestus-backend/pkg/config"
)

func TestInstallersAndExactRelease(t *testing.T) {
	r := Current()
	if r.AgentVersion != r.ServerVersion || r.Compatibility != "exact" || len(r.Packages) != 5 {
		t.Fatalf("invalid contract: %+v", r)
	}
	for _, p := range r.Packages {
		if strings.Contains(p.URL, "latest") || !strings.Contains(p.URL, Repository+"/releases/download/"+r.AgentVersion+"/") {
			t.Fatalf("not version pinned: %s", p.URL)
		}
	}
	bat, err := Installer("install.bat")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(bat)), "\r\n")
	payload, err := base64.StdEncoding.DecodeString(lines[len(lines)-1])
	if err != nil {
		t.Fatal(err)
	}
	ps, _ := Installer("install.ps1")
	if !bytes.Equal(payload, ps) {
		t.Fatal("BAT does not contain its complete PowerShell installer")
	}
}

// Exercise real terminal input, including invalid bytes and hidden passwords,
// then parse the installed YAML to check that quoting preserved the values.
func TestBashInstallerInteractiveUTF8(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash installer is for Linux/macOS")
	}
	for _, program := range []string{"bash", "python3", "curl", "unzip", "iconv", "script"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Skip(program + " unavailable")
		}
	}
	root := t.TempDir()
	installer, _ := Installer("install.sh")
	script := filepath.Join(root, "install.sh")
	if err := os.WriteFile(script, installer, 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("python3", "testdata/interactive_utf8.py", script, root)
	command.Env = append(os.Environ(), "HEPHAESTUS_AGENT_RELEASE_BASE=", "HEPHAESTUS_SERVER_URL=")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("interactive installation failed: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(root, "node", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var installed config.Config
	if err := yaml.Unmarshal(data, &installed); err != nil {
		t.Fatalf("installed YAML is invalid: %v", err)
	}
	if installed.Database.Host != "10.211.55.2" || installed.Database.User != "postgres" || installed.Database.Password != "测试'fixture" || installed.Database.SSLMode != "disable" || installed.Database.Port != "5432" || strings.Join(installed.Agent.Roles, ",") != "ai,scan" {
		t.Fatal("interactive configuration values changed")
	}
	if installed.Agent.AIConcurrency != 4 || installed.Agent.ScanConcurrency != 3 {
		t.Fatalf("installed Worker quotas: %+v", installed.Agent)
	}
}

// Stop at the version query so default directory selection is exercised without
// installing a service or touching any privileged host directory.
func TestBashInstallerDefaultDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash installer is for Linux/macOS")
	}
	root := t.TempDir()
	script := filepath.Join(root, "install.sh")
	installer, _ := Installer("install.sh")
	if err := os.WriteFile(script, installer, 0700); err != nil {
		t.Fatal(err)
	}
	commands := filepath.Join(root, "commands")
	_ = os.Mkdir(commands, 0700)
	for name, body := range map[string]string{
		"id":    "printf '0\\n'",
		"uname": "case \"$1\" in -s) printf 'Linux\\n';; -m) printf 'aarch64\\n';; esac",
		"curl":  "printf '000'; exit 7",
	} {
		if err := os.WriteFile(filepath.Join(commands, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, service := range []bool{false, true} {
		t.Run(fmt.Sprint("service=", service), func(t *testing.T) {
			cwd := filepath.Join(root, t.Name()[strings.LastIndex(t.Name(), "/")+1:]+" with spaces")
			_ = os.Mkdir(cwd, 0700)
			args := []string{script, "--server", "http://fixture.invalid", "--no-start"}
			if service {
				args = append(args, "--service")
			}
			command := exec.Command("bash", args...)
			command.Dir = cwd
			command.Env = append(os.Environ(), "PATH="+commands+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(out), "compatible version query: cannot connect") {
				t.Fatalf("fixture must stop before service installation: %v %s", err, out)
			}
			if info, err := os.Stat(filepath.Join(cwd, "hephaestus-agent")); err != nil || !info.IsDir() {
				t.Fatalf("default install directory does not follow current directory: %v", err)
			}
		})
	}
}

// Exercise real HTTP downloads with a tiny executable fixture for the SDK.
func TestBashInstallerLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash installer is for Linux/macOS")
	}
	for _, program := range []string{"bash", "unzip", "curl", "iconv"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Skip(program + " unavailable")
		}
	}
	root := t.TempDir()
	dir := filepath.Join(root, "node with spaces")
	source := filepath.Join(root, "install.sh")
	installer, _ := Installer("install.sh")
	if err := os.WriteFile(source, installer, 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "provided.yaml")
	originalConfig := []byte("database:\n  type: postgres\n  password: fixture\nagent:\n  id: stable-fixture-node\n")
	if err := os.WriteFile(config, originalConfig, 0600); err != nil {
		t.Fatal(err)
	}
	version, embeddedVersion, badChecksum, missingPackage := "v0.4.6", "v0.4.6", false, false
	failure := ""
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests = append(requests, req.URL.Path)
		if (failure == "version-timeout" && req.URL.Path == "/api/v1/agent/release") ||
			(failure == "checksum-timeout" && strings.HasSuffix(req.URL.Path, "/SHA256SUMS.txt")) ||
			(failure == "updater-timeout" && req.URL.Path == "/api/v1/agent/install.sh") ||
			(failure == "package-timeout" && strings.HasSuffix(req.URL.Path, ".zip")) {
			time.Sleep(1200 * time.Millisecond)
			return
		}
		if strings.HasSuffix(req.URL.Path, ".zip") {
			switch failure {
			case "reset":
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
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
		if req.URL.Path == "/api/v1/agent/install.sh" {
			_, _ = w.Write(installer)
			return
		}
		if missingPackage {
			http.NotFound(w, req)
			return
		}
		var buffer bytes.Buffer
		z := zip.NewWriter(&buffer)
		entry, _ := z.Create("hephaestus-scan-agent")
		fmt.Fprintf(entry, "#!/usr/bin/env bash\nif [[ $1 == version ]]; then printf 'Hephaestus scan-agent %s (cli mode)\\n'; else printf 'fixture ran\\n'; fi\n", embeddedVersion)
		_ = z.Close()
		if strings.HasSuffix(req.URL.Path, "/SHA256SUMS.txt") {
			sum := fmt.Sprintf("%x", sha256.Sum256(buffer.Bytes()))
			if badChecksum {
				sum = strings.Repeat("0", 64)
			}
			fmt.Fprintf(w, "%s  hephaestus-agent-%s-%s.zip\n", sum, runtime.GOOS, runtime.GOARCH)
			return
		}
		_, _ = w.Write(buffer.Bytes())
	}))
	defer server.Close()
	// Use real curl against the HTTP fixture, with a short deadline for network
	// failures. This does not replace curl's transport errors with mocked output.
	curl, _ := exec.LookPath("curl")
	commands := filepath.Join(root, "network-commands")
	_ = os.Mkdir(commands, 0700)
	_ = os.WriteFile(filepath.Join(commands, "curl"), []byte("#!/bin/sh\nexec \"$TEST_REAL_CURL\" \"$@\" --max-time 1 --retry 0\n"), 0700)
	run := func(script string, args ...string) (string, error) {
		command := exec.Command("bash", append([]string{script, "--install-dir", dir, "--no-start"}, args...)...)
		command.Env = append(os.Environ(), "HEPHAESTUS_AGENT_RELEASE_BASE=", "HEPHAESTUS_SERVER_URL=", "PATH="+commands+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_REAL_CURL="+curl, "no_proxy=127.0.0.1,localhost", "NO_PROXY=127.0.0.1,localhost")
		out, err := command.CombinedOutput()
		return string(out), err
	}
	if out, err := run(source, "--server", server.URL, "--release-base", server.URL+"/releases/download", "--config", config); err != nil {
		t.Fatalf("first install: %v\n%s", err, out)
	}
	binary := filepath.Join(dir, "hephaestus-scan-agent")
	initial, _ := os.ReadFile(binary)
	if info, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("configuration is not owner-only")
	}
	resource := filepath.Join(dir, "config", "keep.txt")
	_ = os.WriteFile(resource, []byte("keep"), 0600)
	update := filepath.Join(dir, "install.sh")
	assertPreserved := func() {
		t.Helper()
		cfg, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
		if !bytes.Equal(cfg, originalConfig) {
			t.Fatal("configuration/node identity changed")
		}
		if data, _ := os.ReadFile(resource); string(data) != "keep" {
			t.Fatal("resources changed")
		}
	}

	version, embeddedVersion = "v0.4.7", "v0.4.7"
	if out, err := run(update, "--update"); err != nil {
		t.Fatalf("update: %v\n%s", err, out)
	}
	assertPreserved()
	previous, _ := os.ReadFile(binary + ".previous")
	if !bytes.Equal(previous, initial) {
		t.Fatal("previous binary missing")
	}
	installed, _ := os.ReadFile(binary)
	assertUnchanged := func() {
		t.Helper()
		actual, _ := os.ReadFile(binary)
		if !bytes.Equal(actual, installed) {
			t.Fatal("failed update replaced working binary")
		}
		assertPreserved()
	}
	for _, scenario := range []string{"checksum", "binary-version", "unpublished", "incompatible", "running"} {
		t.Run(scenario, func(t *testing.T) {
			args := []string{"--update"}
			switch scenario {
			case "checksum":
				badChecksum = true
				defer func() { badChecksum = false }()
			case "binary-version":
				embeddedVersion = "v0.3.0"
				defer func() { embeddedVersion = version }()
			case "unpublished":
				missingPackage = true
				defer func() { missingPackage = false }()
			case "incompatible":
				args = append(args, "--version", "v0.3.0")
			case "running":
				_ = os.Mkdir(filepath.Join(dir, ".run-lock"), 0700)
				defer os.Remove(filepath.Join(dir, ".run-lock"))
			}
			if out, err := run(update, args...); err == nil {
				t.Fatalf("unsafe update accepted: %s", out)
			} else if scenario == "unpublished" && !strings.Contains(out, "HTTP 404") {
				t.Fatalf("missing package was not diagnosed as HTTP 404: %s", out)
			}
			assertUnchanged()
		})
	}
	for _, scenario := range []struct {
		name, resource, message string
	}{
		{"package-timeout", "package hephaestus-agent-", "request timed out"},
		{"version-timeout", "compatible version query", "request timed out"},
		{"checksum-timeout", "checksum file", "request timed out"},
		{"updater-timeout", "updater script", "request timed out"},
		{"reset", "package hephaestus-agent-", "connection interrupted or reset"},
		{"forbidden", "package hephaestus-agent-", "HTTP 403"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			failure = scenario.name
			defer func() { failure = "" }()
			out, err := run(update, "--update")
			if err == nil || !strings.Contains(out, scenario.resource) || !strings.Contains(out, scenario.message) || strings.Contains(out, "was not found") || strings.Contains(out, "has no") {
				t.Fatalf("incorrect download failure diagnosis: %v\n%s", err, out)
			}
			assertUnchanged()
			if _, err := os.Stat(filepath.Join(dir, ".install-lock")); !os.IsNotExist(err) {
				t.Fatal("download failure left the installation locked")
			}
		})
	}
	for _, request := range requests {
		if strings.Contains(request, "latest") {
			t.Fatal("installer requested latest")
		}
	}
	t.Run("invalid-config-encoding", func(t *testing.T) {
		path := filepath.Join(dir, "config.yaml")
		invalid := append(append([]byte{}, originalConfig...), 0xff)
		if err := os.WriteFile(path, invalid, 0600); err != nil {
			t.Fatal(err)
		}
		defer os.WriteFile(path, originalConfig, 0600)
		for _, action := range []string{"--update", "--run"} {
			out, err := run(update, action)
			if err == nil || !strings.Contains(out, "config.yaml contains invalid UTF-8") {
				t.Fatalf("invalid configuration was not diagnosed: %v\n%s", err, out)
			}
			if data, _ := os.ReadFile(binary); !bytes.Equal(data, installed) {
				t.Fatal("invalid configuration changed the installed binary")
			}
			if data, _ := os.ReadFile(path); !bytes.Equal(data, invalid) {
				t.Fatal("invalid existing configuration was overwritten")
			}
		}
	})
	// A managed node may only be replaced after the service acknowledges exit.
	// Mock systemctl and id; no host service or privilege is touched by this test.
	t.Run("service-stop-and-rollback", func(t *testing.T) {
		commands := filepath.Join(root, "fake-service-commands")
		_ = os.Mkdir(commands, 0700)
		_ = os.WriteFile(filepath.Join(commands, "id"), []byte("#!/bin/sh\nprintf '0\\n'\n"), 0700)
		mock := "#!/bin/sh\ncase \"$1\" in\nshow) if [ \"$4\" = ExecStart ]; then printf '%s\\n' \"$TEST_NODE_BINARY\"; else printf 'inactive\\n'; fi;;\nstop) if [ \"$TEST_STOP_FAIL\" = 1 ]; then exit 1; fi;;\nstart) exit 1;;\nesac\n"
		_ = os.WriteFile(filepath.Join(commands, "systemctl"), []byte(mock), 0700)
		_ = os.WriteFile(filepath.Join(dir, ".service-name"), []byte("hephaestus-scan-agent\n"), 0600)
		defer os.Remove(filepath.Join(dir, ".service-name"))
		version, embeddedVersion = "v0.4.8", "v0.4.8"
		for _, failStop := range []string{"1", "0"} {
			command := exec.Command("bash", update, "--update", "--install-dir", dir)
			command.Env = append(os.Environ(), "PATH="+commands+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_NODE_BINARY="+binary, "TEST_STOP_FAIL="+failStop)
			if out, err := command.CombinedOutput(); err == nil {
				t.Fatalf("failed service transition accepted: %s", out)
			}
			assertUnchanged()
			marker, _ := os.ReadFile(filepath.Join(dir, ".installed-version"))
			if strings.TrimSpace(string(marker)) != "v0.4.7" {
				t.Fatal("failed update changed version marker")
			}
		}
	})
	command := exec.Command("bash", filepath.Join(dir, "run.sh"))
	if out, err := command.CombinedOutput(); err != nil || !strings.Contains(string(out), "fixture ran") {
		t.Fatalf("run failed: %v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".run-lock")); !os.IsNotExist(err) {
		t.Fatal("run lock left behind after normal exit")
	}
}
