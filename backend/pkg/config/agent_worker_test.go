package config

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentWorkerCapacityConfiguration(t *testing.T) {
	for _, key := range []string{"AGENT_ROLES", "AGENT_AI_CONCURRENCY", "AGENT_CONCURRENCY"} {
		t.Setenv(key, "")
	}
	external, err := ResolveAgentWorkers(AgentConfig{}, false, 3)
	if err != nil || external.AIEnabled || !external.ScanEnabled || external.AIConcurrency != 4 || external.ScanConcurrency != 3 {
		t.Fatalf("external defaults: %+v %v", external, err)
	}
	external, err = ResolveAgentWorkers(AgentConfig{Roles: []string{"ai", "scan"}, AIConcurrency: 1, ScanConcurrency: 1}, false, 3)
	if err != nil || !external.AIEnabled || !external.ScanEnabled || external.AIConcurrency != 1 || external.ScanConcurrency != 1 {
		t.Fatalf("existing external capacity: %+v %v", external, err)
	}
	local, err := ResolveAgentWorkers(AgentConfig{}, true, 3)
	if err != nil || !local.AIEnabled || !local.ScanEnabled || local.AIConcurrency != 4 || local.ScanConcurrency != 3 {
		t.Fatalf("local defaults: %+v %v", local, err)
	}
	local, err = ResolveAgentWorkers(AgentConfig{Roles: []string{"ai"}, AIConcurrency: 4, ScanConcurrency: 7}, true, 3)
	if err != nil || !local.AIEnabled || local.ScanEnabled || local.AIConcurrency != 4 || local.ScanConcurrency != 7 {
		t.Fatalf("YAML capacity: %+v %v", local, err)
	}
	t.Setenv("AGENT_ROLES", "ai,scan")
	t.Setenv("AGENT_AI_CONCURRENCY", "2")
	t.Setenv("AGENT_CONCURRENCY", "1")
	shrunk, err := ResolveAgentWorkers(AgentConfig{AIConcurrency: 4, ScanConcurrency: 7}, false, 3)
	if err != nil || !shrunk.AIEnabled || !shrunk.ScanEnabled || shrunk.AIConcurrency != 2 || shrunk.ScanConcurrency != 1 {
		t.Fatalf("environment capacity: %+v %v", shrunk, err)
	}
	for _, value := range []string{"0", "-1", "invalid"} {
		t.Setenv("AGENT_CONCURRENCY", value)
		if _, err := ResolveAgentWorkers(AgentConfig{}, false, 1); err == nil {
			t.Fatalf("accepted bad capacity %s", value)
		}
	}
	t.Setenv("AGENT_CONCURRENCY", "")
	t.Setenv("AGENT_ROLES", "unknown")
	if _, err := ResolveAgentWorkers(AgentConfig{}, false, 1); err == nil {
		t.Fatal("accepted unknown role")
	}
}
func TestPersistAgentIDPreservesConfigurationAcrossRestarts(t *testing.T) {
	resetConfigGlobalsForTest()
	t.Cleanup(resetConfigGlobalsForTest)
	t.Setenv("HEPHAESTUS_CONFIG_DIR", t.TempDir())
	path := GetConfigPath()
	original := []byte("# user settings\ndatabase:\n  type: postgres\n  password: private\nagent:\n  roles: [ai, scan]\n  aiConcurrency: 4\n  scanConcurrency: 2\ncustom: keep-me\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := PersistAgentID("stable-node"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err = yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.ID != "stable-node" || cfg.Agent.AIConcurrency != 4 || cfg.Agent.ScanConcurrency != 2 || !strings.Contains(string(raw), "custom: keep-me") || !strings.Contains(string(raw), "# user settings") {
		t.Fatalf("configuration not preserved: %s", raw)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("configuration permissions changed")
	}
	if err := PersistAgentID("stable-node"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadConfig()
	if err != nil || reloaded.Agent.ID != "stable-node" {
		t.Fatalf("identity changed on restart: %+v %v", reloaded, err)
	}
	files, _ := filepath.Glob(filepath.Join(GetConfigBaseDir(), ".agent-config-*"))
	if len(files) != 0 {
		t.Fatal("temporary configuration left behind")
	}
}
