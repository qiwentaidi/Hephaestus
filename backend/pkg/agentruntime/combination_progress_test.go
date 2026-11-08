package agentruntime

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/taskstate"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCombinationAgentAggregationPreservesOverallProgress(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "agent.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.VulnScanTaskItem{}); err != nil {
		t.Fatal(err)
	}
	parent := database.VulnScanTask{ID: "parent", Version: 1, ScanType: "combination", ScanMode: "agent", ParentCombinationID: "combo", StepModule: "portscan", Status: "running", Progress: 33}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	lease := time.Now().Add(time.Minute)
	items := []database.VulnScanTaskItem{
		{TaskID: parent.ID, Version: 1, ScanType: "portscan", Target: "one", Status: "completed"},
		{TaskID: parent.ID, Version: 1, ScanType: "portscan", Target: "two", Status: "running", WorkerAgentID: "worker", WorkerGeneration: 1, LeaseExpiresAt: &lease},
		{TaskID: parent.ID, Version: 1, ScanType: "webscan", Target: "old", Status: "completed"},
	}
	if err := db.Create(&items).Error; err != nil {
		t.Fatal(err)
	}
	payload := TaskPayload{ID: parent.ID, Version: 1, ItemID: items[1].ID, AgentID: "worker", Generation: 1}
	if err := saveWorkerPortScanStage(context.Background(), db, &payload, "portscan:alive", "存活探测"); err != nil {
		t.Fatal(err)
	}
	if err := AggregateAgentTaskProgress(db, parent.ID, 1); err != nil {
		t.Fatal(err)
	}
	var got database.VulnScanTask
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	checkpoint := taskstate.ScanCheckpoint(got, "portscan", true)
	if got.Progress != 33 || got.Status != "running" || checkpoint.Progress != 50 || checkpoint.Meta["totalCount"] != float64(2) {
		t.Fatalf("agent aggregation replaced overall progress or counted another module: parent=%#v checkpoint=%#v", got, checkpoint)
	}
	if err := db.Model(&database.VulnScanTaskItem{}).Where("id = ?", items[1].ID).Update("status", "completed").Error; err != nil {
		t.Fatal(err)
	}
	if err := AggregateAgentTaskProgress(db, parent.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Progress != 33 || got.Status != "running" || taskstate.ScanCheckpoint(got, "portscan", true).Stage != "portscan:completed" {
		t.Fatalf("agent module completion prematurely completed the combination: %#v", got)
	}
	events, _ := taskstate.ParseCheckpoint(got.CheckpointJSON).Meta["stageEvents"].([]interface{})
	if len(events) != 2 {
		t.Fatalf("combination summary lost the port scan timeline: %#v", events)
	}
}
