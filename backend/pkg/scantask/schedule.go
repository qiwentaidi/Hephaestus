package scantask

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/taskstate"
)

type cronField struct {
	Any    bool
	Values map[int]struct{}
}

type parsedCronSchedule struct {
	Minutes     cronField
	Hours       cronField
	DaysOfMonth cronField
	Months      cronField
	DaysOfWeek  cronField
}

func normalizeWeekdays(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value < 1 || value > 7 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func UnmarshalScheduleWeekdays(raw string, fallback int) []int {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		var values []int
		if err := json.Unmarshal([]byte(trimmed), &values); err == nil {
			return normalizeWeekdays(values)
		}
	}
	if fallback >= 1 && fallback <= 7 {
		return []int{fallback}
	}
	return nil
}

func parseScheduleClock(raw string) (int, int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, 0, fmt.Errorf("schedule time is required")
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		// Accept a single-digit hour while the time picker is being typed.
		parsed, err = time.Parse("3:04", value)
	}
	if err != nil {
		return 0, 0, fmt.Errorf("invalid schedule time: %s", value)
	}
	return parsed.Hour(), parsed.Minute(), nil
}

func parseCronExpression(raw string) (parsedCronSchedule, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return parsedCronSchedule{}, fmt.Errorf("schedule cron expression is required")
	}
	parts := strings.Fields(value)
	if len(parts) != 5 {
		return parsedCronSchedule{}, fmt.Errorf("schedule cron expression must contain 5 fields")
	}

	minutes, err := parseCronField("minute", parts[0], 0, 59, false)
	if err != nil {
		return parsedCronSchedule{}, err
	}
	hours, err := parseCronField("hour", parts[1], 0, 23, false)
	if err != nil {
		return parsedCronSchedule{}, err
	}
	daysOfMonth, err := parseCronField("day of month", parts[2], 1, 31, true)
	if err != nil {
		return parsedCronSchedule{}, err
	}
	months, err := parseCronField("month", parts[3], 1, 12, false)
	if err != nil {
		return parsedCronSchedule{}, err
	}
	daysOfWeek, err := parseCronField("day of week", parts[4], 0, 7, true)
	if err != nil {
		return parsedCronSchedule{}, err
	}

	return parsedCronSchedule{
		Minutes:     minutes,
		Hours:       hours,
		DaysOfMonth: daysOfMonth,
		Months:      months,
		DaysOfWeek:  daysOfWeek,
	}, nil
}

func parseCronField(name, raw string, minValue, maxValue int, allowQuestion bool) (cronField, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return cronField{}, fmt.Errorf("cron %s field is required", name)
	}
	if value == "*" || (allowQuestion && value == "?") {
		return cronField{Any: true}, nil
	}

	result := cronField{Values: make(map[int]struct{})}
	for _, token := range strings.Split(value, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			return cronField{}, fmt.Errorf("invalid cron %s field: %s", name, raw)
		}
		if token == "*" || (allowQuestion && token == "?") {
			result.Any = true
			continue
		}

		base := token
		step := 1
		if strings.Contains(token, "/") {
			pieces := strings.Split(token, "/")
			if len(pieces) != 2 {
				return cronField{}, fmt.Errorf("invalid cron %s field: %s", name, raw)
			}
			base = strings.TrimSpace(pieces[0])
			parsedStep, err := strconv.Atoi(strings.TrimSpace(pieces[1]))
			if err != nil || parsedStep <= 0 {
				return cronField{}, fmt.Errorf("invalid cron %s step: %s", name, token)
			}
			step = parsedStep
		}

		start, end, err := parseCronFieldRange(name, base, minValue, maxValue, allowQuestion)
		if err != nil {
			return cronField{}, err
		}
		for current := start; current <= end; current += step {
			result.Values[current] = struct{}{}
		}
	}
	if len(result.Values) == 0 && !result.Any {
		return cronField{}, fmt.Errorf("invalid cron %s field: %s", name, raw)
	}
	return result, nil
}

func parseCronFieldRange(name, raw string, minValue, maxValue int, allowQuestion bool) (int, int, error) {
	value := strings.TrimSpace(raw)
	if value == "*" || (allowQuestion && value == "?") {
		return minValue, maxValue, nil
	}
	if strings.Contains(value, "-") {
		pieces := strings.Split(value, "-")
		if len(pieces) != 2 {
			return 0, 0, fmt.Errorf("invalid cron %s range: %s", name, raw)
		}
		start, err := strconv.Atoi(strings.TrimSpace(pieces[0]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid cron %s range: %s", name, raw)
		}
		end, err := strconv.Atoi(strings.TrimSpace(pieces[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid cron %s range: %s", name, raw)
		}
		if start > end {
			return 0, 0, fmt.Errorf("invalid cron %s range: %s", name, raw)
		}
		if start < minValue || end > maxValue {
			return 0, 0, fmt.Errorf("cron %s value out of range: %s", name, raw)
		}
		return start, end, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid cron %s value: %s", name, raw)
	}
	if parsed < minValue || parsed > maxValue {
		return 0, 0, fmt.Errorf("cron %s value out of range: %s", name, raw)
	}
	return parsed, parsed, nil
}

func sortedCronValues(values map[int]struct{}) []int {
	if len(values) == 0 {
		return nil
	}
	result := make([]int, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func cronFieldMatches(field cronField, value int) bool {
	if field.Any {
		return true
	}
	_, ok := field.Values[value]
	return ok
}

func cronDayOfWeekMatches(field cronField, weekday time.Weekday) bool {
	if field.Any {
		return true
	}
	if _, ok := field.Values[int(weekday)]; ok {
		return true
	}
	if weekday == time.Sunday {
		_, ok := field.Values[7]
		return ok
	}
	return false
}

func nextCronRunAt(cronExpression string, from time.Time) (*time.Time, error) {
	cron, err := parseCronExpression(cronExpression)
	if err != nil {
		return nil, err
	}

	loc := from.Location()
	candidate := from.In(loc).Truncate(time.Minute)
	if candidate.Before(from) {
		candidate = candidate.Add(time.Minute)
	}
	maxCandidate := candidate.AddDate(5, 0, 0)
	for !candidate.After(maxCandidate) {
		if cronFieldMatches(cron.Minutes, candidate.Minute()) &&
			cronFieldMatches(cron.Hours, candidate.Hour()) &&
			cronFieldMatches(cron.Months, int(candidate.Month())) &&
			cronFieldMatches(cron.DaysOfMonth, candidate.Day()) &&
			cronDayOfWeekMatches(cron.DaysOfWeek, candidate.Weekday()) {
			result := candidate
			return &result, nil
		}
		candidate = candidate.Add(time.Minute)
	}
	return nil, fmt.Errorf("failed to compute next schedule")
}

func weekdayToScheduleValue(weekday time.Weekday) int {
	if weekday == time.Sunday {
		return 7
	}
	return int(weekday)
}

func cronDayOfWeekToScheduleValue(weekday int) int {
	if weekday == 0 || weekday == 7 {
		return 7
	}
	return weekday
}

func IsSchedulableTerminalStatus(status string) bool {
	switch status {
	case taskstate.StatusPending, taskstate.StatusCompleted, taskstate.StatusFailed, taskstate.StatusCancelled:
		return true
	default:
		return false
	}
}
