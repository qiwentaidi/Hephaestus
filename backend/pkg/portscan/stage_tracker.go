package portscan

import (
	"context"
	"sync"
	"time"

	"hephaestus-backend/pkg/taskstate"
)

const (
	portScanStageSubnet = "portscan:subnets"
	portScanStageAlive  = "portscan:alive"
	portScanStageScan   = "portscan:scan"
	StageComplete       = "portscan:complete"
	StageNoHosts        = "portscan:no_hosts"
)

type stageReporterKey struct{}

// WithStageReporter lets the Worker publish stages without sharing its resume state.
func WithStageReporter(ctx context.Context, report func(context.Context, string, string) error) context.Context {
	return context.WithValue(ctx, stageReporterKey{}, report)
}

func reportStage(ctx context.Context, stage, label string) error {
	if report, ok := ctx.Value(stageReporterKey{}).(func(context.Context, string, string) error); ok {
		return report(ctx, stage, label)
	}
	return nil
}

type portScanStageEvent struct {
	Time  time.Time `json:"time"`
	Stage string    `json:"stage"`
	Label string    `json:"label"`
}

type portScanStageTracker struct {
	mu      sync.Mutex
	events  []portScanStageEvent
	lastIdx map[string]int
}

func newPortScanStageTracker(checkpoint taskstate.Checkpoint) *portScanStageTracker {
	tracker := &portScanStageTracker{lastIdx: make(map[string]int)}
	if checkpoint.Meta == nil {
		return tracker
	}
	rawEvents, ok := checkpoint.Meta["stageEvents"].([]interface{})
	if !ok {
		return tracker
	}
	for _, rawEvent := range rawEvents {
		fields, ok := rawEvent.(map[string]interface{})
		if !ok {
			continue
		}
		stage, _ := fields["stage"].(string)
		label, _ := fields["label"].(string)
		rawTime, _ := fields["time"].(string)
		if stage == "" || label == "" {
			continue
		}
		eventTime, err := time.Parse(time.RFC3339Nano, rawTime)
		if err != nil {
			eventTime = time.Now()
		}
		tracker.events = append(tracker.events, portScanStageEvent{
			Time:  eventTime,
			Stage: stage,
			Label: label,
		})
		tracker.lastIdx[stage] = len(tracker.events) - 1
	}
	return tracker
}

func (t *portScanStageTracker) Record(stage, label string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	if idx, ok := t.lastIdx[stage]; ok {
		t.events[idx].Time = now
		t.events[idx].Label = label
		return
	}
	t.events = append(t.events, portScanStageEvent{Time: now, Stage: stage, Label: label})
	t.lastIdx[stage] = len(t.events) - 1
}

func (t *portScanStageTracker) Events() []portScanStageEvent {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make([]portScanStageEvent, len(t.events))
	copy(out, t.events)
	return out
}

func (t *portScanStageTracker) Meta(fields map[string]interface{}) map[string]interface{} {
	meta := make(map[string]interface{}, len(fields)+1)
	for key, value := range fields {
		meta[key] = value
	}
	meta["stageEvents"] = t.Events()
	return meta
}
