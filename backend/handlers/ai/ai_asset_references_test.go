package ai

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/pkg/database"
)

func TestAssetReferencesFreezeSelectionAndScopeToConversation(t *testing.T) {
	db := assetReferenceTestDB(t)
	if err := db.AutoMigrate(&database.AIAssetReference{}, &database.PortScanResult{}); err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{1, 2} {
		if err := db.Create(&database.VulnScanTask{ID: "scan", Version: version, TaskName: "Ports", ScanType: "portscan", Status: "running"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&database.PortScanResult{TaskID: "scan", Version: version, Host: "example.test", Port: 443}).Error; err != nil {
			t.Fatal(err)
		}
	}
	refs, err := captureAssetReferences(db, "conversation-a", []assetReferenceSelection{{TaskID: "scan", Version: 1, Selection: "filtered"}})
	if err != nil || len(refs) != 1 || refs[0].Count != 1 || refs[0].Version != 1 {
		t.Fatal(refs, err)
	}
	if err := db.Create(&database.PortScanResult{TaskID: "scan", Version: 1, Host: "new.test", Port: 80}).Error; err != nil {
		t.Fatal(err)
	}
	// Older whole-task references have no read_mode; they must remain frozen.
	if err := db.Model(&refs[0]).Updates(map[string]any{"read_mode": "", "selection": "all"}).Error; err != nil {
		t.Fatal(err)
	}
	out, err := readAssetReference(db, "conversation-a", refs[0].ID, "", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := out.(gin.H)
	rows := got["assets"].([]referencedAsset)
	if got["total"] != 1 || got["hasMore"] != false || rows[0].Value != "example.test:443" || rows[0].Version != 1 {
		t.Fatal(got)
	}
	if _, err := readAssetReference(db, "conversation-b", refs[0].ID, "", 0, 20); err == nil {
		t.Fatal("cross-conversation reference accepted")
	}
	if _, err := readAssetReference(db, "conversation-a", refs[0].ID, "", -1, 20); err == nil {
		t.Fatal("negative offset accepted")
	}
	if err := db.Where("id = ? AND version = ?", "scan", 1).Delete(&database.VulnScanTask{}).Error; err != nil {
		t.Fatal(err)
	}
	out, err = readAssetReference(db, "conversation-a", refs[0].ID, "", 0, 20)
	if err != nil || out.(gin.H)["sourceAvailable"] != false {
		t.Fatal(out, err)
	}
}

func TestAssetReferencesFilterSelectionAndAtomicFailure(t *testing.T) {
	db := assetReferenceTestDB(t)
	if err := db.AutoMigrate(&database.AIAssetReference{}, &database.PortScanResult{}); err != nil {
		t.Fatal(err)
	}
	task := database.VulnScanTask{ID: "ports", Version: 1, ScanType: "portscan", Status: "completed"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	for _, port := range []int{80, 443} {
		if err := db.Create(&database.PortScanResult{TaskID: task.ID, Version: 1, Host: "example.test", Port: port}).Error; err != nil {
			t.Fatal(err)
		}
	}
	refs, err := captureAssetReferences(db, "c", []assetReferenceSelection{{TaskID: task.ID, Version: 1, Selection: "filtered", Query: "443", Kind: "port"}})
	if err != nil || refs[0].Count != 1 {
		t.Fatal(refs, err)
	}
	page, err := readAssetReference(db, "c", refs[0].ID, "", 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	assets := page.(gin.H)["assets"].([]referencedAsset)
	explicit, err := captureAssetReferences(db, "c", []assetReferenceSelection{{TaskID: task.ID, Version: 1, Selection: "selected", Query: "unrelated filter", AssetIDs: []string{assets[0].ID}}})
	if err != nil || explicit[0].Count != 1 {
		t.Fatal(explicit, err)
	}
	_, err = captureAssetReferences(db, "c", []assetReferenceSelection{{TaskID: task.ID, Version: 1, Selection: "all"}, {TaskID: task.ID, Version: 1, Selection: "selected", AssetIDs: []string{"other-task/asset"}}})
	if err == nil {
		t.Fatal("invalid asset accepted")
	}
	var count int64
	db.Model(&database.AIAssetReference{}).Count(&count)
	if count != 2 {
		t.Fatalf("failed request left partial references: %d", count)
	}
}

func TestAssetReferenceMessagePresentationAndHistory(t *testing.T) {
	refs := []database.AIAssetReference{{ID: "r", TaskID: "scan", Version: 2, Name: "Task", Count: 12}}
	payload, _ := json.Marshal(refs)
	content := "Test selected assets" + assetReferenceMarker + string(payload) + assetReferenceEnd + "\nRead with tools."
	text, got := splitAssetReferenceContent(content)
	if text != "Test selected assets" || len(got) != 1 || got[0].ID != "r" {
		t.Fatal(text, got)
	}
	text, got = splitAssetReferenceContent("Unrelated <hephaestus_asset_references> text")
	if text != "Unrelated <hephaestus_asset_references> text" || len(got) != 0 {
		t.Fatal(text, got)
	}
}

func assetReferenceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("HEPHAESTUS_ASSET_REFERENCE_TEST_DSN")
	if dsn == "" {
		return testfixture.SQLiteDB(t)
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := "asset_refs_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		if sqlDB, e := admin.DB(); e == nil {
			sqlDB.Close()
		}
	})
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			sqlDB.Close()
		}
	})
	if err = db.AutoMigrate(&database.VulnScanTask{}, &database.CombinationTask{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAssetReferenceTaskSearchAndCombinationSources(t *testing.T) {
	db := assetReferenceTestDB(t)
	if err := db.AutoMigrate(&database.AIAssetReference{}, &database.PortScanResult{}); err != nil {
		t.Fatal(err)
	}
	combo := database.CombinationTask{ID: "combo", Name: "Combined assessment", Targets: `["example.test"]`}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.VulnScanTask{ID: "combo-parent", Version: 1, ScanType: "combination", ParentCombinationID: "combo", TaskName: "Combined assessment", Targets: `["example.test"]`}).Error; err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{1, 2} {
		task := database.VulnScanTask{ID: "child", Version: version, ScanType: "portscan", ParentCombinationID: "combo", TaskName: "Ports", Targets: `["example.test"]`}
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&database.PortScanResult{TaskID: task.ID, Version: version, Host: "example.test", Port: 443}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var tasks []referenceTask
	if err := referenceTasksQuery(db, "example.test").Find(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatal(tasks)
	}
	refs, err := captureAssetReferences(db, "c", []assetReferenceSelection{{TaskID: "combo", Selection: "all"}})
	if err != nil || len(refs) != 1 || refs[0].Count != 1 {
		t.Fatal(refs, err)
	}
	page, err := readAssetReference(db, "c", refs[0].ID, "", 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	assets := page.(gin.H)["assets"].([]referencedAsset)
	if len(assets) != 1 || assets[0].TaskID != "child" || assets[0].Version != 2 {
		t.Fatal(assets)
	}
	// New child tasks/versions are not silently added to an existing reference.
	for _, id := range []string{"child", "new-child"} {
		if err := db.Create(&database.VulnScanTask{ID: id, Version: 3, ScanType: "portscan", ParentCombinationID: "combo"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&database.PortScanResult{TaskID: id, Version: 3, Host: "new.test", Port: 80}).Error; err != nil {
			t.Fatal(err)
		}
	}
	page, err = readAssetReference(db, "c", refs[0].ID, "", 0, 20)
	if err != nil || page.(gin.H)["total"] != int64(1) {
		t.Fatal(page, err)
	}

	// Deleting a combination removes its parent scan rows, but older databases
	// may retain the combination_task row. Such an orphan must not be offered by @.
	if err := db.Where("id = ?", "combo-parent").Delete(&database.VulnScanTask{}).Error; err != nil {
		t.Fatal(err)
	}
	tasks = nil
	if err := referenceTasksQuery(db, "example.test").Find(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.ID == "combo" && task.ScanType == "combination" {
			t.Fatalf("orphaned combination task was returned: %+v", task)
		}
	}
}

func TestWholeTaskReferencePagesBeyondSnapshotLimit(t *testing.T) {
	db := assetReferenceTestDB(t)
	if err := db.AutoMigrate(&database.AIAssetReference{}, &database.PortScanResult{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.VulnScanTask{ID: "large-ports", Version: 1, ScanType: "portscan", Status: "running"}).Error; err != nil {
		t.Fatal(err)
	}
	ports := make([]database.PortScanResult, maxReferencedAssets+1)
	for i := range ports {
		ports[i] = database.PortScanResult{TaskID: "large-ports", Version: 1, Host: fmt.Sprintf("host-%05d.test", i), Port: 443, Protocol: "tcp"}
	}
	if err := db.CreateInBatches(ports, 300).Error; err != nil {
		t.Fatal(err)
	}
	refs, err := captureAssetReferences(db, "main", []assetReferenceSelection{{TaskID: "large-ports", Selection: "all", Query: "ignored", Kind: "ignored"}})
	if err != nil {
		t.Fatal(err)
	}
	ref := refs[0]
	if ref.ReadMode != "task" || ref.Version != 1 || ref.Count != len(ports) || ref.AssetsJSON != "[]" || len(ref.Sources) != 1 {
		t.Fatalf("eager or unpinned reference: %+v", ref)
	}
	content, err := formatAssetReferenceContent("Read existing results", refs)
	if err != nil || len(content) > 6000 {
		t.Fatalf("unbounded message: %d %v", len(content), err)
	}
	visible, parsed := splitAssetReferenceContent(content)
	if visible != "Read existing results" || len(parsed) != 1 || parsed[0].ReadMode != "task" {
		t.Fatal(visible, parsed)
	}
	seen := map[string]bool{}
	for offset := 0; ; {
		out, err := readAssetReference(db, "main", ref.ID, "", offset, 100)
		if err != nil {
			t.Fatal(err)
		}
		page := out.(gin.H)
		if page["total"] != int64(len(ports)) || page["sourceAvailable"] != true {
			t.Fatal(page)
		}
		rows := page["assets"].([]referencedAsset)
		if len(rows) > 100 {
			t.Fatal("page limit exceeded")
		}
		for _, row := range rows {
			if seen[row.ID] || row.Version != 1 {
				t.Fatalf("duplicate/wrong version: %+v", row)
			}
			seen[row.ID] = true
		}
		offset = page["nextOffset"].(int)
		if page["hasMore"] == false {
			break
		}
		if len(rows) == 0 {
			t.Fatal("pagination made no progress")
		}
	}
	if len(seen) != len(ports) {
		t.Fatalf("read %d of %d", len(seen), len(ports))
	}
	if _, err = readAssetReference(db, "other", ref.ID, "", 0, 100); err == nil {
		t.Fatal("cross-conversation reference accepted")
	}
	if _, err = captureAssetReferences(db, "main", []assetReferenceSelection{{TaskID: "large-ports", Version: 1, Selection: "filtered", Kind: "port"}}); err == nil {
		t.Fatal("filtered snapshot limit removed")
	}
	// The selected set stays fixed even when the task contains more than 10,000 rows.
	selected, err := captureAssetReferences(db, "main", []assetReferenceSelection{{TaskID: "large-ports", Version: 1, Selection: "selected", AssetIDs: []string{fmt.Sprintf("large-ports/1/port/%d", ports[0].ID)}}})
	if err != nil || selected[0].Count != 1 || selected[0].ReadMode != "snapshot" {
		t.Fatal(selected, err)
	}
	if err = db.Create(&database.VulnScanTask{ID: "large-ports", Version: 2, ScanType: "portscan", Status: "completed"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{1, 2} {
		if err = db.Create(&database.PortScanResult{TaskID: "large-ports", Version: version, Host: "later.test", Port: 80}).Error; err != nil {
			t.Fatal(err)
		}
	}
	out, err := readAssetReference(db, "main", ref.ID, "later.test", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	page := out.(gin.H)
	rows := page["assets"].([]referencedAsset)
	if page["total"] != int64(1) || len(rows) != 1 || rows[0].Version != 1 {
		t.Fatal(page)
	}
	// Deleted versions cannot silently switch to the latest one.
	if err = db.Where("id = ? AND version = ?", "large-ports", 1).Delete(&database.VulnScanTask{}).Error; err != nil {
		t.Fatal(err)
	}
	out, err = readAssetReference(db, "main", ref.ID, "", 0, 100)
	if err != nil || out.(gin.H)["sourceAvailable"] != false || len(out.(gin.H)["assets"].([]referencedAsset)) != 0 {
		t.Fatal(out, err)
	}
	out, err = readAssetReference(db, "main", selected[0].ID, "", 0, 100)
	if err != nil || len(out.(gin.H)["assets"].([]referencedAsset)) != 1 {
		t.Fatal("selected snapshot lost", out, err)
	}
}
