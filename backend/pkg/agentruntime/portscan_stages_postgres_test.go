package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/portscan"
	"hephaestus-backend/pkg/taskstate"

	"gorm.io/gorm"
)

func TestPostgresPortScanWorkerStages(t *testing.T) {
	db, local, remote := postgresScanWorkerFixture(t)
	ctx := context.Background()

	for _, scenario := range []struct {
		name    string
		noHosts bool
	}{
		{name: "port scan"},
		{name: "no confirmed hosts", noHosts: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			task := database.VulnScanTask{
				ID: scenario.name, Version: 1, ScanType: "portscan", ScanMode: "local", Status: "running",
				CheckpointJSON: taskstate.MarshalCheckpoint("prepare", 0, map[string]interface{}{"legacy": true}),
			}
			if err := db.Create(&task).Error; err != nil {
				t.Fatal(err)
			}
			lease := time.Now().Add(time.Minute)
			items := []database.VulnScanTaskItem{
				{TaskID: task.ID, Version: 1, ScanType: "portscan", Target: "127.0.0.1", Status: "running", WorkerAgentID: local.AgentID, WorkerGeneration: 2, LeaseExpiresAt: &lease},
				{TaskID: task.ID, Version: 1, ScanType: "portscan", Target: "127.0.0.2", Status: "running", WorkerAgentID: remote.AgentID, WorkerGeneration: 2, LeaseExpiresAt: &lease},
			}
			if err := db.Create(&items).Error; err != nil {
				t.Fatal(err)
			}
			payload := TaskPayload{ID: task.ID, Version: 1, ItemID: items[0].ID, AgentID: local.AgentID, Generation: 1}
			if err := saveWorkerPortScanStage(ctx, db, &payload, "portscan:subnets", "网段预探测"); !errors.Is(err, context.Canceled) {
				t.Fatalf("stale worker wrote a stage: %v", err)
			}
			payload.Generation = 2
			second := TaskPayload{ID: task.ID, Version: 1, ItemID: items[1].ID, AgentID: remote.AgentID, Generation: 2}
			writes := make(chan error, 2)
			for _, worker := range []*TaskPayload{&payload, &second} {
				go func() { writes <- saveWorkerPortScanStage(ctx, db, worker, "portscan:alive", "存活探测") }()
			}
			for range items {
				if err := <-writes; err != nil {
					t.Fatal(err)
				}
			}
			checkpoint, events := loadWorkerPortScanStages(t, db, task.ID)
			if len(events) != 1 || events[0]["stage"] != "portscan:alive" || checkpoint.Meta["legacy"] != true {
				t.Fatalf("running task lost metadata or repeated stages: %#v", checkpoint)
			}
			if !scenario.noHosts {
				if err := saveWorkerPortScanStage(ctx, db, &second, "portscan:scan", "端口扫描 · CONNECT"); err != nil {
					t.Fatal(err)
				}
			}
			for i, item := range items {
				if err := db.Model(&database.VulnScanTaskItem{}).Where("id = ?", item.ID).
					Updates(map[string]interface{}{"status": "completed", "port_scan_no_hosts": scenario.noHosts}).Error; err != nil {
					t.Fatal(err)
				}
				if err := AggregateAgentTaskProgress(db, task.ID, 1); err != nil {
					t.Fatal(err)
				}
				_, events := loadWorkerPortScanStages(t, db, task.ID)
				last := events[len(events)-1]
				if i == 0 && last["stage"] == portscan.StageComplete {
					t.Fatal("timeline finished before the other worker exited")
				}
				if i == len(items)-1 {
					wantLabel, wantCount := "扫描结束", 3
					if scenario.noHosts {
						wantLabel, wantCount = "未确认可扫描主机", 2
					}
					if last["stage"] != portscan.StageComplete || last["label"] != wantLabel || len(events) != wantCount {
						t.Fatalf("incorrect completed timeline: %#v", events)
					}
				}
			}
		})
	}
}

func loadWorkerPortScanStages(t *testing.T, db *gorm.DB, taskID string) (taskstate.Checkpoint, []map[string]interface{}) {
	t.Helper()
	var task database.VulnScanTask
	if err := db.Where("id = ? AND version = 1", taskID).First(&task).Error; err != nil {
		t.Fatal(err)
	}
	checkpoint := taskstate.ParseCheckpoint(task.CheckpointJSON)
	raw, _ := checkpoint.Meta["stageEvents"].([]interface{})
	events := make([]map[string]interface{}, len(raw))
	for i, event := range raw {
		events[i] = event.(map[string]interface{})
	}
	return checkpoint, events
}
