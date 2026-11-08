package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func resetConfigGlobalsForTest() {
	configBaseDir = ""
	configBaseOnce = sync.Once{}
	logsDirOnce = sync.Once{}
	dataDirOnce = sync.Once{}

	runtimeMu.Lock()
	runtimeMode = RuntimeModeServer
	runtimeMu.Unlock()
}

func TestGetConfigPathUsesDedicatedFilePerRuntime(t *testing.T) {
	resetConfigGlobalsForTest()

	tempDir := t.TempDir()
	t.Setenv("HEPHAESTUS_CONFIG_DIR", tempDir)

	SetRuntimeMode(RuntimeModeServer)
	if got, want := GetConfigPath(), filepath.Join(tempDir, "config.yaml"); got != want {
		t.Fatalf("server config path = %q, want %q", got, want)
	}

	SetRuntimeMode(RuntimeModeDesktop)
	if got, want := GetConfigPath(), filepath.Join(tempDir, "config-desktop.yaml"); got != want {
		t.Fatalf("desktop config path = %q, want %q", got, want)
	}
}

func TestIsAgentProcessUsesRuntimeMode(t *testing.T) {
	resetConfigGlobalsForTest()

	if isAgentProcess() {
		t.Fatalf("isAgentProcess() should be false in server mode")
	}

	SetRuntimeMode(RuntimeModeAgent)
	if !isAgentProcess() {
		t.Fatalf("isAgentProcess() should be true in agent mode")
	}
}

func TestGenerateInitialConfigFileUsesServerDefaults(t *testing.T) {
	resetConfigGlobalsForTest()

	tempDir := t.TempDir()
	t.Setenv("HEPHAESTUS_CONFIG_DIR", tempDir)
	SetRuntimeMode(RuntimeModeServer)

	configPath := filepath.Join(tempDir, "config.yaml")
	if err := generateInitialConfigFile(configPath); err != nil {
		t.Fatalf("generateInitialConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	content := string(data)
	for _, expected := range []string{
		"type: postgres",
		"host: 127.0.0.1",
		"port: \"5432\"",
		"user: postgres",
		`password: ""`,
		"dbname: hephaestus",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("generated server config missing %q:\n%s", expected, content)
		}
	}
}

func TestGenerateInitialAgentConfigLeavesDatabasePasswordEmpty(t *testing.T) {
	resetConfigGlobalsForTest()
	t.Setenv("HEPHAESTUS_CONFIG_DIR", t.TempDir())
	SetRuntimeMode(RuntimeModeAgent)

	if err := generateInitialConfigFile(GetConfigPath()); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Password != "" {
		t.Fatal("generated Agent config must leave the database password empty")
	}
}

func TestGenerateInitialConfigFileUsesDesktopDefaults(t *testing.T) {
	resetConfigGlobalsForTest()

	tempDir := t.TempDir()
	t.Setenv("HEPHAESTUS_CONFIG_DIR", tempDir)
	SetRuntimeMode(RuntimeModeDesktop)

	configPath := filepath.Join(tempDir, "config-desktop.yaml")
	if err := generateInitialConfigFile(configPath); err != nil {
		t.Fatalf("generateInitialConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "type: sqlite") {
		t.Fatalf("generated desktop config should pin sqlite:\n%s", content)
	}
	if !strings.Contains(content, GetDefaultSQLitePath()) {
		t.Fatalf("generated desktop config should include sqlite path %q:\n%s", GetDefaultSQLitePath(), content)
	}
}

func TestGetDatabaseConfigDesktopMigratesLegacySQLitePath(t *testing.T) {
	resetConfigGlobalsForTest()

	tempDir := t.TempDir()
	t.Setenv("HEPHAESTUS_CONFIG_DIR", tempDir)
	SetRuntimeMode(RuntimeModeDesktop)

	configPath := filepath.Join(tempDir, "config-desktop.yaml")
	legacy := "database:\n  type: sqlite\n  path: C:/sqlite/hephaestus.db\n"
	if err := os.WriteFile(configPath, []byte(legacy), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := GetDatabaseConfig()
	if err != nil {
		t.Fatalf("GetDatabaseConfig() error = %v", err)
	}

	wantPath := filepath.Join(tempDir, "data", "hephaestus.db")
	if cfg.Path != wantPath {
		t.Fatalf("sqlite path = %q, want %q", cfg.Path, wantPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	content := string(data)
	if strings.Contains(content, "C:/sqlite") {
		t.Fatalf("legacy sqlite path should be migrated in config file:\n%s", content)
	}
	if !strings.Contains(content, wantPath) {
		t.Fatalf("migrated config should include sqlite path %q:\n%s", wantPath, content)
	}
}

func TestLoadConfigDesktopDoesNotReadServerConfig(t *testing.T) {
	resetConfigGlobalsForTest()

	tempDir := t.TempDir()
	t.Setenv("HEPHAESTUS_CONFIG_DIR", tempDir)

	serverConfig := "database:\n  type: postgres\n  host: server-only\n"
	if err := os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(serverConfig), 0644); err != nil {
		t.Fatalf("os.WriteFile(server) error = %v", err)
	}

	SetRuntimeMode(RuntimeModeDesktop)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Database.Type != DatabaseTypeSQLite {
		t.Fatalf("desktop runtime should load its own default sqlite config, got %q", cfg.Database.Type)
	}
	if cfg.Database.Host == "server-only" {
		t.Fatalf("desktop runtime should not read server config file")
	}
	if _, err := os.Stat(filepath.Join(tempDir, "config-desktop.yaml")); err != nil {
		t.Fatalf("desktop config file should be created: %v", err)
	}
}

func TestResolveAgentIdentityRewritesPlaceholderDefaults(t *testing.T) {
	id, displayName := ResolveAgentIdentity("scan-agent", "Scan Agent")
	if id == "" || id == "scan-agent" {
		t.Fatalf("ResolveAgentIdentity() should rewrite placeholder id, got %q", id)
	}
	if displayName != "Scan Agent" {
		t.Fatalf("ResolveAgentIdentity() displayName = %q, want %q", displayName, "Scan Agent")
	}
}

func TestResolveAgentIdentityKeepsExplicitValues(t *testing.T) {
	id, displayName := ResolveAgentIdentity("agent-prod-01", "Production Agent")
	if id != "agent-prod-01" {
		t.Fatalf("ResolveAgentIdentity() id = %q, want %q", id, "agent-prod-01")
	}
	if displayName != "Production Agent" {
		t.Fatalf("ResolveAgentIdentity() displayName = %q, want %q", displayName, "Production Agent")
	}
}

func TestNormalizeAIConfigMigratesLegacySingleProvider(t *testing.T) {
	cfg := NormalizeAIConfig(AIConfig{
		Enabled: true,
		API:     "https://api.deepseek.com/v1",
		Key:     "sk-test",
		Model:   "deepseek-chat",
	})

	if cfg.DefaultProviderID == "" {
		t.Fatal("default provider id should be populated")
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("expected one migrated provider, got %d", len(cfg.Providers))
	}
	provider := cfg.Providers[0]
	if provider.Protocol != AIProtocolOpenAICompatible {
		t.Fatalf("provider protocol = %q, want %q", provider.Protocol, AIProtocolOpenAICompatible)
	}
	if provider.API != "https://api.deepseek.com/v1" || provider.Key != "sk-test" || provider.Model != "deepseek-chat" {
		t.Fatalf("legacy values were not migrated into provider: %#v", provider)
	}
	if cfg.API != provider.API || cfg.Key != provider.Key || cfg.Model != provider.Model {
		t.Fatalf("top-level compatibility fields should mirror default provider: %#v", cfg)
	}
}

func TestResolveAIRequestConfigForRouteUsesOverrideProvider(t *testing.T) {
	cfg := NormalizeAIConfig(AIConfig{
		Enabled:           true,
		DefaultProviderID: "openai",
		Providers: []AIProviderConfig{
			{
				ID:       "openai",
				Name:     "OpenAI",
				Protocol: AIProtocolOpenAICompatible,
				Enabled:  true,
				API:      "https://api.openai.com/v1",
				Key:      "sk-openai",
				Model:    "gpt-4o-mini",
			},
			{
				ID:       "claude",
				Name:     "Claude",
				Protocol: AIProtocolAnthropic,
				Enabled:  true,
				API:      "https://api.anthropic.com/v1",
				Key:      "sk-ant",
				Model:    "claude-3-5-sonnet-latest",
			},
		},
		Routing: AIRoutingConfig{
			Workbench: "claude",
		},
	})

	resolved := ResolveAIRequestConfigForRoute(cfg, AIRouteWorkbench)
	if resolved.Protocol != AIProtocolAnthropic {
		t.Fatalf("resolved protocol = %q, want %q", resolved.Protocol, AIProtocolAnthropic)
	}
	if resolved.Key != "sk-ant" || resolved.Model != "claude-3-5-sonnet-latest" {
		t.Fatalf("resolved route provider fields are wrong: %#v", resolved)
	}
}

func TestNormalizeAIConfigKeepsValidWorkbenchFallbackOnly(t *testing.T) {
	cfg := NormalizeAIConfig(AIConfig{
		Enabled:           true,
		DefaultProviderID: "primary",
		Providers: []AIProviderConfig{
			{
				ID:       "primary",
				Protocol: AIProtocolOpenAICompatible,
				Enabled:  true,
				API:      "https://primary.example.com/v1",
				Key:      "sk-primary",
				Model:    "primary-model",
			},
			{
				ID:       "backup",
				Protocol: AIProtocolOpenAICompatible,
				Enabled:  true,
				API:      "https://backup.example.com/v1",
				Key:      "sk-backup",
				Model:    "backup-model",
			},
		},
		Routing: AIRoutingConfig{
			WorkbenchFallbackEnabled: true,
			WorkbenchFallback:        "backup",
		},
	})
	if !cfg.Routing.WorkbenchFallbackEnabled || cfg.Routing.WorkbenchFallback != "backup" {
		t.Fatalf("valid fallback was not preserved: %#v", cfg.Routing)
	}

	cfg = NormalizeAIConfig(AIConfig{
		Enabled:           true,
		DefaultProviderID: "primary",
		Providers: []AIProviderConfig{
			{
				ID:       "primary",
				Protocol: AIProtocolOpenAICompatible,
				Enabled:  true,
				API:      "https://primary.example.com/v1",
				Key:      "sk-primary",
				Model:    "primary-model",
			},
		},
		Routing: AIRoutingConfig{
			WorkbenchFallbackEnabled: true,
			WorkbenchFallback:        "primary",
		},
	})
	if cfg.Routing.WorkbenchFallbackEnabled || cfg.Routing.WorkbenchFallback != "" {
		t.Fatalf("invalid fallback should be cleared: %#v", cfg.Routing)
	}
}

func TestWriteConfigFileAddsImmutableAgentIDComment(t *testing.T) {
	resetConfigGlobalsForTest()
	SetRuntimeMode(RuntimeModeAgent)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &Config{
		Agent: AgentConfig{
			ID:          "agent-test-123",
			DisplayName: "Scan Agent",
			Server: AgentServerConfig{
				Address: "http://127.0.0.1:18181",
			},
		},
	}

	if err := writeConfigFile(configPath, cfg); err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "agent.id 为节点唯一标识，首次生成后不可修改") {
		t.Fatalf("agent config should include immutable id comment:\n%s", content)
	}
	if !strings.Contains(content, "id: agent-test-123") {
		t.Fatalf("agent config should include id value:\n%s", content)
	}
	if strings.Contains(content, "displayName:") {
		t.Fatalf("agent config should not persist displayName anymore:\n%s", content)
	}
	if strings.Contains(content, "address:") {
		t.Fatalf("agent config should not persist server.address anymore:\n%s", content)
	}
}

func TestLoadConfigAgentStripsLegacyDisplayNameAndServerAddress(t *testing.T) {
	resetConfigGlobalsForTest()
	SetRuntimeMode(RuntimeModeAgent)

	configDir := t.TempDir()
	t.Setenv("HEPHAESTUS_CONFIG_DIR", configDir)
	configPath := filepath.Join(configDir, "config.yaml")
	legacy := `database:
  type: postgres
  host: localhost
  port: "5432"
  user: postgres
  password: local-test-password
  dbname: hephaestus
  sslmode: disable
agent:
  id: legacy-agent-id
  displayName: Legacy Agent
  server:
    address: http://127.0.0.1:18181
`
	if err := os.WriteFile(configPath, []byte(legacy), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Database.Password != "local-test-password" {
		t.Fatal("LoadConfig() did not preserve the explicitly configured database password")
	}
	if got := cfg.Agent.ID; got != "legacy-agent-id" {
		t.Fatalf("cfg.Agent.ID = %q, want %q", got, "legacy-agent-id")
	}
	if got := cfg.Agent.DisplayName; got != "" {
		t.Fatalf("cfg.Agent.DisplayName = %q, want empty", got)
	}
	if got := cfg.Agent.Server.Address; got != "" {
		t.Fatalf("cfg.Agent.Server.Address = %q, want empty", got)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	content := string(data)
	if strings.Contains(content, "displayName:") {
		t.Fatalf("migrated agent config should not persist displayName:\n%s", content)
	}
	if strings.Contains(content, "address:") {
		t.Fatalf("migrated agent config should not persist server.address:\n%s", content)
	}
}

func TestGetLogsDirMigratesLegacyRuntimeLogs(t *testing.T) {
	resetConfigGlobalsForTest()

	legacyRoot := t.TempDir()
	configRoot := t.TempDir()
	legacyLogsDir := filepath.Join(legacyRoot, "data", "logs")
	if err := os.MkdirAll(legacyLogsDir, 0755); err != nil {
		t.Fatalf("mkdir legacy logs dir: %v", err)
	}
	legacyLogPath := filepath.Join(legacyLogsDir, "2026-04-29.log")
	if err := os.WriteFile(legacyLogPath, []byte("legacy-log\n"), 0644); err != nil {
		t.Fatalf("write legacy log: %v", err)
	}

	t.Setenv("HEPHAESTUS_CONFIG_DIR", configRoot)

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()
	if err := os.Chdir(legacyRoot); err != nil {
		t.Fatalf("chdir legacy root: %v", err)
	}

	logsDir := GetLogsDir()
	if got, want := logsDir, filepath.Join(configRoot, "logs"); got != want {
		t.Fatalf("logs dir = %q, want %q", got, want)
	}

	newLogPath := filepath.Join(logsDir, "2026-04-29.log")
	data, err := os.ReadFile(newLogPath)
	if err != nil {
		t.Fatalf("read migrated log: %v", err)
	}
	if string(data) != "legacy-log\n" {
		t.Fatalf("unexpected migrated log content: %q", string(data))
	}
	if _, err := os.Stat(legacyLogPath); !os.IsNotExist(err) {
		t.Fatalf("legacy log should be removed after migration, stat err = %v", err)
	}
}

func TestGetLogsDirMigratesConflictingLegacyLogsWithSuffix(t *testing.T) {
	resetConfigGlobalsForTest()

	legacyRoot := t.TempDir()
	configRoot := t.TempDir()
	legacyLogsDir := filepath.Join(legacyRoot, "data", "logs")
	if err := os.MkdirAll(legacyLogsDir, 0755); err != nil {
		t.Fatalf("mkdir legacy logs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyLogsDir, "2026-04-29.log"), []byte("legacy-log\n"), 0644); err != nil {
		t.Fatalf("write legacy log: %v", err)
	}

	newLogsDir := filepath.Join(configRoot, "logs")
	if err := os.MkdirAll(newLogsDir, 0755); err != nil {
		t.Fatalf("mkdir new logs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newLogsDir, "2026-04-29.log"), []byte("new-log\n"), 0644); err != nil {
		t.Fatalf("write new log: %v", err)
	}

	t.Setenv("HEPHAESTUS_CONFIG_DIR", configRoot)

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()
	if err := os.Chdir(legacyRoot); err != nil {
		t.Fatalf("chdir legacy root: %v", err)
	}

	logsDir := GetLogsDir()
	if got, want := logsDir, newLogsDir; got != want {
		t.Fatalf("logs dir = %q, want %q", got, want)
	}

	data, err := os.ReadFile(filepath.Join(newLogsDir, "2026-04-29.log"))
	if err != nil {
		t.Fatalf("read existing log: %v", err)
	}
	if string(data) != "new-log\n" {
		t.Fatalf("existing log should stay unchanged: %q", string(data))
	}

	migratedData, err := os.ReadFile(filepath.Join(newLogsDir, "2026-04-29.migrated.1.log"))
	if err != nil {
		t.Fatalf("read migrated conflict log: %v", err)
	}
	if string(migratedData) != "legacy-log\n" {
		t.Fatalf("unexpected migrated conflict log content: %q", string(migratedData))
	}
}

func TestGetDataDirMigratesLegacyDataContents(t *testing.T) {
	resetConfigGlobalsForTest()

	legacyRoot := t.TempDir()
	configRoot := t.TempDir()
	for _, rel := range []string{
		filepath.Join("screenshot", "screenshot.bin"),
		filepath.Join("favicon", "favicon.bin"),
		filepath.Join("exports", "hunter", "user-a", "hunter.csv"),
		filepath.Join("exports", "aggregated", "summary.json"),
	} {
		path := filepath.Join(legacyRoot, "data", rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir legacy path: %v", err)
		}
		if err := os.WriteFile(path, []byte(rel), 0644); err != nil {
			t.Fatalf("write legacy data: %v", err)
		}
	}

	t.Setenv("HEPHAESTUS_CONFIG_DIR", configRoot)

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()
	if err := os.Chdir(legacyRoot); err != nil {
		t.Fatalf("chdir legacy root: %v", err)
	}

	dataDir := GetDataDir()
	if got, want := dataDir, filepath.Join(configRoot, "data"); got != want {
		t.Fatalf("data dir = %q, want %q", got, want)
	}

	for _, rel := range []string{
		filepath.Join("screenshot", "screenshot.bin"),
		filepath.Join("favicon", "favicon.bin"),
		filepath.Join("exports", "hunter", "user-a", "hunter.csv"),
		filepath.Join("exports", "aggregated", "summary.json"),
	} {
		newPath := filepath.Join(dataDir, rel)
		data, err := os.ReadFile(newPath)
		if err != nil {
			t.Fatalf("read migrated data %s: %v", rel, err)
		}
		if string(data) != rel {
			t.Fatalf("unexpected migrated data %s content: %q", rel, string(data))
		}

		oldPath := filepath.Join(legacyRoot, "data", rel)
		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Fatalf("legacy data %s should be removed after migration, stat err = %v", rel, err)
		}
	}
}

func TestAIRequestRateConfigRoundTripAndLegacyDefaults(t *testing.T) {
	oldGet, oldSet := getSettingFunc, setSettingFunc
	t.Cleanup(func() { getSettingFunc, setSettingFunc = oldGet, oldSet })
	var stored any
	setSettingFunc = func(_ string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &stored)
	}
	getSettingFunc = func(string) (any, error) { return stored, nil }
	for _, tc := range []struct {
		name     string
		provider AIProviderConfig
		wantRPM  int
		wantUnit string
	}{
		{name: "new default", wantUnit: "second"},
		{name: "legacy minute", provider: AIProviderConfig{WorkbenchRequestsPerMinute: 60}, wantRPM: 60, wantUnit: "minute"},
		{name: "five per second", provider: AIProviderConfig{WorkbenchRequestsPerMinute: 300, WorkbenchRequestRateUnit: "second"}, wantRPM: 300, wantUnit: "second"},
		{name: "explicit unlimited", provider: AIProviderConfig{WorkbenchRequestRateUnit: "minute"}, wantUnit: "minute"},
		{name: "invalid negative", provider: AIProviderConfig{WorkbenchRequestsPerMinute: -1}, wantUnit: "second"},
		{name: "fractional second displayed per minute", provider: AIProviderConfig{WorkbenchRequestsPerMinute: 30, WorkbenchRequestRateUnit: "second"}, wantRPM: 30, wantUnit: "minute"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := UpdateAIConfig(AIConfig{Providers: []AIProviderConfig{tc.provider}}); err != nil {
				t.Fatal(err)
			}
			cfg, err := GetAIConfig()
			if err != nil {
				t.Fatal(err)
			}
			got := cfg.Providers[0]
			if got.WorkbenchRequestsPerMinute != tc.wantRPM || got.WorkbenchRequestRateUnit != tc.wantUnit {
				t.Fatalf("got %d/%s want %d/%s", got.WorkbenchRequestsPerMinute, got.WorkbenchRequestRateUnit, tc.wantRPM, tc.wantUnit)
			}
		})
	}
}
