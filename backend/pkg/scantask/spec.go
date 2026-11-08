package scantask

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Canonical schedule kinds. A schedule has exactly one representation; the
// legacy mode/alarm fields are only accepted as input and converted here.
const (
	ScheduleKindOnce     = "once"
	ScheduleKindDaily    = "daily"
	ScheduleKindWeekdays = "weekdays"
	ScheduleKindWeekly   = "weekly"
	ScheduleKindMonthly  = "monthly"
	ScheduleKindCron     = "cron"
)

const (
	maxScheduleTimes   = 24
	scheduleSearchDays = 400
)

// ScheduleSpec is the single canonical representation of a recurrence rule.
// Clock times are complete HH:MM values, never a product of hour and minute
// sets, so "02:30 and 18:45" cannot silently become four runs.
type ScheduleSpec struct {
	Kind       string   `json:"kind"`
	Times      []string `json:"times,omitempty"`
	Date       string   `json:"date,omitempty"`
	Weekdays   []int    `json:"weekdays,omitempty"`
	DayOfMonth int      `json:"dayOfMonth,omitempty"`
	Cron       []string `json:"cron,omitempty"`
}

// ScheduleConfig pairs a spec with its enable switch and clock. A nil Spec
// means the task is manual: there is no schedule at all.
type ScheduleConfig struct {
	Enabled bool          `json:"enabled"`
	Spec    *ScheduleSpec `json:"spec,omitempty"`
}

func NormalizeScheduleSpec(value *ScheduleSpec) (*ScheduleSpec, error) {
	if value == nil {
		return nil, fmt.Errorf("schedule spec is required")
	}
	out := *value
	out.Kind = strings.ToLower(strings.TrimSpace(out.Kind))
	out.Date = strings.TrimSpace(out.Date)
	out.Times = append([]string(nil), out.Times...)
	out.Weekdays = append([]int(nil), out.Weekdays...)
	out.Cron = append([]string(nil), out.Cron...)

	switch out.Kind {
	case ScheduleKindOnce:
		out.Weekdays = nil
		out.DayOfMonth = 0
		out.Cron = nil
		if _, err := time.Parse("2006-01-02", out.Date); err != nil {
			return nil, fmt.Errorf("invalid schedule date")
		}
		times, err := normalizeClockTimes(out.Times)
		if err != nil {
			return nil, err
		}
		if len(times) != 1 {
			return nil, fmt.Errorf("one-time schedule must contain exactly one time")
		}
		out.Times = times
	case ScheduleKindDaily, ScheduleKindWeekdays:
		out.Date = ""
		out.Weekdays = nil
		out.DayOfMonth = 0
		out.Cron = nil
		times, err := normalizeClockTimes(out.Times)
		if err != nil {
			return nil, err
		}
		out.Times = times
	case ScheduleKindWeekly:
		out.Date = ""
		out.DayOfMonth = 0
		out.Cron = nil
		times, err := normalizeClockTimes(out.Times)
		if err != nil {
			return nil, err
		}
		out.Times = times
		out.Weekdays = normalizeWeekdays(out.Weekdays)
		if len(out.Weekdays) == 0 {
			return nil, fmt.Errorf("select at least one weekday")
		}
	case ScheduleKindMonthly:
		out.Date = ""
		out.Weekdays = nil
		out.Cron = nil
		times, err := normalizeClockTimes(out.Times)
		if err != nil {
			return nil, err
		}
		out.Times = times
		if out.DayOfMonth != -1 && (out.DayOfMonth < 1 || out.DayOfMonth > 31) {
			return nil, fmt.Errorf("schedule day must be 1-31 or -1 (last day)")
		}
	case ScheduleKindCron:
		out.Date = ""
		out.Times = nil
		out.Weekdays = nil
		out.DayOfMonth = 0
		cron, err := normalizeCronLines(out.Cron)
		if err != nil {
			return nil, err
		}
		out.Cron = cron
	default:
		return nil, fmt.Errorf("unsupported schedule kind: %s", value.Kind)
	}
	return &out, nil
}

func NormalizeScheduleConfig(value ScheduleConfig) (ScheduleConfig, error) {
	if value.Spec == nil {
		return ScheduleConfig{Enabled: false}, nil
	}
	spec, err := NormalizeScheduleSpec(value.Spec)
	if err != nil {
		return ScheduleConfig{}, err
	}
	return ScheduleConfig{Enabled: value.Enabled, Spec: spec}, nil
}

func normalizeClockTimes(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one schedule time is required")
	}
	if len(values) > maxScheduleTimes {
		return nil, fmt.Errorf("schedule supports at most %d times", maxScheduleTimes)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		hour, minute, err := parseScheduleClock(value)
		if err != nil {
			return nil, err
		}
		result = append(result, fmt.Sprintf("%02d:%02d", hour, minute))
	}
	slices.Sort(result)
	return slices.Compact(result), nil
}

func normalizeCronLines(lines []string) ([]string, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("at least one schedule cron expression is required")
	}
	if len(lines) > maxScheduleTimes {
		return nil, fmt.Errorf("schedule supports at most %d cron expressions", maxScheduleTimes)
	}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		expression := strings.Join(strings.Fields(line), " ")
		if expression == "" {
			return nil, fmt.Errorf("schedule cron expression is required")
		}
		if _, err := parseCronExpression(expression); err != nil {
			return nil, err
		}
		result = append(result, expression)
	}
	return slices.Compact(result), nil
}

// Next returns the first run strictly after the given instant. ok is false when
// the schedule is exhausted (a one-time run in the past).
func (s ScheduleSpec) Next(after time.Time, loc *time.Location) (time.Time, bool, error) {
	spec, err := NormalizeScheduleSpec(&s)
	if err != nil {
		return time.Time{}, false, err
	}
	if loc == nil {
		loc = time.Local
	}
	if spec.Kind == ScheduleKindCron {
		var earliest time.Time
		found := false
		for _, expression := range spec.Cron {
			next, err := nextCronRunAt(expression, after.In(loc))
			if err != nil {
				return time.Time{}, false, err
			}
			if next == nil {
				continue
			}
			if !found || next.Before(earliest) {
				earliest, found = *next, true
			}
		}
		if !found {
			return time.Time{}, false, nil
		}
		return earliest, true, nil
	}
	if spec.Kind == ScheduleKindOnce {
		date, err := time.ParseInLocation("2006-01-02", spec.Date, loc)
		if err != nil {
			return time.Time{}, false, err
		}
		hour, minute, err := parseScheduleClock(spec.Times[0])
		if err != nil {
			return time.Time{}, false, err
		}
		candidate := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, loc)
		if !candidate.After(after) {
			return time.Time{}, false, nil
		}
		return candidate, true, nil
	}

	local := after.In(loc)
	startDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	for offset := 0; offset < scheduleSearchDays; offset++ {
		day := startDay.AddDate(0, 0, offset)
		if !scheduleDayMatches(spec, day) {
			continue
		}
		for _, clock := range spec.Times {
			hour, minute, err := parseScheduleClock(clock)
			if err != nil {
				return time.Time{}, false, err
			}
			candidate := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc)
			// A clock time skipped by a DST transition must not roll into a
			// different hour (which would run the scan twice that day).
			if candidate.Hour() != hour || candidate.Minute() != minute {
				continue
			}
			if candidate.After(after) {
				return candidate, true, nil
			}
		}
	}
	return time.Time{}, false, fmt.Errorf("failed to compute next schedule within %d days", scheduleSearchDays)
}

func scheduleDayMatches(spec *ScheduleSpec, day time.Time) bool {
	switch spec.Kind {
	case ScheduleKindDaily:
		return true
	case ScheduleKindWeekdays:
		return weekdayToScheduleValue(day.Weekday()) <= 5
	case ScheduleKindWeekly:
		return slices.Contains(spec.Weekdays, weekdayToScheduleValue(day.Weekday()))
	case ScheduleKindMonthly:
		if spec.DayOfMonth == -1 {
			last := time.Date(day.Year(), day.Month()+1, 0, 0, 0, 0, 0, day.Location()).Day()
			return day.Day() == last
		}
		return day.Day() == spec.DayOfMonth
	default:
		return false
	}
}

// NextRunAt is the single entry point used by create, update, preview and the
// dispatcher. A nil result means manual or disabled.
func NextRunAt(config ScheduleConfig, from time.Time) (*time.Time, error) {
	if config.Spec == nil || !config.Enabled {
		return nil, nil
	}
	config, err := NormalizeScheduleConfig(config)
	if err != nil {
		return nil, err
	}
	next, ok, err := config.Spec.Next(from, time.Local)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return &next, nil
}

// ValidateScheduleConfig rejects a past one-time schedule when it would be
// activated. Disabled schedules may keep an expired spec for later editing.
func ValidateScheduleConfig(config ScheduleConfig, now time.Time) error {
	if config.Spec == nil || !config.Enabled {
		return nil
	}
	config, err := NormalizeScheduleConfig(config)
	if err != nil {
		return err
	}
	next, err := NextRunAt(config, now)
	if err != nil {
		return err
	}
	if next == nil && config.Spec.Kind == ScheduleKindOnce {
		return fmt.Errorf("one-time schedule must be in the future")
	}
	return nil
}

func MarshalScheduleSpec(value *ScheduleSpec) string {
	if value == nil {
		return ""
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(payload)
}

func UnmarshalScheduleSpec(raw string) *ScheduleSpec {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var spec ScheduleSpec
	if json.Unmarshal([]byte(trimmed), &spec) != nil {
		return nil
	}
	if strings.TrimSpace(spec.Kind) == "" {
		return nil
	}
	return &spec
}

// ParseScheduleConfigFromMap accepts the canonical {enabled, spec} payload.
// A payload without a spec is a manual task; schedules always follow the
// server's local timezone.
func ParseScheduleConfigFromMap(raw map[string]interface{}) (ScheduleConfig, error) {
	if raw == nil {
		return ScheduleConfig{}, nil
	}
	if nested, ok := raw["schedule"].(map[string]interface{}); ok {
		raw = nested
	}
	if specRaw, ok := raw["spec"]; ok && specRaw != nil {
		payload, err := json.Marshal(specRaw)
		if err != nil {
			return ScheduleConfig{}, err
		}
		var spec ScheduleSpec
		if err := json.Unmarshal(payload, &spec); err != nil {
			return ScheduleConfig{}, fmt.Errorf("invalid schedule spec: %w", err)
		}
		enabled := true
		if value, ok := raw["enabled"].(bool); ok {
			enabled = value
		}
		return NormalizeScheduleConfig(ScheduleConfig{Enabled: enabled, Spec: &spec})
	}
	return ScheduleConfig{}, nil
}

// ScheduleConfigFromSpec accepts the raw canonical columns so the dispatcher
// does not need to materialise a versioned task row.
func ScheduleConfigFromSpec(specRaw string, enabled *bool) (ScheduleConfig, error) {
	spec := UnmarshalScheduleSpec(specRaw)
	if spec == nil {
		return NormalizeScheduleConfig(ScheduleConfig{Enabled: false})
	}
	return NormalizeScheduleConfig(ScheduleConfig{
		Enabled: enabled == nil || *enabled,
		Spec:    spec,
	})
}

func splitCronLines(raw string) []string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}
