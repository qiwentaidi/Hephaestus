package scan

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"hephaestus-backend/pkg/agentruntime"
	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/taskstate"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCombinationTargetsForWebModuleFallsBackBeforeDerivation(t *testing.T) {
	ctx := &combinationContext{
		InitialTargets: []string{"192.168.38.0/24"},
	}

	targets, err := ctx.targetsFor("dirscan")
	if err != nil {
		t.Fatalf("targetsFor returned error: %v", err)
	}
	if !reflect.DeepEqual(targets, []string{"192.168.38.0/24"}) {
		t.Fatalf("unexpected fallback targets: %#v", targets)
	}
}

func TestCombinationTargetsForWebModuleSkipsAfterEmptyPortscanDerivation(t *testing.T) {
	ctx := &combinationContext{
		InitialTargets:  []string{"192.168.38.0/24"},
		WebTargetsReady: true,
	}

	targets, err := ctx.targetsFor("dirscan")
	if err != nil {
		t.Fatalf("targetsFor returned error: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("expected no targets after empty derivation, got %#v", targets)
	}
}

func TestCombinationTargetsForWebModuleUsesDerivedTargets(t *testing.T) {
	ctx := &combinationContext{
		InitialTargets:  []string{"192.168.38.0/24"},
		WebTargets:      []string{"http://192.168.38.8:80", "https://192.168.38.9:443"},
		WebTargetsReady: true,
	}

	targets, err := ctx.targetsFor("dirscan")
	if err != nil {
		t.Fatalf("targetsFor returned error: %v", err)
	}
	want := []string{"http://192.168.38.8:80", "https://192.168.38.9:443"}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("unexpected derived targets: got %#v want %#v", targets, want)
	}
}

func TestMergeCombinationContextPreservesQueuedStepFromParallelScheduler(t *testing.T) {
	now := time.Now()
	queuedAt := now.Add(2 * time.Second)

	base := &combinationContext{
		Completed: map[string]bool{
			"portscan": true,
			"webscan":  true,
			"crack":    true,
		},
		Running: map[string]bool{
			"jsscan": true,
		},
		Steps: []*combinationStepLog{
			{Module: "crack", TaskID: "task-1", Status: "completed", StartedAt: &now, CompletedAt: &now},
			{Module: "webscan", TaskID: "task-1", Status: "completed", StartedAt: &now, CompletedAt: &now},
			{Module: "jsscan", TaskID: "task-1", Status: "queued", StartedAt: &queuedAt},
		},
	}
	overlay := &combinationContext{
		Completed: map[string]bool{
			"portscan": true,
			"webscan":  true,
			"crack":    true,
		},
		Running: map[string]bool{
			"crack": true,
		},
		Steps: []*combinationStepLog{
			{Module: "crack", TaskID: "task-1", Status: "completed", StartedAt: &now, CompletedAt: &now},
			{Module: "webscan", TaskID: "task-1", Status: "completed", StartedAt: &now, CompletedAt: &now},
			{Module: "jsscan", TaskID: "task-1", Status: "pending"},
		},
	}

	merged := mergeCombinationContext(base, overlay)
	step := merged.stepLog("jsscan")
	if step == nil {
		t.Fatal("expected merged jsscan step")
	}
	if step.Status != "queued" {
		t.Fatalf("expected jsscan step to stay queued, got %q", step.Status)
	}
	if step.StartedAt == nil || !step.StartedAt.Equal(queuedAt) {
		t.Fatalf("expected queued timestamp to be preserved, got %#v", step.StartedAt)
	}
}

func TestCombinationResultTaskIDsIncludesParentAndChildren(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	task := database.VulnScanTask{
		ID:                  "parent-task",
		Version:             1,
		ScanType:            "combination",
		ParentCombinationID: "combo-1",
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := db.Create(&database.VulnScanTask{
		ID:                  "child-webscan",
		Version:             1,
		ScanType:            "webscan",
		ParentCombinationID: "combo-1",
	}).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}

	ids, err := combinationResultTaskIDs(db, task, 1)
	if err != nil {
		t.Fatalf("combinationResultTaskIDs returned error: %v", err)
	}
	want := []string{"parent-task", "child-webscan"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("unexpected task ids: got %#v want %#v", ids, want)
	}
}

func TestPauseCombinationTaskCancelsRunningSubtasks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.CombinationTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	combo := database.CombinationTask{
		ID:      "combo-pause-cancel",
		Name:    "combo",
		Targets: `["https://example.com"]`,
		Nodes:   `[{"id":"node-1","moduleId":"webscan","stageIndex":0}]`,
		Status:  taskstate.StatusRunning,
	}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatalf("create combo: %v", err)
	}
	parentTask := database.VulnScanTask{
		ID:                  "task-pause-cancel",
		Version:             1,
		TaskName:            "combo parent",
		ScanType:            "combination",
		ParentCombinationID: combo.ID,
		Status:              taskstate.StatusRunning,
	}
	if err := db.Create(&parentTask).Error; err != nil {
		t.Fatalf("create parent task: %v", err)
	}

	ctx, cancel := control.GetTaskController().CreateSubTaskContext(parentTask.ID, "0-webscan")
	defer cancel()

	if err := pauseCombinationTask(db, &parentTask); err != nil {
		t.Fatalf("pause combination task: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("expected running combination subtask context to be cancelled")
	}

	var gotCombo database.CombinationTask
	if err := db.Where("id = ?", combo.ID).First(&gotCombo).Error; err != nil {
		t.Fatalf("load combo: %v", err)
	}
	if !gotCombo.PauseRequested {
		t.Fatal("expected combo pause_requested to be set before cancellation")
	}
}

func TestPauseCombinationTaskFinalizesAfterRunningModuleAcknowledgesCancellation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.CombinationTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	combo := database.CombinationTask{
		ID:        "combo-pause-e2e",
		Name:      "combo",
		Targets:   `["https://example.com"]`,
		Nodes:     `[{"id":"node-1","moduleId":"webscan","stageIndex":0}]`,
		Status:    taskstate.StatusRunning,
		StartedAt: &now,
	}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatalf("create combo: %v", err)
	}
	parentTask := database.VulnScanTask{
		ID:                  "task-pause-e2e",
		Version:             1,
		TaskName:            "combo parent",
		ScanMode:            "local",
		ScanType:            "combination",
		ParentCombinationID: combo.ID,
		Status:              taskstate.StatusRunning,
		Targets:             `["https://example.com"]`,
		StartedAt:           &now,
	}
	if err := db.Create(&parentTask).Error; err != nil {
		t.Fatalf("create parent task: %v", err)
	}

	started := make(chan struct{})
	originalHandler := scanTypeHandlers["webscan"]
	scanTypeHandlers["webscan"] = func(ctx context.Context, taskID string, targets []string, config map[string]interface{}) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	defer func() {
		scanTypeHandlers["webscan"] = originalHandler
	}()

	ctxState := newCombinationContext([]string{"https://example.com"})
	nodes := []combinationPipelineNode{{ID: "node-1", ModuleID: "webscan", StageIndex: 0}}
	done := make(chan error, 1)
	go func() {
		done <- runCombinationModule(db, &combo, &parentTask, nodes, nodes[0], 0, []string{"https://example.com"}, ctxState)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("combination module did not start")
	}
	if err := pauseCombinationTask(db, &parentTask); err != nil {
		t.Fatalf("pause combination task: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run combination module: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("combination module did not acknowledge pause cancellation")
	}

	var gotCombo database.CombinationTask
	if err := db.Where("id = ?", combo.ID).First(&gotCombo).Error; err != nil {
		t.Fatalf("load combo: %v", err)
	}
	if gotCombo.Status != taskstate.StatusPaused || gotCombo.PauseRequested {
		t.Fatalf("expected finalized combo pause, got status=%q pauseRequested=%t", gotCombo.Status, gotCombo.PauseRequested)
	}
	var gotParent database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).First(&gotParent).Error; err != nil {
		t.Fatalf("load parent task: %v", err)
	}
	if gotParent.Status != taskstate.StatusPaused || gotParent.PauseRequested {
		t.Fatalf("expected finalized parent pause, got status=%q pauseRequested=%t", gotParent.Status, gotParent.PauseRequested)
	}
}

func TestRunCombinationModuleFinalizesQueuedPauseBeforeExecution(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.CombinationTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	combo := database.CombinationTask{
		ID:             "combo-1",
		Name:           "combo",
		Targets:        `["https://example.com"]`,
		Nodes:          `[{"id":"node-1","moduleId":"webscan","stageIndex":0,"laneIndex":0}]`,
		Context:        `{"runningModules":{"webscan":true},"steps":[{"module":"webscan","status":"queued","taskId":"task-1"}]}`,
		Status:         taskstate.StatusRunning,
		PauseRequested: true,
		CurrentStep:    1,
		TotalSteps:     1,
		StartedAt:      &now,
	}
	if err := db.Create(&combo).Error; err != nil {
		t.Fatalf("create combo: %v", err)
	}

	parentTask := database.VulnScanTask{
		ID:                  "task-1",
		Version:             1,
		TaskName:            "combo parent",
		ScanMode:            "local",
		ScanType:            "combination",
		ParentCombinationID: combo.ID,
		Status:              taskstate.StatusRunning,
		PauseRequested:      true,
		Targets:             `["https://example.com"]`,
		StartedAt:           &now,
	}
	if err := db.Create(&parentTask).Error; err != nil {
		t.Fatalf("create parent task: %v", err)
	}

	ctxState := &combinationContext{
		InitialTargets: []string{"https://example.com"},
		Running:        map[string]bool{"webscan": true},
		Steps: []*combinationStepLog{
			{
				Module:    "webscan",
				TaskID:    parentTask.ID,
				Status:    taskstate.StatusRunning,
				StartedAt: &now,
			},
		},
	}
	nodes := []combinationPipelineNode{
		{ID: "node-1", ModuleID: "webscan", StageIndex: 0, LaneIndex: 0},
	}

	originalHandler := scanTypeHandlers["webscan"]
	scanTypeHandlers["webscan"] = func(ctx context.Context, taskID string, targets []string, config map[string]interface{}) error {
		return context.Canceled
	}
	defer func() {
		scanTypeHandlers["webscan"] = originalHandler
	}()

	if err := runCombinationModule(db, &combo, &parentTask, nodes, nodes[0], 0, []string{"https://example.com"}, ctxState); err != nil {
		t.Fatalf("runCombinationModule returned error: %v", err)
	}

	var gotCombo database.CombinationTask
	if err := db.Where("id = ?", combo.ID).First(&gotCombo).Error; err != nil {
		t.Fatalf("load combo: %v", err)
	}
	if gotCombo.Status != taskstate.StatusPaused {
		t.Fatalf("expected combo status %q, got %q", taskstate.StatusPaused, gotCombo.Status)
	}
	if gotCombo.PauseRequested {
		t.Fatalf("expected combo pause_requested to be cleared")
	}

	var gotTask database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).First(&gotTask).Error; err != nil {
		t.Fatalf("load parent task: %v", err)
	}
	if gotTask.Status != taskstate.StatusPaused {
		t.Fatalf("expected parent task status %q, got %q", taskstate.StatusPaused, gotTask.Status)
	}
	if gotTask.PauseRequested {
		t.Fatalf("expected parent task pause_requested to be cleared")
	}
}

func TestDispatchCombinationModuleToAgentCreatesModuleTypedItems(t *testing.T) {
	dsn := fmt.Sprintf("file:dispatch_agent_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.VulnScanTaskItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	parentTask := database.VulnScanTask{
		ID:                  "task-agent-dispatch",
		Version:             1,
		TaskName:            "combo parent",
		ScanMode:            "agent",
		ScanType:            "combination",
		AssignedAgentID:     "scan-agent-1",
		Status:              taskstate.StatusRunning,
		Targets:             `["192.168.1.10"]`,
		ParentCombinationID: "combo-agent-dispatch",
		Progress:            33,
	}
	if err := db.Create(&parentTask).Error; err != nil {
		t.Fatalf("create parent task: %v", err)
	}

	node := combinationPipelineNode{ID: "node-1", ModuleID: "portscan", StageIndex: 0}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- dispatchCombinationModuleToAgent(ctx, db, &parentTask, node, 0, []string{"192.168.1.10"}, map[string]interface{}{"isCombination": true})
	}()

	deadline := time.Now().Add(2 * time.Second)
	var items []database.VulnScanTaskItem
	for time.Now().Before(deadline) {
		if err := db.Where("task_id = ? AND version = ?", parentTask.ID, parentTask.Version).Find(&items).Error; err != nil {
			t.Fatalf("load items: %v", err)
		}
		if len(items) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(items) == 0 {
		t.Fatal("expected agent task items to be created")
	}
	for _, item := range items {
		if item.ScanType != "portscan" {
			t.Fatalf("expected item scan_type to be module id %q, got %q", "portscan", item.ScanType)
		}
	}

	var gotTask database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).First(&gotTask).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if gotTask.ScanType != "combination" {
		t.Fatalf("expected parent scan_type to stay combination, got %q", gotTask.ScanType)
	}
	if gotTask.Status != taskstate.StatusPending {
		t.Fatalf("expected dispatched task status pending, got %q", gotTask.Status)
	}

	if err := db.Model(&database.VulnScanTaskItem{}).
		Where("task_id = ? AND version = ?", parentTask.ID, parentTask.Version).
		Update("status", taskstate.StatusCompleted).Error; err != nil {
		t.Fatalf("complete items: %v", err)
	}
	if err := agentruntime.AggregateAgentTaskProgress(db, parentTask.ID, parentTask.Version); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("dispatch returned error: %v", err)
		}
		if err := db.Where("id = ? AND version = ?", parentTask.ID, parentTask.Version).First(&gotTask).Error; err != nil {
			t.Fatal(err)
		}
		if gotTask.Progress != 33 || gotTask.Status == taskstate.StatusCompleted {
			t.Fatalf("agent module completed the parent: %#v", gotTask)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dispatch did not return after module completion")
	}
}
