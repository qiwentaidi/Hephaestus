package scantask

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/portpresets"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openPortGroupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "port-groups.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&database.Setting{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	return db
}

func savePortGroupTestSetting(t *testing.T, db *gorm.DB, key string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal setting: %v", err)
	}
	if err := db.Save(&database.Setting{Key: key, Value: string(raw)}).Error; err != nil {
		t.Fatalf("save setting: %v", err)
	}
}

func TestLoadDefaultPortGroupFallsBackToTop1000(t *testing.T) {
	db := openPortGroupTestDB(t)

	group, err := LoadDefaultPortGroupWithDB(db)
	if err != nil {
		t.Fatalf("load default port group: %v", err)
	}
	if group.ID != defaultPortGroupID || group.Ports != portpresets.WorkbenchPortRangeTop1000 || !group.BuiltIn {
		t.Fatalf("unexpected fallback group: %#v", group)
	}
}

func TestApplyPortScanPortGroupDefaultsUsesConfiguredDefault(t *testing.T) {
	db := openPortGroupTestDB(t)
	want := DefaultPortGroup{ID: "custom-web", Name: "Web 服务", Ports: "80,443,8080"}
	if err := SaveDefaultPortGroupWithDB(db, want); err != nil {
		t.Fatalf("save default port group: %v", err)
	}

	config := map[string]interface{}{}
	if err := ApplyPortScanPortGroupDefaultsWithDB(db, config); err != nil {
		t.Fatalf("apply default port group: %v", err)
	}
	if config["portRange"] != want.Ports || config["portGroupId"] != want.ID || config["portGroupName"] != want.Name {
		t.Fatalf("default port group was not applied: %#v", config)
	}
}

func TestApplyPortScanPortGroupDefaultsPreservesExplicitRange(t *testing.T) {
	db := openPortGroupTestDB(t)
	config := map[string]interface{}{
		"portRange": "80, 443, 8080",
	}

	if err := ApplyPortScanPortGroupDefaultsWithDB(db, config); err != nil {
		t.Fatalf("apply explicit port range: %v", err)
	}
	if config["portRange"] != "80,443,8080" || config["portGroupId"] != customPortGroupID {
		t.Fatalf("explicit port range was replaced: %#v", config)
	}
}

func TestApplyPortScanPortGroupDefaultsResolvesRequestedCustomGroup(t *testing.T) {
	db := openPortGroupTestDB(t)
	groups := []CustomPortGroup{{ID: "middleware", Name: "中间件", Ports: "7001,8080,9000"}}
	savePortGroupTestSetting(t, db, SettingKeyCustomPortGroups, groups)
	config := map[string]interface{}{"portGroupId": "middleware"}

	if err := ApplyPortScanPortGroupDefaultsWithDB(db, config); err != nil {
		t.Fatalf("resolve custom port group: %v", err)
	}
	if config["portRange"] != groups[0].Ports || config["portGroupName"] != groups[0].Name {
		t.Fatalf("custom port group was not resolved: %#v", config)
	}
}
