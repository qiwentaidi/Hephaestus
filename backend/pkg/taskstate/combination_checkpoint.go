package taskstate

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const moduleCheckpointsKey = "moduleCheckpoints"

// ScanCheckpoint isolates resume state from the other modules of a combination.
// Old checkpoints are usable only when their stage identifies this module.
func ScanCheckpoint(task database.VulnScanTask, module string, combination bool) Checkpoint {
	checkpoint := ParseCheckpoint(task.CheckpointJSON)
	if !combination {
		return checkpoint
	}
	if saved, ok := moduleCheckpoints(checkpoint)[module]; ok {
		return saved
	}
	if strings.HasPrefix(checkpoint.Stage, module+":") {
		return checkpoint
	}
	return Checkpoint{}
}

func moduleCheckpoints(checkpoint Checkpoint) map[string]Checkpoint {
	modules := make(map[string]Checkpoint)
	if raw, ok := checkpoint.Meta[moduleCheckpointsKey]; ok {
		data, err := json.Marshal(raw)
		if err == nil {
			_ = json.Unmarshal(data, &modules)
		}
	}
	if modules == nil {
		modules = make(map[string]Checkpoint)
	}
	if len(modules) == 0 {
		if legacyModule, _, ok := strings.Cut(checkpoint.Stage, ":"); ok {
			modules[legacyModule] = checkpoint
		} else if events := checkpointEvents(checkpoint.Meta["stageEvents"]); len(events) > 0 {
			modules["legacy"] = Checkpoint{Meta: map[string]interface{}{"stageEvents": events}}
		}
	}
	return modules
}

// CombinationSummaryCheckpoint changes the overall percentage while retaining
// module resume state, including the last attributable historical checkpoint.
func CombinationSummaryCheckpoint(raw string, progress int) string {
	modules := moduleCheckpoints(ParseCheckpoint(raw))
	var events []map[string]interface{}
	keys := make([]string, 0, len(modules))
	for key := range modules {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		events = append(events, checkpointEvents(modules[key].Meta["stageEvents"])...)
	}
	sort.SliceStable(events, func(i, j int) bool {
		a, _ := events[i]["time"].(string)
		b, _ := events[j]["time"].(string)
		ta, _ := time.Parse(time.RFC3339Nano, a)
		tb, _ := time.Parse(time.RFC3339Nano, b)
		return ta.Before(tb)
	})
	return MarshalCheckpoint("combination", progress, map[string]interface{}{
		moduleCheckpointsKey: modules, "stageEvents": events,
	})
}

func checkpointEvents(raw interface{}) []map[string]interface{} {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var events []map[string]interface{}
	_ = json.Unmarshal(data, &events)
	return events
}

func mergeCheckpointEvents(previous, incoming interface{}) []map[string]interface{} {
	events := checkpointEvents(previous)
	indices := make(map[string]int)
	for i, event := range events {
		stage, _ := event["stage"].(string)
		indices[stage] = i
	}
	for _, event := range checkpointEvents(incoming) {
		stage, _ := event["stage"].(string)
		if idx, ok := indices[stage]; ok {
			oldTime, _ := events[idx]["time"].(string)
			newTime, _ := event["time"].(string)
			oldAt, _ := time.Parse(time.RFC3339Nano, oldTime)
			newAt, _ := time.Parse(time.RFC3339Nano, newTime)
			if !newAt.Before(oldAt) {
				events[idx] = event
			}
		} else {
			indices[stage] = len(events)
			events = append(events, event)
		}
	}
	return events
}

// SaveCombinationCheckpoint stores each module separately under a locked parent
// row. Only the scheduler writes the parent's overall progress and lifecycle.
func SaveCombinationCheckpoint(db *gorm.DB, taskID string, version int, module, stage string, progress int, meta map[string]interface{}) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var task database.VulnScanTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND version = ?", taskID, version).First(&task).Error; err != nil {
			return err
		}
		if task.Status == StatusCancelled || task.Status == StatusDeleting {
			return context.Canceled
		}
		if task.Status == StatusCompleted {
			return nil
		}
		checkpoint := ParseCheckpoint(task.CheckpointJSON)
		modules := moduleCheckpoints(checkpoint)
		previous := modules[module]
		nextMeta := make(map[string]interface{}, len(meta)+1)
		for key, value := range meta {
			nextMeta[key] = value
		}
		if progress < previous.Progress {
			stage, progress = previous.Stage, previous.Progress
			nextMeta = make(map[string]interface{}, len(previous.Meta)+1)
			for key, value := range previous.Meta {
				nextMeta[key] = value
			}
		}
		if events := mergeCheckpointEvents(previous.Meta["stageEvents"], meta["stageEvents"]); len(events) > 0 {
			nextMeta["stageEvents"] = events
		}
		now := time.Now()
		modules[module] = Checkpoint{Stage: stage, Progress: progress, Meta: nextMeta, UpdatedAt: now}
		raw := MarshalCheckpoint("combination", task.Progress, map[string]interface{}{moduleCheckpointsKey: modules})
		return tx.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", taskID, version).
			Updates(map[string]interface{}{
				"current_stage":   "combination",
				"checkpoint_json": CombinationSummaryCheckpoint(raw, task.Progress),
				"heartbeat_at":    now, "updated_at": now,
			}).Error
	})
}

func CombinationCheckpointHandlers(module string) (
	func(*gorm.DB, string, int, string, int, map[string]interface{}) error,
	func(context.Context, *gorm.DB, string, int, string, int, map[string]interface{}) error,
) {
	save := func(db *gorm.DB, taskID string, version int, stage string, progress int, meta map[string]interface{}) error {
		return SaveCombinationCheckpoint(db, taskID, version, module, stage, progress, meta)
	}
	pause := func(ctx context.Context, db *gorm.DB, taskID string, version int, stage string, progress int, meta map[string]interface{}) error {
		if err := CheckTaskControlState(ctx, db, taskID, version); err != nil && err != ErrTaskPaused {
			return err
		}
		if err := save(db, taskID, version, stage, progress, meta); err != nil {
			return err
		}
		// The parent may be marked paused only after every module exits.
		return CheckTaskControlState(ctx, db, taskID, version)
	}
	return save, pause
}
