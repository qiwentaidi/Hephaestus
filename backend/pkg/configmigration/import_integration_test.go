package configmigration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
)

func TestMigrationRoundTrip(t *testing.T) {
	// The database singleton is initialized once per test binary, so this test
	// owns the global setup and exercises the full export/import path.
	dir := t.TempDir()
	t.Setenv("HEPHAESTUS_CONFIG_DIR", dir)
	config.SetRuntimeMode(config.RuntimeModeServer)

	configPath := filepath.Join(dir, "config.yaml")
	content := "database:\n  type: sqlite\n  path: " + filepath.Join(dir, "hephaestus-test.db") + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	db, err := database.GetDB()
	if err != nil {
		t.Fatalf("database.GetDB() error = %v", err)
	}
	config.InitSpaceEngineConfigDB(database.GetSetting, database.SetSetting, database.SettingKeySpaceEngines)
	config.InitNotificationConfigDB(database.GetSetting, database.SetSetting, database.SettingKeyNotifications)
	config.InitAIConfigDB(database.GetSetting, database.SetSetting, database.SettingKeyAI)

	aiConfig := config.AIConfig{
		Enabled: true,
		Providers: []config.AIProviderConfig{{
			ID:       "provider-1",
			Name:     "Provider 1",
			Protocol: config.AIProtocolOpenAICompatible,
			Enabled:  true,
			API:      "https://api.example.com/v1",
			Key:      "sk-migration-secret",
			Model:    "model-1",
		}},
	}
	if err := config.UpdateAIConfig(aiConfig); err != nil {
		t.Fatalf("UpdateAIConfig() error = %v", err)
	}

	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		t.Fatalf("GetSpaceEngineConfig() error = %v", err)
	}
	spaceConfig.FOFA.Key = "fofa-migration-secret"
	spaceConfig.Github.Token = "github-migration-token"
	if err := config.UpdateSpaceEngineConfig(spaceConfig); err != nil {
		t.Fatalf("UpdateSpaceEngineConfig() error = %v", err)
	}

	notificationConfig, err := config.GetNotificationConfig()
	if err != nil {
		t.Fatalf("GetNotificationConfig() error = %v", err)
	}
	notificationConfig.Channels[0].Enabled = true
	notificationConfig.Channels[0].Secret = "dingtalk-migration-secret"
	notificationConfig.Channels[0].WebhookURL = "https://oapi.dingtalk.com/robot/send?access_token=migration"
	if err := config.UpdateNotificationConfig(notificationConfig); err != nil {
		t.Fatalf("UpdateNotificationConfig() error = %v", err)
	}

	templates := []database.ScanTemplate{
		{
			ID:          database.GenerateNanoid(),
			Name:        "Migration Web Template",
			Description: "first",
			TaskType:    "webscan",
			Config:      `{"threads":10}`,
		},
		{
			ID:          database.GenerateNanoid(),
			Name:        "Migration Crack Template",
			Description: "second",
			TaskType:    "crack",
			Config:      `{"username":"admin","password":"pass"}`,
		},
	}
	for index := range templates {
		if err := db.Create(&templates[index]).Error; err != nil {
			t.Fatalf("create template: %v", err)
		}
	}

	bundle, err := Export(nil)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(bundle.Modules) != len(moduleOrder) {
		t.Fatalf("Export() modules = %d, want %d", len(bundle.Modules), len(moduleOrder))
	}
	serialized, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	for _, secret := range []string{"sk-migration-secret", "fofa-migration-secret", "dingtalk-migration-secret", "github-migration-token"} {
		if strings.Contains(string(serialized), secret) {
			t.Fatalf("exported bundle contains plaintext secret %q", secret)
		}
	}
	if !strings.Contains(string(serialized), encryptedValuePrefix) {
		t.Fatal("exported bundle does not contain encrypted secret markers")
	}

	// Clear live configuration to prove the import restores it.
	if err := config.UpdateAIConfig(config.AIConfig{}); err != nil {
		t.Fatalf("reset AI config: %v", err)
	}
	if err := config.UpdateSpaceEngineConfig(config.SpaceEngineConfig{}); err != nil {
		t.Fatalf("reset space config: %v", err)
	}
	if err := config.UpdateNotificationConfig(config.NotificationConfig{}); err != nil {
		t.Fatalf("reset notification config: %v", err)
	}

	imported, err := Import(ImportRequest{Bundle: bundle})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if len(imported.Modules) != len(moduleOrder) {
		t.Fatalf("Import() modules = %d, want %d", len(imported.Modules), len(moduleOrder))
	}

	restoredAI, err := config.GetAIConfig()
	if err != nil {
		t.Fatalf("GetAIConfig() error = %v", err)
	}
	if restoredAI.Key != "sk-migration-secret" {
		t.Fatalf("AI key not restored: %q", restoredAI.Key)
	}
	restoredSpace, err := config.GetSpaceEngineConfig()
	if err != nil {
		t.Fatalf("GetSpaceEngineConfig() error = %v", err)
	}
	if restoredSpace.FOFA.Key != "fofa-migration-secret" || restoredSpace.Github.Token != "github-migration-token" {
		t.Fatalf("API keys not restored: %#v", restoredSpace)
	}
	restoredNotifications, err := config.GetNotificationConfig()
	if err != nil {
		t.Fatalf("GetNotificationConfig() error = %v", err)
	}
	if len(restoredNotifications.Channels) == 0 || restoredNotifications.Channels[0].Secret != "dingtalk-migration-secret" {
		t.Fatalf("notification secret not restored: %#v", restoredNotifications)
	}

	var count int64
	if err := db.Model(&database.ScanTemplate{}).Where("name LIKE ?", "Migration %").Count(&count).Error; err != nil {
		t.Fatalf("count templates: %v", err)
	}
	if count != int64(len(templates)) {
		t.Fatalf("template count = %d, want %d", count, len(templates))
	}
	var crackTemplate database.ScanTemplate
	if err := db.Where("name = ?", "Migration Crack Template").First(&crackTemplate).Error; err != nil {
		t.Fatalf("read crack template: %v", err)
	}
	var crackConfig map[string]string
	if err := json.Unmarshal([]byte(crackTemplate.Config), &crackConfig); err != nil {
		t.Fatalf("decode crack template config: %v", err)
	}
	if crackConfig["username"] != "admin" || crackConfig["password"] != "pass" {
		t.Fatalf("crack template config not preserved: %s", crackTemplate.Config)
	}

	// Importing the same bundle again must update templates by name, not duplicate them.
	if _, err := Import(ImportRequest{Bundle: bundle}); err != nil {
		t.Fatalf("second Import() error = %v", err)
	}
	if err := db.Model(&database.ScanTemplate{}).Where("name LIKE ?", "Migration %").Count(&count).Error; err != nil {
		t.Fatalf("count templates after second import: %v", err)
	}
	if count != int64(len(templates)) {
		t.Fatalf("template count after second import = %d, want %d", count, len(templates))
	}

	// A selective import only applies the chosen module.
	changedAI := restoredAI
	changedAI.Providers[0].Key = "changed-ai-key"
	if err := config.UpdateAIConfig(changedAI); err != nil {
		t.Fatalf("change AI config: %v", err)
	}
	changedSpace := restoredSpace
	changedSpace.FOFA.Key = "changed-api-key"
	if err := config.UpdateSpaceEngineConfig(changedSpace); err != nil {
		t.Fatalf("change space config: %v", err)
	}
	if _, err := Import(ImportRequest{Bundle: bundle, Modules: []string{ModuleAI}}); err != nil {
		t.Fatalf("selective Import() error = %v", err)
	}
	afterSelectiveAI, err := config.GetAIConfig()
	if err != nil {
		t.Fatalf("GetAIConfig() after selective import error = %v", err)
	}
	if afterSelectiveAI.Key != "sk-migration-secret" {
		t.Fatalf("selective import did not restore AI key: %q", afterSelectiveAI.Key)
	}
	afterSelectiveSpace, err := config.GetSpaceEngineConfig()
	if err != nil {
		t.Fatalf("GetSpaceEngineConfig() after selective import error = %v", err)
	}
	if afterSelectiveSpace.FOFA.Key != "changed-api-key" {
		t.Fatalf("selective import touched API keys: %q", afterSelectiveSpace.FOFA.Key)
	}
}
