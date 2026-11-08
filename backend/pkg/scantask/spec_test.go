package scantask

import (
	"testing"
	"time"
)

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestScheduleSpecNext(t *testing.T) {
	shanghai := mustLocation(t, "Asia/Shanghai")
	for _, tc := range []struct {
		name string
		spec ScheduleSpec
		from string
		want string
		zone string
	}{
		{"daily multiple times", ScheduleSpec{Kind: ScheduleKindDaily, Times: []string{"18:45", "02:30"}}, "2026-11-03T02:31:00+08:00", "2026-11-03T18:45:00+08:00", ""},
		{"weekdays skips weekend", ScheduleSpec{Kind: ScheduleKindWeekdays, Times: []string{"02:30"}}, "2026-11-06T03:00:00+08:00", "2026-11-09T02:30:00+08:00", ""},
		{"weekly", ScheduleSpec{Kind: ScheduleKindWeekly, Weekdays: []int{1, 5}, Times: []string{"02:30"}}, "2026-11-03T03:00:00+08:00", "2026-11-06T02:30:00+08:00", ""},
		{"monthly skips missing day", ScheduleSpec{Kind: ScheduleKindMonthly, DayOfMonth: 31, Times: []string{"02:30"}}, "2026-02-01T00:00:00+08:00", "2026-03-31T02:30:00+08:00", ""},
		{"monthly last day", ScheduleSpec{Kind: ScheduleKindMonthly, DayOfMonth: -1, Times: []string{"02:30"}}, "2028-02-01T00:00:00+08:00", "2028-02-29T02:30:00+08:00", ""},
		{"once", ScheduleSpec{Kind: ScheduleKindOnce, Date: "2026-11-10", Times: []string{"02:30"}}, "2026-11-03T00:00:00Z", "2026-11-10T02:30:00+08:00", ""},
		{"cron earliest of several", ScheduleSpec{Kind: ScheduleKindCron, Cron: []string{"0 18 * * *", "30 2 * * *"}}, "2026-11-03T03:00:00+08:00", "2026-11-03T18:00:00+08:00", ""},
		{"DST gap skipped", ScheduleSpec{Kind: ScheduleKindDaily, Times: []string{"02:30"}}, "2026-03-08T00:00:00-05:00", "2026-03-09T02:30:00-04:00", "America/New_York"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from, err := time.Parse(time.RFC3339, tc.from)
			if err != nil {
				t.Fatal(err)
			}
			want, err := time.Parse(time.RFC3339, tc.want)
			if err != nil {
				t.Fatal(err)
			}
			loc := shanghai
			if tc.zone != "" {
				loc = mustLocation(t, tc.zone)
			}
			got, ok, err := tc.spec.Next(from, loc)
			if err != nil {
				t.Fatal(err)
			}
			if !ok || !got.Equal(want) {
				t.Fatalf("got %v ok=%v, want %v", got, ok, want)
			}
		})
	}

	t.Run("exhausted once", func(t *testing.T) {
		from, _ := time.Parse(time.RFC3339, "2026-11-11T00:00:00Z")
		spec := ScheduleSpec{Kind: ScheduleKindOnce, Date: "2026-11-10", Times: []string{"02:30"}}
		if _, ok, err := spec.Next(from, shanghai); err != nil || ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
}

func TestNormalizeScheduleSpec(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec ScheduleSpec
	}{
		{"nil times", ScheduleSpec{Kind: ScheduleKindDaily}},
		{"bad time", ScheduleSpec{Kind: ScheduleKindDaily, Times: []string{"25:00"}}},
		{"once multiple times", ScheduleSpec{Kind: ScheduleKindOnce, Date: "2026-11-10", Times: []string{"02:00", "03:00"}}},
		{"once bad date", ScheduleSpec{Kind: ScheduleKindOnce, Date: "2026-02-30", Times: []string{"02:00"}}},
		{"weekly without days", ScheduleSpec{Kind: ScheduleKindWeekly, Times: []string{"02:00"}}},
		{"monthly bad day", ScheduleSpec{Kind: ScheduleKindMonthly, DayOfMonth: 32, Times: []string{"02:00"}}},
		{"cron bad field", ScheduleSpec{Kind: ScheduleKindCron, Cron: []string{"0 25 * * *"}}},
		{"unknown kind", ScheduleSpec{Kind: "yearly"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeScheduleSpec(&tc.spec); err == nil {
				t.Fatalf("accepted %+v", tc.spec)
			}
		})
	}

	normalized, err := NormalizeScheduleSpec(&ScheduleSpec{Kind: " DAILY ", Times: []string{"18:45", "2:30", "02:30"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(normalized.Times); got != 2 || normalized.Times[0] != "02:30" {
		t.Fatalf("times=%v", normalized.Times)
	}
}

func TestScheduleSpecNextMatchesBruteForce(t *testing.T) {
	shanghai := mustLocation(t, "Asia/Shanghai")
	specs := []ScheduleSpec{
		{Kind: ScheduleKindDaily, Times: []string{"02:30", "18:45"}},
		{Kind: ScheduleKindWeekdays, Times: []string{"23:10"}},
		{Kind: ScheduleKindWeekly, Weekdays: []int{1, 5}, Times: []string{"02:30"}},
		{Kind: ScheduleKindMonthly, DayOfMonth: 31, Times: []string{"02:30"}},
		{Kind: ScheduleKindMonthly, DayOfMonth: -1, Times: []string{"02:30"}},
		{Kind: ScheduleKindCron, Cron: []string{"*/15 * * * *"}},
		{Kind: ScheduleKindOnce, Date: "2026-12-01", Times: []string{"09:00"}},
	}
	froms := []string{"2026-01-30T23:59:00+08:00", "2026-11-03T02:31:00+08:00", "2026-12-01T08:59:00+08:00"}
	for _, spec := range specs {
		for _, raw := range froms {
			from, _ := time.Parse(time.RFC3339, raw)
			got, ok, err := spec.Next(from, shanghai)
			if err != nil {
				t.Fatalf("%+v from %s: %v", spec, raw, err)
			}
			want, wantOK := bruteForceNext(t, spec, from, shanghai)
			if ok != wantOK || (ok && !got.Equal(want)) {
				t.Fatalf("%+v from %s: got %v/%v want %v/%v", spec, raw, got, ok, want, wantOK)
			}
		}
	}
}

// bruteForceNext scans minute by minute for 45 days using only the public
// calendar interpretation, independent from Next's day loop.
func bruteForceNext(t *testing.T, spec ScheduleSpec, after time.Time, loc *time.Location) (time.Time, bool) {
	t.Helper()
	normalized, err := NormalizeScheduleSpec(&spec)
	if err != nil {
		t.Fatal(err)
	}
	spec = *normalized
	start := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	limit := start.AddDate(0, 0, 400)
	for current := start; !current.After(limit); current = current.Add(time.Minute) {
		if !bruteForceMatches(&spec, current) {
			continue
		}
		if spec.Kind == ScheduleKindOnce {
			date, _ := time.ParseInLocation("2006-01-02", spec.Date, loc)
			if date.Year() != current.Year() || date.Month() != current.Month() || date.Day() != current.Day() {
				continue
			}
		}
		return current, true
	}
	return time.Time{}, false
}

func bruteForceMatches(spec *ScheduleSpec, current time.Time) bool {
	clock := current.Format("15:04")
	switch spec.Kind {
	case ScheduleKindDaily:
		return containsString(spec.Times, clock)
	case ScheduleKindWeekdays:
		return current.Weekday() >= time.Monday && current.Weekday() <= time.Friday && containsString(spec.Times, clock)
	case ScheduleKindWeekly:
		return containsInt(spec.Weekdays, weekdayToScheduleValue(current.Weekday())) && containsString(spec.Times, clock)
	case ScheduleKindMonthly:
		day := spec.DayOfMonth
		if day == -1 {
			day = time.Date(current.Year(), current.Month()+1, 0, 0, 0, 0, 0, current.Location()).Day()
		}
		return current.Day() == day && containsString(spec.Times, clock)
	case ScheduleKindOnce:
		return containsString(spec.Times, clock)
	case ScheduleKindCron:
		for _, expression := range spec.Cron {
			parsed, err := parseCronExpression(expression)
			if err != nil {
				continue
			}
			if cronFieldMatches(parsed.Minutes, current.Minute()) &&
				cronFieldMatches(parsed.Hours, current.Hour()) &&
				cronFieldMatches(parsed.Months, int(current.Month())) &&
				cronFieldMatches(parsed.DaysOfMonth, current.Day()) &&
				cronDayOfWeekMatches(parsed.DaysOfWeek, current.Weekday()) {
				return true
			}
		}
		return false
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
